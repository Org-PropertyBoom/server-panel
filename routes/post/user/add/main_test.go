package add

import (
	"strings"
	"testing"
)

func TestGenerateUsername(t *testing.T) {
	username, err := generateUsername()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(username, "user-") || len(username) != len("user-")+8 {
		t.Fatalf("generated username = %q", username)
	}
	if !usernamePattern.MatchString(username) {
		t.Fatalf("generated username is invalid: %q", username)
	}
}

func TestUsernamePattern(t *testing.T) {
	for _, username := range []string{"alice", "web-user", "app_user2"} {
		if !usernamePattern.MatchString(username) {
			t.Errorf("expected %q to be valid", username)
		}
	}
	for _, username := range []string{"", "Alice", "2user", "bad user", strings.Repeat("a", 33)} {
		if usernamePattern.MatchString(username) {
			t.Errorf("expected %q to be invalid", username)
		}
	}
}
