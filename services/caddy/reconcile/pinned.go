package reconcile

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
)

// PinnedHost is a hand-written static-block host derived from the ACTUAL main
// Caddyfile (adapted hosts minus the folder routes we render) — ground truth for
// what is really pinned, with its reverse_proxy upstream(s) from the adapted config.
type PinnedHost struct {
	Host      string
	Upstreams []string
}

// PinnedFromCaddyfile adapts the main Caddyfile and returns the hosts that are NOT
// folder routes (i.e. the real hand-written static blocks), each with its
// reverse_proxy dial upstream(s). This is DISPLAY/DRIFT ground truth — it does NOT
// feed the reload invariant (assertDashboardPresent stays an external config
// declaration, never derived from the file it validates).
func (e *Engine) PinnedFromCaddyfile() ([]PinnedHost, error) {
	if e.adapter == nil {
		return nil, errors.New("no adapter (read-only engine)")
	}
	main, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		return nil, err
	}
	adapted, _, err := e.adapter.Adapt(main, e.cfg.MainCaddyfile)
	if err != nil {
		return nil, err
	}
	adaptedHosts, err := hostSet(adapted)
	if err != nil {
		return nil, err
	}
	folder, err := e.RenderedHosts()
	if err != nil {
		return nil, err
	}
	folderSet := make(map[string]bool, len(folder))
	for _, h := range folder {
		folderSet[strings.ToLower(strings.TrimSpace(h))] = true
	}
	ups := hostUpstreams(adapted)

	var out []PinnedHost
	for h := range adaptedHosts {
		if folderSet[h] {
			continue // a folder route we render — not a hand-written static block
		}
		out = append(out, PinnedHost{Host: h, Upstreams: ups[h]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out, nil
}

// hostUpstreams maps each host in a Caddy JSON config to its reverse_proxy dial
// upstreams — for each server route, its match hosts joined to the dials found
// anywhere in that route's handle subtree.
func hostUpstreams(cfgJSON []byte) map[string][]string {
	out := map[string][]string{}
	var root map[string]any
	if json.Unmarshal(cfgJSON, &root) != nil {
		return out
	}
	servers, _ := dig(root, "apps", "http", "servers").(map[string]any)
	for _, s := range servers {
		srv, _ := s.(map[string]any)
		routes, _ := srv["routes"].([]any)
		for _, r := range routes {
			route, _ := r.(map[string]any)
			hosts := matchHosts(route["match"])
			if len(hosts) == 0 {
				continue
			}
			dials := collectDials(route["handle"])
			for _, h := range hosts {
				h = strings.ToLower(strings.TrimSpace(h))
				for _, d := range dials {
					if !sliceHas(out[h], d) {
						out[h] = append(out[h], d)
					}
				}
			}
		}
	}
	return out
}

func matchHosts(match any) []string {
	arr, _ := match.([]any)
	var out []string
	for _, m := range arr {
		mm, _ := m.(map[string]any)
		hs, _ := mm["host"].([]any)
		for _, h := range hs {
			if s, ok := h.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// collectDials recursively finds every reverse_proxy handler's upstreams[].dial in
// a route handle subtree (subroutes nest the reverse_proxy under handle→routes).
func collectDials(handle any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["handler"] == "reverse_proxy" {
				if arr, ok := t["upstreams"].([]any); ok {
					for _, u := range arr {
						if um, ok := u.(map[string]any); ok {
							if d, ok := um["dial"].(string); ok && d != "" {
								out = append(out, d)
							}
						}
					}
				}
			}
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(handle)
	return out
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func sliceHas(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
