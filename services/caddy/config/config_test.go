package config

import (
	"strings"
	"testing"
)

func TestProtectedHostsIncludesPanelDomain(t *testing.T) {
	c := Config{DashboardDomain: "app.propertyboom.co", PanelDomain: "cp.propertyweb.co"}
	got := c.ProtectedHosts()
	want := map[string]bool{"app.propertyboom.co": true, "cp.propertyweb.co": true}
	if len(got) != 2 {
		t.Fatalf("ProtectedHosts = %v, want 2 entries", got)
	}
	for _, h := range got {
		if !want[h] {
			t.Errorf("unexpected protected host %q", h)
		}
	}
}

func TestProtectedHostsLowercasesAndSkipsEmpty(t *testing.T) {
	c := Config{DashboardDomain: "  APP.propertyboom.co ", PanelDomain: ""}
	got := c.ProtectedHosts()
	if len(got) != 1 || got[0] != "app.propertyboom.co" {
		t.Fatalf("ProtectedHosts = %v, want [app.propertyboom.co]", got)
	}
}

func TestUpstreamFor(t *testing.T) {
	c := defaults()
	if up, ok := c.UpstreamFor("golang"); !ok || up != "127.0.0.1:8005" {
		t.Errorf("golang -> %q,%v", up, ok)
	}
	if up, ok := c.UpstreamFor(" LARAVEL "); !ok || up != "127.0.0.1:8004" {
		t.Errorf("laravel (padded/cased) -> %q,%v", up, ok)
	}
	if _, ok := c.UpstreamFor("oracle"); ok {
		t.Error("unknown stack must not resolve (never guess a port)")
	}
}

func TestEncodeFormatsNormalizes(t *testing.T) {
	if got := (Config{Encode: "  zstd   gzip "}).EncodeFormats(); got != "zstd gzip" {
		t.Errorf("EncodeFormats = %q", got)
	}
	if got := (Config{Encode: ""}).EncodeFormats(); got != "" {
		t.Errorf("empty encode = %q, want empty", got)
	}
}

func TestSecurityHeaderBlock(t *testing.T) {
	var off *SecurityHeaders
	if off.HeaderBlock() != "" {
		t.Error("nil SecurityHeaders must render no block")
	}
	baseline := &SecurityHeaders{Baseline: true}
	block := baseline.HeaderBlock()
	if !strings.HasPrefix(block, "    header {\n") || !strings.HasSuffix(block, "    }\n") {
		t.Errorf("baseline block malformed:\n%q", block)
	}
	if !strings.Contains(block, "X-Content-Type-Options nosniff") {
		t.Errorf("baseline missing nosniff:\n%q", block)
	}
}

func TestSecurityHeadersRejectPreload(t *testing.T) {
	if err := (&SecurityHeaders{HSTS: "max-age=1; preload"}).Validate(); err == nil {
		t.Error("HSTS preload must be rejected")
	}
	if err := (&SecurityHeaders{HSTS: "max-age=1"}).Validate(); err != nil {
		t.Errorf("plain HSTS should be allowed: %v", err)
	}
}
