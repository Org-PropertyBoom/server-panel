package caddyctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Compiling the adapted config to a caddy-readable file on disk.
//
// WHY THIS EXISTS — the 2026-09-07 full-TLS outage.
//
// caddy.service runs as User=caddy. The vhost source folder (/home/server/.caddy)
// is root/server-only, so the `caddy` user CANNOT read the `import` in the main
// Caddyfile. While the panel only ever POSTed adapted JSON to the admin API, the
// running config was correct but existed ONLY IN MEMORY: on a cold start, reboot,
// or crash-restart, systemd re-read the Caddyfile as the caddy user, the import
// resolved to nothing, every tenant vhost vanished, and every tenant site failed
// TLS (CF-proxied → 525, direct → ERR_SSL_PROTOCOL_ERROR).
//
// The host now boots from a COMPILED, caddy-readable /etc/caddy/caddy.json. This
// file's job is to keep that artifact current and authoritative, so the running
// config always equals the boot config. The rule that follows from the outage:
// NEVER push a config to the admin API that was not also persisted to disk.

// compiledFileMode is the mode of the compiled config: root:caddy 0644. It must
// be READABLE BY THE caddy USER — that readability is the entire point.
const compiledFileMode os.FileMode = 0o644

// PersistConfig writes adaptedJSON to path ATOMICALLY: a temp file in the SAME
// directory (so rename() cannot cross a filesystem boundary and degrade to a
// non-atomic copy), fsync'd, then rename()d into place. A reader — including
// systemd starting Caddy at boot — therefore sees either the whole old file or
// the whole new one, never a partial write.
//
// The parent directory is fsync'd too, so the rename itself survives a power
// loss; without it the file contents can be durable while the directory entry
// pointing at them is not.
//
// Ownership is set to root:caddy where the caddy group exists. A failure to
// chown is NOT fatal: 0644 is world-readable, so Caddy can still read it, and
// refusing to publish a valid config over a cosmetic ownership problem would
// trade a working TLS config for a tidy one.
func PersistConfig(path string, adaptedJSON []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("persist: empty compiled-config path")
	}
	if !json.Valid(adaptedJSON) {
		// Belt and braces with the engine's own gate: never let a non-JSON body
		// reach the file systemd boots from.
		return fmt.Errorf("persist: refusing to write %s — adapted output is not valid JSON", path)
	}
	if len(bytes.TrimSpace(adaptedJSON)) < minAdaptedLen {
		return fmt.Errorf("persist: refusing to write %s — adapted config is empty/short (%d bytes)", path, len(adaptedJSON))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persist: mkdir %s: %w", dir, err)
	}

	// Same directory => same filesystem => rename() is atomic.
	tmp, err := os.CreateTemp(dir, ".caddy.json.tmp-*")
	if err != nil {
		return fmt.Errorf("persist: create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Any early return from here on must not leave the temp file behind.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(adaptedJSON); err != nil {
		tmp.Close()
		return fmt.Errorf("persist: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil { // durable before it is visible
		tmp.Close()
		return fmt.Errorf("persist: fsync temp: %w", err)
	}
	if err := tmp.Chmod(compiledFileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("persist: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("persist: close temp: %w", err)
	}
	if gid, ok := caddyGID(); ok {
		// Best-effort: see the doc comment. 0644 already makes it readable.
		_ = os.Chown(tmpName, 0, gid)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("persist: rename into %s: %w", path, err)
	}
	syncDir(dir) // make the rename itself durable
	return nil
}

// caddyGID resolves the "caddy" group. Returns ok=false when the group does not
// exist (a dev box, a container), which callers treat as "skip the chown".
func caddyGID() (int, bool) {
	g, err := user.LookupGroup("caddy")
	if err != nil {
		return 0, false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, false
	}
	return gid, true
}

// syncDir fsyncs a directory so a rename() within it is durable. Best-effort:
// some filesystems refuse to open a directory for sync, and the config is
// already written either way.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}

// ReloadFromFile reloads Caddy FROM THE PERSISTED FILE:
// `caddy reload --config <path>`.
//
// Run as the panel's own (root) process, never as the caddy user. Reloading from
// the same file systemd boots from is what makes the running config and the boot
// config provably identical — reloading from memory is precisely what let them
// diverge and caused the outage.
//
// The file is JSON, so Caddy loads it directly with no adapter and no folder
// read; the privileged adapt already happened in Adapt().
func ReloadFromFile(ctx context.Context, path string, timeout time.Duration) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("reload: empty compiled-config path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("reload: compiled config %s is not readable: %w", path, err)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "caddy", "reload", "--config", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("caddy reload --config %s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// PersistConfig / ReloadFromFile on the Client, so a Reloader can satisfy
// reconcile.ConfigPersister without the engine importing os/exec itself.

// PersistConfig implements reconcile.ConfigPersister.
func (c *Client) PersistConfig(path string, adaptedJSON []byte) error {
	return PersistConfig(path, adaptedJSON)
}

// ReloadFromFile implements reconcile.ConfigPersister.
func (c *Client) ReloadFromFile(ctx context.Context, path string) error {
	return ReloadFromFile(ctx, path, 0)
}
