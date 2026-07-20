// Package vhostfs is the low-level, safe interface to the vhosts folder the
// engine SOLELY owns. It provides only the primitives the reconcile core
// composes — atomic write, listing, and removal of `<host>.caddy` files —
// deliberately WITHOUT any policy (what to write, what to prune, what is
// protected). Policy lives in the reconcile layer; this package just makes each
// individual file operation safe and crash-consistent.
//
// Atomicity matters because Caddy (via its root reload) reads this folder
// concurrently: a half-written file must never be importable. Every write is a
// temp-file-in-the-same-dir + rename, so a reader sees either the old bytes or
// the complete new bytes, never a torn file.
package vhostfs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Ext is the extension of every vhost file. The folder is imported by Caddy as
// `import <dir>/*`, but the engine only ever writes/enumerates `*.caddy`.
const Ext = ".caddy"

// Dir is a handle to the owned vhosts folder.
type Dir struct {
	path string
}

// New returns a Dir for path. It does not create or stat the folder; call
// Ensure to guarantee it exists.
func New(path string) Dir {
	return Dir{path: strings.TrimRight(filepath.ToSlash(path), "/")}
}

// Path is the folder path.
func (d Dir) Path() string { return d.path }

// Ensure verifies the folder exists and is a directory (it must be pre-created
// and owned by the engine's user on the host). Returns a clear error otherwise.
func (d Dir) Ensure() error {
	info, err := os.Stat(d.path)
	if err != nil {
		return fmt.Errorf("vhosts dir %q: %w", d.path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vhosts dir %q is not a directory", d.path)
	}
	return nil
}

// File is one `.caddy` file present in the folder.
type File struct {
	Name     string // basename incl. .caddy, e.g. "example.com.caddy"
	Contents string
}

// List returns every regular `*.caddy` file directly in the folder (no
// recursion, no subdirs), sorted by name, each with its current contents.
func (d Dir) List() ([]File, error) {
	names, err := d.ListNames()
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(d.path, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		out = append(out, File{Name: name, Contents: string(b)})
	}
	return out, nil
}

// ListNames returns the sorted basenames of every regular `*.caddy` file
// directly in the folder.
func (d Dir) ListNames() ([]string, error) {
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return nil, fmt.Errorf("read vhosts dir %q: %w", d.path, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, Ext) {
			continue
		}
		// Skip anything that isn't a plain regular file (e.g. a symlink), which
		// could otherwise let a write/remove escape the folder.
		if info, err := e.Info(); err != nil || !info.Mode().IsRegular() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Write atomically writes contents to <dir>/<name>. name must be a plain
// basename ending in .caddy (no path separators, no traversal). It writes a temp
// file in the same directory, fsyncs it, then renames over the target so a
// concurrent reader never sees a partial file. Returns true if the file's bytes
// changed (created or differing content), false if the file already held exactly
// these bytes (no write performed — keeps mtimes stable and avoids needless churn).
func (d Dir) Write(name, contents string) (changed bool, err error) {
	if err := validName(name); err != nil {
		return false, err
	}
	target := filepath.Join(d.path, name)

	if existing, err := os.ReadFile(target); err == nil && string(existing) == contents {
		return false, nil // already correct — no-op
	}

	tmp, err := os.CreateTemp(d.path, ".ppt-vhost-*"+Ext+".tmp")
	if err != nil {
		return false, fmt.Errorf("create temp for %s: %w", name, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.WriteString(contents); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("write temp for %s: %w", name, err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("sync temp for %s: %w", name, err)
	}
	if err = tmp.Close(); err != nil {
		return false, fmt.Errorf("close temp for %s: %w", name, err)
	}
	if err = os.Rename(tmpName, target); err != nil {
		return false, fmt.Errorf("rename temp to %s: %w", name, err)
	}
	return true, nil
}

// Remove deletes <dir>/<name> if present. A missing file is not an error
// (idempotent). Returns true if a file was actually removed. name must be a
// plain basename ending in .caddy.
func (d Dir) Remove(name string) (removed bool, err error) {
	if err := validName(name); err != nil {
		return false, err
	}
	target := filepath.Join(d.path, name)
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	// Only remove a plain regular file — never follow a symlink out of the folder.
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("refusing to remove non-regular file %s", name)
	}
	if err := os.Remove(target); err != nil {
		return false, fmt.Errorf("remove %s: %w", name, err)
	}
	return true, nil
}

// validName rejects anything that is not a plain `<something>.caddy` basename:
// no separators, no "..", no absolute path, no empty/NUL. This is the single
// guard that keeps every write/remove strictly inside the folder.
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("empty file name")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "\x00") {
		return fmt.Errorf("invalid file name %q (must be a plain basename)", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid file name %q", name)
	}
	if !strings.HasSuffix(name, Ext) {
		return fmt.Errorf("file name %q must end in %s", name, Ext)
	}
	return nil
}
