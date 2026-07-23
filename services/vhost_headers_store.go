package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ppt/server-panel/services/caddy/render"
)

// vhost response-header store — panel-local (server-panel's own SQLite), NOT the
// shared pc-owned platform_hosts schema. Keyed by host; rendered into the system
// host's Caddy block by the reconcile engine (System hosts only).

// normalizeHost lowercases + trims a host used as the store key (matches the render
// key so lookups line up).
func normalizeHostKey(host string) string { return strings.ToLower(strings.TrimSpace(host)) }

// SanitizeHeaders validates a header map: names must be [A-Za-z0-9-]+ and values
// must be safe inside a quoted Caddy directive (no newlines/quotes/braces/control).
// Returns a cleaned map (trimmed keys) or an error naming the first bad entry.
func SanitizeHeaders(in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for name, value := range in {
		n := strings.TrimSpace(name)
		if n == "" {
			continue // skip blank rows from the UI
		}
		if !render.ValidHeaderName(n) {
			return nil, fmt.Errorf("invalid header name %q (letters, digits, hyphens only)", n)
		}
		if !render.ValidHeaderValue(value) {
			return nil, fmt.Errorf("invalid value for header %q (no newlines, quotes, backslash, braces, or control characters)", n)
		}
		out[n] = value
	}
	return out, nil
}

// VhostHeaders returns the response headers configured for one host ({} if none).
func (s *SettingsService) VhostHeaders(host string) (map[string]string, error) {
	var raw string
	err := s.db.QueryRow("SELECT headers FROM vhost_response_headers WHERE host = ?", normalizeHostKey(host)).Scan(&raw)
	if err != nil {
		return map[string]string{}, nil //nolint:nilerr // no row = no headers
	}
	m := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &m)
	return m, nil
}

// AllVhostHeaders returns every host's configured headers (host -> name->value),
// for injection into a reconcile snapshot. Hosts with no headers are omitted.
func (s *SettingsService) AllVhostHeaders() (map[string]map[string]string, error) {
	rows, err := s.db.Query("SELECT host, headers FROM vhost_response_headers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var host, raw string
		if err := rows.Scan(&host, &raw); err != nil {
			return nil, err
		}
		m := map[string]string{}
		if json.Unmarshal([]byte(raw), &m) == nil && len(m) > 0 {
			out[host] = m
		}
	}
	return out, rows.Err()
}

// SetVhostHeaders replaces the headers for a host (validated). An empty map deletes
// the row (back to default behavior).
func (s *SettingsService) SetVhostHeaders(host string, headers map[string]string) error {
	key := normalizeHostKey(host)
	if key == "" {
		return fmt.Errorf("host is required")
	}
	clean, err := SanitizeHeaders(headers)
	if err != nil {
		return err
	}
	if len(clean) == 0 {
		_, err := s.db.Exec("DELETE FROM vhost_response_headers WHERE host = ?", key)
		return err
	}
	// Marshal deterministically (sorted keys) so the stored blob is stable.
	names := make([]string, 0, len(clean))
	for n := range clean {
		names = append(names, n)
	}
	sort.Strings(names)
	ordered := make(map[string]string, len(clean))
	for _, n := range names {
		ordered[n] = clean[n]
	}
	blob, err := json.Marshal(ordered)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO vhost_response_headers (host, headers, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(host) DO UPDATE SET headers = excluded.headers, updated_at = CURRENT_TIMESTAMP`, key, string(blob))
	return err
}

// DeleteVhostHeaders removes a host's headers (called when the system host is
// deleted or renamed away).
func (s *SettingsService) DeleteVhostHeaders(host string) error {
	_, err := s.db.Exec("DELETE FROM vhost_response_headers WHERE host = ?", normalizeHostKey(host))
	return err
}
