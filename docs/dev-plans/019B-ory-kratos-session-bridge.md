# DEV-PLAN-019B：ORY Kratos 接入与本地 Session 桥接（PoC）

**状态**: 进行中（2025-12-16 09:05 UTC）

> 本文是 `DEV-PLAN-019` 的子计划，聚焦 **身份域（Kratos）** 的“代码级详细设计”。RLS 见 `DEV-PLAN-019A`；企业 SSO 见 `DEV-PLAN-019C`。

## 1. 背景与上下文 (Context)
- 当前认证体系为本地 `users.password` + `sessions`（`sid` cookie），支持 Google OAuth，但缺少标准化的注册/登录/MFA/恢复等完整身份流程。
- 现有登录在无租户上下文时会跨租户按 email 查询用户（`modules/core/infrastructure/persistence/user_repository.go`），当同一邮箱存在多个租户时存在歧义。
- ORY Kratos 提供 Headless 身份流程能力，适合保留 HTMX/Templ UI 的前提下，把认证能力外置并标准化。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 在不破坏现有 `/login` UI 的前提下，引入 Kratos 完成“密码登录”PoC。
- [ ] 登录成功后仍创建本地 `sessions`（`sid` cookie），保持现有 `pkg/middleware/auth.go` 的 session/tenant 注入链路不变。
- [ ] 支持“同一 email 不同租户”场景：登录必须先确定租户，再进行用户查找/身份校验。
- [ ] 通过 Feature Flag 可灰度、可回滚到 legacy 认证。

### 2.2 非目标
- 不在 PoC 阶段完成全量用户/密码迁移到 Kratos（迁移策略单独评审后执行）。
- 不在 PoC 阶段上线完整注册/找回/MFA UI（Kratos 能力预留，但先聚焦登录链路）。
- 不在 PoC 阶段替换 Google OAuth（保持现状，后续可改为 Kratos social login）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图（Mermaid）
```mermaid
sequenceDiagram
  participant B as Browser (HTMX)
  participant A as Go App (/login)
  participant K as ORY Kratos (Public API)
  participant DB as Postgres (users/sessions)

  B->>A: POST /login (Email, Password)
  A->>A: resolve tenant by Host -> ctx tenant_id
  A->>K: Create login flow (API)
  A->>K: Submit login (password_identifier, password, csrf_token)
  K-->>A: success (session token / flow)
  A->>K: whoami (get identity_id + traits)
  A->>DB: find/create local user + create local session
  A-->>B: Set-Cookie sid=...; 302 next
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：Kratos 只负责“认人”，应用继续负责“发本地会话”**
  - 原因：现有权限/导航/租户上下文依赖本地 `sessions`，PoC 阶段保持稳定面最重要。
- **决策 2：登录必须先确定租户**
  - PoC 默认策略：通过 `r.Host` 映射到 `tenants.domain`（仓库已有 `TenantService.GetByDomain`）。
- **决策 3：Kratos identifier 采用租户作用域**
  - 由于 Kratos 的 identifier 通常要求全局唯一，而本系统允许 `(tenant_id, email)` 唯一，PoC 采用：
    - `password_identifier = "{tenant_id}:{lower(email)}"`
    - traits 里保留 `email` 与 `tenant_id` 用于展示与映射。

### 3.3 系统级前置决策（引用 `DEV-PLAN-019`）
> 本计划直接采纳 `docs/dev-plans/019-multi-tenant-toolchain.md` 的系统级契约，并将其在身份域 PoC 中具体化。

- **Tenant Domain Contract（必须一致）**：未登录场景的 tenant 解析以 `tenants.domain` 为权威；值必须为 `lowercase(hostname)` 且不含端口。PoC 默认 tenant 域名统一为 `default.localhost`，不得混用 `default.example.com` 等历史值。
- **Fail-Closed（不得回退跨租户）**：`/login` 与 `/oauth/google/callback` 若无法解析 tenant，必须直接拒绝（`404 Not Found`），不得降级到“跨租户按 email 查 user”。
- **Cookie Domain（PoC 选定）**：为支持按 host 多租户，PoC 的应用侧 `sid`/`oauthState` cookie 在 **非 production** 环境必须为 host-only（不设置 `Domain` 属性）；production 才允许通过 `DOMAIN` 设置 apex 域名策略。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 本地用户扩展（PoC）
为实现“身份对账/防串号”，在本地 `users` 增加 Kratos identity 映射列：
```sql
ALTER TABLE users
  ADD COLUMN kratos_identity_id uuid NULL;

CREATE UNIQUE INDEX users_kratos_identity_id_uq
  ON users (kratos_identity_id)
  WHERE kratos_identity_id IS NOT NULL;
```

约束与行为：
- 同一 Kratos identity 只能绑定一个本地 user（全局唯一）。
- 本地仍保持 `UNIQUE (tenant_id, email)`，允许同 email 多租户。

### 4.2 Kratos Identity Schema（PoC 子集）
traits 需至少包含：
- `tenant_id`（uuid string）
- `email`（string）
- `name.first` / `name.last`（可选）

credentials：
- password identifier 使用 `{tenant_id}:{email}`（见决策 3）。

## 5. 接口契约 (API Contracts)
### 5.1 应用侧（保持入口稳定）
现有入口：
- `GET /login`（`modules/core/presentation/controllers/login_controller.go`）
- `POST /login`（同上）

PoC 行为（在不改变表单字段名的前提下）：
- 当 `IDENTITY_MODE=legacy`：保持当前 `AuthService.CookieAuthenticate(ctx, email, password)`。
- 当 `IDENTITY_MODE=kratos`：改为走 Kratos 验证后再创建本地 session。

### 5.2 配置项（环境变量）
- `IDENTITY_MODE=legacy|kratos`（默认 `legacy`）
- `KRATOS_PUBLIC_URL=http://kratos:4433`（示例）
- （可选）`KRATOS_TIMEOUT=3s`

### 5.3 Kratos（最小集成子集）
> 以 Kratos OpenAPI/版本为准；以下仅定义 PoC 需要的最小交互。

1) 创建登录 Flow（API 模式）
- `GET {KRATOS_PUBLIC}/self-service/login/api`

2) 提交密码登录
- `POST {KRATOS_PUBLIC}/self-service/login?flow={flow_id}`
- Payload（示意）：
  ```json
  {
    "method": "password",
    "password_identifier": "00000000-0000-0000-0000-000000000001:user@example.com",
    "password": "******",
    "csrf_token": "..."
  }
  ```

3) 获取 identity（用于桥接）
- 通过 Kratos 的 whoami/session introspection 获取：
  - `identity.id`
  - `identity.traits.tenant_id` / `identity.traits.email`

### 5.4 Kratos 错误消息到 UI 的映射（标准化）
目标：不引入新的 UI 体系，复用现有 `/login` 页面结构（`errorsMap` + `error` flash）。

- 全局错误（Global）：
  - 来源：`flow.ui.messages` 或非字段错误（如 CSRF/flow 过期）。
  - 映射：写入 `shared.SetFlash(w, "error", ...)`，在页面顶部用现有 Alert/Toast 机制展示。
- 字段错误（Field）：
  - 来源：`flow.ui.nodes[].messages`（按 node 的 `attributes.name` 归类）。
  - 映射：写入 `shared.SetFlashMap(w, "errorsMap", ...)`，并按本地表单字段名对齐：
    - Kratos `password_identifier` → 本地表单 `Email`
    - Kratos `password` → 本地表单 `Password`
  - 备注：未知字段/无法归类的消息降级为全局错误，避免“报错但页面无提示”。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 租户解析（未登录场景）
新增中间件（或在 login controller 内部实现 PoC 版本）：
- 解析 `r.Host`（去除端口并 `strings.ToLower`），调用 `TenantService.GetByDomain(ctx, host)`。
- 将 `tenant_id` 注入 ctx：`composables.WithTenantID(ctx, tenant.ID())`。

适用范围（最小集）：
- `/login`（GET/POST）
- `/oauth/google/callback`（建议同样解析 tenant，避免同 email 多租户时串号）

失败策略（PoC）：
- host 找不到 tenant：返回 `404 Not Found`（fail-closed）；不得降级为跨租户 email 查询。

### 6.2 Kratos 登录 → 本地 Session（算法）
伪代码（示意）：
```go
tenantID := mustResolveTenant(ctx)
identifier := fmt.Sprintf("%s:%s", tenantID, strings.ToLower(email))

flow := kratos.CreateLoginFlow(ctx)
result := kratos.SubmitPassword(ctx, flow.ID, identifier, password, flow.CSRF)
identity := kratos.Whoami(ctx, result.SessionToken)

// 以 (tenant_id, email) 为主键找到本地用户
u, err := users.GetByEmail(composables.WithTenantID(ctx, tenantID), identity.Traits.Email)
if err == ErrUserNotFound {
  u = users.Create(ctx, tenantID, identity.Traits) // PoC: 创建影子用户（password 为空）
}

// 防串号：若本地已绑定 kratos_identity_id 且不一致 -> 拒绝/报警
if u.KratosIdentityID != nil && *u.KratosIdentityID != identity.ID {
  return error("identity mismatch")
}

// 写回绑定
users.SetKratosIdentityID(ctx, u.ID, identity.ID)

// 创建本地 session（复用现有 AuthService/sessionService）
sess := sessionService.Create(ctx, u.ID, tenantID, ip, userAgent)
setSidCookie(sess.Token)
```

关键约束（避免一致性陷阱）：
- PoC 优先走 **API flow（服务端代理）**：只有当本地 user/session 落库成功后才给浏览器下发 `sid` cookie；避免出现“Kratos 已登录但应用未登录”的割裂状态。
- user 绑定必须幂等：同一 `(tenant_id, email)` 重试不会创建重复用户；`kratos_identity_id` 写回允许重复执行但必须防串号（identity mismatch 直接拒绝并告警）。

### 6.3 兼容性与回滚
- legacy 与 kratos 两条路径必须并存，且切换仅依赖 `IDENTITY_MODE`。
- 回滚时不需要清理 `kratos_identity_id` 数据；它只是映射信息。

## 7. 安全与鉴权 (Security & Authz)
- tenant 解析必须是“可信 host”（生产环境需要与反代策略配合，避免 Host header 注入）。
- 禁止记录明文密码；日志只记录 `tenant_id`、`user_id`、`kratos_identity_id`、`flow_id` 等。
- 登录成功后本地 session 的 `TenantID` 必须与解析 tenant 一致（见 `AuthService.authenticate` 逻辑）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
1. [ ] 在 `compose.dev.yml` 增加 Kratos（PoC 环境可先跑最小配置）。
2. [X] 实现 tenant 解析并注入 ctx（至少覆盖 `/login` 与 `/oauth/google/callback`），并移除跨租户 email 回退。（2025-12-16 09:05 UTC）
3. [ ] 实现 Kratos 密码登录链路（API flow）与错误渲染映射。
4. [ ] 实现 identity → 本地 user 绑定（含 `kratos_identity_id` 列与约束）。
5. [ ] 本地开发体验（DX）：更新 `devhub.yml`/Makefile 增加可选 Kratos 依赖的一键启动方式；默认保持 `IDENTITY_MODE=legacy`，让不做身份域相关开发的同学无需启动 Kratos。
6. [ ] 灰度开关与回滚手册（见第 10 节）。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] `IDENTITY_MODE=legacy` 时，现有登录与 Google OAuth 不受影响。
- [ ] `IDENTITY_MODE=kratos` 时，能在 `/login` 完成登录并创建本地 `sid` session。
- [ ] 同一 email 在不同租户存在时，登录必须以 host 解析到的 tenant 为准（不得跨租户命中）。
- [ ] `make check lint` 与相关 `go test` 通过（如涉及迁移按 AGENTS 触发器补跑 DB 验证）。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 关键日志字段（建议）
- `request_id`, `tenant_id`, `identity_mode`, `kratos_flow_id`, `kratos_identity_id`, `user_id`, `error`

### 10.2 回滚（PoC）
- 将 `IDENTITY_MODE=legacy`，服务端立即回到旧认证链路。
- 保留 Kratos 服务但不走链路（或下线 Kratos 容器）。
