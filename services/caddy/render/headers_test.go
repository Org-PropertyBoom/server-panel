package render

import "testing"

func TestValidHeaderName(t *testing.T) {
	for _, ok := range []string{"X-Robots-Tag", "Cache-Control", "X-Frame-Options"} {
		if !ValidHeaderName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "X Robots", "X:Tag", "X_Tag", "bad\nname", "X;rm"} {
		if ValidHeaderName(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestValidHeaderValue(t *testing.T) {
	if !ValidHeaderValue("noindex") || !ValidHeaderValue("public, max-age=3600") {
		t.Error("plain values should be valid")
	}
	// Anything that could break the quoted directive or escape the block.
	for _, bad := range []string{"", "a\"b", "a}b", "a\nb", "a\rb", "a\\b", "a\x00b", "a\x7fb"} {
		if ValidHeaderValue(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestHeaderDirectives_SortedAndQuoted(t *testing.T) {
	got := HeaderDirectives(map[string]string{"X-Robots-Tag": "noindex", "Cache-Control": "no-store"})
	want := "    header Cache-Control \"no-store\"\n    header X-Robots-Tag \"noindex\"\n"
	if got != want {
		t.Errorf("HeaderDirectives =\n%q\nwant\n%q", got, want)
	}
	if HeaderDirectives(nil) != "" || HeaderDirectives(map[string]string{}) != "" {
		t.Error("empty headers must render nothing")
	}
	// A bad entry is skipped, not emitted.
	if out := HeaderDirectives(map[string]string{"Bad Name": "x", "X-Ok": "y"}); out != "    header X-Ok \"y\"\n" {
		t.Errorf("invalid name must be skipped; got %q", out)
	}
}
