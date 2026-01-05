# DEV-PLAN-094：V4 全局路由策略统一（UI/HTMX/API/Webhooks）

**状态**: 草拟中（2026-01-05 09:56 UTC）

> 适用范围：**077 以及之后计划的 V4 Greenfield 全新实现**。  
> 本文以 `DEV-PLAN-018` 为蓝本，冻结 V4 的“全局路由命名空间 + 返回契约 + 安全暴露基线 + 门禁”口径，避免实现期各模块各写一套导致长期漂移。

## 1. 背景与上下文 (Context)

- V4 选择 Greenfield 全新实施（077+），不承担存量路由形态/legacy alias 的兼容包袱，但仍需要“路由治理型 SSOT”来保证一致性与可验证性。
- V4 同时包含 UI（SSR/HTMX）、内部 JSON API（与 UI 同仓同发）、对外 API（如未来引入）、第三方回调（webhooks）、运维端点与测试/开发端点；若缺少统一规则，会直接放大鉴权、错误返回与安全暴露的漂移风险。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] 冻结 V4 **全局路由命名空间**：UI / 内部 API / 对外 API / Webhooks / AuthN / Ops / Dev / Test / Static / Websocket。
- [ ] 冻结每类命名空间的**返回契约**：成功/失败（尤其 404/405/500）的 Content-Type 与 payload 形态。
- [ ] 冻结**内容协商优先级**与统一实现方式（避免 controller 里手写分支）。
- [ ] 冻结 **安全暴露基线**：生产环境默认不暴露 dev/test/playground；ops 端点必须有最小保护基线。
- [ ] 将关键约束固化为**可执行门禁**（routing gates），并给出统一本地入口（参考 `DEV-PLAN-018B` 的 Gate-A/B/C/D 思路）。

### 2.2 非目标（明确不做）

- 不在本计划内迁移/复刻现仓库的历史路由与别名窗口；V4 默认从“强命名空间”起步。
- 不在本计划内引入新的路由框架或 OpenAPI 生成门禁；如需引入，另开子计划并明确边界。
- 不在本计划内规定具体模块路由清单（属于各模块 dev-plan 的职责）；本文只定义全局约束与门禁。

## 3. 工具链与门禁（SSOT 引用）

> 本计划不复制命令矩阵；触发器与门禁以 SSOT 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`
- 参考蓝本（路由 SSOT + 门禁方案）：`docs/dev-plans/018-routing-strategy.md`、`docs/dev-plans/018B-routing-strategy-gates.md`
- allowlist SSOT（路由分类事实源）：`config/routing/allowlist.yaml`（entrypoint key **冻结为**：`server`、`superadmin`）

## 4. 关键决策（ADR 摘要）

### 4.0 5 分钟主流程（叙事 + 草图）

1) 请求进入 router 后，先用 `path -> route_class` 做单点分类（allowlist + 规则兜底，且做 segment 边界匹配）。  
2) 路由注册阶段按 `route_class` 绑定默认 middleware stack（避免 controller 自行拼装导致漂移）。  
3) 业务 handler 只关心“成功响应”，不负责兜底错误格式。  
4) 未命中路由/方法不允许/全局 panic 时，统一进入全局 responder，按 `route_class` 选择 JSON-only 或 HTML/文本响应。  
5) allowlist 不可用时直接 fail-fast，避免静默降级导致“测试口径 != 运行时口径”。

```mermaid
flowchart LR
  R[Request] --> C[ClassifyPath (allowlist)]
  C --> M[Middleware by route_class]
  M --> H[Handler]
  H -->|ok| Resp[Response]
  H -->|404/405/panic| G[Global responder by route_class]
  G --> Resp
```

### 4.1 采用“强命名空间 + route_class 驱动”的治理模型（选定）

- **选定**：沿用 `DEV-PLAN-018` 的核心思想：
  - 路径前缀明确表达语义（UI / API / Webhook / Ops / Dev/Test 等），避免“/api 语义歧义”。
  - `route_class` 由“路径分类器”单点判定，用于驱动：
    - 全局错误处理（404/405/500）返回契约；
    - 默认 middleware stack（按类别统一绑定）。
- **选定**：allowlist SSOT 继续采用 YAML，并支持**多 entrypoint**（tenant app / superadmin app / 未来其他二进制）。
- **选定（冻结）**：entrypoint key 使用稳定枚举（不随“V4/V3”变化）：`server`（tenant app）、`superadmin`（控制面 app）。

### 4.2 命名空间与不变量（选定）

> 约束：所有前缀匹配必须做 **segment 边界** 判断（避免 `/api/v1x` 误命中 `/api/v1`）。

| 路由类别 | 前缀（规范） | 返回契约（默认） | 备注 |
| --- | --- | --- | --- |
| UI（模块 UI） | `/{module}/*` | HTML（HTMX 时 partial） | 避免新增根路径 UI 例外；确需例外必须登记 allowlist |
| 内部 API | `/{module}/api/*` | JSON-only | Always Latest；与 UI 同仓同发，不做对外兼容承诺 |
| 对外 API | `/api/v1/*` | JSON-only | 仅当需要对外兼容承诺时才引入；禁止新增非版本化 `/api/*` |
| Webhooks | `/webhooks/{provider}/*` | JSON-only | 不依赖 UI session；安全中间件必须在前缀级强制绑定 |
| AuthN | `/login` `/logout` `/oauth/*`（或等价） | HTML/Redirect 为主 | 作为“认证边界”特殊入口，必须在 allowlist 明确分类 |
| Ops | `/health` `/debug/prometheus`（或等价） | JSON/文本（按实现） | 生产必须有最小保护基线（见 4.5） |
| Dev-only | `/_dev/*` `/playground` | 任意（但仅 dev） | 生产默认不注册 |
| Test-only | `/__test__/*` | JSON/文本（按实现） | 生产默认不注册 |
| Static/Uploads | `/assets/*` `/static/*` `/uploads/*` | 静态资源 | 需纳入 allowlist，避免被误分类为 UI/API |
| Websocket | `/ws` | upgrade | 需纳入 allowlist，便于 error/guard 处理一致 |

### 4.3 内容协商优先级（选定）

- **选定**：对 UI 类路由（`route_class=ui`），协商顺序固定为：
  1. 显式 JSON：`Accept` 含 `application/json`（返回 JSON）
  2. HTMX：`Hx-Request: true`（返回 HTML partial + HX-* headers）
  3. 默认：HTML 全页
- **选定**：对 API/Webhook 类路由（`internal_api/public_api/webhook`），无论 `Accept`/`Hx-Request` 如何，均 **JSON-only**（避免“API 请求拿到 HTML”）。

### 4.4 全局错误返回契约（选定：按 route_class 决定）

- **选定**：全局 `404/405/500`（包含 panic recovery）必须基于 `route_class` 选择 responder：
  - `internal_api/public_api/webhook`：JSON-only（稳定 envelope，最小字段冻结为：`code`、`message`、`request_id`、`meta.path`、`meta.method`；仅允许向后兼容追加字段）。
  - `ui/authn/static/websocket/ops/dev_only/test`：按类别策略返回（默认 HTML/文本；UI 可在显式 JSON 时返回 JSON）。
- **选定**：分类器与门禁使用同一 allowlist 事实源，避免“测试通过但运行时口径不同”。

### 4.5 安全暴露基线（选定）

- **选定**：生产环境默认不注册 `/_dev/*`、`/playground`、`/__test__/*`（以可测试的方式实现）。
- **选定**：`/health` 与 `/debug/prometheus` 必须具备至少一层应用侧保护（例如 OpsGuard/BasicAuth/显式开关 + deny-by-default）；无法从代码可靠判断的网关侧策略不纳入自动门禁，但必须进入 review checklist。
- **选定（失败模式冻结）**：allowlist 加载失败 / entrypoint 缺失属于“配置不可用”，必须 fail-fast（启动直接失败），禁止静默降级到“无规则/全 UI”。

## 5. V4 门禁（Routing Quality Gates）(Checklist)

> 目标：把本计划的关键契约固化为 CI 可阻断的 gates；实现思路参考 `DEV-PLAN-018B`。

1. [ ] Gate-A：Route Lint —— 禁止新增非版本化 `/api/*`（除非明确允许的例外并登记 allowlist）。
2. [ ] Gate-B：Allowlist 健康检查 —— allowlist 可加载且 entrypoint（`server` / `superadmin`）存在且非空，关键前缀分类稳定。
3. [ ] Gate-C：API/Webhook 全局错误契约 —— 对 `internal_api/public_api/webhook` 的 404/405/500 断言 JSON-only；对 UI 404 保持非 JSON-only（除非显式 `Accept: application/json`）。
4. [ ] Gate-D：环境暴露基线 —— 生产默认不暴露 `/_dev/*`、`/playground`、`/__test__/*`；ops 端点保护基线可测。
5. [ ] 本地入口：提供 `make check routing`（或等价）聚合运行 Gate-A/B/C/D，并写入 `Makefile` 与 CI workflow（以 SSOT 为准）。

## 6. 实施步骤 (Checklist)

1. [ ] 明确二进制边界（tenant app / superadmin app），并在 allowlist SSOT 中维护 `server`、`superadmin` 两个 entrypoint（禁止“v4-server”之类的变体命名）。
2. [ ] 落地“路径分类器”（`path -> route_class`）与单一 responder 入口（用于 404/405/500 与 panic recovery）。
3. [ ] 落地“按 route_class 绑定默认 middleware stack”的 router builder 约束（减少 controller 自由发挥）。
4. [ ] 落地 Gate-A/B/C/D，并将其纳入 CI required checks（以 `.github/workflows/quality-gates.yml` 为准）。
5. [ ] 在后续每个模块 dev-plan 中引用本计划作为路由契约 SSOT；新增根路径例外、webhook 入口或 public API 时，必须补齐 allowlist 与对应 gate 断言。

## 7. 验收标准 (Acceptance Criteria)

- [ ] 新增路由若违反命名空间规则（尤其新增非版本化 `/api/*`）会被 CI 阻断。
- [ ] allowlist 缺失/损坏/entrypoint 缺失不会静默降级，且会被门禁阻断。
- [ ] `internal_api/public_api/webhook` 的 404/405/500 在门禁中稳定断言为 JSON-only。
- [ ] 生产默认不暴露 dev/test/playground 端点，且具备可测试的断言。
- [ ] 本计划被后续 077+ 子计划引用并保持为路由治理 SSOT（避免重复写另一套规则）。
