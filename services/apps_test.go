package services

import (
	"slices"
	"testing"
)

func TestKnownAppsIncludeContainersNodeAndPHP(t *testing.T) {
	want := []string{"docker", "podman", "node", "php"}
	found := make(map[string]appDefinition, len(knownApps))
	for _, app := range knownApps {
		found[app.name] = app
	}

	for _, name := range want {
		if _, ok := found[name]; !ok {
			t.Errorf("knownApps does not contain %q", name)
		}
	}
}

func TestPodmanIsNotManagedAsRootService(t *testing.T) {
	for _, app := range knownApps {
		if app.name == "podman" && len(app.services) != 0 {
			t.Fatalf("Podman services = %v, want none for per-user rootless isolation", app.services)
		}
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

func TestSemanticVersionPattern(t *testing.T) {
	for _, test := range []struct {
		output string
		want   string
	}{
		{output: "nginx version: nginx/1.24.0", want: "1.24.0"},
		{output: "Docker version 27.3.1, build ce12230", want: "27.3.1"},
		{output: "v22.14.0", want: "22.14.0"},
	} {
		if got := semanticVersionPattern.FindString(test.output); got != test.want {
			t.Errorf("version from %q = %q, want %q", test.output, got, test.want)
		}
	}
}
