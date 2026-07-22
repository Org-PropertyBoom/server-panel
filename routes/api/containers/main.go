package containers

import (
	"encoding/json"
	"errors"
	"net/http"

	"ppt/server-panel/services"
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

func UserDockerfileHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		id := r.URL.Query().Get("id")
		var result services.ContainerDockerfile
		var err error
		if r.Method == http.MethodGet {
			result, err = containers.DockerfileCurrentUser(session.Username, id)
		} else {
			var input struct {
				Content string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			result, err = containers.WriteDockerfileCurrentUser(session.Username, id, input.Content)
		}
		if err != nil {
			if errors.Is(err, services.ErrContainerDockerfileMissing) {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusForbidden)
			}
			return
		}
		writeJSON(w, result)
	})
}

func UserInspectHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		details, err := containers.InspectCurrentUser(session.Username, r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, details)
	})
}

func UserActionHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		var input struct {
			Action string `json:"action"`
			ID     string `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := containers.ActionCurrentUser(session.Username, input.ID, input.Action); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})
}

func UserLogsHandler(sessions *services.SessionService, containers *services.ContainerService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		logs, err := containers.LogsCurrentUser(session.Username, r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"logs": logs})
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
