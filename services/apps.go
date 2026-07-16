package services

import (
	"context"
	"os/exec"
	"time"
)

type AppStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
}

type appDefinition struct {
	name     string
	binaries []string
	services []string
}

var knownApps = []appDefinition{
	{name: "nginx", binaries: []string{"nginx"}, services: []string{"nginx.service"}},
	{name: "mariadb", binaries: []string{"mariadbd", "mysqld", "mariadb"}, services: []string{"mariadb.service", "mysql.service"}},
	{name: "php", binaries: []string{"php-fpm", "php-fpm8.4", "php-fpm8.3", "php-fpm8.2", "php"}, services: []string{"php8.4-fpm.service", "php8.3-fpm.service", "php8.2-fpm.service", "php-fpm.service"}},
	{name: "redis", binaries: []string{"redis-server"}, services: []string{"redis-server.service", "redis.service"}},
}

func DetectApps() []AppStatus {
	statuses := make([]AppStatus, 0, len(knownApps))
	for _, app := range knownApps {
		installed := hasBinary(app.binaries)
		statuses = append(statuses, AppStatus{
			Name:      app.name,
			Installed: installed,
			Running:   installed && hasActiveService(app.services),
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

func hasActiveService(names []string) bool {
	for _, name := range names {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", name).Run()
		cancel()
		if err == nil {
			return true
		}
	}
	return false
}
