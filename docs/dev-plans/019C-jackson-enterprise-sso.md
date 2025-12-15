# DEV-PLAN-019C：BoxyHQ Jackson 企业 SSO（PoC）

**状态**: 规划中（2025-12-15 13:47 UTC）

> 本文是 `DEV-PLAN-019` 的子计划，聚焦 **企业 SSO（Jackson）** 的“代码级详细设计”。RLS 见 `DEV-PLAN-019A`；Kratos 登录与本地会话桥接见 `DEV-PLAN-019B`。

## 1. 背景与上下文 (Context)
- B2B 场景下，企业租户通常要求对接自有 IdP（Okta/Azure AD/ADFS 等），最常见协议为 SAML。
- BoxyHQ Jackson 可作为“IdP 兼容层”：对外提供 OIDC（应用更易集成），对内对接 SAML/OIDC IdP。
- 本仓库的身份域 PoC 选择 ORY Kratos；在 SSO 场景下建议采用：**Jackson（OIDC Provider）→ Kratos（OIDC Client）→ 应用本地 Session**。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 为租户提供“登录页可见的 SSO 入口”（PoC：配置驱动，不做后台 UI）。
- [ ] 用户通过 SSO 登录后，仍创建本地 `sid` session（复用 `DEV-PLAN-019B` 的桥接策略）。
- [ ] 支持租户级启停/allowlist，避免未配置 IdP 的租户误入 SSO 流程。
- [ ] 具备可回滚路径：随时关闭租户 SSO 或回退到密码登录。

### 2.2 非目标
- 不在 PoC 阶段实现 SCIM/目录同步、自动建号、租户管理员自助配置 UI。
- 不在 PoC 阶段实现多连接编排（如同时支持多个 IdP、策略路由、Just-in-time provisioning 的复杂规则），先支持“每租户 1~N 个连接”的静态配置。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图（Mermaid）
```mermaid
sequenceDiagram
  participant B as Browser
  participant A as Go App
  participant K as Kratos (OIDC Client)
  participant J as Jackson (OIDC Provider)
  participant I as Enterprise IdP (SAML/OIDC)

  B->>A: GET /login (Host=tenant-domain)
  A-->>B: Render login page (SSO buttons if enabled)
  B->>A: GET /login/sso/{connection}
  A-->>B: 302 redirect to Kratos login (provider=jackson-{connection})
  B->>K: start login flow (OIDC)
  K->>J: OIDC auth request
  J->>I: SAML/OIDC federation
  I-->>J: assertion
  J-->>K: id_token
  K-->>B: 302 return_to=/login/sso/callback
  B->>A: GET /login/sso/callback (Kratos session cookie present on same host)
  A->>K: whoami (forward Kratos cookie)
  A-->>B: Set-Cookie sid=...; 302 next
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：应用不直接实现 OIDC/SAML 协议**
  - PoC 由 Jackson 与 Kratos 承担协议细节；应用只负责租户选择、入口路由与“本地 Session 桥接”。
- **决策 2：Kratos session cookie 共享同一 host**
  - PoC 约束：Kratos Public 与应用必须在同一 `tenant_domain` 下（端口不同可接受），使浏览器能把 Kratos session cookie 发送到应用回调端点。
- **决策 3：SSO 连接以租户配置驱动**
  - PoC 用配置文件表达（避免 DB 迁移扩大范围）；后续如要支持租户自助管理，再落库与 UI（需独立计划/门禁）。
- **决策 4：回调桥接必须可重试（避免“Kratos 有 Session 但应用无 Session”）**
  - SSO 场景下 Kratos 通常会在浏览器侧持有 session cookie；若应用侧本地 session 创建失败，用户会处于“已完成联邦登录但无法进入应用”的割裂状态。
  - PoC 要求：`/login/sso/callback` 逻辑幂等、可重复调用；并在 `/login` 提供可选的“继续完成登录/重试桥接”路径（见 §6.4）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 SSO 连接配置（PoC：配置文件）
建议新增配置文件（路径示例）：`config/sso/connections.yaml`

结构（示意）：
```yaml
tenants:
  - tenant_id: "00000000-0000-0000-0000-000000000001"
    tenant_domain: "default.localhost"
    sso:
      enabled: true
      connections:
        - id: "acme-okta"
          display_name: "Login with Okta"
          protocol: "saml" # or oidc
          # Jackson 服务地址（用于健康检查/跳转拼装/运维排障）
          jackson_base_url: "http://jackson:5225"
          # 对应 Kratos OIDC provider 的 id（必须与 kratos.yml 配置一致）
          kratos_provider_id: "jackson-acme-okta"
```

约束：
- `tenant_id + connection.id` 必须唯一。
- `kratos_provider_id` 必须唯一（全局），避免选择 provider 时冲突。

### 4.2 Kratos 配置要求（摘要）
PoC 需要在 Kratos 中静态配置 OIDC provider 指向 Jackson（以 Kratos 版本与配置格式为准）：
- Issuer：Jackson 的 `.well-known/openid-configuration` 对应地址
- client_id / client_secret：由 Jackson 生成或配置
- scopes：至少 `openid email profile`

## 5. 接口契约 (API Contracts)
### 5.1 应用侧端点
- `GET /login`：渲染登录页；若当前 tenant 有启用的 `connections`，在页面上渲染 SSO 按钮列表。
- `GET /login/sso/{connection}`：
  - 行为：校验当前 tenant 是否启用该 connection；构造 Kratos 登录入口并 302 跳转。
  - 失败：404（未知 connection）或 403（租户未启用/不在 allowlist）。
- `GET /login/sso/callback`：
  - 行为：从当前请求读取 Kratos session cookie，调用 Kratos whoami 获取 identity，然后复用 `DEV-PLAN-019B` 的逻辑创建本地 `sid` session 并跳转到 `next`。

### 5.2 Feature Flag / Allowlist
- `SSO_ENABLED_TENANTS`：逗号分隔的 tenant_id allowlist（或 `*` 表示全部，默认空表示禁用）
- `SSO_MODE=disabled|enforce`（默认 `disabled`）

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 `/login` 渲染逻辑（伪代码）
```go
tenant := resolveTenantByHost(r.Host)
cfg := ssoConfig.ForTenant(tenant.ID)
if cfg.Enabled && allowlisted(tenant.ID) {
  props.SSOConnections = cfg.Connections // 用于渲染按钮
}
renderLogin(props)
```

### 6.2 `/login/sso/{connection}` 跳转逻辑（伪代码）
```go
tenant := resolveTenantByHost(r.Host)
conn := ssoConfig.MustGet(tenant.ID, connectionID)
assertAllowlisted(tenant.ID)

// 关键：让 Kratos 在完成 OIDC 后回到本应用回调
returnTo := fmt.Sprintf("%s/login/sso/callback?next=%s", appOrigin, url.QueryEscape(next))

// PoC：以 Kratos 的 browser flow 为主；provider 选择方式以 Kratos UI nodes 为准
redirect := kratos.BuildOIDCLoginURL(conn.KratosProviderID, returnTo)
http.Redirect(w, r, redirect, http.StatusFound)
```

### 6.3 `/login/sso/callback` 会话桥接（伪代码）
```go
// 前提：Kratos session cookie 会随请求进入本应用（同 host）
identity := kratos.WhoamiForwardCookie(r)

// 复用 019B：按 tenant + identity.traits.email 找/建本地 user，绑定 kratos_identity_id
u := ensureLocalUser(identity)

sid := sessionService.Create(ctx, u.ID, tenantID, ip, ua)
setSidCookie(sid)
redirect(next)
```

### 6.4 桥接失败的自愈/重试（PoC）
当 `/login/sso/callback` 在“本地落库/发 cookie”阶段失败时（例如 DB 短暂不可用）：
- 用户已在 Kratos 侧具备 session cookie，但应用侧没有 `sid`。
- PoC 要求提供至少一种可重试路径（两者可选其一或同时支持）：
  1. **回调可重试**：对 `/login/sso/callback` 做幂等处理（绑定 identity、创建 session），用户刷新即可重试。
  2. **登录页自动修复**：`GET /login` 检测 Kratos session（通过 whoami）且本地未登录时，提示“检测到已完成 SSO，点击继续”并触发桥接（避免用户卡死在登录页）。

## 7. 安全与鉴权 (Security & Authz)
- `connection` 必须进行租户归属校验，避免用户通过路径探测使用其他租户的 IdP 配置。
- callback 端点必须校验 `tenant_id` 一致性（host 解析的 tenant 与 identity.traits.tenant_id / 本地 user.tenant_id 一致）。
- 建议记录并告警：`sso_connection_id`, `kratos_identity_id`, `tenant_id` 的不一致或缺失。

## 8. 依赖与里程碑 (Dependencies & Milestones)
1. [ ] `DEV-PLAN-019B` 完成 tenant 解析与 Kratos whoami 桥接能力（SSO 回调复用）。
2. [ ] 部署 Jackson（dev compose）并准备一个可用的 demo IdP（或使用 Jackson 示例 IdP）。
3. [ ] 在 Kratos 配置 OIDC provider 指向 Jackson（静态配置）。
4. [ ] 实现 `/login` SSO 入口渲染与 `/login/sso/*` 路由。
5. [ ] 本地开发体验（DX）：更新 `devhub.yml`/Makefile 增加 Jackson 的可选一键启动，并提供“仅打开 SSO/仅打开 Kratos”的组合方式。
6. [ ] 完成 end-to-end：SSO 登录 → 本地 sid session → 可访问受保护页面。
7. [ ] 实现桥接失败自愈/重试路径（§6.4），并将其纳入验收。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] 未启用 SSO 的租户：登录页不展示 SSO 按钮，且访问 `/login/sso/{connection}` 返回 403/404。
- [ ] 启用 SSO 的租户：完成一次 SSO 登录后，本地 `sid` session 生效。
- [ ] 回调时 tenant 不一致或 connection 不属于租户：拒绝并记录结构化日志。
- [ ] 桥接失败可恢复：模拟本地 session 创建失败后，再次访问 `/login/sso/callback` 或 `/login` 能完成桥接并进入应用。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 关键日志字段（建议）
- `request_id`, `tenant_id`, `sso_connection_id`, `kratos_flow_id`, `kratos_identity_id`, `error`

### 10.2 回滚（PoC）
- 租户级：将该 tenant 的 `sso.enabled=false` 或从 `SSO_ENABLED_TENANTS` allowlist 移除。
- 全局：`SSO_MODE=disabled`，隐藏入口并拒绝路由。
