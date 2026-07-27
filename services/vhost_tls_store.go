package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	caddydb "ppt/server-panel/services/caddy/db"
)

// --- Origin certificate registry ---
//
// A LIST of Cloudflare Origin certs with NO "default" entry: propertyweb.co and
// propertyboom.co are peer zones, so calling either one the default would be
// arbitrary (and a third zone makes it worse). The panel picks the entry whose PEM
// covers the hostname being switched to cf_origin.

// OriginCertRow is one registered cert as stored (paths only; coverage is parsed
// from the PEM on read by the engine).
type OriginCertRow struct {
	CertPath string `json:"certPath"`
	KeyPath  string `json:"keyPath"`
}

// OriginCerts lists the registered Origin certificates, oldest first (registration
// order is the documented tie-break when two certs cover the same hostname).
func (s *SettingsService) OriginCerts() ([]OriginCertRow, error) {
	rows, err := s.db.Query("SELECT cert_path, key_path FROM origin_certs ORDER BY added_at, cert_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OriginCertRow
	for rows.Next() {
		var r OriginCertRow
		if err := rows.Scan(&r.CertPath, &r.KeyPath); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddOriginCert registers a cert+key pair (both absolute paths).
func (s *SettingsService) AddOriginCert(certPath, keyPath string) error {
	certPath, keyPath = strings.TrimSpace(certPath), strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("certificate and key paths are both required")
	}
	if err := validateOptionalAbsPath(certPath); err != nil {
		return err
	}
	if err := validateOptionalAbsPath(keyPath); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO origin_certs (cert_path, key_path, added_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(cert_path) DO UPDATE SET key_path = excluded.key_path`, certPath, keyPath)
	return err
}

// DeleteOriginCert unregisters a cert. Hosts already rendered with it keep their
// stored path until they're switched again (the file on disk is untouched).
func (s *SettingsService) DeleteOriginCert(certPath string) error {
	_, err := s.db.Exec("DELETE FROM origin_certs WHERE cert_path = ?", strings.TrimSpace(certPath))
	return err
}

// --- on-demand TLS ask allowlist (persisted) ---
//
// Caddy consults /internal/tls-ask on HANDSHAKES for any hostname not in its cert
// cache — not only when issuing. The endpoint is fail-closed, so if the panel is
// restarting (or its data source isn't ready) every on-demand host loses TLS. This
// allowlist records hosts the panel HAS authorized, survives restarts, and is read
// back at boot so those hosts keep answering 200 while the panel warms up.

// TLSAskAllowlist loads the persisted allowlist (host -> when it was authorized).
func (s *SettingsService) TLSAskAllowlist() (map[string]time.Time, error) {
	rows, err := s.db.Query("SELECT host, allowed_at FROM tls_ask_allow")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var host string
		var at time.Time
		if err := rows.Scan(&host, &at); err != nil {
			return nil, err
		}
		out[host] = at
	}
	return out, rows.Err()
}

// SetTLSAskAllowed records/refreshes a host as authorized for on-demand TLS.
func (s *SettingsService) SetTLSAskAllowed(host string) error {
	key := normalizeHostKey(host)
	if key == "" {
		return fmt.Errorf("host is required")
	}
	_, err := s.db.Exec(`INSERT INTO tls_ask_allow (host, allowed_at) VALUES (?, CURRENT_TIMESTAMP)
		ON CONFLICT(host) DO UPDATE SET allowed_at = CURRENT_TIMESTAMP`, key)
	return err
}

// DeleteTLSAskAllowed drops a host from the allowlist — called when the operator
// disables/suppresses it, so the ask starts refusing immediately instead of waiting
// out the cache TTL.
func (s *SettingsService) DeleteTLSAskAllowed(host string) error {
	_, err := s.db.Exec("DELETE FROM tls_ask_allow WHERE host = ?", normalizeHostKey(host))
	return err
}

// Per-host TLS mode store — panel-local (server-panel's own SQLite). A host with
// mode "cf_origin" serves a static Cloudflare Origin cert (`tls <cert> <key>`)
// instead of on-demand Let's Encrypt — mutually exclusive, because a host proxied
// through Cloudflare can't complete an ACME challenge (it terminates at the CF
// edge). Absent row = "ondemand" (the default). Never a stack-DB write.

const (
	TLSModeOnDemand = "ondemand"
	TLSModeCFOrigin = "cf_origin"
)

// AllHostTLSModes returns every host's TLS mode, for injection into a reconcile
// snapshot. Only non-default (cf_origin) rows are stored, so the map is small. The
// cert/key are NOT stored per host — they're resolved from the Origin cert registry
// by hostname coverage, so there is exactly one way a host gets a certificate.
func (s *SettingsService) AllHostTLSModes() (map[string]caddydb.TLSOverride, error) {
	rows, err := s.db.Query("SELECT host, mode FROM vhost_tls_modes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]caddydb.TLSOverride{}
	for rows.Next() {
		var host, mode string
		if err := rows.Scan(&host, &mode); err != nil {
			return nil, err
		}
		out[host] = caddydb.TLSOverride{Mode: mode}
	}
	return out, rows.Err()
}

// SetHostTLSMode sets a host's TLS mode: "cf_origin" persists a row; "ondemand" or
// "" deletes it, returning the host to the default on-demand behavior.
func (s *SettingsService) SetHostTLSMode(host, mode string) error {
	key := normalizeHostKey(host)
	if key == "" {
		return fmt.Errorf("host is required")
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TLSModeCFOrigin:
		_, err := s.db.Exec(`INSERT INTO vhost_tls_modes (host, mode, updated_at)
			VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(host) DO UPDATE SET mode = excluded.mode, updated_at = CURRENT_TIMESTAMP`,
			key, TLSModeCFOrigin)
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
