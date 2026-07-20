package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"
)

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
	current, err := user.Current()
	if err != nil || current.Username != username {
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
