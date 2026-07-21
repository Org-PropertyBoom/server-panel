package post

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	postapis "ppt/server-panel/routes/post/apis"
	postapps "ppt/server-panel/routes/post/apps"
	appconfig "ppt/server-panel/routes/post/apps/config"
	postcontainers "ppt/server-panel/routes/post/containers"
	"ppt/server-panel/routes/post/datasources"
	postfiles "ppt/server-panel/routes/post/files"
	postlogin "ppt/server-panel/routes/post/login"
	"ppt/server-panel/routes/post/ping"
	"ppt/server-panel/routes/post/session"
	settingsroute "ppt/server-panel/routes/post/settings"
	"ppt/server-panel/routes/post/terminal"
	"ppt/server-panel/routes/post/update"
	useradd "ppt/server-panel/routes/post/user/add"
	userapps "ppt/server-panel/routes/post/user/apps"
	userdelete "ppt/server-panel/routes/post/user/delete"
	userlist "ppt/server-panel/routes/post/user/list"
	userlogin "ppt/server-panel/routes/post/user/login"
	userpassword "ppt/server-panel/routes/post/user/password"
	postvhost "ppt/server-panel/routes/post/vhost"
	"ppt/server-panel/services"
)

type Dependencies struct {
	Auth        *services.AuthService
	Sessions    *services.SessionService
	Startup     services.StartupConfig
	Update      *services.UpdateService
	System      *services.SystemService
	Settings    *services.SettingsService
	VhostEngine *services.VhostEngineService
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
	mux.Handle("GET /post/user/list", postOnly(deps.Startup, userlist.Handler()))
	mux.Handle("POST /post/user/password", postOnly(deps.Startup, userpassword.Handler()))
	mux.Handle("GET /post/files", postOnly(deps.Startup, postfiles.Handler(deps.Sessions)))
	mux.Handle("GET /post/apps", postOnly(deps.Startup, postapps.Handler(deps.Sessions)))
	mux.Handle("GET /post/containers", postOnly(deps.Startup, postcontainers.Handler(deps.Sessions, services.NewContainerService())))
	mux.Handle("POST /post/containers/action", postOnly(deps.Startup, postcontainers.ActionHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /post/containers/logs", postOnly(deps.Startup, postcontainers.LogsHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("GET /post/containers/dockerfile", postOnly(deps.Startup, postcontainers.DockerfileHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("PUT /post/containers/dockerfile", postOnly(deps.Startup, postcontainers.DockerfileHandler(deps.Sessions, services.NewContainerService())))
	mux.Handle("POST /post/apps", postOnly(deps.Startup, postapps.Handler(deps.Sessions)))
	mux.Handle("GET /post/apps/config", postOnly(deps.Startup, appconfig.Handler(deps.Sessions, services.NewAppConfigService())))
	mux.Handle("PUT /post/apps/config", postOnly(deps.Startup, appconfig.Handler(deps.Sessions, services.NewAppConfigService())))
	mux.Handle("GET /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("POST /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("PATCH /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("DELETE /post/apis", postOnly(deps.Startup, postapis.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("GET /post/settings", postOnly(deps.Startup, settingsroute.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("PUT /post/settings", postOnly(deps.Startup, settingsroute.Handler(deps.Sessions, deps.Settings)))
	mux.Handle("GET /post/datasources", postOnly(deps.Startup, datasources.Handler(deps.Sessions, services.NewDataSourceService())))
	mux.Handle("PUT /post/datasources", postOnly(deps.Startup, datasources.Handler(deps.Sessions, services.NewDataSourceService())))
	mux.Handle("DELETE /post/datasources", postOnly(deps.Startup, datasources.Handler(deps.Sessions, services.NewDataSourceService())))
	mux.Handle("POST /post/datasources/test", postOnly(deps.Startup, datasources.TestHandler(deps.Sessions, services.NewDataSourceService())))
	mux.Handle("GET /post/vhost/state", postOnly(deps.Startup, postvhost.StateHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/reconcile", postOnly(deps.Startup, postvhost.ReconcileHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/reload", postOnly(deps.Startup, postvhost.ReloadHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/system", postOnly(deps.Startup, postvhost.SystemHostHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("PUT /post/vhost/system", postOnly(deps.Startup, postvhost.SystemHostHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("DELETE /post/vhost/system", postOnly(deps.Startup, postvhost.SystemHostHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/redirect", postOnly(deps.Startup, postvhost.RedirectHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("PUT /post/vhost/redirect", postOnly(deps.Startup, postvhost.RedirectHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("DELETE /post/vhost/redirect", postOnly(deps.Startup, postvhost.RedirectHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/orphan/prune", postOnly(deps.Startup, postvhost.OrphanPruneHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("POST /post/vhost/gate", postOnly(deps.Startup, postvhost.GateHandler(deps.Sessions, deps.VhostEngine)))
	mux.Handle("GET /post/vhost/rendered", intranetOnly(deps.Startup, postvhost.RenderedHandler(deps.VhostEngine)))
	mux.Handle("GET /post/vhost", postOnly(deps.Startup, postvhost.Handler(deps.Sessions, services.NewVHostService())))
	mux.Handle("GET /post/vhost/", postOnly(deps.Startup, postvhost.Handler(deps.Sessions, services.NewVHostService())))
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

// intranetOnly gates a READ-ONLY route to loopback + the Docker-bridge source IPs
// (no session/token) so co-located stack apps can read it server-to-server, while
// it stays unreachable from the public internet. Still root-only (the data comes
// from the root-owned vhosts folder). Use ONLY for non-mutating endpoints.
func intranetOnly(startup services.StartupConfig, next http.Handler) http.Handler {
	return sourceIntranetOnly(rootOnly(startup, next))
}

func sourceIntranetOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isIntranet(remoteHost(r.RemoteAddr)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// intranetNets defaults to loopback + the Docker bridge range (172.16.0.0/12 covers
// the default bridge and user-defined networks). Override with VHOST_INTRANET_CIDRS
// (comma-separated CIDRs).
var intranetNets = parseIntranetCIDRs()

func parseIntranetCIDRs() []*net.IPNet {
	spec := os.Getenv("VHOST_INTRANET_CIDRS")
	if strings.TrimSpace(spec) == "" {
		spec = "127.0.0.0/8,::1/128,172.16.0.0/12"
	}
	var nets []*net.IPNet
	for _, c := range strings.Split(spec, ",") {
		if c = strings.TrimSpace(c); c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func isIntranet(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range intranetNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
