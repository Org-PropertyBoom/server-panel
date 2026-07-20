package config

import (
	"encoding/json"
	"errors"
	"net/http"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService, configs *services.AppConfigService) http.Handler {
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

		app := r.URL.Query().Get("app")
		path := r.URL.Query().Get("path")
		if r.Method == http.MethodGet {
			result, err := configs.Read(app, path)
			writeResult(w, result, err)
			return
		}

		var input struct {
			Content string `json:"content"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		result, err := configs.Write(app, path, input.Content)
		writeResult(w, result, err)
	})
}

func writeResult(w http.ResponseWriter, result services.AppConfigFile, err error) {
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAppConfigDenied):
			http.Error(w, err.Error(), http.StatusForbidden)
		case errors.Is(err, services.ErrAppConfigInvalid), errors.Is(err, services.ErrAppConfigTooLarge):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "app configuration unavailable", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
