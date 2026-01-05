# DEV-PLAN-079：Position v4（事务性事件溯源 + 同步投射）方案（去掉 org_ 前缀）

**状态**: 草拟中（2026-01-04 04:20 UTC）

> 本计划的定位：为 `DEV-PLAN-078` 的 cutover 提供 **Position/Assignment 的 v4 权威契约**（DB Kernel + Go Facade + One Door），并与 `DEV-PLAN-077`（OrgUnit v4）对齐“事件 SoT + 同步投射 + 可重放”的范式。

## 1. 背景与上下文 (Context)
- 当前 Position/Assignment 位于 `modules/org` 的 schema 与实现中（`org_positions/org_position_slices/org_assignments` 等），并强依赖 `org_nodes`（FK）。
- `DEV-PLAN-078` 选择彻底替换 OrgUnit（077）并 drop 旧表，因此 Position/Assignment 必须同步给出 v4 替代，否则无法 drop `org_nodes`。
- 本计划采用与 077 相同的 Kernel 边界：DB 负责不变量与投射，Go 只做鉴权/事务/调用与错误映射。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 提供 Position/Assignment v4 的 schema（events + versions）与最小不变量集合（可识别、可验收、可重放）。
- [ ] 与 `DEV-PLAN-077` 的口径对齐：Valid Time=DATE、同日事件唯一、**同事务全量重放（delete+replay）**、versions **no-overlap + gapless**、One Door（`submit_*_event`）。
- [ ] 表命名去掉 `org_` 前缀（见 3.2），并与 Job Catalog v4（`DEV-PLAN-080`）可组合使用。

### 2.2 非目标（明确不做）
- 不提供对旧 API/旧数据的兼容；cutover 后仅以 v4 为准（由 `DEV-PLAN-078` 约束）。
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
  - OrgUnit v4：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`
  - Org v4 cutover：`docs/dev-plans/078-org-v4-full-replacement-no-compat.md`
  - 多租户隔离（RLS）：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`（对齐 `docs/dev-plans/019-multi-tenant-toolchain.md` / `docs/dev-plans/019A-rls-tenant-isolation.md`）
  - Job Catalog v4：`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`
  - 时间语义（Valid Time=DATE）：`docs/dev-plans/064-effective-date-day-granularity.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 Kernel 边界（与 077 对齐）
- **DB = Projection Kernel（权威）**：插入事件（幂等）+ 同步投射 versions + 不变量裁决 + 可 replay。
- **Go = Command Facade**：鉴权/租户与操作者上下文 + 事务边界 + 调 Kernel + 错误映射到 `pkg/serrors`。
- **多租户隔离（RLS）**：v4 tenant-scoped 表默认启用 PostgreSQL RLS（fail-closed；见 `DEV-PLAN-081`），因此访问 v4 的运行态必须 `RLS_ENFORCE=enforce`，并在事务内注入 `app.current_tenant`（对齐 `DEV-PLAN-019/019A`）。
- **One Door Policy（写入口唯一）**：除 `submit_*_event` 与运维 replay 外，应用层不得直写事件表/versions 表/identity 表（`positions/assignments`），不得直调 `apply_*_logic`。
- **同步投射机制（选定）**：每次写入都触发**同事务全量重放**（delete+replay），保持逻辑简单，拒绝增量缝补逻辑分叉。

### 3.2 表命名：去掉 `org_` 前缀（评估结论：采用）
**结论（选定）**：Position/Assignment v4 表统一去掉 `org_` 前缀，采用 `positions/position_events/position_versions` 与 `assignments/assignment_events/assignment_versions`。

原因：
- `org_` 在本仓库中已强语义绑定 OrgUnit（组织树）子域；Position/Job Catalog 在 078 的 v4 里将按独立子域落地，继续使用 `org_` 容易造成“权威表达边界”混淆。
- 去前缀后仍保留足够的域前缀（`position_*`/`assignment_*`），可降低与其他模块表名冲突的概率，并为未来从 `modules/org` 抽离模块预留空间。

### 3.3 时间语义（选定）
- Valid Time：`date`；versions 使用 `daterange` 且统一 `[start,end)`（day-range）。
- Audit/Tx Time：`timestamptz`（`transaction_time/created_at`）。

### 3.4 幂等与同日唯一（选定）
- 事件表提供 `event_id` 幂等键（建议应用传入，重试同 `event_id` 不重复投射）。
- 同一 `position_id`（或 `assignment_id`）在同一 `effective_date` 只允许一条事件（不引入 `effseq`）。

### 3.5 gapless（选定，纳入合同）
- `position_versions` / `assignment_versions` 必须无间隙：相邻切片满足 `upper(prev.validity)=lower(next.validity)`，最后一段 `upper_inf(validity)=true`。
- 不允许用“缺行”表达停用/撤销：必须用 `lifecycle_status/status` 的切片表达（保持时间轴连续）。

### 3.6 汇报线无环（选定，纳入合同）
> 需求确认：Position 的汇报线必须无环（acyclic），且该不变量由 DB Kernel 裁决，禁止仅靠应用层约定。

- **无环定义（as-of）**：对任意 `query_date`，取该日所有 `lifecycle_status='active'` 的 `position_versions`，其 `reports_to_position_id` 形成的有向图必须无环（forest）。
- **禁止自指**：`reports_to_position_id = position_id` 必须被拒绝。
- **引用可用性（as-of）**：`reports_to_position_id` 必须引用 as-of 存在且 `lifecycle_status='active'` 的 Position；否则拒绝（稳定错误码见 7.1）。
- **校验触发点（选定）**：`submit_position_event → replay_position_versions → validate_position_reporting_acyclic(p_tenant_id, p_position_id)`（同一事务内）。  
  说明：由于支持“插入历史事件（retro）”，校验必须覆盖事件写入后该 `position_id` 在未来时间窗可能形成的环（不能只校验 `effective_date` 当天）。

### 3.7 跨聚合不变量（避免边界漂移）
- **占编/容量不变量（必须）**：对任意 `query_date`，同一 `position_id` 的 active assignments `SUM(allocated_fte) <= position.capacity_fte`。  
  关键点：`capacity_fte` 可能由 Position 事件改变，因此该不变量必须在 **Assignment 写入** 与 **Position 写入** 两侧都裁决（否则边界不干净：Position 降容会让既有 assignments 变为“事后非法”而无人发现）。
- **停用不变量（选定）**：对任意 `query_date`，若 `position.lifecycle_status<>'active'`，则不允许存在 `status='active'` 的 assignments 引用该 position（可在同一裁决函数中实现为 `SUM(active allocated_fte)=0`）。

## 4. 数据模型与约束 (Data Model & Constraints)
> 说明：以下为 schema 级合同（字段/约束/索引），具体 DDL 最终以 cutover 时落盘的 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为准（由 078 约束）。

### 4.1 `positions`（稳定实体）
```sql
CREATE TABLE positions (
  tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code           varchar(64) NOT NULL,
  is_auto_created boolean NOT NULL DEFAULT FALSE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT positions_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT positions_tenant_id_code_key UNIQUE (tenant_id, code)
);
```

identity 合同（v1，选定以保持简单）：
- `positions` 仅承载“稳定锚点 + code 唯一性”；所有有效期字段落在 `position_versions`。
- `positions.code` 视为不可变；如需更换 code，采用“新建 Position + disable 旧 Position（versions）”，避免更新 identity 引入第二事实源。
- `positions` 行仅允许由 `submit_position_event(event_type='CREATE')` 创建；应用层禁止直写。

### 4.2 `position_events`（Write Side / SoT）
```sql
CREATE TABLE position_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,

  position_id      uuid NOT NULL,
  event_type       text NOT NULL,     -- CREATE/UPDATE/CORRECT/RESCIND/SHIFT_BOUNDARY/...
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,

  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT position_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT position_events_one_per_day_unique UNIQUE (tenant_id, position_id, effective_date),
  CONSTRAINT position_events_position_fk FOREIGN KEY (tenant_id, position_id) REFERENCES positions(tenant_id, id) ON DELETE RESTRICT
);
```

事件类型与 payload 合同（v1 最小集，选定以可实现为准）：
- `CREATE`
  - 前置：`positions(tenant_id, id)` 不存在。
  - 必填：`payload.code`（写入 identity），`payload.org_unit_id`（写入 versions）。
  - 可选：`payload.name`、`payload.reports_to_position_id`、`payload.job_profile_id`、`payload.job_level_id`、`payload.capacity_fte`、`payload.profile`、`payload.is_auto_created`。
- `UPDATE`
  - 前置：`positions(tenant_id, id)` 已存在。
  - 语义：payload 为 patch；缺省字段保持不变；显式 `null` 表示置空（仅对允许为 NULL 的字段）。
  - 允许 keys：`org_unit_id`、`name`、`reports_to_position_id`、`job_profile_id`、`job_level_id`、`capacity_fte`、`profile`、`lifecycle_status`。
  - 禁止 keys：`code`（identity 不可变）。
- `DISABLE`
  - 前置：`positions(tenant_id, id)` 已存在。
  - 语义：等价于 `UPDATE`，强制设置 `lifecycle_status='disabled'`，并要求 `reports_to_position_id IS NULL`（或由 Kernel 在投射时清空）。

payload 形状约束（必须）：
- `payload` 必须为 JSON object；未知 key 必须拒绝（稳定错误码 `POSITION_INVALID_ARGUMENT`）。
- `capacity_fte` 必须为正数（`>0`）；当 `lifecycle_status='disabled'` 时，允许保持历史值，但 **不允许** 继续被 assignments 占用（见 3.7 的裁决）。

### 4.3 `position_versions`（Read Side / Projection）
```sql
CREATE TABLE position_versions (
  id            bigserial PRIMARY KEY,
  tenant_id     uuid NOT NULL,

  position_id   uuid NOT NULL,
  org_unit_id   uuid NOT NULL,          -- 引用 OrgUnit v4 的 org_id（无 FK；由 Kernel 校验 as-of 存在性）

  reports_to_position_id uuid NULL,

  job_profile_id uuid NULL,
  job_level_id   uuid NULL,

  name          text NULL,              -- title/name 等显示字段（可选）
  lifecycle_status text NOT NULL DEFAULT 'active',
  capacity_fte  numeric(9,2) NOT NULL DEFAULT 1.0,
  profile       jsonb NOT NULL DEFAULT '{}'::jsonb,

  validity      daterange NOT NULL,
  last_event_id bigint NOT NULL REFERENCES position_events(id),

  CONSTRAINT position_versions_validity_check CHECK (NOT isempty(validity)),
  CONSTRAINT position_versions_validity_bounds_check CHECK (lower_inc(validity) AND NOT upper_inc(validity)),
  CONSTRAINT position_versions_capacity_fte_check CHECK (capacity_fte > 0),
  CONSTRAINT position_versions_profile_is_object_check CHECK (jsonb_typeof(profile) = 'object'),

  CONSTRAINT position_versions_position_fk FOREIGN KEY (tenant_id, position_id) REFERENCES positions(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT position_versions_reports_to_fk FOREIGN KEY (tenant_id, reports_to_position_id) REFERENCES positions(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT position_versions_job_profile_fk FOREIGN KEY (tenant_id, job_profile_id) REFERENCES job_profiles(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT position_versions_job_level_fk FOREIGN KEY (tenant_id, job_level_id) REFERENCES job_levels(tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE position_versions
  ADD CONSTRAINT position_versions_no_overlap
  EXCLUDE USING gist (
    tenant_id gist_uuid_ops WITH =,
    position_id gist_uuid_ops WITH =,
    validity WITH &&
  );
```

索引/查询效率评估要点（被大量引用时）：
- `position_versions_no_overlap` 会生成一份 GiST 索引（`tenant_id + position_id + validity`），对最常见的点查/关联非常友好：  
  `WHERE tenant_id=? AND position_id=? AND validity @> ?::date` → 预期为 index-driven 且至多命中 1 行（no-overlap + gapless）。
- **租户快照**（`WHERE tenant_id=? AND validity @> ?::date`）在存在大量历史 versions 时，建议补一条更匹配的索引以避免扫描大量旧切片（是否需要以 `EXPLAIN` 验证为准）：  
  ```sql
  CREATE INDEX position_versions_active_day_gist
    ON position_versions
    USING gist (tenant_id gist_uuid_ops, validity)
    WHERE lifecycle_status = 'active';
  ```
- 若查询经常按组织/汇报关系过滤（例如“某 OrgUnit 下的岗位”“某岗位的 direct reports”），建议按维度补充 GiST（同样以 `EXPLAIN` 验证为准）：  
  ```sql
  CREATE INDEX position_versions_org_unit_day_gist
    ON position_versions
    USING gist (tenant_id gist_uuid_ops, org_unit_id gist_uuid_ops, validity)
    WHERE lifecycle_status = 'active';

  CREATE INDEX position_versions_reports_to_day_gist
    ON position_versions
    USING gist (tenant_id gist_uuid_ops, reports_to_position_id gist_uuid_ops, validity)
    WHERE reports_to_position_id IS NOT NULL;
  ```

> 约束说明：
> - `org_unit_id` 由于 OrgUnit v4 没有 identity 表，无法用 FK 表达；必须由 Kernel 在写入时校验“as-of 存在且可用”。
> - `job_profile_id/job_level_id` 可做存在性 FK（指向 identity 表），但“as-of 有效”仍需 Kernel 校验。
>
> gapless 合同：
> - `position_versions` 必须无间隙（相邻切片严丝合缝）且末段到 infinity；
> - 由 `replay_position_versions` 生成并在事务内校验（避免实现期靠应用层“补洞”导致漂移）。

### 4.4 `assignments` / `assignment_events` / `assignment_versions`
> Assignment v4 作为 Position v4 的同域能力一并替换（对齐 078 的 drop 清单）。

最小形状（合同约束重点）：
- `assignment_events`：同 `position_events`，不变量为 `UNIQUE(tenant_id, assignment_id, effective_date)`。
- `assignment_versions`：使用 `daterange validity` + no-overlap + gapless，并冗余 `subject_id/position_id/assignment_type` 以落地“primary 唯一/占编校验”等不变量。
- **边界（选定，保持干净）**：`assignment_versions` **不冗余** `org_unit_id`；需要组织口径时由读接口 `get_assignment_snapshot` 在 as-of 语义下 join `position_versions` 派生（避免 Position 变更触发 Assignment 大规模重放）。
- **占编校验（必须）**：Assignment 写入必须在同一事务内锁定目标 `position_id` 的 as-of version（或等价互斥策略），并校验 `SUM(allocated_fte) <= capacity_fte`（以 Kernel 为最终裁判）。

示例 DDL（v1；字段可按产品收敛，但约束形状必须保留）：
```sql
CREATE TABLE assignments (
  tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assignments_tenant_id_id_key UNIQUE (tenant_id, id)
);

CREATE TABLE assignment_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL,
  assignment_id    uuid NOT NULL,
  event_type       text NOT NULL,
  effective_date   date NOT NULL,
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assignment_events_event_id_unique UNIQUE (event_id),
  CONSTRAINT assignment_events_one_per_day_unique UNIQUE (tenant_id, assignment_id, effective_date),
  CONSTRAINT assignment_events_assignment_fk FOREIGN KEY (tenant_id, assignment_id) REFERENCES assignments(tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE assignment_versions (
  id            bigserial PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  assignment_id uuid NOT NULL,
  subject_id    uuid NOT NULL,    -- person_id 等；不做 FK（由 Kernel 校验 as-of 可用性）
  position_id   uuid NOT NULL,
  assignment_type text NOT NULL DEFAULT 'primary',
  allocated_fte numeric(9,2) NOT NULL DEFAULT 1.0,
  status        text NOT NULL DEFAULT 'active',
  validity      daterange NOT NULL,
  last_event_id bigint NOT NULL REFERENCES assignment_events(id),
  CONSTRAINT assignment_versions_validity_check CHECK (NOT isempty(validity)),
  CONSTRAINT assignment_versions_validity_bounds_check CHECK (lower_inc(validity) AND NOT upper_inc(validity)),
  CONSTRAINT assignment_versions_allocated_fte_check CHECK (allocated_fte > 0),
  CONSTRAINT assignment_versions_assignment_fk FOREIGN KEY (tenant_id, assignment_id) REFERENCES assignments(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT assignment_versions_position_fk FOREIGN KEY (tenant_id, position_id) REFERENCES positions(tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE assignment_versions
  ADD CONSTRAINT assignment_versions_no_overlap
  EXCLUDE USING gist (
    tenant_id gist_uuid_ops WITH =,
    assignment_id gist_uuid_ops WITH =,
    validity WITH &&
  );
```

Assignment 事件类型与 payload 合同（v1 最小集）：
- `CREATE`
  - 前置：`assignments(tenant_id, id)` 不存在。
  - 必填：`payload.subject_id`、`payload.position_id`。
  - 可选：`payload.assignment_type`（默认 `primary`）、`payload.allocated_fte`（默认 `1.0`）、`payload.status`（默认 `active`）。
- `UPDATE`
  - 前置：`assignments(tenant_id, id)` 已存在。
  - 语义：payload 为 patch；允许 keys：`subject_id`、`position_id`、`assignment_type`、`allocated_fte`、`status`。
- `DISABLE`
  - 前置：`assignments(tenant_id, id)` 已存在。
  - 语义：等价于 `UPDATE` 设置 `status='inactive'`（仍保持 gapless）。

不变量（必须）：
- **占编/容量**：见 3.7；写入口在 Position/Assignment 两侧都需裁决。
- **Primary 唯一（v1，选定）**：对任意 `query_date`，同一 `subject_id` 至多存在一条 `assignment_type='primary' AND status='active'` 的 Assignment（由 Kernel 校验）。

索引建议（实现期以 `EXPLAIN` 验证）：
```sql
CREATE INDEX assignment_versions_position_day_gist
  ON assignment_versions
  USING gist (tenant_id gist_uuid_ops, position_id gist_uuid_ops, validity)
  WHERE status = 'active';

CREATE INDEX assignment_versions_subject_day_gist
  ON assignment_versions
  USING gist (tenant_id gist_uuid_ops, subject_id gist_uuid_ops, validity)
  WHERE assignment_type = 'primary' AND status = 'active';
```

## 5. Kernel 写入口（One Door）
> 选定：**同事务全量重放（delete+replay）**。写入口唯一：应用层只调用 `submit_*_event`，它在同一事务内完成：事件入库（幂等）→ 全量重放（删除并重建 versions）→ 不变量裁决（含 gapless/占编）。

### 5.1 并发互斥（Advisory Lock）
**锁粒度（选定）**：同一 `tenant_id` 的 Position+Assignment 写入串行化，简化 `reports_to_position_id` 与占编等跨实体不变量的裁决。

锁 key（文本）：`org:v4:<tenant_id>:Position`

### 5.2 `submit_position_event`（同事务全量重放）
函数签名（建议，与 077 对齐）：
```sql
CREATE OR REPLACE FUNCTION submit_position_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_position_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;
```

合同语义（必须）：
0) 多租户上下文（RLS）：函数开头必须断言 `p_tenant_id` 与 `app.current_tenant` 一致（对齐 `DEV-PLAN-081`）。
1) 获取互斥锁：`org:v4:<tenant_id>:Position`（同一事务内）。
2) 参数校验：`p_event_type` 必须为 `CREATE/UPDATE/DISABLE`；`p_payload` 必须为 object（空则视为 `{}`）。
3) identity 处理：
   - `CREATE`：从 `payload.code` 创建 `positions` 行（并可设置 `is_auto_created`）；若已存在则拒绝（`POSITION_ALREADY_EXISTS`）。
   - 非 `CREATE`：要求 `positions` 行已存在；否则拒绝（`POSITION_NOT_FOUND`）。
4) 写入 `position_events`（以 `event_id` 幂等；同一实体同日唯一由约束拒绝）。
5) 幂等复用校验：若 `event_id` 已存在但参数不同，拒绝（`POSITION_IDEMPOTENCY_REUSED`）；若完全相同则返回既有 event 行 id（不重复投射）。
6) 插入成功后调用 `replay_position_versions(p_tenant_id, p_position_id)`（同一事务内），由其负责：
   - 生成 gapless versions；
   - 裁决引用可用性、汇报线无环、以及容量不变量（3.6/3.7）。

### 5.3 `replay_position_versions`
合同语义（必须）：
- 多租户上下文（RLS）：函数开头必须断言 `p_tenant_id` 与 `app.current_tenant` 一致（对齐 `DEV-PLAN-081`）。
- `DELETE FROM position_versions WHERE tenant_id=? AND position_id=?;`
- 按 `(effective_date, id)` 顺序读取该 `position_id` 的全部事件并重建切片：
  - 每条事件产生一个新的版本窗 `[effective_date, next_effective_date)`；
  - 最后一段必须为 `[last_effective_date, infinity)`。
- 在事务内校验：
  - no-overlap（由 `position_versions_no_overlap` 强制）
  - gapless（相邻切片严丝合缝 + 末段 infinity；失败时报 `POSITION_VALIDITY_GAP` / `POSITION_VALIDITY_NOT_INFINITE`）
  - 引用校验：
    - `org_unit_id`：as-of 存在（`org_unit_versions` 有覆盖该日的版本切片）。
    - `job_profile_id/job_level_id`：identity 存在（FK）；其 `is_active` 仅作为 UI/筛选语义，不作为 Position 引用合法性的硬约束（避免 Job Catalog → Position 的反向耦合校验引入额外复杂度）。
    - `reports_to_position_id`：as-of 存在且 `lifecycle_status='active'`（并受无环不变量约束）。
  - 汇报线无环：调用 `validate_position_reporting_acyclic(p_tenant_id, p_position_id)`（见 3.6）。
  - 容量不变量：调用 `validate_position_capacity(p_tenant_id, p_position_id)`（见 3.7）。

汇报线无环校验算法（建议，保证 retro 场景正确）：
- 使用递归 CTE 沿 `reports_to_position_id` 向上追溯，并携带“时间窗交集”（`daterange`）避免误报；当发现“回到起点 position_id 且时间窗非空”即判定存在环并拒绝。示意：
```sql
WITH RECURSIVE walk AS (
  SELECT
    pv.position_id AS start_position_id,
    pv.reports_to_position_id AS current_parent_id,
    pv.validity AS window,
    ARRAY[pv.position_id] AS path
  FROM position_versions pv
  WHERE pv.tenant_id = p_tenant_id
    AND pv.position_id = p_position_id
    AND pv.lifecycle_status = 'active'
    AND pv.reports_to_position_id IS NOT NULL
  UNION ALL
  SELECT
    w.start_position_id,
    pv.reports_to_position_id,
    (w.window * pv.validity) AS window,
    (w.path || pv.position_id) AS path
  FROM walk w
  JOIN position_versions pv
    ON pv.tenant_id = p_tenant_id
   AND pv.position_id = w.current_parent_id
   AND pv.lifecycle_status = 'active'
   AND pv.validity && w.window
  WHERE NOT isempty(w.window * pv.validity)
    AND pv.reports_to_position_id IS NOT NULL
    AND NOT (pv.position_id = ANY(w.path))
)
SELECT 1
FROM walk
WHERE current_parent_id = start_position_id
LIMIT 1;
```

### 5.4 `submit_assignment_event` / `replay_assignment_versions`
函数签名（建议，与 077 对齐）：
```sql
CREATE OR REPLACE FUNCTION submit_assignment_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_assignment_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;
```

合同语义（必须）：
0) 多租户上下文（RLS）：函数开头必须断言 `p_tenant_id` 与 `app.current_tenant` 一致（对齐 `DEV-PLAN-081`）。
1) 获取互斥锁：`org:v4:<tenant_id>:Position`（同一事务内）。
2) identity 处理：
   - `CREATE`：创建 `assignments` 行；若已存在则拒绝（`ASSIGNMENT_ALREADY_EXISTS`）。
   - 非 `CREATE`：要求 `assignments` 行已存在；否则拒绝（`ASSIGNMENT_NOT_FOUND`）。
3) 写入 `assignment_events`（以 `event_id` 幂等；同一实体同日唯一由约束拒绝）。
4) 插入成功后调用 `replay_assignment_versions(p_tenant_id, p_assignment_id)`（同一事务内）并生成 gapless versions。
5) 不变量裁决（必须，Kernel）：
   - 对本次写入影响到的 `position_id` 集合，按 versions 边界日（`lower(validity)`）枚举检查点；
   - 对每个检查点 `d`：计算 `SUM(allocated_fte)`（as-of `d` 的 active assignments）与 `capacity_fte`（as-of `d` 的 position version），要求 `SUM <= capacity_fte`；
   - 失败时报稳定错误码（例如 `ASSIGNMENT_CAPACITY_EXCEEDED`）。
   - 对本次写入影响到的 `subject_id` 集合，校验 primary 唯一（若启用该不变量）。

> `replay_*` / `apply_*_logic` 属于 Kernel 内部实现细节：用于把事件投射到 versions（以及必要的关系表/校验），禁止应用角色直接执行。

## 6. 读模型封装与查询
函数签名（建议）：
```sql
CREATE OR REPLACE FUNCTION get_position_snapshot(
  p_tenant_id uuid,
  p_query_date date
) RETURNS TABLE (...);

CREATE OR REPLACE FUNCTION get_assignment_snapshot(
  p_tenant_id uuid,
  p_query_date date
) RETURNS TABLE (...);
```

语义：
- `get_position_snapshot(p_tenant_id, p_query_date)`：返回 as-of 的 Position 列表（含 `org_unit_id`、`job_profile_id` 等）。
- `get_assignment_snapshot(p_tenant_id, p_query_date)`：返回 as-of 的 Assignment 列表（用于占编/人员视图），并在 DB 内 join `position_versions` 派生 `org_unit_id` 等展示字段（不落盘到 `assignment_versions`）。

> 被大量引用时的查询形状建议：优先“先取快照再 join”，避免对 `position_versions` 做 N 次点查导致 nested-loop 放大。典型形状：  
> `WITH p AS (SELECT * FROM get_position_snapshot($tenant, $day)) SELECT ... FROM <many rows> x JOIN p ON ...;`

## 7. Go 层集成（事务 + 调用 DB）
- Go 仅负责：鉴权 →（可选 try-lock）→ 开事务 → 调 `submit_*_event` → 提交。
- 错误契约对齐 077：优先用 `SQLSTATE + constraint name` 做稳定映射；业务级拒绝使用“稳定 code（`MESSAGE`）+ `DETAIL`”的异常形状。
- 多租户隔离（RLS）相关失败路径与稳定映射对齐 `DEV-PLAN-081`（fail-closed 缺 tenant 上下文 / tenant mismatch / policy 拒绝）。

### 7.1 错误契约（DB → Go → serrors）
最小映射表（v1，示例 code；落地时以模块错误码表收敛为准）：

| 场景 | DB 侧来源 | 识别方式（建议） | Go `serrors` code |
| --- | --- | --- | --- |
| Position/Assignment 写被占用（fail-fast lock） | `pg_try_advisory_xact_lock` 返回 false | 应用层布尔结果 | `POSITION_BUSY` |
| Position/Assignment 不存在 / 已存在 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `POSITION_NOT_FOUND` / `POSITION_ALREADY_EXISTS` / `ASSIGNMENT_NOT_FOUND` / `ASSIGNMENT_ALREADY_EXISTS` |
| 参数/事件类型/payload 不合法 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `POSITION_INVALID_ARGUMENT` / `ASSIGNMENT_INVALID_ARGUMENT` |
| 幂等键复用但参数不同 | `submit_*_event` 明确拒绝 | DB exception `MESSAGE` | `POSITION_IDEMPOTENCY_REUSED` / `ASSIGNMENT_IDEMPOTENCY_REUSED` |
| 同一实体同日重复事件 | `*_events_one_per_day_unique` | `23505` + constraint name | `*_EVENT_CONFLICT_SAME_DAY` |
| 有效期重叠（破坏 no-overlap） | `*_versions_no_overlap` | `23P01` + constraint name | `*_VALIDITY_OVERLAP` |
| gapless 被破坏（出现间隙/末段非 infinity） | `replay_*_versions` 校验失败 | DB exception `MESSAGE` | `*_VALIDITY_GAP` / `*_VALIDITY_NOT_INFINITE` |
| 引用对象不存在（FK） | FK 约束失败 | `23503` + constraint name | `*_REF_NOT_FOUND` |
| 引用对象 as-of 不存在/不可用 | `replay_*_versions` 校验失败（例如 org_unit/reports_to 的 as-of 检查） | DB exception `MESSAGE` | `*_REF_NOT_FOUND_AS_OF` |
| 汇报线出现环 / 自指 | `validate_position_reporting_acyclic` 校验失败 | DB exception `MESSAGE` | `POSITION_REPORTING_CYCLE` |
| 占编超限 | `submit_assignment_event` 校验失败 | DB exception `MESSAGE` | `ASSIGNMENT_CAPACITY_EXCEEDED` |

## 8. 测试与验收标准 (Acceptance Criteria)
- [ ] RLS（对齐 081）：缺失 `app.current_tenant` 时对 v4 表的读写必须 fail-closed；tenant mismatch 必须稳定失败可映射。
- [ ] 事件幂等：同 `event_id` 重试不重复投射。
- [ ] 全量重放：每次写入都在同一事务内 delete+replay 对应 versions，且写后读强一致。
- [ ] 同日唯一：同一实体同日提交第二条事件被拒绝且可稳定映射错误码。
- [ ] versions no-overlap：任意写入不会产生重叠有效期。
- [ ] versions gapless：相邻切片无间隙且末段到 infinity（失败可稳定映射错误码）。
- [ ] 汇报线无环：任意日期 as-of 图无环；retro 插入也不能在未来时间窗制造环（失败可稳定映射错误码）。
- [ ] 占编校验：Assignment 写入与 Position 降容均不会造成“事后超编”（Kernel 裁决为准）。
- [ ] as-of 查询：任意日期快照结果与 versions 语义一致（`validity @> date`）。

## 9. 运维与灾备（Rebuild / Replay）
当投射逻辑缺陷导致 versions 错误时，可通过 replay 重建读模型（versions 可丢弃重建）：
- Position：`SELECT replay_position_versions('<tenant_id>'::uuid, '<position_id>'::uuid);`
- Assignment：`SELECT replay_assignment_versions('<tenant_id>'::uuid, '<assignment_id>'::uuid);`

> 建议在执行前复用同一把维护互斥锁（`org:v4:<tenant_id>:Position`）确保与在线写入互斥。

> 多租户隔离（RLS，见 `DEV-PLAN-081`）：replay 必须在显式事务内先注入 `app.current_tenant`，否则会 fail-closed。
