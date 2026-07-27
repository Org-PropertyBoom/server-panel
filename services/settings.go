package services

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const settingsDBEnv = "SETTINGS_DB_PATH"

type SettingsService struct {
	db *sql.DB
}

func NewSettingsService() (*SettingsService, error) {
	path := settingsDBPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS apis (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		key_prefix TEXT NOT NULL,
		accepted_ips TEXT NOT NULL DEFAULT '[]',
		enabled INTEGER NOT NULL DEFAULT 1,
		last_used_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`ALTER TABLE apis ADD COLUMN accepted_ips TEXT NOT NULL DEFAULT '[]'`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		_ = db.Close()
		return nil, err
	}
	// Per-system-host response headers (panel-local render config — NOT the shared
	// pc-owned platform_hosts schema). host -> JSON map of header name->value.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS vhost_response_headers (
		host TEXT PRIMARY KEY,
		headers TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Hosts the operator disabled at the Caddy edge (panel-local; the stack DB row is
	// never touched — reconcile just stops rendering these).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS vhost_suppressed_hosts (
		host TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Per-host TLS mode (panel-local). Absent row = "ondemand" (Let's Encrypt via the
	// on-demand template). "cf_origin" renders a static `tls <cert> <key>` instead — for
	// hosts proxied through Cloudflare, which can't complete an ACME challenge (it
	// terminates at the CF edge). cert_path/key_path override the global default paths.
	// Registered Cloudflare Origin certificates. A LIST, deliberately with no
	// "default": propertyweb.co and propertyboom.co are peer zones, so designating
	// one as default would be arbitrary. The panel selects the entry whose PEM
	// covers a given hostname (x509 SAN match) when a host is switched to cf_origin.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS origin_certs (
		cert_path TEXT PRIMARY KEY,
		key_path TEXT NOT NULL,
		added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Migrate the legacy single "global default" pair into the registry as its first
	// entry, so existing cf_origin hosts (grafana, media) keep working unchanged.
	if _, err := db.Exec(`INSERT OR IGNORE INTO origin_certs (cert_path, key_path)
		SELECT c.value, k.value FROM settings c, settings k
		WHERE c.key = 'vhost_origin_cert_path' AND k.key = 'vhost_origin_key_path'
		  AND c.value != '' AND k.value != ''`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Hosts previously authorized for on-demand TLS. Caddy consults /internal/tls-ask
	// on HANDSHAKES (not just at issuance), and the endpoint is fail-closed — so this
	// allowlist is PERSISTED and re-read at boot, letting the panel answer 200 for
	// already-known-good hosts while it is restarting or still warming up. Without it,
	// a routine panel restart takes HTTPS down for every on-demand host.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tls_ask_allow (
		host TEXT PRIMARY KEY,
		allowed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS vhost_tls_modes (
		host TEXT PRIMARY KEY,
		mode TEXT NOT NULL DEFAULT 'ondemand',
		cert_path TEXT NOT NULL DEFAULT '',
		key_path TEXT NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Retire the per-host cert/key override (must run AFTER both tables exist): any
	// host that named its own cert has that pair PROMOTED into the registry, then the
	// override is cleared, so coverage-based selection routes the host to the same
	// file. Two mechanisms for one decision caused drift — hosts silently running on
	// a hand-typed path while the registry looked near-empty — so selection is now
	// the single way a host gets a cert. The columns stay for schema stability but
	// are no longer read or written.
	if _, err := db.Exec(`INSERT OR IGNORE INTO origin_certs (cert_path, key_path)
		SELECT cert_path, key_path FROM vhost_tls_modes
		WHERE cert_path != '' AND key_path != ''`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`UPDATE vhost_tls_modes SET cert_path = '', key_path = ''
		WHERE cert_path != '' OR key_path != ''`); err != nil {
		_ = db.Close()
		return nil, err
	}
	for oldKey, newKey := range map[string]string{
		"app_name": "general_app_name", "color_mode": "general_color_mode", "header_apps": "apps_header",
	} {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value)
			SELECT ?, value FROM settings WHERE key = ?`, newKey, oldKey); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	for key, value := range map[string]string{
		"general_app_name":    "Ppt Server Panel",
		"general_color_mode":  "system",
		"apps_header":         "[]",
		"users_default_shell": "/bin/bash",
		"users_home_base":     "/home",
		"users_create_home":   "true",
		"users_auto_username": "false",
	} {
		if _, err := db.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)", key, value); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &SettingsService{db: db}, nil
}

func (s *SettingsService) All() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *SettingsService) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
	return err
}

func (s *SettingsService) Get(key, fallback string) string {
	var value string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value); err != nil {
		return fallback
	}
	return value
}

func settingsDBPath() string {
	if path := os.Getenv(settingsDBEnv); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".ppt-server-panel", "data", "db.sqlite")
	}
	return filepath.Join(home, ".ppt-server-panel", "data", "db.sqlite")
}
