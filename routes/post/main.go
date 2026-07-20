package post

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	postapis "mthan/vps/routes/post/apis"
	postapps "mthan/vps/routes/post/apps"
	postcontainers "mthan/vps/routes/post/containers"
	postfiles "mthan/vps/routes/post/files"
	postlogin "mthan/vps/routes/post/login"
	"mthan/vps/routes/post/ping"
	"mthan/vps/routes/post/session"
	"mthan/vps/routes/post/terminal"
	"mthan/vps/routes/post/update"
	useradd "mthan/vps/routes/post/user/add"
	userapps "mthan/vps/routes/post/user/apps"
	userdelete "mthan/vps/routes/post/user/delete"
	userlogin "mthan/vps/routes/post/user/login"
	"mthan/vps/routes/post/users"
	settingsroute "mthan/vps/routes/settings"
	"mthan/vps/services"
)

type Dependencies struct {
	Auth     *services.AuthService
	Sessions *services.SessionService
	Startup  services.StartupConfig
	Update   *services.UpdateService
	System   *services.SystemService
	Settings *services.SettingsService
}

func Register(mux *http.ServeMux, deps Dependencies) {
	mux.Handle("OPTIONS /post/", postOnly(deps.Startup, http.HandlerFunc(noContent)))
	mux.Handle("POST /post/login", postOnly(deps.Startup, postlogin.Handler(deps.Auth, deps.Sessions)))
	mux.Handle("POST /post/user/login", postOnly(deps.Startup, userlogin.Handler(deps.Auth)))
	mux.Handle("POST /post/user/add", postOnly(deps.Startup, useradd.Handler(deps.Settings)))
	mux.Handle("GET /post/user/apps", postOnly(deps.Startup, userapps.Handler()))
	mux.Handle("POST /post/user/apps", postOnly(deps.Startup, userapps.Handler()))
	mux.Handle("POST /post/user/delete", postOnly(deps.Startup, userdelete.Handler()))
	mux.Handle("GET /post/session", postOnly(deps.Startup, session.Handler(deps.Sessions)))
	mux.Handle("GET /post/system", postOnly(deps.Startup, authenticatedSystemHandler(deps.Sessions, deps.System)))
	mux.Handle("GET /post/update", postOnly(deps.Startup, update.CheckHandler(deps.Update)))
	mux.Handle("POST /post/update", postOnly(deps.Startup, update.SelfUpdateHandler(deps.Update)))
	mux.Handle("POST /post/ping", postOnly(deps.Startup, ping.Handler()))
	mux.Handle("GET /post/users", postOnly(deps.Startup, users.Handler()))
	mux.Handle("GET /post/files", postOnly(deps.Startup, postfiles.Handler(deps.Sessions)))
	mux.Handle("GET /post/apps", postOnly(deps.Startup, postapps.Handler(deps.Sessions)))
	mux.Handle("GET /post/containers", postOnly(deps.Startup, postcontainers.Handler(deps.Sessions, services.NewContainerService())))
	mux.Handle("POST /post/apps", postOnly(deps.Startup, postapps.Handler(deps.Sessions)))
	mux.Handle("GET /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("POST /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("PATCH /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("DELETE /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("GET /post/settings", postOnly(deps.Startup, settingsroute.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("PUT /post/settings", postOnly(deps.Startup, settingsroute.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("GET /post/terminal", postOnly(deps.Startup, terminal.Handler(deps.Sessions)))
}

func authenticatedSystemHandler(sessions *services.SessionService, system *services.SystemService) http.Handler {
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
		status, err := system.Status()
		if err != nil {
			http.Error(w, "system information unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			http.Error(w, "system information unavailable", http.StatusInternalServerError)
		}
	})
}

func postOnly(startup services.StartupConfig, next http.Handler) http.Handler {
	return sameDomainOrLocalhostOnly(rootOnly(startup, next))
}

func rootOnly(startup services.StartupConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !startup.IsRoot {
			http.Error(w, "root process required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func sameDomainOrLocalhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := requestSource(r)
		if !isAllowedPostSource(r, source) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if source != "" {
			w.Header().Set("Access-Control-Allow-Origin", source)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		next.ServeHTTP(w, r)
	})
}

func requestSource(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin
	}

	if referer := r.Header.Get("Referer"); referer != "" {
		return referer
	}

	return ""
}

func isAllowedPostSource(r *http.Request, source string) bool {
	if source == "" {
		return isLocalhost(remoteHost(r.RemoteAddr))
	}

	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" {
		return false
	}

	sourceHost := strings.ToLower(parsed.Hostname())
	requestHost := strings.ToLower(hostname(r.Host))

	return sourceHost == requestHost || isLocalhost(sourceHost)
}

func hostname(host string) string {
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(parsedHost, "[]")
	}

	return strings.Trim(host, "[]")
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return hostname(remoteAddr)
	}

	return strings.ToLower(host)
}

func isLocalhost(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func noContent(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
