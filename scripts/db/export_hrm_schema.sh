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
fi

TMP_FILE=$(mktemp)
trap 'rm -f "$TMP_FILE"' EXIT

TABLE_FLAGS=()
for table in "${TABLES[@]}"; do
  TABLE_FLAGS+=("--table=$table")
fi

PGPASSWORD="${DB_PASSWORD}" pg_dump \
  --schema-only \
  --no-owner \
  --no-privileges \
  "${TABLE_FLAGS[@]}" \
  --dbname="${DB_NAME}" \
  --host="${DB_HOST}" \
  --port="${DB_PORT}" \
  --username="${DB_USER}" \
  > "$TMP_FILE"

mv "$TMP_FILE" "$SCHEMA_PATH"
trap - EXIT

echo "Exported HRM schema to $SCHEMA_PATH"
