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
			var got []string
			for _, svc := range parseComposeServices([]byte(tc.in)) {
				got = append(got, svc.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseComposeServices() names = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseComposeServices_Details covers the fields added so a NOT DEPLOYED row
// can state what it would run. Before this, such a row showed only its name and a
// column of dashes: the compose file already held the image and ports, and the
// parser was simply throwing them away.
func TestParseComposeServices_Details(t *testing.T) {
	in := `services:
  minio:
    image: "minio/minio:latest"   # quoted, with a trailing comment
    ports:
      - "9000:9000"
      - '9001:9001'
    volumes:
      - ./data:/data
  openinary:
    build: .
    ports:
      - "3000:3000"
  worker:
    build:
      context: ./worker
      dockerfile: Dockerfile
`
	got := parseComposeServices([]byte(in))
	if len(got) != 3 {
		t.Fatalf("got %d services, want 3: %+v", len(got), got)
	}

	// Quotes and the trailing inline comment are stripped from the image.
	if got[0].Name != "minio" || got[0].Image != "minio/minio:latest" {
		t.Errorf("minio = %+v, want name=minio image=minio/minio:latest", got[0])
	}
	// Both quote styles are accepted; the volumes: list must not leak into ports.
	if !reflect.DeepEqual(got[0].Ports, []string{"9000:9000", "9001:9001"}) {
		t.Errorf("minio ports = %v, want [9000:9000 9001:9001]", got[0].Ports)
	}

	// A built service has no image: — the build context stands in, so the row is
	// not left blank.
	if got[1].Name != "openinary" || got[1].Image != "" || got[1].Build != "." {
		t.Errorf("openinary = %+v, want name=openinary image=\"\" build=.", got[1])
	}
	if !reflect.DeepEqual(got[1].Ports, []string{"3000:3000"}) {
		t.Errorf("openinary ports = %v, want [3000:3000]", got[1].Ports)
	}

	// Block-form build: has no scalar to read; it must still be marked as built
	// rather than reported as a pulled image.
	if got[2].Name != "worker" || got[2].Build != "(build)" {
		t.Errorf("worker = %+v, want name=worker build=(build)", got[2])
	}
	if len(got[2].Ports) != 0 {
		t.Errorf("worker ports = %v, want none", got[2].Ports)
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
