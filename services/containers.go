package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var allowedContainerID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

var (
	ErrContainerDockerfileDenied  = errors.New("Dockerfile access denied")
	ErrContainerDockerfileMissing = errors.New("Dockerfile not found")
)

type ContainerDockerfile struct {
	Content string `json:"content"`
	Path    string `json:"path"`
}

type Container struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Command   string   `json:"command,omitempty"`
	Engine    string   `json:"engine"`
	Owner     string   `json:"owner"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"createdAt,omitempty"`
	Ports     []string `json:"ports"`
	// Reverse route view (the mirror of /vhosts route→container): which hostnames
	// route to this container, joined by its published 127.0.0.1:PORT. Populated by
	// VhostEngineService.AnnotateContainers; empty when there's no host-source.
	RouteHosts       []string `json:"routeHosts,omitempty"`       // App-route hostnames (platform_hosts) pointing here
	RouteTenantCount int      `json:"routeTenantCount,omitempty"` // tenant sites (website_hosts) via this container's stack
	RouteTenantStack string   `json:"routeTenantStack,omitempty"` // the stack name backing those tenants
	// Compose awareness (docker compose labels; "" for standalone / non-compose).
	Project    string `json:"project,omitempty"`    // com.docker.compose.project
	Service    string `json:"service,omitempty"`    // com.docker.compose.service
	WorkingDir string `json:"workingDir,omitempty"` // com.docker.compose.project.working_dir
	Deployed   bool   `json:"deployed"`             // false = a compose service with NO container (not deployed)
	// Managed = the panel holds this container's compose file (it lives under the
	// managed root), so it can be edited and re-applied. Unmanaged containers were
	// made by hand or by a stack pipeline: shown as-is, never retro-given a file.
	Managed bool `json:"managed"`
	// In-use guard: a project dir can be load-bearing even when its service is
	// NOT deployed — a running container bind-mounts a path out of it, or a host
	// process runs from it. Populated only for non-running rows. "Not deployed"
	// ≠ "unused": these say WHY the directory isn't safe to delete.
	InUse       bool       `json:"inUse,omitempty"`
	InUseMounts []MountUse `json:"inUseMounts,omitempty"` // running containers bind-mounting out of the dir
	InUseProcs  []string   `json:"inUseProcs,omitempty"`  // host processes running from inside the dir
}

// ContainerDetails is a curated view of `<engine> inspect <id>` — the fields worth
// showing in a details drawer, plus the pretty-printed raw JSON for a raw view.
type ContainerDetails struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Image         string             `json:"image"`
	ImageID       string             `json:"imageId,omitempty"`
	Created       string             `json:"created,omitempty"`
	Platform      string             `json:"platform,omitempty"`
	Engine        string             `json:"engine"`
	Owner         string             `json:"owner"`
	Command       string             `json:"command,omitempty"`
	Entrypoint    string             `json:"entrypoint,omitempty"`
	WorkingDir    string             `json:"workingDir,omitempty"`
	User          string             `json:"user,omitempty"`
	RestartPolicy string             `json:"restartPolicy,omitempty"`
	State         ContainerState     `json:"state"`
	Env           []string           `json:"env,omitempty"`
	Labels        map[string]string  `json:"labels,omitempty"`
	Ports         []ContainerPortMap `json:"ports,omitempty"`
	Mounts        []ContainerMount   `json:"mounts,omitempty"`
	Networks      []ContainerNetwork `json:"networks,omitempty"`
	SizeRw        *int64             `json:"sizeRw,omitempty"`     // writable layer bytes (docker --size); nil if not computed
	SizeRootFs    *int64             `json:"sizeRootFs,omitempty"` // container rootfs bytes (snapshotter; dedups shared layers)
	ImageSize     *int64             `json:"imageSize,omitempty"`  // image `.Size` — compressed content/pull size on the containerd store (matches `docker images` CONTENT SIZE)
	Raw           string             `json:"raw,omitempty"`
}

type ContainerState struct {
	Status              string `json:"status,omitempty"`
	Running             bool   `json:"running"`
	ExitCode            int    `json:"exitCode"`
	StartedAt           string `json:"startedAt,omitempty"`
	FinishedAt          string `json:"finishedAt,omitempty"`
	Health              string `json:"health,omitempty"`
	HealthTest          string `json:"healthTest,omitempty"`          // the HEALTHCHECK command
	HealthFailingStreak int    `json:"healthFailingStreak,omitempty"` // consecutive failures
	HealthLastExit      int    `json:"healthLastExit,omitempty"`      // last probe exit code
	HealthLastOutput    string `json:"healthLastOutput,omitempty"`    // last probe output (the reason)
	RestartCount        int    `json:"restartCount,omitempty"`
}

type ContainerPortMap struct {
	Container string `json:"container"`      // e.g. "80/tcp"
	Host      string `json:"host,omitempty"` // e.g. "0.0.0.0:8004"; "" = unpublished
}

type ContainerMount struct {
	Type        string `json:"type,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Mode        string `json:"mode,omitempty"`
	RW          bool   `json:"rw"`
}

type ContainerNetwork struct {
	Name       string `json:"name"`
	IPAddress  string `json:"ipAddress,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	MacAddress string `json:"macAddress,omitempty"`
}

// rawInspect maps the subset of the docker/podman inspect JSON we surface. Both
// engines follow the Docker schema for these fields.
type rawInspect struct {
	Id       string `json:"Id"`
	Name     string `json:"Name"`
	Created  string `json:"Created"`
	Platform string `json:"Platform"`
	Image    string `json:"Image"`
	State    struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status        string `json:"Status"`
			FailingStreak int    `json:"FailingStreak"`
			Log           []struct {
				ExitCode int    `json:"ExitCode"`
				Output   string `json:"Output"`
			} `json:"Log"`
		} `json:"Health"`
	} `json:"State"`
	RestartCount int    `json:"RestartCount"`
	SizeRw       *int64 `json:"SizeRw"`     // present only with `inspect --size`
	SizeRootFs   *int64 `json:"SizeRootFs"` // present only with `inspect --size`
	Config       struct {
		Image       string            `json:"Image"`
		Cmd         []string          `json:"Cmd"`
		Entrypoint  []string          `json:"Entrypoint"`
		WorkingDir  string            `json:"WorkingDir"`
		User        string            `json:"User"`
		Env         []string          `json:"Env"`
		Labels      map[string]string `json:"Labels"`
		Healthcheck *struct {
			Test []string `json:"Test"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Mode        string `json:"Mode"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIp   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
		Networks map[string]struct {
			IPAddress  string `json:"IPAddress"`
			Gateway    string `json:"Gateway"`
			MacAddress string `json:"MacAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type containerCommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type timedContainerCommandRunner struct{}

func (timedContainerCommandRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type ContainerService struct {
	runner containerCommandRunner
}

func NewContainerService() *ContainerService {
	return &ContainerService{runner: timedContainerCommandRunner{}}
}

func (s *ContainerService) ListAll() []Container {
	result := s.listDocker()
	users, _ := HomeUsers()

	var mu sync.Mutex
	var wait sync.WaitGroup
	limit := make(chan struct{}, 4)
	for _, linuxUser := range users {
		if linuxUser.UID < 0 {
			continue
		}
		wait.Add(1)
		go func(linuxUser LinuxUser) {
			defer wait.Done()
			limit <- struct{}{}
			containers := s.listRootlessPodman(linuxUser)
			<-limit
			mu.Lock()
			result = append(result, containers...)
			mu.Unlock()
		}(linuxUser)
	}
	wait.Wait()
	sortContainers(result)
	return result
}

func (s *ContainerService) ListCurrentUser(username string) []Container {
	if !isCurrentUser(username) {
		return []Container{}
	}
	output, err := s.runner.Run("podman", "ps", "-a", "--format", "json")
	if err != nil {
		return []Container{}
	}
	result := parsePodmanContainers(output, username)
	sortContainers(result)
	return result
}

func (s *ContainerService) ActionAll(engine, owner, id, action string) error {
	args, err := containerActionArgs(id, action)
	if err != nil {
		return err
	}
	_, err = s.runForOwner(engine, owner, args...)
	return err
}

func (s *ContainerService) LogsAll(engine, owner, id string) (string, error) {
	if !allowedContainerID.MatchString(id) {
		return "", errors.New("invalid container")
	}
	output, err := s.runForOwner(engine, owner, "logs", "--tail", "200", id)
	return string(output), err
}

func (s *ContainerService) ActionCurrentUser(username, id, action string) error {
	if !isCurrentUser(username) {
		return errors.New("container owner unavailable")
	}
	args, err := containerActionArgs(id, action)
	if err != nil {
		return err
	}
	_, err = s.runner.Run("podman", args...)
	return err
}

func (s *ContainerService) LogsCurrentUser(username, id string) (string, error) {
	if !isCurrentUser(username) || !allowedContainerID.MatchString(id) {
		return "", errors.New("invalid container")
	}
	output, err := s.runner.Run("podman", "logs", "--tail", "200", id)
	return string(output), err
}

// InspectAll returns curated `<engine> inspect` details for a root-visible
// container (Docker as root, or a user's rootless Podman container).
func (s *ContainerService) InspectAll(engine, owner, id string) (ContainerDetails, error) {
	if !allowedContainerID.MatchString(id) {
		return ContainerDetails{}, errors.New("invalid container")
	}
	// Docker's `inspect --size` walks the graph driver to compute SizeRw/SizeRootFs,
	// which can exceed the 5s list timeout — run it detached with a longer budget.
	// Podman inspect has no --size flag, so it reports no size (fine).
	var output []byte
	var err error
	if engine == "docker" && (owner == "root" || owner == "system") {
		output, err = runContainerCommand("", 30*time.Second, "docker", "inspect", "--size", id)
	} else {
		output, err = s.runForOwner(engine, owner, "inspect", id)
	}
	if err != nil {
		return ContainerDetails{}, err
	}
	details, perr := parseContainerDetails(output, engine, owner)
	if perr != nil {
		return details, perr
	}
	// The container inspect gives writable + rootfs sizes but not the image's own
	// size — fetch `.Size` (the compressed content/pull size on the containerd
	// store) so the panel can show the transfer footprint.
	if engine == "docker" && details.ImageID != "" {
		if out, e := runContainerCommand("", 10*time.Second, "docker", "image", "inspect", details.ImageID, "--format", "{{.Size}}"); e == nil {
			if n, pe := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); pe == nil && n > 0 {
				details.ImageSize = &n
			}
		}
	}
	return details, nil
}

// InspectCurrentUser returns curated inspect details for one of the calling user's
// own rootless Podman containers.
func (s *ContainerService) InspectCurrentUser(username, id string) (ContainerDetails, error) {
	if !isCurrentUser(username) || !allowedContainerID.MatchString(id) {
		return ContainerDetails{}, errors.New("invalid container")
	}
	output, err := s.runner.Run("podman", "inspect", id)
	if err != nil {
		return ContainerDetails{}, err
	}
	return parseContainerDetails(output, "podman", username)
}

func parseContainerDetails(output []byte, engine, owner string) (ContainerDetails, error) {
	var arr []rawInspect
	if err := json.Unmarshal(output, &arr); err != nil || len(arr) == 0 {
		return ContainerDetails{}, errors.New("could not read container details")
	}
	r := arr[0]
	d := ContainerDetails{
		ID:         r.Id,
		Name:       strings.TrimPrefix(r.Name, "/"),
		Image:      firstNonEmpty(r.Config.Image, r.Image),
		ImageID:    r.Image,
		Created:    r.Created,
		Platform:   r.Platform,
		Engine:     engine,
		Owner:      owner,
		Command:    strings.TrimSpace(strings.Join(r.Config.Cmd, " ")),
		Entrypoint: strings.TrimSpace(strings.Join(r.Config.Entrypoint, " ")),
		WorkingDir: r.Config.WorkingDir,
		User:       r.Config.User,
		Env:        r.Config.Env,
		Labels:     r.Config.Labels,
		State: ContainerState{
			Status:       r.State.Status,
			Running:      r.State.Running,
			ExitCode:     r.State.ExitCode,
			StartedAt:    r.State.StartedAt,
			FinishedAt:   r.State.FinishedAt,
			RestartCount: r.RestartCount,
		},
	}
	if r.State.Health != nil {
		d.State.Health = r.State.Health.Status
		d.State.HealthFailingStreak = r.State.Health.FailingStreak
		if n := len(r.State.Health.Log); n > 0 {
			last := r.State.Health.Log[n-1]
			d.State.HealthLastExit = last.ExitCode
			d.State.HealthLastOutput = strings.TrimSpace(last.Output)
		}
	}
	if hc := r.Config.Healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] != "NONE" {
		switch hc.Test[0] {
		case "CMD-SHELL":
			d.State.HealthTest = strings.Join(hc.Test[1:], " ")
		case "CMD":
			d.State.HealthTest = strings.Join(hc.Test[1:], " ")
		default:
			d.State.HealthTest = strings.Join(hc.Test, " ")
		}
	}
	d.SizeRw, d.SizeRootFs = r.SizeRw, r.SizeRootFs
	if name := r.HostConfig.RestartPolicy.Name; name != "" {
		d.RestartPolicy = name
		if r.HostConfig.RestartPolicy.MaximumRetryCount > 0 {
			d.RestartPolicy = fmt.Sprintf("%s (max %d)", name, r.HostConfig.RestartPolicy.MaximumRetryCount)
		}
	}
	for portKey, bindings := range r.NetworkSettings.Ports {
		if len(bindings) == 0 {
			d.Ports = append(d.Ports, ContainerPortMap{Container: portKey})
			continue
		}
		for _, b := range bindings {
			host := b.HostPort
			if b.HostIp != "" {
				host = b.HostIp + ":" + b.HostPort
			}
			d.Ports = append(d.Ports, ContainerPortMap{Container: portKey, Host: host})
		}
	}
	sort.Slice(d.Ports, func(i, j int) bool { return d.Ports[i].Container < d.Ports[j].Container })
	for _, m := range r.Mounts {
		d.Mounts = append(d.Mounts, ContainerMount{Type: m.Type, Source: m.Source, Destination: m.Destination, Mode: m.Mode, RW: m.RW})
	}
	for name, net := range r.NetworkSettings.Networks {
		d.Networks = append(d.Networks, ContainerNetwork{Name: name, IPAddress: net.IPAddress, Gateway: net.Gateway, MacAddress: net.MacAddress})
	}
	sort.Slice(d.Networks, func(i, j int) bool { return d.Networks[i].Name < d.Networks[j].Name })
	var pretty bytes.Buffer
	if json.Indent(&pretty, output, "", "  ") == nil {
		d.Raw = pretty.String()
	}
	return d, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ContainerStat is one container's live resource usage from `docker stats`.
type ContainerStat struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPerc    float64 `json:"cpuPerc"`
	MemUsed    int64   `json:"memUsed"`
	MemLimit   int64   `json:"memLimit"`
	MemPerc    float64 `json:"memPerc"`
	NetRx      int64   `json:"netRx"`
	NetTx      int64   `json:"netTx"`
	BlockRead  int64   `json:"blockRead"`
	BlockWrite int64   `json:"blockWrite"`
	PIDs       int     `json:"pids"`
}

// Stats returns live per-container CPU/memory/network/block-IO from a single
// `docker stats --no-stream` snapshot (root Docker). It samples CPU over a short
// interval so it takes ~1-2s — run detached with a generous timeout.
func (s *ContainerService) Stats() ([]ContainerStat, error) {
	out, err := runContainerCommand("", 20*time.Second, "docker", "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var result []ContainerStat
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			ID       string `json:"ID"`
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
			MemPerc  string `json:"MemPerc"`
			NetIO    string `json:"NetIO"`
			BlockIO  string `json:"BlockIO"`
			PIDs     string `json:"PIDs"`
		}
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		st := ContainerStat{
			ID:      raw.ID,
			Name:    raw.Name,
			CPUPerc: parsePercent(raw.CPUPerc),
			MemPerc: parsePercent(raw.MemPerc),
		}
		st.MemUsed, st.MemLimit = parseSizePair(raw.MemUsage)
		st.NetRx, st.NetTx = parseSizePair(raw.NetIO)
		st.BlockRead, st.BlockWrite = parseSizePair(raw.BlockIO)
		if n, err := strconv.Atoi(strings.TrimSpace(raw.PIDs)); err == nil {
			st.PIDs = n
		}
		result = append(result, st)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].MemUsed > result[j].MemUsed })
	return result, nil
}

// parsePercent parses docker's "12.34%" → 12.34.
func parsePercent(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), 64)
	return n
}

// parseSizePair splits docker's "1.2GiB / 7.6GiB" (or "1.2kB / 0B") into two byte
// counts.
func parseSizePair(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return parseDockerSize(s), 0
	}
	return parseDockerSize(parts[0]), parseDockerSize(parts[1])
}

// parseDockerSize converts a docker size token to bytes, handling both binary
// (KiB/MiB/GiB/TiB) and SI (kB/MB/GB/TB) units docker mixes across fields.
func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(s[i:])) {
	case "b", "":
		return int64(num)
	case "kb":
		return int64(num * 1e3)
	case "mb":
		return int64(num * 1e6)
	case "gb":
		return int64(num * 1e9)
	case "tb":
		return int64(num * 1e12)
	case "kib":
		return int64(num * 1024)
	case "mib":
		return int64(num * 1024 * 1024)
	case "gib":
		return int64(num * 1024 * 1024 * 1024)
	case "tib":
		return int64(num * 1024 * 1024 * 1024 * 1024)
	}
	return int64(num)
}

// ContainerCreateSpec is the `docker run` form: image is required; everything
// else optional. Values are validated and passed as separate exec args (no shell).
type ContainerCreateSpec struct {
	Image   string   `json:"image"`
	Name    string   `json:"name"`
	Ports   []string `json:"ports"`   // "[bind:]host:container" or "…/proto"
	Env     []string `json:"env"`     // "KEY=VALUE"
	Volumes []string `json:"volumes"` // "src:dst" or "src:dst:ro"
	Restart string   `json:"restart"` // no | always | unless-stopped | on-failure
	// Network is the container network mode: bridge (default) | host | none.
	// host/none make port publishing inapplicable — Docker rejects -p with them.
	Network string `json:"network"`
	// EnvFile is a path passed as --env-file. The panel NEVER reads or stores the
	// contents: a path reference keeps secrets out of the DOM, request logs and
	// panel storage.
	EnvFile string `json:"envFile"`
	// User is an optional "uid:gid" the container runs as (compose `user:`), the
	// remedy sitting next to the runs-as-root warning.
	User string `json:"user"`
	// MemLimit / CPUs are optional resource caps, empty by default. Not
	// hypothetical here: openinary_processor ran uncapped and leaked to ~1.5 GB
	// before being capped by hand — an uncapped container on a shared box can
	// starve every tenant site on it.
	MemLimit string `json:"memLimit"` // e.g. "512m"
	CPUs     string `json:"cpus"`     // e.g. "1.5"
	// Privileged grants effectively the same host access as mounting docker.sock,
	// by another route — so it is blocked by the same typed confirmation. Blocking
	// one escape hatch and not the other would not be a guard.
	Privileged        bool     `json:"privileged"`
	ConfirmPrivileged bool     `json:"confirmPrivileged"`
	CapAdd            []string `json:"capAdd"`     // allowed, but each named at Review
	AlwaysPull        bool     `json:"alwaysPull"` // pull_policy: always — a real answer to :latest staleness
	// ConfirmDockerSock authorizes mounting /var/run/docker.sock, which grants
	// full host root via container escape. Blocked unless explicitly confirmed.
	ConfirmDockerSock bool `json:"confirmDockerSock"`
}

// ContainerPlan is the Review step: the compose FILE that will be written, where
// it goes, and every guard — gathered before anything executes. Built by the same
// code path that creates the container, so what's shown cannot drift from what
// happens.
type ContainerPlan struct {
	Name     string   `json:"name"`    // resolved service/container name (derived if left blank)
	Path     string   `json:"path"`    // /home/server/containers/<name>/docker-compose.yml
	Compose  string   `json:"compose"` // the generated YAML
	Warnings []string `json:"warnings"`
	Blocks   []string `json:"blocks"` // non-empty ⇒ Create refuses
}

// managedComposeRoot is the ONLY directory the panel writes compose files into.
// Stack repos have their own deploy pipelines and must never be written to, near,
// or over — confining every write to one root is what makes that guarantee
// checkable rather than merely intended. Override with CONTAINER_COMPOSE_ROOT.
func managedComposeRoot() string {
	if v := strings.TrimSpace(os.Getenv("CONTAINER_COMPOSE_ROOT")); v != "" {
		return filepath.Clean(v)
	}
	return "/home/server/containers"
}

// IsManagedComposeDir reports whether a compose working directory belongs to the
// panel — i.e. the panel holds the file and may edit it. Anything else is
// unmanaged: pre-existing, hand-made, or from a stack pipeline.
func IsManagedComposeDir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	root := managedComposeRoot()
	clean := filepath.Clean(dir)
	return clean == root || strings.HasPrefix(clean, root+string(filepath.Separator))
}

var (
	allowedComposeService = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	allowedImageRef       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)
	// Accepts an OPTIONAL explicit bind address, so `127.0.0.1:9001:8080` (the new
	// safe default, and anything the operator spelled out themselves) validates.
	// Without the bind group Docker publishes on 0.0.0.0 — every interface.
	allowedPortMapping   = regexp.MustCompile(`^((\d{1,3}\.){3}\d{1,3}:)?(\d{1,5}:)?\d{1,5}(/(tcp|udp))?$`)
	allowedEnvKey        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	allowedMemLimit      = regexp.MustCompile(`^\d+(\.\d+)?[bkmgBKMG]?$`)
	allowedCPUs          = regexp.MustCompile(`^\d+(\.\d+)?$`)
	allowedCapability    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	allowedRestartPolicy = map[string]bool{"no": true, "always": true, "unless-stopped": true, "on-failure": true}
)

// buildTimeout is the ceiling for an image build/rebuild. Defaults to 30m (a
// from-scratch build of a heavy image — apt + compiled extensions — easily exceeds
// 10m); override with CONTAINER_BUILD_TIMEOUT (whole minutes).
func buildTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("CONTAINER_BUILD_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 30 * time.Minute
}

// runContainerCommand runs a long-lived container command (image build / pull),
// well beyond the 5s list timeout, optionally in a working directory. It uses a
// detached context so a client disconnect doesn't abort a build mid-flight. On the
// timeout it returns whatever output was produced plus a CLEAR timed-out error
// (rather than a cryptic "signal: killed").
func runContainerCommand(dir string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timed out after %s — the build was still running when it was stopped. Raise CONTAINER_BUILD_TIMEOUT (minutes) in the root env if this build legitimately needs longer", timeout)
	}
	return out, err
}

// RebuildAll rebuilds + recreates a Docker Compose-managed root container from its
// (possibly just-edited) Dockerfile: `docker compose up -d --build --no-deps
// <service>` in the container's compose working dir. Compose only recreates the
// service on a successful build, so a bad Dockerfile leaves the running container
// untouched. Returns the combined build log. Not supported for non-compose or
// rootless Podman containers.
func (s *ContainerService) RebuildAll(engine, owner, id string) (string, error) {
	if engine != "docker" || (owner != "root" && owner != "system") {
		return "", errors.New("rebuild is only supported for root Docker containers")
	}
	if !allowedContainerID.MatchString(id) {
		return "", errors.New("invalid container")
	}
	output, err := s.runForOwner(engine, owner, "inspect", id)
	if err != nil {
		return "", err
	}
	var arr []rawInspect
	if json.Unmarshal(output, &arr) != nil || len(arr) == 0 {
		return "", errors.New("could not read container details")
	}
	labels := arr[0].Config.Labels
	workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	if workingDir == "" || service == "" {
		return "", errors.New("rebuild needs a Docker Compose-managed container (compose labels missing) — recreate it from its stack instead")
	}
	if !allowedComposeService.MatchString(service) {
		return "", errors.New("invalid compose service name")
	}
	if !filepath.IsAbs(workingDir) {
		return "", errors.New("invalid compose working directory")
	}
	args := []string{"compose"}
	for _, cf := range composeConfigFiles(labels["com.docker.compose.project.config_files"], workingDir) {
		args = append(args, "-f", cf)
	}
	args = append(args, "up", "-d", "--build", "--no-deps", service)
	out, err := runContainerCommand(workingDir, buildTimeout(), "docker", args...)
	return string(out), err
}

// RecreateAll recreates a Docker Compose-managed root container from its CURRENT
// image + compose config, WITHOUT rebuilding: `docker compose up -d --no-deps
// --force-recreate <service>`. Use it to re-apply a changed compose file / env, or
// to pick up a re-pulled image. Returns the combined output.
func (s *ContainerService) RecreateAll(engine, owner, id string) (string, error) {
	if engine != "docker" || (owner != "root" && owner != "system") {
		return "", errors.New("recreate is only supported for root Docker containers")
	}
	if !allowedContainerID.MatchString(id) {
		return "", errors.New("invalid container")
	}
	output, err := s.runForOwner(engine, owner, "inspect", id)
	if err != nil {
		return "", err
	}
	var arr []rawInspect
	if json.Unmarshal(output, &arr) != nil || len(arr) == 0 {
		return "", errors.New("could not read container details")
	}
	labels := arr[0].Config.Labels
	workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	if workingDir == "" || service == "" {
		return "", errors.New("recreate needs a Docker Compose-managed container (compose labels missing)")
	}
	if !allowedComposeService.MatchString(service) {
		return "", errors.New("invalid compose service name")
	}
	if !filepath.IsAbs(workingDir) {
		return "", errors.New("invalid compose working directory")
	}
	args := []string{"compose"}
	for _, cf := range composeConfigFiles(labels["com.docker.compose.project.config_files"], workingDir) {
		args = append(args, "-f", cf)
	}
	args = append(args, "up", "-d", "--no-deps", "--force-recreate", service)
	out, err := runContainerCommand(workingDir, buildTimeout(), "docker", args...)
	return string(out), err
}

// composeConfigFiles resolves the compose project's config-file label (comma-
// separated, possibly relative to the working dir) into absolute paths. Empty
// label → nil, letting compose use its default file resolution in the working dir.
func composeConfigFiles(label, workingDir string) []string {
	label = strings.TrimSpace(label)
	if label == "" || label == "<nil>" {
		return nil
	}
	var files []string
	for _, part := range strings.Split(label, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !filepath.IsAbs(part) {
			part = filepath.Join(workingDir, part)
		}
		files = append(files, part)
	}
	return files
}

// CreateContainer runs a new detached Docker container (`docker run -d`). Root
// Docker only. Every field is validated and passed as a discrete exec arg so
// there's no shell to inject into. Returns the run output (new container id on
// success, or the error output on failure).
// CreateContainer writes the generated docker-compose.yml into the managed root
// and runs `docker compose up -d` there. It does NOT shell out to `docker run`:
// a run command evaporates once issued, leaving Docker's own state as the only
// record, whereas the file persists — editable, diffable, backup-able, and
// surviving a host rebuild.
func (s *ContainerService) CreateContainer(spec ContainerCreateSpec) (string, error) {
	plan, err := s.PlanContainer(spec)
	if err != nil {
		return "", err
	}
	// Blocking guards are refused HERE too, not only in the UI — the Review step is
	// an explanation, never the enforcement.
	if len(plan.Blocks) > 0 {
		return "", errors.New(strings.Join(plan.Blocks, "; "))
	}
	dir := filepath.Dir(plan.Path)
	// Belt-and-braces on the hard guard: refuse any path that escaped the managed
	// root, however it was constructed.
	if !IsManagedComposeDir(dir) {
		return "", fmt.Errorf("refusing to write outside %s", managedComposeRoot())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s: %w", dir, err)
	}
	if err := os.WriteFile(plan.Path, []byte(plan.Compose), 0o644); err != nil {
		return "", fmt.Errorf("could not write %s: %w", plan.Path, err)
	}
	out, err := runContainerCommand(dir, 5*time.Minute, "docker", "compose", "up", "-d")
	return string(out), err
}

// PlanContainer validates a spec and generates the docker-compose.yml that will be
// written, plus the guard warnings/blocks. CreateContainer writes exactly this
// file, so the Review step reviews the real artifact — not a synthesized command.
func (s *ContainerService) PlanContainer(spec ContainerCreateSpec) (ContainerPlan, error) {
	plan := ContainerPlan{Warnings: []string{}, Blocks: []string{}}

	image := strings.TrimSpace(spec.Image)
	if image == "" || strings.HasPrefix(image, "-") || !allowedImageRef.MatchString(image) {
		return plan, errors.New("a valid image is required")
	}

	// A compose project needs a directory, so unlike `docker run` there is no
	// "let Docker auto-name it". When the field is blank we derive a name from the
	// image and show it at Review, which is what the operator needs to see.
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = deriveContainerName(image)
	}
	if !allowedContainerID.MatchString(name) {
		return plan, errors.New("invalid container name")
	}
	plan.Name = name
	plan.Path = filepath.Join(managedComposeRoot(), name, "docker-compose.yml")
	if s.containerNameExists(name) {
		// Docker's duplicate-name error is cryptic; say it plainly, up front.
		plan.Blocks = append(plan.Blocks, fmt.Sprintf("a container named %q already exists — pick another name", name))
	}

	restart := strings.TrimSpace(spec.Restart)
	if restart == "" {
		restart = "unless-stopped"
	}
	if !allowedRestartPolicy[restart] {
		return plan, errors.New("invalid restart policy")
	}

	network := strings.TrimSpace(spec.Network)
	if network == "" {
		network = "bridge"
	}
	if network != "bridge" && network != "host" && network != "none" {
		return plan, fmt.Errorf("invalid network mode %q (bridge, host or none)", network)
	}

	ports := trimmedNonEmpty(spec.Ports)
	if len(ports) > 0 && network != "bridge" {
		// A real Docker rule — surface it as a block rather than letting it come
		// back as a confusing runtime error.
		plan.Blocks = append(plan.Blocks, fmt.Sprintf("port mapping does not apply with %s networking — the container uses the host's ports directly", network))
	}
	for _, p := range ports {
		if !allowedPortMapping.MatchString(p) {
			return plan, fmt.Errorf("invalid port mapping %q (use host:container)", p)
		}
		// Warn unless the publish is bound to loopback. A mapping with NO bind
		// address defaults to 0.0.0.0, and an explicit 0.0.0.0 is public too —
		// this is how MinIO and mysql:3306 ended up on the internet, so name it
		// every time rather than trusting the operator to remember.
		if bind, hostPort := splitPublish(p); !isLoopbackBind(bind) {
			where := bind
			if where == "" {
				where = "0.0.0.0"
			}
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("PUBLIC: port %s is published on %s — reachable from any address on the internet", hostPort, where))
		}
	}

	envFile := strings.TrimSpace(spec.EnvFile)
	if envFile != "" {
		if !filepath.IsAbs(envFile) {
			return plan, errors.New("the env file path must be absolute")
		}
		info, err := os.Stat(envFile)
		if err != nil {
			return plan, fmt.Errorf("cannot read the env file at %s: %w", envFile, err)
		}
		if info.IsDir() {
			return plan, fmt.Errorf("%s is a directory, not an env file", envFile)
		}
		if info.Mode().Perm()&0o077 != 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s is mode %04o — readable beyond its owner; 0600 is expected for a secrets file", envFile, info.Mode().Perm()))
		}
	}

	envs := trimmedNonEmpty(spec.Env)
	for _, e := range envs {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 || !allowedEnvKey.MatchString(e[:eq]) {
			return plan, fmt.Errorf("invalid environment variable %q (use KEY=VALUE)", e)
		}
	}

	volumes := trimmedNonEmpty(spec.Volumes)
	for _, v := range volumes {
		if strings.HasPrefix(v, "-") || !strings.Contains(v, ":") {
			return plan, fmt.Errorf("invalid volume %q (use src:dst)", v)
		}
		src := v[:strings.Index(v, ":")]
		switch {
		case src == "/var/run/docker.sock":
			if !spec.ConfirmDockerSock {
				plan.Blocks = append(plan.Blocks, "mounting /var/run/docker.sock grants full host root through container escape — confirm explicitly if you truly intend it")
			} else {
				plan.Warnings = append(plan.Warnings, "docker.sock is mounted — this container has full host root")
			}
		case src == "/" || src == "/etc" || src == "/root":
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("mounting %s exposes host system files to the container", src))
		case src == "/home/server/.caddy" || strings.HasPrefix(src, "/home/server/.caddy/"):
			plan.Warnings = append(plan.Warnings, "mounting /home/server/.caddy — Caddy's certificate store, readable only by root and `server`. A wrong mount here drops every vhost.")
		}
	}

	if !strings.Contains(image, ":") || strings.HasSuffix(image, ":latest") {
		note := "`:latest` means whatever it is today — a restart months from now may run different code"
		if spec.AlwaysPull {
			note += " (always-pull is on, so a restart takes the newest image rather than a stale local copy)"
		}
		plan.Warnings = append(plan.Warnings, note)
	}

	// Resource caps. Empty = no limit, which is why the UI says so plainly.
	memLimit := strings.TrimSpace(spec.MemLimit)
	if memLimit != "" && !allowedMemLimit.MatchString(memLimit) {
		return plan, fmt.Errorf("invalid memory limit %q (e.g. 512m, 2g)", memLimit)
	}
	cpus := strings.TrimSpace(spec.CPUs)
	if cpus != "" && !allowedCPUs.MatchString(cpus) {
		return plan, fmt.Errorf("invalid CPU limit %q (e.g. 1.5)", cpus)
	}
	if memLimit == "" {
		plan.Warnings = append(plan.Warnings, "no memory limit — this container can consume all host RAM, and every site on this box shares it")
	}

	// privileged: the same host access as mounting docker.sock, by another route.
	if spec.Privileged && !spec.ConfirmPrivileged {
		plan.Blocks = append(plan.Blocks, "privileged mode grants effectively full host access — confirm explicitly if you truly intend it")
	} else if spec.Privileged {
		plan.Warnings = append(plan.Warnings, "privileged mode is on — this container has effectively full host access")
	}

	caps := trimmedNonEmpty(spec.CapAdd)
	for _, c := range caps {
		if !allowedCapability.MatchString(c) {
			return plan, fmt.Errorf("invalid capability %q", c)
		}
		plan.Warnings = append(plan.Warnings, "added Linux capability "+c)
	}

	plan.Compose = renderComposeFile(composeSpec{
		Name: name, Image: image, Restart: restart, Network: network,
		Ports: ports, EnvFile: envFile, Env: envs, Volumes: volumes, User: strings.TrimSpace(spec.User),
		MemLimit: memLimit, CPUs: cpus, Privileged: spec.Privileged, CapAdd: caps, AlwaysPull: spec.AlwaysPull,
	})
	return plan, nil
}

type composeSpec struct {
	Name, Image, Restart, Network, EnvFile, User string
	Ports, Env, Volumes, CapAdd                  []string
	MemLimit, CPUs                               string
	Privileged, AlwaysPull                       bool
}

// renderComposeFile writes the compose YAML by hand — the values are already
// validated against strict patterns, so quoting each scalar is enough and it
// avoids pulling a YAML dependency in for one emitter.
func renderComposeFile(c composeSpec) string {
	var b strings.Builder
	b.WriteString("# Written by Ppt Server Panel — edit and re-run `docker compose up -d` here.\n")
	b.WriteString("services:\n")
	b.WriteString("  " + c.Name + ":\n")
	b.WriteString("    image: " + yamlScalar(c.Image) + "\n")
	b.WriteString("    container_name: " + yamlScalar(c.Name) + "\n")
	b.WriteString("    restart: " + c.Restart + "\n")
	if c.Network != "bridge" {
		b.WriteString("    network_mode: " + c.Network + "\n")
	}
	if c.User != "" {
		b.WriteString("    user: " + yamlScalar(c.User) + "\n")
	}
	if len(c.Ports) > 0 {
		b.WriteString("    ports:\n")
		for _, p := range c.Ports {
			b.WriteString("      - " + yamlScalar(p) + "\n")
		}
	}
	if c.EnvFile != "" {
		// The PATH is referenced, never the values — a compose file that inlines
		// secrets is a secret store nobody chose to create.
		b.WriteString("    env_file:\n      - " + yamlScalar(c.EnvFile) + "\n")
	}
	if len(c.Env) > 0 {
		b.WriteString("    environment:\n")
		for _, e := range c.Env {
			b.WriteString("      - " + yamlScalar(e) + "\n")
		}
	}
	if len(c.Volumes) > 0 {
		b.WriteString("    volumes:\n")
		for _, v := range c.Volumes {
			b.WriteString("      - " + yamlScalar(v) + "\n")
		}
	}
	if c.MemLimit != "" {
		b.WriteString("    mem_limit: " + c.MemLimit + "\n")
	}
	if c.CPUs != "" {
		b.WriteString("    cpus: " + yamlScalar(c.CPUs) + "\n")
	}
	if c.AlwaysPull {
		b.WriteString("    pull_policy: always\n")
	}
	if c.Privileged {
		b.WriteString("    privileged: true\n")
	}
	if len(c.CapAdd) > 0 {
		b.WriteString("    cap_add:\n")
		for _, cp := range c.CapAdd {
			b.WriteString("      - " + cp + "\n")
		}
	}
	return b.String()
}

// yamlScalar double-quotes a value so ports like "9001:8080" are never parsed as
// sexagesimals and colons never split a mapping.
func yamlScalar(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// deriveContainerName builds a name from the image when the field is left blank
// ("nocodb/nocodb:latest" -> "nocodb"), so the compose directory always has one.
func deriveContainerName(image string) string {
	name := image
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexAny(name, ":@"); i >= 0 {
		name = name[:i]
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			return r
		default:
			return '-'
		}
	}, name)
	return strings.Trim(name, "-.")
}

// splitPublish separates an optional bind address from a `[bind:]host:container`
// publish spec, returning ("" , hostPort) when no bind address is present.
func splitPublish(p string) (bind, hostPort string) {
	p = strings.TrimSuffix(strings.TrimSuffix(p, "/tcp"), "/udp")
	parts := strings.Split(p, ":")
	switch len(parts) {
	case 3: // bind:host:container
		return parts[0], parts[1]
	case 2: // host:container
		return "", parts[0]
	default: // container port only — Docker picks a random host port on 0.0.0.0
		return "", parts[0]
	}
}

// isLoopbackBind reports whether a publish bind address keeps the port private.
// Only loopback qualifies: "" means Docker's 0.0.0.0 default, and an explicit
// 0.0.0.0 is every interface — both are public.
func isLoopbackBind(bind string) bool {
	if bind == "" {
		return false
	}
	ip := net.ParseIP(bind)
	return ip != nil && ip.IsLoopback()
}

func trimmedNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// containerNameExists reports whether a container (running or not) already uses
// this name, so the duplicate is caught before `docker run` is attempted.
func (s *ContainerService) containerNameExists(name string) bool {
	out, err := s.runner.Run("docker", "ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		return false // can't tell — don't invent a block; docker will still refuse
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func (s *ContainerService) DockerfileAll(engine, owner, id string) (ContainerDockerfile, error) {
	path, err := s.containerDockerfilePath(engine, owner, id)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	return readContainerDockerfile(path)
}

func (s *ContainerService) WriteDockerfileAll(engine, owner, id, content string) (ContainerDockerfile, error) {
	path, err := s.containerDockerfilePath(engine, owner, id)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	return writeContainerDockerfile(path, content)
}

func (s *ContainerService) DockerfileCurrentUser(username, id string) (ContainerDockerfile, error) {
	if !isCurrentUser(username) || !allowedContainerID.MatchString(id) {
		return ContainerDockerfile{}, ErrContainerDockerfileDenied
	}
	output, err := s.runner.Run("podman", "inspect", id)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	path, err := containerDockerfilePathFromInspect(output, "podman", username)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	return readContainerDockerfile(path)
}

func (s *ContainerService) WriteDockerfileCurrentUser(username, id, content string) (ContainerDockerfile, error) {
	if !isCurrentUser(username) || !allowedContainerID.MatchString(id) {
		return ContainerDockerfile{}, ErrContainerDockerfileDenied
	}
	output, err := s.runner.Run("podman", "inspect", id)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	path, err := containerDockerfilePathFromInspect(output, "podman", username)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	return writeContainerDockerfile(path, content)
}

func (s *ContainerService) containerDockerfilePath(engine, owner, id string) (string, error) {
	if !allowedContainerID.MatchString(id) {
		return "", ErrContainerDockerfileDenied
	}
	output, err := s.runForOwner(engine, owner, "inspect", id)
	if err != nil {
		return "", err
	}
	return containerDockerfilePathFromInspect(output, engine, owner)
}

func containerDockerfilePathFromInspect(output []byte, engine, owner string) (string, error) {
	var inspected []map[string]any
	if json.Unmarshal(output, &inspected) != nil || len(inspected) == 0 {
		return "", ErrContainerDockerfileMissing
	}
	config, _ := inspected[0]["Config"].(map[string]any)
	labels, _ := config["Labels"].(map[string]any)
	path := strings.TrimSpace(fmt.Sprint(labels["mthan.dockerfile"]))
	if path == "" || path == "<nil>" {
		workingDirectory := strings.TrimSpace(fmt.Sprint(labels["com.docker.compose.project.working_dir"]))
		if workingDirectory != "" && workingDirectory != "<nil>" {
			path = filepath.Join(workingDirectory, "Dockerfile")
		}
	}
	path = filepath.Clean(path)
	if path == "." || !filepath.IsAbs(path) {
		return "", ErrContainerDockerfileMissing
	}
	if engine == "podman" {
		linuxUser, exists, lookupErr := HomeUser(owner)
		if lookupErr != nil || !exists || !pathWithin(path, linuxUser.Home) {
			return "", ErrContainerDockerfileDenied
		}
	}
	return path, nil
}

func readContainerDockerfile(path string) (ContainerDockerfile, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ContainerDockerfile{}, ErrContainerDockerfileMissing
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxAppConfigSize {
		return ContainerDockerfile{}, ErrContainerDockerfileDenied
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ContainerDockerfile{}, err
	}
	return ContainerDockerfile{Content: string(content), Path: path}, nil
}

func writeContainerDockerfile(path, content string) (ContainerDockerfile, error) {
	if len(content) > maxAppConfigSize || strings.ContainsRune(content, 0) {
		return ContainerDockerfile{}, ErrContainerDockerfileDenied
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ContainerDockerfile{}, ErrContainerDockerfileDenied
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mthan-dockerfile-*")
	if err != nil {
		return ContainerDockerfile{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		temporary.Close()
		return ContainerDockerfile{}, err
	}
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return ContainerDockerfile{}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return ContainerDockerfile{}, err
	}
	if err := temporary.Close(); err != nil {
		return ContainerDockerfile{}, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return ContainerDockerfile{}, err
	}
	return ContainerDockerfile{Content: content, Path: path}, nil
}

func pathWithin(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator))
}

func (s *ContainerService) runForOwner(engine, owner string, args ...string) ([]byte, error) {
	switch engine {
	case "docker":
		if owner != "root" && owner != "system" {
			return nil, errors.New("invalid Docker owner")
		}
		return s.runner.Run("docker", args...)
	case "podman":
		linuxUser, exists, err := HomeUser(owner)
		if err != nil || !exists || linuxUser.UID < 0 {
			return nil, errors.New("invalid Podman owner")
		}
		command := []string{
			"--user", linuxUser.Username, "--", "env", "HOME=" + linuxUser.Home,
			fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", linuxUser.UID), "podman",
		}
		return s.runner.Run("runuser", append(command, args...)...)
	default:
		return nil, errors.New("invalid container engine")
	}
}

func containerActionArgs(id, action string) ([]string, error) {
	if !allowedContainerID.MatchString(id) {
		return nil, errors.New("invalid container")
	}
	switch action {
	case "start", "stop", "restart", "kill":
		return []string{action, id}, nil
	case "remove":
		return []string{"rm", "-f", id}, nil // force: stop + remove in one step
	default:
		return nil, errors.New("invalid container action")
	}
}

func isCurrentUser(username string) bool {
	current, err := user.Current()
	return err == nil && current.Username == username
}

func (s *ContainerService) listDocker() []Container {
	output, err := s.runner.Run("docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return nil
	}
	return parseDockerContainers(output)
}

func (s *ContainerService) listRootlessPodman(linuxUser LinuxUser) []Container {
	output, err := s.runner.Run(
		"runuser", "--user", linuxUser.Username, "--", "env",
		"HOME="+linuxUser.Home, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", linuxUser.UID),
		"podman", "ps", "-a", "--format", "json",
	)
	if err != nil {
		return nil
	}
	return parsePodmanContainers(output, linuxUser.Username)
}

func parseDockerContainers(output []byte) []Container {
	var result []Container
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(line), &item) != nil {
			continue
		}
		labels := textField(item, "Labels")
		result = append(result, Container{
			ID: textField(item, "ID"), Name: textField(item, "Names"), Image: textField(item, "Image"),
			Command: textField(item, "Command"), Engine: "docker", Owner: "root",
			State: textField(item, "State"), Status: textField(item, "Status"),
			CreatedAt: textField(item, "CreatedAt"), Ports: splitDockerPorts(textField(item, "Ports")),
			Project:    composeLabel(labels, "com.docker.compose.project"),
			Service:    composeLabel(labels, "com.docker.compose.service"),
			WorkingDir: composeLabel(labels, "com.docker.compose.project.working_dir"),
			Deployed:   true,
		})
	}
	return result
}

func parsePodmanContainers(output []byte, owner string) []Container {
	var items []map[string]any
	if json.Unmarshal(output, &items) != nil {
		return []Container{}
	}
	result := make([]Container, 0, len(items))
	for _, item := range items {
		result = append(result, Container{
			ID: firstTextField(item, "Id", "ID"), Name: firstName(item), Image: firstTextField(item, "Image", "ImageName"),
			Command: joinedField(item["Command"]), Engine: "podman", Owner: owner,
			State: firstTextField(item, "State", "Status"), Status: firstTextField(item, "Status", "State"),
			CreatedAt: formatCreatedAt(item["CreatedAt"]), Ports: podmanPorts(item["Ports"]),
			Deployed: true,
		})
	}
	return result
}

func textField(item map[string]any, key string) string {
	if value, ok := item[key].(string); ok {
		return value
	}
	for candidate, raw := range item {
		if strings.EqualFold(candidate, key) {
			if value, ok := raw.(string); ok {
				return value
			}
		}
	}
	return ""
}

func firstTextField(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := textField(item, key); value != "" {
			return value
		}
	}
	return ""
}

func firstName(item map[string]any) string {
	if names, ok := item["Names"].([]any); ok && len(names) > 0 {
		return fmt.Sprint(names[0])
	}
	return firstTextField(item, "Names", "Name")
}

func joinedField(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case []any:
		parts := make([]string, 0, len(current))
		for _, part := range current {
			parts = append(parts, fmt.Sprint(part))
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func splitDockerPorts(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func podmanPorts(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(items))
	for _, raw := range items {
		port, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		hostIP := fmt.Sprint(port["host_ip"])
		if hostIP == "<nil>" || hostIP == "" {
			hostIP = "0.0.0.0"
		}
		hostPort := numberText(port["host_port"])
		containerPort := numberText(port["container_port"])
		protocol := fmt.Sprint(port["protocol"])
		if protocol == "<nil>" || protocol == "" {
			protocol = "tcp"
		}
		if hostPort != "" && containerPort != "" {
			result = append(result, hostIP+":"+hostPort+"->"+containerPort+"/"+protocol)
		} else if containerPort != "" {
			result = append(result, containerPort+"/"+protocol)
		}
	}
	return result
}

func numberText(value any) string {
	switch number := value.(type) {
	case float64:
		return fmt.Sprintf("%.0f", number)
	case string:
		return number
	default:
		return ""
	}
}

func formatCreatedAt(value any) string {
	if value == nil {
		return ""
	}
	if seconds, ok := value.(float64); ok && seconds > 0 {
		return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
	}
	return fmt.Sprint(value)
}

func sortContainers(containers []Container) {
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Owner == containers[j].Owner {
			if containers[i].Engine == containers[j].Engine {
				return containers[i].Name < containers[j].Name
			}
			return containers[i].Engine < containers[j].Engine
		}
		return containers[i].Owner < containers[j].Owner
	})
}
