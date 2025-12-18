# DEV-PLAN-030 Readiness

> 目标：记录 DEV-PLAN-030（Org 变更请求与预检）落地后的本地门禁执行结果，作为 CI 对齐与回归依据。

## 变更摘要
- change-requests API：`/org/api/change-requests*`（draft/submit/cancel + list/get）
- preflight API：`POST /org/api/preflight`
- feature flags：
  - `ORG_CHANGE_REQUESTS_ENABLED`（默认 `false`）
  - `ORG_PREFLIGHT_ENABLED`（默认 `false`）

## 环境信息（本次执行）
- 日期（UTC）：2025-12-18
- 分支：`main-020`
- Git Revision：`355e3be9`
- Go：`go1.24.10 linux/amd64`
- 工作区状态：门禁执行后 `git status --porcelain` 为空；回填本文件后存在未提交改动（仅本文件）

## 门禁执行记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-18 22:38 UTC | 本地 | `go fmt ./...` | 格式化无 diff | 无 diff（门禁执行后工作区为空） | 通过 |
| 2025-12-18 22:38 UTC | 本地 | `go vet ./...` | 无 vet 报错 | 无输出 | 通过 |
| 2025-12-18 22:39 UTC | 本地 | `make check lint` | golangci-lint + cleanarchguard 通过 | `0 issues.`；`go-cleanarch 检查通过。` | 通过 |
| 2025-12-18 22:48 UTC | 本地 | `make test` | 测试全通过 | 通过（耗时约 8m46s） | 通过 |
| 2025-12-18 22:57 UTC | 本地 | `make authz-pack` | 聚合策略生成且无 diff | 无 diff | 通过 |
| 2025-12-18 22:57 UTC | 本地 | `make authz-test` | authz 单测通过 | 通过 | 通过 |
| 2025-12-18 22:57 UTC | 本地 | `make authz-lint` | authz fixture parity 通过 | `fixture parity passed` | 通过 |
| 2025-12-18 22:58 UTC | 本地 | `make check routing` | routing gates 通过 | 通过 | 通过 |

## 备注
- `make check routing` 与 CI `Routing Gates` job 对齐（见 `.github/workflows/quality-gates.yml`）。
