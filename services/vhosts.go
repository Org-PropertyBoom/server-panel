package services

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrVHostNotFound = errors.New("vhost not found")

type PublicPort struct {
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	Server    string `json:"server"`
	Listening bool   `json:"listening"`
}

type VHost struct {
	Hostname    string   `json:"hostname"`
	Aliases     []string `json:"aliases"`
	Server      string   `json:"server"`
	Listen      []string `json:"listen"`
	TLS         bool     `json:"tls"`
	Upstreams   []string `json:"upstreams"`
	Roots       []string `json:"roots"`
	ConfigFiles []string `json:"configFiles"`
}

type VHostSummary struct {
	Hostname string   `json:"hostname"`
	Aliases  []string `json:"aliases"`
	Server   string   `json:"server"`
	Listen   []string `json:"listen"`
	TLS      bool     `json:"tls"`
}

type VHostStatus struct {
	Proxy       string       `json:"proxy"`
	PublicPorts []PublicPort `json:"publicPorts"`
	VHosts      int          `json:"vhosts"`
}

type commandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type timedCommandRunner struct{}

func (timedCommandRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type VHostService struct {
	runner commandRunner
}

func NewVHostService() *VHostService {
	return &VHostService{runner: timedCommandRunner{}}
}

func (s *VHostService) Status() VHostStatus {
	vhosts := s.List()
	ports := s.publicPorts()
	proxy := "caddy"
	return VHostStatus{Proxy: proxy, PublicPorts: ports, VHosts: len(vhosts)}
}

func (s *VHostService) List() []VHost {
	var all []VHost
	if output, err := s.runner.Run("caddy", "adapt", "--config", "/etc/caddy/Caddyfile"); err == nil {
		all = append(all, parseCaddyVHosts(output, "/etc/caddy/Caddyfile")...)
	}
	all = mergeVHosts(all)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Hostname == all[j].Hostname {
			return all[i].Server < all[j].Server
		}
		return all[i].Hostname < all[j].Hostname
	})
	return all
}

func (s *VHostService) Summaries() []VHostSummary {
	vhosts := s.List()
	result := make([]VHostSummary, 0, len(vhosts))
	for _, host := range vhosts {
		result = append(result, VHostSummary{
			Hostname: host.Hostname, Aliases: host.Aliases, Server: host.Server,
			Listen: host.Listen, TLS: host.TLS,
		})
	}
	return result
}

func (s *VHostService) Get(hostname string) (VHost, error) {
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	for _, host := range s.List() {
		if strings.EqualFold(host.Hostname, hostname) || containsFold(host.Aliases, hostname) {
			return host, nil
		}
	}
	return VHost{}, ErrVHostNotFound
}

func (s *VHostService) publicPorts() []PublicPort {
	ports := []PublicPort{
		{Port: 80, Protocol: "http", Server: "unknown"},
		{Port: 443, Protocol: "https", Server: "unknown"},
	}
	output, err := s.runner.Run("ss", "-H", "-ltnp")
	if err != nil {
		return ports
	}
	for i := range ports {
		server, listening := serverForPort(string(output), ports[i].Port)
		ports[i].Server = server
		ports[i].Listening = listening
	}
	return ports
}

func serverForPort(output string, port int) (string, bool) {
	portPattern := regexp.MustCompile(`(?:\]:|:)(` + strconv.Itoa(port) + `)\s`)
	for _, line := range strings.Split(output, "\n") {
		if !portPattern.MatchString(line + " ") {
			continue
		}
		lower := strings.ToLower(line)
		for _, candidate := range []struct{ process, server string }{
			{"caddy", "caddy"}, {"nginx", "nginx"}, {"apache2", "apache"}, {"httpd", "apache"},
		} {
			if strings.Contains(lower, `"`+candidate.process+`"`) {
				return candidate.server, true
			}
		}
		return "unknown", true
	}
	return "unknown", false
}

var nginxFilePattern = regexp.MustCompile(`(?m)^# configuration file ([^:]+):`)

func parseNginxVHosts(output string) []VHost {
	var result []VHost
	for _, block := range nginxServerBlocks(output) {
		body := output[block[1]:block[2]]
		names := directiveValues(body, "server_name")
		if len(names) == 0 {
			continue
		}
		listen := directiveLines(body, "listen")
		file := lastNginxFile(output[:block[0]])
		validNames := make([]string, 0, len(names))
		for _, hostname := range names {
			if hostname != "_" && !strings.HasPrefix(hostname, "~") {
				validNames = append(validNames, hostname)
			}
		}
		if len(validNames) > 0 {
			result = append(result, VHost{
				Hostname: validNames[0], Aliases: validNames[1:], Server: "nginx", Listen: listen,
				TLS: listensTLS(listen), Upstreams: directiveValues(body, "proxy_pass"),
				Roots: directiveValues(body, "root"), ConfigFiles: nonEmptySlice(file),
			})
		}
	}
	return result
}

func nginxServerBlocks(output string) [][3]int {
	startPattern := regexp.MustCompile(`\bserver\s*\{`)
	var blocks [][3]int
	for offset := 0; offset < len(output); {
		location := startPattern.FindStringIndex(output[offset:])
		if location == nil {
			break
		}
		start := offset + location[0]
		bodyStart := offset + location[1]
		depth := 1
		quote := byte(0)
		escaped := false
		end := bodyStart
		for ; end < len(output) && depth > 0; end++ {
			char := output[end]
			if escaped {
				escaped = false
				continue
			}
			if quote != 0 {
				if char == '\\' {
					escaped = true
				} else if char == quote {
					quote = 0
				}
				continue
			}
			if char == '\'' || char == '"' {
				quote = char
				continue
			}
			switch char {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth == 0 {
			blocks = append(blocks, [3]int{start, bodyStart, end - 1})
		}
		offset = end
	}
	return blocks
}

func directiveValues(body, name string) []string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+([^;]+);`)
	var values []string
	for _, match := range re.FindAllStringSubmatch(body, -1) {
		values = append(values, strings.Fields(strings.TrimSpace(match[1]))...)
	}
	return uniqueStrings(values)
}

func directiveLines(body, name string) []string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+([^;]+);`)
	var values []string
	for _, match := range re.FindAllStringSubmatch(body, -1) {
		values = append(values, strings.TrimSpace(match[1]))
	}
	return uniqueStrings(values)
}

func lastNginxFile(prefix string) string {
	matches := nginxFilePattern.FindAllStringSubmatch(prefix, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

var apacheHostPattern = regexp.MustCompile(`(?m)port\s+(\d+)\s+namevhost\s+(\S+)\s+\((.+):\d+\)`)

func parseApacheVHosts(output string) []VHost {
	var result []VHost
	for _, match := range apacheHostPattern.FindAllStringSubmatch(output, -1) {
		port := match[1]
		result = append(result, VHost{
			Hostname: match[2], Server: "apache", Listen: []string{":" + port},
			TLS: port == "443", ConfigFiles: []string{match[3]},
		})
	}
	return result
}

func parseCaddyVHosts(output []byte, configFile string) []VHost {
	var config map[string]any
	if json.Unmarshal(output, &config) != nil {
		return nil
	}
	httpApp := nestedMap(config, "apps", "http")
	servers, _ := httpApp["servers"].(map[string]any)
	var result []VHost
	for _, rawServer := range servers {
		server, _ := rawServer.(map[string]any)
		listen := stringArray(server["listen"])
		routes, _ := server["routes"].([]any)
		for _, rawRoute := range routes {
			route, _ := rawRoute.(map[string]any)
			hosts := caddyRouteHosts(route)
			if len(hosts) == 0 {
				continue
			}
			upstreams, roots := caddyRouteTargets(route)
			result = append(result, VHost{
				Hostname: hosts[0], Aliases: hosts[1:], Server: "caddy", Listen: listen,
				TLS: listensTLS(listen), Upstreams: upstreams, Roots: roots,
				ConfigFiles: []string{configFile},
			})
		}
	}
	return result
}

func caddyRouteHosts(route map[string]any) []string {
	var hosts []string
	matchers, _ := route["match"].([]any)
	for _, raw := range matchers {
		matcher, _ := raw.(map[string]any)
		hosts = append(hosts, stringArray(matcher["host"])...)
	}
	return uniqueStrings(hosts)
}

func caddyRouteTargets(value any) ([]string, []string) {
	var upstreams, roots []string
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			if current["handler"] == "reverse_proxy" {
				for _, raw := range anyArray(current["upstreams"]) {
					upstream, _ := raw.(map[string]any)
					if dial, ok := upstream["dial"].(string); ok {
						upstreams = append(upstreams, dial)
					}
				}
			}
			if current["handler"] == "file_server" {
				if root, ok := current["root"].(string); ok {
					roots = append(roots, root)
				}
			}
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	return uniqueStrings(upstreams), uniqueStrings(roots)
}

func nestedMap(value map[string]any, keys ...string) map[string]any {
	current := value
	for _, key := range keys {
		next, _ := current[key].(map[string]any)
		current = next
	}
	return current
}

func anyArray(value any) []any { result, _ := value.([]any); return result }
func stringArray(value any) []string {
	var result []string
	for _, item := range anyArray(value) {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func listensTLS(listen []string) bool {
	for _, value := range listen {
		if strings.Contains(value, "443") || strings.EqualFold(value, "ssl") {
			return true
		}
	}
	return false
}

func mergeVHosts(input []VHost) []VHost {
	result := make([]VHost, 0, len(input))
	index := map[string]int{}
	for _, host := range input {
		host.Hostname = strings.ToLower(strings.TrimSuffix(host.Hostname, "."))
		key := host.Server + "\x00" + host.Hostname
		if existing, ok := index[key]; ok {
			current := &result[existing]
			current.Aliases = uniqueStrings(append(current.Aliases, host.Aliases...))
			current.Listen = uniqueStrings(append(current.Listen, host.Listen...))
			current.Upstreams = uniqueStrings(append(current.Upstreams, host.Upstreams...))
			current.Roots = uniqueStrings(append(current.Roots, host.Roots...))
			current.ConfigFiles = uniqueStrings(append(current.ConfigFiles, host.ConfigFiles...))
			current.TLS = current.TLS || host.TLS
			continue
		}
		index[key] = len(result)
		result = append(result, host)
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func nonEmptySlice(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}
