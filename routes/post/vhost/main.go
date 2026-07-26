package vhost

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"ppt/server-panel/services"
)

// Handler serves the legacy read-only Caddy viewer (status / list / get-by-host)
// backed by `caddy adapt` of the running config.
func Handler(sessions *services.SessionService, vhosts *services.VHostService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/post/vhost")
		switch path {
		case "", "/":
			writeJSON(w, vhosts.Status())
		case "/list":
			writeJSON(w, map[string]any{"vhosts": vhosts.Summaries()})
		default:
			hostname := strings.TrimPrefix(path, "/")
			if hostname == "" || strings.Contains(hostname, "/") {
				http.Error(w, "vhost not found", http.StatusNotFound)
				return
			}
			host, err := vhosts.Get(hostname)
			if errors.Is(err, services.ErrVHostNotFound) {
				http.Error(w, "vhost not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "vhost information unavailable", http.StatusInternalServerError)
				return
			}
			writeJSON(w, host)
		}
	})
}

// StateHandler returns the read-only Caddy vhost DRIFT view: desired state read
// from the chosen host-source Data Source vs the vhosts folder. Mutates nothing.
func StateHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		writeJSON(w, engine.State(r.Context()))
	})
}

// ReconcileHandler applies desired state (render → validate → reload). GATED by
// CADDY_LIVE_RELOAD; the truthful Result is returned either way.
func ReconcileHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		res, _ := engine.Reconcile(r.Context())
		writeJSON(w, res)
	})
}

// ReloadHandler re-validates and reloads the current folder. GATED.
func ReloadHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		res, _ := engine.ReloadOnly(r.Context())
		writeJSON(w, res)
	})
}

// SystemHostHandler manages platform_hosts rows (create/update/soft-delete). DB
// writes only — changes go live on the next Reconcile.
func SystemHostHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut:
			var f services.SystemHostForm
			if json.NewDecoder(r.Body).Decode(&f) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if err := engine.SaveSystemHost(r.Context(), f); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		case http.MethodDelete:
			id := queryID(r)
			if id == 0 {
				http.Error(w, "id is required", http.StatusBadRequest)
				return
			}
			if err := engine.DeleteSystemHost(r.Context(), id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// RedirectHandler manages platform_redirect_hosts rows.
func RedirectHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut:
			var f services.RedirectForm
			if json.NewDecoder(r.Body).Decode(&f) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if err := engine.SaveRedirect(r.Context(), f); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		case http.MethodDelete:
			id := queryID(r)
			if id == 0 {
				http.Error(w, "id is required", http.StatusBadRequest)
				return
			}
			if err := engine.DeleteRedirect(r.Context(), id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// OrphanPruneHandler removes one orphan file (refusing protected/wildcard) then
// reconciles. GATED.
func OrphanPruneHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Name  string   `json:"name"`
			Names []string `json:"names"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		names := body.Names
		if len(names) == 0 && body.Name != "" {
			names = []string{body.Name}
		}
		if len(names) == 0 {
			http.Error(w, "name or names is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.PruneOrphans(r.Context(), names)
		writeJSON(w, res)
	})
}

// GateHandler flips the runtime live-reconcile gate (persisted setting, immediate,
// no restart). Root-only + authed via postOnly. The coded safety net (first-pass
// suppression, dashboard assert, validate + backup before reload, drop-guard) is
// unaffected — this only flips the operational gate; disarm is always safe.
func GateHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := engine.SetLiveReload(body.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("vhost live-reconcile gate toggled: enabled=%v", body.Enabled)
		writeJSON(w, map[string]bool{"liveReload": engine.LiveReloadEnabled()})
	})
}

// OnDemandTLSHandler flips the on-demand-TLS render toggle (persisted setting).
// When ON, the next Reconcile rewrites every host file with a `tls { on_demand }`
// block so Caddy issues certs traffic-driven (gated by the /internal/tls-ask
// endpoint). Requires the global `on_demand_tls ask` block in the main Caddyfile
// FIRST — otherwise the adapt-validate guard refuses the reload (safe, no outage).
func OnDemandTLSHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := engine.SetOnDemandTLS(body.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("vhost on-demand-TLS render toggled: enabled=%v", body.Enabled)
		writeJSON(w, map[string]bool{"onDemandTls": engine.OnDemandTLSEnabled()})
	})
}

// RedirectTargetsHandler returns the active tenant domains as redirect-target
// suggestions for the combobox (read-only).
func RedirectTargetsHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		targets, err := engine.RedirectTargets(r.Context())
		if err != nil {
			writeJSON(w, map[string]any{"targets": []any{}})
			return
		}
		writeJSON(w, map[string]any{"targets": targets})
	})
}

// PinnedRemoveHandler removes a "Pinned · unmanaged" static block from the main
// Caddyfile (validated + reloaded). GATED; the truthful Result is returned either way.
func PinnedRemoveHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Host string `json:"host"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Host) == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.RemovePinnedBlock(r.Context(), body.Host)
		writeJSON(w, res)
	})
}

// PinHandler converts an Active platform_hosts row into a static Caddyfile block
// (adds the block + validated reload, then drops the DB row). GATED; the truthful
// Result is returned either way.
func PinHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			ID int64 `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.ID == 0 {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.PinRoute(r.Context(), body.ID)
		writeJSON(w, res)
	})
}

// UnpinHandler converts a "Pinned · unmanaged" static block back into a managed
// platform_hosts row (removes the block + validated reload, then adopts the DB
// row). REFUSED on protected domains. GATED; the truthful Result is returned either way.
func UnpinHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Host string `json:"host"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Host) == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.UnpinRoute(r.Context(), body.Host)
		writeJSON(w, res)
	})
}

// SuppressHandler edge-disables/enables a host at Caddy — tenant or redirect
// (panel-local, no DB write) then reconciles. GATED; returns the truthful Result.
func SuppressHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Host       string `json:"host"`
			Suppressed bool   `json:"suppressed"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Host) == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.SuppressHost(r.Context(), body.Host, body.Suppressed)
		writeJSON(w, res)
	})
}

// TLSModeHandler sets a host's TLS mode (ondemand | cf_origin) with an optional
// per-host cert/key override, then reconciles. cf_origin serves a static Cloudflare
// Origin cert (for proxied hosts); ondemand returns it to on-demand LE. Returns the
// reconcile Result.
func TLSModeHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Host     string `json:"host"`
			Mode     string `json:"mode"`
			CertPath string `json:"certPath"`
			KeyPath  string `json:"keyPath"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Host) == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.SetHostTLSMode(r.Context(), body.Host, body.Mode, body.CertPath, body.KeyPath)
		writeJSON(w, res)
	})
}

// OriginCertHandler persists the global default Cloudflare Origin cert + key paths
// (absolute) that cf_origin hosts fall back to. Takes effect on the next Reconcile.
func OriginCertHandler(sessions *services.SessionService, engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed(sessions, r) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			Cert string `json:"cert"`
			Key  string `json:"key"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := engine.SetOriginCertPaths(body.Cert, body.Key); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cert, key := engine.OriginCertPaths()
		writeJSON(w, map[string]string{"originCert": cert, "originKey": key})
	})
}

func authed(sessions *services.SessionService, r *http.Request) bool {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return false
	}
	_, ok := sessions.Get(cookie.Value)
	return ok
}

func queryID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	return id
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
