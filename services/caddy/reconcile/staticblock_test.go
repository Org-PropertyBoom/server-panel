package reconcile

import (
	"strings"
	"testing"
)

const sampleCaddyfile = `{
	admin localhost:2019
}

app.propertyboom.co {
	reverse_proxy 127.0.0.1:8002
}

caddydash.propertyweb.co {
	reverse_proxy 127.0.0.1:8090
}

cp.propertyweb.co {
	reverse_proxy 127.0.0.1:2205
}

import /home/server/.caddy/*
`

func TestRemoveCaddyBlock_RemovesOnlyTarget(t *testing.T) {
	out, ok := removeCaddyBlock([]byte(sampleCaddyfile), "caddydash.propertyweb.co")
	if !ok {
		t.Fatal("expected the block to be found + removed")
	}
	s := string(out)
	if strings.Contains(s, "caddydash.propertyweb.co") || strings.Contains(s, "127.0.0.1:8090") {
		t.Errorf("caddydash block still present:\n%s", s)
	}
	// Everything else must survive.
	for _, keep := range []string{"app.propertyboom.co", "cp.propertyweb.co", "import /home/server/.caddy/*", "admin localhost:2019"} {
		if !strings.Contains(s, keep) {
			t.Errorf("removed too much — %q missing:\n%s", keep, s)
		}
	}
}

func TestRemoveCaddyBlock_NotFound(t *testing.T) {
	if _, ok := removeCaddyBlock([]byte(sampleCaddyfile), "nope.example.com"); ok {
		t.Error("should report not-found for an absent host")
	}
}

func TestBlockMatchesHost_ExactTokenOnly(t *testing.T) {
	if blockMatchesHost("myapp.propertyboom.co", "app.propertyboom.co") {
		t.Error("must not match a superstring host")
	}
	if !blockMatchesHost("https://app.propertyboom.co:443, www.app.propertyboom.co", "app.propertyboom.co") {
		t.Error("must match one of several comma-separated addresses (scheme/port stripped)")
	}
}

func TestAssertOnlyTargetDropped(t *testing.T) {
	before := map[string]bool{"a.com": true, "b.com": true, "cp.propertyweb.co": true, "app.propertyboom.co": true}
	protected := []string{"cp.propertyweb.co", "app.propertyboom.co"}

	// Exactly the target dropped → OK.
	after := map[string]bool{"a.com": true, "cp.propertyweb.co": true, "app.propertyboom.co": true}
	if _, err := assertOnlyTargetDropped(before, after, "b.com", protected); err != nil {
		t.Errorf("clean single-drop should pass: %v", err)
	}
	// Another host also dropped → refuse.
	after2 := map[string]bool{"cp.propertyweb.co": true, "app.propertyboom.co": true}
	if dropped, err := assertOnlyTargetDropped(before, after2, "b.com", protected); err == nil || len(dropped) != 1 || dropped[0] != "a.com" {
		t.Errorf("must refuse when another host also drops; dropped=%v err=%v", dropped, err)
	}
	// A protected domain dropped → refuse.
	after3 := map[string]bool{"a.com": true, "app.propertyboom.co": true}
	if _, err := assertOnlyTargetDropped(before, after3, "b.com", protected); err == nil {
		t.Error("must refuse when a protected domain drops")
	}
	// Target still present (not removed) → refuse.
	if _, err := assertOnlyTargetDropped(before, before, "b.com", protected); err == nil {
		t.Error("must refuse when the target is still present")
	}
	// A host added → refuse.
	after4 := map[string]bool{"a.com": true, "b.com": true, "new.com": true, "cp.propertyweb.co": true, "app.propertyboom.co": true}
	if _, err := assertOnlyTargetDropped(before, after4, "b.com", protected); err == nil {
		t.Error("must refuse when a host is added")
	}
}
