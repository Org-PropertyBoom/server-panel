package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	IsBinary bool   `json:"isBinary"`
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

	// Check if binary by reading first 512 bytes
	file, err := os.Open(filePath)
	if err != nil {
		return FileContent{}, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)

	isBinary := false
	for i := 0; i < n; i++ {
		if buffer[i] == 0 {
			isBinary = true
			break
		}
	}

	if isBinary {
		return FileContent{
			Size:     stat.Size(),
			IsBinary: true,
		}, nil
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

	return FileContent{
		Content:  content,
		Size:     stat.Size(),
		IsBinary: false,
	}, nil
}
