package services

import "testing"

func TestIsCloudflareIP(t *testing.T) {
	// The documented cutover A records must classify as Cloudflare — this is what
	// the Edge column keys on for "how many of the 88 are left?".
	for _, ip := range []string{"104.21.69.82", "172.67.206.132", "104.16.0.1", "162.159.1.1"} {
		if !isCloudflareIP(ip) {
			t.Errorf("%s should be classified Cloudflare", ip)
		}
	}
	// Our AWS origins must NOT be mistaken for Cloudflare.
	for _, ip := range []string{"52.76.29.0", "52.76.123.15", "3.1.252.222", "8.8.8.8"} {
		if isCloudflareIP(ip) {
			t.Errorf("%s must not be classified Cloudflare", ip)
		}
	}
	if isCloudflareIP("not-an-ip") {
		t.Error("garbage input must not classify as Cloudflare")
	}
}

func TestOriginIPsDefault(t *testing.T) {
	ours := originIPs()
	for _, ip := range []string{"52.76.29.0", "52.76.123.15", "3.1.252.222"} {
		if !ours[ip] {
			t.Errorf("%s should be a known origin IP", ip)
		}
	}
}

func TestPreflightLabelsCoverSpec(t *testing.T) {
	// The spec's fixed probe list — DNS can't be enumerated, so a missing label is
	// a record we'd silently never check before a nameserver switch.
	want := []struct{ label, typ string }{
		{"@", "MX"}, {"@", "TXT"}, {"_dmarc", "TXT"},
		{"google._domainkey", "TXT"}, {"default._domainkey", "TXT"},
		{"selector1._domainkey", "TXT"}, {"selector2._domainkey", "TXT"},
		{"resend._domainkey", "TXT"}, {"k1._domainkey", "TXT"},
		{"mail", "A"}, {"smtp", "A"}, {"imap", "A"}, {"pop", "A"},
		{"webmail", "A"}, {"autodiscover", "A"}, {"autoconfig", "A"},
		{"www", "A"}, {"ftp", "A"}, {"cpanel", "A"}, {"blog", "A"}, {"shop", "A"}, {"m", "A"},
		{"_cf-custom-hostname", "TXT"}, {"_acme-challenge", "TXT"},
		{"@", "NS"}, {"@", "DS"},
	}
	have := map[string]bool{}
	for _, p := range preflightLabels {
		have[p.label+"/"+p.typ] = true
	}
	for _, w := range want {
		if !have[w.label+"/"+w.typ] {
			t.Errorf("pre-flight scan is missing %s %s", w.label, w.typ)
		}
	}
}

func TestFqdn(t *testing.T) {
	if got := fqdn("@", "example.com"); got != "example.com" {
		t.Errorf("apex = %q, want example.com", got)
	}
	if got := fqdn("_dmarc", "example.com"); got != "_dmarc.example.com" {
		t.Errorf("label = %q, want _dmarc.example.com", got)
	}
}

func TestSameAnswers(t *testing.T) {
	if !sameAnswers([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("identical answers should match")
	}
	if sameAnswers([]string{"a"}, []string{"a", "b"}) {
		t.Error("a dropped record must NOT match — that's the whole point of the gate")
	}
	if sameAnswers([]string{"a"}, []string{"b"}) {
		t.Error("altered record must not match")
	}
	if !sameAnswers(nil, nil) {
		t.Error("both-absent should match")
	}
}
