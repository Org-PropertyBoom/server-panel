package services

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type AppStatus struct {
	Name       string `json:"name"`
	Installed  bool   `json:"installed"`
	Manageable bool   `json:"manageable"`
	Running    bool   `json:"running"`
	Service    string `json:"serviceName,omitempty"`
}

type appDefinition struct {
	name       string
	binaries   []string
	services   []string
	phpVersion string
}

var knownApps = []appDefinition{
	{name: "nginx", binaries: []string{"nginx"}, services: []string{"nginx.service"}},
	{name: "mariadb", binaries: []string{"mariadbd", "mysqld", "mariadb"}, services: []string{"mariadb.service", "mysql.service"}},
	{name: "redis", binaries: []string{"redis-server"}, services: []string{"redis-server.service", "redis.service"}},
	{name: "docker", binaries: []string{"docker", "/usr/bin/docker", "/usr/local/bin/docker"}, services: []string{"docker.service"}},
	{name: "podman", binaries: []string{"podman", "/usr/bin/podman", "/usr/local/bin/podman"}, services: []string{"podman.service", "podman.socket"}},
	{name: "node", binaries: []string{"node", "nodejs", "/usr/bin/node", "/usr/local/bin/node"}},
	{name: "php8.1", phpVersion: "8.1", binaries: phpBinaries("8.1"), services: phpServices("8.1")},
	{name: "php8.2", phpVersion: "8.2", binaries: phpBinaries("8.2"), services: phpServices("8.2")},
	{name: "php8.3", phpVersion: "8.3", binaries: phpBinaries("8.3"), services: phpServices("8.3")},
	{name: "php8.4", phpVersion: "8.4", binaries: phpBinaries("8.4"), services: phpServices("8.4")},
}

func DetectApps() []AppStatus {
	statuses := make([]AppStatus, 0, len(knownApps))
	phpVersion := genericPHPVersion()
	for _, app := range knownApps {
		installed := hasBinary(app.binaries)
		if !installed && app.phpVersion != "" {
			installed = phpVersion == app.phpVersion
		}

		service := ""
		if len(app.services) > 0 {
			service = app.services[0]
		}
		running := false
		if installed {
			service, running = serviceStatus(app.services)
		}

		statuses = append(statuses, AppStatus{
			Name:       app.name,
			Installed:  installed,
			Manageable: len(app.services) > 0,
			Running:    running,
			Service:    service,
		})
	}
	return statuses
}

func hasBinary(names []string) bool {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func serviceStatus(names []string) (string, bool) {
	for _, name := range names {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", name).Run()
		cancel()
		if err == nil {
			return name, true
		}
	}
	if len(names) > 0 {
		return names[0], false
	}
	return "", false
}

func phpBinaries(version string) []string {
	return []string{
		"php" + version,
		"php-fpm" + version,
		"php-fpm" + strings.ReplaceAll(version, ".", ""),
		"/usr/bin/php" + version,
		"/usr/sbin/php-fpm" + version,
		"/usr/local/bin/php" + version,
		"/opt/remi/php" + strings.ReplaceAll(version, ".", "") + "/root/usr/sbin/php-fpm",
	}
}

func phpServices(version string) []string {
	compact := strings.ReplaceAll(version, ".", "")
	return []string{
		"php" + version + "-fpm.service",
		"php-fpm" + version + ".service",
		"php" + compact + "-php-fpm.service",
		"php-fpm.service",
	}
}

func genericPHPVersion() string {
	php, err := exec.LookPath("php")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, php, "-r", "echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
