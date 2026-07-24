package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// BuildStamp is a container's deployed build identity, resolved from its public
// route (NOT from image labels — those carry the base image's revision, not the
// app's). Found is false when no stamp could be resolved.
type BuildStamp struct {
	Commit     string `json:"commit,omitempty"`
	DeployedAt string `json:"deployedAt,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Source     string `json:"source,omitempty"` // "header" | "up"
	Found      bool   `json:"found"`
}

var (
	buildStampMu     sync.Mutex
	buildStampCache  = map[string]buildStampEntry{}
	buildStampTTL    = 60 * time.Second
	buildStampClient = &http.Client{Timeout: 3 * time.Second}
	// __BUILD__ = { ... } — a flat JSON object (no nested braces).
	buildJSONRe = regexp.MustCompile(`__BUILD__\s*=\s*(\{[^{}]*\})`)
	hostnameRe  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)
)

type buildStampEntry struct {
	stamp BuildStamp
	at    time.Time
}

// isPublicRouteHost restricts stamp fetches to real public domains — rejects
// localhost / private-range literals so this can't be turned into an SSRF probe.
func isPublicRouteHost(host string) bool {
	if !hostnameRe.MatchString(host) || !strings.Contains(host, ".") || host == "localhost" {
		return false
	}
	for _, p := range []string{"127.", "10.", "169.254.", "192.168.", "172.16.", "172.17.", "0."} {
		if strings.HasPrefix(host, p) {
			return false
		}
	}
	return true
}

// ResolveBuildStamp returns the cached-or-freshly-fetched stamp for one route host.
func ResolveBuildStamp(ctx context.Context, host string) BuildStamp {
	host = strings.ToLower(strings.TrimSpace(host))
	if !isPublicRouteHost(host) {
		return BuildStamp{}
	}
	buildStampMu.Lock()
	e, ok := buildStampCache[host]
	buildStampMu.Unlock()
	if ok && time.Since(e.at) < buildStampTTL {
		return e.stamp
	}
	stamp := fetchBuildStamp(ctx, host)
	buildStampMu.Lock()
	buildStampCache[host] = buildStampEntry{stamp, time.Now()}
	buildStampMu.Unlock()
	return stamp
}

// ResolveBuildStamps resolves many hosts concurrently (capped), returning only the
// ones that produced a stamp. Deduplicates hosts.
func ResolveBuildStamps(ctx context.Context, hosts []string) map[string]BuildStamp {
	out := map[string]BuildStamp{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	seen := map[string]bool{}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			sem <- struct{}{}
			s := ResolveBuildStamp(ctx, host)
			<-sem
			if s.Found {
				mu.Lock()
				out[host] = s
				mu.Unlock()
			}
		}(h)
	}
	wg.Wait()
	return out
}

// fetchBuildStamp tries the route in priority order: (a) x-build-commit response
// header, else (b) window.__BUILD__ on /up. Any failure → empty (not found).
func fetchBuildStamp(ctx context.Context, host string) BuildStamp {
	base := "https://" + host

	if req, err := http.NewRequestWithContext(ctx, http.MethodHead, base+"/", nil); err == nil {
		if resp, err := buildStampClient.Do(req); err == nil {
			resp.Body.Close()
			if c := strings.TrimSpace(resp.Header.Get("x-build-commit")); c != "" {
				return BuildStamp{
					Commit:     c,
					DeployedAt: strings.TrimSpace(resp.Header.Get("x-build-deployed-at")),
					Ref:        strings.TrimSpace(resp.Header.Get("x-build-ref")),
					Source:     "header",
					Found:      true,
				}
			}
		}
	}

	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/up", nil); err == nil {
		if resp, err := buildStampClient.Do(req); err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
			if m := buildJSONRe.FindSubmatch(body); m != nil {
				var raw struct {
					Commit     string `json:"commit"`
					DeployedAt string `json:"deployedAt"`
					Ref        string `json:"ref"`
					Repo       string `json:"repo"`
				}
				if json.Unmarshal(m[1], &raw) == nil && raw.Commit != "" {
					return BuildStamp{
						Commit:     raw.Commit,
						DeployedAt: raw.DeployedAt,
						Ref:        raw.Ref,
						Repo:       raw.Repo,
						Source:     "up",
						Found:      true,
					}
				}
			}
		}
	}

	return BuildStamp{}
}
