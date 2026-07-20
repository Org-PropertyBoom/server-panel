package settings

import (
	"encoding/json"
	"net/http"

	"ppt/server-panel/services"
)

func Handler(sessions *services.SessionService, settings *services.SettingsService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r, sessions) {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		handle(w, r, settings)
	})
}

func validSession(r *http.Request, sessions *services.SessionService) bool {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return false
	}
	_, ok := sessions.Get(cookie.Value)
	return ok
}

func handle(w http.ResponseWriter, r *http.Request, settings *services.SettingsService) {
	if r.Method == http.MethodGet {
		values, err := settings.All()
		if err != nil {
			http.Error(w, "could not load settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"settings": values})
		return
	}
	var input struct{ Key, Value string }
	if json.NewDecoder(r.Body).Decode(&input) != nil || !services.ValidSetting(input.Key, input.Value) {
		http.Error(w, "invalid setting", http.StatusBadRequest)
		return
	}
	if err := settings.Set(input.Key, input.Value); err != nil {
		http.Error(w, "could not save setting", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
