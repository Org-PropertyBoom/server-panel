package services

import (
	"path/filepath"
	"testing"
)

func TestSessionPersistsToDisk(t *testing.T) {
	t.Setenv("SESSION_PATH", filepath.Join(t.TempDir(), "session"))

	first := NewSessionService()
	session, err := first.Create(AuthenticatedUser{
		UID:      0,
		Username: "root",
	}, "root")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	second := NewSessionService()
	loaded, ok := second.Get(session.Token)
	if !ok {
		t.Fatal("expected persisted session to load")
	}
	if loaded.Username != "root" || loaded.Mode != "root" {
		t.Fatalf("unexpected session: %+v", loaded)
	}
}
