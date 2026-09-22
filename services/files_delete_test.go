package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// DeleteFile now removes directories recursively (os.RemoveAll), gated by
// isCriticalDir so a recursive delete can never take out the OS or a whole home.

func TestDeleteFile_RemovesRegularFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFile(f, dir, true); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
}

func TestDeleteFile_RemovesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "empty")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFile(sub, dir, true); err != nil {
		t.Fatalf("delete empty dir: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatalf("dir still present: %v", err)
	}
}

func TestDeleteFile_RemovesNonEmptyDirRecursively(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nocodb", "data", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "dump.rdb"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "nocodb")
	if err := DeleteFile(target, dir, true); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dir tree still present: %v", err)
	}
}

func TestDeleteFile_RefusesCriticalAndAncestorDirs(t *testing.T) {
	// DeleteFile stats the path before the guard, so the paths under test must
	// actually exist. Build a real home tree under a temp dir and verify the home
	// itself and its ancestors are refused; use "/" for the always-present root.
	base := t.TempDir()
	home := filepath.Join(base, "home", "server", "htdocs")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	refused := []string{
		home,                                  // the home itself
		filepath.Join(base, "home", "server"), // ancestor of home
		filepath.Join(base, "home"),           // ancestor of home
		"/",                                   // filesystem root (in criticalDirs)
	}
	for _, p := range refused {
		if err := DeleteFile(p, home, true); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("DeleteFile(%q) = %v, want ErrProtectedPath", p, err)
		}
	}
	// The home tree must survive every refused attempt.
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("home was damaged by a refused delete: %v", err)
	}
}

func TestIsCriticalDir(t *testing.T) {
	home := "/home/server/htdocs"
	protected := []string{"/", "/etc", "/usr", "/var", "/home", "/root", "/home/server", home}
	for _, p := range protected {
		if !isCriticalDir(p, home) {
			t.Errorf("isCriticalDir(%q) = false, want true", p)
		}
	}
	allowed := []string{"/home/server/htdocs/nocodb", "/home/server/htdocs/redis/data", "/opt/app/logs"}
	for _, p := range allowed {
		if isCriticalDir(p, home) {
			t.Errorf("isCriticalDir(%q) = true, want false (should be deletable)", p)
		}
	}
}
