package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionUserHomeCreatesDefaultDirectories(t *testing.T) {
	home := filepath.Join(t.TempDir(), "user-test")
	if err := ProvisionUserHome(home, os.Getuid(), os.Getgid()); err != nil {
		t.Fatalf("ProvisionUserHome() error = %v", err)
	}

	for _, name := range DefaultUserDirectories {
		info, err := os.Stat(filepath.Join(home, name))
		if err != nil {
			t.Fatalf("expected %s directory: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", name)
		}
	}
}
