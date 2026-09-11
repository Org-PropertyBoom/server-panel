package origin

import (
	"context"
	"strings"
	"sync"
	"time"
)

// EdgeCache caches each host's raw DNS answer (the slow part) and classifies it
// against the CURRENT origin set on every read.
//
// Caching the classification itself froze a host's label for the whole TTL: an
// origin address added later (EC2 discovery, a config change) left its tenants
// showing "Elsewhere" for hours, while the reachability probe, which reads the set
// every cycle, already judged them ours. Classifying at read time keeps the Edge
// column and the probe on the same definition of "ours" with no invalidation step.
type EdgeCache struct {
	// Lookup resolves one host (net.DefaultResolver.LookupHost in production).
	Lookup func(ctx context.Context, host string) ([]string, error)
	// Ours returns the current origin-IP set. Read on every Edges call.
	Ours func() map[string]bool
	// TTL returns how long a DNS answer is reused; zero or nil means 6h.
	TTL func() time.Duration
	// LookupTimeout bounds each lookup; zero means 5s.
	LookupTimeout time.Duration
	// Workers caps concurrent lookups; zero means 16.
	Workers int

	now     func() time.Time // test seam; nil means time.Now
	mu      sync.Mutex
	answers map[string]dnsAnswer
}

type dnsAnswer struct {
	addrs []string
	err   error
	at    time.Time
}

// Classified is one host's Edge label plus the IPs worth showing.
type Classified struct {
	Edge string
	IPs  []string
}

// Edges classifies every non-empty host (keys lower-cased and trimmed). DNS answers
// younger than TTL are reused; the rest are resolved concurrently. Lookup errors are
// cached too, so dead hosts don't re-resolve on every page view.
func (c *EdgeCache) Edges(ctx context.Context, hosts []string) map[string]Classified {
	now := c.clock()
	ttl := c.ttl()

	got := map[string]dnsAnswer{}
	queued := map[string]bool{}
	var pending []string

	c.mu.Lock()
	if c.answers == nil {
		c.answers = map[string]dnsAnswer{}
	}
	for _, h := range hosts {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" || queued[key] {
			continue
		}
		if _, done := got[key]; done {
			continue
		}
		if a, ok := c.answers[key]; ok && now().Sub(a.at) < ttl {
			got[key] = a
			continue
		}
		queued[key] = true
		pending = append(pending, key)
	}
	c.mu.Unlock()

	if len(pending) > 0 {
		workers := c.Workers
		if workers <= 0 {
			workers = 16
		}
		timeout := c.LookupTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		var wg sync.WaitGroup
		var gotMu sync.Mutex
		limit := make(chan struct{}, workers)
		for _, h := range pending {
			wg.Add(1)
			go func(host string) {
				defer wg.Done()
				limit <- struct{}{}
				lookupCtx, cancel := context.WithTimeout(ctx, timeout)
				addrs, err := c.Lookup(lookupCtx, host)
				cancel()
				<-limit
				a := dnsAnswer{addrs: addrs, err: err, at: now()}
				gotMu.Lock()
				got[host] = a
				gotMu.Unlock()
				c.mu.Lock()
				c.answers[host] = a
				c.mu.Unlock()
			}(h)
		}
		wg.Wait()
	}

	ours := map[string]bool{}
	if c.Ours != nil {
		ours = c.Ours()
	}
	out := make(map[string]Classified, len(got))
	for host, a := range got {
		edge, ips := ClassifyEdge(a.addrs, a.err, ours)
		out[host] = Classified{Edge: edge, IPs: ips}
	}
	return out
}

func (c *EdgeCache) clock() func() time.Time {
	if c.now != nil {
		return c.now
	}
	return time.Now
}

func (c *EdgeCache) ttl() time.Duration {
	if c.TTL != nil {
		if d := c.TTL(); d > 0 {
			return d
		}
	}
	return 6 * time.Hour
}
