#!/usr/bin/env bash
set -euo pipefail

APP_NAME="ppt-server-panel"
CTL_NAME="pptctl"
SERVICE_NAME="ppt-server-panel"
DIST_URL="${DIST_URL:-https://cdn.jsdelivr.net/gh/Org-PropertyBoom/server-panel-dist@main/public/dist}"
BINARY_URL="${BINARY_URL:-${DIST_URL}/${APP_NAME}}"
CTL_BINARY_URL="${CTL_BINARY_URL:-${DIST_URL}/${CTL_NAME}}"
INSTALL_PATH="${INSTALL_PATH:-/usr/local/bin/${APP_NAME}}"
CTL_INSTALL_PATH="${CTL_INSTALL_PATH:-/usr/local/bin/${CTL_NAME}}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}@.service"
ROOT_ADDR="${ROOT_ADDR:-:2205}"
ROOT_SERVICE_UNIT=""
REINSTALL=0

usage() {
  cat <<EOF
Usage: install.sh [--reinstall]

Options:
  --reinstall   Stop old service instances, replace binaries, recreate service files, and restart root service.
  -h, --help    Show this help message.

Environment:
  ROOT_ADDR         Root panel bind address. Default: ${ROOT_ADDR}
  DIST_URL          Base URL for both binaries. Default: ${DIST_URL}
  BINARY_URL        Full URL for ${APP_NAME}. Default: ${BINARY_URL}
  CTL_BINARY_URL    Full URL for ${CTL_NAME}. Default: ${CTL_BINARY_URL}
  INSTALL_PATH      Install path for ${APP_NAME}. Default: ${INSTALL_PATH}
  CTL_INSTALL_PATH  Install path for ${CTL_NAME}. Default: ${CTL_INSTALL_PATH}
EOF
}

parse_args() {
  for arg in "$@"; do
    case "${arg}" in
      --reinstall)
        REINSTALL=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "unknown argument: ${arg}" >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

require_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    return
  fi

  echo "${SERVICE_NAME} installer must be run as root" >&2
  exit 1
}

require_command() {
  local command_name="$1"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
}

has_libcrypt() {
  if command -v ldconfig >/dev/null 2>&1 && ldconfig -p 2>/dev/null | grep -q 'libcrypt\.so\.1'; then
    return 0
  fi

  [[ -e /lib/libcrypt.so.1 ]] ||
    [[ -e /usr/lib/libcrypt.so.1 ]] ||
    [[ -e /lib64/libcrypt.so.1 ]] ||
    [[ -e /usr/lib64/libcrypt.so.1 ]] ||
    [[ -e /lib/x86_64-linux-gnu/libcrypt.so.1 ]] ||
    [[ -e /usr/lib/x86_64-linux-gnu/libcrypt.so.1 ]]
}

install_libcrypt() {
  if has_libcrypt; then
    return
  fi

  echo "Missing runtime dependency: libcrypt.so.1"

  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y libcrypt1
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y libxcrypt-compat
  elif command -v yum >/dev/null 2>&1; then
    yum install -y libxcrypt-compat
  elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm libxcrypt-compat
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache libxcrypt-compat
  else
    echo "unable to install libcrypt.so.1 automatically; install libcrypt1 or libxcrypt-compat and rerun this script" >&2
    exit 1
  fi

  if ! has_libcrypt; then
    echo "libcrypt.so.1 is still missing after package installation" >&2
    exit 1
  fi
}

install_caddy() {
  if command -v caddy >/dev/null 2>&1; then
    echo "Caddy is already installed"
  elif command -v apt-get >/dev/null 2>&1; then
    echo "Installing Caddy from the official Debian repository"
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl gnupg
    local key_file
    key_file="$(mktemp)"
    curl -1fsSL "https://dl.cloudsmith.io/public/caddy/stable/gpg.key" -o "${key_file}"
    gpg --batch --yes --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg "${key_file}"
    rm -f "${key_file}"
    curl -1fsSL "https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt" -o /etc/apt/sources.list.d/caddy-stable.list
    chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y caddy
  elif command -v dnf >/dev/null 2>&1; then
    echo "Installing Caddy from the official COPR repository"
    dnf install -y dnf-plugins-core
    dnf -y copr enable @caddy/caddy
    dnf install -y caddy
  elif command -v yum >/dev/null 2>&1; then
    echo "Installing Caddy from the official COPR repository"
    yum install -y yum-plugin-copr
    yum -y copr enable @caddy/caddy
    yum install -y caddy
  elif command -v pacman >/dev/null 2>&1; then
    echo "Installing Caddy from the Arch repository"
    pacman -Sy --noconfirm --needed caddy
  else
    echo "unable to install required Caddy public server on this distribution" >&2
    exit 1
  fi

  if ! command -v caddy >/dev/null 2>&1; then
    echo "Caddy installation did not provide a caddy binary" >&2
    exit 1
  fi

  systemctl enable --now caddy
}

download_binary() {
  local name="$1"
  local url="$2"
  local install_path="$3"
  local tmp_file
  tmp_file="$(mktemp)"

  echo "Downloading ${name} from ${url}"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${url}" -o "${tmp_file}"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "${tmp_file}" "${url}"
  else
    echo "missing required command: curl or wget" >&2
    rm -f "${tmp_file}"
    exit 1
  fi

  install -m 0755 "${tmp_file}" "${install_path}"
  rm -f "${tmp_file}"
}

download_binaries() {
  download_binary "${APP_NAME}" "${BINARY_URL}" "${INSTALL_PATH}"
  download_binary "${CTL_NAME}" "${CTL_BINARY_URL}" "${CTL_INSTALL_PATH}"
}

create_service() {
  cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=Ppt Server Panel service for %I
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%I
ExecStart=${INSTALL_PATH}
Restart=on-failure
RestartSec=3
Environment=APP_ENV=production
EnvironmentFile=-/etc/${SERVICE_NAME}/%i.env

[Install]
WantedBy=multi-user.target
EOF
}

write_service_env() {
  local env_file="/etc/${SERVICE_NAME}/root.env"

  mkdir -p "/etc/${SERVICE_NAME}"

  cat >"${env_file}" <<EOF
APP_ADDR=${ROOT_ADDR}
EOF
}

start_service() {
  systemctl daemon-reload
  write_service_env
  ROOT_SERVICE_UNIT="${SERVICE_NAME}@root.service"
  systemctl enable "${ROOT_SERVICE_UNIT}"
  if [[ "${REINSTALL}" == "1" ]]; then
    systemctl restart "${ROOT_SERVICE_UNIT}"
  else
    systemctl start "${ROOT_SERVICE_UNIT}"
  fi
}

resolve_root_url() {
  local port
  port=$(echo "${ROOT_ADDR}" | sed 's/.*://')

  local ip=""
  # Try to fetch public IP (with timeout to prevent hanging)
  if command -v curl >/dev/null 2>&1; then
    ip=$(curl -fsSL --connect-timeout 2 https://ipinfo.io/ip 2>/dev/null || true)
  elif command -v wget >/dev/null 2>&1; then
    ip=$(wget -qO- --timeout=2 https://ipinfo.io/ip 2>/dev/null || true)
  fi

  # Fallback to local IP if public IP failed
  if [[ -z "${ip}" ]]; then
    if command -v hostname >/dev/null 2>&1; then
      ip=$(hostname -I | awk '{print $1}' || true)
    fi
  fi

  # Ultimate fallback
  if [[ -z "${ip}" ]]; then
    ip="<YOUR_SERVER_IP>"
  fi

  echo "http://${ip}:${port}"
}

cleanup_old_install() {
  echo "Cleaning up old installation..."

  # Stop and disable active/loaded service instances of ppt-server-panel or the
  # legacy mthan-vps / vps services (so a rename/reinstall retires the old ones).
  local units
  units=$(
    {
      systemctl list-units --type=service --all --plain --no-legend --no-pager "${SERVICE_NAME}@*.service" "mthan-vps@*.service" "vps@*.service" 2>/dev/null | awk '{print $1}'
      systemctl list-unit-files --type=service --no-legend --no-pager 2>/dev/null | awk '{print $1}'
    } | grep -E "^(${SERVICE_NAME}|mthan-vps|vps)@[^.]+\\.service$" | sort -u
  ) || true
  for unit in ${units}; do
    if [[ -n "${unit}" ]]; then
      echo "Stopping service: ${unit}"
      systemctl stop "${unit}" || true
      echo "Disabling service: ${unit}"
      systemctl disable "${unit}" || true
    fi
  done

  # Clean up service files (current + legacy names)
  rm -f "/etc/systemd/system/${SERVICE_NAME}@.service"
  rm -f "/etc/systemd/system/mthan-vps@.service"
  rm -f "/etc/systemd/system/vps@.service"

  # Clean up binaries (current + legacy names)
  rm -f "/usr/local/bin/vps"
  rm -f "/usr/local/bin/mthan-vps"
  rm -f "/usr/local/bin/mthanctl"
  rm -f "${INSTALL_PATH}"
  rm -f "${CTL_INSTALL_PATH}"

  systemctl daemon-reload || true
}

main() {
  parse_args "$@"

  require_root
  require_command install
  require_command mktemp
  require_command systemctl

  install_libcrypt
  install_caddy

  if [[ "${REINSTALL}" == "1" ]]; then
    cleanup_old_install
  fi
  download_binaries
  create_service
  start_service

  local root_url
  root_url=$(resolve_root_url)

  echo "${SERVICE_NAME} installed successfully"
  echo "Binary: ${INSTALL_PATH}"
  echo "Control binary: ${CTL_INSTALL_PATH}"
  echo "Service template: ${SERVICE_FILE}"
  echo "Root service instance: ${ROOT_SERVICE_UNIT}"
  echo "Root service URL: ${root_url}"
}

main "$@"
