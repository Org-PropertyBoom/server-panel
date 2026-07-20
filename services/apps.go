package services

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type AppStatus struct {
	Name       string   `json:"name"`
	Installed  bool     `json:"installed"`
	Manageable bool     `json:"manageable"`
	Running    bool     `json:"running"`
	Service    string   `json:"serviceName,omitempty"`
	Version    string   `json:"version"`
	Versions   []string `json:"versions,omitempty"`
}

type appDefinition struct {
	name        string
	binaries    []string
	services    []string
	versionArgs []string
}

var knownApps = []appDefinition{
	{name: "nginx", binaries: []string{"nginx"}, services: []string{"nginx.service"}, versionArgs: []string{"-v"}},
	{name: "mariadb", binaries: []string{"mariadbd", "mysqld", "mariadb"}, services: []string{"mariadb.service", "mysql.service"}, versionArgs: []string{"--version"}},
	{name: "redis", binaries: []string{"redis-server"}, services: []string{"redis-server.service", "redis.service"}, versionArgs: []string{"--version"}},
	{name: "docker", binaries: []string{"docker", "/usr/bin/docker", "/usr/local/bin/docker"}, services: []string{"docker.service"}, versionArgs: []string{"--version"}},
	{name: "podman", binaries: []string{"podman", "/usr/bin/podman", "/usr/local/bin/podman"}, versionArgs: []string{"--version"}},
	{name: "node", binaries: []string{"node", "nodejs", "/usr/bin/node", "/usr/local/bin/node"}, versionArgs: []string{"--version"}},
	{name: "php", services: allPHPServices()},
}

var semanticVersionPattern = regexp.MustCompile(`\d+(?:\.\d+){1,3}`)

func DetectApps() []AppStatus {
	statuses := make([]AppStatus, 0, len(knownApps))
	for _, app := range knownApps {
		installed := hasBinary(app.binaries)
		var versions []string
		version := ""
		if app.name == "php" {
			versions = installedPHPVersions()
			installed = len(versions) > 0
			version = strings.Join(versions, ", ")
			app.services = nil
			for _, version := range versions {
				app.services = append(app.services, phpServices(version)...)
			}
		}
		if installed && app.name != "php" {
			version = detectedAppVersion(app.binaries, app.versionArgs)
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
			Version:    version,
			Versions:   versions,
		})
	}
	return statuses
}

func detectedAppVersion(binaries, args []string) string {
	for _, name := range binaries {
		binary, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		output, _ := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		cancel()
		if version := semanticVersionPattern.FindString(string(output)); version != "" {
			return version
		}
	}
	return ""
}

var supportedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4"}

func installedPHPVersions() []string {
	genericVersion := genericPHPVersion()
	versions := make([]string, 0, len(supportedPHPVersions))
	for _, version := range supportedPHPVersions {
		if hasBinary(phpBinaries(version)) || genericVersion == version {
			versions = append(versions, version)
		}
	}
	return versions
}

func allPHPServices() []string {
	services := make([]string, 0, len(supportedPHPVersions)*4)
	for _, version := range supportedPHPVersions {
		services = append(services, phpServices(version)...)
	}
	return services
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
