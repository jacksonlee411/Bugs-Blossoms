#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mapfile -t packages < <("$SCRIPT_DIR/go-test-packages.sh")

if [[ ${#packages[@]} -eq 0 ]]; then
  echo "No Go packages to test (all packages filtered)." >&2
  exit 0
fi

has_timeout=0
for arg in "$@"; do
  if [[ "$arg" == -timeout || "$arg" == -timeout=* ]]; then
    has_timeout=1
    break
  fi
done

extra_args=()
if [[ $has_timeout -eq 0 ]]; then
  extra_args+=("-timeout=20m")
fi

GOWORK=off go test "${extra_args[@]}" "$@" "${packages[@]}"
