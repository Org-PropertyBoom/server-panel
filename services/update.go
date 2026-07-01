package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultBinaryURL = "https://github.com/antoine-mai/mthan-tools-vps/raw/main/bin/mthan-vps"
const defaultVersionURL = "https://github.com/antoine-mai/mthan-tools-vps/raw/main/bin/version.json"

var ErrUpdateRequiresRoot = errors.New("self update requires root")

type UpdateResult struct {
	BinaryURL   string    `json:"binaryUrl"`
	InstallPath string    `json:"installPath"`
	Restart     bool      `json:"restart"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpdateCheckResult struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	LocalBuildTime  string `json:"localBuildTime"`
	RemoteBuildTime string `json:"remoteBuildTime"`
}

type UpdateService struct {
	binaryURL      string
	versionURL     string
	httpClient     *http.Client
	installPath    string
	localBuildTime string
}

func NewUpdateService(localBuildTime string) *UpdateService {
	return &UpdateService{
		binaryURL:      getEnv("BINARY_URL", defaultBinaryURL),
		versionURL:     getEnv("VERSION_URL", defaultVersionURL),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		installPath:    updateInstallPath(),
		localBuildTime: localBuildTime,
	}
}

func (s *UpdateService) CheckUpdate(ctx context.Context) (UpdateCheckResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.versionURL, nil)
	if err != nil {
		return UpdateCheckResult{}, err
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		return UpdateCheckResult{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return UpdateCheckResult{}, errors.New("failed to fetch remote version info")
	}

	var remote struct {
		BuildTime string `json:"buildTime"`
	}
	if err := json.NewDecoder(response.Body).Decode(&remote); err != nil {
		return UpdateCheckResult{}, err
	}

	updateAvailable := false
	if remote.BuildTime != "" && s.localBuildTime != "" {
		tRemote, err1 := time.Parse(time.RFC3339, remote.BuildTime)
		tLocal, err2 := time.Parse(time.RFC3339, s.localBuildTime)
		if err1 == nil && err2 == nil {
			updateAvailable = tRemote.After(tLocal)
		} else {
			updateAvailable = remote.BuildTime != s.localBuildTime
		}
	} else if remote.BuildTime != "" {
		// If we don't have a local build time (e.g. dev build), assume update is available if remote exists
		updateAvailable = true
	}

	return UpdateCheckResult{
		UpdateAvailable: updateAvailable,
		LocalBuildTime:  s.localBuildTime,
		RemoteBuildTime: remote.BuildTime,
	}, nil
}

func (s *UpdateService) SelfUpdate(ctx context.Context) (UpdateResult, error) {
	if os.Geteuid() != 0 {
		return UpdateResult{}, ErrUpdateRequiresRoot
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.binaryURL, nil)
	if err != nil {
		return UpdateResult{}, err
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		return UpdateResult{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return UpdateResult{}, errors.New("binary download failed")
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(s.installPath), ".vps-update-*")
	if err != nil {
		return UpdateResult{}, err
	}

	tmpPath := tmpFile.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmpFile, response.Body); err != nil {
		_ = tmpFile.Close()
		return UpdateResult{}, err
	}

	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return UpdateResult{}, err
	}

	if err := tmpFile.Close(); err != nil {
		return UpdateResult{}, err
	}

	if err := os.Rename(tmpPath, s.installPath); err != nil {
		return UpdateResult{}, err
	}
	removeTmp = false

	return UpdateResult{
		BinaryURL:   s.binaryURL,
		InstallPath: s.installPath,
		Restart:     true,
		UpdatedAt:   time.Now(),
	}, nil
}

func updateInstallPath() string {
	if installPath := os.Getenv("INSTALL_PATH"); installPath != "" {
		return installPath
	}

	executable, err := os.Executable()
	if err == nil && executable != "" {
		return executable
	}

	return "/usr/local/bin/mthan-vps"
}
