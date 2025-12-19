# DEV-PLAN-029 Readiness

本记录用于复现 `docs/dev-plans/029-org-closure-and-deep-read-optimization.md` 的最小可用交付（闭包表/快照表 + refresh 工具 + 读路径切换 + 一致性与 query budget 测试）与门禁验证路径。

## 1. 本次交付范围（MVP）

- Org 迁移（Goose / `migrations/org`）：
  - `20251219090000_org_hierarchy_closure_and_snapshots.sql`
  - 新增表：
    - `org_hierarchy_closure_builds` / `org_hierarchy_closure`
    - `org_hierarchy_snapshot_builds` / `org_hierarchy_snapshots`
- Go 侧能力：
  - refresh/build/activate/prune：`cmd/org-deep-read`
  - make 入口（SSOT）：`make org-snapshot-build` / `make org-closure-build` / `make org-closure-activate` / `make org-closure-prune`
  - 读路径开关：
    - `ORG_DEEP_READ_ENABLED=true|false`
    - `ORG_DEEP_READ_BACKEND=edges|closure|snapshot`
  - 一致性与 query budget（最小覆盖：祖先链/子树 + role assignments 祖先继承）：
    - `modules/org/services/org_029_deep_read_consistency_test.go`

## 2. 本地验证命令（触发器对齐）

- Go 门禁（全仓库）：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`（本地若因环境性能导致 package timeout，可先最小跑 `go test ./modules/org/services -run '^TestOrg029' -count=1`）
- Org Atlas+Goose：
  - `make org lint`
  - `make org migrate up`
- 文档门禁：
  - `make check doc`

## 3. Refresh 工具入口（Make / CLI）

> 默认 dry-run（不落库）；`APPLY=1` 才会写入并切换 active build。

- snapshot build：
  - `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 make org-snapshot-build`
  - `TENANT_ID=<tenant_uuid> AS_OF_DATE=2025-01-01 APPLY=1 make org-snapshot-build`
- closure build：
  - `TENANT_ID=<tenant_uuid> make org-closure-build`
  - `TENANT_ID=<tenant_uuid> APPLY=1 make org-closure-build`
- closure activate（回滚）：
  - `TENANT_ID=<tenant_uuid> BUILD_ID=<build_uuid> make org-closure-activate`
- closure prune：
  - `TENANT_ID=<tenant_uuid> KEEP=2 make org-closure-prune`

## 4. 灰度/回滚开关

- 一键回滚到 baseline（优先）：
  - `ORG_DEEP_READ_ENABLED=false` 或 `ORG_DEEP_READ_BACKEND=edges`
- build 指针回滚（closure/snapshot 已支持 active build）：
  - closure：`make org-closure-activate` 切回上一 build
  - snapshot：重新 `make org-snapshot-build APPLY=1`（按同 `as_of_date` 覆盖 active build）

## 5. 实际跑通记录（2025-12-19）

- 环境：
  - Go：`go version go1.24.10 linux/amd64`
  - Postgres：17.7（docker `postgres:17`，`DB_HOST=localhost` `DB_PORT=5439`）
  - DB：`DB_NAME=iota_erp_015b4`（以 `.env/.env.local` 为准）
- Org 迁移：
  - `make org lint`：通过
  - `make org migrate up`：通过（包含 `20251219090000_org_hierarchy_closure_and_snapshots.sql`）
- 最小数据集（3 节点）：
  - `TENANT_ID=00000000-0000-0000-0000-000000000001`
  - `effective_date=2025-01-01T00:00:00Z`
- Refresh/build（JSON 输出示例）：
  - snapshot dry-run：`RowCount=5 MaxDepth=1`
  - snapshot apply：`Activated=true BuildID=aa898d2e-c4ac-4f23-b923-7a7c3fd1d2a2`
  - closure dry-run：`RowCount=5 MaxDepth=1`
  - closure apply：`Activated=true BuildID=1c9cd04f-c894-46f1-b105-3342fbfe3bff`
  - closure activate（回滚演练）：切回 `BuildID=1c9cd04f-c894-46f1-b105-3342fbfe3bff`
  - closure prune：`DeletedBuilds=1`

