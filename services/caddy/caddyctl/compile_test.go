package caddyctl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const validCfg = `{"apps":{"http":{"servers":{"srv0":{"listen":[":443"],"routes":[]}}}}}`

func TestPersistConfig_WritesFileAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "caddy.json")

	if err := PersistConfig(path, []byte(validCfg)); err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != validCfg {
		t.Errorf("content = %q, want %q", got, validCfg)
	}

	// The temp file must be renamed, never left behind — a stray .caddy.json.tmp-*
	// in /etc/caddy is confusing at best and bootable garbage at worst.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file in dir, got %d", len(entries))
	}
}

func TestPersistConfig_IsReadableByOtherUsers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "caddy.json")
	if err := PersistConfig(path, []byte(validCfg)); err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// 0644. The whole point of this file is that the `caddy` user can read it —
	// a mode that excludes it recreates the outage this change exists to fix.
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("mode = %o, want 644 (must be readable by the caddy user)", perm)
	}
}

func TestPersistConfig_ReplacesExistingCompletely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "caddy.json")
	long := `{"apps":{"http":{"servers":{"srv0":{"listen":[":443"],"routes":[],"pad":"` + strings.Repeat("x", 400) + `"}}}}}`
	if err := PersistConfig(path, []byte(long)); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := PersistConfig(path, []byte(validCfg)); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, _ := os.ReadFile(path)
	// rename() replaces the inode wholesale; a naive in-place write would leave
	// the tail of the longer previous config behind and produce invalid JSON.
	if string(got) != validCfg {
		t.Errorf("content = %q (len %d), want the shorter config exactly", got, len(got))
	}
}

func TestPersistConfig_RejectsInvalidAndShort(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
	}{
		{"not json", strings.Repeat("z", 200)},
		{"truncated json", `{"apps":{"http":{"servers":{"srv0":`},
		{"valid but too short", `{"a":1}`},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := PersistConfig(path, []byte(tc.body)); err == nil {
				t.Fatal("expected an error, got nil")
			}
			// Abort must mean NOTHING was written — not a truncated file systemd
			// would later try to boot from.
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("file was created despite the error: %v", err)
			}
		})
	}
}

func TestPersistConfig_RejectsEmptyPath(t *testing.T) {
	if err := PersistConfig("   ", []byte(validCfg)); err == nil {
		t.Error("expected an error for an empty path, got nil")
	}
}
