package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubRunner lets the plan tests control what `docker ps -a` reports, so the
// duplicate-name guard can be exercised without a Docker daemon.
type stubRunner struct{ names string }

func (s stubRunner) Run(name string, args ...string) ([]byte, error) {
	return []byte(s.names), nil
}

func planSvc(existingNames ...string) *ContainerService {
	return &ContainerService{runner: stubRunner{names: strings.Join(existingNames, "\n")}}
}

func TestPlan_PortWithBindAddressIsPassedThrough(t *testing.T) {
	// The safe default the UI emits. It must VALIDATE (the old regex rejected any
	// explicit bind address) and must NOT be flagged public.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx:1.27", Ports: []string{"127.0.0.1:9001:8080"}})
	if err != nil {
		t.Fatalf("explicit bind address must validate: %v", err)
	}
	if !strings.Contains(plan.Compose, `"127.0.0.1:9001:8080"`) {
		t.Errorf("compose = %q, want the bind address preserved verbatim", plan.Compose)
	}
	for _, wmsg := range plan.Warnings {
		if strings.Contains(wmsg, "PUBLIC") {
			t.Errorf("loopback publish must not warn PUBLIC, got %q", wmsg)
		}
	}
}

func TestPlan_BarePortWarnsPublic(t *testing.T) {
	// `9001:8080` publishes on 0.0.0.0 — the exact default that put MinIO and
	// mysql:3306 on the internet. It must be named every time.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx", Ports: []string{"9001:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, wmsg := range plan.Warnings {
		if strings.Contains(wmsg, "PUBLIC") && strings.Contains(wmsg, "9001") {
			found = true
		}
	}
	if !found {
		t.Errorf("a bare host:container publish must warn PUBLIC, warnings = %v", plan.Warnings)
	}
}

func TestPlan_ExplicitZeroBindIsStillPublic(t *testing.T) {
	// An explicit 0.0.0.0 bind is every interface — as public as omitting it. A
	// "has a bind address ⇒ private" shortcut would silently miss this.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx", Ports: []string{"0.0.0.0:9001:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Warnings, " "), "PUBLIC") {
		t.Errorf("explicit 0.0.0.0 must warn PUBLIC, warnings = %v", plan.Warnings)
	}
	// A non-loopback LAN bind is likewise not private.
	lan, _ := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx", Ports: []string{"192.168.1.5:9001:8080"}})
	if !strings.Contains(strings.Join(lan.Warnings, " "), "PUBLIC") {
		t.Errorf("a non-loopback bind must warn, warnings = %v", lan.Warnings)
	}
}

func TestPlan_DuplicateNameBlocks(t *testing.T) {
	plan, err := planSvc("ppt-phalcon-app", "grafana").PlanContainer(ContainerCreateSpec{Image: "nginx", Name: "grafana"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) == 0 || !strings.Contains(strings.Join(plan.Blocks, " "), "already exists") {
		t.Errorf("duplicate name must BLOCK before docker run, blocks = %v", plan.Blocks)
	}
	// And the block must be enforced server-side, not just displayed.
	if _, err := planSvc("grafana").CreateContainer(ContainerCreateSpec{Image: "nginx", Name: "grafana"}); err == nil {
		t.Error("CreateContainer must refuse a blocked plan, not merely report it")
	}
}

func TestPlan_DockerSockBlocksUnlessConfirmed(t *testing.T) {
	spec := ContainerCreateSpec{Image: "nginx", Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock"}}
	plan, err := planSvc().PlanContainer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) == 0 {
		t.Error("mounting docker.sock must block by default — it is full host root")
	}
	spec.ConfirmDockerSock = true
	confirmed, err := planSvc().PlanContainer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed.Blocks) != 0 {
		t.Errorf("an explicit confirmation must clear the block, blocks = %v", confirmed.Blocks)
	}
	if len(confirmed.Warnings) == 0 {
		t.Error("confirmed docker.sock must still warn")
	}
}

func TestPlan_HostNetworkingDisablesPorts(t *testing.T) {
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "gateway-service:latest", Network: "host", Ports: []string{"8087:8087"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) == 0 {
		t.Error("ports with host networking must block — Docker rejects the combination")
	}
	if !strings.Contains(plan.Compose, "network_mode: host") {
		t.Errorf("compose = %q, want network_mode: host", plan.Compose)
	}
}

// The spec's concrete acceptance case: GatewayService must be expressible.
func TestPlan_GatewayServiceIsCreatable(t *testing.T) {
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{
		Image:   "gateway-service:latest",
		Name:    "gateway-service",
		Network: "host",
		Restart: "unless-stopped",
	})
	if err != nil {
		t.Fatalf("GatewayService must be expressible from the modal: %v", err)
	}
	if len(plan.Blocks) != 0 {
		t.Errorf("unexpected blocks: %v", plan.Blocks)
	}
	for _, want := range []string{"container_name: \"gateway-service\"", "restart: unless-stopped", "network_mode: host", "image: \"gateway-service:latest\""} {
		if !strings.Contains(plan.Compose, want) {
			t.Errorf("compose = %q, missing %q", plan.Compose, want)
		}
	}
}

func TestPlan_LatestTagNoted(t *testing.T) {
	for _, image := range []string{"nginx", "nginx:latest"} {
		plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: image})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(plan.Warnings, " "), ":latest") {
			t.Errorf("%s should carry the :latest note, warnings = %v", image, plan.Warnings)
		}
	}
	plan, _ := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx:1.27"})
	if strings.Contains(strings.Join(plan.Warnings, " "), ":latest") {
		t.Error("a pinned tag must not carry the :latest note")
	}
}

func TestPlan_WritesUnderManagedRootOnly(t *testing.T) {
	// The hard guard: every write is confined to the managed root, so a stack
	// repo's compose file can never be written into, near, or over.
	root := managedComposeRoot()
	for _, name := range []string{"nocodb", "gateway-service"} {
		plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: name})
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(root, name, "docker-compose.yml")
		if plan.Path != want {
			t.Errorf("path = %q, want %q", plan.Path, want)
		}
		if !IsManagedComposeDir(filepath.Dir(plan.Path)) {
			t.Errorf("%q must be inside the managed root", plan.Path)
		}
	}
	// A stack repo is NOT managed — no Edit, no retro-generated file.
	for _, dir := range []string{"/home/server/htdocs/phalcon", "/home/server/htdocs/golang", "/etc", "/home/server/containers-evil"} {
		if IsManagedComposeDir(dir) {
			t.Errorf("%q must NOT be treated as panel-managed", dir)
		}
	}
	// A traversal-shaped name can't escape: it fails name validation outright.
	if _, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "../../htdocs/phalcon"}); err == nil {
		t.Error("a name containing path separators must be rejected")
	}
}

func TestPlan_EnvFileIsReferencedNotInlined(t *testing.T) {
	f := filepath.Join(t.TempDir(), "svc.env")
	if err := os.WriteFile(f, []byte("SECRET_TOKEN=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", EnvFile: f})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Compose, "env_file:") || !strings.Contains(plan.Compose, f) {
		t.Errorf("compose must reference the env file path, got %q", plan.Compose)
	}
	if strings.Contains(plan.Compose, "hunter2") || strings.Contains(plan.Compose, "SECRET_TOKEN") {
		t.Error("the env file's CONTENTS must never appear in the compose file")
	}
}

func TestPlan_DerivesNameWhenBlank(t *testing.T) {
	// A compose project needs a directory, so a blank name is derived and shown.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "nocodb/nocodb:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "nocodb" {
		t.Errorf("derived name = %q, want nocodb", plan.Name)
	}
}

func TestPlan_PortsQuotedInYAML(t *testing.T) {
	// Unquoted, YAML reads 9001:8080 as a sexagesimal integer, not a string.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", Ports: []string{"9001:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Compose, `- "9001:8080"`) {
		t.Errorf("ports must be quoted scalars, got %q", plan.Compose)
	}
}

func TestPlan_PrivilegedBlocksUnlessConfirmed(t *testing.T) {
	// privileged is the same host access as docker.sock by another route —
	// blocking one and not the other would not be a guard.
	spec := ContainerCreateSpec{Image: "x:1", Name: "svc", Privileged: true}
	plan, err := planSvc().PlanContainer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) == 0 {
		t.Error("privileged must block by default")
	}
	spec.ConfirmPrivileged = true
	ok, err := planSvc().PlanContainer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(ok.Blocks) != 0 {
		t.Errorf("an explicit confirmation must clear the block, blocks = %v", ok.Blocks)
	}
	if !strings.Contains(ok.Compose, "privileged: true") {
		t.Errorf("compose = %q, want privileged: true", ok.Compose)
	}
	if !strings.Contains(strings.Join(ok.Warnings, " "), "privileged") {
		t.Error("confirmed privileged must still warn")
	}
}

func TestPlan_ResourceLimits(t *testing.T) {
	// openinary_processor leaked to ~1.5 GB uncapped on this host — an unlimited
	// container starves every tenant site, so the absence is called out.
	none, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(none.Warnings, " "), "no memory limit") {
		t.Errorf("an uncapped container must warn, warnings = %v", none.Warnings)
	}
	capped, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", MemLimit: "512m", CPUs: "1.5"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capped.Compose, "mem_limit: 512m") || !strings.Contains(capped.Compose, `cpus: "1.5"`) {
		t.Errorf("compose = %q, want mem_limit and cpus", capped.Compose)
	}
	if strings.Contains(strings.Join(capped.Warnings, " "), "no memory limit") {
		t.Error("a capped container must not warn about being uncapped")
	}
	if _, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "s", MemLimit: "512 megs"}); err == nil {
		t.Error("a malformed memory limit must be rejected")
	}
}

func TestPlan_CapAddNamedAtReview(t *testing.T) {
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", CapAdd: []string{"NET_ADMIN"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Compose, "cap_add:") || !strings.Contains(plan.Compose, "NET_ADMIN") {
		t.Errorf("compose = %q, want cap_add NET_ADMIN", plan.Compose)
	}
	if !strings.Contains(strings.Join(plan.Warnings, " "), "NET_ADMIN") {
		t.Error("each added capability must be named at Review")
	}
}

// §8 makes the port input structured rows. The emitted compose must stay
// byte-identical, so this pins the exact output the widget has to reproduce.
func TestPlan_PortOutputIsStable(t *testing.T) {
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{
		Image: "x:1", Name: "svc", MemLimit: "512m",
		Ports: []string{"127.0.0.1:9001:8080", "127.0.0.1:9002:8443/udp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "    ports:\n      - \"127.0.0.1:9001:8080\"\n      - \"127.0.0.1:9002:8443/udp\"\n"
	if !strings.Contains(plan.Compose, want) {
		t.Errorf("ports block changed — §8 requires byte-identical output.\ngot:\n%s\nwant to contain:\n%s", plan.Compose, want)
	}
}

func TestPlan_AttachToExistingNetworks(t *testing.T) {
	// The shared-service pattern: mysql sits on four networks so each app's
	// containers reach it by name. Ports still apply alongside — both, not either.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{
		Image: "exporter:1", Name: "exporter", MemLimit: "128m",
		Network:  "attach",
		Networks: []string{"go-actions", "laravel-frankenphp", "mysql_default"},
		Ports:    []string{"127.0.0.1:9104:9104"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) != 0 {
		t.Errorf("attaching networks must not block ports, blocks = %v", plan.Blocks)
	}
	if !strings.Contains(plan.Compose, "    networks:\n      - go-actions\n      - laravel-frankenphp\n      - mysql_default\n") {
		t.Errorf("compose = %q, want the service attached to all three", plan.Compose)
	}
	// 🔴 The panel attaches to networks; it must NEVER create or own one.
	for _, n := range []string{"go-actions", "laravel-frankenphp", "mysql_default"} {
		if !strings.Contains(plan.Compose, "  "+n+":\n    external: true\n") {
			t.Errorf("network %s must be declared external: true — the panel must never create a network.\ngot:\n%s", n, plan.Compose)
		}
	}
	// Ports coexist with attached networks.
	if !strings.Contains(plan.Compose, `- "127.0.0.1:9104:9104"`) {
		t.Errorf("published ports must survive network attachment, got %q", plan.Compose)
	}
	// network_mode must NOT be emitted — it is mutually exclusive with networks.
	if strings.Contains(plan.Compose, "network_mode:") {
		t.Errorf("attach mode must not emit network_mode, got %q", plan.Compose)
	}
}

func TestPlan_AttachRequiresANetwork(t *testing.T) {
	if _, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", Network: "attach"}); err == nil {
		t.Error("attach mode with no networks selected must be rejected")
	}
	if _, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "x:1", Name: "svc", Network: "attach", Networks: []string{"bad name!"}}); err == nil {
		t.Error("an invalid network name must be rejected")
	}
}

func TestPlan_DefaultsPreserved(t *testing.T) {
	// Conservation: restart policy default stays unless-stopped.
	plan, err := planSvc().PlanContainer(ContainerCreateSpec{Image: "nginx:1.27"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Compose, "restart: unless-stopped") {
		t.Errorf("compose = %q, want the unless-stopped default preserved", plan.Compose)
	}
}
