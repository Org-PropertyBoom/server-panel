// Package tlsask serves Caddy's on-demand-TLS `ask` endpoint: GET
// /internal/tls-ask?domain=<host> → 200 to authorize obtaining a cert, any
// non-200 to refuse. Caddy calls this over loopback on every handshake for an
// uncached hostname, so issuance becomes traffic-driven AND authorized against
// the panel's own host data (active platform/website/redirect rows) — dead
// domains never trigger ACME.
//
// The handler is registered behind a loopback-only guard (Caddy calls it on
// 127.0.0.1) and root mode only. It is deliberately FAIL-CLOSED: a missing engine
// or any authorization error answers non-200, which only pauses NEW issuance —
// existing certs keep serving from Caddy's storage.
package tlsask

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ppt/server-panel/services"
)

func Handler(engine *services.VhostEngineService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := strings.TrimSpace(r.URL.Query().Get("domain"))
		if domain == "" {
			http.Error(w, "domain query parameter is required", http.StatusBadRequest)
			return
		}
		if engine == nil {
			http.Error(w, "vhost engine unavailable", http.StatusServiceUnavailable) // fail closed
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		allowed, err := engine.CertAllowed(ctx, domain)
		if err != nil || !allowed {
			// Any error or an unknown/disabled host → refuse. Caddy treats every
			// non-200 as "do not issue".
			http.Error(w, "not authorized for on-demand TLS", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
