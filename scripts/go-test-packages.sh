#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -n "${GO_TEST_EXCLUDE_PATTERN:-}" ]]; then
  export GO_PACKAGE_EXCLUDE_PATTERN="${GO_TEST_EXCLUDE_PATTERN}"
fi

exec "${SCRIPT_DIR}/go-active-packages.sh"
