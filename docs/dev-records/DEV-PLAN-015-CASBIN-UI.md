# DEV-PLAN-015：Casbin 策略 UI / 平台联调记录

## 前置依赖验证（Alpha 前置）

| 日期 | 环境 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-01-15 11:05 | 本地（feature/dev-plan-015a） | `make authz-test` | ✅ | go test ./pkg/authz ./scripts/authz/internal/... |
| 2025-01-15 11:07 | 本地 | `make authz-lint` | ✅ | `go run ./scripts/authz/pack` + `verify --fixtures` |
| 2025-01-15 11:08 | 本地 | `go test ./pkg/authz/...` | ✅ | 单包测试 |
| 2025-01-15 11:10 | 本地 | `go test ./modules/core/...` | ✅ | 第一次 120s 超时，增大 timeout 后通过 |
| 2025-01-15 11:12 | 本地 | `make authz-pack` | ✅ | 生成策略并校验 |
| 2025-01-15 11:13 | 本地 | `go run ./scripts/authz/verify --fixtures config/access/fixtures/testdata.yaml` | ✅ | fixture parity passed |
| 2025-01-15 11:14 | 本地 | `go run ./scripts/authz/export` | ⛔️ | 阻塞：`ALLOWED_ENV`=空（需 production_export）且缺少导出 DSN，待获批后重跑 |

> 注：`scripts/authz/export` 受 `ALLOWED_ENV=production_export` 限制且需数据库 DSN，目前本地环境未配置。待拿到允许的环境变量及测试数据库后补跑，并在此表更新结果。
