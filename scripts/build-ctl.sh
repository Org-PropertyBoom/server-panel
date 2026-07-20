#!/usr/bin/env bash
set -euo pipefail

APP_NAME="pptctl"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/public/dist}"
BINARY_PATH="${DIST_DIR}/${APP_NAME}"

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
GO_BUILD_FLAGS="${GO_BUILD_FLAGS:--buildvcs=false}"

require_command() {
  local command_name="$1"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
}

build_binary() {
  require_command go

  mkdir -p "${DIST_DIR}"

  echo "Building ${APP_NAME} binary for ${GOOS}/${GOARCH}"
  (
    cd "${ROOT_DIR}"
    GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED="${CGO_ENABLED}" \
      go build ${GO_BUILD_FLAGS} -tags ctl -o "${BINARY_PATH}" .
  )

  chmod 0755 "${BINARY_PATH}"
}

main() {
  build_binary
  echo "Build complete: ${BINARY_PATH}"
}

main "$@"
