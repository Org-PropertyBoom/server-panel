package session

import (
	"encoding/json"
	"net/http"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(services.SessionCookieName)
		if err != nil {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		session, ok := sessions.Get(cookie.Value)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session": session,
			"status":  "ok",
		})
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
