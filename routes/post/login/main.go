package login

import (
	"encoding/json"
	"errors"
	"net/http"

	"ppt/server-panel/services"
)

type response struct {
	Session *services.Session          `json:"session,omitempty"`
	Status  string                     `json:"status"`
	User    services.AuthenticatedUser `json:"user"`
}

func Handler(auth *services.AuthService, sessions *services.SessionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var credentials services.LoginCredentials
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := auth.AuthenticateLinuxUser(credentials)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		session, err := sessions.Create(user, "root")
		if err != nil {
			http.Error(w, "session could not be created", http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, r, sessions, session)
		writeJSON(w, http.StatusOK, response{
			Session: &session,
			Status:  "ok",
			User:    user,
		})
	})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, sessions *services.SessionService, session services.Session) {
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		MaxAge:   sessions.MaxAge(),
		Name:     services.SessionCookieName,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		Value:    session.Token,
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
