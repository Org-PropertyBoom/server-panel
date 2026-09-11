package origin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeDNS answers from a table and counts lookups per host.
type fakeDNS struct {
	mu      sync.Mutex
	answers map[string][]string
	errs    map[string]error
	calls   map[string]int
}

func (f *fakeDNS) lookup(_ context.Context, host string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[host]++
	if err := f.errs[host]; err != nil {
		return nil, err
	}
	return append([]string(nil), f.answers[host]...), nil
}

func (f *fakeDNS) count(host string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[host]
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// The review finding: a host cached as "Elsewhere" must flip to "Origin" as soon as
// its address joins the origin set, within the TTL and without re-resolving.
func TestEdgeCacheReclassifiesWhenOriginSetGrows(t *testing.T) {
	dns := &fakeDNS{answers: map[string][]string{"tenant.example": {"52.77.202.62"}}}
	var setMu sync.Mutex
	ours := map[string]bool{"52.76.123.15": true}
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	c := &EdgeCache{
		Lookup: dns.lookup,
		Ours: func() map[string]bool {
			setMu.Lock()
			defer setMu.Unlock()
			out := map[string]bool{}
			for k, v := range ours {
				out[k] = v
			}
			return out
		},
		TTL: func() time.Duration { return 6 * time.Hour },
		now: clock.now,
	}

	if got := c.Edges(context.Background(), []string{"tenant.example"})["tenant.example"]; got.Edge != EdgeElsewhere {
		t.Fatalf("before the address is ours: edge=%q, want %q", got.Edge, EdgeElsewhere)
	}

	setMu.Lock()
	ours["52.77.202.62"] = true // e.g. EC2 discovery added ppt2
	setMu.Unlock()
	clock.advance(time.Minute) // well inside the TTL

	got := c.Edges(context.Background(), []string{"tenant.example"})["tenant.example"]
	if got.Edge != EdgeOrigin {
		t.Fatalf("after the address joined the set: edge=%q, want %q (stale cached label)", got.Edge, EdgeOrigin)
	}
	if n := dns.count("tenant.example"); n != 1 {
		t.Fatalf("lookups=%d, want 1: reclassifying must reuse the cached DNS answer", n)
	}
}

func TestEdgeCacheReusesAnswersWithinTTLAndRefreshesAfter(t *testing.T) {
	dns := &fakeDNS{answers: map[string][]string{"a.example": {"52.76.29.0"}}}
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	c := &EdgeCache{
		Lookup: dns.lookup,
		Ours:   func() map[string]bool { return map[string]bool{"52.76.29.0": true} },
		TTL:    func() time.Duration { return time.Hour },
		now:    clock.now,
	}
	c.Edges(context.Background(), []string{"a.example"})
	clock.advance(59 * time.Minute)
	c.Edges(context.Background(), []string{"a.example"})
	if n := dns.count("a.example"); n != 1 {
		t.Fatalf("within TTL: lookups=%d, want 1", n)
	}
	clock.advance(2 * time.Minute)
	if got := c.Edges(context.Background(), []string{"a.example"})["a.example"]; got.Edge != EdgeOrigin {
		t.Fatalf("after refresh: edge=%q, want %q", got.Edge, EdgeOrigin)
	}
	if n := dns.count("a.example"); n != 2 {
		t.Fatalf("after TTL: lookups=%d, want 2", n)
	}
}

func TestEdgeCacheCachesLookupErrors(t *testing.T) {
	dns := &fakeDNS{errs: map[string]error{"dead.example": errors.New("no such host")}}
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	c := &EdgeCache{Lookup: dns.lookup, TTL: func() time.Duration { return time.Hour }, now: clock.now}
	for i := 0; i < 3; i++ {
		if got := c.Edges(context.Background(), []string{"dead.example"})["dead.example"]; got.Edge != EdgeNXDomain {
			t.Fatalf("call %d: edge=%q, want %q", i, got.Edge, EdgeNXDomain)
		}
	}
	if n := dns.count("dead.example"); n != 1 {
		t.Fatalf("lookups=%d, want 1: a failed lookup is cached like any answer", n)
	}
}

func TestEdgeCacheNormalisesAndDedupesHosts(t *testing.T) {
	dns := &fakeDNS{answers: map[string][]string{"mixed.example": {"104.21.69.82"}}}
	c := &EdgeCache{Lookup: dns.lookup, TTL: func() time.Duration { return time.Hour }}
	out := c.Edges(context.Background(), []string{" Mixed.Example ", "mixed.example", "MIXED.EXAMPLE", "", "   "})
	if len(out) != 1 {
		t.Fatalf("got %d entries %v, want exactly one (normalised key, blanks skipped)", len(out), out)
	}
	if got := out["mixed.example"]; got.Edge != EdgeCloudflare {
		t.Fatalf("edge=%q, want %q", got.Edge, EdgeCloudflare)
	}
	if n := dns.count("mixed.example"); n != 1 {
		t.Fatalf("lookups=%d, want 1 for duplicate spellings of one host", n)
	}
}
