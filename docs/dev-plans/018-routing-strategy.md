# DEV-PLAN-018：全局路由策略统一（UI/HTMX/API/Webhooks）

**状态**: 已合并（PR #54，merge commit `47b599b7`，2025-12-16 UTC）

## 0. 实施进度（落地记录）

> 说明：本计划为“治理型 SSOT + 渐进落地”。本节用于登记已合入的最小落地（M1）与剩余工作，避免“实现已做但文档未登记”或“文档已写但实现未落地”的漂移。

- **已落地（M1，已合入）**：
  - allowlist SSOT：`config/routing/allowlist.yaml`
  - route_class 判定：`pkg/routing/classifier.go`
  - 全局 404/405（internal/public API 下 JSON-only）：`modules/core/presentation/controllers/errors_controller.go`
  - 全局 500（panic recovery）：API 命名空间返回 JSON 且不再 re-panic：`pkg/middleware/logging.go`
  - OpsGuard（仅生产生效）：保护 `/health` 与 `/debug/prometheus`：`pkg/middleware/ops_guard.go`、`internal/server/default.go`
  - Dev-only gate：生产默认不注册 `/_dev/*` 与 `/playground`：`modules/core/module.go`、`modules/core/presentation/controllers/graphql_controller.go`
  - route-lint（禁止新增非版本化 `/api/*`，allowlist 例外）：`internal/routelint/routelint_test.go`
  - 试点迁移（含 legacy alias 窗口期）：
    - `/api/lens/events/*` → `/core/api/lens/events/*`：`modules/core/presentation/controllers/lens_events_controller.go`
    - `/api/website/ai-chat/*` → `/api/v1/website/ai-chat/*`：`modules/website/module.go`
- **待完成（M2+）**：
  - [X] 403 responder/协商工具进一步收敛：统一实现迁移到 `modules/core/presentation/templates/layouts/authz_forbidden_responder.go`；core/hrm/logging 复用同一套 JSON/HTMX/HTML 403 行为（2025-12-16 13:44 UTC）。
  - [X] Webhooks 基线：allowlist 增加 `/webhooks -> webhook`，全局 404/405/500 对 webhook 采用 JSON-only；新增 `pkg/webhooks` 签名校验/重放保护中间件与门禁断言（2025-12-16 13:44 UTC）。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 项目同时包含：服务端渲染 UI（`templ` + HTMX）、内部 JSON API（用于权限申请/调试/局部交互）、第三方回调（支付/短信/聊天）、测试专用端点以及运维端点。
  - 当前路由命名空间与返回形态存在“不完全一致”的现象：同类能力在不同模块下使用不同前缀/协商规则，导致实现、鉴权与测试口径分散。
- **现状盘点（示例，非穷举）**：
- UI（HTML/HTMX）：`/hrm/employees`、`/logs`、`/website/ai-chat`、`/bi-chat`、`/superadmin/*` 等。
  - 内部 API（JSON）：`/core/api/authz/*`，以及少量 `/_dev`、`/api/*` 风格的内部端点。
  - AuthN（认证边界）：`/login`、`/logout`、`/oauth/*`。
  - Webhooks/第三方回调：`/twilio`、支付网关回调（路径可能由配置决定）。
  - 运维/测试：`/health`、`/__test__/*`、`/ws`、`/assets/*`。
- **痛点**：
  1. **命名空间语义不清**：`/api/*` 既可能是内部 JSON，也可能被误解为“对外 API”；`/core/api/*` 又被 UI 直接依赖（如权限申请）。
  2. **内容协商不一致**：部分 UI 路由在 `Hx-Request` 时返回 partial，但 `Accept: application/json` 时是否返回 JSON/HTML 的规则不统一，容易出现“HTMX 误拿 JSON / API 误拿 HTML”的边缘情况。
  3. **鉴权/错误返回口径分散**：403 的返回格式（HTML 页面 vs HTML partial vs JSON payload）在模块间存在差异，增加 E2E 与排障成本。
- **目标定位**：
  - 本计划产出“全局路由空间划分 + 命名约束 + 内容协商/鉴权/错误返回规范 + 迁移策略”，作为后续新增/重构路由的 SSOT（Single Source of Truth）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 定义**全局路由命名空间**与语义：哪些前缀用于 UI、哪些用于内部 API、哪些用于对外 API、哪些用于 Webhooks/运维/测试。
- [ ] 定义**返回形态契约**：每类命名空间默认返回 HTML 还是 JSON；HTMX 如何协作（partial/OOB/HX-* headers）。
- [ ] 定义**内容协商优先级**与一致实现方式（避免控制器各自手写分支逻辑）。
- [ ] 定义**鉴权与 403 行为**：HTML 页面/partial 与 JSON forbidden payload 的统一口径，并明确允许的例外场景。
- [ ] 给出**迁移与兼容策略**：不要求一次性改完所有路由，但要求“新增按新规、存量可渐进收敛”，并提供 deprecate/alias 的节奏。

### 2.2 非目标 (Out of Scope)
- 不在本计划内一次性重写所有控制器与前端引用（避免大范围破坏性变更）。
- 不引入新的路由框架（继续使用 `gorilla/mux`）。
- 不强制改变第三方回调路径（支付/短信等通常受外部平台约束），但要求纳入统一分类与安全基线。
- 不在本计划内引入 OpenAPI/Swagger 自动生成（后续如要纳入，需要独立计划定义范围与门禁）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 路由空间总览（Mermaid）
```mermaid
graph TD
  U[UI Routes<br/>HTML + HTMX] -->|may call| IA[Internal API<br/>JSON + HX headers]
  U -->|GET only| P[Public pages<br/>HTML]
  C[Clients/CLI/Integrations] --> PA[Public API<br/>/api/v1/* JSON]
  X[External Providers] --> WH[Webhooks<br/>/webhooks/* or legacy paths]
  Ops[Ops/CI] --> H[Health/Ready/Metrics]
  E2E[E2E runner] --> T[__test__ endpoints]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：保留“软隔离”，新增能力走“强命名空间”**
  - 原因：项目已存在 UI 路由 + 内部 JSON API + webhooks 混存的现实；全量硬隔离需要大规模迁移与外部对接变更，性价比不高。
  - 策略：从现在开始，新增/重构路由必须落在明确命名空间；存量逐步迁移并保留 alias/redirect。
- **决策 2：对外 API 强制 `/api/v1/*`，内部 API 统一 `/{module}/api/*`**
  - `/api/v1/*`：稳定、版本化、面向程序消费（CLI/下游系统/自动化），默认 JSON-only。
  - `/{module}/api/*`：面向本应用 UI 的内部 JSON API（含 HTMX 触发头），默认 session-based（同源）而非 token-based。
  - 说明：内部 API 与模块边界对齐（例如 core 内部 API 为 `/core/api/*`，天然承接既有的 `/core/api/authz/*`，并避免出现“所有模块 API 都挂在 core 下”的语义负担）。
- **决策 3：UI 成功响应以 HTML 为主；JSON 主要用于内部/对外 API 与“错误诊断”**
  - UI（HTML/HTMX）侧不追求“成功也返回 JSON”，避免前端二次渲染与协议复杂化。
  - 允许 UI 路由在显式 `Accept: application/json` 时返回 JSON（至少 403 forbidden payload），用于 E2E/调试/自动化断言。
- **决策 4：Webhooks 与 Ops 采用独立一级前缀（新增强制，存量渐进/例外）**
  - Webhooks 推荐统一在 `/webhooks/{provider}/*`；存量已绑定外部平台的路径可保留为 legacy（需在例外清单登记安全基线）。
  - Ops 推荐统一在 `/_ops/*`；存量 `/health`、`/debug/prometheus` 作为顶层例外保留并纳入安全基线。

### 3.3 ADR-018-01：内部 API 前缀裁决（必须对齐 Org）
> 目标：消除 “`/api/*` 语义歧义”，并让新增路由可被 reviewer 与门禁稳定判定其类别（UI/Internal/Public/Webhooks/Ops/Test）。

- **裁决**：内部 JSON API **按模块归属**使用 `/{module}/api/{capability}/...`（例如：core 为 `/core/api/*`，org 为 `/org/api/*`）。
- **强约束**：
  - `/api/v1/*` **仅用于对外 API**（版本化）；除 `/api/v1/*` 外，禁止新增任何 `/api/*` 路由。
  - 现存 `/api/*` 存量端点必须迁移到对应命名空间，并在迁移窗口期保留 alias（写请求/回调类端点不得依赖 redirect）。
- **与 Org 系列计划对齐**：
  - `/org/*` 保留给 Org UI（HTML/HTMX）。
  - Org 内部 API 统一为 `/org/api/*`（Session + Authz，JSON-only）。
  - 若未来需要“真正对外”的 Org API，则必须走 `/api/v1/org/*`（不得复用 UI Session cookie；认证模型需显式声明）。
  - 以上要求必须同步落盘到 `docs/dev-plans/020-organization-lifecycle.md` 与 `docs/dev-plans/026-org-api-authz-and-events.md`。

## 4. 路由命名空间与约束 (Route Space & Constraints)
### 4.1 顶层前缀（规范）
> 说明：仓库内至少存在两个 HTTP 服务入口（`cmd/server` 与 `cmd/superadmin`），它们加载的模块与路由集合并不完全一致；同一路径（例如 `/`）在不同入口可能语义不同。本表描述“全仓库的路由分类与命名空间约束”，并允许按入口取子集实现。

| 类别 | 前缀 | 默认返回 | 认证模型 | 备注 |
|---|---|---|---|---|
| UI（Authenticated） | `/{module}/...` + 少量根路径（如 `/settings`） | HTML | Session（cookie） | HTMX partial 通过 `Hx-Request` 返回 |
| AuthN（认证边界） | `/login`、`/logout`、`/oauth/*` | HTML/redirect | Anonymous（可选 Session） | 不属于业务 UI 模块；需明确其“允许未认证访问”的边界 |
| UI（Public） | `/`（公开页）等 | HTML | Anonymous | 如无公开页，可不新增；避免将“需要登录”的页面误归类为 public |
| 内部 API（Internal） | `/{module}/api/{capability}/...` | JSON | Session（cookie） | JSON-only；可设置 `HX-Trigger/HX-Redirect/Hx-Push-Url` 等响应头 |
| 对外 API（Public） | `/api/v1/{domain}/...` | JSON | Token/Key 或 Anonymous（按模块定义） | 强制版本化；禁止返回 HTML；若允许匿名必须补齐 anti-abuse（限流/验证码/审计等） |
| Webhooks（Inbound） | `/webhooks/{provider}/...`（新增推荐） | JSON/表单 | Provider 签名校验 | 存量路径允许保留（如 `/twilio`、`/billing/*`），但需登记安全基线 |
| 运维（Ops） | `/_ops/*`（新增推荐）+ legacy（`/health`、`/debug/prometheus`） | JSON/text | 无/受限 | 建议通过网关/内网白名单/BasicAuth 做访问控制 |
| 测试（Test only） | `/__test__/*` | JSON | 配置开关 | 仅在 `EnableTestEndpoints` 开启时存在 |
| 静态资源 | `/assets/*`、`/{UPLOADS_PATH}/*`（默认 `/static/*`） | file | 无 | build assets 与上传文件前缀需区分并纳入缓存策略 |
| WebSocket | `/ws` | WS | Session | 仅升级连接 |
| GraphQL（Legacy 例外） | `/query`、`/playground`、`/query/*` | JSON/HTML | Session（推荐） | `/playground` 默认生产关闭；`/query` 若保留则按内部能力处理（JSON-only + AuthN/Authz 基线） |
| Dev-only | `/_dev/*` | HTML/JSON | Session/可选 | 必须明确仅开发/测试环境启用，避免生产暴露 |

### 4.2 路径命名约束（规范）
- 路径 segment 一律小写，推荐 `kebab-case`（允许保留历史路径，如 `/bi-chat`、`/__test__`）。
- 资源命名优先使用复数名词：`/employees`、`/payments`、`/requests`。
- 操作型子路由（非纯 REST）统一采用后缀动词：`/requests/{id}/approve`（存量保留），新增应避免随意扩散“动作路由”。
- 不允许新增“语义不明”的顶层前缀（例如随意引入 `/api2`、`/internal`）；新增前缀必须先更新本计划并评审通过。

### 4.3 例外清单与登记规则（规范）
- 所有路由必须能被归类到 4.1 的某一类别；若路径形态不符合其类别的默认前缀（例如 UI 不是 `/{module}/...`、API 不在 `/{module}/api/*` 或 `/api/v1/*`），则视为**例外**。
- 每个例外必须登记在“附录 B”，并明确：
  - 归类（route_class）与入口（server/superadmin）；
  - 保留原因（外部平台绑定/历史兼容/二进制差异等）；
  - 迁移目标（若要迁移）与兼容策略（alias/redirect/弃用窗口）；
  - 安全基线（AuthN/Authz/签名校验/anti-abuse/环境开关）。
- 新增例外属于“路由契约变更”，必须先更新本计划再引入代码实现。

## 5. 内容协商与返回契约 (Negotiation & Contracts)
### 5.1 UI 路由的协商规则（建议统一实现）
> 目标：同一 UI 路由可同时支持整页渲染与 HTMX partial；并允许在显式 JSON 请求下返回诊断用 JSON（至少 403）。

优先级（高 → 低）：
1. **显式 JSON**：若 `Accept` 包含 `application/json`，返回 JSON（用于 API 客户端/E2E/诊断）。
2. **HTMX partial**：若 `Hx-Request: true`，返回 HTML partial（或 OOB），并允许使用 `HX-*` 响应头。
3. **默认整页**：返回 HTML full page。

说明：
- 若同时满足 `Accept: application/json` 与 `Hx-Request: true`，以 **JSON 优先**（便于 E2E/诊断稳定拿到 JSON；避免“JSON 请求拿到 HTML partial”）。
- 常规 HTMX 请求的 `Accept` 通常为 `text/html`，因此不会触发 JSON 分支；如某个 HTMX 调用需要 HTML partial，应避免显式请求 JSON。

### 5.2 内部 API（`/{module}/api/*`）契约
- 成功与失败均返回 JSON；不得返回 HTML（避免 UI 与 API 混淆）。
- 允许在 HTMX 调用场景下设置 `HX-Trigger/HX-Redirect/Hx-Push-Url` 等响应头，但 body 仍保持 JSON。
- 版本策略：**不做版本化（Always Latest）**，与同仓库 UI 同步发布；不承诺对外部调用者的向后兼容性（需要稳定兼容/对外承诺的接口必须走 `/api/v1/*`）。
- 认证与鉴权：
  - 默认按 Session + Authz 能力校验；
  - 403 统一返回 forbidden payload（字段口径与现有 `modules/core/authzutil.BuildForbiddenPayload` 对齐），且**不依赖 `Accept` 协商**（内部 API 必须 JSON-only）。

### 5.3 对外 API（`/api/v1/*`）契约
- JSON-only；错误统一 envelope（`code/message/details/request_id` 等）并可演进版本。
- 认证模型按模块定义（token/key 或明确允许匿名），不得复用 UI Session cookie 作为唯一凭证（避免跨站/CSRF 风险）。

### 5.4 403 Forbidden 的统一口径
- JSON：返回统一 forbidden payload（object/action/domain/subject/missing_policies/debug_url/base_revision/request_id）。
- HTML：
  - full page：渲染 Unauthorized 页面（复用 `components/authorization/unauthorized.templ`）。
  - HTMX：返回 Unauthorized partial，并设置 `Hx-Retarget: body`、`Hx-Reswap: innerHTML`（与现有实现对齐）。

### 5.5 404/405/500 等全局错误返回契约（必须对齐）
> 背景：即使单个 controller 做到了 JSON-only，仍可能在“未命中路由/方法不允许/全局 panic”时走到全局 handler；这些路径同样必须遵循命名空间契约。

- 对 `/{module}/api/*` 与 `/api/v1/*`：
  - 404/405/500 等错误必须返回 `application/json`，不得渲染 HTML 页面或返回纯文本。
  - 错误 payload：
    - 内部 API：可复用现有 `APIError` 形态（或等价的 `code/message/request_id`）。
    - 对外 API：必须返回 5.3 定义的错误 envelope。
- 对 UI 路由：
  - 默认返回 HTML（整页或 HTMX partial）；当 `Accept: application/json` 时允许返回 JSON（用于 E2E/诊断）。
- 实现提示（落地任务）：
  - `NotFoundHandler`/`MethodNotAllowedHandler` 需要基于 `r.URL.Path` 做 route_class 判定（例如：`/api/v1/*`、`/{module}/api/*` 等），再选择对应 responder；避免“API 命名空间下出现 HTML 404”。
  - route_class 判定必须满足：
    - **不依赖 `Accept`/`Hx-Request`**（否则会出现“UI 404 因 Accept 变成 JSON”或“HTMX 404 误拿 JSON”的漂移）；
    - **可维护/可验证**：route-lint 与 NotFound 使用同一套规则来源；
    - **兜底为 UI（HTML）**：未命中任何已知前缀时，默认为 UI（减少无意的 API 探测/信息泄露面）。
  - 推荐判定算法（可直接编码）：
    1. 从“附录 B 顶层入口盘点”（或等价 allowlist 文件）生成一组 `prefix -> route_class` 规则（按入口区分：server/superadmin 各自一份）。
    2. 按 prefix 长度倒序做**最长前缀匹配**；匹配应做 segment 边界检查（避免 `/api/v1x` 误命中 `/api/v1`）。
    3. 若未命中 allowlist，再用规则兜底：
       - `^/api/v1(/|$)` → `public_api`
       - `^/[^/]+/api(/|$)` → `internal_api`
    4. 最终兜底：`ui`。

    ```go
    // 伪代码：仅说明判定顺序与边界规则
    func ClassifyPath(path string) routeClass {
      for _, rule := range allowlistSortedByPrefixLenDesc {
        if hasPrefixOnSegmentBoundary(path, rule.prefix) {
          return rule.class
        }
      }
      if rePublicAPI.MatchString(path) { return publicAPI }
      if reInternalAPI.MatchString(path) { return internalAPI }
      return ui
    }
    ```

## 6. 中间件与控制器组织 (Middleware & Controller Conventions)
> 目标：让“路由类别”决定默认 middleware stack，减少控制器自由发挥导致的不一致。

### 6.1 UI（Authenticated）推荐栈（示意）
- `Authorize()` → `RedirectNotAuthenticated()` → `RequireAuthorization()` → `ProvideUser()` → `ProvideLocalizer()` → `NavItems()` → `WithPageContext()`
- 写请求额外：`WithTransaction()`。

### 6.2 内部 API 推荐栈（示意）
- `Authorize()` → `RequireAuthorization()` → `ProvideUser()` → `ProvideLocalizer()` →（可选）`WithTransaction()`
- 默认不需要 `NavItems/WithPageContext`（内部 API 必须保持 JSON-only；避免在 API 层渲染 Unauthorized HTML）。

### 6.3 Key() 语义（建议）
- 控制器的 `Key()` 建议返回稳定标识（优先使用其 basePath），便于日志/诊断与测试一致性；避免返回与路由无关的常量字符串（存量不强制改，但新增需遵循）。

### 6.4 Webhooks 推荐栈（必须可被基础设施层强制）
> 目标：避免 webhook 的签名校验/重放保护/CSRF 豁免分散在各 controller，形成安全策略漂移。

- 约束：
  - 新增 webhook 必须落在 `/webhooks/{provider}/*`（存量 legacy 需登记在附录 B，并给出迁移/安全基线）。
  - webhook 的安全中间件必须在“路由构建层（mux subrouter）”按前缀强制绑定，而不是由每个 controller 自行选择性配置。
- 推荐栈（示意）：
  - `VerifyProviderSignature()` → `ReplayProtection()` →（未来如有）`CSRFExempt()` → `WithTransaction()`（按需）→ handler
  - 默认不依赖 Session/Cookie，不应复用 `Authorize()/ProvideUser()` 作为唯一凭证来源。
- 建议的代码抽象（可复用、可强制）：
  - 在 `pkg/webhooks`（或等价目录）提供 `Verifier` 接口（例如：`VerifySignature(r *http.Request) error`），并提供将 verifier 绑定到 `mux` 子路由的标准 middleware。
  - provider 特定实现（Stripe/Twilio 等）可按需逐步补齐；M1 只要求“接口 + 强制绑定点 + 统一错误返回与审计字段口径”。

### 6.5 Ops 推荐栈与访问基线（必须明确）
> 目标：避免 `/health`、`/debug/prometheus`、`/_ops/*` 在生产“默认公网可达”。

- 访问基线（生产必须满足其一）：
  1. 网关/负载均衡层内网隔离或 CIDR allowlist；或
  2. BasicAuth / 静态 token（header）等应用侧鉴权；或
  3. 专用 `OpsGuard` 中间件（按配置启用，拒绝非允许来源）。
- 建议：在 `pkg/middleware` 提供 `OpsGuard`（或等价）并在路由构建层为 ops 前缀统一绑定；避免每个 controller 自行配置。

### 6.6 Middleware Stack Builder / Factory（推荐落地为强约束）
> 目标：将 6.1/6.2/6.4/6.5 的“推荐栈”固化为代码层的可复用工厂，避免各模块在 `module.go` 中手工拼装导致遗漏（例如漏掉 `ProvideUser`、未来的 CSRF、或 webhook 的签名校验）。

- 推荐落点：`pkg/server` 或 `pkg/middleware` 提供标准构建器（按 route_class 维度）。
- 推荐形式（示意，非唯一）：
  - `NewUIStack(...) []mux.MiddlewareFunc`
  - `NewInternalAPIStack(...) []mux.MiddlewareFunc`
  - `NewWebhookStack(providerVerifier ...) []mux.MiddlewareFunc`
  - `NewOpsStack(...) []mux.MiddlewareFunc`
- 注册方式（示意）：
  - 基础设施层创建 `subrouter := r.PathPrefix("/org").Subrouter()`，并统一 `subrouter.Use(NewUIStack(...)...)`；
  - 模块只负责在已绑定 stack 的 subrouter 上注册具体 handler（避免在模块内再拼 middleware）。
- 例外规则：
  - 若某条路由确需偏离标准栈（例如 webhook 需要额外 IP allowlist，或 UI 某页需要匿名访问），必须先在本计划登记为例外并说明原因/安全基线，否则视为违规。

## 7. 安全与鉴权 (Security & Authz)
- **同源与 Cookie**：UI 与内部 API 默认依赖同源 Session；内部 API 禁止被跨站调用（建议在边界处增加 Origin/Referer 校验或同站策略说明）。
- **CSRF**：如未来引入 CSRF 机制，应优先覆盖“会改数据的 UI 写请求 + 内部 API 写请求”，并给出 HTMX 传 token 的标准方案。
- **Webhooks**：
  - 必须进行 provider 签名校验、时间戳/重放保护与 IP allowlist（如适用）。
  - 必须与租户隔离策略明确（例如通过 header/路径/回调字段映射 tenant）。
- **测试端点**：`/__test__/*` 必须受配置开关保护，且默认在生产关闭。
- **Dev-only 端点**：`/_dev/*` 与 `/playground` 必须受配置开关保护且默认生产关闭（应在“是否注册 controller”层面生效，而非仅依赖前端隐藏）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- `pkg/htmx`：用于 HTMX 识别与 `HX-*` 响应头设置。
- 现有 authz forbidden payload 口径（`modules/core/authzutil`）。

### 8.2 里程碑与任务清单
0. [x] **ADR 对齐**：固化 3.3 的裁决，并同步更新 Org 系列计划的 API 前缀（`docs/dev-plans/020-organization-lifecycle.md`、`docs/dev-plans/026-org-api-authz-and-events.md`）。
1. [x] **路由盘点**：输出当前顶层前缀与路由类别清单（UI/Internal API/Public API/Webhooks/Ops/Test），标注“是否符合 4.1”；将结果落盘到附录 B。
2. [x] **策略落盘**：补齐例外清单与迁移策略（存量路径必须说明为何不能迁移/迁移目标/兼容策略/安全基线）。
3. [ ] **统一 responder/协商工具**（推荐）：
   - [ ] 提供统一的 route_class 判定与 responder：HTML/HTMX/JSON forbidden + JSON error envelope。
   - [ ] 让内部 API 的 forbidden/error 不受 `Accept` 影响（保持 JSON-only）。
4. [x] **全局错误契约落地**：
   - [x] 让 `NotFoundHandler`/`MethodNotAllowedHandler` 在 `/{module}/api/*` 与 `/api/v1/*` 下返回 JSON（见 5.5）。
5. [x] **试点迁移（最小改动）**：
   - [x] 将 `/api/lens/events/*` 迁移到 `/core/api/lens/events/*`（保留 alias；写请求不得依赖 redirect）。
   - [x] 将 `/api/website/ai-chat/*` 迁移到 `/api/v1/website/ai-chat/*`（并明确其认证/anti-abuse 基线）。
6. [ ] **新增约束（可验证）**：
   - [x] 增加 route-lint（测试或 lint 规则）：
     - 禁止新增非版本化 `/api/*`（允许 `/api/v1/*`）。
     - 新增“顶层例外/legacy 前缀”必须同步更新附录 B（或等价 allowlist 文件），否则视为违规。
     - 冻结模块（billing/crm/finance）的 legacy 路由仅允许存在于 allowlist 中（冻结政策见仓库根 `AGENTS.md`）；禁止在非冻结模块引入新的 legacy 前缀（防止破窗效应）。
     - 推荐实现：**运行时路由检测（go test）**
       - 构建 server/superadmin 的 router（按入口分别跑），使用 `mux.Router.Walk()` 收集 `PathTemplate/PathPrefix`；
       - 将所有顶层前缀与路由模板按 4.1/4.3 的规则归类并校验（含 `/api/v1`、`/{module}/api`、例外白名单）；
       - route-lint 的 allowlist 输入与 5.5 的 ClassifyPath 规则保持同源（避免两个系统判定口径漂移）。
   - [ ] PR 模板/Reviewer checklist 增加“路由类别与命名空间”校验点。

### 8.3 混合控制器重构模式（建议）
> 背景：存量控制器可能同时包含 HTML/HTMX 与 JSON 的 handler，导致内容协商与错误返回口径容易分散。

- 推荐拆分模式：
  - `XxxPageController`：只服务 UI（例如 `/users`、`/org/*`），默认返回 HTML（按 5.1 支持显式 JSON 诊断）。
  - `XxxAPIController`：只服务内部 API（例如 `/core/api/users`、`/org/api/*`），JSON-only（按 5.2/5.5）。
- 共享方式：
  - 共享 service/domain 层与 DTO（或 mapper），避免在 controller 层复制校验与错误封装逻辑。
  - 403/404/500 统一走同一 responder 工具（见 8.2 的“统一 responder/协商工具”与 5.5 的全局错误契约）。

## 9. 测试与验收标准 (Acceptance Criteria)
- 文档验收：
  - [x] 本计划明确列出并解释所有顶层命名空间与约束（4/5/6 章节完整）。
  - [x] 明确存量例外清单与迁移策略（8.2）。
- 行为验收：
  - [x] 至少 1 个试点模块落地统一协商规则（5.1）且不破坏现有 UI 行为。
  - [x] 内部 API（`/{module}/api/*`）与对外 API（`/api/v1/*`）不会返回 HTML（JSON-only），包含 404/405/500 等全局错误路径（5.5）。
  - [x] 403 forbidden payload 字段口径一致，E2E 可稳定断言（参考现有 authz gating 用例）。
- 安全验收：
  - [x] `/_dev/*` 与 `/playground` 默认在生产不可用（配置开关关闭时应为 404）。
  - [x] Webhooks 入口（`/webhooks/*` + legacy）具备签名校验与重放保护基线（不得依赖 controller 自由发挥）。当前主干无 webhook 路由（`/billing`、`/twilio` 已由 DEV-PLAN-040 移除并为 404）；后续新增必须按本条执行。
  - [x] Ops 入口满足访问基线（网关 allowlist / BasicAuth / OpsGuard 至少一种），不得默认公网可达。
- 门禁：
  - [x] 新增/更新文档通过 `make check doc`。
  - [x] 路由门禁（DEV-PLAN-018B）：`make check routing`（聚合 route-lint、allowlist 健康检查、API 全局错误契约、暴露基线）。

## 10. 运维与监控 (Ops & Monitoring)
- 访问日志字段建议包含：`route_class(ui|authn|internal_api|public_api|webhook|ops|test|static|websocket|dev_only)`、`path_template`、`request_id`、`tenant_id`（如可得）、`authz_object/action`（如适用）。
- 建议为 `/{module}/api/*` 与 `/api/v1/*` 分别设置独立的 rate limit key（按 endpoint + tenant/user），避免 UI 行为影响对外 API 或反之。

--- 

## 附录 A：存量路径处理原则（摘要）
- **外部平台已绑定的回调路径**：优先保留（记录为例外），只做签名校验与安全基线补齐。
- **内部调用广泛的路径（如 `/core/api/authz/*`）**：保留并作为 `/{module}/api/*` 内部 API 规范的“典型样例”。
- **历史偶发/开发工具路径（如 `/_dev`）**：必须受配置开关保护（默认生产关闭），并明确不进入对外文档与稳定契约。

## 附录 B：顶层入口盘点与例外清单（规范性）
> 说明：本表用于支撑 reviewer 与后续 route-lint；只列“顶层入口/单例入口”，不枚举所有子路由。
>
> 字段含义：
> - **处理策略**：`keep`（长期保留）/`migrate`（需迁移）/`gate`（环境开关）/`legacy`（短期保留，后续裁撤）。

| 顶层入口/前缀 | 类别（route_class） | 处理策略 | 迁移目标/备注 |
|---|---|---|---|
| `/assets/*` | static | keep | build 静态资源；长期白名单 |
| `/{UPLOADS_PATH}/*`（默认 `/static/*`） | static | keep | 上传文件静态前缀；需明确缓存策略 |
| `/uploads` | ui | keep | 根路径写接口（POST，返回 HTML/HTMX）；后续可评估是否收敛到 `/core/api/uploads` |
| `/login`、`/logout`、`/oauth/*` | authn | keep | AuthN 边界入口；必须允许 anonymous 访问 |
| `/account`、`/settings`、`/users`、`/roles`、`/groups`、`/spotlight` | ui | keep | core 的根路径 UI 例外（非 `/{module}/...`） |
| `/core/authz` | ui | keep | 权限申请 UI（HTML） |
| `/core/api/authz/*` | internal_api | keep | 既成事实的内部 API SSOT |
| `/api/lens/events/*` | internal_api | migrate | 迁移到 `/core/api/lens/events/*`，保留 alias 窗口期 |
| `/api/website/ai-chat/*` | public_api | migrate | 当前实现更像匿名 public integration；默认迁移到 `/api/v1/website/ai-chat/*` 并补齐 anti-abuse/审计；若后续改为同源 Session 内部交互，则必须改为 `/website/api/ai-chat/*` 并更新本表 |
| `/webhooks/*` | webhook | keep | 新增 webhook 推荐统一前缀；必须满足签名校验与重放保护基线（见 6.4），并在 allowlist 登记 |
| `/query`、`/query/*` | internal_api | legacy | 若保留则补齐 AuthN/Authz 并确保 JSON-only；否则迁移或下线 |
| `/playground` | dev_only | gate | 默认生产关闭（配置开关） |
| `/_dev/*` | dev_only | gate | 默认生产关闭（配置开关） |
| `/debug/prometheus` | ops | keep | 建议网关层白名单/BasicAuth |
| `/health` | ops | keep | 健康检查；长期白名单 |
| `/__test__/*` | test | gate | 受 `ENABLE_TEST_ENDPOINTS` 开关保护，默认生产关闭 |
| `/ws` | websocket | keep | Websocket 升级入口 |
| `/logs`、`/bi-chat` | ui | legacy | 非 `/{module}/...` 的 UI 存量入口；评估是否迁移到模块前缀下 |
| `/superadmin/*`、`/metrics` | ui | keep | superadmin 入口；注意 `/metrics` 不等于 Prometheus scrape |
