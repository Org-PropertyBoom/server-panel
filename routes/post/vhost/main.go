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

// PhysicalHandler is the READ-ONLY physical-vhost status feed for the stack apps.
// No session/token — the route is gated to loopback + the Docker bridge by
// intranetOnly. It reports server-panel's own view (files it owns + the shared DB
// it reads), so the stacks never query Caddy. Never mutates.
func PhysicalHandler(engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, engine.PhysicalStatus(r.Context()))
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
