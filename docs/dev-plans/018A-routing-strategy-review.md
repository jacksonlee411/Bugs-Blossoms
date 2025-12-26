# DEV-PLAN-018A：全局路由策略统一（DEV-PLAN-018）评审输入与对齐建议

**状态**: 草拟中（2025-12-15 05:03 UTC）

**评审结论（摘要）**：DEV-PLAN-018 的方向总体“有益且必要”，属于治理型 SSOT（统一命名空间/协商/鉴权/错误返回），不是过度设计。此前与 Org 系列计划（DEV-PLAN-020/026）存在一个会放大漂移的关键冲突：Org API 与 UI 都落在 `/org/*`，而 018 要求内部 API 具备强命名空间。该冲突已通过 018 的 ADR-018-01 落盘解决：内部 API 统一为 `/{module}/api/*`，Org 内部 API 为 `/org/api/*`，`/org/*` 保留 UI；并已同步更新 DEV-PLAN-020/026。后续重点转为：例外清单落盘、全局错误返回契约（404/405/500）、以及可验证门禁（route-lint）。

## 1. 背景与上下文 (Context)
- 仓库同时包含 SSR UI（templ + HTMX）、JSON API、第三方回调（webhooks）、运维/测试端点。路由命名空间与返回契约若不统一，会导致：
  - UI/HTMX 与 API 的内容协商分散（partial/整页/JSON 混用）；
  - 403/鉴权返回口径不一致，E2E 与排障成本上升；
  - “/api” 语义歧义：内部 API 与对外 API 混在同一前缀下，易误用与误暴露。
- 本文档定位：把对 DEV-PLAN-018 的调查证据、冲突点、风险与建议整理为进一步评估与对齐的输入（不替代 018 本身）。

## 2. 本次调查范围 (Scope)
### 2.1 相关计划与约束
- 路由治理：`docs/dev-plans/018-routing-strategy.md`
- Org 模块规划（与路由强相关）：`docs/dev-plans/020-organization-lifecycle.md`、`docs/dev-plans/026-org-api-authz-and-events.md`
- 仓库开发/门禁/冻结政策：`AGENTS.md`、`docs/ARCHITECTURE.md`

### 2.2 代码现状抽样（用于举证而非穷举）
- 内部 API 与 403 契约：
  - `modules/core/authzutil/forbidden_response.go`（统一 forbidden payload）
  - `modules/core/presentation/controllers/authz_helpers.go`（`ensureAuthz` + UI/HTMX/JSON 403 分支）
  - `modules/hrm/presentation/controllers/authz_helpers.go`、`modules/logging/presentation/controllers/authz_helpers.go`（存在重复实现，体现“口径分散”风险）
- 路由前缀歧义样例：
  - `/api/lens/events`：`modules/core/presentation/controllers/lens_events_controller.go`
  - `/api/website/ai-chat`：`modules/website/module.go`
- 运维/测试端点：
  - `/health`：`modules/core/presentation/controllers/health_controller.go`
  - `/__test__/*`：`modules/testkit/presentation/controllers/test_endpoints_controller.go`

## 3. 现状盘点（证据导向）(Findings)
### 3.1 已存在的“统一 403 契约”雏形（可复用）
- forbidden payload 结构已集中在 `modules/core/authzutil/forbidden_response.go`，并使用 `/core/api/authz/policies/apply`（管理员维护生效）与 `/core/api/authz/debug` 作为标准入口。
- UI 侧对 403 的分流已体现 018 的协商优先级思想：
  - 显式 `Accept: application/json` 返回 JSON；
  - HTMX 请求设置 `Hx-Retarget: body`、`Hx-Reswap: innerHTML` 返回 Unauthorized partial；
  - 否则返回整页 Unauthorized。
  - **澄清**：协商优先级应为 **JSON（显式 Accept）> HTMX（Hx-Request）> HTML**（见 018 的 5.1），不要反转为 “HTMX > JSON”，否则 E2E/诊断会在 HTMX 场景下拿不到稳定 JSON。

### 3.2 “/api/*” 在仓库内并非都表示对外 API
- `/api/lens/events` 与 `/api/website/ai-chat` 明显是“应用内部交互端点”（与 UI 同源协作），但路径形态会被误解为“对外 API”，与 018 的痛点描述一致。
- 这类路径若继续增长，会使 018 期望建立的“对外 API = `/api/v1/*`”失去约束力，形成长期技术债。

### 3.3 顶层入口存在多个“合理例外”，需纳入 SSOT
- GraphQL：`/query`、`/playground`（`modules/core/presentation/controllers/graphql_controller.go`）
- Prometheus：默认 `/debug/prometheus`（`pkg/metrics/prometheus_controller.go`）
- 上传与静态资源：`/assets/*`、上传文件前缀（`modules/core/presentation/controllers/upload_controller.go`）
- 这些路径并非错误，但若不被 018 明确分类/例外化，会导致“SSOT 不完整”，未来 review 难以落地一致性。

### 3.4 冻结模块约束会直接影响“迁移节奏”
- `modules/billing`、`modules/crm`、`modules/finance` 禁止修改（见 `AGENTS.md`）。因此 018 的“试点迁移”应避免选取这些模块内路径作为第一批落地对象；否则只能停留在“文档正确但无法收敛实现”的状态。

## 4. 关键冲突与缺口 (Gap Analysis)
### 4.1 与 Org 系列计划的命名空间冲突（必须先解）
- DEV-PLAN-020/026 早期草案将 Org controller 前缀定义为 `/org`，并把 REST API 也放在 `/org/*`。
- DEV-PLAN-018 的关键决策是：
  - 内部 API 按模块归属为 `/{module}/api/*`；
  - 对外 API 强制 `/api/v1/*`；
  - UI 路由默认 HTML（成功响应不追求 JSON 化）。
- 如果保持“Org API 与 UI 都落在 `/org/*`”不变，Org 会同时承担 UI 与 JSON API 的双重语义，与 018 的“强命名空间”目标形成结构性冲突。
- **更新**：该冲突已在 `docs/dev-plans/018-routing-strategy.md` 的 ADR-018-01 裁决并对齐：
  - `/org/*`：UI（HTML/HTMX）
  - `/org/api/*`：内部 API（JSON-only）
  - 对外 API 若引入：`/api/v1/org/*`

### 4.2 “内部 API = `/core/api/*`”的语义成本
- 本节仅针对“将所有模块内部 API 归并到 `/core/api/*`”这一选项（B）。现已选择选项 A（`/{module}/api/*`），因此该语义成本不再成立；`/core/api/authz/*` 作为既成事实继续保留。

### 4.3 例外清单与迁移策略需要更具体（否则落不下去）
- 018 已声明“存量可渐进收敛并保留 alias/redirect”，但当前还缺：
  - 存量顶层前缀盘点清单（最少覆盖 `/api/*`、`/query`、`/debug/*`、`/_dev`、`/twilio` 等）；
  - 每个例外的“保留原因/是否迁移/目标位置/安全基线”说明。

### 4.4 特殊流量类型的路由策略缺失
- **Webhooks (第三方回调)**：文中提及了 webhooks 存在，但未定义其标准前缀（如 `/webhooks/*` 或 `/api/webhooks/*`）。这类请求通常需要**豁免 CSRF** 且使用特定的**签名校验**中间件。若不独立切分，难以在全局中间件层面统一管理，容易引发安全配置漂移。
- **AuthN (认证) 边界不明确**：目前仅讨论了 AuthZ (403) 返回格式。实际上，路由前缀通常也绑定了认证方式（如 `/api/v1` 强制 Token，`/core/api` 复用 Session）。明确这一点能增强“拆分内外部 API”的架构合理性。
- **Ops/Observability (运维观测)**：`/health`、`/metrics` 目前散落。018 应明确它们是继续保持“顶层例外”，还是收敛到如 `/_ops/*` 或 `/_system/*` 的统一命名空间，以便负载均衡或网关层统一配置访问白名单。

## 5. 对整体项目框架的影响 (Impact)
### 5.1 对 DDD/CleanArchGuard
- 正向：路由类别决定默认 middleware/返回契约，有助于减少 controller 自由发挥导致的行为漂移；对 `make check lint` 的长期稳定性是加分项。
- 风险：若把跨模块共享的协商/403 responder 分散在各 module，会引入重复与不一致；建议优先沉到 `pkg/*`（或保持在 core 但明确“跨模块复用”的依赖边界与约束），避免 CleanArchGuard 压力上升。

### 5.2 对 DEV-PLAN-020（Org）落地路径
- 若不先裁决命名空间，020/026 的 API 前缀将把 Org 变成最大例外；后续 021-035 的大量路由会在“UI vs API”边界上反复摇摆，放大成本与风险。

## 6. 建议与决策点 (Recommendations & Decisions)
### 6.1 建议先做一次 ADR：“内部 API 前缀”裁决（两选一）
1. **选项 A（推荐）：内部 API 按模块归属为 `/{module}/api/*`**
   - 例：core 内部 API 继续 `/core/api/*`；Org 内部 API 为 `/org/api/*`；website 内部 API 为 `/website/api/*`。
   - 好处：路径语义与模块边界一致；与 020 的 “模块前缀 `/org`”天然对齐（仅需把 API 从 `/org/*` 收敛到 `/org/api/*`）。
2. **选项 B：坚持 018 原案，内部 API 全部归并到 `/core/api/*`**
   - 例：Org 内部 API 变更为 `/core/api/org/*`，并声明 `/org/*` 仅用于 UI（HTML/HTMX）。
   - 好处：统一性更强、前缀更少；代价是语义反直觉与跨模块依赖讨论成本上升。

> 无论选 A/B，**对外 API** 建议保持 `docs/dev-plans/018-routing-strategy.md` 的 `/api/v1/*` 强制版本化约束。

### 6.2 对 DEV-PLAN-018 的补强建议（落盘为可执行约束）
- 增补“顶层入口分类与例外清单”章节：明确 GraphQL/Prometheus/uploads/websocket 等归类。
- 明确“冻结模块路径的处理策略”：列为长期例外或待解冻后迁移（避免 reviewer 期待短期收敛）。
- **明确 Webhooks 与 Ops 路由策略**：建议 Webhooks 使用独立一级前缀（如 `/webhooks/*`）以隔离中间件策略；建议 Ops 路由标准化或明确列为永久白名单。
- 明确“试点迁移候选”优先级：建议从 `modules/core` 与 `modules/website` 的 `/api/*` 存量端点开始，按 alias/redirect 渐进迁移。
- route-lint 必须可执行且可维护：建议以 018 的“附录 B 例外清单（或等价 allowlist 文件）”作为白名单来源；禁止新增非版本化 `/api/*`，并阻止在非冻结模块引入新的 legacy 前缀（防止破窗效应）。
- Webhooks/Ops 中间件应在路由构建层强制绑定：避免由 controller 自行选择性配置导致签名校验/访问控制漂移。

### 6.3 对 DEV-PLAN-020/026 的对齐建议（取决于 6.1 的选项）
- 若选项 A：将 Org JSON API 统一迁移到 `/org/api/*`，并声明 `/org/*` 为 UI（HTML/HTMX）。
- 若选项 B：将 Org JSON API 迁移到 `/core/api/org/*`，并保持 `/org/*` 为 UI。

## 7. 下一步（评估输入待办）(Next Actions)
1. [x] 确认 6.1 的 ADR 选择（A），并在 `docs/dev-plans/018-routing-strategy.md` 固化为明确规范。
2. [x] 同步更新 `docs/dev-plans/020-organization-lifecycle.md` 与 `docs/dev-plans/026-org-api-authz-and-events.md` 的 API 前缀约定，消除冲突。
3. [x] 输出“存量路由盘点表”（至少覆盖 `/api/*`、`/query`、`/debug/*`、`/_dev`、`/twilio`、`/__test__`、`/health`、`/ws`、`/assets`；见附录 A）。
4. [ ] 选择 1-2 个非冻结模块的存量 `/api/*` 端点做试点迁移（保留 alias/redirect），并补充 E2E 断言与回滚策略。

---

## 附录 A：存量路由盘点表（顶层前缀/入口）

> 说明：本盘点表用于支撑 DEV-PLAN-018 的“顶层命名空间/例外清单/迁移策略”落盘；以代码中 `controller.Register()` / `module.go` 的路由注册为准，列出**顶层前缀与关键单例入口**（不是逐条枚举所有子路由）。
>
> 重要背景：本仓库至少存在两个 HTTP 服务入口，路由集合并不完全一致：
> - `cmd/server/main.go`：加载 `modules/BuiltInModules`（见 `modules/load.go`），并额外注册 `/assets/*`、GraphQL（`/query`、`/playground`）与 Prometheus（可选）。
> - `cmd/superadmin/main.go`：只加载 `superadmin` 模块 + core 的少量 AuthN/UI 控制器（`/login`、`/logout`、`/account`、`/uploads`），并注册 `/assets/*`；不加载 core 模块的其余业务控制器。

### A.1 顶层入口总览（按类别）

| 顶层前缀/入口 | 类别（建议） | server | superadmin | 用途/备注（与 018 的关系） | 证据（代码） |
|---|---|---:|---:|---|---|
| `/assets/*` | 静态资源（build assets） | ✓ | ✓ | UI 静态资源；属于“顶层例外”且应长期保留 | `modules/core/presentation/controllers/static_files_controller.go` |
| `/{UPLOADS_PATH}/*`（默认 `/static/*`） | 静态资源（上传文件） | ✓ | ✓ | 配置驱动的静态前缀；需要明确与 `/assets/*` 的边界与缓存策略 | `pkg/configuration/environment.go`、`modules/core/presentation/controllers/upload_controller.go` |
| `/uploads` | UI/内部写接口（文件上传） | ✓ | ✓ | 当前是“根路径写接口”（POST）；应在 018 中明确其分类（UI 写请求 vs 内部 API）与 CSRF 策略 | `modules/core/presentation/controllers/upload_controller.go` |
| `/` | UI（Authenticated） | ✓ | ✓ | **同一路径在不同二进制语义不同**：server 是 core dashboard；superadmin 是 superadmin dashboard | `modules/core/presentation/controllers/dashboard_controller.go`、`modules/superadmin/presentation/controllers/dashboard_controller.go` |
| `/metrics` | UI（HTMX/局部加载） | ✗ | ✓ | superadmin dashboard 的局部数据端点；避免与观测指标语义混淆（Prometheus 已用 `/debug/prometheus`） | `modules/superadmin/presentation/controllers/dashboard_controller.go` |
| `/login` | AuthN（登录页/表单） | ✓ | ✓ | 必须作为“AuthN 特殊入口”写入 SSOT（避免被全局 auth middleware 误拦截） | `modules/core/presentation/controllers/login_controller.go` |
| `/oauth/google/callback` | AuthN（第三方回调） | ✓ | ✓ | OAuth callback；属于 AuthN 边界，应与 webhooks 机制区分 | `modules/core/presentation/controllers/login_controller.go` |
| `/logout` | AuthN（登出） | ✓ | ✓ | session cookie 清理；通常允许 GET 但需要评估 CSRF/副作用 | `modules/core/presentation/controllers/logout_controller.go` |
| `/account` | UI（Authenticated） | ✓ | ✓ | 用户账号页；在 superadmin 中也暴露（因复用 core Auth 服务） | `modules/core/presentation/controllers/account_controller.go`、`cmd/superadmin/main.go` |
| `/settings` | UI（Authenticated） | ✓ | ✗ | 根路径 UI（非 `/{module}/...`）；应明确为 core 顶层例外 | `modules/core/presentation/controllers/settings_controller.go`、`modules/core/module.go` |
| `/users` | UI（Authenticated） | ✓ | ✗ | core 管理 UI（根路径）；属于 018 里“少量根路径 UI”范畴，但需明确边界 | `modules/core/module.go` |
| `/roles` | UI（Authenticated） | ✓ | ✗ | core 管理 UI（根路径） | `modules/core/module.go` |
| `/groups` | UI（Authenticated） | ✓ | ✗ | core 管理 UI（根路径） | `modules/core/module.go` |
| `/spotlight` | UI（Authenticated） | ✓ | ✗ | 全局搜索/快捷入口（根路径） | `modules/core/presentation/controllers/spotlight_controller.go`、`modules/core/module.go` |
| `/_dev/*` | Dev-only（开发工具/预览） | ✓ | ✗ | 需要在 018 中明确：只在 dev 启用、不可对外、是否永久例外 | `modules/core/presentation/controllers/showcase_controller.go`、`modules/core/module.go` |
| `/core/authz` | UI（Authenticated） | ✓ | ✗ | 权限申请 UI（HTML）；与 `/core/api/authz/*` 形成 UI/API 配对 | `modules/core/presentation/controllers/authz_request_controller.go`、`modules/core/module.go` |
| `/core/api/authz/*` | 内部 API（JSON/HTMX） | ✓ | ✗ | 既成事实的内部 API SSOT；对齐 018 后仍作为内部 API 的核心基准 | `modules/core/presentation/controllers/authz_api_controller.go`、`modules/core/module.go` |
| `/api/lens/events/*` | 内部 API（JSON） | ✓ | ✗ | **“/api 语义歧义”存量样例**：需迁移到内部 API 命名空间或明确例外 | `modules/core/presentation/controllers/lens_events_controller.go` |
| `/api/website/ai-chat/*` | API（当前更像 public integration） | ✓ | ✗ | 当前未显式使用 session/authz 中间件；属于“非版本化 API”风险点；对齐 018 后建议迁移到 `/api/v1/website/ai-chat/*`（或明确为长期例外）并补齐 anti-abuse/审计 | `modules/website/module.go`、`modules/website/presentation/controllers/aichat_api_controller.go` |
| `/_dev/api/showcase/*` | Dev/内部 API（JSON） | ✓ | ✗ | 与 `/_dev/*` 强相关；应明确是否仅 dev 暴露 | `modules/core/presentation/controllers/showcase_controller.go` |
| `/query`、`/playground`、`/query/*` | GraphQL | ✓ | ✗ | 顶层例外；需要明确认证模型/暴露范围（目前未强制登录） | `modules/core/presentation/controllers/graphql_controller.go`、`cmd/server/main.go` |
| `/debug/prometheus`（可配置） | Ops/Observability | ✓ | ✗ | Prometheus scrape；顶层例外且通常需网关层保护/白名单 | `pkg/metrics/prometheus_controller.go`、`cmd/server/main.go`、`.env.example` |
| `/health` | Ops/Health | ✓ | ✗ | 健康检查（JSON）；顶层例外 | `modules/core/presentation/controllers/health_controller.go`、`modules/core/module.go` |
| `/__test__/*` | Test-only | ✓ | ✗ | 受 `ENABLE_TEST_ENDPOINTS` 开关保护；必须在 018 明确“生产禁用”基线 | `modules/testkit/presentation/controllers/test_endpoints_controller.go`、`modules/load.go`、`pkg/configuration/environment.go` |
| `/ws` | Websocket | ✓ | ✗ | 顶层例外；需要在 018 中明确认证/Origin/升级握手策略 | `modules/core/presentation/controllers/websocket_controller.go`、`modules/core/module.go` |
| `/twilio` | Webhook（第三方回调） | ✓ | ✗ | CRM 短信回调；典型 webhook，需签名校验/重放保护/租户映射策略（目前仅 `WithTransaction()`） | `modules/crm/presentation/controllers/twilio_controller.go` |
| `/billing/*`（如 `/billing/click/*`） | Webhook（支付回调） | ✓ | ✗ | 多支付网关回调；由于冻结模块短期无法迁移，但必须纳入 018 例外清单与安全基线 | `modules/billing/module.go`、`modules/billing/presentation/controllers/*_controller.go` |
| `/superadmin/*` | UI（Superadmin） | ✗ | ✓ | superadmin 专用命名空间；与普通业务 UI 分离良好，但需写入 018 的“多入口/多域”策略 | `modules/superadmin/presentation/controllers/tenants_controller.go`、`modules/superadmin/module.go` |

### A.2 模块级 UI 前缀（server）

> 这里列出 “`/{module}/...`” 风格的主要 UI 前缀，用于对齐 018 的“模块 UI 命名空间”约束；细项略。

| 模块 UI 前缀 | 备注 | 证据（代码） |
|---|---|---|
| `/hrm/*`（如 `/hrm/employees`） | 典型模块前缀 | `modules/hrm/presentation/controllers/employee_controller.go` |
| `/superadmin/*` | 典型模块前缀（Superadmin Server） | `modules/superadmin/presentation/controllers/tenants_controller.go` |
| `/logs` | 模块 UI 但未使用 `/{module}/...` 前缀（logging 例外） | `modules/logging/presentation/controllers/logs_controller.go` |
| `/website/*`（如 `/website/ai-chat`） | 典型模块前缀 | `modules/website/module.go` |
| `/bi-chat` | 非 `/{module}/...` 风格（bichat 例外） | `modules/bichat/presentation/controllers/bichat_controller.go` |
