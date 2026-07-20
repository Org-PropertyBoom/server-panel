// Package config holds the Caddy-vhost engine's runtime configuration for
// server-panel: the folder it manages, the main Caddyfile it adapts, the admin
// API it reloads through, the server_stack->upstream port map, and the protected
// domains. The desired-state DB connection is NOT here — it comes from a named
// Data Source the operator selects (see services.DataSourceService).
package config

import (
	"os"
	"strings"
)

// Config is the resolved engine configuration (defaults + env overrides).
type Config struct {
	// VhostsDir is the flat folder the engine SOLELY owns; it renders every
	// <host>.caddy here. Root-only on the host (750 server:server).
	VhostsDir string
	// MainCaddyfile is the existing main Caddyfile the engine adapts (static
	// dashboard block + `import <VhostsDir>/*`). Adapted in-process/as-root; never
	// rewritten.
	MainCaddyfile string
	// CaddyAdminURL is the Caddy admin API base; reload = POST adapted JSON to /load.
	CaddyAdminURL string
	// BackupDir holds prior adapted-config snapshots for one-command rollback.
	BackupDir string
	// DashboardDomain is the ABSOLUTE guard: never removed/rendered as a folder
	// file, pinned as a static Caddyfile block; a reload whose adapted config drops
	// it is refused.
	DashboardDomain string
	// PanelDomain is server-panel's own public host — also protected so the panel
	// never renders/prunes its own front door.
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
		VhostsDir:       "/home/server/.caddy",
		MainCaddyfile:   "/etc/caddy/Caddyfile",
		CaddyAdminURL:   "http://localhost:2019",
		BackupDir:       "/var/lib/ppt-server-panel/caddy-backups",
		DashboardDomain: "app.propertyboom.co",
		PanelDomain:     "cp.propertyweb.co",
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
	if v := os.Getenv("CADDY_DASHBOARD_DOMAIN"); v != "" {
		cfg.DashboardDomain = v
	}
	if v := os.Getenv("CADDY_PANEL_DOMAIN"); v != "" {
		cfg.PanelDomain = v
	}
	if v, ok := os.LookupEnv("CADDY_ENCODE"); ok {
		cfg.Encode = v
	}
	return cfg
}

// ProtectedHosts returns the hosts that must never be rendered or pruned as
// folder files — the dashboard domain and (if set) the panel domain. Both are
// static Caddyfile blocks the operator owns.
func (c Config) ProtectedHosts() []string {
	out := make([]string, 0, 2)
	if h := strings.ToLower(strings.TrimSpace(c.DashboardDomain)); h != "" {
		out = append(out, h)
	}
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

// EncodeFormats returns the normalized (whitespace-collapsed) encode directive,
// e.g. "zstd gzip", or "" when compression is off.
func (c Config) EncodeFormats() string {
	return strings.Join(strings.Fields(c.Encode), " ")
}
