# DEV-PLAN-015：Casbin 策略 UI / 平台联调记录

> [!IMPORTANT]
> 自 DEV-PLAN-015C 起，策略草稿（requests）/审批/bot 链路已移除；本记录仅用于追溯历史联调过程，不再作为现行口径。现行口径见 `docs/runbooks/authz-policy-apply-api.md`。

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
| 2025-12-09 08:05 | 本地（feature/015b2-user-policy-board） | `go test ./modules/core/...` | ✅ | 120s 超时后重跑通过，覆盖 controllers/query 等 |
| 2025-12-09 08:05 | 本地 | `make check lint` | ✅ | golangci-lint + cleanarch 通过 |
| 2025-12-09 08:05 | 本地 | `make check tr` | ✅ | 多语言键一致（新增 Stage/SLA 文案） |

> 注：`scripts/authz/export` 受 `ALLOWED_ENV=production_export` 限制且需数据库 DSN，目前本地环境未配置。待拿到允许的环境变量及测试数据库后补跑，并在此表更新结果。

## 阶段 Gamma（Authz.Debug 调用记录）

| 日期 | 环境 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-12-05 10:45 | 本地（feature/dev-plan-015a） | `curl -H "X-Request-Id: gamma-test" "http://localhost:3000/core/api/authz/debug?subject=role:core.superadmin&domain=global&object=core.users&action=list"` | ✅ | 响应包含 `allowed=true`、`latency_ms≈2`、`trace.matched_policy=["role:core.superadmin","*","*","*","allow"]`，attributes 为空，日志打印 request id/subject，Prom metrics `authz_debug_requests_total` 增量 1。限流 20 req/min 生效，未出现 429。 |

## 阶段 Delta（Bot 运行记录模板）

| 日期 | 环境 | Request ID | 基线修订 | PR 链接 | Bot Locker | 结果/备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 2025-12-04 07:40 | 本地（feature/dev-plan-015a） | `c0c6cd84-4e00-4c35-ac24-359b3f8477de` | `ec522ef3… → 7dc0adfc…` | https://github.com/jacksonlee411/Bugs-Blossoms/pull/11 | `DESKTOP-S9U9E9K-54966-1764805201` | ✅ merged；bot 首次运行即完成，回写 snapshot/PR link 成功 |
| 2025-12-04 07:44 | 本地（feature/dev-plan-015a） | `c256f1a3-83cd-480d-867e-f5d18c0168f0` | `ec522ef3… → 8f25298b…` | https://github.com/jacksonlee411/Bugs-Blossoms/pull/12 | `DESKTOP-S9U9E9K-58601-1764805469` | ✅ merged；首次推送因 HTTPS token 解析报错被标记失败，手动解锁后重试成功 |

> 填写建议：在草稿状态从 `approved` 转为 `merged` 后，记录 bot 输出、`applied_policy_snapshot` 是否写入、相关 PR、Locker 等信息；若失败，记录 `error_log` 并注明是否使用 `force-release` 重试。

## 阶段验证（015B2 收尾）

| 日期 | 环境 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-12-09 16:45 | 本地（feature/015b2-user-policy-board） | `BASE_URL=http://localhost:3201 npx playwright test --workers=1 --reporter=line` | ✅ | 全量 8/8 通过（包含 users 编辑用例稳定性修复），依赖本地 DB:5438/Redis |
| 2025-12-09 17:26 | 本地 | `make check lint` / `make check tr` | ✅ | golangci-lint + cleanarch 通过；多语言键一致 |

## 阶段验证（015B3 授权体验）

| 日期 | 环境 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-12-11 08:35 | 本地 | `go test ./modules/logging/... ./modules/hrm/... ./modules/core/authzutil/... ./components/authorization/...` | ✅ | 覆盖 HRM/Logging 403 base_revision 域名映射与 Unauthorized helper；新增 e2e 无权态断言（未跑浏览器）。 |
| 2025-12-11 10:39 | 本地（feature/015b3-business-authz-ui） | `BASE_URL=http://localhost:3201 npx playwright test tests/logs/logs.spec.ts tests/users/register.spec.ts --reporter=line` | ✅ | 修正 Logging 域授权与 g2 绑定后，超级管理员可访问 /logs，用户编辑表单稳定；未改 locales，模板经 `templ generate && make css`。 |

## 阶段验证（015B4 串联与反馈）

| 日期 | 环境 | 命令/操作 | 结果 | 备注 |
| --- | --- | --- | --- | --- |
| 2025-12-11 15:21 | 本地（feature/015b4） | `GOFLAGS=-buildvcs=false make check lint` | ✅ | golangci-lint + cleanarch 通过，authz/templ 改动均已检查。 |
| 2025-12-11 15:22 | 本地 | `GOFLAGS=-buildvcs=false go test ./modules/core/... ./components/authorization/...` | ✅ | 覆盖 AuthzAPIController 新增重试 token/限流、Unauthorized 轮询脚本等。 |
| 2025-12-11 16:10 | 本地 | 键盘巡检 Unauthorized/PolicyInspector：Tab/Shift+Tab 聚焦提交、复制 request_id、触发 bot 重试 | ✅ | 无新增模态；按钮可用 Enter/Space 触发，重试按钮随终态隐藏；无额外 aria 需求。 |
