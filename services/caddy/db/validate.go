package db

import (
	"fmt"
	"net/url"
	"strings"
)

// Validation for the management-UI writes. PURE (no DB, no config) — the caller
// supplies the two policy checks (isProtected, stackKnown) from config so this
// stays unit-testable. Normalizes host/target in place and returns the cleaned
// input plus any error.

// Guard is the policy the service injects into validation.
type Guard struct {
	// IsProtected reports whether a host must never be written (the dashboard/panel
	// domain — a static Caddyfile block, never a managed row/file).
	IsProtected func(host string) bool
	// StackKnown reports whether a server_stack is in the stack->port map.
	StackKnown func(stack string) bool
}

// ValidateSystemHost normalizes and validates a platform_hosts input.
func ValidateSystemHost(in SystemHostInput, g Guard) (SystemHostInput, error) {
	in.Host = normalizeHostField(in.Host)
	in.ServerStack = strings.ToLower(strings.TrimSpace(in.ServerStack))
	in.Target = strings.TrimSpace(in.Target)

	if in.Host == "" {
		return in, fmt.Errorf("host is required")
	}
	if g.IsProtected != nil && g.IsProtected(in.Host) {
		return in, fmt.Errorf("%q is a protected domain — it is a static Caddyfile block and cannot be managed as a row", in.Host)
	}
	// server_stack is a free label for a system host (a service/container name), NOT
	// restricted to the code stacks — a system host proxies to ANY container's
	// upstream, not just phalcon/laravel/golang/rust. The upstream is `target`.
	if in.ServerStack == "" {
		in.ServerStack = "system"
	}
	if in.Target == "" {
		return in, fmt.Errorf("target (upstream host:port) is required")
	}
	if !looksLikeHostPort(in.Target) {
		return in, fmt.Errorf("target %q should be an upstream host:port (e.g. 127.0.0.1:8002)", in.Target)
	}
	return in, nil
}

// ValidateRedirect normalizes and validates a platform_redirect_hosts input.
func ValidateRedirect(in RedirectInput, g Guard) (RedirectInput, error) {
	in.Host = normalizeHostField(in.Host)
	in.Target = strings.TrimSpace(in.Target)
	if in.Code == 0 {
		in.Code = 301
	}

	if in.Host == "" {
		return in, fmt.Errorf("host is required")
	}
	if g.IsProtected != nil && g.IsProtected(in.Host) {
		return in, fmt.Errorf("%q is a protected domain and cannot be managed as a row", in.Host)
	}
	if in.Target == "" {
		return in, fmt.Errorf("target URL is required")
	}
	if !strings.HasPrefix(in.Target, "http://") && !strings.HasPrefix(in.Target, "https://") {
		return in, fmt.Errorf("target %q must be an absolute URL (http:// or https://)", in.Target)
	}
	if u, err := url.Parse(in.Target); err == nil && strings.EqualFold(u.Hostname(), in.Host) {
		return in, fmt.Errorf("target host equals the source host %q — that is a redirect loop", in.Host)
	}
	if in.Code != 301 && in.Code != 302 {
		return in, fmt.Errorf("redirect_code must be 301 or 302, got %d", in.Code)
	}
	return in, nil
}

func normalizeHostField(h string) string {
	return strings.ToLower(strings.TrimSpace(h))
}

// looksLikeHostPort is a light sanity check for an upstream target: a non-empty
// host, a colon, and a numeric port. It does NOT resolve or fully validate — it
// just catches obvious mistakes (a bare host, a URL) before a write.
func looksLikeHostPort(s string) bool {
	if strings.Contains(s, "/") {
		return false
	}
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return false
	}
	host, port := s[:i], s[i+1:]
	if host == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
