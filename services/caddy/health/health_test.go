package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The four origin addresses (all serve this Caddy's certificate).
var origins = []string{"52.76.29.0", "52.76.123.15", "3.1.252.222", "52.77.202.62"}

func originSet() map[string]bool {
	m := map[string]bool{}
	for _, ip := range origins {
		m[ip] = true
	}
	return m
}

// fakeNet answers DNS from a table and TLS per IP, recording what was touched.
type fakeNet struct {
	mu      sync.Mutex
	dns     map[string][]string // host → answers; missing → NXDOMAIN
	tlsErr  map[string]error    // ip → TLS failure; missing → valid cert
	looked  []string
	dialled []string
}

func (f *fakeNet) lookup(_ context.Context, host string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.looked = append(f.looked, host)
	ips, ok := f.dns[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return append([]string(nil), ips...), nil
}

func (f *fakeNet) tls(_ context.Context, host, ip string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dialled = append(f.dialled, host+"@"+ip)
	if err := f.tlsErr[ip]; err != nil {
		return time.Time{}, err
	}
	return time.Unix(2000000000, 0), nil
}

func newProber(threshold int, hosts []string, f *fakeNet) *Prober {
	p := New(Config{
		Threshold: threshold,
		OriginIPs: originSet,
		Hosts:     func(context.Context) ([]string, error) { return hosts, nil },
	})
	p.lookupHost = f.lookup
	p.checkTLS = f.tls
	return p
}

func TestOriginHostsWithGoodTLSAreNotAlerted(t *testing.T) {
	f := &fakeNet{dns: map[string][]string{}}
	var hosts []string
	for i, ip := range origins {
		h := "tenant" + string(rune('a'+i)) + ".com"
		hosts = append(hosts, h)
		f.dns[h] = []string{ip}
	}
	p := newProber(1, hosts, f)
	for i := 0; i < 3; i++ {
		p.probeAll(context.Background())
	}
	for _, h := range hosts {
		s := p.Snapshot()[h]
		if s.Alert || !s.OnOrigin || !s.TLSOk || s.Failures != 0 {
			t.Errorf("%s (%v) should be a healthy origin host, got %+v", h, s.ResolvedIPs, s)
		}
	}
	if p.AlertCount() != 0 {
		t.Errorf("AlertCount = %d, want 0", p.AlertCount())
	}
}

func TestOriginHostWithFailingTLSAlertsAfterThreshold(t *testing.T) {
	for _, reason := range []string{"connection refused", "no certificate presented", "certificate expired", "certificate does not cover this host"} {
		f := &fakeNet{
			dns:    map[string][]string{"broken.com": {"52.77.202.62"}},
			tlsErr: map[string]error{"52.77.202.62": errors.New(reason)},
		}
		p := newProber(2, []string{"broken.com"}, f)
		p.probeAll(context.Background())
		if s := p.Snapshot()["broken.com"]; s.Alert || s.Failures != 1 {
			t.Fatalf("%s: after 1 failure want no alert (debounce), got %+v", reason, s)
		}
		p.probeAll(context.Background())
		s := p.Snapshot()["broken.com"]
		if !s.Alert || s.Failures != 2 || !s.OnOrigin || s.TLSOk {
			t.Fatalf("%s: after 2 failures want alert, got %+v", reason, s)
		}
		if !strings.Contains(s.LastError, "52.77.202.62") || !strings.Contains(s.LastError, reason) {
			t.Errorf("%s: lastError should name the IP and reason, got %q", reason, s.LastError)
		}
	}
}

func TestCloudflareElsewhereAndNXDomainAreNotAlerted(t *testing.T) {
	f := &fakeNet{dns: map[string][]string{
		"proxied.com":   {"104.21.69.82", "172.67.206.132"}, // Cloudflare
		"elsewhere.com": {"8.8.8.8"},                        // someone else's server
		"v6only.com":    {"2606:4700::6810:4552"},
		// "gone.com" absent → NXDOMAIN
	}}
	hosts := []string{"proxied.com", "elsewhere.com", "v6only.com", "gone.com"}
	p := newProber(1, hosts, f)
	for i := 0; i < 3; i++ {
		p.probeAll(context.Background())
	}
	for _, h := range hosts {
		s := p.Snapshot()[h]
		if s.Alert || s.OnOrigin || s.Failures != 0 || s.LastError != "" {
			t.Errorf("%s must not be judged or alerted, got %+v", h, s)
		}
	}
	if len(f.dialled) != 0 {
		t.Errorf("no TLS check may run for non-origin hosts, dialled %v", f.dialled)
	}
	if p.AlertCount() != 0 {
		t.Errorf("AlertCount = %d, want 0", p.AlertCount())
	}
}

// The panel domain being proxied by Cloudflare must change nothing: "ours" comes
// from the origin set, never from the panel domain's DNS.
func TestPanelDomainBehindCloudflareChangesNothing(t *testing.T) {
	f := &fakeNet{
		dns: map[string][]string{
			"cp.propertyweb.co": {"104.21.69.82", "172.67.206.132"}, // the panel domain
			"origin.com":        {"52.76.123.15"},
			"samezone.com":      {"104.21.69.82"}, // Cloudflare tenant on the panel zone's pair
		},
		tlsErr: map[string]error{"104.21.69.82": errors.New("should never be dialled")},
	}
	var seen map[string]bool
	p := newProber(1, []string{"origin.com", "samezone.com"}, f)
	p.probeFn = func(ctx context.Context, host string, ours map[string]bool) probeResult {
		seen = ours
		return p.realProbe(ctx, host, ours)
	}
	p.probeAll(context.Background())

	for _, h := range f.looked {
		if h == "cp.propertyweb.co" {
			t.Error("the panel domain must never be resolved to define 'our' IPs")
		}
	}
	if seen["104.21.69.82"] || seen["172.67.206.132"] || len(seen) != 4 {
		t.Errorf("'ours' must be exactly the 4 origins, got %v", seen)
	}
	snap := p.Snapshot()
	if s := snap["origin.com"]; s.Alert || !s.OnOrigin || !s.TLSOk {
		t.Errorf("origin tenant must be checked and healthy, got %+v", s)
	}
	if s := snap["samezone.com"]; s.Alert || s.OnOrigin {
		t.Errorf("Cloudflare tenant on the panel zone's IPs must not be judged, got %+v", s)
	}
}

// A host with one origin record and one foreign record: only our IP is dialled.
func TestMixedAnswerDialsOnlyOurIPs(t *testing.T) {
	f := &fakeNet{dns: map[string][]string{"mixed.com": {"8.8.8.8", "52.77.202.62"}}}
	p := newProber(1, []string{"mixed.com"}, f)
	p.probeAll(context.Background())
	if len(f.dialled) != 1 || f.dialled[0] != "mixed.com@52.77.202.62" {
		t.Errorf("want only our IP dialled, got %v", f.dialled)
	}
}

// A host that moves off our origin clears its pending failures.
func TestLeavingOriginResetsDebounce(t *testing.T) {
	f := &fakeNet{
		dns:    map[string][]string{"moving.com": {"52.76.29.0"}},
		tlsErr: map[string]error{"52.76.29.0": errors.New("connection refused")},
	}
	p := newProber(2, []string{"moving.com"}, f)
	p.probeAll(context.Background())
	f.mu.Lock()
	f.dns["moving.com"] = []string{"104.21.69.82"}
	f.mu.Unlock()
	p.probeAll(context.Background())
	if s := p.Snapshot()["moving.com"]; s.Alert || s.Failures != 0 {
		t.Errorf("a host now on Cloudflare must not carry origin failures, got %+v", s)
	}
}

func TestApplyResult_DebouncesAlert(t *testing.T) {
	p := New(Config{Threshold: 2})
	fail := probeResult{dnsOk: true, onOrigin: true, err: "TLS 52.76.29.0: certificate expired"}
	p.applyResult("a.com", fail)
	if s := p.Snapshot()["a.com"]; s.Alert || s.Failures != 1 {
		t.Fatalf("after 1 failure: alert=%v failures=%d, want alert=false failures=1", s.Alert, s.Failures)
	}
	p.applyResult("a.com", fail)
	if s := p.Snapshot()["a.com"]; !s.Alert || s.Failures != 2 {
		t.Fatalf("after 2 failures: alert=%v failures=%d, want alert=true failures=2", s.Alert, s.Failures)
	}
	// A success clears both the counter and the alert.
	p.applyResult("a.com", probeResult{dnsOk: true, onOrigin: true, tlsOk: true, resolvedIPs: []string{"52.76.29.0"}})
	if s := p.Snapshot()["a.com"]; s.Alert || s.Failures != 0 || s.LastError != "" {
		t.Fatalf("after recovery: alert=%v failures=%d err=%q, want reset", s.Alert, s.Failures, s.LastError)
	}
}

func TestProbeAll_PrunesRemovedHostsAndReportsAlert(t *testing.T) {
	hosts := []string{"live.com", "dead.com"}
	p := New(Config{
		Threshold: 1,
		OriginIPs: originSet,
		Hosts:     func(context.Context) ([]string, error) { return hosts, nil },
	})
	p.probeFn = func(_ context.Context, host string, _ map[string]bool) probeResult {
		if host == "live.com" {
			return probeResult{dnsOk: true, onOrigin: true, tlsOk: true, resolvedIPs: []string{"52.76.123.15"}, certExpiry: time.Unix(2000000000, 0)}
		}
		return probeResult{dnsOk: true, onOrigin: true, err: "TLS 52.76.123.15: connection refused"}
	}

	p.probeAll(context.Background())
	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 statuses, got %d", len(snap))
	}
	if snap["live.com"].Alert {
		t.Error("live.com must not alert")
	}
	if !snap["dead.com"].Alert {
		t.Error("dead.com must alert")
	}
	if p.AlertCount() != 1 {
		t.Errorf("AlertCount = %d, want 1", p.AlertCount())
	}

	// dead.com is deactivated (removed from the host list) — its status is pruned,
	// so a removed host never lingers as a stale alert.
	hosts = []string{"live.com"}
	p.probeAll(context.Background())
	snap = p.Snapshot()
	if _, ok := snap["dead.com"]; ok {
		t.Error("dead.com should be pruned once it leaves the active host set")
	}
	if p.AlertCount() != 0 {
		t.Errorf("AlertCount after prune = %d, want 0", p.AlertCount())
	}
}

func TestProbeAll_SkipsWhenNoOriginIPs(t *testing.T) {
	for name, src := range map[string]func() map[string]bool{
		"nil source":   nil,
		"empty set":    func() map[string]bool { return map[string]bool{} },
		"garbage only": func() map[string]bool { return map[string]bool{"not-an-ip": true} },
	} {
		probed := false
		p := New(Config{
			OriginIPs: src,
			Hosts:     func(context.Context) ([]string, error) { return []string{"x.com"}, nil },
		})
		p.probeFn = func(_ context.Context, _ string, _ map[string]bool) probeResult {
			probed = true
			return probeResult{}
		}
		p.probeAll(context.Background())
		if probed {
			t.Errorf("%s: must NOT probe when our own IPs can't be determined", name)
		}
		if len(p.Snapshot()) != 0 {
			t.Errorf("%s: no statuses should be recorded when the cycle is skipped", name)
		}
	}
}

// realCheckTLS against a loopback TLS server (no external network). httptest's
// cert covers example.com and 127.0.0.1 and expires in 2084.
func TestRealCheckTLS_AgainstLoopbackServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	p := New(Config{TLSTimeout: 2 * time.Second})
	p.tlsPort = port

	if exp, err := p.checkTLS(context.Background(), "example.com", "127.0.0.1"); err != nil || exp.IsZero() {
		t.Errorf("valid cert: exp=%v err=%v", exp, err)
	}
	if _, err := p.checkTLS(context.Background(), "tenant.example.org", "127.0.0.1"); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Errorf("wrong host: want 'does not cover', got %v", err)
	}
	p.now = func() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }
	if _, err := p.checkTLS(context.Background(), "example.com", "127.0.0.1"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("expired: want 'expired', got %v", err)
	}

	srv.Close()
	p.now = time.Now
	if _, err := p.checkTLS(context.Background(), "example.com", "127.0.0.1"); err == nil {
		t.Error("closed port: want a connect error")
	}
}
