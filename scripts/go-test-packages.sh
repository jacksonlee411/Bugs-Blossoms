#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EXCLUDE_PATTERN="${GO_TEST_EXCLUDE_PATTERN:-^github.com/iota-uz/iota-sdk/modules/finance}"

cd "$REPO_ROOT"

packages="$(GOWORK=off go list ./... | grep -Ev "$EXCLUDE_PATTERN" || true)"

if [[ -z "$packages" ]]; then
  exit 0
fi

printf "%s\n" "$packages"
