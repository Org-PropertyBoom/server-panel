package vhost

import (
	"encoding/json"
	"errors"
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
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		res, _ := engine.PruneOrphan(r.Context(), body.Name)
		writeJSON(w, res)
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
