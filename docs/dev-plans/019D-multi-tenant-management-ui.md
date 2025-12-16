# DEV-PLAN-019D：多租户管理页面可视化管理方案（Tenant Console）

**状态**: 规划中（2025-12-16 11:48 UTC）

> 本文是 `DEV-PLAN-019` 的子计划，聚焦“多租户控制面（Control Plane）”的可视化管理：让平台 SuperAdmin（以及可选的租户管理员）能够在页面上查看与管理租户域名、登录方式（legacy/Kratos/SSO）、SSO 连接与健康状态，并可视化 RLS 推进状态；同时严格遵循 `DEV-PLAN-019A/019B/019C` 的安全边界与 fail-closed 契约。

## 1. 背景与上下文 (Context)

- `DEV-PLAN-019` 系列引入三条主线：RLS（`019A`）、Kratos 身份域（`019B`）、Jackson 企业 SSO（`019C`）。
- PoC 阶段大量依赖 `ENV/config/sql` 手工变更（例如 `IDENTITY_MODE`、`SSO_MODE`、`SSO_ENABLED_TENANTS`、`config/sso/connections.yaml`），缺少“看得见 + 可操作 + 可追溯”的控制面。
- 仓库已具备独立部署的 superadmin server（见 `docs/SUPERADMIN.md`）与基础租户/用户页面（`modules/superadmin`），但缺少 domain/identity/sso/rls 的关键状态展示与安全变更工作流。
- 域名与登录链路属于高风险配置：误操作会导致“无法登录/回调失败/跨租户串号”等严重问题；需要可视化引导、校验、审计与回滚路径。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 目标（Goals）

- [ ] 在 superadmin server 中提供“租户总览 + 租户详情”的可视化页面，覆盖：
  - 域名（主域名/别名/验证状态）
  - 登录方式（`legacy|kratos`）与登录入口（password/google/sso）的可视化与可控开关
  - SSO 连接（每租户 `1..N`）的创建/编辑/启停/测试
  - RLS 推进状态（全局 enforce 开关、已启用 RLS 的表/模块清单、fail-closed 观察指标）——以“可视化与排障”为主，不在 UI 里直接改写 policy
- [ ] 所有配置变更具备：输入校验、风险提示、可回滚（至少“禁用/恢复旧配置”）、审计记录。
- [ ] UI 交互遵循项目技术栈：HTMX + Templ + 组件（`components/`），最小 JS（Alpine 用于 Modal/Tab）。
- [ ] 与 `DEV-PLAN-019B` 的“Host → tenant 解析 + fail-closed”以及 `DEV-PLAN-019A` 的 RLS 边界保持一致：不引入“隐式跨租户回退路径”。

### 2.2 非目标（Non-Goals）

- 不在本计划内交付：SCIM/目录同步、复杂 SSO 规则路由、全量 IAM 后台替换、RLS policy 通过 UI 动态改写。
- 不在本计划内处理：生产级 DNS/证书自动化（可提供验证指引与状态）。
- 不在本计划内做跨租户业务数据报表（仅做控制面的配置与健康/指标展示）。

## 3. 角色与权限边界 (Security Model)

### 3.1 角色

- **平台 SuperAdmin**：`users.type = superadmin`；使用独立部署的 superadmin server（`docs/SUPERADMIN.md`）。具备跨租户配置权限。
- **租户管理员（可选 M3）**：普通用户 + Casbin 权限；仅能管理本租户安全配置（例如 SSO 连接），不能影响其他租户。

### 3.2 关键边界与约束

- **跨租户控制面只在 superadmin server**：主应用继续保持“租户内视角”，避免把跨租户能力泄露到业务面。
- **RLS 旁路必须显式**：若未来 superadmin 需要读取启用 RLS 的业务表，必须采用 `DEV-PLAN-019A` 规定的明确旁路（专用 DB role/连接池 + 审计），禁止通过“放宽 policy”绕过。
- **租户解析事实源**：
  - PoC：`tenants.domain`（`DEV-PLAN-019B/019C` 已选定）
  - 目标态：迁移到 `tenant_domains`（本计划提出），但仍保持 fail-closed（找不到 domain ⇒ 404）。

## 4. 信息架构（IA）与路由 (Routes)

### 4.1 SuperAdmin（控制面）

| 页面 | 路由 | 目标 | 备注 |
| --- | --- | --- | --- |
| 租户列表 | `GET /superadmin/tenants` | 搜索/筛选/状态一屏可见 | 复用现有 table scaffold |
| 租户详情（概览） | `GET /superadmin/tenants/{tenant_id}` | 基本信息 + 健康状态 | 新增 |
| 域名管理 | `GET /superadmin/tenants/{tenant_id}/domains` | 域名列表/新增/验证/设为主域 | 新增 |
| 登录方式 | `GET /superadmin/tenants/{tenant_id}/auth` | identity_mode + 登录入口开关 | 新增 |
| SSO 连接 | `GET /superadmin/tenants/{tenant_id}/sso` | 连接 CRUD + 测试 + 启停 | 新增 |
| 租户用户 | `GET /superadmin/tenants/{tenant_id}/users` | 用户列表/重置密码 | 已有 |
| 审计日志 | `GET /superadmin/tenants/{tenant_id}/audit` | 配置变更记录 | 新增（最小可用） |

推荐采用“二级路由 + Tabs（Tabs 只是 UI 表现）”，避免所有操作堆在一个页面，便于分享链接与权限控制。

### 4.2 Tenant Admin（自助，可选 M3）

| 页面 | 路由 | 权限（Casbin） |
| --- | --- | --- |
| SSO 配置 | `GET /settings/security/sso` | `tenant.sso.manage` |
| 域名申请（只读/申请） | `GET /settings/security/domains` | `tenant.domains.read` |

### 4.3 导航入口与页面层级（与现有页面关系）

> 目标：让用户能“从哪来、到哪去”一眼看懂，并明确哪些是现有页面增强，哪些是新增页面。

Superadmin server（控制面，`modules/superadmin`）：

```text
Sidebar（现有）：Dashboard "/"  ·  Tenants "/superadmin/tenants"

Tenants（租户管理）：
└─ /superadmin/tenants                         # 租户列表（现有：增强列与入口）
   ├─ /superadmin/tenants/{tenant_id}          # 租户详情概览（新增）
   ├─ /superadmin/tenants/{tenant_id}/domains  # 域名管理（新增）
   ├─ /superadmin/tenants/{tenant_id}/auth     # 身份/登录方式（新增）
   ├─ /superadmin/tenants/{tenant_id}/sso      # SSO 连接（新增）
   ├─ /superadmin/tenants/{tenant_id}/users    # 租户用户（已有：纳入详情 Tabs 入口）
   └─ /superadmin/tenants/{tenant_id}/audit    # 审计日志（新增）
```

说明：
- “Tabs” 只是 UI 表现：每个 Tab 对应独立路由，支持复制链接、单独做权限控制与审计（避免把所有操作堆在一个页面）。
- 现有租户用户页（`/superadmin/tenants/{tenant_id}/users`）保留：作为详情 Tabs 的 Users 入口；当前页面的 breadcrumb/卡片布局可复用。

主应用（租户内视角，M3 可选）：
- 租户管理员如需自助，仅在主应用的 Settings 下提供 `/settings/security/*`，不把跨租户能力下放到业务面（保持边界清晰）。

## 5. 页面交互（可视化方案）

### 5.0 全局布局（复用现有 superadmin 布局）

租户管理页面在现有 superadmin 的“Sidebar + Navbar + Content”骨架内扩展（HTMX 局部刷新为主）：

```text
┌───────────────────────┬───────────────────────────────────────────────┐
│ Sidebar               │ Top Navbar                                    │
│  - Dashboard (/)      ├───────────────────────────────────────────────┤
│  - Tenants            │ Content（随路由变化）                           │
│                       │  - /superadmin/tenants：租户表格（增强）         │
│                       │  - /superadmin/tenants/{id}：详情+Tabs（新增）  │
└───────────────────────┴───────────────────────────────────────────────┘
```

### 5.1 租户列表（增强现有 `modules/superadmin` 页面）

新增列（视觉优先，避免过度拥挤）：
- 主域名（Primary Domain）
- Identity（`Legacy` / `Kratos`）
- SSO（`Off` / `N connections`）
- RLS（`disabled` / `enforce`，全局态，仅展示为 badge）

操作：
- 行内入口：详情 / 用户
- 导出：保持现状

线框示意：

```text
/superadmin/tenants

[Title: Tenants]  [日期范围筛选] [导出] [搜索]
┌─────────────────────────────────────────────────────────────────────┐
│ Name | Primary Domain | Identity | SSO | RLS | Created | Actions     │
│ ...  | ...            | ...      | ... | ... | ...     | [详情][用户] │
└─────────────────────────────────────────────────────────────────────┘
```

### 5.2 租户详情概览（Overview）

布局建议：
- 顶部：Tenant 名称 + 主域名 + 状态 badge（Active/Inactive）
- 统计卡片（4~6 张）：
  - Identity Mode
  - SSO 连接数 + 最近一次测试结果
  - 最近登录/最近活动（若已有数据）
  - RLS 状态（只读：enforce/disabled；提示受哪些表影响）
- 下方：Tabs（Domains / Auth / SSO / Users / Audit）

线框示意：

```text
/superadmin/tenants/{tenant_id}

Breadcrumb: Dashboard > Tenants > {Tenant}
[Header: TenantName  PrimaryDomain  StatusBadge]
[Cards: Identity | SSO(连接数+最近测试) | Activity | RLS(只读)]
Tabs: Overview | Domains | Auth | SSO | Users | Audit
└─ Tab 内容区：表单/列表均 HTMX 局部刷新（成功后刷新概览卡片）
```

### 5.3 域名管理（Domains）

功能：
- 列表字段：`hostname`、`is_primary`、`verified_at`、`created_at`
- 新增域名：
  - 输入校验：lowercase、去端口、禁止 scheme、长度、字符集
  - 唯一性：全局唯一（同 hostname 不能绑定多个租户）
- 验证方式（MVP，可二选一或同时支持）：
  1. DNS TXT：`_iota-verify.<hostname>` = `<token>`
  2. HTTP challenge：`https://<hostname>/.well-known/iota-verify` 返回 `<token>`
  - UX/可维护性：
    - DNS 传播存在 TTL 延迟；verify 需要支持“手动重试/轮询刷新”，避免用户误以为卡死。
    - 后端应记录 `last_verification_attempt_at` / `last_verification_error`，用于 UI 提示与排障。
- “设为主域名”：
  - 仅允许 verified 域名
  - 弹窗确认：提示影响登录/回调 URL；提供回滚（切回旧主域）
- 删除域名：
  - 不能删除唯一主域名；需先切换主域

### 5.4 身份与登录方式（Auth）

展示并可配置：
- `identity_mode`：`legacy|kratos`
- 登录入口：password / google / sso（每租户开关）

强约束（避免“锁死”）：
- 若启用 SSO 且禁用 password：必须至少存在 `1` 个 `enabled` 且“测试通过/可用”的连接。
- 切换到 kratos：必须通过 Kratos 健康检查（服务可用、schema/配置版本匹配）。

交互建议：
- 表单用 HTMX 提交；成功后通过 `HX-Trigger` 或局部刷新更新 Overview 卡片。

### 5.5 SSO 连接管理（SSO）

列表字段：
- `connection_id`（slug）
- `display_name`
- `protocol`（saml/oidc）
- `enabled`
- `last_test_status` / `last_test_at`

创建/编辑向导（MVP）：
1. 基本信息：`display_name`、`connection_id`、`protocol`
2. Jackson/Kratos 绑定：`jackson_base_url`、`kratos_provider_id`
3. IdP 输入（按协议）：
   - SAML：`metadata_url` 或 `metadata_xml`（择一）
   - OIDC：`issuer` / `client_id` / `client_secret_ref`（仅路线 A：只存引用；支持 `ENV:`/`FILE:`；见 §6.3）
4. 保存后提供“测试连接”（server-to-server）按钮：
   - SAML：拉取 metadata、校验基本字段
   - OIDC：请求 `.well-known/openid-configuration`、校验 issuer 与必要 endpoints；并校验 secret 可解析（避免“启用后才发现缺 secret”）
    - 目标是“配置可用性校验”，不强制走完整浏览器联邦登录

回滚手段：
- disable 连接必须永远可用（作为最小回滚）
- delete 需二次确认，并提示对登录页按钮的影响

## 6. 数据与配置来源（从 PoC 到 Product）

### 6.1 现状（PoC）

- tenant 域名：`tenants.domain`
- identity 模式：`IDENTITY_MODE`（全局 env）
- SSO：`config/sso/connections.yaml` + `SSO_ENABLED_TENANTS` allowlist + `SSO_MODE`

主要问题：
- 变更不可审计、不可追溯、难回滚
- 往往需要重启/发布才能生效（env/config）

### 6.2 目标态（本计划）

引入 DB 配置（控制面）并保留 env 作为 bootstrapping / emergency override：
- DB 为主：superadmin UI CRUD
- env 为兜底：kill switch（全局禁用/强制回退）

优先级建议（SSO 举例）：
1. `SSO_MODE=disabled` ⇒ 全局禁用（最高优先级）
2. DB per-tenant setting（enabled/allowlist/连接）
3. 过渡期：从配置文件导入/只读展示（不再作为最终事实源）

### 6.3 SSO Client Secret 管理（路线 A：`secret_ref`）

> 决策：采用 **路线 A**。数据库只存 `secret_ref`（引用外部 secret），不在 DB/audit 中落 secret 内容。

共同约束（无论选哪条路线）：
- **不在 DB/audit 中落明文 secret**；审计 `payload` 必须过滤敏感字段（secret、token、cookie 等）。
- **fail-closed**：secret 缺失/不可解析 ⇒ 该连接不得启用（`enabled=false`），且“测试连接”必须返回可读错误。
- **最小可用回滚**：disable 连接永远可用；全局 kill switch（`SSO_MODE=disabled`）优先级最高。

数据表达：
- `oidc_client_secret_ref` 采用带 scheme 的字符串：`ENV:<NAME>` 或 `FILE:<ABS_PATH>`
  - `ENV:`：本地/dev 或紧急兜底；变更通常需重启/发布才能生效。
  - `FILE:`：production 首选（K8s Secret volume）；secret 更新可由运行环境刷新文件内容，应用侧按需读取即可生效。
- 解析规则（fail-closed）：
  - scheme 不支持 / 目标不存在 / 内容为空 ⇒ 解析失败（连接不可启用，测试返回错误）。
  - 读取到的内容需 `strings.TrimSpace()`，并拒绝包含换行的值（避免误把多行文件直接当 secret）。

UI/运维约定（最小可用）：
- 表单字段使用 `client_secret_ref`（不收集明文 secret），并提供 `ENV:`/`FILE:` 示例与“如何在部署环境注入”的指引。
- SSO 列表应展示 secret 状态（仅“已配置/未配置/解析失败”），不泄露 secret 片段。
- “测试连接”至少校验：issuer/well-known 可达 + secret_ref 可解析（不保证 secret 正确性）。

未来可选（不在本计划内）：如确需“完全 UI 动态 secret”，再评审引入 DB 密文或外部 Secret Store，并补齐独立安全清单与回滚策略。

## 7. 数据模型与约束 (Data Model & Constraints)

> 本节定义“目标态 contract”。PoC 过渡期允许 dual-read（DB 优先，fallback 旧字段/配置），但必须明确退场计划。

### 7.1 `tenant_domains`（新增，替代 `tenants.domain`）

字段（示意）：
- `id uuid PK DEFAULT gen_random_uuid()`
- `tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE`
- `hostname varchar(255) NOT NULL`
- `is_primary bool NOT NULL DEFAULT false`
- `verification_token varchar(64) NOT NULL`（生成后用于 TXT/HTTP challenge；UI 需可见）
- `last_verification_attempt_at timestamptz NULL`
- `last_verification_error text NULL`
- `verified_at timestamptz NULL`
- `created_at/updated_at timestamptz DEFAULT now()`

约束：
- `UNIQUE (hostname)`（全局唯一）
- `UNIQUE (tenant_id) WHERE is_primary`（每租户至多一个主域）
- 可选 CHECK：`hostname = lower(hostname)`、`position(':' in hostname)=0`

迁移策略：
1. 新增表并 backfill：从 `tenants.domain` 写入 `tenant_domains`（`is_primary=true`，并生成 `verification_token`；为避免破坏现网登录，backfill 的主域名可标记为 `verified_at=now()`）。
2. 解析逻辑升级：优先查 `tenant_domains` 的 primary；过渡期 fallback `tenants.domain`。
3. 稳定后再评估是否移除 `tenants.domain` 或仅保留为冗余/兼容字段。

### 7.2 `tenant_auth_settings`（新增：租户级登录方式）

- `tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE`
- `identity_mode varchar(20) NOT NULL`（`legacy|kratos`）
- `allow_password bool NOT NULL DEFAULT true`
- `allow_google bool NOT NULL DEFAULT true`
- `allow_sso bool NOT NULL DEFAULT false`
- `updated_at timestamptz DEFAULT now()`

### 7.3 `tenant_sso_connections`（新增：租户级连接）

- `id uuid PRIMARY KEY DEFAULT gen_random_uuid()`
- `tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE`
- `connection_id varchar(64) NOT NULL`（slug）
- `display_name varchar(255) NOT NULL`
- `protocol varchar(10) NOT NULL`（`saml|oidc`）
- `enabled bool NOT NULL DEFAULT false`
- `jackson_base_url varchar(255) NOT NULL`
- `kratos_provider_id varchar(64) NOT NULL`
- `saml_metadata_url varchar(1024) NULL`
- `saml_metadata_xml text NULL`
- `oidc_issuer varchar(1024) NULL`
- `oidc_client_id varchar(255) NULL`
- `oidc_client_secret_ref varchar(255) NULL`（只存引用；支持 `ENV:`/`FILE:`；见 §6.3）
- `last_test_status varchar(20) NULL`
- `last_test_error text NULL`
- `last_test_at timestamptz NULL`
- `created_at/updated_at timestamptz DEFAULT now()`

约束（示意）：
- `UNIQUE (tenant_id, connection_id)`
- `CHECK`：按 protocol 约束必填字段（例如 `protocol='saml'` ⇒ metadata_url/xml 至少其一）
  - `CHECK`：当 `protocol='oidc'` 时，secret_ref 必须存在（`oidc_client_secret_ref IS NOT NULL`）

### 7.4 审计（最小可用）

推荐新增跨租户审计表（避免复用 tenant-scoped 的 action_logs 产生语义混乱）：

`superadmin_audit_logs`（建议）
- `id bigserial PRIMARY KEY`
- `actor_user_id int8 NULL`（弱关联；用户删除后可置空/保持快照）
- `actor_email_snapshot varchar(255) NOT NULL`
- `actor_name_snapshot varchar(255) NULL`
- `tenant_id uuid NULL REFERENCES tenants(id)`（可为空表示全局动作）
- `action varchar(64) NOT NULL`（例如 `tenant.domains.add` / `tenant.sso.update`）
- `payload jsonb NOT NULL`（diff + request context；必须过滤 secret 等敏感字段）
- `ip_address inet NULL`
- `user_agent text NULL`
- `created_at timestamptz DEFAULT now()`

索引（建议）：
- `INDEX (tenant_id, created_at DESC)`
- `INDEX (created_at DESC)`

## 8. 接口契约 (API Contracts)

以 superadmin server 为主，HTML + HTMX partials 为主：

- `GET /superadmin/tenants/{id}`：Overview（HTML）
- `GET /superadmin/tenants/{id}/domains`：Domains 页面（HTML）
- `POST /superadmin/tenants/{id}/domains`：新增域名（Form）
  - 200：返回列表/行片段
  - 409：hostname 冲突
  - 422：校验失败（返回带错误的表单片段）
- `POST /superadmin/tenants/{id}/domains/{domain_id}/verify`：触发验证/刷新状态
- `POST /superadmin/tenants/{id}/domains/{domain_id}/make-primary`：设为主域名（成功后刷新 Overview 卡片）
- `GET /superadmin/tenants/{id}/auth`：Auth 页面（HTML）
- `POST /superadmin/tenants/{id}/auth`：更新租户 auth settings（Form）
- `GET /superadmin/tenants/{id}/sso`：SSO 页面（HTML）
- `POST /superadmin/tenants/{id}/sso`：创建连接（Form）
- `POST /superadmin/tenants/{id}/sso/{conn_id}`：更新连接（Form）
- `POST /superadmin/tenants/{id}/sso/{conn_id}/enable|disable|test`：启停/测试

错误码/边界（统一约定）：
- `403 Forbidden`：非 superadmin
- `404 Not Found`：资源不存在或不属于当前 tenant（必须校验归属）
- `409 Conflict`：唯一性冲突（hostname / connection_id）
- `422 Unprocessable Entity`：表单校验失败（HTMX 返回表单片段）

## 9. 依赖与里程碑 (Dependencies & Milestones)

### 9.1 依赖

- `DEV-PLAN-019B`：tenant 解析契约（Host → tenant，fail-closed）
- `DEV-PLAN-019C`：SSO 连接字段语义与 Kratos provider id 的命名约束
- `DEV-PLAN-019A`：RLS 边界与 superadmin 旁路策略（若涉及跨租户读取启用 RLS 的表）

### 9.2 里程碑（建议拆分）

**M1：可视化只读 + 轻量写入（快速见效）**
1. [ ] superadmin 租户列表补齐 domain/identity/sso/rls 状态列（数据源可先来自现有表/配置）。
2. [ ] 新增租户详情页面与 Tabs（Overview/Domains/Auth/SSO/Users/Audit）。
3. [ ] RLS 状态展示（env + DB introspection 只读）。
4. [ ] 补齐 i18n keys（`modules/superadmin/presentation/locales/*.toml`）。

**M2：DB 配置与 CRUD（把手工变更收敛到控制面）**
1. [ ] 引入 `tenant_domains`/`tenant_auth_settings`/`tenant_sso_connections` 迁移与 schema 同步。
2. [ ] 登录页与 tenant 解析改为使用 `tenant_domains`（带过渡 fallback）。
3. [ ] 落地 §6.3（路线 A：`secret_ref`，支持 `ENV:`/`FILE:`）；SSO 连接 CRUD + 测试（Jackson/Kratos health + metadata/well-known 拉取 + secret 可解析校验）。
4. [ ] 审计日志落库并在 UI 展示（至少记录变更 diff + actor）。

**M3：租户自助（若需要）**
1. [ ] 在主应用增加 tenant admin settings 页面（Casbin 鉴权）。
2. [ ] 高风险操作（更换主域名）保留 superadmin 专属；租户侧仅允许“申请/只读”或低风险配置。

## 10. 测试与验收标准 (Acceptance Criteria)

- UI：
  - [ ] `/superadmin/tenants` 能看到 domain/identity/sso/rls 状态；筛选/搜索不引入跨租户回退逻辑。
  - [ ] 租户详情 Tabs 可切换，HTMX partial 更新无整页刷新。
- 域名：
  - [ ] 不能添加带端口/scheme 的 hostname；hostname 全局唯一。
  - [ ] verified 前不能设为主域名；主域名切换可回滚。
  - [ ] 验证失败可提示原因（`last_verification_error`）且可重试。
- SSO：
  - [ ] 连接 CRUD 生效；禁用连接后登录页不再展示对应按钮（同 tenant）。
  - [ ] “测试连接”能给出可读错误（metadata 拉取失败、issuer 不匹配等）。
- 安全：
  - [ ] 所有 superadmin 路由均受 `RequireSuperAdmin()` 保护。
  - [ ] 审计日志记录关键变更（actor snapshot、tenant、action、diff），且 `payload` 不包含 secret 等敏感字段。
- 门禁：
  - [ ] `make check doc` 通过；若引入 Go/迁移/locale 等变更，按 `AGENTS.md` 的触发器矩阵补跑对应命令。

## 11. 回滚与应急 (Rollback & Kill Switch)

- 全局 kill switch（运维兜底）：
  - `SSO_MODE=disabled`：隐藏并拒绝所有 `/login/sso/*`。
  - `IDENTITY_MODE=legacy`：回退到旧登录逻辑（遵循 `DEV-PLAN-019B` 的回滚约定）。
- 租户级回滚（控制面）：
  - 禁用 SSO 连接（`enabled=false`）。
  - 切回旧主域名（`make-primary` 回滚）。
- 数据回滚：
  - migrations 提供 down 仅用于开发/测试环境；生产回滚需另行定义备份与恢复策略。
