package services

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"ppt/server-panel/services/caddy/health"
)

// HealthProbeService owns the alert-only reachability probe (DNS + TLS per active
// tenant host). It is READ-ONLY: it never writes a file, removes a vhost, or
// reconciles — it only surfaces a warning signal orthogonal to reconcile drift.
//
// Enabled by default in root mode; disable with CADDY_HEALTH_PROBE=0. Tunable via
// CADDY_HEALTH_INTERVAL (seconds), CADDY_HEALTH_THRESHOLD (consecutive failures),
// and CADDY_HEALTH_SERVER_IPS (pin our IPs instead of deriving from the protected
// domains).
type HealthProbeService struct {
	prober  *health.Prober
	enabled bool
}

// NewHealthProbeService builds the probe over the engine's configured tenant hosts,
// using the protected (dashboard/panel) domains as the "our server" IP reference.
func NewHealthProbeService(engine *VhostEngineService) *HealthProbeService {
	cfg := health.Config{
		Interval:         envSeconds("CADDY_HEALTH_INTERVAL", 3*time.Minute),
		Threshold:        envPositiveInt("CADDY_HEALTH_THRESHOLD", 2),
		ReferenceDomains: engine.cfg.ProtectedHosts(),
		PinnedIPs:        splitCSV(os.Getenv("CADDY_HEALTH_SERVER_IPS")),
		Hosts:            engine.TenantHosts,
	}
	return &HealthProbeService{prober: health.New(cfg), enabled: healthProbeEnabled()}
}

// Enabled reports whether the probe loop runs.
func (h *HealthProbeService) Enabled() bool { return h != nil && h.enabled }

// Start launches the probe loop (root-only caller). No-op when disabled.
func (h *HealthProbeService) Start(ctx context.Context) {
	if !h.Enabled() {
		return
	}
	go h.prober.Run(ctx)
}

// Snapshot returns the latest per-host health, or nil when disabled.
func (h *HealthProbeService) Snapshot() map[string]health.Status {
	if !h.Enabled() {
		return nil
	}
	return h.prober.Snapshot()
}

// AlertCount is the number of hosts currently flagged unreachable (debounced).
func (h *HealthProbeService) AlertCount() int {
	if !h.Enabled() {
		return 0
	}
	return h.prober.AlertCount()
}

func healthProbeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CADDY_HEALTH_PROBE"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

func envSeconds(key string, def time.Duration) time.Duration {
	if n := envPositiveInt(key, 0); n > 0 {
		return time.Duration(n) * time.Second
	}
	return def
}

func envPositiveInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}
