# DEV-PLAN-088：V4 租户管理与登录认证（Kratos 认人 → RLS 圈地 → Casbin 管事）

**状态**: 草拟中（2026-01-05 07:52 UTC）

> 适用范围：**全新实现的 V4 新代码仓库（Greenfield）**。本文总结现仓库在“租户/认证/会话/RLS/Authz”上的既有实现与已评审契约（`DEV-PLAN-019*`、`DEV-PLAN-081`），并给出 V4 的最小可落地方案。  
> 对齐要求：`DEV-PLAN-082`（DDD 分层框架）、`DEV-PLAN-083`（HR 业务域 4 模块骨架）；本文引入一个 **平台 IAM/Tenancy 模块**，不计入 HR 业务域模块数量。

## 1. 现仓库实现总结（作为输入，不做兼容包袱）

### 1.1 租户（Tenancy）
- **数据模型**：`tenants` 表（含 `id/name/domain/is_active/...`），domain 入库会 `lowercase + trim`，并作为未登录态的 tenant 解析事实源（见 `modules/core/infrastructure/persistence/tenant_repository.go`）。
- **tenant 上下文**：通过 `context` 注入 `tenant_id`（见 `pkg/composables/tenant.go`），业务代码普遍依赖 `composables.UseTenantID(ctx)`；缺 tenant 即报错，避免“跨租户兜底查询”。
- **tenant 解析契约（已有）**：`Host → tenants.domain`，找不到即 fail-closed（见 `docs/dev-plans/019B-ory-kratos-session-bridge.md` 的 Tenant Domain Contract）。

### 1.2 认证与会话（AuthN + Session）
- **现状**：本地用户密码 + 本地 `sessions`（`sid` cookie），并支持 Google OAuth（见 `modules/core/services/auth_service.go`、`modules/core/presentation/controllers/login_controller.go`、`pkg/middleware/auth.go`）。
- **中间件链路**：`sid` cookie/`Authorization` header → 查 session → 注入 session & tenant_id → 再加载 user（见 `pkg/middleware/auth.go`）。
- **已评审演进方向**：`DEV-PLAN-019` 及子计划采用 **ORY Kratos** 作为 Headless Identity，应用保留 `/login` UI；PoC 选择 “Kratos 认人 → 本地 session 桥接”（见 `docs/dev-plans/019B-ory-kratos-session-bridge.md`）。

### 1.3 数据隔离（RLS）
- **现状接口**：事务内设置 `app.current_tenant`（`set_config`），由 RLS policy 读取，实现 fail-closed（见 `pkg/composables/rls.go`；RLS 设计契约见 `docs/dev-plans/019A-rls-tenant-isolation.md`、V4 推进见 `docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`）。

### 1.4 授权（AuthZ）
- **系统级口径**：Casbin（“管事”）与 tenant subject（`tenant:{id}:user:{id}`）结合，形成纵深防御（`docs/dev-plans/019-multi-tenant-toolchain.md`）。

## 2. V4 目标与非目标（Greenfield 口径）

### 2.1 目标（Goals）
- [ ] **租户解析 fail-closed**：任何需要 tenant 语义的入口在 tenant 未解析时必须拒绝（404/401），不得回退跨租户逻辑。
- [ ] **统一认证入口**：仅实现一种主链路（推荐：Kratos password login），避免 legacy/多分支并存。
- [ ] **统一会话模型**：应用侧 `sid` session 作为唯一运行态会话来源，承载 `tenant_id` 与 `principal_id`（对齐现仓库稳定链路）。
- [ ] **RLS 默认开启（fail-closed）**：所有 tenant-scoped 表默认启用 RLS，并要求在事务内注入 `app.current_tenant`。
- [ ] **最小控制面（Tenant Console）**：提供 SuperAdmin 创建/禁用租户、绑定域名、初始化租户管理员的能力（可先无 DNS/HTTP verify）。
- [ ] **对齐 DDD 分层与共享准入**：IAM/Tenancy 作为平台模块按 `modules/{module}/{domain,infrastructure,services,presentation}` 落地；跨模块共用能力优先下沉 `pkg/**`（遵循 `DEV-PLAN-082`）。
- [ ] **测试覆盖率门禁**：新仓库按 100% 覆盖率门禁执行（遵循 `DEV-PLAN-000` 新增要求）。

### 2.2 非目标（Non-Goals）
- 不在本计划内交付：企业 SSO（Jackson）、MFA、SCIM、目录同步、复杂邀请/审批流。
- 不在本计划内兼容现仓库用户/租户数据；如需迁移，单独出计划（含风险评审与回滚）。

### 2.3 关键决策（ADR 摘要，避免实现阶段“撞出来”）
- **决策 1：采用 Kratos，但应用仍保留本地 session**
  - 选择：Kratos 负责“认人”，应用负责“发会话/圈地/管事”（`sid` session + RLS + Casbin）。
  - 理由：运行态只需要一个稳定的 session 事实源；RLS 依赖 `tenant_id`，因此应用必须可确定注入。
- **决策 2：tenant 解析 SSOT 为 `tenant_domains.hostname`**
  - 选择：解析只读 `tenant_domains`（hostname 全局唯一），`tenants.primary_domain` 仅作展示/缓存字段。
  - 理由：避免未来“从 primary_domain 切到 domains”的迁移与回滚复杂度；并明确多域名场景的权威来源。
- **决策 3：登录必须先确定 tenant（fail-closed）**
  - 选择：`/login` 入口也必须先解析 tenant，否则 404；禁止跨租户 email 查询/兜底。
  - 理由：消灭“同一邮箱多租户”歧义，避免串租户与安全事故。
- **决策 4：控制面与数据面隔离**
  - 选择：Tenant Console 在 superadmin server，数据面业务 server 永远不提供跨租户控制入口。
  - 理由：把高风险跨租户能力收口在单一边界，便于审计与回滚（对齐 `DEV-PLAN-019D` 的思路）。

## 3. 模块与分层方案（对齐 082/083）

### 3.1 新增平台模块：`modules/iam`
> 说明：`DEV-PLAN-083` 的 4 个 HR 业务域模块保持不变；`iam` 属于平台能力（Identity & Access Management），为 V4 运行态提供“租户/认证/授权基础设施”。

- `modules/iam/domain/`：
  - 聚合：`tenant`（Tenant + Domain 列表/主域规则）、`principal`（租户内登录主体）、`session`（运行态会话）。
  - 值对象：`hostname`、`email`、`identity_mode`（V4 仅 `kratos`）。
  - 端口（接口）：`TenantRepository`、`PrincipalRepository`、`SessionRepository`、`IdentityProvider`（Kratos client port）、`AuditSink`（可选）。
- `modules/iam/infrastructure/`：
  - Postgres repo（tenant/principal/session）。
  - Kratos client（实现 `IdentityProvider`，仅承载 login/whoami/logout 的最小子集）。
  - RLS 注入（复用 `pkg/` 统一入口）。
- `modules/iam/services/`：
  - `TenantService`：创建租户、绑定/切换主域、启停租户。
  - `AuthService`：登录/登出（使用 `IdentityProvider`），创建/销毁本地 session。
- `modules/iam/presentation/`：
  - Tenant App：`/login`、`/logout`、（可选）`/settings/account`。
  - SuperAdmin：`/superadmin/tenants/**`（Tenant Console MVP）。

### 3.2 `pkg/**` 下沉（跨模块共享）
- `pkg/tenancy`：Host 规范化、tenant 解析中间件、ctx 注入/读取（可直接复用 `composables` 的模式，但建议在新仓库中收敛命名）。
- `pkg/rls`：事务内注入 `app.current_tenant` 的统一入口（对齐现仓库 `pkg/composables/rls.go` 的语义）。
- `pkg/http/middleware`：认证态注入（session→principal→tenant）与 fail-closed guard（对齐现仓库 `pkg/middleware/auth.go` 的语义）。

### 3.3 依赖方向（保证“可替换性/局部性”）
- HR 业务域模块（`orgunit/jobcatalog/staffing/person`）**不得依赖** `modules/iam` 的 domain 类型与 service；它们只依赖：
  - `pkg/tenancy` 提供的 `tenant_id`（以及可选的 `principal_id`）上下文读取；
  - `pkg/rls` 事务注入契约；
  - `pkg/authz`（如有）提供的鉴权门面。
- `modules/iam` 可以依赖 `pkg/**`，但不得反向依赖任何 HR 模块（对齐 `DEV-PLAN-082` 的依赖方向约束）。

## 4. 关键契约与不变量（避免后续试错）

### 4.1 Tenant 解析（Host → Tenant）
- **SSOT**：`tenant_domains.hostname`（`hostname` 全局唯一；`tenants.primary_domain` 仅用于展示/缓存）。
- **规范化**：`lowercase(hostname)` + 去端口；禁止空字符串；禁止 wildcard。
- **fail-closed**：
  - tenant 未解析：返回 `404`（未登录入口）或 `401`（已登录且 session 缺 tenant）。
  - 禁止按 email “全局查找用户”。
- **信任边界**：生产环境只信任反代写入的 host（建议使用 `X-Forwarded-Host` 白名单策略或由网关做 host 校验），禁止 Host header 注入导致“串租户”。

### 4.2 身份（Kratos）到本地主体（Principal）的映射
沿用已评审的 PoC 结论（`DEV-PLAN-019B`），但移除 legacy 分支：
- **identifier**：`{tenant_id}:{lower(email)}`（解决“同一 email 多租户”）。
- **traits（最小子集）**：`tenant_id`、`email`、（可选）`name`。
- **本地 principal**：以 `(tenant_id, email)` 唯一；并绑定 `kratos_identity_id`（全局唯一）用于防串号。

### 4.3 会话（Session）与运行态上下文
- **唯一运行态会话来源**：应用侧 `sid` cookie（或 `Authorization: Bearer <sid>`）。
- **session 必含 tenant_id**：中间件链路必须保证 tenant_id 注入，RLS 才可能 fail-closed。
- **登出**：删除本地 session；（可选）同时调用 Kratos 失效化其 session，避免外部身份仍“认为已登录”。

### 4.4 授权（Casbin）与主体表达
- **主体（Subject）建议**：`tenant:{tenant_id}:principal:{principal_id}`
- **边界**：
  - AuthN（登录）只负责建立 `principal_id` 与 `tenant_id` 的可信来源；
  - AuthZ（Casbin）只负责“是否允许做事”，不得承担 tenant 解析与 session 校验职责；
  - DB（RLS）只负责“圈地”，不得放宽 policy 作为跨租户旁路。

## 5. 数据模型（V4 新仓库建议）

> 本节为目标态 schema 草案；是否启用 domain verify、是否引入企业 SSO、以及 superadmin 的更完整审计模型，后续按子计划扩展。

### 5.1 Tenant（控制面）
- `tenants`
  - `id uuid pk`
  - `name text not null`
  - `primary_domain text not null unique`（仅用于展示/缓存；解析 SSOT 见 `tenant_domains`）
  - `is_active bool not null default true`
  - `created_at/updated_at timestamptz`
- `tenant_domains`
  - `id uuid pk`
  - `tenant_id uuid not null`
  - `hostname text not null unique`
  - `is_primary bool not null default false`
  - `verified_at timestamptz null`（MVP 可不启用 verify，但字段保留）
  - 约束：同 tenant 至多一个 `is_primary=true`；且 `is_primary=true` 的那条必须与 `tenants.primary_domain` 一致（以事务保证）。

### 5.2 Principal / Session（数据面）
- `principals`
  - `id uuid pk`
  - `tenant_id uuid not null`
  - `email text not null`
  - `display_name text null`
  - `status text not null`（`active|disabled`）
  - `kratos_identity_id uuid not null unique`
  - `created_at/updated_at timestamptz`
  - 约束：`unique (tenant_id, email)`
- `sessions`
  - `token text pk`（随机高熵）
  - `tenant_id uuid not null`
  - `principal_id uuid not null`
  - `expires_at timestamptz not null`
  - `ip text null`、`user_agent text null`
  - `created_at timestamptz not null`

### 5.3 RLS（必须）
- `tenants` / `tenant_domains`（控制面）默认不启用 RLS，由 superadmin server 保护访问边界。
- `sessions`（数据面）**不启用 RLS**：
  - 原因：`sid`→`session` 查询发生在 tenant 解析/注入之前；若对 `sessions` 启用 tenant RLS，将形成“先有 tenant 才能取 session / 先有 session 才能得 tenant”的环。
  - 约束：只允许按 `token` 精确查询；不得提供“按 tenant 列表 session”等接口（防止横向枚举）。
- `principals`（数据面）可启用 RLS：
  - 登录入口已通过 Host 解析 tenant；因此可以在查询 principal 之前先注入 `app.current_tenant`。
  - 若实现成本过高，可先不启用 RLS，但必须保持所有查询显式包含 `tenant_id`，并用测试覆盖“缺 tenant 即失败/不跨租户命中”。
- 推荐：**superadmin 使用独立 DB role/连接池**（旁路在连接层完成），tenant app 的 DB role 对业务表强制开启 RLS（对齐 `DEV-PLAN-081` 的 v4 口径）。

## 6. 路由与 UI（最小集）

### 6.1 Tenant App
- `GET /login`：渲染登录页（Host 解析 tenant；未解析返回 404）
- `POST /login`：Kratos login flow（server-side）→ whoami → upsert principal → create session → set `sid`（错误返回 422 并渲染表单错误）
- `POST /logout`：删除 session（可选调用 Kratos logout；无论是否存在 session 都应幂等成功）

### 6.2 SuperAdmin（Tenant Console MVP）
- `GET /superadmin/tenants`：列表
- `POST /superadmin/tenants`：创建租户（含 primary_domain）
- `GET /superadmin/tenants/{tenant_id}`：详情（基础信息 + is_active）
- `POST /superadmin/tenants/{tenant_id}/disable|enable`：启停

SuperAdmin 认证（MVP 建议）：
- 由于本计划聚焦“租户登录链路 + 运行态隔离”，且新仓库处于早期阶段，Tenant Console MVP 可先采用 **环境级保护**（例如反代 BasicAuth / 仅内网可达 / 固定 allowlist）。
- 后续若需要把 superadmin 也纳入同一套 Identity（Kratos/Jackson），建议单独出子计划（例如 088A），避免在本计划中引入第二套 session/主体模型而导致边界不清。

### 6.3 Bootstrap（避免“第一天就锁死”）
- 最小要求：在没有任何 tenant/domain/principal 的情况下，能完成以下闭环：
  1) 创建第一个租户 + 主域名
  2) 创建/绑定第一个租户管理员（principal）
  3) 使用该租户管理员通过 `/login` 登录成功
- 建议方案（Greenfield）：提供一次性 CLI/脚本入口（例如 `cmd/superadmin bootstrap`），用于生成初始租户与管理员，并记录为可审计的“执行痕迹”（但不得输出明文密码/secret）。

## 7. 安全与失败路径（必须显式）
- **fail-closed**：tenant 未解析 / session 缺 tenant_id / RLS 注入失败 ⇒ 直接拒绝请求；不得“默认租户”。
- **cookie 策略**：非 production 使用 host-only cookie；production 允许显式配置 apex domain（对齐 `DEV-PLAN-019B`）。
- **敏感信息**：禁止把密码、token、cookie 写入日志；审计日志需过滤 secret。

## 8. 实施步骤（Greenfield 里程碑）
1. [ ] `modules/iam` 骨架落地（对齐 `DEV-PLAN-082` 分层与依赖方向），补齐端口与最小实体。
2. [ ] Tenant Console MVP：创建/禁用租户与主域名。
3. [ ] tenant 解析中间件：Host → tenant_id 注入（fail-closed）。
4. [ ] Kratos 集成：login flow + whoami 最小子集（`IdentityProvider` 实现）。
5. [ ] 本地 session：创建/校验/登出 + 中间件注入 principal/tenant。
6. [ ] RLS：tenant app 事务内注入 `app.current_tenant`；为 tenant-scoped 表编写 policy（fail-closed）。
7. [ ] Casbin：最小授权（superadmin/tenant admin）与路由保护。

## 9. 测试与验收（新仓库 100% 覆盖率门禁）
- 覆盖率口径与统计范围：按新仓库 SSOT（`Makefile`/CI workflow）执行与记录。
- 单元测试（domain/service）：
  - tenant 域名规范化、主域唯一性、启停行为。
  - principal upsert、防串号（kratos_identity_id 不一致）失败路径。
  - session 创建/过期/登出。
- 集成测试（infrastructure）：
  - Postgres repo + RLS 注入：缺 tenant 时必须失败（fail-closed）。
  - Kratos client：用 stub server 或容器化 Kratos 做契约测试（只覆盖最小端点）。
- E2E（可选，若新仓库已有 e2e 体系）：登录→访问受保护页面→登出。

## 10. 回滚与停止线（避免把复杂度留给实现阶段）
- 回滚（Greenfield 口径）：
  - 禁止引入 “legacy 认证分支” 作为回滚；回滚应通过“保持控制面可用 + 修复配置/数据 + 重试”完成。
  - 对 Kratos 依赖的回滚：允许在本地/dev 通过切换到 stub IdP 或禁用外部调用来维持开发效率，但不得进入生产口径。
- 停止线（命中即打回）：
  - [ ] tenant 未解析时出现跨租户兜底查询（按 email 全局查 principal/user）。
  - [ ] session 中缺 tenant_id 仍允许进入 tenant-scoped 业务查询路径。
  - [ ] 通过放宽 RLS policy 解决 superadmin 跨租户读写需求（必须走显式旁路）。

## 11. Simple > Easy Review（DEV-PLAN-045，自评）
### 结构（解耦/边界）
- 通过：HR 业务域与 `modules/iam` 解耦（只依赖 `pkg/**` 的上下文契约），边界可替换。
- 警告：superadmin 与 tenant app 的“认证/会话”若共享同一套 middleware，易引入隐式耦合；实现时需保持两条路由链路清晰分层。

### 演化（规格/确定性）
- 通过：关键决策以 ADR 摘要固定（Kratos+本地 session、tenant_domains SSOT、fail-closed、控制面隔离）。
- 待补齐：落地前需把 `tenant_domains` 与 `tenants.primary_domain` 的一致性约束写成数据库级约束/事务算法（避免靠“代码约定”）。

### 认知（本质/偶然复杂度）
- 通过：复杂度直接对应不变量（先 tenant 后登录、session 必含 tenant、RLS fail-closed）。
- 警告：Bootstrap 如果仅靠 UI 交互可能演变成“试错流程”；因此明确要求 CLI bootstrap 与可审计痕迹。

### 维护（可理解/可解释）
- 通过：主流程可用一句话描述：`Host 解析 tenant → Kratos 认人 → 本地 session → RLS 圈地 → Casbin 管事`。
