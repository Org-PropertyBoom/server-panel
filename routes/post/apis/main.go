package apis

import (
	"encoding/json"
	"net/http"
	"strings"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService, settings *services.SettingsService) http.Handler {
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

		switch r.Method {
		case http.MethodGet:
			keys, err := settings.APIKeys()
			if err != nil {
				http.Error(w, "API keys could not be loaded", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"apis": keys})
		case http.MethodPost:
			var input struct {
				AcceptedIPs []string `json:"acceptedIps"`
				Name        string   `json:"name"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			key, secret, err := settings.CreateAPIKey(input.Name, input.AcceptedIPs)
			if err != nil {
				http.Error(w, "API key could not be created", http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"api": key, "secret": secret})
		case http.MethodPatch:
			var input struct {
				AcceptedIPs *[]string `json:"acceptedIps"`
				Enabled     *bool     `json:"enabled"`
				ID          string    `json:"id"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.ID) == "" {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if input.Enabled == nil && input.AcceptedIPs == nil {
				http.Error(w, "no API key changes provided", http.StatusBadRequest)
				return
			}
			if input.Enabled != nil && settings.SetAPIKeyEnabled(input.ID, *input.Enabled) != nil {
				http.Error(w, "API key could not be updated", http.StatusNotFound)
				return
			}
			if input.AcceptedIPs != nil && settings.SetAPIKeyAcceptedIPs(input.ID, *input.AcceptedIPs) != nil {
				http.Error(w, "accepted IPs are invalid", http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		case http.MethodDelete:
			id := strings.TrimSpace(r.URL.Query().Get("id"))
			if id == "" {
				http.Error(w, "API key ID is required", http.StatusBadRequest)
				return
			}
			if err := settings.DeleteAPIKey(id); err != nil {
				http.Error(w, "API key could not be deleted", http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
