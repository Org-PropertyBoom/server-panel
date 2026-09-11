package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	containersroute "ppt/server-panel/routes/api/containers"
	apifiles "ppt/server-panel/routes/api/files"
	settingsroute "ppt/server-panel/routes/api/settings"
	vhostroute "ppt/server-panel/routes/api/vhost"
	"ppt/server-panel/services"
)

type Dependencies struct {
	Health      *services.HealthService
	PostBaseURL string
	Sessions    *services.SessionService
	System      *services.SystemService
	Settings    *services.SettingsService
}

func Register(mux *http.ServeMux, deps Dependencies) {
	postClient := newPostClient(deps.PostBaseURL)

	mux.Handle("OPTIONS /api", public(http.HandlerFunc(noContent)))
	mux.Handle("OPTIONS /api/", public(http.HandlerFunc(noContent)))

	mux.Handle("GET /api", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "vps",
			"status":  "ok",
		})
	})))

	mux.Handle("GET /api/healthz", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Status())
	})))

	mux.Handle("GET /api/session", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := requestSession(r, deps.Sessions)
		if !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session": session,
			"status":  "ok",
		})
	})))

	mux.Handle("GET /api/system", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requestSession(r, deps.Sessions); !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		status, err := deps.System.Status()
		if err != nil {
			http.Error(w, "system information unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})))

	// Read-only update status for any logged-in session. It asks the root process
	// (the one that can install) rather than GitHub, so every session shares root's
	// cached check and the notice reflects what the Root Session would install.
	// Installing stays POST /post/update, root-only.
	mux.Handle("GET /api/update", public(updateStatusHandler(deps.Sessions, postClient)))

	mux.Handle("GET /api/apps", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requestSession(r, deps.Sessions); !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"apps": services.DetectApps()})
	})))
	mux.Handle("GET /api/settings", public(settingsroute.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("PUT /api/settings", public(settingsroute.Handler(deps.Sessions, deps.Settings)))

	mux.Handle("POST /api/login", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var credentials services.LoginCredentials
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := postClient.LoginUser(r.Context(), credentials)
		if err != nil {
			writeLoginError(w, err)
			return
		}

		session, err := deps.Sessions.Create(user, "user")
		if err != nil {
			http.Error(w, "session could not be created", http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, r, deps.Sessions, session)
		writeJSON(w, http.StatusOK, loginResponse{
			Session: &session,
			Status:  "ok",
			User:    user,
		})
	})))

	mux.Handle("GET /api/files", public(apifiles.Handler(deps.Sessions)))
	mux.Handle("GET /api/containers", public(containersroute.UserHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("POST /api/containers/action", public(containersroute.UserActionHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /api/containers/logs", public(containersroute.UserLogsHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /api/containers/inspect", public(containersroute.UserInspectHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /api/containers/dockerfile", public(containersroute.UserDockerfileHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("PUT /api/containers/dockerfile", public(containersroute.UserDockerfileHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /api/vhost", public(vhostroute.Handler(deps.Sessions, services.NewVHostService())))
	mux.Handle("GET /api/vhost/", public(vhostroute.Handler(deps.Sessions, services.NewVHostService())))

	mux.Handle("GET /healthz", public(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Status())
	})))
}

func requestSession(r *http.Request, sessions *services.SessionService) (services.Session, bool) {
	cookie, err := r.Cookie(services.SessionCookieName)
	if err != nil {
		return services.Session{}, false
	}

	return sessions.Get(cookie.Value)
}

var (
	errPostAuthUnavailable = errors.New("post auth unavailable")
	errPostLoginFailed     = errors.New("post login failed")
)

type loginResponse struct {
	Session *services.Session          `json:"session,omitempty"`
	Status  string                     `json:"status"`
	User    services.AuthenticatedUser `json:"user"`
}

type postClient struct {
	baseURL    string
	httpClient *http.Client
	// updateClient allows longer than login: on a cache miss the root process waits
	// on GitHub before it answers.
	updateClient *http.Client
}

func newPostClient(baseURL string) postClient {
	return postClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		updateClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c postClient) LoginUser(ctx context.Context, credentials services.LoginCredentials) (services.AuthenticatedUser, error) {
	payload, err := json.Marshal(credentials)
	if err != nil {
		return services.AuthenticatedUser{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/post/user/login", bytes.NewReader(payload))
	if err != nil {
		return services.AuthenticatedUser{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return services.AuthenticatedUser{}, errPostAuthUnavailable
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return services.AuthenticatedUser{}, services.ErrInvalidCredentials
	case http.StatusForbidden, http.StatusServiceUnavailable:
		return services.AuthenticatedUser{}, errPostAuthUnavailable
	default:
		return services.AuthenticatedUser{}, errPostLoginFailed
	}

	var login loginResponse
	if err := json.NewDecoder(response.Body).Decode(&login); err != nil {
		return services.AuthenticatedUser{}, errPostLoginFailed
	}

	return login.User, nil
}

// updateStatusHandler serves GET /api/update: a logged-in session gets the root
// process's cached update check. There is no install path here.
func updateStatusHandler(sessions *services.SessionService, client postClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requestSession(r, sessions); !ok {
			http.Error(w, "session invalid", http.StatusUnauthorized)
			return
		}
		refresh := r.URL.Query().Get("refresh") == "1"
		status, err := client.UpdateStatus(r.Context(), refresh)
		if err != nil {
			http.Error(w, "update status unavailable", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

// UpdateStatus asks the root process for its cached update check. Like LoginUser it
// sends no Origin or Referer: postOnly accepts a source-less request only from
// localhost, which is how the panel's processes reach each other.
//
// refresh forwards a manual "Check Update". The query string is built here rather
// than copied from the incoming request, so nothing else a caller appends can
// reach the root process.
func (c postClient) UpdateStatus(ctx context.Context, refresh bool) (services.UpdateCheckResult, error) {
	target := c.baseURL + "/post/update"
	if refresh {
		target += "?refresh=1"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return services.UpdateCheckResult{}, err
	}

	response, err := c.updateClient.Do(request)
	if err != nil {
		return services.UpdateCheckResult{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return services.UpdateCheckResult{}, errors.New("root update check failed: " + response.Status)
	}

	var result services.UpdateCheckResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return services.UpdateCheckResult{}, err
	}
	return result, nil
}

func writeLoginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	case errors.Is(err, errPostAuthUnavailable):
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "login failed", http.StatusInternalServerError)
	}
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

func public(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		next.ServeHTTP(w, r)
	})
}

func noContent(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
