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
	// KindStatic serves FILES from a directory — no upstream, no app container.
	// Target is the filesystem root. Used for internal docs sites, which are
	// files on disk and depend on no stack.
	KindStatic
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
	// OnDemandTLS emits a `tls { on_demand }` block so Caddy only obtains this
	// host's cert when it's actually visited (and the ask endpoint authorizes it),
	// instead of trying at startup for every configured domain. NEVER emitted for
	// wildcard hosts — on-demand issuance can't satisfy a wildcard (needs DNS).
	OnDemandTLS bool
	// TLSCertPath/TLSKeyPath, when BOTH set, render a static `tls <cert> <key>`
	// (Cloudflare Origin cert mode) INSTEAD of on_demand — mutually exclusive. Used
	// for hosts proxied through Cloudflare, which can't complete an ACME challenge
	// (it terminates at the CF edge, never reaching Caddy).
	TLSCertPath string
	TLSKeyPath  string
	// ValidationRoot, when set, emits an explicit `http://<host>` block that serves
	// /.well-known/pki-validation/ from that shared directory over PLAIN HTTP and
	// redirects everything else to HTTPS (preserving Caddy's normal behaviour).
	// It lets us prove domain ownership for a Cloudflare for SaaS custom hostname
	// from the server we control, instead of asking the domain owner for TXT records.
	ValidationRoot string
	// BasicAuth is a pre-rendered `basic_auth { ... }` block (KindStatic only),
	// carrying a bcrypt hash — never a plaintext password.
	BasicAuth string
}

// validationBlock renders the plain-HTTP domain-ownership validation site for a
// host, used to prove ownership for a Cloudflare for SaaS custom hostname without
// the domain's owner touching DNS.
//
// EXPLICIT `http://<host>` block: with only the normal site block, Caddy's
// automatic HTTPS would 308 the validator to https, and at validation time the
// domain may not have a usable certificate yet — so the fetch would fail. Taking
// over the HTTP site means Caddy no longer generates that redirect, so the trailing
// handle re-creates it; every non-validation request behaves exactly as before.
//
// ⚠ /.well-known/acme-challenge/ IS ALSO THE PATH CADDY USES FOR ITS OWN LET'S
// ENCRYPT HTTP-01 CHALLENGES, which all managed hosts depend on. This does not
// shadow them: Caddy checks for an active challenge in Server.ServeHTTP —
// `if s.tlsApp.HandleHTTPChallenge(w, r) { return }` — BEFORE any route matching,
// so its own challenge always wins. That call returns false only when no issuer has
// a live challenge for the host, and the request then falls through to these
// routes. So the file_server is strictly a FALLBACK for tokens Caddy isn't
// currently solving. (Reasoned from Caddy's request path, not measured on the host
// — see docs/tls-scaling.md for the issuance acceptance test that proves it live.)
//
// Both well-known paths are served from the same flat directory via handle_path,
// which strips the prefix: a token dropped at <root>/<token> answers either path.
// ACME tokens are extensionless (body = token.thumbprint); pki-validation files are
// <token>.txt. Skipped for wildcard hosts.
func validationBlock(host, root string) string {
	root = strings.TrimSpace(root)
	if root == "" || strings.HasPrefix(normalizeHost(host), "*.") {
		return ""
	}
	serve := func(prefix string) string {
		return "    handle_path " + prefix + "/* {\n" +
			"        root * " + root + "\n" +
			"        file_server\n" +
			"    }\n"
	}
	return "http://" + host + " {\n" +
		serve("/.well-known/acme-challenge") +
		serve("/.well-known/pki-validation") +
		"    handle {\n" +
		"        redir https://{host}{uri} 308\n" +
		"    }\n" +
		"}\n\n"
}

// tlsBlock renders the `tls` line for a site, 4-space indented. The two modes are
// MUTUALLY EXCLUSIVE and Cloudflare-Origin-cert takes precedence:
//
//   - certPath+keyPath both set → `tls <cert> <key>` (cf_origin): the host is
//     proxied through Cloudflare and serves a static Origin cert; it must NOT also
//     carry on_demand, since a proxied host can't complete an ACME challenge.
//   - else on_demand → `tls { on_demand … }` with `issuer acme` pinned (Let's
//     Encrypt production; drops the dead ZeroSSL fallback / code 2977). The ACME
//     email is inherited from the global Caddyfile `email` option. Never emitted
//     for wildcard hosts (on-demand can't satisfy a wildcard — needs DNS).
//   - else "" (no tls line → Caddy's default automatic HTTPS).
func tlsBlock(host string, onDemand bool, certPath, keyPath string) string {
	if certPath != "" && keyPath != "" {
		return "    tls " + certPath + " " + keyPath + "\n"
	}
	if onDemand && !strings.HasPrefix(normalizeHost(host), "*.") {
		return "    tls {\n        on_demand\n        issuer acme\n    }\n"
	}
	return ""
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
		return FileName(host), validationBlock(host, h.ValidationRoot) +
			proxySnippet(host, up, strings.TrimSpace(h.Encode), h.HeaderBlock, tlsBlock(host, h.OnDemandTLS, h.TLSCertPath, h.TLSKeyPath)), nil
	case KindRedirect:
		target := strings.TrimSpace(h.Target)
		if target == "" {
			return "", "", fmt.Errorf("render: redirect host %q has no target URL", host)
		}
		return FileName(host), validationBlock(host, h.ValidationRoot) +
			redirectSnippet(host, target, h.RedirectCode, tlsBlock(host, h.OnDemandTLS, h.TLSCertPath, h.TLSKeyPath)), nil
	case KindStatic:
		root := strings.TrimSpace(h.Target)
		if root == "" {
			return "", "", fmt.Errorf("render: static host %q has no filesystem root", host)
		}
		if !strings.HasPrefix(root, "/") {
			return "", "", fmt.Errorf("render: static host %q needs an absolute root, got %q", host, root)
		}
		return FileName(host), validationBlock(host, h.ValidationRoot) +
			staticSnippet(host, root, h.BasicAuth, h.HeaderBlock, strings.TrimSpace(h.Encode), tlsBlock(host, h.OnDemandTLS, h.TLSCertPath, h.TLSKeyPath)), nil
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
func proxySnippet(host, upstream, encode, headerBlock, tlsBlock string) string {
	var b strings.Builder
	b.WriteString(host + " {\n")
	b.WriteString(tlsBlock)    // "" or "    tls {\n        on_demand\n    }\n"
	b.WriteString(headerBlock) // "" or a complete, indented block ending in \n
	if encode != "" {
		b.WriteString("    encode " + encode + "\n")
	}
	b.WriteString("    reverse_proxy " + upstream + "\n}\n")
	return b.String()
}

// staticSnippet serves files from a directory — no upstream, no app container.
//
// basicAuth, when set, is a pre-rendered `basic_auth { ... }` block. It is
// rendered BEFORE root/file_server so an internal site cannot be served
// unauthenticated: an unset credential must mean "no site", never "public site".
// Callers that require internal access enforce that; this only renders what it's
// given, which is why Render refuses a static host with neither auth nor an
// explicit acknowledgement upstream.
func staticSnippet(host, root, basicAuth, headerBlock, encode, tlsBlock string) string {
	var b strings.Builder
	b.WriteString(host + " {\n")
	b.WriteString(tlsBlock)
	b.WriteString(basicAuth)
	b.WriteString(headerBlock)
	if encode != "" {
		b.WriteString("    encode " + encode + "\n")
	}
	b.WriteString("    root * " + root + "\n")
	b.WriteString("    file_server\n}\n")
	return b.String()
}

// BasicAuthBlock renders a `basic_auth` block from a username and a BCRYPT HASH.
// The hash is what Caddy stores — a plaintext password must never reach a vhost
// file, which is world-readable to anything that can read the folder.
func BasicAuthBlock(username, bcryptHash string) string {
	username, bcryptHash = strings.TrimSpace(username), strings.TrimSpace(bcryptHash)
	if username == "" || bcryptHash == "" {
		return ""
	}
	return "    basic_auth {\n        " + username + " " + bcryptHash + "\n    }\n"
}

// redirectSnippet is the redir block. With no tlsBlock it is byte-identical to
// CaddyVhostsService::redirectSnippet (numeric code; <=0 -> 301).
func redirectSnippet(host, target string, code int, tlsBlock string) string {
	if code <= 0 {
		code = 301
	}
	return fmt.Sprintf("%s {\n%s    redir %s %d\n}\n", host, tlsBlock, target, code)
}

// normalizeHost lower-cases and trims a host, matching the apps' strtolower/trim
// so filenames and snippet bodies agree case-for-case.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
