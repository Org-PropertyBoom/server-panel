package vhost

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService, vhosts *services.VHostService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(services.SessionCookieName)
		if err != nil {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		if _, ok := sessions.Get(cookie.Value); !ok {
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

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
