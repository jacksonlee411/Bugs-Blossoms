#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/setup-worktree.sh [--force] [--project-name <name>] [--pg-port <port>] [--redis-port <port>] [--db-name <name>]

Generates/updates .env.local with shared local dev infrastructure defaults (single Postgres/Redis):
  - COMPOSE_PROJECT_NAME, PG_PORT, REDIS_PORT (shared across worktrees)
  - DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

By default it only fills missing keys and never overwrites existing values unless --force is provided.
EOF
}

force=0
explicit_project_name=""
explicit_pg_port=""
explicit_redis_port=""
explicit_db_name=""

while [ $# -gt 0 ]; do
	case "$1" in
	--force)
		force=1
		shift
		;;
	--project-name)
		explicit_project_name="${2:-}"
		shift 2
		;;
	--pg-port)
		explicit_pg_port="${2:-}"
		shift 2
		;;
	--redis-port)
		explicit_redis_port="${2:-}"
		shift 2
		;;
	--db-name)
		explicit_db_name="${2:-}"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown argument: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if ! command -v python3 >/dev/null 2>&1; then
	echo "python3 is required for env file updates." >&2
	exit 1
fi

env_file=".env.local"

read_existing_value() {
	local key="$1"
	if [ ! -f "$env_file" ]; then
		return 1
	fi
	python3 - "$env_file" "$key" <<'PY'
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
key = sys.argv[2]
pattern = re.compile(rf"^{re.escape(key)}=(.*)$")

for line in path.read_text(encoding="utf-8").splitlines():
	m = pattern.match(line.strip())
	if not m:
		continue
	value = m.group(1)
	if value == "":
		sys.exit(1)
	print(value)
	sys.exit(0)
sys.exit(1)
PY
}

project_name="${explicit_project_name}"
if [ -z "$project_name" ]; then
	if [ "$force" -eq 1 ]; then
		project_name="iota-sdk-dev"
	elif v="$(read_existing_value "COMPOSE_PROJECT_NAME" 2>/dev/null)"; then
		project_name="$v"
	else
		project_name="iota-sdk-dev"
	fi
fi

pg_port="${explicit_pg_port}"
if [ -z "$pg_port" ]; then
	if [ "$force" -eq 1 ]; then
		pg_port="5438"
	elif v="$(read_existing_value "PG_PORT" 2>/dev/null)"; then
		pg_port="$v"
	else
		pg_port="5438"
	fi
fi

redis_port="${explicit_redis_port}"
if [ -z "$redis_port" ]; then
	if [ "$force" -eq 1 ]; then
		redis_port="6379"
	elif v="$(read_existing_value "REDIS_PORT" 2>/dev/null)"; then
		redis_port="$v"
	else
		redis_port="6379"
	fi
fi

db_name="${explicit_db_name}"
if [ -z "$db_name" ]; then
	if [ "$force" -eq 1 ]; then
		db_name="iota_erp"
	elif v="$(read_existing_value "DB_NAME" 2>/dev/null)"; then
		db_name="$v"
	else
		db_name="iota_erp"
	fi
fi

export SETUP_FORCE="$force"
export SETUP_COMPOSE_PROJECT_NAME="$project_name"
export SETUP_PG_PORT="$pg_port"
export SETUP_REDIS_PORT="$redis_port"
export SETUP_DB_HOST="localhost"
export SETUP_DB_PORT="$pg_port"
export SETUP_DB_USER="postgres"
export SETUP_DB_PASSWORD="postgres"
export SETUP_DB_NAME="$db_name"

python3 - "$env_file" <<'PY'
import os
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
force = os.environ.get("SETUP_FORCE", "0") == "1"

desired = {
	"COMPOSE_PROJECT_NAME": os.environ["SETUP_COMPOSE_PROJECT_NAME"],
	"PG_PORT": os.environ["SETUP_PG_PORT"],
	"REDIS_PORT": os.environ["SETUP_REDIS_PORT"],
	"DB_HOST": os.environ["SETUP_DB_HOST"],
	"DB_PORT": os.environ["SETUP_DB_PORT"],
	"DB_USER": os.environ["SETUP_DB_USER"],
	"DB_PASSWORD": os.environ["SETUP_DB_PASSWORD"],
	"DB_NAME": os.environ["SETUP_DB_NAME"],
}

existing_lines = []
existing = {}

if path.exists():
	existing_lines = path.read_text(encoding="utf-8").splitlines()
	for line in existing_lines:
		line = line.strip()
		if not line or line.startswith("#") or "=" not in line:
			continue
		k, v = line.split("=", 1)
		existing[k.strip()] = v

keys = list(desired.keys())
pattern = re.compile(rf"^({'|'.join(re.escape(k) for k in keys)})=")

out_lines = []
seen = set()
for line in existing_lines:
	m = pattern.match(line.strip())
	if not m:
		out_lines.append(line)
		continue

	k = m.group(1)
	if k in seen:
		continue
	seen.add(k)

	if (not force) and (k in existing):
		if existing[k] == "":
			out_lines.append(f"{k}={desired[k]}")
		else:
			out_lines.append(f"{k}={existing[k]}")
	else:
		out_lines.append(f"{k}={desired[k]}")

missing = [k for k in keys if k not in seen and k not in existing]
if missing:
	if out_lines and out_lines[-1].strip():
		out_lines.append("")
	out_lines.append("# Local dev infra (shared across worktrees)")
	for k in keys:
		if k in existing or k in seen:
			continue
		out_lines.append(f"{k}={desired[k]}")

tmp = path.with_suffix(path.suffix + ".tmp")
tmp.write_text("\n".join(out_lines).rstrip() + "\n", encoding="utf-8")
tmp.replace(path)
PY

echo "Updated $env_file"
echo "  COMPOSE_PROJECT_NAME=$project_name"
echo "  PG_PORT=$pg_port"
echo "  REDIS_PORT=$redis_port"
echo "  DB_PORT=$pg_port"
echo "  DB_NAME=$db_name"
