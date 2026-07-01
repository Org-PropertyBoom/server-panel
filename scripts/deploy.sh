#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the script's directory
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT_DIR}"

# 1. Build everything locally
./build.sh --no-push

# 2. Push everything to git using the existing push script
./push.sh "$@"
