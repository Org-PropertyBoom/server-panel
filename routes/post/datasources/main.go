package datasources

import (
	"encoding/json"
	"errors"
	"net/http"

	"mthan/vps/services"
)

// Handler serves the data-source list CRUD. Registered under the root-only
// postOnly middleware, so the secret-bearing surface is reachable only from the
// root process on the same host. Passwords are never returned (List yields views).
func Handler(sessions *services.SessionService, svc *services.DataSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			list, err := svc.List()
			if err != nil {
				http.Error(w, "could not load data sources", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"dataSources": list})
		case http.MethodPut, http.MethodPost:
			var in services.DataSource
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			view, err := svc.Save(in)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, services.ErrDataSourceNotFound) {
					status = http.StatusNotFound
				}
				http.Error(w, err.Error(), status)
				return
			}
			writeJSON(w, http.StatusOK, view)
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id is required", http.StatusBadRequest)
				return
			}
			if err := svc.Delete(id); err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, services.ErrDataSourceNotFound) {
					status = http.StatusNotFound
				}
				http.Error(w, err.Error(), status)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// TestHandler runs a per-source liveness Test (ping + ProbeQuery).
func TestHandler(sessions *services.SessionService, svc *services.DataSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, svc.Test(r.Context(), body.ID))
	})
}

func validSession(r *http.Request, sessions *services.SessionService) bool {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return false
	}
	_, ok := sessions.Get(cookie.Value)
	return ok
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
