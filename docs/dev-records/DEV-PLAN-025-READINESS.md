# DEV-PLAN-025 Readiness

本记录用于复现 `docs/dev-plans/025-org-time-and-audit.md` 的落地路径（冻结窗口 + 审计表 + Correct/Rescind/ShiftBoundary）。

## 1. 本次交付范围

- 新增表（Org）：`org_settings`、`org_audit_logs`（并已同步到 schema SSOT）
  - 迁移：`migrations/org/20251218130000_org_settings_and_audit.sql`
  - schema：`modules/org/infrastructure/persistence/schema/org-schema.sql`
- 冻结窗口：按 `org_settings.freeze_mode/freeze_grace_days`（默认 enforce + 3 天宽限期）
- 新增 API（JSON-only，internal API）：
  - `POST /org/api/nodes/{id}:correct`
  - `POST /org/api/nodes/{id}:rescind`
  - `POST /org/api/nodes/{id}:shift-boundary`
  - `POST /org/api/nodes/{id}:correct-move`
  - `POST /org/api/assignments/{id}:correct`
  - `POST /org/api/assignments/{id}:rescind`
- 审计：写路径在事务内落盘 `org_audit_logs`（含 freeze meta 与 operation 标记）

## 2. 本地验证命令

- Go 门禁：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`
- Org Atlas/Goose（命中 `migrations/org/**` 与 Org schema SSOT）：
  - `atlas migrate hash --dir file://migrations/org --dir-format goose`
  - `make org lint`
  - `make org migrate up`

