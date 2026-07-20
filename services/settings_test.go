package services

import (
	"path/filepath"
	"testing"
)

func TestSettingsServiceCreatesAndUpdatesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ppt-server-panel", "data", "db.sqlite")
	t.Setenv(settingsDBEnv, path)

	service, err := NewSettingsService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.db.Close()

	if err := service.Set("apps_header", `["nginx"]`); err != nil {
		t.Fatal(err)
	}
	if err := service.Set("apps_header", `["nginx","docker"]`); err != nil {
		t.Fatal(err)
	}
	settings, err := service.All()
	if err != nil {
		t.Fatal(err)
	}
	if got := settings["apps_header"]; got != `["nginx","docker"]` {
		t.Fatalf("apps_header = %q", got)
	}
}

func TestSettingsDBPathUsesUserHome(t *testing.T) {
	t.Setenv(settingsDBEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".ppt-server-panel", "data", "db.sqlite")
	if got := settingsDBPath(); got != want {
		t.Fatalf("settingsDBPath() = %q, want %q", got, want)
	}
}
