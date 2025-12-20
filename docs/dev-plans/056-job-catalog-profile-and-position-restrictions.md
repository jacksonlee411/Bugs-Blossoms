# DEV-PLAN-056：Job Catalog / Job Profile 与 Position Restrictions（对齐 051 阶段 D）

**状态**: 草拟中（2025-12-20 05:35 UTC）

## 0. 评审结论（已采纳）
- **Job Catalog 数据模型（选定）**：四级固定深度（Group → Family → Role → Level），采用“4 张规范化表 + 复合外键（tenant_id,id）”实现层级自洽与租户隔离；只提供启停（disable/enable），不提供 hard delete。
- **Job Profile 绑定关系（选定）**：每个 Job Profile 必须绑定一个 Job Role，可选“允许的 Job Level 集合”，并将该映射作为 Position 写入口的冲突校验 SSOT（对齐 050 §3.1/§7.1）。
- **System/Managed 边界（选定）**：兼容 Org 既有 auto position（System Position）写链路；System Position 允许缺省 masterdata 字段与 restrictions（NULL/空），并默认只读/默认隐藏；Managed Position 逐步开启强校验（对齐 053/055/052）。
- **Restrictions 启用策略（选定）**：Position Restrictions 先落地“可配置模型 + 最小可执行的校验维度”，其他维度先 `shadow` 观测（不阻断）；强制阻断前必须在 059 readiness 中记录灰度与回滚路径（对齐 025 的 freeze mode 口径）。
- **Authz（对齐 054）**：主数据与限制使用独立 object：`org.job_catalog`、`org.job_profiles`、`org.position_restrictions`；action 复用 `read/admin`（不新增 action）。

## 1. 背景与上下文 (Context)
- **需求来源**：[DEV-PLAN-050](050-position-management-business-requirements.md) §3.1/§7.1/§7.7（Job Catalog/Profile 与 Restrictions）；[DEV-PLAN-051](051-position-management-implementation-blueprint.md) 阶段 D（主数据与限制）。
- **仓库现状**：
  - Org BC 已存在 `org_positions/org_assignments`（含 auto position 写链路），但缺少 Job Catalog/Profile 与 Restrictions 的结构化主数据与一致的写入校验入口。
  - HRM 模块存在 legacy `positions` 表（命名冲突风险）；本计划以 Org 的 `org_job_profiles` 为 SSOT，避免形成双主数据。
- **对齐目标**：
  - Position 的“分类（Job Catalog）+ 岗位定义（Job Profile）”口径与 UI/服务端一致（053/055 复用）。
  - Restrictions 的启用与观测遵循项目“先可用、后加严、可灰度、可回滚”的治理策略（对齐 025/059）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **Job Catalog**：四级主数据可维护（最小 CRUD + 启停），并提供 UI/API 消费所需的只读查询（树/级联 options）。
- [ ] **Job Profile**：Profile 主数据可维护（最小 CRUD + 启停），并实现 `Profile -> Role (+ allowed levels)` 绑定/允许集合模型。
- [ ] **Position 扩展字段**：在 `org_positions`（以 053 的 schema 为准）补齐 `job_level_id` 与可选 `job_profile_id`，并把“Profile 与 Catalog 冲突”作为 Position 写入口强校验。
- [ ] **Position Restrictions**：定义并落地可扩展的 restrictions 模型；至少实现“最小可执行维度”的校验（v1），并支持 `disabled/shadow/enforce` 逐步加严。
- [ ] **鉴权与门禁对齐**：主数据/限制的 API 入口接入 `ensureOrgAuthz` 并复用 054 的 object/action；实现阶段按触发器矩阵通过门禁并登记到 059 readiness。

### 2.2 非目标（Out of Scope）
- 不在本计划内引入招聘系统/外部 HR 主数据同步；Restrictions 的“人员属性维度”（雇佣类型/用工类型/地点/成本中心等）只定义数据模型与演进路径，不承诺 v1 全量强校验。
- 不在本计划内改造 Org 的有效期/冻结窗口/审计体系（以 [DEV-PLAN-025](025-org-time-and-audit.md) 与 053 的落地为准）。
- 不在本计划内实现报表与统计聚合（见 [DEV-PLAN-057](057-position-reporting-and-operations.md)）。

## 2.3 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器/工具链”，避免复制命令细节导致 drift；具体命令以 `AGENTS.md`/`Makefile`/`.github/workflows/quality-gates.yml` 为准。

- **触发器清单（勾选本计划命中的项）**：
  - [ ] DB 迁移 / Schema（Org Atlas+Goose 工具链，见 021A）
  - [ ] Go 代码（repo/service/controller + 校验 + 测试）
  - [ ] Authz（若新增策略碎片/fixture，需对齐 054 流程）
  - [ ] `.templ` / Tailwind（若交付维护 UI）
  - [ ] 多语言 JSON（若新增 UI 文案 keys）
  - [ ] 文档（本计划与相关 runbook/readiness 记录）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
  - Org API/Authz/Outbox：`docs/dev-plans/026-org-api-authz-and-events.md`
  - 路由策略（如命中）：`docs/dev-plans/018-routing-strategy.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  UI[Org UI / Admin UI] --> API[/org/api/masterdata & positions/]
  API --> Authz[ensureOrgAuthz<br/>ForbiddenPayload]
  API --> S[Org Staffing Services]
  S --> R[Org Repository]
  R --> DB[(Postgres)]

  subgraph Masterdata[Masterdata]
    DB --> Catalog[org_job_* tables]
    DB --> Profiles[org_job_profiles + allowed_levels]
  end

  subgraph Positions[Positions]
    DB --> Pos[org_positions (job_level_id/job_profile_id/restrictions)]
  end
```

### 3.2 关键设计决策（必须冻结）
1. **Catalog 层级实现（选定：四表）**
   - 约束表达清晰（FK 自洽 + status/unique），避免“通用树 + 业务校验”导致 drift。
2. **启停策略（选定：不级联写入，级联校验）**
   - 禁用父节点不自动改写子节点状态；但任何“被禁用父链路”上的子节点在选择/校验时视为不可用。
   - 禁用节点前做引用检查：存在引用（Job Profile/Managed Position）则拒绝并返回可观测错误（409）。
3. **Job Profile 允许集合（选定：`allow_all_levels` + 明细表）**
   - `allow_all_levels=true`：允许 Role 下全部 levels（无明细行）。
   - `allow_all_levels=false`：必须至少配置 1 个 allowed level（由 service 强校验）。
4. **Position 存储（选定：最小冗余）**
   - Position 存储 `job_level_id`（决定四级路径），可选 `job_profile_id`；不冗余存四级 id，避免一致性约束爆炸。
   - System/auto positions 允许 `job_level_id/job_profile_id` 为空；Managed positions 在写入口强制非空。
5. **Restrictions 存储（选定：Position 上的 `restrictions jsonb`）**
   - 限制条件随 Position 版本演进（有效期一致），避免“限制表与 Position 时间线不同步”。
   - 通过独立 endpoint + 独立 Authz object（`org.position_restrictions`）实现“可授权但不授予 position write”的治理能力。
6. **Restrictions 模式（选定：对齐 freeze mode）**
   - 在 `org_settings` 增加 `position_restrictions_mode`：`disabled/shadow/enforce`。
   - `shadow` 时不阻断写入，但必须在 `org_audit_logs.meta` 记录 `restrictions_mode/violation/details`，并在日志中输出 request_id 便于排障。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本节以“字段类型 + 约束 + 索引”的粒度冻结契约；最终落地通过 Org Atlas+Goose（见 021A）。

### 4.1 Job Catalog（四级）
> 说明：四张表的 `status` 统一使用 `active/disabled`（或等价枚举）；字段名与约束以 Org 既有风格为准（snake_case）。

**`org_job_family_groups`**
- `tenant_id (uuid) NOT NULL`
- `id (uuid) PK`
- `code (varchar(64)) NOT NULL`
- `name (text) NOT NULL`
- `status (text) NOT NULL DEFAULT 'active'`，`CHECK (status IN ('active','disabled'))`
- `created_at/updated_at (timestamptz) NOT NULL DEFAULT now()`
- 约束：
  - `UNIQUE (tenant_id, id)`（用于复合外键）
  - `UNIQUE (tenant_id, code)`
- 索引：`(tenant_id, status, code)`

**`org_job_families`**
- `tenant_id (uuid) NOT NULL`
- `id (uuid) PK`
- `job_family_group_id (uuid) NOT NULL`（FK → `org_job_family_groups (tenant_id,id)`）
- `code (varchar(64)) NOT NULL`
- `name (text) NOT NULL`
- `status (text) NOT NULL DEFAULT 'active'` + check
- `created_at/updated_at`
- 约束：`UNIQUE (tenant_id, id)`、`UNIQUE (tenant_id, code)`
- 索引：`(tenant_id, job_family_group_id, status, code)`

**`org_job_roles`**
- `tenant_id (uuid) NOT NULL`
- `id (uuid) PK`
- `job_family_id (uuid) NOT NULL`（FK → `org_job_families (tenant_id,id)`）
- `code (varchar(64)) NOT NULL`
- `name (text) NOT NULL`
- `status (text) NOT NULL DEFAULT 'active'` + check
- `created_at/updated_at`
- 约束：`UNIQUE (tenant_id, id)`、`UNIQUE (tenant_id, code)`
- 索引：`(tenant_id, job_family_id, status, code)`

**`org_job_levels`**
- `tenant_id (uuid) NOT NULL`
- `id (uuid) PK`
- `job_role_id (uuid) NOT NULL`（FK → `org_job_roles (tenant_id,id)`）
- `code (varchar(64)) NOT NULL`
- `name (text) NOT NULL`
- `status (text) NOT NULL DEFAULT 'active'` + check
- `created_at/updated_at`
- 约束：`UNIQUE (tenant_id, id)`、`UNIQUE (tenant_id, code)`
- 索引：`(tenant_id, job_role_id, status, code)`

### 4.2 Job Profile
**`org_job_profiles`**
- `tenant_id (uuid) NOT NULL`
- `id (uuid) PK`
- `code (varchar(64)) NOT NULL`
- `name (text) NOT NULL`
- `description (text) NULL`
- `status (text) NOT NULL DEFAULT 'active'`，`CHECK (status IN ('active','disabled'))`
- `job_role_id (uuid) NOT NULL`（FK → `org_job_roles (tenant_id,id)`）
- `allow_all_levels (boolean) NOT NULL DEFAULT true`
- `created_at/updated_at`
- 约束：`UNIQUE (tenant_id, id)`、`UNIQUE (tenant_id, code)`
- 索引：`(tenant_id, status, code)`、`(tenant_id, job_role_id)`

**`org_job_profile_allowed_levels`**
- `tenant_id (uuid) NOT NULL`
- `job_profile_id (uuid) NOT NULL`（FK → `org_job_profiles (tenant_id,id)`）
- `job_level_id (uuid) NOT NULL`（FK → `org_job_levels (tenant_id,id)`）
- `created_at (timestamptz) NOT NULL DEFAULT now()`
- 约束：`PRIMARY KEY (tenant_id, job_profile_id, job_level_id)`
- 业务约束（service 层）：
  - `allow_all_levels=true` 时禁止存在明细行；
  - `allow_all_levels=false` 时至少 1 行；
  - 明细行的 `job_level_id` 必须归属 `org_job_profiles.job_role_id`（防“跨 role 允许集合”）。

### 4.3 Position 扩展字段（在 053 schema 基础上追加）
> 说明：本计划只冻结“主数据相关字段”；其它 Position Core 字段以 053 为准。

在 `org_positions` 增加：
- `job_level_id (uuid) NULL`（FK → `org_job_levels (tenant_id,id)`）
- `job_profile_id (uuid) NULL`（FK → `org_job_profiles (tenant_id,id)`）
- `restrictions (jsonb) NOT NULL DEFAULT '{}'::jsonb`

建议索引：
- `(tenant_id, job_level_id, effective_date)`（过滤/报表维度）
- `(tenant_id, job_profile_id, effective_date)`

建议约束（实现阶段择机启用，避免破坏存量链路）：
- `CHECK (is_auto_created OR job_level_id IS NOT NULL)`（Managed positions 强制分类）

### 4.4 Org Settings 扩展（Restrictions mode）
在 `org_settings` 增加：
- `position_restrictions_mode (text) NOT NULL DEFAULT 'shadow'`
- 约束：`CHECK (position_restrictions_mode IN ('disabled','shadow','enforce'))`

### 4.5 Restrictions JSON 形状（v1 契约）
> `org_positions.restrictions` 的 JSON 形状（v1）：

```json
{
  "allowed_assignment_types": ["primary", "matrix", "dotted"],
  "allowed_subject_types": ["person"],
  "notes": "free text (optional)"
}
```

- 解释：
  - `allowed_assignment_types` 缺省/空数组：视为“不限制”（允许全部）。
  - `allowed_subject_types` 缺省：默认仅 `["person"]`；v1 不开放其它 subject_type（与现有 schema 一致）。
  - 预留扩展：未来可追加 `employment_types`、`worker_types`、`locations`、`cost_centers` 等维度，但必须先补齐数据来源与回滚策略（对齐 059）。

## 5. 接口契约 (API Contracts)
> 路由分层对齐 018：JSON-only 内部 API 使用 `/org/api/*`；UI 使用 `/org/*`。

### 5.1 Job Catalog（读）
- `GET /org/api/job-catalog/tree` → `org.job_catalog read`
  - 返回四级树（含 `id/code/name/status/children`），供 UI 级联选择与过滤使用。

### 5.2 Job Catalog（写/启停）
- `POST /org/api/job-catalog/family-groups` → `org.job_catalog admin`
- `PATCH /org/api/job-catalog/family-groups/{id}`（含 `status` 启停）→ `org.job_catalog admin`
- Families/Roles/Levels 同理（路径可在实现阶段收敛，但必须保持可审计与可回滚）。
- 禁用/改关键字段的错误码（建议）：
  - `409 ORG_JOB_CATALOG_IN_USE`：存在引用（profiles/positions）不允许禁用或改关键字段。
  - `422 ORG_JOB_CATALOG_INVALID_PARENT`：父级不存在或跨租户。

### 5.3 Job Profiles（读写）
- `GET /org/api/job-profiles` → `org.job_profiles read`
  - 支持 `status=active|disabled|all`、`q=` 过滤（按 `code/name`）。
- `POST /org/api/job-profiles` → `org.job_profiles admin`
- `PATCH /org/api/job-profiles/{id}` → `org.job_profiles admin`
  - `job_role_id/allow_all_levels/allowed_levels` 的变更必须做影响面检查（见 §6.3）。

### 5.4 Position（写入口的新增校验）
> Position 写入口由 053 定义；本计划只补充“masterdata 相关字段与错误码”。

- 若 `job_level_id` 缺失（Managed positions）→ `422 ORG_POSITION_JOB_LEVEL_REQUIRED`
- 若 `job_level_id` 指向 disabled 链路（任意祖先 disabled）→ `422 ORG_JOB_CATALOG_DISABLED`
- 若 `job_profile_id` 与 `job_level_id` 冲突（role 不匹配或 level 不在允许集合）→ `409 ORG_JOB_PROFILE_CATALOG_CONFLICT`

### 5.5 Position Restrictions（读写）
- `GET /org/api/positions/{id}/restrictions?effective_date=...` → `org.position_restrictions read`
- `POST /org/api/positions/{id}:set-restrictions?effective_date=...` → `org.position_restrictions admin`
  - 语义：创建一个新的 Position 版本（或等价更新策略，以 053 的时间线实现为准），仅变更 `restrictions` 字段，并写入审计 `change_type=position.restrictions.updated`（或等价）。

### 5.6 403 契约（对齐 026）
- 所有 `/org/api/*` 入口鉴权失败必须返回 `modules/core/authzutil.ForbiddenPayload`（含 `missing_policies`、`debug_url`、`request_id`），保证“拒绝 → 申请/调试”闭环。

## 6. 核心逻辑与校验 (Business Logic & Validation)
### 6.1 Catalog 可用性校验（读/写共用）
1. 读取 `job_level_id` 的 role/family/group 链路（同租户）。
2. 任一级 `status=disabled` 视为不可用（返回 `422 ORG_JOB_CATALOG_DISABLED`）。

### 6.2 Profile → Catalog 冲突校验（Position 写入口）
当 Position 同时提供 `job_profile_id` 与 `job_level_id`：
- `job_profile.job_role_id` 必须等于 `job_level.job_role_id`，否则冲突。
- 若 `job_profile.allow_all_levels=false`：`job_level_id` 必须在 allowed_levels 集合内，否则冲突。

### 6.3 Masterdata 变更的引用检查（防“改主数据=静默破坏 Position”）
对以下操作执行引用检查（建议在 service 层实现，失败返回 409 并带可观测信息）：
- 禁用 Job Catalog 节点：检查是否存在引用它的 `org_job_profiles` 或 Managed Positions（as-of now + future window 的具体口径以 052/053 冻结）。
- 修改 Job Profile 的 `job_role_id` 或收缩 allowed_levels：检查是否存在引用该 profile 的 Managed Positions，且会导致冲突；存在则拒绝。

### 6.4 Restrictions 校验（Assignment 写入口）
> v1 最小可执行维度：`allowed_assignment_types`。

1. 读取 Position at `effective_date`，拿到 `restrictions.allowed_assignment_types`。
2. 若限制存在且 `assignment_type` 不在集合内：
   - `org_settings.position_restrictions_mode=disabled`：忽略（不记录）。
   - `shadow`：不阻断，但在 `org_audit_logs.meta` 记录 `restrictions_violation=true` 与 details（并在日志输出）。
   - `enforce`：返回 `409 ORG_POSITION_RESTRICTIONS_VIOLATION`。

## 7. 安全与鉴权 (Security & Authz)
- object/action：复用 054 的命名与边界：`org.job_catalog read/admin`、`org.job_profiles read/admin`、`org.position_restrictions read/admin`。
- 角色建议：复用 054 的 `role:org.staffing.masterdata.admin` 与 `role:org.staffing.viewer/editor/admin` 的只读能力。
- 测试要求：若本计划落地策略碎片，必须补齐 `config/access/fixtures/testdata.yaml` 用例并通过 `make authz-lint`（细节见 054）。

## 8. 测试与验收标准 (Acceptance Criteria)
- [ ] **Schema/迁移**：Org Atlas+Goose plan/lint/migrate up 通过；新增表与约束满足本计划 §4 的契约。
- [ ] **Service 校验**：Profile→Catalog 冲突、Catalog disabled、Restrictions enforce 等核心分支具备单元/集成测试。
- [ ] **鉴权/403 契约**：masterdata 与 restrictions 端点鉴权失败返回 ForbiddenPayload，且 object/action 映射准确（对齐 054/026）。
- [ ] **观测**：shadow 模式下能从审计/日志定位到 restrictions 违例（含 request_id）。
- [ ] **Readiness**：执行门禁并把命令/结果/时间戳登记到 059 指定的 readiness 记录（执行时填写）。

## 9. 运维与监控 (Ops & Monitoring)
- **灰度**：`position_restrictions_mode` 以租户级配置灰度；从 `disabled/shadow` → `enforce` 前必须先补齐策略/数据与回滚路径（对齐 059）。
- **排障**：错误响应与日志必须包含 `request_id/tenant_id`；冲突类错误需包含可读 message（例如“Job Profile 与 Job Catalog 冲突”）。

## 10. 依赖与里程碑 (Dependencies & Milestones)
### 10.1 依赖
- [DEV-PLAN-053](053-position-core-schema-service-api.md)：Position/Assignment 写入口与时间线实现（本计划的字段/校验挂载点）。
- [DEV-PLAN-054](054-position-authz-policy-and-gates.md)：object/action 与策略碎片边界。
- [DEV-PLAN-055](055-position-ui-org-integration.md)：如 UI 需要展示/选择 catalog/profile/restrictions，需消费本计划的 read endpoints。
- [DEV-PLAN-059](059-position-rollout-readiness-and-observability.md)：灰度、门禁记录与回滚约定。

### 10.2 里程碑（建议）
1. [ ] 数据模型与 API 契约冻结（§4-§6）
2. [ ] Org schema 迁移落地（Catalog/Profile/Position 字段）
3. [ ] masterdata 读写 API + 鉴权接入
4. [ ] Profile→Catalog 冲突校验在 Position 写入口生效
5. [ ] Restrictions（最小维度）在 Assignment 写入口生效（shadow→enforce 可灰度）
6. [ ] 测试与 readiness 记录补齐

## 11. 实施步骤
1. [ ] 冻结数据模型与接口契约（对齐 050/051/054/053）
2. [ ] Org Atlas+Goose：新增 Job Catalog/Profile 表与索引/约束
3. [ ] Org Atlas+Goose：扩展 `org_positions` 与 `org_settings`（含兼容性策略）
4. [ ] 服务层：实现 Catalog 可用性校验 + Profile→Catalog 冲突校验
5. [ ] 服务层：实现 masterdata 引用检查（禁用/关键变更）
6. [ ] 服务层：实现 Restrictions 模式与最小维度校验（Assignment 写入口）
7. [ ] API/controller：接入 `ensureOrgAuthz` 与 403 ForbiddenPayload 契约（对齐 026）
8. [ ] 测试：补齐 service/controller 用例 +（若命中）Authz fixtures
9. [ ] 门禁与 readiness：按触发器矩阵执行并记录到 059（执行时填写）

## 12. 交付物
- Job Catalog（四级）与 Job Profile 主数据（含启停与引用治理）的 schema + API。
- Position 的 `job_level_id/job_profile_id/restrictions` 字段与 Profile→Catalog 冲突校验。
- Position Restrictions 的最小可执行校验与 `disabled/shadow/enforce` 灰度能力（含可观测错误与审计 meta）。
