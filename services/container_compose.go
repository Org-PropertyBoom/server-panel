package services

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Compose-project awareness for the Containers page.
//
// The panel lists RUNTIME objects (docker ps), but what the operator actually
// manages are SERVICES that should-or-shouldn't be running. A `docker compose
// down` removes the container entirely — it then vanishes from `docker ps -a`
// and from the panel, leaving no way to see (or restart) what was turned off.
//
// So we join two sources:
//   - runtime: the containers docker reports (with their compose labels), and
//   - definitions: the `services:` keys parsed from on-disk compose files.
//
// A service defined on disk with no matching container is "not deployed" — it
// gets a synthetic row (Deployed=false) so it's visible and startable. The scan
// is read-only and cached; the ONLY mutation is the explicit Start action.

// composeLabel pulls one label value out of the comma-separated "k=v,k=v" string
// that `docker ps --format {{json .}}` puts in the "Labels" field. Compose label
// values (project name, service name, working dir) never contain commas, so a
// plain comma split is safe.
func composeLabel(labels, key string) string {
	for _, kv := range strings.Split(labels, ",") {
		if i := strings.IndexByte(kv, '='); i > 0 && kv[:i] == key {
			return strings.TrimSpace(kv[i+1:])
		}
	}
	return ""
}

var composeFileNames = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

type composeProject struct {
	Project    string   // compose project name (dir basename — the compose default)
	WorkingDir string   // absolute project directory
	Services   []string // top-level `services:` keys
}

// composeScanRoots are the directories scanned for compose projects. Defaults to
// the deploy checkout root; override with COMPOSE_SCAN_DIRS (comma-separated).
func composeScanRoots() []string {
	if v := strings.TrimSpace(os.Getenv("COMPOSE_SCAN_DIRS")); v != "" {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"/home/server/htdocs"}
}

var (
	composeScanMu   sync.Mutex
	composeScanAt   time.Time
	composeScanData map[string]composeProject
)

const composeScanTTL = 30 * time.Second

// discoverComposeProjects returns the compose projects found on disk, keyed by
// absolute working directory. It scans the base roots plus any extra dirs (the
// working_dir labels of live containers, so relocated projects aren't missed).
// The result is cached for composeScanTTL to avoid re-reading files every render.
func discoverComposeProjects(extraDirs []string) map[string]composeProject {
	composeScanMu.Lock()
	defer composeScanMu.Unlock()
	if composeScanData != nil && time.Since(composeScanAt) < composeScanTTL {
		return composeScanData
	}

	dirs := map[string]bool{}
	for _, base := range composeScanRoots() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs[filepath.Join(base, e.Name())] = true
			}
		}
	}
	for _, d := range extraDirs {
		if d = strings.TrimSpace(d); d != "" {
			dirs[filepath.Clean(d)] = true
		}
	}

	out := map[string]composeProject{}
	for dir := range dirs {
		for _, fn := range composeFileNames {
			data, err := os.ReadFile(filepath.Join(dir, fn))
			if err != nil {
				continue
			}
			services := parseComposeServices(data)
			if len(services) == 0 {
				continue
			}
			out[filepath.Clean(dir)] = composeProject{
				Project:    filepath.Base(dir),
				WorkingDir: filepath.Clean(dir),
				Services:   services,
			}
			break // first matching compose file wins
		}
	}

	composeScanData = out
	composeScanAt = time.Now()
	return out
}

// parseComposeServices extracts the top-level `services:` keys from a compose
// file. It's a deliberately small structural parser (not a full YAML load): find
// the `services:` block at column 0, then collect the keys at the first indent
// level under it, stopping at the next top-level key. That's all we need — we
// never interpret the service bodies. Handles any consistent indent width.
func parseComposeServices(content []byte) []string {
	var services []string
	inServices := false
	serviceIndent := -1
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if !inServices {
			if indent == 0 && (trimmed == "services:" || trimmed == "services :") {
				inServices = true
			}
			continue
		}
		if indent == 0 {
			break // reached the next top-level key (volumes:, networks:, ...)
		}
		if serviceIndent == -1 {
			serviceIndent = indent
		}
		// A service key sits at the first indent level and is a bare "name:"
		// (a mapping key with no inline value). Deeper lines are its properties.
		if indent == serviceIndent && strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			if name != "" {
				services = append(services, name)
			}
		}
	}
	return services
}

// ListWithCompose returns the runtime container list joined against on-disk
// compose definitions: every real container (Deployed=true) plus a synthetic
// "not deployed" row (Deployed=false) for each compose service that currently
// has no container. Rows are grouped by project (standalone/non-compose last)
// so same-project services sit together.
func (s *ContainerService) ListWithCompose() []Container {
	real := s.ListAll()

	deployed := map[string]bool{}
	var extraDirs []string
	for i := range real {
		if real[i].WorkingDir != "" && real[i].Service != "" {
			dir := filepath.Clean(real[i].WorkingDir)
			deployed[dir+"\x00"+real[i].Service] = true
			extraDirs = append(extraDirs, dir)
		}
	}

	projects := discoverComposeProjects(extraDirs)
	for dir, proj := range projects {
		for _, svc := range proj.Services {
			if deployed[dir+"\x00"+svc] {
				continue
			}
			real = append(real, Container{
				Name:       svc,
				Engine:     "docker",
				Owner:      "root",
				State:      "not_deployed",
				Status:     "Not deployed",
				Project:    proj.Project,
				Service:    svc,
				WorkingDir: dir,
				Deployed:   false,
			})
		}
	}

	sort.SliceStable(real, func(i, j int) bool {
		pi, pj := real[i].Project, real[j].Project
		if pi != pj {
			if pi == "" {
				return false // standalone / non-compose sorts last
			}
			if pj == "" {
				return true
			}
			return pi < pj
		}
		return real[i].Name < real[j].Name
	})
	return real
}

// ComposeUp starts a not-yet-deployed compose service: `docker compose up -d
// <service>` in its project directory. It re-validates against the discovered
// projects (the workingDir must be a known compose project and the service must
// be defined there) so the client can't drive an arbitrary `compose up` in an
// arbitrary directory. Root Docker only; mutating — the caller confirms first.
func (s *ContainerService) ComposeUp(workingDir, service string) (string, error) {
	workingDir = filepath.Clean(workingDir)
	if !filepath.IsAbs(workingDir) {
		return "", errors.New("invalid compose working directory")
	}
	if !allowedComposeService.MatchString(service) {
		return "", errors.New("invalid compose service name")
	}

	proj, ok := discoverComposeProjects(nil)[workingDir]
	if !ok {
		return "", errors.New("unknown compose project — no compose file found in that directory")
	}
	defined := false
	for _, sv := range proj.Services {
		if sv == service {
			defined = true
			break
		}
	}
	if !defined {
		return "", errors.New("service is not defined in this compose project")
	}

	out, err := runContainerCommand(workingDir, buildTimeout(), "docker", "compose", "up", "-d", service)
	return string(out), err
}
