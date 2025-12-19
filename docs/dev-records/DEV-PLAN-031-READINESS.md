# DEV-PLAN-031 Readiness

> 目标：记录 DEV-PLAN-031（Org 数据质量与修复）落地后的本地门禁执行结果与最小冒烟路径，作为 CI 对齐与回归依据。

## 变更摘要
- `org-data quality`：`check/plan/apply/rollback`（默认 dry-run；`--apply --yes` 才允许写入/回滚）
- 质量规则 v1：`ORG_Q_001`~`ORG_Q_008`（详见 `docs/dev-plans/031-org-data-quality-and-fixes.md` §6.2）
- 配置（安全网）：
  - `ORG_DATA_QUALITY_ENABLED`（默认 `false`；关闭时禁止 `quality apply/rollback`）
  - `ORG_DATA_FIXES_MAX_COMMANDS`（默认 `100`）

## 环境信息（本次执行）
- 日期（UTC）：2025-12-19T01:46:36Z
- 分支：feature/dev-plan-031-org-data-quality-fixes
- Git Revision：c2c94d1d1a08f4a744c2f939bbd159fa7ac2ce55
- Go：go1.24.10 linux/amd64
- 测试 DB：PostgreSQL `127.0.0.1:5432`（如无本地 PG，可用 `docker run -d --name bugs-blossoms-test-db -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:17`）
- 工作区状态：当前工作区包含 DEV-PLAN-031 的未提交改动；门禁执行通过

## 门禁执行记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-19 | 本地 | `go fmt ./...` | 格式化无 diff | 通过 | OK |
| 2025-12-19 | 本地 | `go vet ./...` | 无 vet 报错 | 通过 | OK |
| 2025-12-19 | 本地 | `make check lint` | golangci-lint + cleanarchguard 通过 | 通过 | OK |
| 2025-12-19 | 本地 | `make test` | 测试全通过 | 通过 | OK |
| 2025-12-19 | 本地 | `make check doc` | 文档门禁通过 | 通过 | OK |

## 最小冒烟（示例）

> 说明：API backend 依赖可用的 `Authorization`（可为 `sid` 或其它可被 `pkg/middleware.Authorize` 识别的 token），并要求其所属 tenant 与 `--tenant` 一致。

1) 生成质量报告（DB backend，默认）：
- `go run ./cmd/org-data quality check --tenant <tenant_uuid> --output /tmp/org-quality`

2) 生成修复计划：
- `go run ./cmd/org-data quality plan --report /tmp/org-quality/org_quality_report_<tenant>_<asof>_<run_id>.json --output /tmp/org-quality`

3) 执行修复（dry-run，走 `/org/api/batch` dry-run）：
- `ORG_DATA_QUALITY_ENABLED=true go run ./cmd/org-data quality apply --fix-plan /tmp/org-quality/org_quality_fix_plan_<tenant>_<asof>_<run_id>.json --auth-token <token> --base-url http://localhost:3200 --output /tmp/org-quality`

4) 执行修复（apply）：
- `ORG_DATA_QUALITY_ENABLED=true go run ./cmd/org-data quality apply --fix-plan /tmp/org-quality/org_quality_fix_plan_<tenant>_<asof>_<run_id>.json --auth-token <token> --base-url http://localhost:3200 --output /tmp/org-quality --apply --yes`

5) 回滚（dry-run）：
- `ORG_DATA_QUALITY_ENABLED=true go run ./cmd/org-data quality rollback --manifest /tmp/org-quality/org_quality_fix_manifest_<tenant>_<asof>_<run_id>.json --auth-token <token> --base-url http://localhost:3200`

6) 回滚（apply）：
- `ORG_DATA_QUALITY_ENABLED=true go run ./cmd/org-data quality rollback --manifest /tmp/org-quality/org_quality_fix_manifest_<tenant>_<asof>_<run_id>.json --auth-token <token> --base-url http://localhost:3200 --apply --yes`
