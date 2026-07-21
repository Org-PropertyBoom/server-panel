package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RemoveStaticBlock surgically removes ONE host's hand-written static block from
// the main Caddyfile — server-panel's only write to that operator-owned file, used
// to clear a stale "Pinned · unmanaged" block from the UI. FULL OUTAGE DISCIPLINE:
//
//  1. adapt the CURRENT Caddyfile (baseline host set).
//  2. refuse if host is protected (dashboard/panel), not actually present, or is a
//     rendered folder route (DB-managed — use its table, not this).
//  3. remove exactly that host's block; write the edit to a TEMP file.
//  4. adapt the temp file, then DIFF: the ONLY host that may disappear is the
//     target, nothing may be added, and dashboard+panel+all others must survive —
//     else ABORT (the real file is still untouched).
//  5. back up the original, commit the edit (atomic rename), reload via /load.
//     On reload failure, RESTORE the backup. The real file changes only after the
//     edit validated AND adapted clean.
func (e *Engine) RemoveStaticBlock(ctx context.Context, host string) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	start := e.now()
	res := Result{}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		res.Error = "host is required"
		return res, errors.New(res.Error)
	}
	for _, p := range e.cfg.ProtectedHosts() {
		if strings.EqualFold(p, host) {
			res.Error = fmt.Sprintf("%q is a protected domain — its block is never removed from the UI", host)
			res.DurationMS = e.since(start)
			return res, errors.New(res.Error)
		}
	}
	if e.adapter == nil || e.reloader == nil {
		res.Error = "remove-block: engine has no adapter/reloader (read-only)"
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	original, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "read main Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	adaptedBefore, _, err := e.adapter.Adapt(original, e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "validate current Caddyfile (adapt): " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	hostsBefore, err := hostSet(adaptedBefore)
	if err != nil {
		res.Error = "parse adapted config: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if !hostsBefore[host] {
		res.Error = fmt.Sprintf("%q is not present in the current Caddyfile", host)
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	// A rendered folder route is DB-managed, not a hand-written static block.
	if folder, ferr := e.RenderedHosts(); ferr == nil {
		for _, f := range folder {
			if strings.EqualFold(f, host) {
				res.Error = fmt.Sprintf("%q is a rendered route (DB-managed) — remove it via its table, not the Caddyfile", host)
				res.DurationMS = e.since(start)
				return res, errors.New(res.Error)
			}
		}
	}

	edited, ok := removeCaddyBlock(original, host)
	if !ok {
		res.Error = fmt.Sprintf("could not find a static block for %q in the Caddyfile", host)
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	// Write the edit to a temp file in the SAME dir (so the absolute `import`
	// resolves identically) and adapt it — the real file stays untouched.
	dir := filepath.Dir(e.cfg.MainCaddyfile)
	tmp, err := os.CreateTemp(dir, ".caddyfile-edit-*")
	if err != nil {
		res.Error = "create temp: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(edited); err != nil {
		_ = tmp.Close()
		res.Error = "write temp: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if err := tmp.Close(); err != nil {
		res.Error = "close temp: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	adaptedAfter, warnings, err := e.adapter.Adapt(edited, tmpPath)
	if err != nil {
		res.Error = "validate edited Caddyfile (adapt): " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	res.AdaptWarnings = warnings
	hostsAfter, err := hostSet(adaptedAfter)
	if err != nil {
		res.Error = "parse edited adapted config: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if dropped, err := assertOnlyTargetDropped(hostsBefore, hostsAfter, host, e.cfg.ProtectedHosts()); err != nil {
		res.BlockedDrops = dropped
		res.Error = err.Error()
		res.DurationMS = e.since(start)
		return res, err
	}

	// Back up the original, then commit the edit (atomic rename), then reload.
	backupPath, berr := e.backupCaddyfile(original)
	if berr != nil {
		res.Error = "backup Caddyfile: " + berr.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	res.BackupPath = backupPath

	if err := os.Rename(tmpPath, e.cfg.MainCaddyfile); err != nil {
		res.Error = "commit Caddyfile edit: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	cleanupTmp = false // committed

	if err := e.reloader.Load(ctx, adaptedAfter); err != nil {
		// Reload failed → restore the original so the file matches the live config.
		if werr := os.WriteFile(e.cfg.MainCaddyfile, original, 0o644); werr != nil {
			res.Error = fmt.Sprintf("reload failed (%v) AND restore failed (%v) — restore from %s", err, werr, backupPath)
		} else {
			res.Error = "reload: " + err.Error() + " (Caddyfile restored)"
		}
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	res.Reloaded = true
	res.Removed = []string{host}
	res.DurationMS = e.since(start)
	return res, nil
}

// backupCaddyfile writes the given Caddyfile bytes to a timestamped file in the
// backup dir and returns its path.
func (e *Engine) backupCaddyfile(content []byte) (string, error) {
	if err := os.MkdirAll(e.cfg.BackupDir, 0o755); err != nil {
		return "", err
	}
	ts := e.now().UTC().Format("20060102T150405Z")
	path := filepath.Join(e.cfg.BackupDir, "caddyfile-prior-"+ts+".caddy")
	if err := os.WriteFile(path, content, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

// removeCaddyBlock removes the top-level block whose site-address matches host. A
// best-effort line/brace scan — the caller's adapt-diff is the real safety net, so
// any over/under-removal is caught and aborted before the file is committed.
func removeCaddyBlock(content []byte, host string) ([]byte, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	lines := strings.Split(string(content), "\n")
	var out []string
	removed := false
	depth := 0
	i := 0
	for i < len(lines) {
		line := lines[i]
		if depth == 0 && !removed {
			if brace := strings.Index(line, "{"); brace >= 0 {
				addr := strings.TrimSpace(line[:brace])
				if addr != "" && blockMatchesHost(addr, host) {
					d := strings.Count(line, "{") - strings.Count(line, "}")
					i++
					for i < len(lines) && d > 0 {
						d += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
						i++
					}
					removed = true
					continue
				}
			}
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth < 0 {
			depth = 0
		}
		out = append(out, line)
		i++
	}
	return []byte(strings.Join(out, "\n")), removed
}

// blockMatchesHost reports whether a site-address line names host (comma/space
// separated addresses, scheme + :port stripped, exact token match).
func blockMatchesHost(addr, host string) bool {
	for _, tok := range strings.FieldsFunc(addr, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		t := strings.ToLower(strings.TrimSpace(tok))
		t = strings.TrimPrefix(t, "https://")
		t = strings.TrimPrefix(t, "http://")
		if idx := strings.Index(t, ":"); idx >= 0 {
			t = t[:idx]
		}
		if t == host {
			return true
		}
	}
	return false
}

// assertOnlyTargetDropped verifies the edit removed EXACTLY the target host and
// nothing else — no host added, no other host dropped, protected domains intact.
// Returns the unexpectedly-dropped hosts (for reporting) alongside the error.
func assertOnlyTargetDropped(before, after map[string]bool, target string, protected []string) ([]string, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	for h := range after {
		if !before[h] {
			return []string{h}, fmt.Errorf("edit introduced an unexpected host %q — refusing", h)
		}
	}
	var alsoDropped []string
	for h := range before {
		if !after[h] && h != target {
			alsoDropped = append(alsoDropped, h)
		}
	}
	if len(alsoDropped) > 0 {
		sort.Strings(alsoDropped)
		return alsoDropped, fmt.Errorf("edit would also drop %d other host(s) — refusing (only %q may be removed): %s",
			len(alsoDropped), target, strings.Join(alsoDropped, ", "))
	}
	if after[target] {
		return nil, fmt.Errorf("%q is still present after the edit — its block was not removed", target)
	}
	for _, p := range protected {
		if !after[strings.ToLower(strings.TrimSpace(p))] {
			return []string{p}, fmt.Errorf("protected domain %q would be missing after the edit — refusing", p)
		}
	}
	return nil, nil
}
