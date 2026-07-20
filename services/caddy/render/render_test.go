package render

import "testing"

// The golden snippet bytes below are copied from property-team
// CaddyVhostsService (vhostSnippet / systemSnippet / redirectSnippet). The engine
// MUST reproduce them exactly — a drift here means switching the writer would
// rewrite every file and churn the folder.

func TestRender_Tenant(t *testing.T) {
	name, body, err := Render(Host{Host: "Example.COM", Kind: KindTenant, Target: "127.0.0.1:8005"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "example.com.caddy" {
		t.Errorf("name = %q", name)
	}
	want := "example.com {\n    reverse_proxy 127.0.0.1:8005\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_System(t *testing.T) {
	name, body, err := Render(Host{Host: "app.propertyboom.co", Kind: KindSystem, Target: "127.0.0.1:8002"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "app.propertyboom.co.caddy" {
		t.Errorf("name = %q", name)
	}
	want := "app.propertyboom.co {\n    reverse_proxy 127.0.0.1:8002\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_Redirect(t *testing.T) {
	name, body, err := Render(Host{Host: "old.example.com", Kind: KindRedirect, Target: "https://new.example.com", RedirectCode: 302})
	if err != nil {
		t.Fatal(err)
	}
	if name != "old.example.com.caddy" {
		t.Errorf("name = %q", name)
	}
	want := "old.example.com {\n    redir https://new.example.com 302\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_RedirectDefaultsTo301(t *testing.T) {
	_, body, err := Render(Host{Host: "a.com", Kind: KindRedirect, Target: "https://b.com", RedirectCode: 0})
	if err != nil {
		t.Fatal(err)
	}
	want := "a.com {\n    redir https://b.com 301\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestFileName_Wildcard(t *testing.T) {
	cases := map[string]string{
		"example.com":   "example.com.caddy",
		"*.example.com": "wildcard_example.com.caddy",
		"*.PROPERTY.io": "wildcard_property.io.caddy",
		"  Sub.Dom.CO ": "sub.dom.co.caddy",
	}
	for in, want := range cases {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRender_WildcardTenant(t *testing.T) {
	name, body, err := Render(Host{Host: "*.example.com", Kind: KindTenant, Target: "127.0.0.1:8002"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "wildcard_example.com.caddy" {
		t.Errorf("name = %q", name)
	}
	// The snippet body keeps the real wildcard host; only the FILENAME is escaped.
	want := "*.example.com {\n    reverse_proxy 127.0.0.1:8002\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_TenantWithEncode(t *testing.T) {
	_, body, err := Render(Host{Host: "example.com", Kind: KindTenant, Target: "127.0.0.1:8005", Encode: "zstd gzip"})
	if err != nil {
		t.Fatal(err)
	}
	want := "example.com {\n    encode zstd gzip\n    reverse_proxy 127.0.0.1:8005\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_SystemWithEncode(t *testing.T) {
	_, body, err := Render(Host{Host: "sys.com", Kind: KindSystem, Target: "127.0.0.1:8002", Encode: "gzip"})
	if err != nil {
		t.Fatal(err)
	}
	want := "sys.com {\n    encode gzip\n    reverse_proxy 127.0.0.1:8002\n}\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestRender_EmptyEncodeIsByteIdentical(t *testing.T) {
	// Encode="" must reproduce the original (migration-era) bytes exactly.
	_, body, err := Render(Host{Host: "a.com", Kind: KindTenant, Target: "127.0.0.1:8005", Encode: ""})
	if err != nil {
		t.Fatal(err)
	}
	if body != "a.com {\n    reverse_proxy 127.0.0.1:8005\n}\n" {
		t.Errorf("empty encode changed the bytes: %q", body)
	}
}

func TestRender_RedirectIgnoresEncode(t *testing.T) {
	// A redirect has no proxied body — encode must never appear on it.
	_, body, err := Render(Host{Host: "old.com", Kind: KindRedirect, Target: "https://new.com", RedirectCode: 301, Encode: "zstd gzip"})
	if err != nil {
		t.Fatal(err)
	}
	if body != "old.com {\n    redir https://new.com 301\n}\n" {
		t.Errorf("redirect must ignore encode; got %q", body)
	}
}

func TestRender_TenantWithHeaderBlockAndEncode(t *testing.T) {
	hb := "    header {\n        X-Content-Type-Options nosniff\n    }\n"
	_, body, err := Render(Host{Host: "example.com", Kind: KindTenant, Target: "127.0.0.1:8005", Encode: "zstd gzip", HeaderBlock: hb})
	if err != nil {
		t.Fatal(err)
	}
	want := "example.com {\n" +
		"    header {\n        X-Content-Type-Options nosniff\n    }\n" +
		"    encode zstd gzip\n" +
		"    reverse_proxy 127.0.0.1:8005\n}\n"
	if body != want {
		t.Errorf("body =\n%q\nwant\n%q", body, want)
	}
}

func TestRender_HeaderBlockNoEncode(t *testing.T) {
	hb := "    header {\n        X-Frame-Options SAMEORIGIN\n    }\n"
	_, body, err := Render(Host{Host: "a.com", Kind: KindTenant, Target: "127.0.0.1:8002", HeaderBlock: hb})
	if err != nil {
		t.Fatal(err)
	}
	want := "a.com {\n    header {\n        X-Frame-Options SAMEORIGIN\n    }\n    reverse_proxy 127.0.0.1:8002\n}\n"
	if body != want {
		t.Errorf("body =\n%q\nwant\n%q", body, want)
	}
}

func TestRender_EmptyHeaderBlockByteIdentical(t *testing.T) {
	_, body, err := Render(Host{Host: "a.com", Kind: KindTenant, Target: "127.0.0.1:8005"})
	if err != nil {
		t.Fatal(err)
	}
	if body != "a.com {\n    reverse_proxy 127.0.0.1:8005\n}\n" {
		t.Errorf("empty header/encode must stay byte-identical: %q", body)
	}
}

func TestRender_Errors(t *testing.T) {
	if _, _, err := Render(Host{Host: "", Kind: KindTenant, Target: "x"}); err == nil {
		t.Error("empty host should error")
	}
	if _, _, err := Render(Host{Host: "a.com", Kind: KindTenant, Target: ""}); err == nil {
		t.Error("empty tenant upstream should error")
	}
	if _, _, err := Render(Host{Host: "a.com", Kind: KindRedirect, Target: ""}); err == nil {
		t.Error("empty redirect target should error")
	}
}
