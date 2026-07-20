package services

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseNginxVHosts(t *testing.T) {
	output := `# configuration file /etc/nginx/sites-enabled/example.conf:
server {
    listen 80;
    listen 443 ssl;
    server_name example.com www.example.com;
    root /srv/example/public;
    location /api {
        proxy_pass http://127.0.0.1:3000;
    }
}
`
	hosts := parseNginxVHosts(output)
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1", len(hosts))
	}
	host := hosts[0]
	if host.Hostname != "example.com" || !reflect.DeepEqual(host.Aliases, []string{"www.example.com"}) {
		t.Fatalf("unexpected names: %#v", host)
	}
	if !host.TLS || !reflect.DeepEqual(host.Upstreams, []string{"http://127.0.0.1:3000"}) {
		t.Fatalf("unexpected route details: %#v", host)
	}
	if !reflect.DeepEqual(host.ConfigFiles, []string{"/etc/nginx/sites-enabled/example.conf"}) {
		t.Fatalf("unexpected config files: %#v", host.ConfigFiles)
	}
}

func TestParseCaddyVHosts(t *testing.T) {
	config := map[string]any{"apps": map[string]any{"http": map[string]any{"servers": map[string]any{
		"srv0": map[string]any{
			"listen": []any{":443"},
			"routes": []any{map[string]any{
				"match":  []any{map[string]any{"host": []any{"example.com", "www.example.com"}}},
				"handle": []any{map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": "127.0.0.1:8080"}}}},
			}},
		},
	}}}}
	output, _ := json.Marshal(config)
	hosts := parseCaddyVHosts(output, "/etc/caddy/Caddyfile")
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1", len(hosts))
	}
	if hosts[0].Hostname != "example.com" || !hosts[0].TLS {
		t.Fatalf("unexpected host: %#v", hosts[0])
	}
	if !reflect.DeepEqual(hosts[0].Upstreams, []string{"127.0.0.1:8080"}) {
		t.Fatalf("unexpected upstreams: %#v", hosts[0].Upstreams)
	}
}

func TestParseApacheVHosts(t *testing.T) {
	output := `*:80 is a NameVirtualHost
         port 80 namevhost example.com (/etc/apache2/sites-enabled/example.conf:1)
         port 443 namevhost secure.example.com (/etc/apache2/sites-enabled/secure.conf:2)`
	hosts := parseApacheVHosts(output)
	if len(hosts) != 2 {
		t.Fatalf("got %d hosts, want 2", len(hosts))
	}
	if hosts[0].TLS || !hosts[1].TLS {
		t.Fatalf("unexpected TLS values: %#v", hosts)
	}
}

func TestServerForPort(t *testing.T) {
	output := `LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:(("nginx",pid=12,fd=6))
LISTEN 0 4096 *:443 *:* users:(("caddy",pid=8,fd=7))`
	if server, listening := serverForPort(output, 80); server != "nginx" || !listening {
		t.Fatalf("port 80 = %q, %v", server, listening)
	}
	if server, listening := serverForPort(output, 443); server != "caddy" || !listening {
		t.Fatalf("port 443 = %q, %v", server, listening)
	}
}
