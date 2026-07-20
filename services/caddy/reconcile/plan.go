// Package reconcile is the heart of the engine: it turns a DB snapshot of desired
// state into a safe, idempotent set of folder operations, then (Phase 2) validates
// and reloads Caddy.
//
// The planning half (this file) is PURE — given a snapshot, the current folder
// file names, and config, it computes a Plan (writes / removes / orphans / skips)
// with no I/O — so the safety-critical decisions are exhaustively unit-testable.
// The applying half (engine.go) performs the I/O.
//
// Safety rules encoded here:
//   - Render ONLY rows whose server_stack is known (in the port map); an unknown
//     stack is SKIPPED + reported, never guessed (the cross-stack-misroute class).
//   - Removals come ONLY from KNOWN rows observed inactive/soft-deleted — never a
//     blanket "delete files lacking a DB row" sweep. Files with no backing row are
//     ORPHANS: reported, never auto-pruned.
//   - The dashboard/panel domains are STATIC Caddyfile blocks, not folder files:
//     never rendered, never removed. Wildcard folder files are reserved: never removed.
package reconcile

import (
	"fmt"
	"sort"
	"strings"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
	"ppt/server-panel/services/caddy/render"
)

// FileOp is a single desired file write.
type FileOp struct {
	Name     string // "<host>.caddy"
	Host     string
	Contents string
}

// Skip records a row that was NOT rendered, with the reason (surfaced to the
// operator so a misconfigured row is visible, not silently dropped).
type Skip struct {
	Table  string
	Host   string
	Reason string
}

// Plan is the computed, not-yet-applied reconcile diff.
type Plan struct {
	Writes  []FileOp // active rows to (re)render — sorted by name
	Removes []string // filenames to remove: KNOWN inactive/soft-deleted rows whose file exists — sorted
	Orphans []string // filenames present with NO backing row — REPORTED, never auto-pruned — sorted
	Skips   []Skip   // rows not rendered (unknown stack, empty target, collision, protected) — sorted
}

// Empty reports whether the plan changes nothing on disk.
func (p Plan) Empty() bool { return len(p.Writes) == 0 && len(p.Removes) == 0 }

// BuildPlan computes the reconcile plan (pure). folderNames is the set of
// `*.caddy` filenames currently in the folder (from vhostfs.ListNames).
func BuildPlan(cfg config.Config, snap db.Snapshot, folderNames []string) Plan {
	// Protected files: the dashboard domain + the panel domain (static Caddyfile
	// blocks the operator owns) are never rendered or pruned as folder files.
	protectedFiles := map[string]bool{}
	for _, h := range cfg.ProtectedHosts() {
		protectedFiles[render.FileName(h)] = true
	}

	// Desired (active) hosts, keyed by filename so a cross-table host collision is
	// detected. Value carries the FileOp + which table won.
	type won struct {
		op    FileOp
		table string
	}
	desired := map[string]won{}
	var skips []Skip

	// Known-but-disabled filenames (inactive OR soft-deleted rows). A file here
	// whose host is NOT also desired, and which exists, is a removal candidate.
	disabled := map[string]bool{}

	for _, r := range snap.Rows {
		host := strings.ToLower(strings.TrimSpace(r.Host))
		if host == "" {
			continue
		}
		name := render.FileName(host)

		// The dashboard + panel domains are static Caddyfile blocks, never folder files.
		if protectedFiles[name] {
			skips = append(skips, Skip{r.Table, host, "protected domain (dashboard/panel) — static Caddyfile block, never a folder file"})
			continue
		}

		if !r.Desired() {
			// Known, disabled row → its file should not exist (subject to the
			// first-run + protection guards applied when the plan is turned into
			// removals below).
			disabled[name] = true
			continue
		}

		h, reason := toHost(cfg, r)
		if reason != "" {
			skips = append(skips, Skip{r.Table, host, reason})
			continue
		}
		_, contents, err := render.Render(h)
		if err != nil {
			skips = append(skips, Skip{r.Table, host, "render: " + err.Error()})
			continue
		}
		if prev, ok := desired[name]; ok {
			// Host desired by two rows (across tables) — unique-across-three is a
			// schema invariant, but never clobber silently: keep the first (stable
			// by table read order website<platform<redirect) and report the clash.
			skips = append(skips, Skip{r.Table, host, "duplicate host — already rendered from " + prev.table})
			continue
		}
		desired[name] = won{op: FileOp{Name: name, Host: host, Contents: contents}, table: r.Table}
	}

	// A host that is desired by one row wins over a disabled row for the same host
	// — do not remove a file we are actively writing.
	for name := range desired {
		delete(disabled, name)
	}

	folder := map[string]bool{}
	for _, n := range folderNames {
		folder[n] = true
	}

	var writes []FileOp
	for _, w := range desired {
		writes = append(writes, w.op)
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].Name < writes[j].Name })

	var removes []string
	for name := range disabled {
		if !folder[name] {
			continue // nothing on disk to remove
		}
		if protectedFromRemoval(name, protectedFiles) {
			continue // dashboard domain / wildcard files are never removed
		}
		removes = append(removes, name)
	}
	sort.Strings(removes)

	// Orphans: present on disk, not desired, not a known-disabled row, not
	// protected. Reported only — never auto-pruned.
	var orphans []string
	for name := range folder {
		if _, ok := desired[name]; ok {
			continue
		}
		if disabled[name] {
			continue // accounted for as a (possible) remove
		}
		if protectedFromRemoval(name, protectedFiles) {
			continue // the dashboard file shouldn't exist, and wildcards are reserved
		}
		orphans = append(orphans, name)
	}
	sort.Strings(orphans)

	sort.Slice(skips, func(i, j int) bool {
		if skips[i].Host != skips[j].Host {
			return skips[i].Host < skips[j].Host
		}
		return skips[i].Table < skips[j].Table
	})

	return Plan{Writes: writes, Removes: removes, Orphans: orphans, Skips: skips}
}

// toHost maps a DB row to a renderable Host, or returns a non-empty reason why it
// cannot be rendered (which becomes a Skip).
func toHost(cfg config.Config, r db.Row) (render.Host, string) {
	host := strings.ToLower(strings.TrimSpace(r.Host))
	switch r.Table {
	case "website_hosts":
		up, ok := cfg.UpstreamFor(r.ServerStack)
		if !ok {
			return render.Host{}, fmt.Sprintf("unknown server_stack %q — not in the stack->port map; SKIPPED (never guess a port)", r.ServerStack)
		}
		// Security headers apply to TENANT webfront hosts only — never the
		// admin/dashboard-app hosts (platform_hosts), which need different framing/CSP.
		return render.Host{Host: host, Kind: render.KindTenant, Target: up, Encode: cfg.EncodeFormats(), HeaderBlock: cfg.SecurityHeaders.HeaderBlock()}, ""
	case "platform_hosts":
		if strings.TrimSpace(r.Target) == "" {
			return render.Host{}, "platform_hosts row has an empty target upstream"
		}
		return render.Host{Host: host, Kind: render.KindSystem, Target: r.Target, Encode: cfg.EncodeFormats()}, ""
	case "platform_redirect_hosts":
		if strings.TrimSpace(r.Target) == "" {
			return render.Host{}, "platform_redirect_hosts row has an empty target URL"
		}
		return render.Host{Host: host, Kind: render.KindRedirect, Target: r.Target, RedirectCode: r.Code}, ""
	default:
		return render.Host{}, "unknown table " + r.Table
	}
}

// protectedFromRemoval reports whether a folder file must never be removed: a
// protected-domain file (dashboard/panel — should not exist, but we never touch
// it) and any wildcard file (reserved — "never delete, never mis-port").
func protectedFromRemoval(name string, protectedFiles map[string]bool) bool {
	if protectedFiles[name] {
		return true
	}
	return strings.HasPrefix(name, "wildcard_")
}
