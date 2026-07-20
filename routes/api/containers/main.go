package containers

import (
	"encoding/json"
	"net/http"

	"mthan/vps/services"
)

func UserHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{"containers": containers.ListCurrentUser(session.Username)})
	})
}

func requestSession(r *http.Request, sessions *services.SessionService) (services.Session, bool) {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return services.Session{}, false
	}
	return sessions.Get(cookie.Value)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
