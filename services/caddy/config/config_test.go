package config

import (
	"strings"
	"testing"
)

func TestProtectedHostsIsPanelDomainOnly(t *testing.T) {
	// The panel's own domain is the SOLE protected host; stack dashboard domains
	// (app./go-app./la-app./rust-app.propertyboom.co) are ordinary managed routes.
	got := Config{PanelDomain: "cp.propertyweb.co"}.ProtectedHosts()
	if len(got) != 1 || got[0] != "cp.propertyweb.co" {
		t.Fatalf("ProtectedHosts = %v, want [cp.propertyweb.co]", got)
	}
}

func TestProtectedHostsLowercasesAndSkipsEmpty(t *testing.T) {
	if got := (Config{PanelDomain: "  CP.propertyweb.co "}).ProtectedHosts(); len(got) != 1 || got[0] != "cp.propertyweb.co" {
		t.Fatalf("ProtectedHosts = %v, want [cp.propertyweb.co]", got)
	}
	if got := (Config{PanelDomain: ""}).ProtectedHosts(); len(got) != 0 {
		t.Fatalf("empty panel domain → no protected hosts, got %v", got)
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
