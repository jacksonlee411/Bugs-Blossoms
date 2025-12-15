# DEV-PLAN-019：多租户工具链选型与落地

**状态**: 规划中（2025-12-15 13:47 UTC）

## 背景
- `DEV-PLAN-009` 第 6 项提出了“多租户管理工具链评估”的需求。当前系统依赖简单的 `tenant_id` 字段过滤（逻辑隔离）和自建的用户表，缺乏统一的身份认证（Identity）、强数据隔离（Isolation）和企业级 SSO 能力。
- 随着业务扩展，特别是 B2B 场景下，租户对数据安全（防泄露）和集成现有 IdP（如 Okta/AD）的需求日益增强。
- 结合本项目（Go 模块化单体 + HTMX/Templ + PostgreSQL 17）的技术栈特点，需要引入轻量、可控且符合 Go 生态的工具链。

## 目标与非目标

### 目标（Goals）
1. **身份认证 (Identity)**：引入 **ORY Kratos** 替代/增强现有用户认证体系；利用 Headless 特性保持对 HTMX/Templ 的 UI 掌控，同时获得注册/登录/MFA/恢复等标准流程能力。
2. **数据隔离 (Isolation)**：使用 **PostgreSQL Row-Level Security (RLS)** 把租户隔离下沉到数据库层，降低“漏写 `WHERE tenant_id`”导致的数据泄露风险。
3. **企业级 SSO**：引入 **BoxyHQ Jackson** 处理 SAML/OIDC 联邦登录，满足企业租户对接自有 IdP 的需求。
4. **统一授权**：与 **Casbin** (DEV-PLAN-012/013) 集成，形成“Kratos 认人 -> RLS 圈地 -> Casbin 管事”的纵深防御体系。

### 非目标（Non-Goals）
- 本计划不在 M1 内交付：SCIM/目录同步、跨租户数据迁移/合并、生产级 HA/灾备、完整的 IAM 管理后台替换。
- 本计划不承诺一次性把所有模块/所有查询全部切到 RLS（将按模块/表逐步推进）。

## 子计划（按 DEV-PLAN-001 拆分）
> 本文作为总纲/选型与全局契约入口；落地实现的“代码级详细设计”以子计划为准。

- `DEV-PLAN-019A`：PostgreSQL RLS 强租户隔离（PoC）—— `docs/dev-plans/019A-rls-tenant-isolation.md`
- `DEV-PLAN-019B`：ORY Kratos 接入与本地 Session 桥接（PoC）—— `docs/dev-plans/019B-ory-kratos-session-bridge.md`
- `DEV-PLAN-019C`：BoxyHQ Jackson 企业 SSO（PoC）—— `docs/dev-plans/019C-jackson-enterprise-sso.md`

## 选型对比（对齐 DEV-PLAN-009 的“对比矩阵”要求）

### 身份域（Identity）候选

| 方案 | 能力覆盖 | 部署复杂度 | 与 HTMX/Templ 的 UI 集成 | 结论 |
| --- | --- | --- | --- | --- |
| Keycloak（Realm） | 覆盖面广（登录/SSO/管理 UI） | 高（组件重、升级/运维成本高） | 中（往往需要适配其页面/主题） | 作为后备方案，不作为首选 PoC |
| ORY Kratos（Headless） | 覆盖主流身份流程（含 MFA/恢复） | 中（需新增服务与配置） | 高（Headless，保持现有页面与交互） | **首选 PoC**：先打通登录与本地会话桥接 |
| 继续自研（现有 users/sessions） | 现状可用 | 低 | 高（已集成） | 仅短期过渡，长期不满足企业需求 |

### 企业 SSO（SAML/OIDC）候选

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| Keycloak SSO | 一体化 | 强绑定 Keycloak 技术路线 | 与 Keycloak 一并评估 |
| BoxyHQ Jackson（SAML→OIDC） | B2B 常用，SAML 适配面广，可作为 IdP 兼容层 | 需要租户级配置管理与回调链路设计 | **PoC 方向**：验证最小链路与配置模型 |
| 直接在应用里做 SAML（自研/库） | 少一套服务 | 安全与协议细节复杂、长期维护成本高 | 不推荐 |

### 数据隔离（Isolation）候选

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 仅应用层过滤 `tenant_id` | 变更小 | 依赖人为约束，难以彻底避免漏写 | 不满足“强隔离”目标 |
| PostgreSQL RLS | DB 强制隔离，能兜底漏写条件 | 需要可靠的“租户上下文注入”，需评估性能 | **首选 PoC** |

## 架构与关键决策（Architecture & Decisions）

### 架构图（Mermaid）

```mermaid
flowchart TD
  Browser[Browser / HTMX] -->|HTTP| App[Go App (IOTA SDK)]

  subgraph Identity[Identity Plane]
    App -->|创建/提交 Login Flow| Kratos[ORY Kratos (Public API)]
    Kratos -->|可选：OIDC 登录| Jackson[BoxyHQ Jackson (OIDC Provider)]
    Jackson -->|联邦登录| EntIdP[Enterprise IdP (SAML/OIDC)]
  end

  subgraph Authz[Authorization]
    App -->|Authorize(subject, object, action, domain)| Casbin[Casbin]
  end

  subgraph Data[Data Plane]
    App -->|本地 Session Cookie (sid)| Sess[(core.sessions)]
    App -->|Tx + set_config(app.current_tenant)| PG[(PostgreSQL 17)]
    PG -->|RLS Policies| Tables[(Tenant-scoped tables)]
  end
```

### 关键决策摘要（PoC）
- **租户识别**：优先从本地 Session/User 注入 `tenant_id`（已有中间件链路）；对于未登录场景（登录页/SSO），以 `r.Host`/domain 解析到 Tenant（`tenants.domain`）作为 PoC 默认策略。
- **Kratos 集成策略**：PoC 优先采用“应用代理调用 Kratos Public API + 本地 Session 桥接”，避免前端跨域/CORS 并复用现有 `/login` 页面与 cookie/session 机制。
- **RLS 注入策略**：连接池下避免串租户；PoC 统一在事务内使用 `set_config('app.current_tenant', ..., true)`（或 `SET LOCAL`）设置租户上下文，且策略采用 `current_setting('app.current_tenant')` 走 fail-closed。
- **SSO 链路定位**：Jackson 作为 **OIDC Provider**（对接企业 IdP），Kratos 作为 **OIDC Client**（其 OIDC 登录方法指向 Jackson），应用侧不直接对接 OIDC 协议细节（PoC 先走 Kratos）。

## 接口契约（API Contracts，集成层）

### 应用侧登录入口（现状复用）
- `GET /login`：渲染登录页（Templ）。
- `POST /login`：提交登录表单；PoC 阶段保持字段不变（`Email`、`Password`），由服务端决定走“legacy”或“kratos”认证路径（受 Feature Flag 控制，见“运维与监控”）。

### Kratos（登录 Flow，PoC 子集）
> 说明：Kratos 的字段与端点以当期版本 OpenAPI 为准；本节只定义应用集成所需的最小子集。

**1）创建 Login Flow（API 模式，便于后端代理）**
- Request（示例）：`GET {KRATOS_PUBLIC}/self-service/login/api`
- Response（示例子集）：
  ```json
  {
    "id": "flow-uuid",
    "ui": {
      "action": "https://kratos-public/self-service/login?flow=flow-uuid",
      "method": "POST",
      "nodes": [
        { "attributes": { "name": "csrf_token", "value": "..." } },
        { "attributes": { "name": "password_identifier" } },
        { "attributes": { "name": "password" } }
      ],
      "messages": []
    }
  }
  ```

**2）提交登录（密码法）**
- Request（示例）：`POST {KRATOS_PUBLIC}/self-service/login?flow={flow_id}`
- Payload（示例，子集）：
  ```json
  {
    "method": "password",
    "password_identifier": "user@example.com",
    "password": "******",
    "csrf_token": "..."
  }
  ```
- Error（示例）：返回更新后的 `flow.ui.messages` / `node.messages`；应用需把这些消息映射为登录页的错误提示（字段级 + 全局）。

**3）获取 Identity（会话桥接所需）**
- 应用在“提交登录成功”后，需要获得 `identity_id` 与关键 traits（例如 email/name）。
- 最小要求：能通过 Kratos 的 “whoami/session introspection” 能力获取 `identity`（子集字段即可），并将其映射为本地用户与本地 Session。

### Jackson（租户级 IdP 配置，PoC 表达）
> PoC 可先用配置文件表达，后续再落库；关键是明确“每租户一组 SSO 连接”的数据结构与字段含义。

示例结构（YAML，仅表达意图）：
```yaml
tenants:
  - tenant_id: "00000000-0000-0000-0000-000000000001"
    tenant_domain: "default.localhost"
    sso:
      enabled: true
      connections:
        - id: "acme-okta"
          protocol: "saml"
          display_name: "Login with Okta"
          jackson_base_url: "http://jackson:5225"
          saml:
            metadata_url: "https://idp.example.com/metadata.xml"
          oidc:
            issuer: "http://jackson:5225/.well-known/openid-configuration"
            client_id: "..."
            client_secret_ref: "ENV:SSO_ACME_OKTA_CLIENT_SECRET"
```

## 核心逻辑与算法（Business Logic & Algorithms）

### RLS 上下文注入（事务内，fail-closed）
目标：对启用 RLS 的表，保证每次读写都在事务内设置 `app.current_tenant`，避免连接池复用造成串租户。

伪代码（示意）：
```go
func InTenantTx(ctx context.Context, fn func(ctx context.Context) error) error {
  tenantID, err := composables.UseTenantID(ctx)
  if err != nil {
    return err // fail-closed
  }

  return composables.InTx(ctx, func(ctx context.Context) error {
    tx, err := composables.BeginTx(ctx)
    if err != nil {
      return err
    }
    if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", tenantID.String()); err != nil {
      return err
    }
    return fn(ctx)
  })
}
```

### Kratos 会话桥接（Kratos 认人 → 本地 Session）
PoC 目标：不推翻现有 cookie/session 模型；在登录成功后以 `identity` 为输入创建本地 session，并复用现有中间件把 `tenant_id` 注入上下文。

伪代码（示意）：
```go
func LoginWithKratos(ctx context.Context, email, password string) (*http.Cookie, error) {
  tenantID := resolveTenantFromHostOrContext(ctx) // PoC: host/domain -> tenant

  flow := kratos.CreateLoginFlow(ctx)
  kratos.SubmitPassword(ctx, flow.ID, email, password, flow.CSRFToken)

  ident := kratos.Whoami(ctx) // {identity_id, traits.email, ...}

  // PoC：优先按 (tenant_id, email) 找到本地用户；必要时创建影子用户
  user := userRepo.GetOrCreateByEmail(ctx, tenantID, ident.Traits.Email)

  // 创建本地 session，设置 sid cookie（复用现有实现）
  session := sessionRepo.Create(ctx, tenantID, user.ID)
  return buildSidCookie(session.Token), nil
}
```

## 运维与监控（Ops & Monitoring，PoC）

### Feature Flag（建议）
- `IDENTITY_MODE`: `legacy` / `kratos`（全局开关，先灰度到测试环境/少量租户）。
- `RLS_ENFORCE`: `disabled` / `enforce`（PoC 先不做 shadow；按表/模块逐步开启，避免“一刀切”）。
- `SSO_ENABLED_TENANTS`: 租户 allowlist（或反向 denylist），避免未配置 IdP 的租户误入 SSO 路径。

### 关键日志（结构化字段建议）
- 通用：`request_id`, `tenant_id`, `user_id`, `path`, `method`.
- RLS 注入失败：额外记录 `rls_setting_key=app.current_tenant`, `sqlstate`, `error`, `tx`/`span` 标识。
- Kratos 流程：`kratos_flow_id`, `kratos_identity_id`, `kratos_method`, `error`.
- SSO/Jackson：`tenant_id`, `sso_connection_id`, `idp_protocol`, `error`.

### 指标（建议）
- `rls_tenant_setting_errors_total`
- `kratos_login_attempts_total`, `kratos_login_errors_total`
- `sso_login_attempts_total`, `sso_login_errors_total`

### 回滚（PoC）
- Identity：将 `IDENTITY_MODE` 切回 `legacy`，保留现有 `/login` 逻辑作为兜底；Kratos/Jackson 可继续运行但不参与链路。
- RLS：对 PoC 表执行 `ALTER TABLE <table> DISABLE ROW LEVEL SECURITY;`（必要时同时移除 policy），并关闭 `RLS_ENFORCE`。
- SSO：从 allowlist 移除租户或将连接 `enabled=false`，回退到密码登录/其它渠道。

## 风险
- **架构复杂度**: 引入 Kratos 和 BoxyHQ 会增加部署组件（Sidecar 或独立服务），需在 `devhub.yml` 和 CI 中适配。
- **迁移成本**: 现有用户数据迁移至 Kratos，以及现有 SQL 查询适配 RLS（需注入 Session 变量）涉及大量改动。
- **性能影响**: RLS 会对查询计划产生影响，需进行基准测试。

## 实施步骤

### 1. 数据隔离增强 (RLS PoC)
1. [ ] **RLS 原型验证**
   - 在 `modules/hrm` 选取关键表（如 `employees`）启用 RLS。
   - 编写 SQL 策略（建议 fail-closed）：`CREATE POLICY tenant_isolation ON employees USING (tenant_id = current_setting('app.current_tenant')::uuid);`。
   - 验证：在不带 `WHERE` 的查询中，通过事务内 `SET LOCAL app.current_tenant = ...` 控制可见性。
2. [ ] **基础设施适配**
   - 以现有事务/连接池入口为落点：`pkg/composables/db_composables.go`（`BeginTx` / `InTx` / `UseTx`）与 `pkg/repo/repo.go`（Tx 抽象）。
   - PoC 约束：访问启用 RLS 的表时，必须在显式事务中执行；在 `BEGIN` 后第一时间执行 `SET LOCAL app.current_tenant = ...`（或 `SELECT set_config('app.current_tenant', ..., true)`）。
   - 租户来源：统一使用 `composables.UseTenantID(ctx)`（当前租户上下文已在 `pkg/middleware/auth.go` 从 session/user 注入）。
3. [ ] **sqlc 集成**
   - 确认 sqlc 生成的代码与 RLS 兼容（通常透明，但需确保事务上下文正确）。

### 2. 身份层改造 (ORY Kratos PoC)
1. [ ] **环境搭建**
   - 在 `compose.dev.yml` 中添加 ORY Kratos 服务与 Postgres 存储。
   - 配置 Kratos 的 `kratos.yml`，定义 Identity Schema（映射现有 User 模型）。
2. [ ] **前端集成 (HTMX)**
   - 明确现有登录页面入口：`modules/core/presentation/templates/pages/login/`，在保持 UI 风格的前提下接入 Kratos。
   - 保持现有 UI 风格；PoC 优先采用 Go 后端代理调用 Kratos Public API（同域），避免前端跨域/CORS 与 CSRF 处理复杂度。
   - 处理 Kratos 的 Flow 数据结构，渲染错误信息。
3. [ ] **数据同步**
   - 明确会话桥接策略（PoC 推荐最小改动）：Kratos 负责认人；应用侧继续创建/使用本地 `sessions`，并由现有中间件注入 `tenant_id` 上下文。
   - 实现 Kratos Webhook 或登录回调：将 Kratos Identity 同步到本地 `users` 表（影子记录，用于外键关联和业务查询）。

### 3. 企业级 SSO (BoxyHQ)
1. [ ] **集成评估**
   - 部署 BoxyHQ Jackson 服务（Docker）。
   - 明确 PoC 链路：企业 IdP（SAML）→ Jackson（SAML→OIDC，OIDC Provider）→ Kratos（OIDC Client）→ 应用本地 Session。
   - 输出“租户级配置”最小模型：每租户保存 IdP 元数据、回调 URL、OIDC client 信息与启停开关（PoC 可先用配置文件表达，后续再落表）。

### 4. 整合与文档
1. [ ] **全链路验证**
   - 验证流程：用户登录 (Kratos) -> 获取 Session -> 中间件提取 tenant_id -> DB 设置 RLS -> Casbin 校验权限 -> 业务操作。
2. [ ] **文档输出**
   - 更新 `ARCHITECTURE.md` 描述新的安全架构。
   - 编写 `docs/guides/multi-tenancy.md` 指导开发者如何新增支持 RLS 的表。
   - 新增仓库级文档时，在 `AGENTS.md` 的 Doc Map 中补充链接，确保可发现性。

## 验收标准（PoC Readiness）
- [ ] **RLS 可验证**：对 `employees` 启用 RLS 后，跨租户读取必须失败（无数据或报错）；同租户读取正常。
- [ ] **RLS 注入路径明确**：所有访问 RLS 表的路径均在事务内设置 `app.current_tenant`，并能从 `composables.UseTenantID(ctx)` 获取租户。
- [ ] **Kratos 登录可用**：能通过现有登录页完成登录；失败时能渲染 Flow 错误；成功后创建本地 Session 并能访问受保护页面。
- [ ] **Jackson 最小链路**：能通过 Jackson 的 OIDC 链路完成一次联邦登录并落回应用本地 Session（可用 demo IdP/fixtures）。
- [ ] **可灰度/可回滚**：`IDENTITY_MODE`/`RLS_ENFORCE`/租户 allowlist 等开关可用，且文档中有明确回滚步骤。
- [ ] **本地门禁通过**：`go fmt ./... && go vet ./... && make check lint`（如涉及迁移/前端生成物，按 AGENTS 触发器矩阵补跑）。

## 交付物
- `docs/dev-plans/019-multi-tenant-toolchain.md` (本计划)。
- RLS 启用脚本与 Go 适配代码 (`pkg/db/rls`).
- Kratos 配置文件与 Docker Compose 更新。
- 验证报告：RLS 性能损耗评估与 Kratos 集成体验。
