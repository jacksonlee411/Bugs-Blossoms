# DEV-PLAN-032 Readiness

> 目标：记录 DEV-PLAN-032（Org 权限映射与业务关联）落地后的本地门禁执行结果与最小冒烟路径，作为 CI 对齐与回归依据。

## 变更摘要
- 迁移（Org）：
  - `migrations/org/20251219195000_org_security_group_mappings_and_links.sql`
  - 新表：`org_security_group_mappings`、`org_links`
- API（Org）：
  - `GET/POST /org/api/security-group-mappings`
  - `POST /org/api/security-group-mappings/{id}:rescind`
  - `GET/POST /org/api/links`
  - `POST /org/api/links/{id}:rescind`
  - `GET /org/api/permission-preview`
- batch 扩展（对齐 026）：
  - `security_group_mapping.create` / `security_group_mapping.rescind`
  - `link.create` / `link.rescind`
- feature flags（默认 `false`）：
  - `ORG_SECURITY_GROUP_MAPPINGS_ENABLED`
  - `ORG_LINKS_ENABLED`
  - `ORG_PERMISSION_PREVIEW_ENABLED`

## 环境信息（本次执行）
- 日期（UTC）：2025-12-19T12:38:45Z
- 分支：`feature/dev-plan-032-impl`
- Git Revision：`f80a8cab46b70ecb92ba7192385f4900974a8416`
- Go：`go1.24.10 linux/amd64`
- 工作区状态：本次变更未提交；门禁命令执行通过

## 门禁执行记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-19 | 本地 | `go fmt ./...` | 格式化无 diff | 通过 | OK |
| 2025-12-19 | 本地 | `go vet ./...` | 无 vet 报错 | 通过 | OK |
| 2025-12-19 | 本地 | `make check lint` | golangci-lint + cleanarchguard 通过 | `0 issues.`；`go-cleanarch 检查通过。` | OK |
| 2025-12-19 | 本地 | `make test` | 测试全通过 | 通过 | OK |
| 2025-12-19 | 本地 | `make org migrate up DB_NAME=iota_erp_org_atlas DB_PORT=5440` | 迁移可执行 | 迁移到 `20251219195000` | OK |
| 2025-12-19 | 本地 | `make org plan DB_NAME=iota_erp_org_atlas DB_PORT=5440` | Atlas diff 结果可解释（无 drift） | diff 仅包含 `DROP TABLE __org_migration_smoke` 与 `DROP TABLE goose_db_version_org` | OK |
| 2025-12-19 | 本地 | `make org lint DB_NAME=iota_erp_org_atlas DB_PORT=5440` | `atlas migrate lint --env org_ci` 通过 | 通过 | OK |
| 2025-12-19 | 本地 | `make authz-pack` | 聚合策略生成且无 diff | 通过（生成 `config/access/policy.csv*`） | OK |
| 2025-12-19 | 本地 | `make authz-test` | authz 单测通过 | 通过 | OK |
| 2025-12-19 | 本地 | `make authz-lint` | authz fixture parity 通过 | `fixture parity passed` | OK |
| 2025-12-19 | 本地 | `make check doc` | 文档门禁通过 | `docs gate: OK` | OK |

## 最小冒烟（示例）

> 说明：以下示例假设 `ORG_ROLLOUT_MODE=enabled` 且当前 tenant 在 `ORG_ROLLOUT_TENANTS`；并且有可用的 `Authorization`（`sid` 或其它可被 `pkg/middleware.Authorize` 识别的 token）。

1) 开启 feature flags（示例）：
- `export ORG_SECURITY_GROUP_MAPPINGS_ENABLED=true ORG_LINKS_ENABLED=true ORG_PERMISSION_PREVIEW_ENABLED=true`

2) 创建 security group mapping（Insert）：
- `curl -sS -X POST "$BASE_URL/org/api/security-group-mappings" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"org_node_id":"<uuid>","security_group_key":"wd:HRBP","applies_to_subtree":true,"effective_date":"2025-03-01"}'`

3) 查询 mapping（as-of）：
- `curl -sS "$BASE_URL/org/api/security-group-mappings?org_node_id=<uuid>&effective_date=2025-03-01" -H "Authorization: Bearer $TOKEN"`

4) 创建 link（Insert）：
- `curl -sS -X POST "$BASE_URL/org/api/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"org_node_id":"<uuid>","object_type":"cost_center","object_key":"CC-100","link_type":"uses","metadata":{},"effective_date":"2025-03-01"}'`

5) Permission preview（含继承来源 + links 截断）：
- `curl -sS "$BASE_URL/org/api/permission-preview?org_node_id=<uuid>&effective_date=2025-03-01&include=security_groups,links&limit_links=200" -H "Authorization: Bearer $TOKEN"`

6) Rescind（终止有效窗）：
- `curl -sS -X POST "$BASE_URL/org/api/security-group-mappings/<id>:rescind" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"effective_date":"2025-04-01","reason":"removed"}'`
- `curl -sS -X POST "$BASE_URL/org/api/links/<id>:rescind" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"effective_date":"2025-04-01","reason":"removed"}'`

7) batch dry-run 演练（示例）
- `curl -sS -X POST "$BASE_URL/org/api/batch" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"dry_run":true,"effective_date":"2025-03-01","commands":[{"type":"security_group_mapping.create","payload":{"org_node_id":"<uuid>","security_group_key":"wd:HRBP","applies_to_subtree":true}},{"type":"link.create","payload":{"org_node_id":"<uuid>","object_type":"cost_center","object_key":"CC-100","link_type":"uses","metadata":{}}}]}'`
