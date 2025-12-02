#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEFAULT_EXCLUDE_PATTERN="^github.com/iota-uz/iota-sdk/modules/(billing|crm|finance)"

if [[ "${GO_PACKAGE_EXCLUDE_PATTERN+x}" == "x" ]]; then
  EXCLUDE_PATTERN="${GO_PACKAGE_EXCLUDE_PATTERN}"
else
  EXCLUDE_PATTERN="${DEFAULT_EXCLUDE_PATTERN}"
fi

cd "$REPO_ROOT"

packages="$(GOWORK=off go list ./...)"
if [[ -n "${EXCLUDE_PATTERN}" ]]; then
  packages="$(printf "%s\n" "${packages}" | grep -Ev "${EXCLUDE_PATTERN}" || true)"
fi

if [[ -z "${packages}" ]]; then
  exit 0
fi

printf "%s\n" "${packages}"
