package health

import (
	"context"
	"testing"
	"time"
)

func TestApplyResult_DebouncesAlert(t *testing.T) {
	p := New(Config{Threshold: 2})
	// Two consecutive failures are needed before Alert flips (anti-flap).
	p.applyResult("a.com", probeResult{dnsOk: false, err: "does not resolve to this server"})
	if s := p.Snapshot()["a.com"]; s.Alert || s.Failures != 1 {
		t.Fatalf("after 1 failure: alert=%v failures=%d, want alert=false failures=1", s.Alert, s.Failures)
	}
	p.applyResult("a.com", probeResult{dnsOk: false, err: "does not resolve to this server"})
	if s := p.Snapshot()["a.com"]; !s.Alert || s.Failures != 2 {
		t.Fatalf("after 2 failures: alert=%v failures=%d, want alert=true failures=2", s.Alert, s.Failures)
	}
	// A success clears both the counter and the alert.
	p.applyResult("a.com", probeResult{dnsOk: true, tlsOk: true, resolvedIPs: []string{"1.2.3.4"}})
	if s := p.Snapshot()["a.com"]; s.Alert || s.Failures != 0 || s.LastError != "" {
		t.Fatalf("after recovery: alert=%v failures=%d err=%q, want reset", s.Alert, s.Failures, s.LastError)
	}
}

func TestApplyResult_DNSOkButTLSFailIsUnreachable(t *testing.T) {
	p := New(Config{Threshold: 1})
	p.applyResult("b.com", probeResult{dnsOk: true, tlsOk: false, err: "TLS: certificate expired"})
	s := p.Snapshot()["b.com"]
	if !s.Alert || s.DNSOk != true || s.TLSOk != false || s.LastError == "" {
		t.Fatalf("dns-ok tls-fail should alert with the tls reason; got %+v", s)
	}
}

func TestProbeAll_PrunesRemovedHostsAndReportsAlert(t *testing.T) {
	hosts := []string{"live.com", "dead.com"}
	p := New(Config{
		Threshold: 1,
		PinnedIPs: []string{"9.9.9.9"}, // non-empty → referenceIPs set, cycle runs
		Hosts:     func(context.Context) ([]string, error) { return hosts, nil },
	})
	p.probeFn = func(_ context.Context, host string, _ map[string]bool) probeResult {
		if host == "live.com" {
			return probeResult{dnsOk: true, tlsOk: true, resolvedIPs: []string{"9.9.9.9"}, certExpiry: time.Unix(2000000000, 0)}
		}
		return probeResult{dnsOk: false, err: "does not resolve to this server"}
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

func TestProbeAll_SkipsWhenNoReferenceIPs(t *testing.T) {
	probed := false
	p := New(Config{
		Hosts: func(context.Context) ([]string, error) { return []string{"x.com"}, nil },
		// no PinnedIPs and no ReferenceDomains → referenceIPs empty → skip cycle
	})
	p.probeFn = func(_ context.Context, _ string, _ map[string]bool) probeResult {
		probed = true
		return probeResult{}
	}
	p.probeAll(context.Background())
	if probed {
		t.Error("must NOT probe when our own IPs can't be determined (avoid a false unreachable storm)")
	}
	if len(p.Snapshot()) != 0 {
		t.Error("no statuses should be recorded when the cycle is skipped")
	}
}
