package origin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var four = []string{"52.76.29.0", "52.76.123.15", "3.1.252.222", "52.77.202.62"}

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestConfigured_DefaultIsAllFourOrigins(t *testing.T) {
	ips, src := Configured(env(nil))
	if src != "default" {
		t.Errorf("source = %q, want default", src)
	}
	have := map[string]bool{}
	for _, ip := range ips {
		have[ip] = true
	}
	for _, ip := range four {
		if !have[ip] {
			t.Errorf("default set is missing %s", ip)
		}
	}
	if len(ips) != 4 {
		t.Errorf("default set has %d IPs, want 4: %v", len(ips), ips)
	}
}

func TestConfigured_OldEnvNamesStillHonoured(t *testing.T) {
	for _, key := range EnvKeys {
		ips, src := Configured(env(map[string]string{key: " 1.2.3.4 , 5.6.7.8"}))
		if src != key || len(ips) != 2 || ips[0] != "1.2.3.4" || ips[1] != "5.6.7.8" {
			t.Errorf("%s: got %v from %q", key, ips, src)
		}
	}
}

func TestConfigured_EnvValuesAreUnionedAndGarbageDropped(t *testing.T) {
	ips, src := Configured(env(map[string]string{
		"CADDY_HEALTH_SERVER_IPS": "52.76.123.15,52.77.202.62",
		"CUTOVER_ORIGIN_IPS":      "52.76.29.0,not-an-ip,52.76.123.15",
	}))
	if src != "CADDY_HEALTH_SERVER_IPS+CUTOVER_ORIGIN_IPS" {
		t.Errorf("source = %q", src)
	}
	if got, want := strings.Join(ips, ","), "52.76.123.15,52.76.29.0,52.77.202.62"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestConfigured_AllGarbageFallsBackToDefault(t *testing.T) {
	ips, src := Configured(env(map[string]string{"PANEL_ORIGIN_IPS": "nope, ,also-nope"}))
	if src != "default" || len(ips) != 4 {
		t.Errorf("got %v from %q, want the 4 defaults", ips, src)
	}
}

// The Edge classifier must put every one of the four origins in Origin, including
// ppt2 (52.77.202.62), which the old CUTOVER_ORIGIN_IPS default missed.
func TestClassifyEdge_AllFourAreOrigin(t *testing.T) {
	ours := NewSet(DefaultIPs, "default").IPs()
	for _, ip := range four {
		if edge, _ := ClassifyEdge([]string{ip}, nil, ours); edge != EdgeOrigin {
			t.Errorf("%s classified %q, want Origin", ip, edge)
		}
	}
}

func TestClassifyEdge_OtherClasses(t *testing.T) {
	ours := NewSet(DefaultIPs, "default").IPs()
	cases := []struct {
		name  string
		addrs []string
		err   error
		want  string
	}{
		{"cloudflare", []string{"104.21.69.82", "172.67.206.132"}, nil, EdgeCloudflare},
		{"cloudflare wins over origin", []string{"104.21.69.82", "52.77.202.62"}, nil, EdgeCloudflare},
		{"elsewhere", []string{"8.8.8.8"}, nil, EdgeElsewhere},
		{"origin among others", []string{"8.8.8.8", "52.77.202.62"}, nil, EdgeOrigin},
		{"v6 only", []string{"2606:4700::6810:4552"}, nil, EdgeElsewhere},
		{"nxdomain", nil, errors.New("no such host"), EdgeNXDomain},
		{"empty answer", nil, nil, EdgeNXDomain},
	}
	for _, c := range cases {
		if got, _ := ClassifyEdge(c.addrs, c.err, ours); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIsCloudflareIP_OriginsAreNot(t *testing.T) {
	for _, ip := range four {
		if IsCloudflareIP(ip) {
			t.Errorf("%s must not be Cloudflare", ip)
		}
	}
	if !IsCloudflareIP("104.21.69.82") || !IsCloudflareIP("172.67.206.132") {
		t.Error("the panel zone's Cloudflare pair must classify Cloudflare")
	}
}

type fakeDiscoverer struct {
	ips []string
	err error
}

func (f fakeDiscoverer) PublicIPv4s(context.Context) ([]string, error) { return f.ips, f.err }

// Metadata only knows THIS instance's addresses. An answer that lacks some
// configured origins (they may be on another box sharing storage) must not drop
// them: the result is a union.
func TestSet_DiscoveryIsUnionNeverReplacement(t *testing.T) {
	s := NewSet(DefaultIPs, "default")
	if err := s.Discover(context.Background(), fakeDiscoverer{ips: []string{"52.76.123.15", "13.250.1.1"}}); err != nil {
		t.Fatal(err)
	}
	got := s.IPs()
	for _, ip := range append(four, "13.250.1.1") {
		if !got[ip] {
			t.Errorf("union is missing %s: %v", ip, s.List())
		}
	}
	if len(got) != 5 {
		t.Errorf("union has %d IPs, want 5: %v", len(got), s.List())
	}

	// A failed refresh keeps the previous discovery; an empty successful one drops
	// only the discovered part. Configured addresses survive both.
	_ = s.Discover(context.Background(), fakeDiscoverer{err: errors.New("imds down")})
	if !s.IPs()["13.250.1.1"] {
		t.Error("a failed refresh must keep the previous discovered addresses")
	}
	_ = s.Discover(context.Background(), fakeDiscoverer{ips: nil})
	got = s.IPs()
	if got["13.250.1.1"] || len(got) != 4 {
		t.Errorf("after an empty discovery want exactly the 4 configured, got %v", s.List())
	}
}

func TestIMDS_PublicIPv4sUsesV2TokenAndWalksMACs(t *testing.T) {
	const token = "tok-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == imdsTokenPath {
			if r.Method != http.MethodPut || r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(token))
			return
		}
		if r.Header.Get("X-aws-ec2-metadata-token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case imdsMACsPath:
			_, _ = w.Write([]byte("0a:1b:2c:3d:4e:5f/\n0a:1b:2c:3d:4e:60/\n"))
		case imdsMACsPath + "0a:1b:2c:3d:4e:5f/public-ipv4s":
			_, _ = w.Write([]byte("52.76.123.15\n52.77.202.62"))
		default: // second ENI has no public address
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := NewIMDS(time.Second)
	m.BaseURL = srv.URL
	ips, err := m.PublicIPv4s(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ips, ",") != "52.76.123.15,52.77.202.62" {
		t.Errorf("got %v", ips)
	}
}

func TestIMDS_UnreachableFailsFastAndSetStands(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listening: connection refused
	m := NewIMDS(300 * time.Millisecond)
	m.BaseURL = srv.URL
	s := NewSet(DefaultIPs, "default")
	start := time.Now()
	if err := s.Discover(context.Background(), m); err == nil {
		t.Error("want an error from an unreachable IMDS")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("discovery took %s; it must fail fast", time.Since(start))
	}
	if len(s.IPs()) != 4 {
		t.Errorf("configured set must stand when IMDS is unreachable, got %v", s.List())
	}
}
