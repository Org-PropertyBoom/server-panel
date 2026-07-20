package files

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/user"

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

		requestedPath := r.URL.Query().Get("path")
		isContent := r.URL.Query().Get("content") == "true"

		homeDir := ""
		u, err := user.Lookup(session.Username)
		if err == nil && u.HomeDir != "" {
			homeDir = u.HomeDir
		} else {
			homeDir = "/home/" + session.Username
		}

		if isContent {
			content, err := services.GetFileContent(requestedPath, homeDir, false)
			if err != nil {
				if errors.Is(err, services.ErrAccessDenied) {
					http.Error(w, "access denied", http.StatusForbidden)
				} else {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			writeJSON(w, http.StatusOK, content)
			return
		}

		list, err := services.ListDirectory(requestedPath, homeDir, false)
		if err != nil {
			if errors.Is(err, services.ErrAccessDenied) {
				http.Error(w, "access denied", http.StatusForbidden)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		writeJSON(w, http.StatusOK, list)
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
