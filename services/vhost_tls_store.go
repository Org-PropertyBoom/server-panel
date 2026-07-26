package services

import (
	"fmt"
	"path/filepath"
	"strings"

	caddydb "ppt/server-panel/services/caddy/db"
)

// Per-host TLS mode store — panel-local (server-panel's own SQLite). A host with
// mode "cf_origin" serves a static Cloudflare Origin cert (`tls <cert> <key>`)
// instead of on-demand Let's Encrypt — mutually exclusive, because a host proxied
// through Cloudflare can't complete an ACME challenge (it terminates at the CF
// edge). Absent row = "ondemand" (the default). Never a stack-DB write.

const (
	TLSModeOnDemand = "ondemand"
	TLSModeCFOrigin = "cf_origin"
)

// AllHostTLSModes returns every host's TLS override (host -> mode + optional
// per-host cert/key paths), for injection into a reconcile snapshot. Only
// non-default (cf_origin) rows are stored, so the map is small.
func (s *SettingsService) AllHostTLSModes() (map[string]caddydb.TLSOverride, error) {
	rows, err := s.db.Query("SELECT host, mode, cert_path, key_path FROM vhost_tls_modes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]caddydb.TLSOverride{}
	for rows.Next() {
		var host, mode, cert, key string
		if err := rows.Scan(&host, &mode, &cert, &key); err != nil {
			return nil, err
		}
		out[host] = caddydb.TLSOverride{Mode: mode, CertPath: cert, KeyPath: key}
	}
	return out, rows.Err()
}

// SetHostTLSMode sets a host's TLS mode. "cf_origin" persists (with optional
// per-host cert/key paths that override the global default); "ondemand" or "" deletes
// the row, returning the host to the default on-demand behavior. Paths, when given,
// must be absolute.
func (s *SettingsService) SetHostTLSMode(host, mode, certPath, keyPath string) error {
	key := normalizeHostKey(host)
	if key == "" {
		return fmt.Errorf("host is required")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	certPath, keyPath = strings.TrimSpace(certPath), strings.TrimSpace(keyPath)
	switch mode {
	case TLSModeCFOrigin:
		if err := validateOptionalAbsPath(certPath); err != nil {
			return err
		}
		if err := validateOptionalAbsPath(keyPath); err != nil {
			return err
		}
		_, err := s.db.Exec(`INSERT INTO vhost_tls_modes (host, mode, cert_path, key_path, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(host) DO UPDATE SET mode = excluded.mode, cert_path = excluded.cert_path, key_path = excluded.key_path, updated_at = CURRENT_TIMESTAMP`,
			key, TLSModeCFOrigin, certPath, keyPath)
		return err
	case TLSModeOnDemand, "":
		_, err := s.db.Exec("DELETE FROM vhost_tls_modes WHERE host = ?", key)
		return err
	default:
		return fmt.Errorf("unknown TLS mode %q (want %q or %q)", mode, TLSModeOnDemand, TLSModeCFOrigin)
	}
}

// DeleteHostTLSMode drops a host's override (called when a system host is deleted
// or renamed away, so a stale cf_origin row doesn't linger).
func (s *SettingsService) DeleteHostTLSMode(host string) error {
	_, err := s.db.Exec("DELETE FROM vhost_tls_modes WHERE host = ?", normalizeHostKey(host))
	return err
}

func validateOptionalAbsPath(p string) error {
	if p == "" {
		return nil // empty → fall back to the global default path
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("path must be absolute: %q", p)
	}
	return nil
}
