#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel)
SCHEMA_PATH="$ROOT_DIR/modules/hrm/infrastructure/sqlc/schema.sql"
TABLES=(
  positions
  employees
  employee_meta
  employee_positions
  employee_contacts
)

DB_NAME=${DB_NAME:-iota_erp}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres}
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}

if [[ "${SKIP_MIGRATE:-0}" != "1" ]]; then
  (cd "$ROOT_DIR" && make db migrate up)
  (cd "$ROOT_DIR" && HRM_MIGRATIONS=1 make db migrate up)
fi

get_server_major() {
  local server_version
  server_version=$(
    PGPASSWORD="${DB_PASSWORD}" psql \
      --dbname="${DB_NAME}" \
      --host="${DB_HOST}" \
      --port="${DB_PORT}" \
      --username="${DB_USER}" \
      -tAc "SHOW server_version" \
      | tr -d '[:space:]'
  )
  echo "${server_version%%.*}"
}

get_pg_dump_major() {
  local version
  version=$(pg_dump --version | awk '{print $3}')
  echo "${version%%.*}"
}

run_pg_dump() {
  local target_file=$1

  local server_major
  server_major=$(get_server_major)

  if command -v pg_dump >/dev/null 2>&1; then
    local client_major
    client_major=$(get_pg_dump_major)
    if [[ "${client_major}" == "${server_major}" ]]; then
      PGPASSWORD="${DB_PASSWORD}" pg_dump \
        --schema-only \
        --no-owner \
        --no-privileges \
        "${TABLE_FLAGS[@]}" \
        --dbname="${DB_NAME}" \
        --host="${DB_HOST}" \
        --port="${DB_PORT}" \
        --username="${DB_USER}" \
        | awk 'substr($0,1,1) != "\\" { print }' > "${target_file}"
      return 0
    fi
  fi

  if command -v docker >/dev/null 2>&1; then
    local container_id
    container_id=$(
      docker ps --format '{{.ID}} {{.Image}} {{.Ports}}' | \
        awk -v version="postgres:${server_major}" -v port=":${DB_PORT}->5432" '$2 == version && $0 ~ port { print $1; exit }'
    )
    if [[ -z "${container_id}" ]]; then
      container_id=$(docker ps --filter "ancestor=postgres:${server_major}" --format '{{.ID}}' | head -n 1)
    fi
    if [[ -n "${container_id}" ]]; then
      docker exec -e PGPASSWORD="${DB_PASSWORD}" "${container_id}" pg_dump \
        --schema-only \
        --no-owner \
        --no-privileges \
        "${TABLE_FLAGS[@]}" \
        --dbname="${DB_NAME}" \
        --host="localhost" \
        --port="5432" \
        --username="${DB_USER}" \
        | awk 'substr($0,1,1) != "\\" { print }' > "${target_file}"
      return 0
    fi
  fi

  echo "pg_dump server/client major version mismatch and no suitable postgres:${server_major} container found." >&2
  echo "Hint: install PostgreSQL client ${server_major} or run against a local postgres:${server_major} container." >&2
  exit 1
}

TMP_FILE=$(mktemp)
TMP_FORMATTED_FILE=$(mktemp)
trap 'rm -f "$TMP_FILE" "$TMP_FORMATTED_FILE"' EXIT

TABLE_FLAGS=()
for table in "${TABLES[@]}"; do
  TABLE_FLAGS+=("--table=$table")
done

run_pg_dump "$TMP_FILE"

if command -v pg_format >/dev/null 2>&1; then
  pg_format "$TMP_FILE" > "$TMP_FORMATTED_FILE"
  mv "$TMP_FORMATTED_FILE" "$SCHEMA_PATH"
else
  mv "$TMP_FILE" "$SCHEMA_PATH"
  echo "Warning: pg_format not found, exported schema may fail CI SQL formatting check." >&2
fi
trap - EXIT

echo "Exported HRM schema to $SCHEMA_PATH"
