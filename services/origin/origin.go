// Package origin is the ONE definition of "our origin IPs": the public addresses
// that serve this panel's Caddy. Both the Edge column (services/dns_probe.go) and
// the reachability probe (services/caddy/health) read it, so they can never again
// disagree about what "here" is.
//
// The set is the union of:
//   - the configured list (env, else DefaultIPs), and
//   - addresses discovered from EC2 instance metadata (optional, see imds.go).
//
// Discovery only ever ADDS. Metadata knows this instance's addresses, but origin
// addresses may live on another box that shares certificate storage and config, so
// a metadata-only list would wrongly disown their tenants.
//
// It is NEVER derived from the panel domain's DNS: that domain sits behind
// Cloudflare, so its A records are Cloudflare's edge, not us.
//
// Pure package (no cgo, no Linux-only syscalls) so it tests on any OS.
package origin

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultIPs are the four origin addresses that all serve this Caddy's certificate
// (identical serial on each, verified from outside AWS on 2026-09-11).
// 52.76.123.15 = ppt1 EIP, 52.77.202.62 = ppt2 EIP.
var DefaultIPs = []string{"52.76.29.0", "52.76.123.15", "3.1.252.222", "52.77.202.62"}

// EnvKeys are read in order and every set value is unioned. PANEL_ORIGIN_IPS is the
// canonical name; the two older per-feature names are still honoured.
var EnvKeys = []string{"PANEL_ORIGIN_IPS", "CADDY_HEALTH_SERVER_IPS", "CUTOVER_ORIGIN_IPS"}

// Configured returns the configured origin list and where it came from. When any
// EnvKeys variable holds a valid IP, the union of those values REPLACES the
// defaults (an override must be able to drop a released address). When none is set,
// or every entry is unparseable, DefaultIPs apply.
func Configured(getenv func(string) string) (ips []string, source string) {
	seen := map[string]bool{}
	var used []string
	for _, key := range EnvKeys {
		added := false
		for _, part := range strings.Split(getenv(key), ",") {
			if ip := Canonical(part); ip != "" && !seen[ip] {
				seen[ip] = true
				ips = append(ips, ip)
				added = true
			}
		}
		if added {
			used = append(used, key)
		}
	}
	if len(ips) == 0 {
		return append([]string(nil), DefaultIPs...), "default"
	}
	sort.Strings(ips)
	return ips, strings.Join(used, "+")
}

// Canonical parses an IP and returns its canonical string, or "" if it isn't one.
func Canonical(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return ""
	}
	return ip.String()
}

// Discoverer yields this instance's public addresses (IMDS in production, a fake in
// tests).
type Discoverer interface {
	PublicIPv4s(ctx context.Context) ([]string, error)
}

// Set is the live origin-IP set. Safe for concurrent use.
type Set struct {
	mu         sync.RWMutex
	configured []string
	source     string
	discovered []string
}

// NewSet builds a set from a configured list (see Configured).
func NewSet(configured []string, source string) *Set {
	s := &Set{source: source}
	for _, ip := range configured {
		if c := Canonical(ip); c != "" {
			s.configured = append(s.configured, c)
		}
	}
	return s
}

// IPs returns a fresh copy of the union (configured ∪ discovered).
func (s *Set) IPs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]bool, len(s.configured)+len(s.discovered))
	for _, ip := range s.configured {
		out[ip] = true
	}
	for _, ip := range s.discovered {
		out[ip] = true
	}
	return out
}

// List returns the union sorted, for logs and display.
func (s *Set) List() []string {
	m := s.IPs()
	out := make([]string, 0, len(m))
	for ip := range m {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

// Source names where the configured part came from ("default" or the env keys).
func (s *Set) Source() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.source
}

// SetDiscovered replaces ONLY the discovered part. The configured part is never
// touched, so a metadata answer that lacks some configured address cannot drop it.
func (s *Set) SetDiscovered(ips []string) {
	var clean []string
	for _, ip := range ips {
		if c := Canonical(ip); c != "" {
			clean = append(clean, c)
		}
	}
	s.mu.Lock()
	s.discovered = clean
	s.mu.Unlock()
}

// Discover runs one discovery attempt. On error the previous discovered addresses
// are kept (a metadata hiccup must not shrink the set).
func (s *Set) Discover(ctx context.Context, d Discoverer) error {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := d.PublicIPv4s(c)
	if err != nil {
		return err
	}
	s.SetDiscovered(ips)
	return nil
}

// Run discovers immediately, then every interval until ctx is cancelled. Errors are
// swallowed: off EC2 (or with IMDS disabled) the configured list simply stands.
func (s *Set) Run(ctx context.Context, d Discoverer, every time.Duration) {
	_ = s.Discover(ctx, d)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Discover(ctx, d)
		}
	}
}
