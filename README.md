# Ppt Server Panel

Simple Go API with a React client.

## Structure

```text
.
├── client/       # React app
├── routes/       # HTTP route registration
│   ├── api/      # Public API routes
│   └── post/     # Root-only localhost/internal POST routes
├── scripts/      # Shell scripts
├── services/     # Business logic, router, and startup services
├── main.go       # Go entrypoint
└── go.mod
```

## Install service

```sh
curl -fsSL https://cdn.jsdelivr.net/gh/Org-PropertyBoom/server-panel-dist@main/public/install.sh | sudo bash
```

The installer must be run as `root`. It downloads `ppt-server-panel` and `pptctl`, installs them to `/usr/local/bin`, creates the systemd service template, writes the root service environment file, and starts the root service.

The app verifies Linux user passwords against `/etc/shadow`, so the binary needs `libcrypt.so.1` at runtime. The installer checks for it and installs the matching package when possible:

```text
Debian/Ubuntu: libcrypt1
RHEL/Fedora/Amazon Linux: libxcrypt-compat
Arch Linux: libxcrypt-compat
Alpine: libxcrypt-compat
```

Default download source:

```text
https://cdn.jsdelivr.net/gh/Org-PropertyBoom/server-panel-dist@main/public/dist
```

Installed files:

```text
/usr/local/bin/ppt-server-panel
/usr/local/bin/pptctl
/etc/systemd/system/ppt-server-panel@.service
/etc/ppt-server-panel/root.env
```

Installed service:

```text
ppt-server-panel@root.service  public root panel and root helper, APP_ADDR=:2205
```

After installation, the script prints the root panel URL, usually:

```text
http://<SERVER_IP>:2205
```

User activation is managed from the root panel after install. Root-only `/post/*` routes reject cross-origin public calls: they accept requests from the same host as the root panel or from localhost.

### Reinstall

Use `--reinstall` to stop old `ppt-server-panel@*.service` and legacy `vps@*.service` instances, remove old service files and binaries, install fresh binaries, and restart the root service:

```sh
curl -fsSL https://cdn.jsdelivr.net/gh/Org-PropertyBoom/server-panel-dist@main/public/install.sh | sudo bash -s -- --reinstall
```

### Installer options

The installer supports:

```text
--reinstall   Stop old service instances, replace binaries, recreate service files, and restart root service.
-h, --help    Show installer help.
```

Environment overrides:

```sh
ROOT_ADDR=":2205" \
DIST_URL="https://cdn.jsdelivr.net/gh/Org-PropertyBoom/server-panel-dist@main/public/dist" \
INSTALL_PATH="/usr/local/bin/ppt-server-panel" \
CTL_INSTALL_PATH="/usr/local/bin/pptctl" \
sudo -E bash public/install.sh
```

Supported variables:

```text
ROOT_ADDR         Root panel bind address. Default: :2205
DIST_URL          Base URL for both binaries.
BINARY_URL        Full URL for ppt-server-panel. Overrides DIST_URL for the app binary.
CTL_BINARY_URL    Full URL for pptctl. Overrides DIST_URL for the control binary.
INSTALL_PATH      Install path for ppt-server-panel.
CTL_INSTALL_PATH  Install path for pptctl.
```

### Service management

```sh
sudo systemctl status ppt-server-panel@root.service
sudo systemctl restart ppt-server-panel@root.service
sudo journalctl -u ppt-server-panel@root.service -f
```

If systemd reports `error while loading shared libraries: libcrypt.so.1`, install the runtime package and restart.

Debian/Ubuntu:

```sh
sudo apt-get update && sudo apt-get install -y libcrypt1
sudo systemctl restart ppt-server-panel@root.service
```

RHEL/Fedora/Amazon Linux:

```sh
sudo dnf install -y libxcrypt-compat
sudo systemctl restart ppt-server-panel@root.service
```

Arch Linux:

```sh
sudo pacman -Sy --noconfirm libxcrypt-compat
sudo systemctl restart ppt-server-panel@root.service
```
