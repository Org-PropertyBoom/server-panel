package config

import (
	"fmt"
	"strings"
)

// SecurityHeaders is the edge response-header policy the engine renders onto
// TENANT webfront vhosts (website_hosts). It is deliberately NOT applied to the
// system/dashboard-app hosts (platform_hosts) — admin panels need different
// framing/CSP. Configured under `security_headers`; omitted/nil = off entirely.
type SecurityHeaders struct {
	// Baseline applies the safe, universal set: X-Content-Type-Options nosniff,
	// X-Frame-Options (FrameOptions), Referrer-Policy strict-origin-when-cross-origin,
	// and Permissions-Policy (PermissionsPolicy).
	Baseline bool `json:"baseline"`

	// FrameOptions is the X-Frame-Options value (default "SAMEORIGIN" when Baseline is on).
	FrameOptions string `json:"frame_options"`

	// PermissionsPolicy is the Permissions-Policy value (default locks
	// geolocation/camera/microphone when Baseline is on).
	PermissionsPolicy string `json:"permissions_policy"`

	// HSTS is the Strict-Transport-Security value. Empty = OFF. `preload` is rejected.
	HSTS string `json:"hsts"`

	// CSP is the Content-Security-Policy value. Empty = OFF. Rendered as the
	// -Report-Only header unless CSPEnforce is true.
	CSP string `json:"csp"`

	// CSPEnforce switches CSP from Report-Only to enforcing.
	CSPEnforce bool `json:"csp_enforce"`
}

const (
	defaultFrameOptions      = "SAMEORIGIN"
	defaultPermissionsPolicy = "geolocation=(), camera=(), microphone=()"
)

// Configured reports whether any header would be emitted.
func (s *SecurityHeaders) Configured() bool {
	if s == nil {
		return false
	}
	return s.Baseline || strings.TrimSpace(s.HSTS) != "" || strings.TrimSpace(s.CSP) != ""
}

// Summary is a short human string for logs/UI.
func (s *SecurityHeaders) Summary() string {
	if !s.Configured() {
		return "off"
	}
	var parts []string
	if s.Baseline {
		parts = append(parts, "baseline")
	}
	if strings.TrimSpace(s.HSTS) != "" {
		parts = append(parts, "hsts")
	}
	if strings.TrimSpace(s.CSP) != "" {
		if s.CSPEnforce {
			parts = append(parts, "csp(enforce)")
		} else {
			parts = append(parts, "csp(report-only)")
		}
	}
	return strings.Join(parts, "+")
}

// HeaderBlock returns the Caddyfile `header { ... }` block (4-space indented,
// trailing newline) to insert into a tenant vhost, or "" when nothing applies.
func (s *SecurityHeaders) HeaderBlock() string {
	if !s.Configured() {
		return ""
	}
	var lines []string
	if s.Baseline {
		fo := strings.TrimSpace(s.FrameOptions)
		if fo == "" {
			fo = defaultFrameOptions
		}
		pp := strings.TrimSpace(s.PermissionsPolicy)
		if pp == "" {
			pp = defaultPermissionsPolicy
		}
		lines = append(lines,
			"        X-Content-Type-Options nosniff",
			"        X-Frame-Options "+fo,
			"        Referrer-Policy strict-origin-when-cross-origin",
			`        Permissions-Policy "`+pp+`"`,
		)
	}
	if h := strings.TrimSpace(s.HSTS); h != "" {
		lines = append(lines, `        Strict-Transport-Security "`+h+`"`)
	}
	if c := strings.TrimSpace(s.CSP); c != "" {
		name := "Content-Security-Policy-Report-Only"
		if s.CSPEnforce {
			name = "Content-Security-Policy"
		}
		lines = append(lines, "        "+name+` "`+c+`"`)
	}
	if len(lines) == 0 {
		return ""
	}
	return "    header {\n" + strings.Join(lines, "\n") + "\n    }\n"
}

// Validate rejects unsafe/irreversible settings (preload).
func (s *SecurityHeaders) Validate() error {
	if s == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(s.HSTS), "preload") {
		return fmt.Errorf("security_headers.hsts must not include `preload` — it is effectively irreversible; set max-age[; includeSubDomains] only")
	}
	return nil
}
