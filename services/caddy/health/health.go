// Package health is an ALERT-ONLY reachability probe for tenant hosts. It answers
// the question the reconcile engine cannot: "a host that points at our origin — is
// our origin actually serving it a valid cert right now?" — orthogonal to the
// DB-vs-file "in sync" signal.
//
// Only hosts whose DNS resolves to one of OUR origin IPs are judged. Hosts on
// Cloudflare, elsewhere, or NXDOMAIN are not "unreachable": the Edge column shows
// where they point, and their TLS (if any) is not ours to test.
//
// SAFETY: this package NEVER writes a file, removes a vhost, or touches Caddy. It
// only performs outbound DNS + TLS reads and records a status. Nothing here can
// deactivate, prune, or reconcile anything — a flaky lookup must never tear down a
// live customer's config. It is wired in read-only and reported as a warning chip.
package health

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is the JSON-facing health of one host. Times are epoch-millis (0 =
// unknown) for clean marshaling and easy `new Date(ms)` on the client.
type Status struct {
	Host         string   `json:"host"`
	Alert        bool     `json:"alert"`    // debounced "not reaching us" (failures >= threshold)
	DNSOk        bool     `json:"dnsOk"`    // the host resolved to at least one address
	OnOrigin     bool     `json:"onOrigin"` // ...and one of them is ours, so TLS was checked
	TLSOk        bool     `json:"tlsOk"`    // every origin IP it resolves to served a valid cert
	ResolvedIPs  []string `json:"resolvedIps,omitempty"`
	CertExpiryMs int64    `json:"certExpiryMs,omitempty"`
	LastError    string   `json:"lastError,omitempty"`
	Failures     int      `json:"failures"`
	CheckedAtMs  int64    `json:"checkedAtMs"`
}

// probeResult is one raw check outcome (pre-debounce).
type probeResult struct {
	dnsOk       bool
	onOrigin    bool
	resolvedIPs []string
	tlsOk       bool
	certExpiry  time.Time
	err         string

	// indeterminate: the DNS lookup failed for a reason other than "no such host"
	// (timeout, SERVFAIL, cancelled). Nothing was observed, so applyResult leaves
	// the debounced status exactly as it was.
	indeterminate bool
}

// Config tunes the prober. Zero values fall back to safe defaults in New.
type Config struct {
	Interval    time.Duration
	Threshold   int // consecutive failures before Alert flips true (anti-flap)
	Concurrency int // max simultaneous probes
	DNSTimeout  time.Duration
	TLSTimeout  time.Duration
	// OriginIPs returns the current set of our origin addresses (services/origin).
	// Read every cycle so late discovery is picked up. Empty → the cycle is skipped.
	// Never derive this from the panel domain's DNS: it's behind Cloudflare.
	OriginIPs func() map[string]bool
	// Hosts returns the current active tenant hostnames to probe.
	Hosts func(context.Context) ([]string, error)
}

// Prober periodically probes hosts and holds the latest status per host.
type Prober struct {
	cfg      Config
	mu       sync.RWMutex
	statuses map[string]*Status
	now      func() time.Time
	// probeFn is the per-host check; overridable in tests to avoid real network.
	probeFn func(ctx context.Context, host string, ours map[string]bool) probeResult
	// lookupHost and checkTLS are realProbe's network seams, also test-overridable.
	lookupHost func(ctx context.Context, host string) ([]string, error)
	checkTLS   func(ctx context.Context, host, ip string) (time.Time, error)
	tlsPort    string
}

// New builds a Prober with defaults applied.
func New(cfg Config) *Prober {
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Minute
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 2
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 12
	}
	if cfg.DNSTimeout <= 0 {
		cfg.DNSTimeout = 4 * time.Second
	}
	if cfg.TLSTimeout <= 0 {
		cfg.TLSTimeout = 6 * time.Second
	}
	p := &Prober{
		cfg:        cfg,
		statuses:   map[string]*Status{},
		now:        time.Now,
		lookupHost: (&net.Resolver{}).LookupHost,
		tlsPort:    "443",
	}
	p.probeFn = p.realProbe
	p.checkTLS = p.realCheckTLS
	return p
}

// Snapshot returns a copy of the current per-host statuses (safe to marshal).
func (p *Prober) Snapshot() map[string]Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]Status, len(p.statuses))
	for h, s := range p.statuses {
		out[h] = *s
	}
	return out
}

// AlertCount returns how many hosts are currently in the debounced alert state.
func (p *Prober) AlertCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for _, s := range p.statuses {
		if s.Alert {
			n++
		}
	}
	return n
}

// Run probes immediately, then every Interval until ctx is cancelled.
func (p *Prober) Run(ctx context.Context) {
	p.probeAll(ctx)
	t := time.NewTicker(p.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.probeAll(ctx)
		}
	}
}

// probeAll reads our origin IPs, prunes statuses for hosts no longer active, and
// probes the current host set with bounded concurrency. If our own IPs can't be
// determined, it SKIPS the cycle rather than judge anything against an empty set.
func (p *Prober) probeAll(ctx context.Context) {
	if p.cfg.Hosts == nil {
		return
	}
	hosts, err := p.cfg.Hosts(ctx)
	if err != nil {
		return
	}
	ours := p.originIPs()
	if len(ours) == 0 {
		return // can't establish "our" IPs — do not judge this cycle
	}

	active := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		active[h] = true
	}
	p.mu.Lock()
	for h := range p.statuses {
		if !active[h] {
			delete(p.statuses, h)
		}
	}
	p.mu.Unlock()

	sem := make(chan struct{}, p.cfg.Concurrency)
	var wg sync.WaitGroup
	for _, h := range hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(host string) {
			defer wg.Done()
			defer func() { <-sem }()
			pr := p.probeFn(ctx, host, ours)
			p.mu.Lock()
			p.applyResult(host, pr)
			p.mu.Unlock()
		}(h)
	}
	wg.Wait()
}

// applyResult folds one raw probe into the debounced status. Caller holds p.mu.
// Only an on-origin host with a failed TLS check counts as a failure; a host that
// doesn't point at us is not judged, so its counter and alert reset. A lookup that
// observed nothing (pr.indeterminate) leaves the status untouched.
func (p *Prober) applyResult(host string, pr probeResult) {
	st := p.statuses[host]
	if st == nil {
		st = &Status{Host: host}
		p.statuses[host] = st
	}
	if pr.indeterminate {
		// A resolver hiccup is not a verdict: it must neither raise nor clear an
		// alert, so failures, alert, last error and the check time all stand.
		return
	}
	st.DNSOk = pr.dnsOk
	st.OnOrigin = pr.onOrigin
	st.TLSOk = pr.tlsOk
	st.ResolvedIPs = pr.resolvedIPs
	st.CheckedAtMs = p.now().UnixMilli()
	if !pr.certExpiry.IsZero() {
		st.CertExpiryMs = pr.certExpiry.UnixMilli()
	}
	if pr.onOrigin && !pr.tlsOk {
		st.Failures++
		st.LastError = pr.err
	} else {
		st.Failures = 0
		st.LastError = ""
	}
	st.Alert = st.Failures >= p.cfg.Threshold
}

// originIPs is a copy of the configured set, canonicalised so lookups match.
func (p *Prober) originIPs() map[string]bool {
	out := map[string]bool{}
	if p.cfg.OriginIPs == nil {
		return out
	}
	for ip := range p.cfg.OriginIPs() {
		if c := canonical(ip); c != "" {
			out[c] = true
		}
	}
	return out
}

// realProbe resolves the host; if any answer is one of our origin IPs it TLS-checks
// EACH such IP directly (SNI = host), so the result says whether our origin serves
// this host, not whatever box a non-origin record happens to point at.
func (p *Prober) realProbe(ctx context.Context, host string, ours map[string]bool) probeResult {
	var pr probeResult
	c, cancel := context.WithTimeout(ctx, p.cfg.DNSTimeout)
	ips, err := p.lookupHost(c, host)
	cancel()
	if err != nil {
		pr.err = "DNS: " + err.Error()
		// A definitive "no such host" means the host doesn't point at us (the Edge
		// column's NXDOMAIN): not ours to judge, so the debounce resets. Any other
		// lookup failure (timeout, SERVFAIL, cancelled) observed nothing.
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
			pr.indeterminate = true
		}
		return pr
	}
	sort.Strings(ips)
	pr.resolvedIPs = ips
	pr.dnsOk = len(ips) > 0

	var mine []string
	for _, ip := range ips {
		if ours[canonical(ip)] {
			mine = append(mine, ip)
		}
	}
	if len(mine) == 0 {
		return pr // Cloudflare / elsewhere: not ours to judge
	}
	pr.onOrigin = true

	var errs []string
	for _, ip := range mine {
		expiry, err := p.checkTLS(ctx, host, ip)
		if !expiry.IsZero() && (pr.certExpiry.IsZero() || expiry.Before(pr.certExpiry)) {
			pr.certExpiry = expiry
		}
		if err != nil {
			errs = append(errs, "TLS "+ip+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		pr.err = strings.Join(errs, "; ")
		return pr
	}
	pr.tlsOk = true
	return pr
}

// realCheckTLS dials ip:443 with SNI=host and inspects the leaf: present, unexpired,
// and covering host. Returns the leaf's expiry whenever a cert was seen.
func (p *Prober) realCheckTLS(ctx context.Context, host, ip string) (time.Time, error) {
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: p.cfg.TLSTimeout},
		Config:    &tls.Config{ServerName: host, InsecureSkipVerify: true}, //nolint:gosec // cert inspected manually below
	}
	c, cancel := context.WithTimeout(ctx, p.cfg.TLSTimeout)
	defer cancel()
	conn, err := d.DialContext(c, "tcp", net.JoinHostPort(ip, p.tlsPort))
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	cs := conn.(*tls.Conn).ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		return time.Time{}, errors.New("no certificate presented")
	}
	leaf := cs.PeerCertificates[0]
	if p.now().After(leaf.NotAfter) {
		return leaf.NotAfter, errors.New("certificate expired")
	}
	if leaf.VerifyHostname(host) != nil {
		return leaf.NotAfter, errors.New("certificate does not cover this host")
	}
	return leaf.NotAfter, nil
}

func canonical(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return ""
	}
	return ip.String()
}
