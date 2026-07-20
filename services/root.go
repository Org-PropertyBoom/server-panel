package services

import (
	"bufio"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type StartupConfig struct {
	Addr        string
	Env         string
	Mode        string
	OSName      string
	OSBranch    string
	PostBaseURL string
	UID         int
	Username    string
	IsRoot      bool
}

type StartupService interface {
	Startup() StartupConfig
}

type RootService struct{}

func NewStartupService() StartupService {
	if os.Geteuid() == 0 {
		return NewRootService()
	}

	return NewUserService()
}

func NewRootService() *RootService {
	return &RootService{}
}

func (s *RootService) Startup() StartupConfig {
	return startupConfig("root", true)
}

func startupConfig(mode string, isRoot bool) StartupConfig {
	uid := os.Geteuid()
	port := loadOrInitializePort()
	release := readOSRelease()

	return StartupConfig{
		Addr:        ":" + strconv.Itoa(port),
		Env:         getEnv("APP_ENV", "development"),
		Mode:        mode,
		OSName:      osName(release),
		OSBranch:    osBranch(release),
		PostBaseURL: os.Getenv("POST_BASE_URL"),
		UID:         uid,
		Username:    username(uid),
		IsRoot:      isRoot,
	}
}

func loadOrInitializePort() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultPortFromEnv()
	}

	configDir := filepath.Join(home, ".ppt-server-panel")
	configPath := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		_ = os.MkdirAll(configDir, 0700)
		port := defaultPortFromEnv()
		_ = os.WriteFile(configPath, []byte("port: "+strconv.Itoa(port)+"\n"), 0600)
		return port
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultPortFromEnv()
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "port:") {
			valStr := strings.TrimSpace(strings.TrimPrefix(line, "port:"))
			if port, err := strconv.Atoi(valStr); err == nil {
				return port
			}
		}
	}

	return defaultPortFromEnv()
}

func defaultPortFromEnv() int {
	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		return 2205
	}

	if _, portStr, err := net.SplitHostPort(addr); err == nil {
		if p, err := strconv.Atoi(portStr); err == nil {
			return p
		}
	}

	if strings.HasPrefix(addr, ":") {
		if p, err := strconv.Atoi(strings.TrimPrefix(addr, ":")); err == nil {
			return p
		}
	}

	if p, err := strconv.Atoi(addr); err == nil {
		return p
	}

	return 2205
}

func readOSRelease() map[string]string {
	values := make(map[string]string)
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return values
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(value, `"'`)
	}
	return values
}

func osName(release map[string]string) string {
	if name := release["PRETTY_NAME"]; name != "" {
		return name
	}
	if name := release["NAME"]; name != "" {
		return name
	}
	return "Linux"
}

func osBranch(release map[string]string) string {
	id := strings.ToLower(release["ID"])
	idLike := strings.ToLower(release["ID_LIKE"])

	// Check ID and ID_LIKE
	isDebian := strings.Contains(id, "debian") || strings.Contains(id, "ubuntu") ||
		strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu")
	isRhel := strings.Contains(id, "rhel") || strings.Contains(id, "fedora") || strings.Contains(id, "centos") || strings.Contains(id, "almalinux") || strings.Contains(id, "rocky") || strings.Contains(id, "amzn") ||
		strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") || strings.Contains(idLike, "centos")
	isArch := strings.Contains(id, "arch") || strings.Contains(idLike, "arch")
	isAlpine := strings.Contains(id, "alpine") || strings.Contains(idLike, "alpine")

	if isDebian {
		return "debian"
	}
	if isRhel {
		return "rhel"
	}
	if isArch {
		return "arch"
	}
	if isAlpine {
		return "alpine"
	}

	// Fallback to checking package managers or command existence
	if exists("/usr/bin/apt-get") || exists("/usr/bin/apt") {
		return "debian"
	}
	if exists("/usr/bin/dnf") || exists("/usr/bin/yum") {
		return "rhel"
	}
	if exists("/usr/bin/pacman") {
		return "arch"
	}
	if exists("/sbin/apk") || exists("/usr/bin/apk") {
		return "alpine"
	}

	return "other"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func username(uid int) string {
	currentUser, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return strconv.Itoa(uid)
	}

	return currentUser.Username
}
