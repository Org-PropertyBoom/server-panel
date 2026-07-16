package services

import (
	"slices"
	"testing"
)

func TestKnownAppsIncludeContainerNodeAndPHPVersions(t *testing.T) {
	want := []string{"docker", "podman", "node", "php8.1", "php8.2", "php8.3", "php8.4"}
	found := make(map[string]appDefinition, len(knownApps))
	for _, app := range knownApps {
		found[app.name] = app
	}

	for _, name := range want {
		if _, ok := found[name]; !ok {
			t.Errorf("knownApps does not contain %q", name)
		}
	}
	if len(found["node"].services) != 0 {
		t.Error("Node.js must not expose system service controls")
	}
}

func TestPHPServicesCoverSupportedLinuxFamilies(t *testing.T) {
	services := phpServices("8.2")
	for _, service := range []string{
		"php8.2-fpm.service",    // Debian and Ubuntu
		"php82-php-fpm.service", // RHEL-family with Remi packages
		"php-fpm.service",       // Arch and RHEL-family default packages
	} {
		if !slices.Contains(services, service) {
			t.Errorf("phpServices does not contain %q", service)
		}
	}
}

func TestPHPBinariesCoverVersionedAndRemiLayouts(t *testing.T) {
	binaries := phpBinaries("8.2")
	for _, binary := range []string{
		"php8.2",
		"/usr/sbin/php-fpm8.2",
		"/opt/remi/php82/root/usr/sbin/php-fpm",
	} {
		if !slices.Contains(binaries, binary) {
			t.Errorf("phpBinaries does not contain %q", binary)
		}
	}
}
