package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	distRepo        = "Org-PropertyBoom/server-panel-dist"
	distBranch      = "main"
	distVersionPath = "public/dist/version.json"
	distBinaryPath  = "public/dist/ppt-server-panel"

	// commitSHAURL resolves the latest commit on the dist branch via the GitHub API.
	// The API reflects a push immediately and is NOT behind the raw-content CDN's
	// ~5-minute cache — so it's our fresh pointer to "what's published right now".
	commitSHAURL = "https://api.github.com/repos/" + distRepo + "/commits/" + distBranch

	// Fallback direct URLs, used only if commit-SHA resolution fails. These go
	// through GitHub's raw CDN (Fastly, max-age=300), so they can be up to ~5 min
	// stale — acceptable only as a degraded fallback.
	defaultVersionURL = "https://github.com/" + distRepo + "/raw/" + distBranch + "/" + distVersionPath
	defaultBinaryURL  = "https://github.com/" + distRepo + "/raw/" + distBranch + "/" + distBinaryPath
)

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
	versionOverride string // VERSION_URL env, if set (escape hatch; used verbatim)
	binaryOverride  string // BINARY_URL env, if set
	httpClient      *http.Client
	installPath     string
	localVersion    string
	localBuildTime  string
	cacheResult     *UpdateCheckResult
	cacheExpires    time.Time
}

func NewUpdateService(localVersion, localBuildTime string) *UpdateService {
	return &UpdateService{
		versionOverride: os.Getenv("VERSION_URL"),
		binaryOverride:  os.Getenv("BINARY_URL"),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		installPath:     updateInstallPath(),
		localVersion:    localVersion,
		localBuildTime:  localBuildTime,
	}
}

func (s *UpdateService) CheckUpdate(ctx context.Context) (UpdateCheckResult, error) {
	if s.cacheResult != nil && time.Now().Before(s.cacheExpires) {
		return *s.cacheResult, nil
	}

	versionURL, _ := s.resolveURLs(ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cacheBust(versionURL), nil)
	if err != nil {
		return UpdateCheckResult{}, err
	}
	noStore(request)

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

	_, binaryURL := s.resolveURLs(ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cacheBust(binaryURL), nil)
	if err != nil {
		return UpdateResult{}, err
	}
	noStore(request)

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
		BinaryURL:   binaryURL,
		InstallPath: s.installPath,
		Restart:     true,
		UpdatedAt:   time.Now(),
	}, nil
}

// resolveURLs returns the version.json + binary URLs to fetch. Explicit env
// overrides win (used verbatim). Otherwise it resolves the latest commit SHA via
// the GitHub API and pins both to that immutable SHA — a SHA-pinned raw URL is a
// distinct object per push, so it is ALWAYS a cache miss (fresh), and version.json
// + binary come from the SAME commit. Falls back to the branch raw URLs if the SHA
// can't be resolved.
func (s *UpdateService) resolveURLs(ctx context.Context) (string, string) {
	if s.versionOverride != "" || s.binaryOverride != "" {
		versionURL := s.versionOverride
		if versionURL == "" {
			versionURL = defaultVersionURL
		}
		binaryURL := s.binaryOverride
		if binaryURL == "" {
			binaryURL = defaultBinaryURL
		}
		return versionURL, binaryURL
	}
	if sha, err := s.latestSHA(ctx); err == nil && sha != "" {
		return rawAtSHA(sha, distVersionPath), rawAtSHA(sha, distBinaryPath)
	}
	return defaultVersionURL, defaultBinaryURL
}

// latestSHA returns the current commit SHA of the dist branch from the GitHub API.
func (s *UpdateService) latestSHA(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cacheBust(commitSHAURL), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github.sha")
	noStore(request)

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("commit sha lookup failed: http %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64))
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(body))
	if len(sha) < 7 {
		return "", errors.New("empty commit sha")
	}
	return sha, nil
}

func rawAtSHA(sha, path string) string {
	return "https://raw.githubusercontent.com/" + distRepo + "/" + sha + "/" + path
}

// cacheBust appends a unique query param — a belt for the fallback (branch) URLs
// and the API call; harmless on the already-unique SHA-pinned URLs.
func cacheBust(rawURL string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "_cb=" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// noStore asks any intermediary not to serve a cached copy.
func noStore(r *http.Request) {
	r.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	r.Header.Set("Pragma", "no-cache")
}

func updateInstallPath() string {
	if installPath := os.Getenv("INSTALL_PATH"); installPath != "" {
		return installPath
	}

	executable, err := os.Executable()
	if err == nil && executable != "" {
		return executable
	}

	return "/usr/local/bin/ppt-server-panel"
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
