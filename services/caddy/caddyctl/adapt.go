// Package caddyctl is the engine's interface to Caddy: it adapts the main
// Caddyfile to JSON by shelling `caddy adapt` AS THE ROOT PROCESS (server-panel
// runs as root, so it can read the root-only vhosts folder), then reloads Caddy
// by POSTing that adapted JSON to the admin API `/load`.
//
// Adapting as root is THE structural fix for the 2026-07-11 outage: the root
// process reads /home/server/.caddy (which it owns) and produces fully-resolved
// JSON; the admin API then merely loads pre-adapted JSON and never does its own
// blind, folder-reading adapt as the `caddy` user. We therefore NEVER POST a raw
// Caddyfile to /load, and NEVER `systemctl reload`.
//
// Shelling the host's own `caddy` binary (rather than an in-process adapter)
// keeps server-panel a lean single binary and gives automatic module-parity with
// the running Caddy.
package caddyctl

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// minAdaptedLen is the floor below which an adapted config is treated as
// "empty/short" and REFUSED — the guard that would have caught the outage (an
// import that read nothing produced a near-empty config).
const minAdaptedLen = 64

// Adapter adapts the main Caddyfile to JSON via `caddy adapt`, run as the current
// (root) process. It satisfies reconcile.Adapter.
type Adapter struct {
	// Timeout bounds the adapt subprocess; 0 uses a 15s default.
	Timeout time.Duration
}

// Adapt runs `caddy adapt --config <filename>` and returns the JSON. The
// caddyfile bytes are ignored — the CLI re-reads the file (and its imports) from
// disk as root, which is exactly what makes the adapt see the folder. Returns an
// error on adapt failure OR an empty/suspiciously-short result (abort-on-empty gate).
func (a Adapter) Adapt(caddyfile []byte, filename string) ([]byte, []string, error) {
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "caddy", "adapt", "--config", filename)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("caddy adapt %s: %v: %s", filename, err, strings.TrimSpace(stderr.String()))
	}

	out := stdout.Bytes()
	if len(bytes.TrimSpace(out)) < minAdaptedLen {
		return nil, nil, fmt.Errorf("caddy adapt %s: refusing an empty/short adapted config (%d bytes) — this is the import-read-nothing signature; NOT reloading", filename, len(out))
	}

	var warnings []string
	if s := strings.TrimSpace(stderr.String()); s != "" {
		warnings = strings.Split(s, "\n")
	}
	return out, warnings, nil
}
