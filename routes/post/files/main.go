package files

import (
	"encoding/json"
	"errors"
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

		_, ok := sessions.Get(cookie.Value)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		requestedPath := r.URL.Query().Get("path")
		isContent := r.URL.Query().Get("content") == "true"

		// Root mode exposes the full filesystem and starts the explorer at /.
		homeDir := "/"

		if r.Method == http.MethodPut {
			var body struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Path == "" {
				http.Error(w, "path and content are required", http.StatusBadRequest)
				return
			}
			if err := services.WriteFileContent(body.Path, body.Content, homeDir, true); err != nil {
				switch {
				case errors.Is(err, services.ErrAccessDenied):
					http.Error(w, "access denied", http.StatusForbidden)
				case errors.Is(err, services.ErrProtectedPath):
					http.Error(w, err.Error(), http.StatusForbidden)
				default:
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}

		if isContent {
			content, err := services.GetFileContent(requestedPath, homeDir, true)
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

		list, err := services.ListDirectory(requestedPath, homeDir, true)
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
