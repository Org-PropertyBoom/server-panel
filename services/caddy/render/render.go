// Package render turns a desired host (a DB row's routing data) into its
// `<host>.caddy` file: the filename and the byte-exact Caddyfile snippet.
//
// The snippet format is BYTE-IDENTICAL to what the stack apps write today
// (property-team CaddyVhostsService: `<host> {\n    reverse_proxy <up>\n}\n` and
// `<host> {\n    redir <target> <code>\n}\n`, 4-space indent, trailing newline).
// Rendering the same bytes means switching the writer produces no folder diff.
//
// Rendering is PURE: it does not read config, the DB, or the filesystem. The
// tenant upstream (website_hosts has no port column) is resolved upstream by the
// caller from the server_stack->port map and handed in via Target, so this
// package stays a deterministic, testable formatter.
package render

import (
	"fmt"
	"strings"
)

// Kind is the class of a desired host, matching the three DB tables.
type Kind int

const (
	// KindTenant is a website_hosts row — a tenant site. Renders a reverse_proxy
	// to the owning stack's upstream (resolved into Target by the caller).
	KindTenant Kind = iota
	// KindSystem is a platform_hosts row — a system/dashboard-app domain. Renders
	// a reverse_proxy to Target (the row's upstream host:port).
	KindSystem
	// KindRedirect is a platform_redirect_hosts row — an edge redirect. Renders a
	// redir to Target (a URL) with RedirectCode.
	KindRedirect
)

func (k Kind) String() string {
	switch k {
	case KindTenant:
		return "tenant"
	case KindSystem:
		return "system"
	case KindRedirect:
		return "redirect"
	default:
		return "unknown"
	}
}

// Host is one desired vhost, normalized from a DB row. Target's meaning depends
// on Kind: for tenant/system it is the reverse_proxy upstream (host:port); for
// redirect it is the destination URL.
type Host struct {
	Host         string // the vhost name, e.g. "example.com" or "*.example.com"
	Kind         Kind
	Target       string // proxy upstream (tenant/system) OR redirect URL (redirect)
	RedirectCode int    // redirect only; <=0 renders as 301
	Encode       string // proxy only: `encode` formats (e.g. "zstd gzip"); "" = none
	HeaderBlock  string // proxy only: a pre-rendered `header { ... }` block (4-space indented, trailing \n); "" = none
}

// FileName is the flat-folder filename for a host: "<host>.caddy", with a
// wildcard host "*.x" mapped to "wildcard_x.caddy" (a "*" is not a legal
// filename char). Matches CaddyVhostsService::vhostFileName so the engine and
// the apps name the same file for the same host.
func FileName(host string) string {
	host = normalizeHost(host)
	if strings.HasPrefix(host, "*.") {
		return "wildcard_" + host[2:] + ".caddy"
	}
	return host + ".caddy"
}

// HostFromFileName is the inverse of FileName: "<host>.caddy" -> "<host>", and
// "wildcard_<x>.caddy" -> "*.<x>". Returns "" if name is not a *.caddy file.
func HostFromFileName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".caddy") {
		return ""
	}
	base := strings.TrimSuffix(name, ".caddy")
	if strings.HasPrefix(base, "wildcard_") {
		return "*." + strings.TrimPrefix(base, "wildcard_")
	}
	return base
}

// Render returns the file name and byte-exact contents for a desired host, or an
// error if the host is unrenderable (empty host, empty target).
func Render(h Host) (name string, contents string, err error) {
	host := normalizeHost(h.Host)
	if host == "" {
		return "", "", fmt.Errorf("render: empty host")
	}
	switch h.Kind {
	case KindTenant, KindSystem:
		up := strings.TrimSpace(h.Target)
		if up == "" {
			return "", "", fmt.Errorf("render: %s host %q has no upstream target", h.Kind, host)
		}
		return FileName(host), proxySnippet(host, up, strings.TrimSpace(h.Encode), h.HeaderBlock), nil
	case KindRedirect:
		target := strings.TrimSpace(h.Target)
		if target == "" {
			return "", "", fmt.Errorf("render: redirect host %q has no target URL", host)
		}
		return FileName(host), redirectSnippet(host, target, h.RedirectCode), nil
	default:
		return "", "", fmt.Errorf("render: unknown kind %d for host %q", h.Kind, host)
	}
}

// proxySnippet is the reverse_proxy block. With no headerBlock/encode it is
// byte-identical to CaddyVhostsService::vhostSnippet / systemSnippet (proxy
// branch); otherwise it inserts the `header { ... }` block then `encode`, both
// before reverse_proxy (Caddy orders directives itself, so placement is cosmetic).
// headerBlock, when non-empty, is a fully-rendered block starting with
// "    header {\n" and ending with "    }\n". Redirect blocks never get either.
func proxySnippet(host, upstream, encode, headerBlock string) string {
	var b strings.Builder
	b.WriteString(host + " {\n")
	b.WriteString(headerBlock) // "" or a complete, indented block ending in \n
	if encode != "" {
		b.WriteString("    encode " + encode + "\n")
	}
	b.WriteString("    reverse_proxy " + upstream + "\n}\n")
	return b.String()
}

// redirectSnippet is the redir block. Byte-identical to
// CaddyVhostsService::redirectSnippet (numeric code; <=0 -> 301).
func redirectSnippet(host, target string, code int) string {
	if code <= 0 {
		code = 301
	}
	return fmt.Sprintf("%s {\n    redir %s %d\n}\n", host, target, code)
}

// normalizeHost lower-cases and trims a host, matching the apps' strtolower/trim
// so filenames and snippet bodies agree case-for-case.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
