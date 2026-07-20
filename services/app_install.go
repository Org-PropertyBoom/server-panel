package services

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var appPackages = map[string]map[string][][]string{
	"apt-get": {
		"caddy": {{"caddy"}}, "nginx": {{"nginx"}}, "mariadb": {{"mariadb-server"}}, "php": {{"php-fpm", "php-cli"}}, "redis": {{"redis-server"}},
		"docker": {{"docker.io"}, {"docker-ce"}}, "podman": {{"podman"}}, "node": {{"nodejs"}},
	},
	"dnf": {
		"caddy": {{"caddy"}}, "nginx": {{"nginx"}}, "mariadb": {{"mariadb-server"}}, "php": {{"php-fpm", "php-cli"}}, "redis": {{"redis"}},
		"docker": {{"docker"}, {"moby-engine"}, {"docker-ce"}}, "podman": {{"podman"}}, "node": {{"nodejs"}},
	},
	"yum": {
		"caddy": {{"caddy"}}, "nginx": {{"nginx"}}, "mariadb": {{"mariadb-server"}}, "php": {{"php-fpm", "php-cli"}}, "redis": {{"redis"}},
		"docker": {{"docker"}, {"moby-engine"}, {"docker-ce"}}, "podman": {{"podman"}}, "node": {{"nodejs"}},
	},
	"pacman": {
		"caddy": {{"caddy"}}, "nginx": {{"nginx"}}, "mariadb": {{"mariadb"}}, "php": {{"php", "php-fpm"}}, "redis": {{"redis"}},
		"docker": {{"docker"}}, "podman": {{"podman"}}, "node": {{"nodejs-lts-jod", "npm"}},
	},
}

func InstallApp(name string) error {
	manager := packageManager()
	if manager == "" {
		return errors.New("no supported package manager found")
	}
	if name == "node" && manager != "pacman" {
		if err := configureNodeSource22(manager); err != nil {
			return err
		}
	}
	options := appPackages[manager][name]
	if len(options) == 0 {
		return errors.New("unsupported app")
	}
	if manager == "apt-get" {
		cmd := exec.Command(manager, "update")
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("package index update failed: %s", conciseCommandOutput(output))
		}
	}
	var lastOutput []byte
	for _, packages := range options {
		args := installArguments(manager, packages)
		cmd := exec.Command(manager, args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastOutput = output
	}
	return fmt.Errorf("app installation failed: %s", conciseCommandOutput(lastOutput))
}

func configureNodeSource22(manager string) error {
	url := "https://rpm.nodesource.com/setup_22.x"
	if manager == "apt-get" {
		url = "https://deb.nodesource.com/setup_22.x"
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Get(url)
	if err != nil {
		return fmt.Errorf("Node.js 22 repository setup download failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Node.js 22 repository setup returned HTTP %d", response.StatusCode)
	}
	file, err := os.CreateTemp("", "mthan-node22-setup-*.sh")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := io.Copy(file, io.LimitReader(response.Body, 2<<20)); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if output, err := exec.Command("bash", path).CombinedOutput(); err != nil {
		return fmt.Errorf("Node.js 22 repository setup failed: %s", conciseCommandOutput(output))
	}
	return nil
}

func packageManager() string {
	for _, manager := range []string{"apt-get", "dnf", "yum", "pacman"} {
		if _, err := exec.LookPath(manager); err == nil {
			return manager
		}
	}
	return ""
}

func installArguments(manager string, packages []string) []string {
	if manager == "pacman" {
		return append([]string{"-Sy", "--noconfirm", "--needed"}, packages...)
	}
	return append([]string{"install", "-y"}, packages...)
}

func conciseCommandOutput(output []byte) string {
	value := strings.TrimSpace(string(output))
	if len(value) > 1000 {
		value = value[len(value)-1000:]
	}
	if value == "" {
		return "package manager returned an error"
	}
	return value
}
