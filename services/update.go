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

const defaultBinaryURL = "https://github.com/Org-PropertyBoom/server-panel-dist/raw/main/public/dist/mthan-vps"
const defaultVersionURL = "https://github.com/Org-PropertyBoom/server-panel-dist/raw/main/public/dist/version.json"

var ErrUpdateRequiresRoot = errors.New("self update requires root")

type UpdateResult struct {
	BinaryURL   string    `json:"binaryUrl"`
	InstallPath string    `json:"installPath"`
	Restart     bool      `json:"restart"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpdateCheckResult struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	LocalVersion    string `json:"localVersion"`
	RemoteVersion   string `json:"remoteVersion"`
	LocalBuildTime  string `json:"localBuildTime"`
	RemoteBuildTime string `json:"remoteBuildTime"`
}

type UpdateService struct {
	binaryURL      string
	versionURL     string
	httpClient     *http.Client
	installPath    string
	localVersion   string
	localBuildTime string
	cacheResult    *UpdateCheckResult
	cacheExpires   time.Time
}

func NewUpdateService(localVersion, localBuildTime string) *UpdateService {
	return &UpdateService{
		binaryURL:      getEnv("BINARY_URL", defaultBinaryURL),
		versionURL:     getEnv("VERSION_URL", defaultVersionURL),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		installPath:    updateInstallPath(),
		localVersion:   localVersion,
		localBuildTime: localBuildTime,
	}
}

func (s *UpdateService) CheckUpdate(ctx context.Context) (UpdateCheckResult, error) {
	if s.cacheResult != nil && time.Now().Before(s.cacheExpires) {
		return *s.cacheResult, nil
	}

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
		Version   string `json:"version"`
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

	res := UpdateCheckResult{
		UpdateAvailable: updateAvailable,
		LocalVersion:    s.localVersion,
		RemoteVersion:   remote.Version,
		LocalBuildTime:  s.localBuildTime,
		RemoteBuildTime: remote.BuildTime,
	}

	s.cacheResult = &res
	s.cacheExpires = time.Now().Add(15 * time.Second)

	return res, nil
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

	if err := validateLinuxExecutable(tmpPath); err != nil {
		return UpdateResult{}, err
	}

	if err := replaceFile(tmpPath, s.installPath); err != nil {
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

func replaceFile(srcPath, dstPath string) error {
	backupPath := dstPath + ".old"
	_ = os.Remove(backupPath)

	hadExisting := true
	if err := os.Rename(dstPath, backupPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		hadExisting = false
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		if hadExisting {
			_ = os.Rename(backupPath, dstPath)
		}
		return err
	}

	return nil
}

func validateLinuxExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < 4 {
		return errors.New("downloaded binary is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return err
	}
	if magic != [4]byte{0x7f, 'E', 'L', 'F'} {
		return errors.New("downloaded file is not a Linux executable")
	}

	return nil
}
