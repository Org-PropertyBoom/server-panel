package apps

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
		if _, ok := sessions.Get(cookie.Value); !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			var input struct {
				Name string `json:"name"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if err := services.InstallApp(input.Name); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"apps": services.DetectApps()}); err != nil {
			http.Error(w, "could not read app status", http.StatusInternalServerError)
		}
	})
}
