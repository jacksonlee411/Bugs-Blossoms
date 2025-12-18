# DEV-PLAN-022 Readiness

本记录用于复现 `docs/dev-plans/022-org-placeholders-and-event-contracts.md` 的落地校验路径（schema/迁移/sqlc/门禁）。

## 1. 前置

- 启动本地 DB：`make db local`
- 选择独立 Org DB（避免与全量业务库混淆）：
  - `org_db`：用于 `make org migrate ...`
  - `org_db_dev`：用于 Atlas diff/lint 的 dev-db

## 2. 迁移生成（Atlas）

1. 创建/重置 dev-db（可选但推荐，保证干净）：
   - `PGPASSWORD=postgres psql 'postgres://postgres@localhost:${DB_PORT}/postgres?sslmode=disable' -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS org_db_dev" -c "CREATE DATABASE org_db_dev TEMPLATE template0"`
2. 组合 Org schema 源（core deps + org schema）：
   - `cat modules/org/infrastructure/atlas/core_deps.sql modules/org/infrastructure/persistence/schema/org-schema.sql > /tmp/org_atlas_schema.sql`
3. 生成 goose 迁移：
   - `atlas migrate diff org_placeholders_and_event_contracts --dir 'file://migrations/org' --dir-format goose --dev-url "postgres://postgres:postgres@localhost:${DB_PORT}/org_db_dev?sslmode=disable" --to 'file:///tmp/org_atlas_schema.sql'`
4. 更新 `atlas.sum`：
   - `atlas migrate hash --dir file://migrations/org`

## 3. 迁移执行与 lint（Org 工具链）

1. 执行 Org 迁移：
   - `make org migrate up DB_NAME=org_db`
2. Atlas lint（对齐 CI env `org_ci`）：
   - `ATLAS_DEV_DB_NAME=org_db_dev make org lint DB_NAME=org_db`

## 4. sqlc 生成

- `make sqlc-generate`
- 期望：生成物已提交，`git status --short` 无未跟踪/未提交改动。

## 5. Go 门禁（对齐仓库规则）

- `go fmt ./...`
- `go vet ./...`
- `make check lint`
- `make test`

## 6. 文档门禁

- `make check doc`

