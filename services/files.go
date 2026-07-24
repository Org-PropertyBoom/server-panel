package services

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrProtectedPath = errors.New("editing this path is blocked for safety")

const maxEditableFileSize = 2 * 1024 * 1024

// protectedFilePaths / protectedFileDirs are files/trees too dangerous to edit
// through the panel — a bad save could lock the operator out or break boot. Edits
// to these are refused (use the root terminal if you truly must).
var protectedFilePaths = map[string]bool{
	"/etc/shadow": true, "/etc/gshadow": true, "/etc/passwd": true, "/etc/group": true,
	"/etc/sudoers": true, "/etc/fstab": true,
}
var protectedFileDirs = []string{"/boot", "/dev", "/proc", "/sys", "/etc/ssh", "/etc/sudoers.d"}

func isProtectedFilePath(p string) bool {
	if protectedFilePaths[p] {
		return true
	}
	for _, d := range protectedFileDirs {
		if p == d || strings.HasPrefix(p, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// WriteFileContent overwrites an EXISTING regular text file: refuses protected
// paths, binary content, and oversized writes; backs up the prior contents to
// `<file>.bak-<UTC ts>` (best-effort); then writes atomically (temp + rename,
// preserving mode). Standard users stay jailed to their home; root is unrestricted
// except the deny-list.
func WriteFileContent(filePath, content, homeDir string, isRoot bool) error {
	filePath = filepath.Clean(filePath)
	if !isRoot {
		cleanHome := filepath.Clean(homeDir)
		if !strings.HasPrefix(filePath, cleanHome+string(filepath.Separator)) {
			return ErrAccessDenied
		}
	}
	if isProtectedFilePath(filePath) {
		return ErrProtectedPath
	}
	if strings.ContainsRune(content, 0) {
		return errors.New("refusing to write binary content")
	}
	if len(content) > maxEditableFileSize {
		return errors.New("file too large to save")
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return err // edit existing files only — no create/traversal onto new paths
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}

	if orig, rerr := os.ReadFile(filePath); rerr == nil {
		backup := fmt.Sprintf("%s.bak-%s", filePath, time.Now().UTC().Format("20060102T150405Z"))
		_ = os.WriteFile(backup, orig, info.Mode().Perm())
	}

	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".ppt-edit-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filePath)
}

type FileInfo struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Path    string    `json:"path"`
}

type DirectoryList struct {
	CurrentPath string     `json:"currentPath"`
	ParentPath  string     `json:"parentPath"`
	Items       []FileInfo `json:"items"`
}

var ErrAccessDenied = errors.New("access denied")

func ListDirectory(requestedPath string, homeDir string, isRoot bool) (DirectoryList, error) {
	// Clean and resolve path
	var targetPath string
	if requestedPath == "" {
		targetPath = homeDir
	} else {
		targetPath = filepath.Clean(requestedPath)
	}

	// Enforce home directory jail for standard users
	if !isRoot {
		cleanHome := filepath.Clean(homeDir)
		if targetPath != cleanHome && !strings.HasPrefix(targetPath, cleanHome+string(filepath.Separator)) {
			return DirectoryList{}, ErrAccessDenied
		}
	}

	// Read directory
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return DirectoryList{}, err
	}

	items := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		items = append(items, FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Path:    filepath.Join(targetPath, entry.Name()),
		})
	}

	parentPath := ""
	if targetPath != "/" {
		// For standard users, do not allow going above home directory
		if !isRoot {
			cleanHome := filepath.Clean(homeDir)
			if targetPath != cleanHome {
				parentPath = filepath.Dir(targetPath)
			}
		} else {
			parentPath = filepath.Dir(targetPath)
		}
	}

	return DirectoryList{
		CurrentPath: targetPath,
		ParentPath:  parentPath,
		Items:       items,
	}, nil
}

type FileContent struct {
	Content  string    `json:"content"`
	Size     int64     `json:"size"`
	IsBinary bool      `json:"isBinary"`
	Modified time.Time `json:"modified"`
	Mode     string    `json:"mode"`            // e.g. "-rw-r--r--"
	Owner    string    `json:"owner,omitempty"` // Linux only
	Group    string    `json:"group,omitempty"` // Linux only
	Lines    int       `json:"lines,omitempty"` // text files only
}

// Directories pruned from search: virtual/huge system trees and per-dir noise
// (VCS/dependency caches). Keeps the walk fast + relevant to operator files.
var searchSkipAbs = map[string]bool{
	"/proc": true, "/sys": true, "/dev": true, "/run": true, "/tmp": true,
	"/usr": true, "/var": true, "/lib": true, "/lib64": true, "/boot": true, "/snap": true,
}
var searchSkipNames = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".cache": true,
	"__pycache__": true, ".npm": true, ".cargo": true, ".terraform": true,
}

// SearchFiles walks root for files whose NAME contains query (case-insensitive),
// best-effort within a time + result budget: it prunes system/noise trees, caps
// results, and stops after ~4s. Standard users stay jailed to their home. Ranked
// name-prefix first, then shallower paths.
func SearchFiles(root, query, homeDir string, isRoot bool) ([]FileInfo, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if len(q) < 2 {
		return nil, nil
	}
	if !isRoot {
		root = filepath.Clean(homeDir)
	}
	const limit = 150
	deadline := time.Now().Add(4 * time.Second)
	stop := errors.New("search-budget-reached")
	var results []FileInfo

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir // unreadable dir → skip its subtree, keep going
			}
			return nil
		}
		if len(results) >= limit || time.Now().After(deadline) {
			return stop
		}
		if d.IsDir() {
			if searchSkipAbs[path] || searchSkipNames[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), q) {
			if info, e := d.Info(); e == nil {
				results = append(results, FileInfo{Name: d.Name(), IsDir: false, Size: info.Size(), ModTime: info.ModTime(), Path: path})
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		return results, err
	}

	sort.Slice(results, func(i, j int) bool {
		pi := strings.HasPrefix(strings.ToLower(results[i].Name), q)
		pj := strings.HasPrefix(strings.ToLower(results[j].Name), q)
		if pi != pj {
			return pi
		}
		if len(results[i].Path) != len(results[j].Path) {
			return len(results[i].Path) < len(results[j].Path)
		}
		return results[i].Path < results[j].Path
	})
	return results, nil
}

func GetFileContent(filePath string, homeDir string, isRoot bool) (FileContent, error) {
	filePath = filepath.Clean(filePath)

	// Enforce home jail for standard users
	if !isRoot {
		cleanHome := filepath.Clean(homeDir)
		if !strings.HasPrefix(filePath, cleanHome+string(filepath.Separator)) {
			return FileContent{}, ErrAccessDenied
		}
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		return FileContent{}, err
	}
	if stat.IsDir() {
		return FileContent{}, errors.New("cannot read a directory")
	}

	owner, group := fileOwnerGroup(stat)
	meta := FileContent{
		Size:     stat.Size(),
		Modified: stat.ModTime(),
		Mode:     stat.Mode().String(),
		Owner:    owner,
		Group:    group,
	}

	// Check if binary by reading first 512 bytes
	file, err := os.Open(filePath)
	if err != nil {
		return FileContent{}, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)

	for i := 0; i < n; i++ {
		if buffer[i] == 0 {
			meta.IsBinary = true
			return meta, nil
		}
	}

	// Read entire file (limit to 2MB)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return FileContent{}, err
	}

	const limit = 2 * 1024 * 1024
	content := string(data)
	if len(content) > limit {
		content = content[:limit] + "\n... [truncated, file too large] ..."
	}
	meta.Content = content
	if len(content) > 0 {
		meta.Lines = strings.Count(content, "\n") + 1
	}
	return meta, nil
}
