package containers

import (
	"encoding/json"
	"net/http"

	"mthan/vps/services"
)

func Handler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
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

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"containers": containers.ListAll()}); err != nil {
			http.Error(w, "could not read containers", http.StatusInternalServerError)
		}
	})
}
