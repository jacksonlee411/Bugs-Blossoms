#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

BASE_REF="${BASE_REF:-origin/main}"
KEEP_WORKTREE="${KEEP_WORKTREE:-0}"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: missing required command: $cmd" >&2
    return 1
  fi
}

if ! require_cmd git; then
  exit 1
fi

tmpdir="$(mktemp -d -t iota_preflight_pr_XXXXXX)"
cleanup() {
  if [[ "$KEEP_WORKTREE" == "1" ]]; then
    echo "KEEP_WORKTREE=1, skip cleanup: $tmpdir" >&2
    return 0
  fi
  git -C "$ROOT_DIR" worktree remove --force "$tmpdir" >/dev/null 2>&1 || true
  rm -rf "$tmpdir" >/dev/null 2>&1 || true
}
trap cleanup EXIT

git -C "$ROOT_DIR" worktree add --detach "$tmpdir" HEAD >/dev/null
cd "$tmpdir"

echo "==> Preflight (PR): worktree=$tmpdir base=$BASE_REF"

changed_files=""
if git show-ref --verify --quiet "refs/remotes/${BASE_REF#refs/remotes/}"; then
  base_sha="$(git merge-base HEAD "$BASE_REF" || true)"
  if [[ -n "$base_sha" ]]; then
    changed_files="$(git diff --name-only "$base_sha"..HEAD || true)"
  fi
fi

templ_changed="0"
locales_changed="0"
sql_changed="0"

if [[ -n "$changed_files" ]]; then
  if echo "$changed_files" | grep -Eq '(\.templ$|^tailwind\.config\.js$|/presentation/assets/)'; then
    templ_changed="1"
  fi
  if echo "$changed_files" | grep -Eq '^modules/.+/presentation/locales/.+\.json$'; then
    locales_changed="1"
  fi
  if echo "$changed_files" | grep -Eq '^modules/.+\.sql$'; then
    sql_changed="1"
  fi
fi

echo "==> Gate: new-doc"
make check doc

echo "==> Gate: templ fmt (must be clean)"
require_cmd templ
templ fmt .
if [[ -n "$(git diff --name-only)" ]]; then
  echo "Error: templ fmt produced diffs (commit the formatted .templ files)" >&2
  git diff >&2
  exit 1
fi

echo "==> Gate: go fmt (must be clean)"
packages="$(./scripts/go-active-packages.sh | tr '\n' ' ' || true)"
if [[ -n "$packages" ]]; then
  go fmt $packages
fi
if [[ -n "$(git diff --name-only)" ]]; then
  echo "Error: go fmt produced diffs (commit the formatted Go files)" >&2
  git diff >&2
  exit 1
fi

if [[ "$templ_changed" == "1" ]]; then
  echo "==> Gate: generate (must be committed)"
  make generate
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: make generate produced uncommitted artifacts" >&2
    git status --short >&2
    exit 1
  fi
fi

if [[ "$locales_changed" == "1" ]]; then
  echo "==> Gate: translations"
  make check tr
fi

if [[ "$sql_changed" == "1" ]]; then
  echo "==> Gate: sql formatting (pg_format)"
  make check sqlfmt
fi

echo "==> Gate: go vet"
if [[ -n "$packages" ]]; then
  go vet $packages
fi

if [[ "$templ_changed" == "1" ]]; then
  echo "==> Gate: css (must be committed)"
  require_cmd tailwindcss
  make css
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: make css produced uncommitted artifacts" >&2
    git status --short >&2
    exit 1
  fi
fi

echo "==> Gate: lint"
make check lint

echo "==> Gate: test"
make test

echo "OK: preflight passed"

