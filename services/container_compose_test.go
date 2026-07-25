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

func TestParseBindMounts(t *testing.T) {
	// A running container (go-actions) bind-mounts three paths out of golang; a
	// named volume and a tmpfs are present too and must be ignored.
	in := `[
	  {"Name":"/go-actions","Mounts":[
	    {"Type":"bind","Source":"/home/server/htdocs/golang/config","Destination":"/app/config"},
	    {"Type":"bind","Source":"/home/server/htdocs/golang/storage","Destination":"/app/storage"},
	    {"Type":"bind","Source":"/home/server/htdocs/golang/production.env","Destination":"/app/production.env"},
	    {"Type":"volume","Source":"/var/lib/docker/volumes/data/_data","Destination":"/data"},
	    {"Type":"tmpfs","Source":"","Destination":"/tmp"}
	  ]},
	  {"Name":"/lonely","Mounts":[]}
	]`
	got := parseBindMounts([]byte(in))
	if len(got) != 1 {
		t.Fatalf("expected 1 container with binds, got %d (%+v)", len(got), got)
	}
	if got[0].name != "go-actions" {
		t.Errorf("name = %q, want go-actions", got[0].name)
	}
	if len(got[0].sources) != 3 {
		t.Errorf("sources = %v, want 3 bind sources", got[0].sources)
	}
}

func TestParseHostExePaths(t *testing.T) {
	in := "/home/server/htdocs/golang/runner/bin/Runner.Listener run\n/usr/bin/dockerd -H fd://\nsshd: root@pts/0\n[kworker/0:1]\n"
	got := parseHostExePaths([]byte(in))
	want := []string{"/home/server/htdocs/golang/runner/bin/Runner.Listener", "/usr/bin/dockerd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHostExePaths() = %v, want %v", got, want)
	}
}

func TestComputeInUse_GolangNearMiss(t *testing.T) {
	dirs := []string{
		"/home/server/htdocs/golang", // live: mounts + runner process
		"/home/server/htdocs/runner", // genuinely dormant (only a Dockerfile+compose)
		"/home/server/htdocs/nocodb", // just down, nothing references it
	}
	binds := []containerBind{
		{name: "go-actions", sources: []string{
			"/home/server/htdocs/golang/config",
			"/home/server/htdocs/golang/storage",
			"/home/server/htdocs/golang/production.env",
		}},
	}
	procs := []string{
		"/home/server/htdocs/golang/runner/bin/Runner.Listener",
		"/usr/bin/dockerd",
	}
	got := computeInUse(dirs, binds, procs)

	golang, ok := got["/home/server/htdocs/golang"]
	if !ok {
		t.Fatal("golang should be flagged in use")
	}
	if len(golang.Mounts) != 1 || golang.Mounts[0].Container != "go-actions" {
		t.Fatalf("golang mounts = %+v, want one go-actions entry", golang.Mounts)
	}
	wantPaths := []string{"config", "production.env", "storage"} // sorted
	if !reflect.DeepEqual(golang.Mounts[0].Paths, wantPaths) {
		t.Errorf("golang mount paths = %v, want %v", golang.Mounts[0].Paths, wantPaths)
	}
	if len(golang.Procs) != 1 || golang.Procs[0] != "/home/server/htdocs/golang/runner/bin/Runner.Listener" {
		t.Errorf("golang procs = %v, want the runner", golang.Procs)
	}
	if _, flagged := got["/home/server/htdocs/runner"]; flagged {
		t.Error("runner is dormant (no mounts/procs) — must NOT be flagged in use")
	}
	if _, flagged := got["/home/server/htdocs/nocodb"]; flagged {
		t.Error("nocodb is just down — must NOT be flagged in use")
	}
}

func TestUnderDir(t *testing.T) {
	if rel, ok := underDir("/home/server/htdocs/golang", "/home/server/htdocs/golang/config"); !ok || rel != "config" {
		t.Errorf("nested = (%q,%v), want (config,true)", rel, ok)
	}
	if rel, ok := underDir("/home/server/htdocs/golang", "/home/server/htdocs/golang"); !ok || rel != "." {
		t.Errorf("exact = (%q,%v), want (.,true)", rel, ok)
	}
	// A sibling with a shared prefix must NOT match (golang-app vs golang).
	if _, ok := underDir("/home/server/htdocs/golang", "/home/server/htdocs/golang-app/x"); ok {
		t.Error("prefix-sibling must not be considered inside the dir")
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
