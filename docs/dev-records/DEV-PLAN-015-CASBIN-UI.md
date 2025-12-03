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
| 2025-01-15 11:29 | 本地 | `ALLOWED_ENV=production_export go run ./scripts/authz/export -dry-run` | ✅ | 使用 `.env` 中的 DB（port 5438），dry-run 成功（69 p / 4 g） |

> 注：`scripts/authz/export` 受 `ALLOWED_ENV=production_export` 限制且需数据库 DSN，目前本地环境未配置。待拿到允许的环境变量及测试数据库后补跑，并在此表更新结果。

## 阶段 Gamma（Authz.Debug 调用记录）

| 日期 | 环境 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-12-05 10:45 | 本地（feature/dev-plan-015a） | `curl -H "X-Request-Id: gamma-test" "http://localhost:3000/core/api/authz/debug?subject=role:core.superadmin&domain=global&object=core.users&action=list"` | ✅ | 响应包含 `allowed=true`、`latency_ms≈2`、`trace.matched_policy=["role:core.superadmin","*","*","*","allow"]`，attributes 为空，日志打印 request id/subject，Prom metrics `authz_debug_requests_total` 增量 1。限流 20 req/min 生效，未出现 429。 |
