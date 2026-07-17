package settings

import (
	"encoding/json"
	"net/http"
	"strings"

	"mthan/vps/services"
)

var allowedKeys = map[string]bool{
	"general_app_name": true, "general_color_mode": true, "apps_header": true,
	"users_default_shell": true, "users_home_base": true, "users_create_home": true,
	"users_auto_username": true,
}

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
			values, err := settings.All()
			if err != nil {
				http.Error(w, "could not load settings", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"settings": values})
		case http.MethodPut:
			var input struct{ Key, Value string }
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !validSetting(input.Key, input.Value) {
				http.Error(w, "invalid setting", http.StatusBadRequest)
				return
			}
			if err := settings.Set(input.Key, input.Value); err != nil {
				http.Error(w, "could not save setting", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func validSetting(key, value string) bool {
	if !allowedKeys[key] {
		return false
	}
	switch key {
	case "general_app_name":
		return strings.TrimSpace(value) != "" && len(value) <= 80
	case "general_color_mode":
		return value == "system" || value == "light" || value == "dark"
	case "users_default_shell", "users_home_base":
		return strings.HasPrefix(value, "/") && !strings.Contains(value, "..") && len(value) <= 255
	case "users_create_home", "users_auto_username":
		return value == "true" || value == "false"
	case "apps_header":
		var apps []string
		if json.Unmarshal([]byte(value), &apps) != nil || len(apps) > 7 {
			return false
		}
		allowed := map[string]bool{"nginx": true, "mariadb": true, "redis": true, "docker": true, "podman": true, "node": true, "php": true}
		seen := make(map[string]bool)
		for _, app := range apps {
			if !allowed[app] || seen[app] {
				return false
			}
			seen[app] = true
		}
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
