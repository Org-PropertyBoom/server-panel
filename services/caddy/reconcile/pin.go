package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ppt/server-panel/services/caddy/render"
)

// A route is the SAME object (domain→backend) whether it's a hand-written static
// Caddyfile block or a platform_hosts DB row rendered to a folder file. Pin/Unpin
// swap it between the two. Both are single-artifact-at-a-time safe: they mutate the
// Caddyfile + folder, re-adapt, and DIFF-ASSERT that the served host set is
// UNCHANGED (the host just moved source); anything else → abort + restore. A static
// block and a folder file for the SAME host can't coexist (Caddy adapt rejects the
// duplicate), so the swap is atomic by construction. Gated + reload via /load.

// PinStaticBlock moves a rendered folder route to a static Caddyfile block: removes
// the folder file and appends `host { reverse_proxy target }` to the main Caddyfile.
// The caller (service) removes the DB row AFTER this succeeds so no reconcile
// re-renders the folder file (which would duplicate the block).
func (e *Engine) PinStaticBlock(ctx context.Context, host, target string) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	start := e.now()
	res := Result{}
	host = strings.ToLower(strings.TrimSpace(host))
	target = strings.TrimSpace(target)
	if host == "" || target == "" {
		res.Error = "host and target are required"
		return res, errors.New(res.Error)
	}
	if reason := e.protectedReason(host); reason != "" {
		res.Error = reason
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if e.adapter == nil || e.reloader == nil {
		res.Error = "pin: read-only engine"
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	original, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "read Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	adaptedBefore, _, err := e.adapter.Adapt(original, e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "adapt current Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	hostsBefore, _ := hostSet(adaptedBefore)
	if !hostsBefore[host] {
		res.Error = fmt.Sprintf("%q is not currently served", host)
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	fileName := render.FileName(host)
	fileBytes, ferr := os.ReadFile(filepath.Join(e.cfg.VhostsDir, fileName))
	if ferr != nil {
		res.Error = fmt.Sprintf("%q has no rendered folder file to pin (%v)", host, ferr)
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	backupPath, berr := e.backupCaddyfile(original)
	if berr != nil {
		res.Error = "backup Caddyfile: " + berr.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	res.BackupPath = backupPath

	edited := append(append([]byte{}, original...), []byte(fmt.Sprintf("\n%s {\n\treverse_proxy %s\n}\n", host, target))...)
	restore := func() {
		_ = os.WriteFile(e.cfg.MainCaddyfile, original, 0o644)
		_, _ = e.dir.Write(fileName, string(fileBytes))
	}
	if _, rerr := e.dir.Remove(fileName); rerr != nil {
		res.Error = "remove folder file: " + rerr.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if werr := os.WriteFile(e.cfg.MainCaddyfile, edited, 0o644); werr != nil {
		_, _ = e.dir.Write(fileName, string(fileBytes)) // re-add file; Caddyfile untouched
		res.Error = "write Caddyfile: " + werr.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	if res2, ok := e.validateAndReload(ctx, edited, hostsBefore, restore, &res, start); !ok {
		return res2, errors.New(res2.Error)
	}
	res.Written = []string{host + " (pinned → static block)"}
	res.DurationMS = e.since(start)
	return res, nil
}

// UnpinStaticBlock moves a static block to a rendered folder route: renders the
// folder file (upstream read from the block) and removes the block. Returns the
// target so the caller creates the platform_hosts row (making it a managed route).
func (e *Engine) UnpinStaticBlock(ctx context.Context, host string) (Result, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	start := e.now()
	res := Result{}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		res.Error = "host is required"
		return res, "", errors.New(res.Error)
	}
	if reason := e.protectedReason(host); reason != "" {
		res.Error = "refusing to unpin — " + reason + " (the panel/dashboard's own domain must stay pinned)"
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	if e.adapter == nil || e.reloader == nil {
		res.Error = "unpin: read-only engine"
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}

	original, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "read Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	adaptedBefore, _, err := e.adapter.Adapt(original, e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "adapt current Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	hostsBefore, _ := hostSet(adaptedBefore)
	if !hostsBefore[host] {
		res.Error = fmt.Sprintf("%q is not currently served", host)
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	fileName := render.FileName(host)
	if _, ferr := os.Stat(filepath.Join(e.cfg.VhostsDir, fileName)); ferr == nil {
		res.Error = fmt.Sprintf("%q is already a rendered folder route, not a static block", host)
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	dials := hostUpstreams(adaptedBefore)[host]
	if len(dials) == 0 {
		res.Error = fmt.Sprintf("could not read the static block's reverse_proxy upstream for %q — unpin only supports a reverse_proxy block", host)
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	target := dials[0]
	_, contents, rerr := render.Render(render.Host{Host: host, Kind: render.KindSystem, Target: target, Encode: e.cfg.EncodeFormats()})
	if rerr != nil {
		res.Error = "render folder file: " + rerr.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	edited, ok := removeCaddyBlock(original, host)
	if !ok {
		res.Error = fmt.Sprintf("could not find a static block for %q", host)
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}

	backupPath, berr := e.backupCaddyfile(original)
	if berr != nil {
		res.Error = "backup Caddyfile: " + berr.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	res.BackupPath = backupPath

	restore := func() {
		_ = os.WriteFile(e.cfg.MainCaddyfile, original, 0o644)
		_, _ = e.dir.Remove(fileName)
	}
	if _, werr := e.dir.Write(fileName, contents); werr != nil {
		res.Error = "write folder file: " + werr.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}
	if werr := os.WriteFile(e.cfg.MainCaddyfile, edited, 0o644); werr != nil {
		_, _ = e.dir.Remove(fileName) // undo file; Caddyfile untouched
		res.Error = "write Caddyfile: " + werr.Error()
		res.DurationMS = e.since(start)
		return res, "", errors.New(res.Error)
	}

	if res2, ok := e.validateAndReload(ctx, edited, hostsBefore, restore, &res, start); !ok {
		return res2, target, errors.New(res2.Error)
	}
	res.Written = []string{host + " (unpinned → folder route)"}
	res.DurationMS = e.since(start)
	return res, target, nil
}

// validateAndReload adapts the edited Caddyfile, diff-asserts the served host set
// is UNCHANGED (a Pin/Unpin only moves the host's source, never adds/drops a host),
// asserts the dashboard survives, and reloads. On ANY failure it runs restore() and
// populates res.Error. Returns (res, ok).
func (e *Engine) validateAndReload(ctx context.Context, edited []byte, hostsBefore map[string]bool, restore func(), res *Result, start time.Time) (Result, bool) {
	adaptedAfter, warnings, aerr := e.adapter.Adapt(edited, e.cfg.MainCaddyfile)
	if aerr != nil {
		restore()
		res.Error = "adapt after edit: " + aerr.Error()
		res.DurationMS = e.since(start)
		return *res, false
	}
	res.AdaptWarnings = warnings
	hostsAfter, _ := hostSet(adaptedAfter)
	if changed := symmetricDiff(hostsBefore, hostsAfter); len(changed) > 0 {
		restore()
		sort.Strings(changed)
		res.BlockedDrops = changed
		res.Error = "the served host set changed unexpectedly (a pin/unpin must only move the host's source): " + strings.Join(changed, ", ")
		res.DurationMS = e.since(start)
		return *res, false
	}
	if derr := e.assertDashboardPresent(adaptedAfter); derr != nil {
		restore()
		res.Error = derr.Error()
		res.DurationMS = e.since(start)
		return *res, false
	}
	if lerr := e.reloader.Load(ctx, adaptedAfter); lerr != nil {
		restore()
		res.Error = "reload: " + lerr.Error() + " (restored)"
		res.DurationMS = e.since(start)
		return *res, false
	}
	res.Reloaded = true
	return *res, true
}

// protectedReason returns a non-empty refusal reason if host is a protected
// (dashboard/panel) domain, else "".
func (e *Engine) protectedReason(host string) string {
	for _, p := range e.cfg.ProtectedHosts() {
		if strings.EqualFold(p, host) {
			return fmt.Sprintf("%q is a protected domain", host)
		}
	}
	return ""
}

// symmetricDiff returns the hosts present in exactly one of the two sets.
func symmetricDiff(a, b map[string]bool) []string {
	var out []string
	for h := range a {
		if !b[h] {
			out = append(out, h)
		}
	}
	for h := range b {
		if !a[h] {
			out = append(out, h)
		}
	}
	return out
}
