# DEV-PLAN-080：Job Catalog v4（事务性事件溯源 + 同步投射）方案（去掉 org_ 前缀）

**状态**: 草拟中（2026-01-04 04:20 UTC）

> 本计划的定位：为 `DEV-PLAN-078` 的 cutover 提供 **Job Catalog v4 权威契约**（DB Kernel + Go Facade + One Door），并与 `DEV-PLAN-077/079` 对齐“事件 SoT + 同步投射 + 可重放”的范式。

## 1. 背景与上下文 (Context)
- 当前 Job Catalog 位于 `modules/org` 的 schema 与实现中（`org_job_*` + `*_slices`），并与 Position（`org_position_slices.job_profile_id/job_level_code/...`）形成强耦合校验与展示链路。
- `DEV-PLAN-078` 选择彻底 drop 旧 `org_job_*` 相关表，因此 Job Catalog 必须同步给出 v4 替代，否则 Position v4（079）将失去引用对象。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 提供 Job Catalog v4 的 schema（events + identity + versions）与最小不变量集合（可识别、可验收、可重放）。
- [ ] 与 077/079 对齐：Valid Time=DATE、同日事件唯一、**同事务全量重放（delete+replay）**、versions **no-overlap + gapless**、One Door（各实体 `submit_*_event`）。
- [ ] 表命名去掉 `org_` 前缀（见 3.2），并与 Position v4 可组合（079 的 FK 以 `(tenant_id, id)` 为基准）。

### 2.2 非目标（明确不做）
- 不提供对旧 API/旧数据的兼容；cutover 后仅以 v4 为准（由 078 约束）。
- 不保留/不替代旧的 outbox/audit/settings 等支撑能力（对齐 078 的“彻底方案”）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划仅声明命中项与 SSOT 链接，不复制命令清单。

- **触发器（实施阶段将命中）**：
  - [ ] DB 迁移 / Schema（Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [ ] Go 代码（`AGENTS.md`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org v4 cutover：`docs/dev-plans/078-org-v4-full-replacement-no-compat.md`
  - OrgUnit v4：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`
  - Position v4：`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`
  - 多租户隔离（RLS）：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`（对齐 `docs/dev-plans/019-multi-tenant-toolchain.md` / `docs/dev-plans/019A-rls-tenant-isolation.md`）
  - 时间语义（Valid Time=DATE）：`docs/dev-plans/064-effective-date-day-granularity.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 Kernel 边界（与 077/079 对齐）
- **DB = Projection Kernel（权威）**：插入事件（幂等）+ 同步投射 versions + 不变量裁决 + 可 replay。
- **Go = Command Facade**：鉴权/租户与操作者上下文 + 事务边界 + 调 Kernel + 错误映射到 `pkg/serrors`。
- **多租户隔离（RLS）**：v4 tenant-scoped 表默认启用 PostgreSQL RLS（fail-closed；见 `DEV-PLAN-081`），因此访问 v4 的运行态必须 `RLS_ENFORCE=enforce`，并在事务内注入 `app.current_tenant`（对齐 `DEV-PLAN-019/019A`）。
- **One Door Policy（写入口唯一）**：除各实体 `submit_*_event` 与运维 replay 外，应用层不得直写事件表/versions 表/identity 表（`job_family_groups/job_families/job_levels/job_profiles`）及关系表，不得直调 `apply_*_logic`。
- **同步投射机制（选定）**：每次写入都触发**同事务全量重放**（delete+replay），保持逻辑简单，拒绝“增量缝补”分支。

### 3.2 表命名：去掉 `org_` 前缀（评估结论：采用）
**结论（选定）**：Job Catalog v4 表统一去掉 `org_` 前缀，采用 `job_*` 命名（例如 `job_profile_events/job_profiles/job_profile_versions` 等）。

原因：
- `org_` 在本仓库中已强语义绑定 OrgUnit（组织树）子域；Job Catalog 属于独立主数据子域，继续使用 `org_` 会扩大“Org”概念边界并制造漂移。
- 采用 `job_*` 域前缀可避免与其他模块通用表名冲突，并降低未来模块抽离的迁移成本。

### 3.3 时间语义（选定）
- Valid Time：`date`；versions 使用 `daterange` 且统一 `[start,end)`（day-range）。
- Audit/Tx Time：`timestamptz`（`transaction_time/created_at`）。

### 3.4 幂等与同日唯一（选定）
- 事件表提供 `event_id` 幂等键。
- 同一张 events 表内（即每类实体各自的 events 表），同一实体在同一 `effective_date` 只允许一条事件（不引入 `effseq`）。

### 3.5 gapless（选定，纳入合同）
- 各 `*_versions` 必须无间隙：相邻切片满足 `upper(prev.validity)=lower(next.validity)`，最后一段 `upper_inf(validity)=true`。
- 不允许用“缺行”表达停用/撤销：必须用 `is_active/status` 的切片表达（保持时间轴连续）。

### 3.6 为什么“分类数据”也需要 versions + replay（意义与边界）
> 直觉上 Job Catalog 像“字典/分类”，但在 HR 领域它更接近“有效期主数据（SCD2）”：它的变化会影响 Position/Assignment 的 as-of 语义与历史报表一致性。

- **避免“改字典=改历史”**：若只保留当前态（identity 行），任何重命名/停用/归属变更都会让历史快照被动改变，破坏可追溯性与 retro 计算可复现性。
- **支持有效期归属/属性**：例如 `job_family_group_id` 的有效期归属（reparenting）天然是 valid-time 事实，应落在 `job_family_versions`，而不是更新 identity。
- **保持写入简单**：采用“事件入库 → 全量重放生成切片”的固定机制，避免在实现期引入区间 split/merge 的增量缝补算法分叉。
- **成本可控**：Job Catalog 单个实体的事件通常很少（低频变更），按实体 replay 的 delete+rebuild 量级可预期且小于 Position/Assignment 的时间线规模。

## 4. 数据模型与约束 (Data Model & Constraints)
> 说明：以下为 schema 级合同（字段/约束/索引），具体 DDL 最终以 cutover 时落盘的 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为准（由 078 约束）。

### 4.1 Events（Write Side / SoT）
> 决策：不使用 `entity_type/entity_id` 分发器的共享事件表，避免“多主体共用事件表”引入的复杂度；每类实体独立 events 表以满足“同表同日唯一”的合同。

```sql
-- 说明：所有 events 表形状同构；每类实体独立表以保持简单。
-- - 幂等：UNIQUE(event_id)
-- - 同日唯一：UNIQUE(tenant_id, <entity_id>, effective_date)

CREATE TABLE job_family_group_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,
  job_family_group_id uuid NOT NULL,
  event_type       text NOT NULL,
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_family_group_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT job_family_group_events_one_per_day_unique UNIQUE (tenant_id, job_family_group_id, effective_date),
  CONSTRAINT job_family_group_events_group_fk FOREIGN KEY (tenant_id, job_family_group_id) REFERENCES job_family_groups(tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE job_family_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,
  job_family_id    uuid NOT NULL,
  event_type       text NOT NULL,
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_family_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT job_family_events_one_per_day_unique UNIQUE (tenant_id, job_family_id, effective_date),
  CONSTRAINT job_family_events_family_fk FOREIGN KEY (tenant_id, job_family_id) REFERENCES job_families(tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE job_level_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,
  job_level_id     uuid NOT NULL,
  event_type       text NOT NULL,
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_level_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT job_level_events_one_per_day_unique UNIQUE (tenant_id, job_level_id, effective_date),
  CONSTRAINT job_level_events_level_fk FOREIGN KEY (tenant_id, job_level_id) REFERENCES job_levels(tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE job_profile_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,
  job_profile_id   uuid NOT NULL,
  event_type       text NOT NULL,
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_profile_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT job_profile_events_one_per_day_unique UNIQUE (tenant_id, job_profile_id, effective_date),
  CONSTRAINT job_profile_events_profile_fk FOREIGN KEY (tenant_id, job_profile_id) REFERENCES job_profiles(tenant_id, id) ON DELETE RESTRICT
);
```

事件类型与 payload 合同（v1 最小集，选定以可实现为准）：
- **统一约束（所有实体）**：
  - `event_type` 仅允许：`CREATE/UPDATE/DISABLE`。
  - `payload` 必须为 JSON object；未知 key 必须拒绝（稳定错误码见 7.1）。
  - `code` 为 identity 字段，仅允许在 `CREATE` 的 payload 中出现；其余事件若包含 `code` 必须拒绝（identity 不可变）。

- **Job Family Group（`job_family_group_*`）**
  - `CREATE`：必填 `payload.code`、`payload.name`；可选 `payload.description`、`payload.external_refs`。
  - `UPDATE`：patch；允许 keys：`name`、`description`、`is_active`、`external_refs`。
  - `DISABLE`：等价于 `UPDATE` 设置 `is_active=false`（仍保持 gapless）。

- **Job Family（`job_family_*`，支持 effective-dated reparenting）**
  - `CREATE`：必填 `payload.code`、`payload.name`、`payload.job_family_group_id`；可选 `payload.description`、`payload.external_refs`。
  - `UPDATE`：patch；允许 keys：`name`、`description`、`is_active`、`external_refs`、`job_family_group_id`（reparenting）。
  - `DISABLE`：等价于 `UPDATE` 设置 `is_active=false`。

- **Job Level（`job_level_*`）**
  - `CREATE`：必填 `payload.code`、`payload.name`；可选 `payload.description`、`payload.external_refs`。
  - `UPDATE`：patch；允许 keys：`name`、`description`、`is_active`、`external_refs`。
  - `DISABLE`：等价于 `UPDATE` 设置 `is_active=false`。

- **Job Profile（`job_profile_*`）**
  - `CREATE`：必填 `payload.code`、`payload.name`、`payload.job_family_ids`、`payload.primary_job_family_id`；可选 `payload.description`、`payload.external_refs`。
  - `UPDATE`：patch；允许 keys：`name`、`description`、`is_active`、`external_refs`、`job_family_ids`、`primary_job_family_id`。
    - 若出现 `job_family_ids`：语义为“该版本的 families 集合整体替换”（非增量 add/remove），并要求包含 `primary_job_family_id`。
  - `DISABLE`：等价于 `UPDATE` 设置 `is_active=false`（families 仍需满足“至少一个/恰好一个 primary”）。

不变量（必须）：
- versions 侧 `no-overlap + gapless`（3.5）。
- `job_profile_version_job_families`：每个 `job_profile_versions.id` **至少一个 family** 且 **恰好一个 primary**（4.4）。

### 4.2 Identity（code 唯一性事实源）
> 说明：identity 表用于承载 **稳定 ID**（被外部引用的锚点）与 **code 唯一性**；所有有效期属性与可变关系统一落在 versions 表。

```sql
CREATE TABLE job_family_groups (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code      varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_family_groups_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT job_family_groups_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE TABLE job_families (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code      varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_families_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT job_families_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE TABLE job_levels (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code      varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_levels_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT job_levels_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE TABLE job_profiles (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code      varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT job_profiles_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT job_profiles_tenant_id_code_key UNIQUE (tenant_id, code)
);
```

**选定（避免边界漂移）**：
- **支持 effective-dated reparenting**：`job_family_group_id` 作为有效期属性，落在 `job_family_versions`（而不是 identity），通过 `job_family_events → replay_job_family_versions` 变更。
- **简化 code 唯一性口径**：`job_families.code` 选定为**租户内全局唯一**，避免“按 group 维度的时态唯一性”导致额外约束与实现分叉。

> v1 约束（建议固化以保持简单）：identity 的 `code` 视为不可变；如需更换 code，采用“新建实体 + disable 旧实体（versions）”，避免更新 identity 引入第二事实源。

identity 合同补充（v1）：
- identity 行仅允许由各自 `submit_*_event(event_type='CREATE')` 创建；应用层禁止直写。
- `job_families.code` 的“租户内全局唯一”是 schema 层强约束（避免因 group reparenting 触发 code 口径漂移）。

### 4.3 Versions（Read Side / Projection）
> 说明：各实体 versions 使用 `daterange validity` + EXCLUDE no-overlap，并由 replay 生成 gapless（相邻切片无间隙且末段 infinity）。

示例（Job Profile）：
```sql
CREATE TABLE job_profile_versions (
  id              bigserial PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  job_profile_id  uuid NOT NULL,

  name            text NOT NULL,
  description     text NULL,
  is_active       boolean NOT NULL DEFAULT TRUE,
  external_refs   jsonb NOT NULL DEFAULT '{}'::jsonb,

  validity        daterange NOT NULL,
  last_event_id   bigint NOT NULL REFERENCES job_profile_events(id),

  CONSTRAINT job_profile_versions_validity_check CHECK (NOT isempty(validity)),
  CONSTRAINT job_profile_versions_validity_bounds_check CHECK (lower_inc(validity) AND NOT upper_inc(validity)),
  CONSTRAINT job_profile_versions_external_refs_is_object_check CHECK (jsonb_typeof(external_refs) = 'object'),
  CONSTRAINT job_profile_versions_profile_fk FOREIGN KEY (tenant_id, job_profile_id) REFERENCES job_profiles(tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE job_profile_versions
  ADD CONSTRAINT job_profile_versions_no_overlap
  EXCLUDE USING gist (
    tenant_id gist_uuid_ops WITH =,
    job_profile_id gist_uuid_ops WITH =,
    validity WITH &&
  );
```

特别：Job Family 归属 Group（effective-dated reparenting）：
```sql
CREATE TABLE job_family_versions (
  id              bigserial PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  job_family_id   uuid NOT NULL,
  job_family_group_id uuid NOT NULL,

  name            text NOT NULL,
  description     text NULL,
  is_active       boolean NOT NULL DEFAULT TRUE,
  external_refs   jsonb NOT NULL DEFAULT '{}'::jsonb,

  validity        daterange NOT NULL,
  last_event_id   bigint NOT NULL REFERENCES job_family_events(id),

  CONSTRAINT job_family_versions_validity_check CHECK (NOT isempty(validity)),
  CONSTRAINT job_family_versions_validity_bounds_check CHECK (lower_inc(validity) AND NOT upper_inc(validity)),
  CONSTRAINT job_family_versions_external_refs_is_object_check CHECK (jsonb_typeof(external_refs) = 'object'),
  CONSTRAINT job_family_versions_family_fk FOREIGN KEY (tenant_id, job_family_id) REFERENCES job_families(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT job_family_versions_group_fk FOREIGN KEY (tenant_id, job_family_group_id) REFERENCES job_family_groups(tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE job_family_versions
  ADD CONSTRAINT job_family_versions_no_overlap
  EXCLUDE USING gist (
    tenant_id gist_uuid_ops WITH =,
    job_family_id gist_uuid_ops WITH =,
    validity WITH &&
  );
```

其余实体（同构）：
- `job_family_group_versions`（FK→`job_family_groups`；`last_event_id`→`job_family_group_events`）
- `job_level_versions`（FK→`job_levels`；`last_event_id`→`job_level_events`）

索引建议（实现期以 `EXPLAIN` 验证）：
- `*_versions_no_overlap` 会生成 GiST 索引（`tenant_id + <entity_id> + validity`），可保证 as-of 点查命中至多 1 行。
- 若大量查询按租户 + day 拉全量快照，可考虑补充 `gist(tenant_id, validity)` 的 partial 索引（例如 `WHERE is_active = true`），避免扫描大量历史切片。

### 4.4 `job_profile_version_job_families`（ProfileVersion↔Families 多值关系）
> 语义：每个 `job_profile_versions.id` 必须关联 **至少一个** family，且 **恰好一个** `is_primary=true`。

```sql
CREATE TABLE job_profile_version_job_families (
  tenant_id            uuid NOT NULL,
  job_profile_version_id bigint NOT NULL,
  job_family_id        uuid NOT NULL,
  is_primary           boolean NOT NULL DEFAULT FALSE,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT job_profile_version_job_families_pkey PRIMARY KEY (tenant_id, job_profile_version_id, job_family_id),
  CONSTRAINT job_profile_version_job_families_profile_version_fk FOREIGN KEY (job_profile_version_id) REFERENCES job_profile_versions(id) ON DELETE CASCADE,
  CONSTRAINT job_profile_version_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id) REFERENCES job_families(tenant_id, id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX job_profile_version_job_families_primary_unique
  ON job_profile_version_job_families (tenant_id, job_profile_version_id)
  WHERE is_primary = TRUE;
```

> “恰好一个 primary”建议使用 DEFERRABLE CONSTRAINT TRIGGER 落地（对齐既有仓库实践），避免靠应用层计数导致漂移。

## 5. Kernel 写入口（One Door）
> 选定：**同事务全量重放（delete+replay）**。每类实体各自 `submit_*_event`，并在同一事务内完成：事件入库（幂等）→ 全量重放（删除并重建对应 `*_versions`/关系表）→ 不变量裁决（含 gapless/primary family 等）。

### 5.1 并发互斥（Advisory Lock）
**锁粒度（选定）**：同一 `tenant_id` 的 Job Catalog 写入串行化，避免跨实体依赖（family↔group、profile↔families）在实现期引入死锁与漂移。

锁 key（文本）：`org:v4:<tenant_id>:JobCatalog`

### 5.2 写入口（按实体 One Door）
函数签名（建议，与 077/079 对齐）：
```sql
CREATE OR REPLACE FUNCTION submit_job_family_group_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_job_family_group_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;

CREATE OR REPLACE FUNCTION submit_job_family_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_job_family_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;

CREATE OR REPLACE FUNCTION submit_job_level_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_job_level_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;

CREATE OR REPLACE FUNCTION submit_job_profile_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_job_profile_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;
```

统一合同语义（必须）：
0) 多租户上下文（RLS）：写入口函数开头必须断言 `p_tenant_id` 与 `app.current_tenant` 一致（对齐 `DEV-PLAN-081`）。
1) 获取互斥锁：`org:v4:<tenant_id>:JobCatalog`（同一事务内）。
2) 参数校验：`p_event_type` 必须为 `CREATE/UPDATE/DISABLE`；`p_payload` 必须为 object（空则视为 `{}`）。
3) identity 处理：
   - `CREATE`：从 `payload.code` 创建对应 identity 行；若已存在则拒绝（`JOB_*_ALREADY_EXISTS`）。
   - 非 `CREATE`：要求 identity 行已存在；否则拒绝（`JOB_*_NOT_FOUND`）。
4) 写入对应 `*_events`（以 `event_id` 幂等；同一实体同日唯一由约束拒绝）。
5) 幂等复用校验：若 `event_id` 已存在但参数不同，拒绝（`JOB_*_IDEMPOTENCY_REUSED`）；若完全相同则返回既有 event 行 id（不重复投射）。
6) 插入成功后调用对应 `replay_*_versions(p_tenant_id, <entity_id>)`（同一事务内）生成 gapless versions，并裁决 `job_profile_version_job_families` 等不变量（4.4）。

> 说明：不提供 `submit_job_catalog_event(entity_type, ...)` 这种分发器入口，避免多主体共享事件流带来的复杂度与漂移。

### 5.3 replay（按实体，全量重放）
- `replay_job_family_group_versions(p_tenant_id uuid, p_job_family_group_id uuid)`
- `replay_job_family_versions(p_tenant_id uuid, p_job_family_id uuid)`
- `replay_job_level_versions(p_tenant_id uuid, p_job_level_id uuid)`
- `replay_job_profile_versions(p_tenant_id uuid, p_job_profile_id uuid)`：重建 `job_profile_versions` 与 `job_profile_version_job_families`（删除旧 versions 行后可依赖 FK `ON DELETE CASCADE` 清理旧关系）。

> `replay_*` / `apply_*_logic` 属于 Kernel 内部实现细节：用于把事件投射到各 `*_versions` 与关系表，禁止应用角色直接执行。

> 多租户隔离（RLS，见 `DEV-PLAN-081`）：`replay_*` 函数开头必须断言 `p_tenant_id` 与 `app.current_tenant` 一致。

## 6. 读模型封装与查询
函数签名（建议）：
```sql
CREATE OR REPLACE FUNCTION get_job_catalog_snapshot(
  p_tenant_id uuid,
  p_query_date date
) RETURNS TABLE (...);
```

语义：
- `get_job_catalog_snapshot(p_tenant_id, p_query_date)`：返回 as-of 的 group/family/level/profile（含 profile↔families 关系）。

## 7. Go 层集成（事务 + 调用 DB）
- Go 仅负责：鉴权 → 开事务 → 调对应实体的 `submit_*_event` → 提交。
- 错误契约对齐 077：优先用 `SQLSTATE + constraint name` 做稳定映射；业务级拒绝使用“稳定 code（`MESSAGE`）+ `DETAIL`”的异常形状。
- 多租户隔离（RLS）相关失败路径与稳定映射对齐 `DEV-PLAN-081`（fail-closed 缺 tenant 上下文 / tenant mismatch / policy 拒绝）。

### 7.1 错误契约（DB → Go → serrors）
最小映射表（v1，示例 code；落地时以模块错误码表收敛为准）：

| 场景 | DB 侧来源 | 识别方式（建议） | Go `serrors` code |
| --- | --- | --- | --- |
| Job Catalog 写被占用（fail-fast lock） | `pg_try_advisory_xact_lock` 返回 false | 应用层布尔结果 | `JOB_CATALOG_BUSY` |
| Job Catalog 实体不存在 / 已存在 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `JOB_*_NOT_FOUND` / `JOB_*_ALREADY_EXISTS` |
| 参数/事件类型/payload 不合法 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `JOB_*_INVALID_ARGUMENT` |
| 幂等键复用但参数不同 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `JOB_*_IDEMPOTENCY_REUSED` |
| 同一实体同日重复事件 | `*_events_one_per_day_unique` | `23505` + constraint name | `JOB_*_EVENT_CONFLICT_SAME_DAY` |
| 有效期重叠（破坏 no-overlap） | `*_versions_no_overlap` | `23P01` + constraint name | `JOB_*_VALIDITY_OVERLAP` |
| gapless 被破坏（出现间隙/末段非 infinity） | `replay_*_versions` 校验失败 | DB exception `MESSAGE` | `JOB_*_VALIDITY_GAP` / `JOB_*_VALIDITY_NOT_INFINITE` |
| profile↔families 违反“至少一个/恰好一个 primary” | 约束/触发器失败 | `23514/23505` + constraint name 或 DB exception `MESSAGE` | `JOB_PROFILE_FAMILY_CONSTRAINT_VIOLATION` |

## 8. 测试与验收标准 (Acceptance Criteria)
- [ ] RLS（对齐 081）：缺失 `app.current_tenant` 时对 v4 表的读写必须 fail-closed；tenant mismatch 必须稳定失败可映射。
- [ ] 事件幂等：同 `event_id` 重试不重复投射。
- [ ] 全量重放：每次写入都在同一事务内 delete+replay 对应 versions，且写后读强一致。
- [ ] 同日唯一：同一实体同日提交第二条事件被拒绝且可稳定映射错误码（每类实体独立 events 表）。
- [ ] versions no-overlap：任一实体不会产生重叠有效期。
- [ ] versions gapless：相邻切片无间隙且末段到 infinity（失败可稳定映射错误码）。
- [ ] profile↔families：每个 profile version 恰好一个 primary family（DB 约束可验收）。
- [ ] as-of 查询：任意日期快照结果与 versions 语义一致（`validity @> date`）。

## 9. 运维与灾备（Rebuild / Replay）
当投射逻辑缺陷导致 versions 错误时，可通过 replay 重建读模型（versions 可丢弃重建）：
- Group：`SELECT replay_job_family_group_versions('<tenant_id>'::uuid, '<job_family_group_id>'::uuid);`
- Family：`SELECT replay_job_family_versions('<tenant_id>'::uuid, '<job_family_id>'::uuid);`
- Level：`SELECT replay_job_level_versions('<tenant_id>'::uuid, '<job_level_id>'::uuid);`
- Profile：`SELECT replay_job_profile_versions('<tenant_id>'::uuid, '<job_profile_id>'::uuid);`

> 建议在执行前复用同一把维护互斥锁（`org:v4:<tenant_id>:JobCatalog`）确保与在线写入互斥。

> 多租户隔离（RLS，见 `DEV-PLAN-081`）：replay 必须在显式事务内先注入 `app.current_tenant`，否则会 fail-closed。
