#!/usr/bin/env bash
set -euo pipefail

err() {
  echo "docs gate: $*" >&2
}

die() {
  err "$@"
  exit 1
}

is_kebab() {
  [[ "$1" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]
}

is_kebab_md() {
  [[ "$1" =~ ^[a-z0-9]+(-[a-z0-9]+)*\.md$ ]]
}

is_root_whitelisted_md() {
  case "$1" in
    README.MD|README.md|AGENTS.md|CLAUDE.md|GEMINI.md) return 0 ;;
    *) return 1 ;;
  esac
}

is_image_path() {
  local ext="${1##*.}"
  ext="${ext,,}"
  case "$ext" in
    png|jpg|jpeg|gif|webp|svg) return 0 ;;
    *) return 1 ;;
  esac
}

is_allowed_image_location() {
  local path="$1"
  [[ "$path" == docs/assets/* || "$path" =~ ^modules/[^/]+/docs/ ]]
}

validate_docs_assets_path() {
  local path="$1"
  local rel="${path#docs/assets/}"
  local part
  IFS='/' read -r -a parts <<< "$rel"
  for part in "${parts[@]}"; do
    if [[ "$part" == "" ]]; then
      continue
    fi
    if [[ "$part" == *.* ]]; then
      local base="${part%.*}"
      if ! is_kebab "$base"; then
        die "docs/assets 文件名必须使用 kebab-case（全小写），发现：$path"
      fi
    else
      if ! is_kebab "$part"; then
        die "docs/assets 目录名必须使用 kebab-case（全小写），发现：$path"
      fi
    fi
  done
}

base_ref=""
if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
  base_ref="origin/${GITHUB_BASE_REF}"
elif git show-ref --verify --quiet refs/remotes/origin/main; then
  base_ref="origin/main"
elif git show-ref --verify --quiet refs/heads/main; then
  base_ref="main"
else
  base_ref="$(git rev-parse HEAD^ 2>/dev/null || git rev-parse HEAD)"
fi

merge_base="$(git merge-base "$base_ref" HEAD 2>/dev/null || true)"
if [[ -z "$merge_base" ]]; then
  merge_base="$base_ref"
fi

diff_end="HEAD"
if [[ "${GITHUB_ACTIONS:-}" != "true" && -z "${CI:-}" ]]; then
  diff_end=""
fi

if [[ -n "$diff_end" ]]; then
  mapfile -t added_committed < <(git diff --name-only --diff-filter=A "$merge_base"...HEAD)
else
  mapfile -t added_committed < <(git diff --name-only --diff-filter=A "$merge_base")
fi
mapfile -t added_untracked < <(git ls-files --others --exclude-standard)

declare -A seen=()
added=()
for f in "${added_committed[@]}" "${added_untracked[@]}"; do
  [[ -n "$f" ]] || continue
  if [[ -z "${seen[$f]:-}" ]]; then
    seen[$f]=1
    added+=("$f")
  fi
done

if [[ "${#added[@]}" -eq 0 ]]; then
  echo "docs gate: no new files detected"
  exit 0
fi

agents_content="$(cat AGENTS.md)"

for path in "${added[@]}"; do
  if is_image_path "$path"; then
    if ! is_allowed_image_location "$path"; then
      die "图片/图表必须放在 docs/assets/ 或 modules/{module}/docs/：$path"
    fi
    if [[ "$path" == docs/assets/* ]]; then
      validate_docs_assets_path "$path"
    fi
    continue
  fi

  if [[ "${path,,}" != *.md ]]; then
    continue
  fi

  if [[ "$path" != */* ]]; then
    if ! is_root_whitelisted_md "$path"; then
      die "禁止在仓库根目录新增 .md（白名单：README.MD/AGENTS.md/CLAUDE.md/GEMINI.md），发现：$path"
    fi
    continue
  fi

  if [[ "$path" =~ ^modules/[^/]+/README\.md$ ]]; then
    continue
  fi
  if [[ "$path" =~ ^modules/[^/]+/docs/ ]]; then
    continue
  fi

  if [[ "$path" == docs/runbooks/* ]]; then
    file="${path#docs/runbooks/}"
    if ! is_kebab_md "$file"; then
      die "docs/runbooks 新增文档必须使用 kebab-case.md：$path"
    fi
    if [[ "$agents_content" != *"$path"* ]]; then
      die "新增仓库级文档必须在 AGENTS.md 的 Doc Map/专题入口中链接：$path"
    fi
    continue
  fi

  if [[ "$path" == docs/guides/* ]]; then
    file="${path#docs/guides/}"
    if ! is_kebab_md "$file"; then
      die "docs/guides 新增文档必须使用 kebab-case.md：$path"
    fi
    if [[ "$agents_content" != *"$path"* ]]; then
      die "新增仓库级文档必须在 AGENTS.md 的 Doc Map/专题入口中链接：$path"
    fi
    continue
  fi

  if [[ "$path" == docs/Archived/* ]]; then
    file="${path#docs/Archived/}"
    if ! is_kebab_md "$file"; then
      die "docs/Archived 新增文档必须使用 kebab-case.md：$path"
    fi
    if [[ "$agents_content" != *"$path"* ]]; then
      die "新增仓库级文档必须在 AGENTS.md 的 Doc Map/专题入口中链接：$path"
    fi
    continue
  fi

  if [[ "$path" == docs/assets/* ]]; then
    validate_docs_assets_path "$path"
    continue
  fi

  if [[ "$path" == docs/ARCHITECTURE.md ]]; then
    if [[ "$agents_content" != *"$path"* ]]; then
      die "新增仓库级文档必须在 AGENTS.md 的 Doc Map/专题入口中链接：$path"
    fi
    continue
  fi

  if [[ "$path" == docs/dev-plans/* || "$path" == docs/dev-records/* ]]; then
    continue
  fi
done

echo "docs gate: OK"
