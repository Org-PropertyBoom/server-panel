package services

import (
	"reflect"
	"testing"
)

func TestParseDockerContainers(t *testing.T) {
	output := []byte(`{"Command":"\"nginx\"","CreatedAt":"2026-07-20 10:00:00 +0000 UTC","ID":"abc123","Image":"nginx:alpine","Names":"web","Ports":"0.0.0.0:8080->80/tcp","State":"running","Status":"Up 2 hours"}`)
	containers := parseDockerContainers(output)
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}
	got := containers[0]
	if got.Engine != "docker" || got.Owner != "root" || got.Name != "web" || got.State != "running" {
		t.Fatalf("unexpected container: %#v", got)
	}
	if !reflect.DeepEqual(got.Ports, []string{"0.0.0.0:8080->80/tcp"}) {
		t.Fatalf("unexpected ports: %#v", got.Ports)
	}
}

func TestParsePodmanContainers(t *testing.T) {
	output := []byte(`[{
        "Id":"def456",
        "Names":["api"],
        "Image":"docker.io/library/node:22",
        "Command":["node","server.js"],
        "State":"running",
        "Status":"Up 5 minutes",
        "CreatedAt":1753005600,
        "Ports":[{"host_ip":"127.0.0.1","host_port":3000,"container_port":3000,"protocol":"tcp"}]
    }]`)
	containers := parsePodmanContainers(output, "alice")
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}
	got := containers[0]
	if got.Engine != "podman" || got.Owner != "alice" || got.Name != "api" || got.Command != "node server.js" {
		t.Fatalf("unexpected container: %#v", got)
	}
	if !reflect.DeepEqual(got.Ports, []string{"127.0.0.1:3000->3000/tcp"}) {
		t.Fatalf("unexpected ports: %#v", got.Ports)
	}
}
