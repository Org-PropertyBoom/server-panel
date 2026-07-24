package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	SizeRootFs    *int64             `json:"sizeRootFs,omitempty"` // total bytes incl. image layers; nil if not computed
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
	return parseContainerDetails(output, engine, owner)
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
	Ports   []string `json:"ports"`   // "host:container" or "host:container/proto"
	Env     []string `json:"env"`     // "KEY=VALUE"
	Volumes []string `json:"volumes"` // "src:dst" or "src:dst:ro"
	Restart string   `json:"restart"` // no | always | unless-stopped | on-failure
}

var (
	allowedComposeService = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	allowedImageRef       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)
	allowedPortMapping    = regexp.MustCompile(`^(\d{1,5}:)?\d{1,5}(/(tcp|udp))?$`)
	allowedEnvKey         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	allowedRestartPolicy  = map[string]bool{"no": true, "always": true, "unless-stopped": true, "on-failure": true}
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
func (s *ContainerService) CreateContainer(spec ContainerCreateSpec) (string, error) {
	image := strings.TrimSpace(spec.Image)
	if image == "" || strings.HasPrefix(image, "-") || !allowedImageRef.MatchString(image) {
		return "", errors.New("a valid image is required")
	}
	args := []string{"run", "-d"}
	if name := strings.TrimSpace(spec.Name); name != "" {
		if !allowedContainerID.MatchString(name) {
			return "", errors.New("invalid container name")
		}
		args = append(args, "--name", name)
	}
	restart := strings.TrimSpace(spec.Restart)
	if restart == "" {
		restart = "unless-stopped"
	}
	if !allowedRestartPolicy[restart] {
		return "", errors.New("invalid restart policy")
	}
	args = append(args, "--restart", restart)
	for _, p := range spec.Ports {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !allowedPortMapping.MatchString(p) {
			return "", fmt.Errorf("invalid port mapping %q (use host:container)", p)
		}
		args = append(args, "-p", p)
	}
	for _, e := range spec.Env {
		if e = strings.TrimSpace(e); e == "" {
			continue
		}
		eq := strings.IndexByte(e, '=')
		if eq <= 0 || !allowedEnvKey.MatchString(e[:eq]) {
			return "", fmt.Errorf("invalid environment variable %q (use KEY=VALUE)", e)
		}
		args = append(args, "-e", e)
	}
	for _, v := range spec.Volumes {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if strings.HasPrefix(v, "-") || !strings.Contains(v, ":") {
			return "", fmt.Errorf("invalid volume %q (use src:dst)", v)
		}
		args = append(args, "-v", v)
	}
	args = append(args, image)
	out, err := runContainerCommand("", 5*time.Minute, "docker", args...)
	return string(out), err
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
	if action != "start" && action != "stop" && action != "restart" {
		return nil, errors.New("invalid container action")
	}
	return []string{action, id}, nil
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
		result = append(result, Container{
			ID: textField(item, "ID"), Name: textField(item, "Names"), Image: textField(item, "Image"),
			Command: textField(item, "Command"), Engine: "docker", Owner: "root",
			State: textField(item, "State"), Status: textField(item, "Status"),
			CreatedAt: textField(item, "CreatedAt"), Ports: splitDockerPorts(textField(item, "Ports")),
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
