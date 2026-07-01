#!/usr/bin/env bash
set -euo pipefail

APP_NAME="mthan-vps"
CTL_NAME="mthanctl"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${BIN_DIR:-${ROOT_DIR}/bin}"
BINARY_PATH="${BIN_DIR}/${APP_NAME}"
CTL_BINARY_PATH="${BIN_DIR}/${CTL_NAME}"

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-1}"
GO_BUILD_FLAGS="${GO_BUILD_FLAGS:--buildvcs=false}"

BUILD_CLIENT="${BUILD_CLIENT:-1}"
PUSH_DIST=1

DIST_REPO_DIR="${DIST_REPO_DIR:-}"
DIST_REPO_URL="${DIST_REPO_URL:-}"
DIST_REPO_BRANCH="${DIST_REPO_BRANCH:-}"
DIST_REPO_BIN_TARGET="${DIST_REPO_BIN_TARGET:-bin}"
DIST_REPO_CLIENT_TARGET="${DIST_REPO_CLIENT_TARGET:-dist/client}"
COMMIT_MESSAGE="${COMMIT_MESSAGE:-build: update ${APP_NAME} dist}"

TMP_REPO_DIR=""
DIST_REPO_WORKDIR=""

usage() {
  cat <<EOF
Usage: ./scripts/build.sh [--push|--no-push]

Environment:
  DIST_REPO_DIR      Local git repo to copy dist files into before pushing.
  DIST_REPO_URL      Git URL to clone and push if DIST_REPO_DIR is not set.
  DIST_REPO_BRANCH   Optional branch to checkout/clone.
  DIST_REPO_BIN_TARGET     Binary target directory inside dist repo. Default: bin
  DIST_REPO_CLIENT_TARGET  Client target directory inside dist repo. Default: dist/client
  BUILD_CLIENT       Build React client. Default: 1
  GOOS               Go target OS. Default: linux
  GOARCH             Go target arch. Default: amd64
EOF
}

cleanup() {
  if [[ -n "${TMP_REPO_DIR}" ]]; then
    rm -rf "${TMP_REPO_DIR}"
  fi
}

require_command() {
  local command_name="$1"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
}

parse_args() {
  for arg in "$@"; do
    case "${arg}" in
      --push)
        PUSH_DIST=1
        ;;
      --no-push)
        PUSH_DIST=0
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "unknown argument: ${arg}" >&2
        usage
        exit 1
        ;;
    esac
  done
}

build_client() {
  if [[ "${BUILD_CLIENT}" != "1" ]]; then
    echo "Skipping React client build"
    return
  fi

  require_command npm

  echo "Building React client"
  (
    cd "${ROOT_DIR}/client"

    if [[ -f package-lock.json ]]; then
      npm ci
    else
      npm install
    fi

    npm run build
  )
}

build_binaries() {
  require_command go

  mkdir -p "${BIN_DIR}"

  echo "Running Go tests"
  (
    cd "${ROOT_DIR}"
    go test . ./routes/... ./services/...
  )

  echo "Building ${APP_NAME} binary for ${GOOS}/${GOARCH}"
  (
    cd "${ROOT_DIR}"
    GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED="${CGO_ENABLED}" \
      go build ${GO_BUILD_FLAGS} -o "${BINARY_PATH}" .
  )

  chmod 0755 "${BINARY_PATH}"

  echo "Building ${CTL_NAME} binary for ${GOOS}/${GOARCH}"
  (
    cd "${ROOT_DIR}"
    GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED="${CGO_ENABLED}" \
      go build ${GO_BUILD_FLAGS} -tags ctl -o "${CTL_BINARY_PATH}" .
  )

  chmod 0755 "${CTL_BINARY_PATH}"
}

prepare_dist_repo() {
  require_command git

  if [[ -n "${DIST_REPO_DIR}" ]]; then
    if [[ ! -d "${DIST_REPO_DIR}/.git" ]]; then
      echo "DIST_REPO_DIR is not a git repo: ${DIST_REPO_DIR}" >&2
      exit 1
    fi

    DIST_REPO_WORKDIR="${DIST_REPO_DIR}"
    return
  fi

  if [[ -z "${DIST_REPO_URL}" ]]; then
    echo "set DIST_REPO_DIR or DIST_REPO_URL to push dist files" >&2
    exit 1
  fi

  TMP_REPO_DIR="$(mktemp -d)"

  if [[ -n "${DIST_REPO_BRANCH}" ]]; then
    git clone --branch "${DIST_REPO_BRANCH}" "${DIST_REPO_URL}" "${TMP_REPO_DIR}"
  else
    git clone "${DIST_REPO_URL}" "${TMP_REPO_DIR}"
  fi

  DIST_REPO_WORKDIR="${TMP_REPO_DIR}"
}

push_dist() {
  local repo_dir="$1"
  local bin_target_dir="${repo_dir}/${DIST_REPO_BIN_TARGET}"
  local client_target_dir="${repo_dir}/${DIST_REPO_CLIENT_TARGET}"

  if [[ -n "${DIST_REPO_BRANCH}" && -z "${TMP_REPO_DIR}" ]]; then
    git -C "${repo_dir}" checkout "${DIST_REPO_BRANCH}"
  fi

  git -C "${repo_dir}" pull --ff-only

  if [[ -n "$(git -C "${repo_dir}" status --porcelain)" ]]; then
    echo "dist repo has uncommitted changes: ${repo_dir}" >&2
    exit 1
  fi

  mkdir -p "${bin_target_dir}"
  install -m 0755 "${BINARY_PATH}" "${bin_target_dir}/${APP_NAME}"
  install -m 0755 "${CTL_BINARY_PATH}" "${bin_target_dir}/${CTL_NAME}"
  if [[ -f "${BIN_DIR}/version.json" ]]; then
    install -m 0644 "${BIN_DIR}/version.json" "${bin_target_dir}/version.json"
  fi

  if [[ -d "${ROOT_DIR}/client/build" ]]; then
    rm -rf "${client_target_dir}"
    mkdir -p "$(dirname "${client_target_dir}")"
    cp -R "${ROOT_DIR}/client/build" "${client_target_dir}"
  fi

  git -C "${repo_dir}" add "${DIST_REPO_BIN_TARGET}"
  if [[ -d "${ROOT_DIR}/client/build" ]]; then
    git -C "${repo_dir}" add "${DIST_REPO_CLIENT_TARGET}"
  fi

  if git -C "${repo_dir}" diff --cached --quiet; then
    echo "No dist changes to push"
    return
  fi

  git -C "${repo_dir}" commit -m "${COMMIT_MESSAGE}"
  git -C "${repo_dir}" push
}

main() {
  trap cleanup EXIT

  parse_args "$@"

  # Generate version.json with UTC build time
  local build_time
  build_time="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  mkdir -p "${BIN_DIR}"
  echo "{\"buildTime\":\"${build_time}\"}" > "${BIN_DIR}/version.json"
  echo "{\"buildTime\":\"${build_time}\"}" > "${ROOT_DIR}/client/public/version.json"
  if [[ -d "${ROOT_DIR}/client/build" ]]; then
    echo "{\"buildTime\":\"${build_time}\"}" > "${ROOT_DIR}/client/build/version.json"
  fi

  build_client
  build_binaries

  if [[ "${PUSH_DIST}" != "1" ]]; then
    echo "Build complete: ${BINARY_PATH}"
    echo "Build complete: ${CTL_BINARY_PATH}"
    return
  fi

  prepare_dist_repo
  push_dist "${DIST_REPO_WORKDIR}"

  echo "${APP_NAME} dist built and pushed successfully"
}

main "$@"
