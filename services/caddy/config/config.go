// Package config holds the Caddy-vhost engine's runtime configuration for
// server-panel: the folder it manages, the main Caddyfile it adapts, the admin
// API it reloads through, the server_stack->upstream port map, and the protected
// domains. The desired-state DB connection is NOT here — it comes from a named
// Data Source the operator selects (see services.DataSourceService).
package config

import (
	"os"
	"sort"
	"strings"
)

// Config is the resolved engine configuration (defaults + env overrides).
type Config struct {
	// VhostsDir is the flat folder the engine SOLELY owns; it renders every
	// <host>.caddy here. Root-only on the host (750 server:server).
	VhostsDir string
	// MainCaddyfile is the existing main Caddyfile the engine adapts (static panel
	// block + `import <VhostsDir>/*`). Adapted in-process/as-root; never rewritten.
	MainCaddyfile string
	// CaddyAdminURL is the Caddy admin API base; reload = POST adapted JSON to /load.
	CaddyAdminURL string
	// BackupDir holds prior adapted-config snapshots for one-command rollback.
	BackupDir string
	// KnownHostsFile persists the set of hostnames ever seen desired, across
	// restarts. Powers deletion detection for the LEAN website_hosts table: a file
	// whose host was previously desired but is now absent (on a healthy read) is a
	// removal, not an orphan.
	KnownHostsFile string
	// PanelDomain is server-panel's own public host — the ABSOLUTE guard: never
	// removed/rendered as a folder file, and a reload whose adapted config drops it
	// is refused (losing it locks the operator out of the panel itself). It's the
	// sole protected domain; stack dashboard domains (app./go-app./la-app./
	// rust-app.propertyboom.co) are ordinary managed routes, not special-cased.
	PanelDomain string
	// StackPorts maps website_hosts.server_stack -> upstream host:port (website_hosts
	// has no port column). An unknown stack is skipped, never guessed.
	StackPorts map[string]string
	// Encode is the response-compression policy for proxied vhosts ("zstd gzip"); "" = off.
	Encode string
	// SecurityHeaders is the edge-header policy for TENANT vhosts only; nil = off.
	SecurityHeaders *SecurityHeaders
}

func defaults() Config {
	return Config{
		VhostsDir:      "/home/server/.caddy",
		MainCaddyfile:  "/etc/caddy/Caddyfile",
		CaddyAdminURL:  "http://localhost:2019",
		BackupDir:      "/var/lib/ppt-server-panel/caddy-backups",
		KnownHostsFile: "/var/lib/ppt-server-panel/vhost-known-hosts.json",
		PanelDomain:    "cp.propertyweb.co",
		StackPorts: map[string]string{
			// From design-templates/docs/stack-deploy-ports.md (host:port so the
			// renderer never guesses the host half either).
			"phalcon": "127.0.0.1:8002",
			"laravel": "127.0.0.1:8004",
			"golang":  "127.0.0.1:8005",
			"rust":    "127.0.0.1:8000",
		},
	}
}

// Load returns the config: defaults overlaid with env overrides.
func Load() Config {
	cfg := defaults()
	if v := os.Getenv("CADDY_VHOSTS_DIR"); v != "" {
		cfg.VhostsDir = v
	}
	if v := os.Getenv("CADDY_MAIN_CADDYFILE"); v != "" {
		cfg.MainCaddyfile = v
	}
	if v := os.Getenv("CADDY_ADMIN_URL"); v != "" {
		cfg.CaddyAdminURL = v
	}
	if v := os.Getenv("CADDY_BACKUP_DIR"); v != "" {
		cfg.BackupDir = v
	}
	if v := os.Getenv("CADDY_KNOWN_HOSTS_FILE"); v != "" {
		cfg.KnownHostsFile = v
	}
	if v := os.Getenv("CADDY_PANEL_DOMAIN"); v != "" {
		cfg.PanelDomain = v
	}
	if v, ok := os.LookupEnv("CADDY_ENCODE"); ok {
		cfg.Encode = v
	}
	return cfg
}

// ProtectedHosts returns the hosts that must never be rendered or pruned as folder
// files and must survive every reload — just the panel's own domain, a static
// Caddyfile block the operator owns. Stack dashboard domains are ordinary routes.
func (c Config) ProtectedHosts() []string {
	out := make([]string, 0, 1)
	if h := strings.ToLower(strings.TrimSpace(c.PanelDomain)); h != "" {
		out = append(out, h)
	}
	return out
}

// UpstreamFor returns the reverse_proxy upstream (host:port) a tenant vhost for
// server_stack dials, and whether the stack is known. An unknown stack is a
// render-time skip the caller surfaces — never silently pick a wrong port.
func (c Config) UpstreamFor(serverStack string) (string, bool) {
	up, ok := c.StackPorts[strings.ToLower(strings.TrimSpace(serverStack))]
	return up, ok
}

// Stacks returns the known server_stack names (sorted) a system host may target.
// Drives the management UI's stack picker so an operator can only pick a stack
// whose upstream port is known — never a free-typed guess.
func (c Config) Stacks() []string {
	out := make([]string, 0, len(c.StackPorts))
	for s := range c.StackPorts {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// EncodeFormats returns the normalized (whitespace-collapsed) encode directive,
// e.g. "zstd gzip", or "" when compression is off.
func (c Config) EncodeFormats() string {
	return strings.Join(strings.Fields(c.Encode), " ")
}
