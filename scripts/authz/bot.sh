#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel)
CMD=${1:-run}
shift || true

if [[ -f "$ROOT_DIR/.env.local" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env.local"
  set +a
fi

run_bot() {
  local remote=${AUTHZ_BOT_GIT_REMOTE:-origin}
  local original_url
  original_url=$(git -C "$ROOT_DIR" config --get "remote.${remote}.url" 2>/dev/null || true)
  local restore=0
  if [[ -n "${AUTHZ_BOT_GIT_TOKEN:-}" ]]; then
    local target_url=${AUTHZ_BOT_GIT_REMOTE_URL:-$original_url}
    if [[ "$target_url" == git@github.com:* ]]; then
      target_url=${target_url/git@github.com:/https://github.com/}
    fi
    target_url=${target_url%.git}
    target_url="https://${AUTHZ_BOT_GIT_TOKEN}@${target_url#https://}"
    git -C "$ROOT_DIR" remote set-url "$remote" "$target_url"
    restore=1
  fi
  (cd "$ROOT_DIR" && go run ./cmd/authzbot "$@")
  if [[ $restore -eq 1 ]]; then
    git -C "$ROOT_DIR" remote set-url "$remote" "$original_url"
  fi
}

case "$CMD" in
  run)
    run_bot "$@"
    ;;
  force-release)
    if [[ $# -lt 1 ]]; then
      echo "usage: $0 force-release <request-id>" >&2
      exit 1
    fi
    (cd "$ROOT_DIR" && go run ./cmd/authzbot -force-release "$1")
    ;;
  *)
    echo "unknown command: $CMD" >&2
    exit 1
    ;;
esac
