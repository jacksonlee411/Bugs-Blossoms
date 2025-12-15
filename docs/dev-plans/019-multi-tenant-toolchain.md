# DEV-PLAN-019：多租户工具链选型与落地

**状态**: 规划中（2025-12-15 12:30 UTC）

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
   - 保持现有 UI 风格，将表单提交目标改为 Kratos Public API，或通过 Go 后端代理转发。
   - 处理 Kratos 的 Flow 数据结构，渲染错误信息。
3. [ ] **数据同步**
   - 明确会话桥接策略（PoC 推荐最小改动）：Kratos 负责认人；应用侧继续创建/使用本地 `sessions`，并由现有中间件注入 `tenant_id` 上下文。
   - 实现 Kratos Webhook 或登录回调：将 Kratos Identity 同步到本地 `users` 表（影子记录，用于外键关联和业务查询）。

### 3. 企业级 SSO (BoxyHQ)
1. [ ] **集成评估**
   - 部署 BoxyHQ Jackson 服务（Docker）。
   - 明确 PoC 链路：企业 IdP（SAML）→ Jackson（SAML→OIDC）→ Kratos（OIDC Provider）→ 应用本地 Session。
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
- [ ] **本地门禁通过**：`go fmt ./... && go vet ./... && make check lint`（如涉及迁移/前端生成物，按 AGENTS 触发器矩阵补跑）。

## 交付物
- `docs/dev-plans/019-multi-tenant-toolchain.md` (本计划)。
- RLS 启用脚本与 Go 适配代码 (`pkg/db/rls`).
- Kratos 配置文件与 Docker Compose 更新。
- 验证报告：RLS 性能损耗评估与 Kratos 集成体验。
