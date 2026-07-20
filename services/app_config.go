package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const maxAppConfigSize = 2 << 20

var (
	ErrAppConfigDenied   = errors.New("app configuration path is not allowed")
	ErrAppConfigInvalid  = errors.New("app configuration is invalid")
	ErrAppConfigTooLarge = errors.New("app configuration is too large")
)

type AppConfigFile struct {
	App     string `json:"app"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
	Path    string `json:"path"`
}

type AppConfigService struct {
	allowed map[string]map[string]bool
}

func NewAppConfigService() *AppConfigService {
	return &AppConfigService{allowed: map[string]map[string]bool{
		"caddy": {
			"/etc/caddy/Caddyfile": true,
		},
		"docker": {
			"/etc/docker/daemon.json": true,
		},
		"podman": {
			"/etc/containers/containers.conf": true,
			"/etc/containers/registries.conf": true,
			"/etc/containers/storage.conf":    true,
			"/etc/containers/policy.json":     true,
		},
	}}
}

func (s *AppConfigService) Read(app, requestedPath string) (AppConfigFile, error) {
	path, err := s.allowedPath(app, requestedPath)
	if err != nil {
		return AppConfigFile{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AppConfigFile{App: app, Content: defaultAppConfig(path), Exists: false, Path: path}, nil
	}
	if err != nil {
		return AppConfigFile{}, err
	}
	if len(data) > maxAppConfigSize {
		return AppConfigFile{}, ErrAppConfigTooLarge
	}
	return AppConfigFile{App: app, Content: string(data), Exists: true, Path: path}, nil
}

func (s *AppConfigService) Write(app, requestedPath, content string) (AppConfigFile, error) {
	path, err := s.allowedPath(app, requestedPath)
	if err != nil {
		return AppConfigFile{}, err
	}
	if len(content) > maxAppConfigSize {
		return AppConfigFile{}, ErrAppConfigTooLarge
	}
	if strings.ContainsRune(content, 0) {
		return AppConfigFile{}, ErrAppConfigInvalid
	}
	if filepath.Ext(path) == ".json" && !json.Valid([]byte(content)) {
		return AppConfigFile{}, ErrAppConfigInvalid
	}

	mode := os.FileMode(0644)
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() {
			return AppConfigFile{}, ErrAppConfigDenied
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return AppConfigFile{}, statErr
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return AppConfigFile{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mthan-config-*")
	if err != nil {
		return AppConfigFile{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return AppConfigFile{}, err
	}
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return AppConfigFile{}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return AppConfigFile{}, err
	}
	if err := temporary.Close(); err != nil {
		return AppConfigFile{}, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return AppConfigFile{}, err
	}
	return AppConfigFile{App: app, Content: content, Exists: true, Path: path}, nil
}

func (s *AppConfigService) allowedPath(app, requestedPath string) (string, error) {
	app = strings.ToLower(strings.TrimSpace(app))
	path := filepath.Clean(strings.TrimSpace(requestedPath))
	if path == "." || !filepath.IsAbs(path) || !s.allowed[app][path] {
		return "", ErrAppConfigDenied
	}
	return path, nil
}

func defaultAppConfig(path string) string {
	if filepath.Ext(path) == ".json" {
		return "{}\n"
	}
	return ""
}
