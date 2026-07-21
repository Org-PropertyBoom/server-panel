// Package health is an ALERT-ONLY reachability probe for tenant hosts. It answers
// the question the reconcile engine cannot: "is this domain actually pointing at
// us and serving a valid cert right now?" — orthogonal to the DB-vs-file "in sync"
// signal.
//
// SAFETY: this package NEVER writes a file, removes a vhost, or touches Caddy. It
// only performs outbound DNS + TLS reads and records a status. Nothing here can
// deactivate, prune, or reconcile anything — a flaky lookup must never tear down a
// live customer's config. It is wired in read-only and reported as a warning chip.
package health

import (
	"context"
	"crypto/tls"
	"net"
	"sort"
	"sync"
	"time"
)

// Status is the JSON-facing health of one host. Times are epoch-millis (0 =
// unknown) for clean marshaling and easy `new Date(ms)` on the client.
type Status struct {
	Host         string   `json:"host"`
	Alert        bool     `json:"alert"` // debounced "not reaching us" (failures >= threshold)
	DNSOk        bool     `json:"dnsOk"`
	TLSOk        bool     `json:"tlsOk"`
	ResolvedIPs  []string `json:"resolvedIps,omitempty"`
	CertExpiryMs int64    `json:"certExpiryMs,omitempty"`
	LastError    string   `json:"lastError,omitempty"`
	Failures     int      `json:"failures"`
	CheckedAtMs  int64    `json:"checkedAtMs"`
}

// probeResult is one raw check outcome (pre-debounce).
type probeResult struct {
	dnsOk       bool
	resolvedIPs []string
	tlsOk       bool
	certExpiry  time.Time
	err         string
}

// Config tunes the prober. Zero values fall back to safe defaults in New.
type Config struct {
	Interval    time.Duration
	Threshold   int // consecutive failures before Alert flips true (anti-flap)
	Concurrency int // max simultaneous probes
	DNSTimeout  time.Duration
	TLSTimeout  time.Duration
	PinnedIPs   []string // explicit "our" IPs; when empty they're derived from ReferenceDomains
	// ReferenceDomains are domains KNOWN to point at us (the protected dashboard/panel
	// domains). Their resolved IPs define "our server" so a tenant host is judged
	// against the same target — no hardcoded IP, and it follows a server move.
	ReferenceDomains []string
	// Hosts returns the current active tenant hostnames to probe.
	Hosts func(context.Context) ([]string, error)
}

// Prober periodically probes hosts and holds the latest status per host.
type Prober struct {
	cfg      Config
	mu       sync.RWMutex
	statuses map[string]*Status
	now      func() time.Time
	resolver *net.Resolver
	// probeFn is the per-host check; overridable in tests to avoid real network.
	probeFn func(ctx context.Context, host string, refIPs map[string]bool) probeResult
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
		cfg:      cfg,
		statuses: map[string]*Status{},
		now:      time.Now,
		resolver: &net.Resolver{},
	}
	p.probeFn = p.realProbe
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

// probeAll resolves the reference (our-server) IPs, prunes statuses for hosts no
// longer active, and probes the current host set with bounded concurrency. If our
// own IPs can't be determined, it SKIPS the cycle rather than flag everything —
// never raise a false "unreachable" storm from our own DNS hiccup.
func (p *Prober) probeAll(ctx context.Context) {
	if p.cfg.Hosts == nil {
		return
	}
	hosts, err := p.cfg.Hosts(ctx)
	if err != nil {
		return
	}
	refIPs := p.referenceIPs(ctx)
	if len(refIPs) == 0 {
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
			pr := p.probeFn(ctx, host, refIPs)
			p.mu.Lock()
			p.applyResult(host, pr)
			p.mu.Unlock()
		}(h)
	}
	wg.Wait()
}

// applyResult folds one raw probe into the debounced status. Caller holds p.mu.
func (p *Prober) applyResult(host string, pr probeResult) {
	st := p.statuses[host]
	if st == nil {
		st = &Status{Host: host}
		p.statuses[host] = st
	}
	st.DNSOk = pr.dnsOk
	st.TLSOk = pr.tlsOk
	st.ResolvedIPs = pr.resolvedIPs
	st.CheckedAtMs = p.now().UnixMilli()
	if !pr.certExpiry.IsZero() {
		st.CertExpiryMs = pr.certExpiry.UnixMilli()
	}
	if pr.dnsOk && pr.tlsOk {
		st.Failures = 0
		st.LastError = ""
	} else {
		st.Failures++
		st.LastError = pr.err
	}
	st.Alert = st.Failures >= p.cfg.Threshold
}

// referenceIPs is the set of IPs that count as "us": the pinned override, else the
// union of the reference domains' A/AAAA records.
func (p *Prober) referenceIPs(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if len(p.cfg.PinnedIPs) > 0 {
		for _, ip := range p.cfg.PinnedIPs {
			out[ip] = true
		}
		return out
	}
	for _, dom := range p.cfg.ReferenceDomains {
		if dom == "" {
			continue
		}
		c, cancel := context.WithTimeout(ctx, p.cfg.DNSTimeout)
		ips, err := p.resolver.LookupHost(c, dom)
		cancel()
		if err != nil {
			continue
		}
		for _, ip := range ips {
			out[ip] = true
		}
	}
	return out
}

// realProbe checks DNS (does the host resolve to one of our IPs?) then, only when
// DNS points here, TLS (is a valid, unexpired cert served for this host?).
func (p *Prober) realProbe(ctx context.Context, host string, refIPs map[string]bool) probeResult {
	var pr probeResult
	c, cancel := context.WithTimeout(ctx, p.cfg.DNSTimeout)
	ips, err := p.resolver.LookupHost(c, host)
	cancel()
	if err != nil {
		pr.err = "DNS: " + err.Error()
		return pr
	}
	sort.Strings(ips)
	pr.resolvedIPs = ips
	for _, ip := range ips {
		if refIPs[ip] {
			pr.dnsOk = true
			break
		}
	}
	if !pr.dnsOk {
		pr.err = "does not resolve to this server"
		return pr
	}

	d := &net.Dialer{Timeout: p.cfg.TLSTimeout}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(host, "443"), &tls.Config{ServerName: host, InsecureSkipVerify: true}) //nolint:gosec // cert inspected manually below
	if err != nil {
		pr.err = "TLS: " + err.Error()
		return pr
	}
	defer conn.Close()
	cs := conn.ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		pr.err = "TLS: no certificate presented"
		return pr
	}
	leaf := cs.PeerCertificates[0]
	pr.certExpiry = leaf.NotAfter
	if p.now().After(leaf.NotAfter) {
		pr.err = "TLS: certificate expired"
		return pr
	}
	if leaf.VerifyHostname(host) != nil {
		pr.err = "TLS: certificate does not cover this host"
		return pr
	}
	pr.tlsOk = true
	return pr
}
