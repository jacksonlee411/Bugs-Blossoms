#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mapfile -t packages < <("$SCRIPT_DIR/go-test-packages.sh")

if [[ ${#packages[@]} -eq 0 ]]; then
  echo "No Go packages to test (all packages filtered)." >&2
  exit 0
fi

GOWORK=off go test "$@" "${packages[@]}"
