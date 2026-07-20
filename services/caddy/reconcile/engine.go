package reconcile

import (
	"sort"
	"sync"
	"time"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
	"ppt/server-panel/services/caddy/vhostfs"
)

// SkipInfo is a JSON-friendly Skip.
type SkipInfo struct {
	Table  string `json:"table"`
	Host   string `json:"host"`
	Reason string `json:"reason"`
}

// Engine owns the vhosts folder and computes reconcile plans. Phase 1 exposes
// only DryRun (read-only drift) — no writes, no adapt, no reload. The live
// reconcile+reload path (with an Adapter + Reloader) is added in Phase 2 behind
// an explicit activation gate.
type Engine struct {
	cfg config.Config
	dir vhostfs.Dir

	mu        sync.Mutex
	firstDone bool // set by the (Phase 2) live Reconcile; DryRun reports it
	now       func() time.Time
}

// NewEngine constructs a read-only (Phase 1) engine over the configured folder.
func NewEngine(cfg config.Config) *Engine {
	return &Engine{cfg: cfg, dir: vhostfs.New(cfg.VhostsDir), now: time.Now}
}

// FileState is one folder file for the drift view.
type FileState struct {
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Contents string `json:"contents"`
}

// DryRunResult is the read-only drift view: what a reconcile WOULD do, computed
// without touching anything. would_write = desired files whose on-disk bytes
// differ (or are missing); would_remove / orphans / skips come from the plan.
type DryRunResult struct {
	VhostsDir     string         `json:"vhosts_dir"`
	Files         []FileState    `json:"files"`
	DesiredCount  int            `json:"desired_count"`
	WouldWrite    []string       `json:"would_write"`
	WouldRemove   []string       `json:"would_remove"`
	Orphans       []string       `json:"orphans"`
	Skips         []SkipInfo     `json:"skips,omitempty"`
	Sources       map[string]int `json:"sources,omitempty"`
	MissingTables []string       `json:"missing_tables,omitempty"`
	InSync        bool           `json:"in_sync"`
	FirstPassDone bool           `json:"first_pass_done"`
}

// DryRun computes the plan for a snapshot against the current folder WITHOUT
// applying anything.
func (e *Engine) DryRun(snap db.Snapshot) (DryRunResult, error) {
	if err := e.dir.Ensure(); err != nil {
		return DryRunResult{}, err
	}
	files, err := e.dir.List()
	if err != nil {
		return DryRunResult{}, err
	}
	onDisk := make(map[string]string, len(files))
	names := make([]string, 0, len(files))
	for _, f := range files {
		onDisk[f.Name] = f.Contents
		names = append(names, f.Name)
	}
	plan := BuildPlan(e.cfg, snap, names)

	var wouldWrite []string
	for _, w := range plan.Writes {
		if cur, ok := onDisk[w.Name]; !ok || cur != w.Contents {
			wouldWrite = append(wouldWrite, w.Name)
		}
	}
	sort.Strings(wouldWrite)

	out := DryRunResult{
		VhostsDir:     e.cfg.VhostsDir,
		DesiredCount:  len(plan.Writes),
		WouldWrite:    wouldWrite,
		WouldRemove:   plan.Removes,
		Orphans:       plan.Orphans,
		Sources:       snap.Sources,
		MissingTables: snap.MissingTables,
		InSync:        len(wouldWrite) == 0 && len(plan.Removes) == 0,
		FirstPassDone: e.firstPassDone(),
	}
	for _, s := range plan.Skips {
		out.Skips = append(out.Skips, SkipInfo(s))
	}
	for _, f := range files {
		out.Files = append(out.Files, FileState{Name: f.Name, Size: len(f.Contents), Contents: f.Contents})
	}
	return out, nil
}

func (e *Engine) firstPassDone() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.firstDone
}
