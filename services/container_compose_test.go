package services

import (
	"reflect"
	"testing"
)

func TestParseComposeServices(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "single service",
			in: `services:
  nocodb:
    image: nocodb/nocodb:latest
    ports:
      - "8080:8080"
`,
			want: []string{"nocodb"},
		},
		{
			name: "multiple services with nested blocks and trailing top-level keys",
			in: `version: "3.8"
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
  cadvisor:
    image: gcr.io/cadvisor/cadvisor
  node-exporter:
    image: prom/node-exporter
  grafana:
    image: grafana/grafana
volumes:
  grafana-data:
networks:
  monitoring:
`,
			want: []string{"prometheus", "cadvisor", "node-exporter", "grafana"},
		},
		{
			name: "four-space indent",
			in: `services:
    openinary:
        image: openinary
    minio:
        image: minio/minio
`,
			want: []string{"openinary", "minio"},
		},
		{
			name: "no services block",
			in: `version: "3"
volumes:
  data:
`,
			want: nil,
		},
		{
			name: "comments and blank lines ignored",
			in: `# my stack
services:
  # the app
  app:
    image: app

  worker:
    image: app
`,
			want: []string{"app", "worker"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseComposeServices([]byte(tc.in))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseComposeServices() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComposeLabel(t *testing.T) {
	labels := "com.docker.compose.project=nocodb,com.docker.compose.service=nocodb,com.docker.compose.project.working_dir=/home/server/htdocs/nocodb"
	if got := composeLabel(labels, "com.docker.compose.service"); got != "nocodb" {
		t.Errorf("service label = %q, want nocodb", got)
	}
	if got := composeLabel(labels, "com.docker.compose.project.working_dir"); got != "/home/server/htdocs/nocodb" {
		t.Errorf("working_dir label = %q, want /home/server/htdocs/nocodb", got)
	}
	if got := composeLabel(labels, "missing.key"); got != "" {
		t.Errorf("missing label = %q, want empty", got)
	}
	if got := composeLabel("", "com.docker.compose.project"); got != "" {
		t.Errorf("empty labels = %q, want empty", got)
	}
}
