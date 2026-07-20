package login

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ppt/server-panel/services"
)

type response struct {
	Status string                     `json:"status"`
	User   services.AuthenticatedUser `json:"user"`
}

func Handler(auth *services.AuthService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var credentials services.LoginCredentials
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if !strings.HasPrefix(credentials.Username, "user-") {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		user, err := auth.AuthenticateLinuxUser(credentials)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response{
			Status: "ok",
			User:   user,
		})
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	case errors.Is(err, services.ErrAuthUnavailable):
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "auth failed", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
