package post

import (
	"net/http"
	"net/url"
	"net"
	"strings"

	postlogin "mthan/vps/routes/post/login"
	"mthan/vps/routes/post/ping"
	"mthan/vps/routes/post/session"
	"mthan/vps/routes/post/terminal"
	"mthan/vps/routes/post/update"
	"mthan/vps/routes/post/users"
	userlogin "mthan/vps/routes/post/user/login"
	"mthan/vps/services"
)

type Dependencies struct {
	Auth     *services.AuthService
	Sessions *services.SessionService
	Startup  services.StartupConfig
	Update   *services.UpdateService
}

func Register(mux *http.ServeMux, deps Dependencies) {
	mux.Handle("OPTIONS /post/", postOnly(deps.Startup, http.HandlerFunc(noContent)))
	mux.Handle("POST /post/login", postOnly(deps.Startup, postlogin.Handler(deps.Auth, deps.Sessions)))
	mux.Handle("POST /post/user/login", postOnly(deps.Startup, userlogin.Handler(deps.Auth)))
	mux.Handle("GET /post/session", postOnly(deps.Startup, session.Handler(deps.Sessions)))
	mux.Handle("GET /post/update", postOnly(deps.Startup, update.CheckHandler(deps.Update)))
	mux.Handle("POST /post/update", postOnly(deps.Startup, update.SelfUpdateHandler(deps.Update)))
	mux.Handle("POST /post/ping", postOnly(deps.Startup, ping.Handler()))
	mux.Handle("GET /post/users", postOnly(deps.Startup, users.Handler()))
	mux.Handle("GET /post/terminal", postOnly(deps.Startup, terminal.Handler(deps.Sessions)))
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
