# Variables
-include .env
-include .env.local

COMPOSE_PROJECT_NAME ?= iota-sdk-dev

PG_PORT ?= 5438
REDIS_PORT ?= 6379

DB_HOST ?= localhost
DB_PORT ?= $(PG_PORT)
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_NAME ?= iota_erp

export COMPOSE_PROJECT_NAME PG_PORT REDIS_PORT DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME

TAILWIND_INPUT := modules/core/presentation/assets/css/main.css
TAILWIND_OUTPUT := modules/core/presentation/assets/css/main.min.css
ATLAS_BIN_DIR ?= $(shell go env GOPATH)/bin
ATLAS_VERSION ?= v0.38.0
ATLAS ?= $(ATLAS_BIN_DIR)/atlas
GOOSE_BIN_DIR ?= $(shell go env GOPATH)/bin
GOOSE_VERSION ?= v3.26.0
GOOSE ?= $(GOOSE_BIN_DIR)/goose

.PHONY: authz-pack
authz-pack:
	go run ./scripts/authz/pack

.PHONY: authz-test
authz-test:
	go test ./pkg/authz ./scripts/authz/internal/...

.PHONY: authz-lint
authz-lint: authz-pack
	go run ./scripts/authz/verify --fixtures config/access/fixtures/testdata.yaml

# Install dependencies
deps:
	go get ./...

.PHONY: atlas-install
atlas-install:
	mkdir -p $(ATLAS_BIN_DIR)
	rm -rf /tmp/atlas-src-install
	git clone --depth 1 --branch $(ATLAS_VERSION) https://github.com/ariga/atlas.git /tmp/atlas-src-install
	cd /tmp/atlas-src-install/cmd/atlas && GOWORK=off go mod tidy
	cd /tmp/atlas-src-install/cmd/atlas && GOWORK=off go build -o $(ATLAS_BIN_DIR)/atlas .
	$(ATLAS) version

.PHONY: goose-install
goose-install:
	GOWORK=off go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	$(GOOSE) -version

.PHONY: dev-env
dev-env:
	./scripts/setup-worktree.sh

# Generate code documentation
docs:
	go run cmd/command/main.go doc --dir . --out docs/LLMS.md --recursive --exclude "vendor,node_modules,tmp,e2e,cmd"

# Template generation with optional subcommands (generate, watch)
generate:
	@$(MAKE) sqlc-generate
	@if [ "$(word 2,$(MAKECMDGOALS))" = "watch" ]; then \
		templ generate --watch; \
	else \
		go generate ./... && templ generate; \
	fi


# Docker compose management with subcommands (up, down, restart, logs)
compose:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "up" ]; then \
		docker compose -f compose.dev.yml up; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "down" ]; then \
		docker compose -f compose.dev.yml down; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "restart" ]; then \
		docker compose -f compose.dev.yml down && docker compose -f compose.dev.yml up; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "logs" ]; then \
		docker compose -f compose.dev.yml logs -f; \
	else \
		echo "Usage: make compose [up|down|restart|logs]"; \
		echo "  up      - Start all development services"; \
		echo "  down    - Stop all development services"; \
		echo "  restart - Stop and start all services"; \
		echo "  logs    - Follow logs from all services"; \
	fi

# Database management with subcommands (local, stop, clean, reset, seed, migrate)
db:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "local" ]; then \
		docker compose -f compose.dev.yml up -d db; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "stop" ]; then \
		docker compose -f compose.dev.yml stop db; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "clean" ]; then \
		docker compose -f compose.dev.yml down -v; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "reset" ]; then \
		docker compose -f compose.dev.yml down -v && docker compose -f compose.dev.yml up -d db; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "seed" ]; then \
		go run cmd/command/main.go seed; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "migrate" ]; then \
		if [ "${PERSON_MIGRATIONS}" = "1" ]; then \
			./scripts/db/run_goose.sh $(word 3,$(MAKECMDGOALS)); \
		else \
			go run cmd/command/main.go migrate $(word 3,$(MAKECMDGOALS)); \
		fi; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "rls-role" ]; then \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable" -v ON_ERROR_STOP=1 -f scripts/db/ensure_iota_app_role.sql; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "plan" ]; then \
		TARGET_DB_NAME="$(DB_NAME)"; \
		DB_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"; \
		DEV_DB_NAME="$${ATLAS_DEV_DB_NAME:-person_dev}"; \
		DEV_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$$DEV_DB_NAME?sslmode=disable"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$TARGET_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$TARGET_DB_NAME"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$DEV_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$DEV_DB_NAME"; \
		TMP_SCHEMA="$$(mktemp -t person_atlas_schema_XXXXXX.sql)"; \
		trap 'rm -f "$$TMP_SCHEMA"' EXIT; \
		cat modules/person/infrastructure/atlas/core_deps.sql modules/person/infrastructure/persistence/schema/person-schema.sql > "$$TMP_SCHEMA"; \
		TO_URL="file:///$${TMP_SCHEMA#/}"; \
		$(ATLAS) schema diff --from $$DB_URL --to $$TO_URL --dev-url $$DEV_URL --format '{{ sql . }}'; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "lint" ]; then \
		TARGET_DB_NAME="$(DB_NAME)"; \
		DB_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"; \
		DEV_DB_NAME="$${ATLAS_DEV_DB_NAME:-person_dev}"; \
		DEV_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$$DEV_DB_NAME?sslmode=disable"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$TARGET_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$TARGET_DB_NAME"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$DEV_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$DEV_DB_NAME"; \
		DB_URL="$$DB_URL" ATLAS_DEV_URL="$$DEV_URL" $(ATLAS) migrate lint --env ci --git-base origin/main; \
	else \
		echo "Usage: make db [local|stop|clean|reset|seed|migrate|rls-role]"; \
		echo "  local   - Start local PostgreSQL database"; \
		echo "  stop    - Stop database container"; \
		echo "  clean   - Remove postgres-data directory"; \
		echo "  reset   - Stop, clean, and restart local database"; \
		echo "  seed    - Seed database with test data"; \
		echo "  migrate - Run database migrations (up/down/redo/status)"; \
		echo "  rls-role - Create/update non-superuser DB role for RLS PoC"; \
		echo "  plan    - Dry-run Atlas diff for Person schema"; \
		echo "  lint    - Run Atlas lint (ci env)"; \
	fi

# Org Atlas+Goose management with subcommands (plan, lint, migrate)
org:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "plan" ]; then \
		TARGET_DB_NAME="$(DB_NAME)"; \
		DB_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"; \
		DEV_DB_NAME="$${ATLAS_DEV_DB_NAME:-org_dev}"; \
		DEV_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$$DEV_DB_NAME?sslmode=disable"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$TARGET_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$TARGET_DB_NAME TEMPLATE template0"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$DEV_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$DEV_DB_NAME TEMPLATE template0"; \
		TMP_SCHEMA="$$(mktemp -t org_atlas_schema_XXXXXX.sql)"; \
		trap 'rm -f "$$TMP_SCHEMA"' EXIT; \
		cat modules/org/infrastructure/atlas/core_deps.sql modules/org/infrastructure/persistence/schema/org-schema.sql > "$$TMP_SCHEMA"; \
		TO_URL="file:///$${TMP_SCHEMA#/}"; \
		$(ATLAS) schema diff --from $$DB_URL --to $$TO_URL --dev-url $$DEV_URL --format '{{ sql . }}'; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "lint" ]; then \
		TARGET_DB_NAME="$(DB_NAME)"; \
		DB_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"; \
		DEV_DB_NAME="$${ATLAS_DEV_DB_NAME:-org_dev}"; \
		DEV_URL="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$$DEV_DB_NAME?sslmode=disable"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$TARGET_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$TARGET_DB_NAME TEMPLATE template0"; \
		PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$$DEV_DB_NAME'" | grep -q 1 || PGPASSWORD="$(DB_PASSWORD)" psql "postgres://$(DB_USER)@$(DB_HOST):$(DB_PORT)/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE $$DEV_DB_NAME TEMPLATE template0"; \
		DB_URL="$$DB_URL" ATLAS_DEV_URL="$$DEV_URL" $(ATLAS) migrate lint --env org_ci --git-base origin/main; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "migrate" ]; then \
		GOOSE_MIGRATIONS_DIR="migrations/org" GOOSE_TABLE="$${GOOSE_TABLE:-goose_db_version_org}" ./scripts/db/run_goose.sh $(word 3,$(MAKECMDGOALS)); \
	else \
		echo "Usage: make org [plan|lint|migrate]"; \
		echo "  plan        - Dry-run Atlas diff for Org schema"; \
		echo "  lint        - Run Atlas lint (org_ci env)"; \
		echo "  migrate     - Run goose migrations (up/down/redo/status)"; \
	fi

.PHONY: org-perf-dataset
org-perf-dataset:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:-00000000-0000-0000-0000-000000000001}"; \
	SCALE="$${SCALE:-1k}"; \
	SEED="$${SEED:-42}"; \
	PROFILE="$${PROFILE:-balanced}"; \
	BACKEND="$${BACKEND:-db}"; \
	APPLY_FLAG=""; \
	if [ "$${APPLY:-0}" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	go run ./cmd/org-perf dataset apply \
	  --tenant "$$TENANT_ID" \
	  --scale "$$SCALE" \
	  --seed "$$SEED" \
	  --profile "$$PROFILE" \
	  --backend "$$BACKEND" \
	  $$APPLY_FLAG

.PHONY: org-perf-bench
org-perf-bench:
	@set -eu; \
	mkdir -p ./tmp/org-perf; \
	TENANT_ID="$${TENANT_ID:-00000000-0000-0000-0000-000000000001}"; \
	SCALE="$${SCALE:-1k}"; \
	SEED="$${SEED:-42}"; \
	PROFILE="$${PROFILE:-balanced}"; \
	BACKEND="$${BACKEND:-db}"; \
	ITERATIONS="$${ITERATIONS:-200}"; \
	WARMUP="$${WARMUP:-50}"; \
	CONCURRENCY="$${CONCURRENCY:-1}"; \
	EFFECTIVE_DATE="$${EFFECTIVE_DATE:-}"; \
	BASE_URL="$${BASE_URL:-http://localhost:3200}"; \
	OUTPUT="$${OUTPUT:-./tmp/org-perf/report.json}"; \
	ED_FLAG=""; \
	if [ -n "$$EFFECTIVE_DATE" ]; then ED_FLAG="--effective-date $$EFFECTIVE_DATE"; fi; \
	go run ./cmd/org-perf bench tree \
	  --tenant "$$TENANT_ID" \
	  --scale "$$SCALE" \
	  --seed "$$SEED" \
	  --profile "$$PROFILE" \
	  --backend "$$BACKEND" \
	  --iterations "$$ITERATIONS" \
	  --warmup "$$WARMUP" \
	  --concurrency "$$CONCURRENCY" \
	  --base-url "$$BASE_URL" \
	  --output "$$OUTPUT" \
	  $$ED_FLAG

.PHONY: org-load-smoke
org-load-smoke:
	@set -eu; \
	BASE_URL="$${BASE_URL:-http://localhost:3200}"; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	SID="$${SID:?SID is required}"; \
	go run ./cmd/org-load smoke \
	  --base-url "$$BASE_URL" \
	  --tenant "$$TENANT_ID" \
	  --sid "$$SID"

.PHONY: org-load-run
org-load-run:
	@set -eu; \
	mkdir -p ./tmp/org-load; \
	BASE_URL="$${BASE_URL:-http://localhost:3200}"; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	SID="$${SID:?SID is required}"; \
	PROFILE="$${PROFILE:-org_read_1k}"; \
	EFFECTIVE_DATE="$${EFFECTIVE_DATE:-$$(date -u +%F)}"; \
	OUTPUT="$${OUTPUT:-./tmp/org-load/org_load_report_$${PROFILE}_$$(date -u +%Y%m%dT%H%M%SZ).json}"; \
	P99_LIMIT_MS="$${P99_LIMIT_MS:-}"; \
	PARENT_NODE_ID="$${PARENT_NODE_ID:-}"; \
	EXTRA_FLAGS=""; \
	if [ -n "$$P99_LIMIT_MS" ]; then EXTRA_FLAGS="$$EXTRA_FLAGS --p99-limit-ms $$P99_LIMIT_MS"; fi; \
	if [ -n "$$PARENT_NODE_ID" ]; then EXTRA_FLAGS="$$EXTRA_FLAGS --parent-node-id $$PARENT_NODE_ID"; fi; \
	go run ./cmd/org-load run \
	  --profile "$$PROFILE" \
	  --base-url "$$BASE_URL" \
	  --tenant "$$TENANT_ID" \
	  --sid "$$SID" \
	  --effective-date "$$EFFECTIVE_DATE" \
	  --out "$$OUTPUT" \
	  $$EXTRA_FLAGS; \
	echo "wrote $$OUTPUT"

.PHONY: org-snapshot-build
org-snapshot-build:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	HIERARCHY_TYPE="$${HIERARCHY_TYPE:-OrgUnit}"; \
	AS_OF_DATE="$${AS_OF_DATE:-$$(date -u +%F)}"; \
	REQ_ID="$${REQUEST_ID:-}"; \
	APPLY_FLAG=""; \
	if [ "$${APPLY:-0}" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	go run ./cmd/org-deep-read snapshot build \
	  --tenant "$$TENANT_ID" \
	  --hierarchy "$$HIERARCHY_TYPE" \
	  --as-of-date "$$AS_OF_DATE" \
	  $$APPLY_FLAG \
	  $${REQ_ID:+--request-id "$$REQ_ID"}

.PHONY: org-closure-build
org-closure-build:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	HIERARCHY_TYPE="$${HIERARCHY_TYPE:-OrgUnit}"; \
	REQ_ID="$${REQUEST_ID:-}"; \
	APPLY_FLAG=""; \
	if [ "$${APPLY:-0}" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	go run ./cmd/org-deep-read closure build \
	  --tenant "$$TENANT_ID" \
	  --hierarchy "$$HIERARCHY_TYPE" \
	  $$APPLY_FLAG \
	  $${REQ_ID:+--request-id "$$REQ_ID"}

.PHONY: org-closure-activate
org-closure-activate:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	HIERARCHY_TYPE="$${HIERARCHY_TYPE:-OrgUnit}"; \
	BUILD_ID="$${BUILD_ID:?BUILD_ID is required}"; \
	go run ./cmd/org-deep-read closure activate \
	  --tenant "$$TENANT_ID" \
	  --hierarchy "$$HIERARCHY_TYPE" \
	  --build-id "$$BUILD_ID"

.PHONY: org-closure-prune
org-closure-prune:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	HIERARCHY_TYPE="$${HIERARCHY_TYPE:-OrgUnit}"; \
	KEEP="$${KEEP:-2}"; \
	go run ./cmd/org-deep-read closure prune \
	  --tenant "$$TENANT_ID" \
	  --hierarchy "$$HIERARCHY_TYPE" \
	  --keep "$$KEEP"

.PHONY: org-reporting-build
org-reporting-build:
	@set -eu; \
	TENANT_ID="$${TENANT_ID:?TENANT_ID is required}"; \
	HIERARCHY_TYPE="$${HIERARCHY_TYPE:-OrgUnit}"; \
	AS_OF_DATE="$${AS_OF_DATE:-$$(date -u +%F)}"; \
	REQ_ID="$${REQUEST_ID:-}"; \
	APPLY_FLAG=""; \
	if [ "$${APPLY:-0}" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	INCLUDE_SG=""; \
	INCLUDE_LINKS=""; \
	if [ "$${INCLUDE_SECURITY_GROUPS:-0}" = "1" ]; then INCLUDE_SG="--include-security-groups"; fi; \
	if [ "$${INCLUDE_LINKS:-0}" = "1" ]; then INCLUDE_LINKS="--include-links"; fi; \
	go run ./cmd/org-reporting build \
	  --tenant "$$TENANT_ID" \
	  --hierarchy "$$HIERARCHY_TYPE" \
	  --as-of-date "$$AS_OF_DATE" \
	  $$INCLUDE_SG \
	  $$INCLUDE_LINKS \
	  $$APPLY_FLAG \
	  $${REQ_ID:+--request-id "$$REQ_ID"}

# Run tests with optional subcommands (test, watch, coverage, verbose, package, docker)
test:
	@if [ "$(word 1,$(MAKECMDGOALS))" != "test" ]; then \
		exit 0; \
	fi
	@if [ "$(word 2,$(MAKECMDGOALS))" = "watch" ]; then \
		GOWORK=off gow test -v $$(./scripts/go-test-packages.sh); \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "coverage" ]; then \
		./scripts/run-go-tests.sh -v -coverprofile=./coverage/coverage.out; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "verbose" ]; then \
		./scripts/run-go-tests.sh -v; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "docker" ]; then \
		docker compose -f compose.testing.yml up --build erp_local; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "score" ]; then \
		go tool cover -func coverage.out | grep "total:" | awk '{print ((int($$3) > 80) != 1) }'; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "report" ]; then \
		go tool cover -html=coverage.out -o ./coverage/cover.html; \
	else \
		./scripts/run-go-tests.sh; \
	fi

# Compile TailwindCSS with optional subcommands (css, watch, dev, clean)
css:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "watch" ]; then \
		tailwindcss -c tailwind.config.js -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --minify --watch; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "dev" ]; then \
		tailwindcss -c tailwind.config.js -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT); \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "clean" ]; then \
		rm -rf $(TAILWIND_OUTPUT); \
	else \
		tailwindcss -c tailwind.config.js -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --minify; \
	fi

# E2E testing management with subcommands (setup, test, reset, seed, run, clean)
e2e:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "test" ]; then \
		go run cmd/command/main.go e2e test; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "reset" ]; then \
		go run cmd/command/main.go e2e reset; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "seed" ]; then \
		go run cmd/command/main.go e2e seed; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "migrate" ]; then \
		go run cmd/command/main.go e2e migrate; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "run" ]; then \
		cd e2e && npx playwright test --ui; \
		elif [ "$(word 2,$(MAKECMDGOALS))" = "ci" ]; then \
			cd e2e && npx playwright test --workers=1 --reporter=list; \
		elif [ "$(word 2,$(MAKECMDGOALS))" = "clean" ]; then \
			go run cmd/command/main.go e2e drop; \
		elif [ "$(word 2,$(MAKECMDGOALS))" = "dev" ]; then \
			PORT=3201 ORIGIN='http://default.localhost:3201' DB_NAME=iota_erp_e2e ENABLE_TEST_ENDPOINTS=true ORG_ROLLOUT_MODE=enabled ORG_ROLLOUT_TENANTS=00000000-0000-0000-0000-000000000001 air; \
		else \
			echo "Usage: make e2e [test|reset|seed|migrate|run|ci|dev|clean]"; \
			echo "  test         - Set up database and run all e2e tests"; \
			echo "  reset        - Drop and recreate e2e database with fresh data"; \
		echo "  seed         - Seed e2e database with test data"; \
		echo "  migrate      - Run migrations on e2e database"; \
		echo "  run          - Open Playwright interactive mode (UI mode)"; \
		echo "  ci           - Run Playwright tests in CI mode (headless, no UI, serial execution)"; \
		echo "  dev          - Start e2e development server with hot reload on port 3201"; \
		echo "  clean        - Drop e2e database"; \
	fi

# Build and release management
build:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "local" ]; then \
		go build -ldflags="-s -w" -o run_server cmd/server/main.go; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "linux" ]; then \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/run_server cmd/server/main.go; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "docker-base" ]; then \
		docker buildx build --push --platform linux/amd64,linux/arm64 -t iotauz/sdk:base-$v --target base .; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "docker-prod" ]; then \
		docker buildx build --push --platform linux/amd64,linux/arm64 -t iotauz/sdk:$v --target production .; \
	else \
		echo "Usage: make build [local|linux|docker-base|docker-prod]"; \
		echo "  local       - Build for local OS"; \
		echo "  linux       - Build for Alpine Linux (production)"; \
		echo "  docker-base - Build and push base Docker image"; \
		echo "  docker-prod - Build and push production Docker image"; \
	fi

# Super Admin server management with subcommands (default, dev, seed)
superadmin:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "dev" ]; then \
		PORT=4000 DOMAIN='localhost:4000' ORIGIN='http://localhost:4000' air -c .air.superadmin.toml; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "seed" ]; then \
		LOG_LEVEL=info go run cmd/command/main.go seed_superadmin; \
	else \
		PORT=4000 DOMAIN='localhost:4000' go run cmd/superadmin/main.go; \
	fi

# Dependency graph generation
graph:
	goda graph ./modules/... | dot -Tpng -o dependencies.png

# Auto-fix code formatting and imports
fix:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "fmt" ]; then \
		go fmt ./... && templ fmt .; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "imports" ]; then \
		find . -name '*.go' -not -name '*_templ.go' -exec goimports -w {} +; \
	else \
		echo "Usage: make fix [fmt|imports]"; \
		echo "  fmt     - Format Go code and templates"; \
		echo "  imports - Organize and format Go imports"; \
	fi

# Code quality checks with subcommands (lint, tr, doc, routing, sqlfmt)
check:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "lint" ]; then \
		GOFLAGS="-buildvcs=false" golangci-lint run ./... && \
		go run ./cmd/cleanarchguard -config .gocleanarch.yml; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "tr" ]; then \
		go run cmd/command/main.go check_tr_keys && \
		go run cmd/command/main.go check_tr_usage; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "doc" ]; then \
		bash ./scripts/docs/check.sh; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "routing" ]; then \
		go test ./pkg/routing ./internal/routelint ./internal/routinggates; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "sqlfmt" ]; then \
		set -eu; \
		if ! command -v pg_format >/dev/null 2>&1; then \
			echo "Error: pg_format not found. Install pgformatter (e.g. apt-get install pgformatter) to run SQL formatting gate."; \
			exit 1; \
		fi; \
		tmpdir="$$(mktemp -d -t sqlfmt_XXXXXX)"; \
		trap 'rm -rf "$$tmpdir"' EXIT; \
		find modules -name "*.sql" -type f > "$$tmpdir/files"; \
		while IFS= read -r sqlfile; do \
			[ -n "$$sqlfile" ] || continue; \
			tmpfile="$$tmpdir/$$(basename "$$sqlfile")"; \
			pg_format "$$sqlfile" > "$$tmpfile"; \
			if ! diff -q "$$sqlfile" "$$tmpfile" > /dev/null; then \
				echo "Error: $$sqlfile is not properly formatted"; \
				diff "$$sqlfile" "$$tmpfile"; \
				exit 1; \
			fi; \
		done < "$$tmpdir/files"; \
		echo "All SQL files are properly formatted"; \
	else \
		echo "Usage: make check [lint|tr|doc|routing|sqlfmt]"; \
		echo "  lint - Run golangci-lint (checks for unused variables/functions)"; \
		echo "  tr   - Check translations for completeness"; \
		echo "  doc  - Check new-doc gate (paths/naming/AGENTS links)"; \
		echo "  routing - Check routing quality gates (DEV-PLAN-018B)"; \
		echo "  sqlfmt - Check SQL formatting (pg_format, aligned with CI)"; \
	fi

# sdk-tools CLI management
.PHONY: sdk-tools
sdk-tools:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "install" ]; then \
		echo "Installing sdk-tools..."; \
		cd .claude/tools && GOWORK=off go install .; \
		echo "✓ Installed sdk-tools to $$(go env GOPATH)/bin/sdk-tools"; \
		echo "Make sure $$(go env GOPATH)/bin is in your PATH"; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "test" ]; then \
		echo "Running sdk-tools tests..."; \
		cd .claude/tools && GOWORK=off go test ./... -v; \
	elif [ "$(word 2,$(MAKECMDGOALS))" = "help" ]; then \
		sdk-tools --help; \
	else \
		echo "Usage: make sdk-tools [install|test|help]"; \
		echo "  install - Install sdk-tools globally to \$$GOPATH/bin"; \
		echo "  test    - Run tests"; \
		echo "  help    - Show sdk-tools help"; \
	fi

# Cloudflared tunnel
tunnel:
	cloudflared tunnel --url http://localhost:3200 --loglevel debug

# Clean build artifacts
clean:
	@if [ "$(firstword $(MAKECMDGOALS))" = "db" ]; then \
		:; \
	else \
		rm -rf $(TAILWIND_OUTPUT); \
	fi

# Development watch mode - run templ and tailwind in watch mode concurrently
dev:
	@if [ "$(word 2,$(MAKECMDGOALS))" = "watch" ]; then \
		echo "Starting development watch mode (templ + tailwind)..."; \
		trap 'kill %1 %2 2>/dev/null; exit' INT TERM; \
		templ generate --watch & \
		tailwindcss -c tailwind.config.js -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --watch & \
		wait; \
	else \
		echo "Usage: make dev [watch]"; \
		echo "  watch - Run templ and tailwind in watch mode concurrently"; \
	fi

# Full setup
setup: deps css
	make fix fmt
	make fix imports
	make check lint

# Prevents make from treating the argument as an undefined target
watch coverage verbose docker score report ci linux docker-base docker-prod up down redo status restart logs local stop reset seed migrate plan lint rls-role install help imports doc routing sqlfmt:
	@:

.PHONY: deps db org test css compose setup e2e build graph docs tunnel clean generate check fix superadmin \
        down restart logs local stop reset watch coverage verbose docker score report \
        dev fmt lint tr doc routing sqlfmt dev-env linux docker-base docker-prod run server sdk-tools install help atlas-install goose-install plan preflight

# One-shot local preflight aligned with CI "Code Quality & Formatting" (+ tests)
preflight:
	bash ./scripts/dev/preflight_pr.sh
# HRM sqlc generation
sqlc-generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.28.0 generate -f sqlc.yaml
	gofmt -w modules/person/infrastructure/sqlc modules/org/infrastructure/sqlc
	go run golang.org/x/tools/cmd/goimports@v0.26.0 -w modules/person/infrastructure/sqlc modules/org/infrastructure/sqlc
