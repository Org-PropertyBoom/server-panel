package services

import (
	"encoding/json"
	"strings"
)

var allowedSettingKeys = map[string]bool{
	"general_app_name": true, "general_color_mode": true, "apps_header": true,
	"users_default_shell": true, "users_home_base": true, "users_create_home": true,
	"users_auto_username": true,
}

func ValidSetting(key, value string) bool {
	if !allowedSettingKeys[key] {
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
		if json.Unmarshal([]byte(value), &apps) != nil || len(apps) > 8 {
			return false
		}
		allowed := map[string]bool{"caddy": true, "nginx": true, "mariadb": true, "redis": true, "docker": true, "podman": true, "node": true, "php": true}
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
