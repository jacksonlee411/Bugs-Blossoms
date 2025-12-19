# DEV-PLAN-033 Readiness

> 目标：记录 DEV-PLAN-033（Org 可视化与高级报告）落地后的本地门禁执行结果与最小冒烟路径，作为 CI 对齐与回归依据。

## 变更摘要
- API（Org）：
  - `GET /org/api/hierarchies:export`
  - `GET /org/api/nodes/{id}:path`
  - `GET /org/api/reports/person-path`
- 迁移（Org）：
  - `migrations/org/20251219220000_org_reporting_nodes_and_view.sql`
  - 新增：`org_reporting_nodes`、`org_reporting`（active snapshot build 视图）
- CLI / Make：
  - `go run ./cmd/org-reporting build ...`
  - `make org-reporting-build`

## 门禁命令记录（触发器对齐）

### Go / Lint / Test
- `go fmt ./...`
- `go vet ./...`
- `make check lint`
- `make test`

### Org Atlas/Goose（命中 `migrations/org/**` 与 Org schema SSOT）
- `atlas migrate hash --dir file://migrations/org --dir-format goose`
- `make org plan ...`
- `make org lint ...`
- `make org migrate up ...`

### 文档门禁
- `make check doc`

## 最小冒烟（示例）

> 说明：以下示例假设 `ORG_ROLLOUT_MODE=enabled` 且当前 tenant 在 `ORG_ROLLOUT_TENANTS`；并且有可用的 `Authorization`（`sid` 或其它可被 `pkg/middleware.Authorize` 识别的 token）。

1) 节点路径：
- `curl -sS "$BASE_URL/org/api/nodes/<node_uuid>:path?effective_date=2025-01-01&format=nodes_with_sources" -H "Authorization: Bearer $TOKEN"`

2) 人员路径：
- `curl -sS "$BASE_URL/org/api/reports/person-path?subject=person:000123&effective_date=2025-01-01" -H "Authorization: Bearer $TOKEN"`

3) 导出（分页）：
- `curl -sS "$BASE_URL/org/api/hierarchies:export?type=OrgUnit&effective_date=2025-01-01&include=edges&limit=2000" -H "Authorization: Bearer $TOKEN"`

4) 报表 build（dry-run / apply）：
- `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 make org-snapshot-build`
- `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 APPLY=1 make org-snapshot-build`
- `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 make org-reporting-build`
- `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 APPLY=1 make org-reporting-build`

## 结果

- `go fmt ./...`：PASS
- `go vet ./...`：PASS（本环境需设置 `GOCACHE=./tmp/gocache`，否则写入 `~/.cache/go-build` 会触发 permission denied）
- `make check lint`：PASS（本环境需设置 `GOCACHE=./tmp/gocache`、`GOLANGCI_LINT_CACHE=./tmp/golangci-lint-cache`）
- `make test`：PASS（Postgres 17 in docker：`localhost:5440`）
- `atlas migrate hash --dir file://migrations/org --dir-format goose`：PASS（`migrations/org/atlas.sum` 已更新）
- `make org plan`：PASS（使用 docker PG：`DB_HOST=localhost DB_PORT=5440 DB_NAME=iota_erp_org_atlas_033 ATLAS_DEV_DB_NAME=org_dev_033`；通过 `make org plan DB_NAME=...` 覆盖 `.env.local` 的 Makefile 变量）
- `make org lint`：PASS（同上 DB 配置）
- `make org migrate up`：PASS（同上 DB 配置；migrate 到 `20251219220000`）
- `make check doc`：PASS
