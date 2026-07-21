package services

import (
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
