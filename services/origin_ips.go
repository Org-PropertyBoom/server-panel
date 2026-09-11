package services

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"ppt/server-panel/services/origin"
)

// The process-wide "our origin IPs" set, shared by the Edge column (dns_probe.go)
// and the reachability probe (health_probe.go). Configure with PANEL_ORIGIN_IPS;
// the older CADDY_HEALTH_SERVER_IPS and CUTOVER_ORIGIN_IPS are still honoured and
// unioned with it. Unset → the four known origin addresses.
var (
	originSetOnce sync.Once
	originSet     *origin.Set
)

// OriginSet returns the shared origin-IP set (built from env on first use).
func OriginSet() *origin.Set {
	originSetOnce.Do(func() {
		originSet = origin.NewSet(origin.Configured(os.Getenv))
	})
	return originSet
}

// StartOriginDiscovery adds this instance's EC2 public addresses (IMDSv2) to the
// shared set in the background, refreshing every 30 minutes. It only ever ADDS to
// the configured list, fails silently off EC2, and never blocks startup. Disable
// with PANEL_ORIGIN_IMDS=0.
func StartOriginDiscovery(ctx context.Context) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PANEL_ORIGIN_IMDS"))) {
	case "0", "false", "no", "off":
		return
	}
	go OriginSet().Run(ctx, origin.NewIMDS(1500*time.Millisecond), 30*time.Minute)
}
