# DEV-PLAN-061：Person（人员）模块重建 + Org-Position-Person 打通 + 最小人事事件（入/转/离）详细设计

**状态**: 已完成（2025-12-23）
**对齐更新**：
- 2025-12-27：对齐 DEV-PLAN-064：Valid Time 统一按天（`YYYY-MM-DD`）闭区间语义；对外 `effective_date/end_date` 一律按 day string 表达；落库兼容期双写 legacy `timestamptz effective_date` + `date effective_on`。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - `docs/dev-plans/060-peoplesoft-corehr-menu-reference.md`（模块边界参考：3 人员、4 人事事件、5 任职记录）
  - `docs/dev-plans/020-organization-lifecycle.md`（主链：Person → Position → Org；术语：`person_uuid/pernr`）
- **当前痛点**：
  1) 现有 `modules/hrm` 的 `employees` 属于早期 CRUD：模型混入薪资字段、主键为 `serial`，与 060/020 的 `person（人员）` 口径不一致；继续叠加会持续放大命名与模型漂移。
  2) Org 的 `org_assignments.subject_id` 当前由 `pernr` 确定性派生（`modules/org/domain/subjectid`）。在我们选定“`person_uuid` 独立生成”的前提下，该派生路径必须退役。
- **业务价值**：
  - 建立 Bugs & Blossoms 的最小 Core HR 主链：人员（Person）主数据可维护、任职记录可治理、入/转/离可追溯。
  - 为后续合同/薪酬/考勤等模块提供稳定的人员主键与任职记录出口（API + outbox 事件）。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [X] **一次性切换**：移除 `modules/hrm`（employees）并新建 `modules/person`（人员），完成路由/导航/locale/Authz/sqlc/schema/migrations/CI gates 的全链路切换。
  - [X] **标识解耦**：`person_uuid`（内部不可变 UUID，独立生成）+ `pernr`（租户内唯一业务工号）；对外交互默认以 `pernr` 工作，系统内部引用一律用 `person_uuid`。
  - [X] **Org-Position-Person 打通**：Org 任职记录写入时以 `pernr -> person_uuid` 解析为准，`org_assignments.subject_id` 存储 `person_uuid`（不再由 pernr 派生）。
  - [X] **最小人事事件**：提供入/转/离（Hire/Transfer/Termination）的语义 API，落库可追溯，并通过 outbox 投递事件；底座复用 Org 的任职记录有效期治理与冲突约束。
  - [X] **任职记录可见**：Person 详情页可查看任职记录时间线（任职经历视图），并能通过 pernr 快速定位。
  - [X] **门禁通过**：本计划命中的工具链/门禁与 CI 一致，按 SSOT 执行并记录 readiness。
- **非目标 (Out of Scope)**：
  - 不在本计划内落地薪酬/福利/考勤/招聘/绩效/培训/合同等完整闭环；仅提供稳定的 Person/Assignment/Position/Org 主链与事件出口。
  - 不在本计划内实现“工号变更历史/重编号/合并”等治理能力（如需支持，另起 dev-plan 定义历史表、审计口径与回放策略）。
  - 不在本计划内引入 Workflow/BP 审批引擎；人事事件仅提供最小“可追溯 + 可投递”闭环。

## 2.1 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器”，命令细节以 `AGENTS.md`/`Makefile`/CI 为准。

- **触发器清单（本计划命中）**：
  - [X] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [X] `.templ` / Tailwind（`make generate && make css`，并确保生成物提交）
  - [X] 多语言 JSON（`make check tr`）
  - [X] Authz（`make authz-test && make authz-lint`；策略聚合用 `make authz-pack`）
  - [X] 路由治理（`make check routing`；必要时更新 `config/routing/allowlist.yaml`）
  - [X] DB 迁移 / Schema（Atlas+Goose：`atlas.hcl` + `migrations/person/**`、`migrations/org/**`）
  - [X] sqlc（`sqlc.yaml` + `modules/person/infrastructure/sqlc/**`）
  - [X] Outbox（对齐 `docs/dev-plans/017-transactional-outbox.md` 与 `docs/dev-plans/022-org-placeholders-and-event-contracts.md`）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - 工具链复用索引：`docs/dev-plans/009A-r200-tooling-playbook.md`

## 2.2 术语：任职记录 vs 任职经历（中国大陆对标建议）
> 结论（本计划采用）：**“任职记录”作为 SSOT 术语**；**“任职经历”作为 UI 视图标题**。

- **任职记录**：结构化 Job Data/任职事实，由 HR 维护（含有效期、组织/职位/用工属性等）。
- **任职经历**：履历视角展示（通常由任职记录聚合/派生而来）。

## 2.3 一致性评审：SSOT 收敛与变更影响面（必须先对齐）
> 本节用于避免“实现与契约漂移”。061 落地前，应先在对应 SSOT 文档完成收敛更新（或明确发布 v2）。

- **阻塞：`org_assignments.subject_id` 的 SSOT 冲突**
  - 现状 SSOT：`docs/dev-plans/026-org-api-authz-and-events.md` 的 §7.3 定义 `subject_id = UUID(namespace, tenant_id:subject_type:pernr)` 的确定性映射，并由代码 `modules/org/domain/subjectid` 复用。
  - 本计划决策（见 §3.2 决策 3）：改为 `subject_id = person_uuid`（独立生成），因此必须在落地前同步更新 026（以及引用 026 的相关计划/工具文档），避免出现两个 SSOT。
- **阻塞：Integration Events 的 SSOT 重复**
  - 现状 SSOT：`docs/dev-plans/022-org-placeholders-and-event-contracts.md` 是 Org integration events（Topic/字段/语义）的单一事实源。
  - 本计划若新增 Topic，必须先在 022 增补并冻结，再由实现引用；061 不单独“冻结 v1”。
- **对齐：错误码与实现口径**
  - Org 写路径的冲突/重叠错误码应对齐 `modules/org/services/pg_errors.go`（例如 `ORG_PRIMARY_CONFLICT`/`ORG_OVERLAP`），避免文档自造新 code 造成实现漂移。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  UI[UI: /person/* (templ/htmx)] --> PC[modules/person/presentation/controllers]
  PC --> PS[modules/person/services.PersonService]
  PS --> PR[modules/person/infrastructure/persistence.PersonRepository]
  PR --> DB[(PostgreSQL)]

  OUI[UI: /org/*] --> OC[modules/org/presentation/controllers]
  OC --> OS[modules/org/services.OrgService]
  OS --> OR[modules/org/infrastructure/persistence.OrgRepository]
  OR --> DB

  OS -->|resolve pernr -> person_uuid (read-only)| DB
  OS -->|Tx enqueue integration events| OUTBOX[(org_outbox)]
  OUTBOX --> RELAY[pkg/outbox relay]
  RELAY --> SUBS[Consumers]
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：一次性移除 `modules/hrm`，切到 `modules/person`**
  - 选项 A：保留 `modules/hrm`，在其内重命名 employees→persons（兼容层）。
  - 选项 B（选定）：移除 `modules/hrm`，新增 `modules/person`，一次性切换入口与门禁。
  - 理由：项目早期、无数据包袱；兼容层会把“Employee/Person”命名漂移固化为长期成本。

- **决策 2：标识模型采用 `person_uuid` + `pernr`**
  - 选项 A：`person_uuid` 由 `pernr` 派生（确定性 UUID）。
  - 选项 B（选定）：`person_uuid` 独立生成（UUID），`pernr` 为租户内唯一业务工号。
  - 理由：解耦业务编号治理与内部引用；对齐主流 HCM 的“内部不可变 ID + 业务编号”模式。

- **决策 3：Org 的 `org_assignments.subject_id` 存储 `person_uuid`**
  - 选项 A：继续 `subject_id = f(pernr)`（确定性派生）。
  - 选项 B（选定）：写入时通过 Person SOR 查询 `pernr -> person_uuid`，再写入 `subject_id=person_uuid`（落地前需先更新 026 §7.3 SSOT 并同步实现/测试/CLI，见 §2.3）。
  - 理由：与决策 2 一致；避免“派生 ID = 事实 ID”的耦合。

- **决策 4：是否移除 `org_assignments.pernr`**
  - 选项 A（选定）：保留 `org_assignments.pernr`（业务字段），读路径仍可直接按 pernr 查询；写路径以 `person_uuid` 作为约束锚点。
  - 选项 B：移除 pernr，强制 join persons 获取。
  - 理由：Org Atlas/Goose 在 CI 中以独立数据库运行，不宜引入跨模块外键/依赖；保留 pernr 可最小化读路径改动与性能风险（同时仍以 `subject_id=person_uuid` 做唯一性约束）。

- **决策 5：跨 SOR 不建 DB 级 FK**
  - 选项 A：`org_assignments.subject_id` 外键引用 `persons.person_uuid`。
  - 选项 B（选定）：不建跨模块 FK；写入时做“强校验（查 person 存在）”，并在事件/日志中保留可对账信息。
  - 理由：对齐 020 的跨 SOR 协议；同时与当前 CI 的分库 smoke（org-atlas/hrm-atlas）兼容。

## 4. 数据模型与约束 (Data Model & Constraints)
> 标准：字段类型/空值/索引/约束必须精确；SQL 为实现 SSOT。

### 4.1 Person 模块：Schema（新增）
**目标文件：**
- `modules/person/infrastructure/persistence/schema/person-schema.sql`
- `modules/person/infrastructure/atlas/core_deps.sql`
- `migrations/person/00001_person_baseline.sql`（goose）

**表：`persons`（最小可用）**
```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS persons (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    person_uuid uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pernr text NOT NULL,
    display_name text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT persons_tenant_id_person_uuid_key UNIQUE (tenant_id, person_uuid),
    CONSTRAINT persons_pernr_not_blank CHECK (btrim(pernr) <> ''),
    CONSTRAINT persons_display_name_not_blank CHECK (btrim(display_name) <> ''),
    CONSTRAINT persons_status_check CHECK (status IN ('active', 'inactive'))
);

CREATE UNIQUE INDEX IF NOT EXISTS persons_tenant_pernr_uq ON persons (tenant_id, pernr);
CREATE INDEX IF NOT EXISTS persons_tenant_name_idx ON persons (tenant_id, display_name);
```

**约束说明：**
- `pernr` 在租户内唯一；M1 不提供 pernr 修改 API（将 pernr 视为业务侧“稳定编号”）。
- `status` 仅表达人员主档是否可用（不等价于任职状态）；离职不删除 Person。

### 4.2 Org 模块：任职记录字段语义调整（无 schema 破坏性变更）
**表：`org_assignments`（已存在）**
- `subject_type`：仍固定为 `person`
- `subject_id`：语义调整为 `person_uuid`（UUID，来自 `persons.person_uuid`）
- `pernr`：保留，作为业务字段（租户内唯一），用于读路径与 UI 展示/过滤

**数据库约束保持：**
- 仍以 `subject_id` 作为任职记录唯一性锚点（primary unique in time、position unique in time）。

### 4.3 Org 模块：最小人事事件落库（新增）
**目标文件：**
- `modules/org/infrastructure/persistence/schema/org-schema.sql`（新增表定义）
- `migrations/org/20YYMMDDHHMMSS_org_personnel_events.sql`（goose；文件名按项目约定生成）

**表：`org_personnel_events`（最小闭环）**
```sql
CREATE TABLE IF NOT EXISTS org_personnel_events (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id text NOT NULL,
    initiator_id uuid NOT NULL,
    event_type text NOT NULL,
    person_uuid uuid NOT NULL,
    pernr text NOT NULL,
    effective_date timestamptz NOT NULL, -- legacy（B1 双轨；待 DEV-PLAN-064 阶段 D 清理）
    effective_on date NOT NULL,
    reason_code text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_personnel_events_request_not_blank CHECK (btrim(request_id) <> ''),
    CONSTRAINT org_personnel_events_event_type_check CHECK (event_type IN ('hire', 'transfer', 'termination')),
    CONSTRAINT org_personnel_events_pernr_not_blank CHECK (btrim(pernr) <> ''),
    CONSTRAINT org_personnel_events_reason_code_not_blank CHECK (btrim(reason_code) <> ''),
    CONSTRAINT org_personnel_events_payload_is_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT org_personnel_events_tenant_request_uq UNIQUE (tenant_id, request_id)
);

CREATE INDEX IF NOT EXISTS org_personnel_events_tenant_person_effective_idx
ON org_personnel_events (tenant_id, person_uuid, effective_date DESC); -- legacy（B1 双轨）

CREATE INDEX IF NOT EXISTS org_personnel_events_tenant_person_effective_on_idx
ON org_personnel_events (tenant_id, person_uuid, effective_on DESC);
```

### 4.4 迁移策略
- **Person（goose）**：
  - `Up`：创建 `pgcrypto`、`tenants` stub、`persons` 表与索引。
  - `Down`：可在早期允许 `DROP TABLE persons`；若未来进入生产约束，需先快照备份再执行。
- **Org（goose）**：
  - `Up`：新增 `org_personnel_events` 表与索引。
  - `Down`：同上（早期可 drop；后期按 runbook 控制）。
- **破坏性移除 HRM**：
  - 删除 `migrations/hrm/**`、`modules/hrm/**` 并更新 `atlas.hcl/sqlc.yaml/CI gates`。
  - 若数据库仍存在旧表（employees/employee_meta 等），在本计划末尾提供“清理迁移”（可选、显式执行）。

## 5. 接口契约 (API Contracts)
> 标准：URL/Method/Payload（必填/选填/类型）/错误码；UI 交互需定义 HTMX 行为。

### 5.0 JSON 错误返回规范（统一）
- `/person/api/*` 与 `/org/api/*` 错误响应统一使用 `modules/core/presentation/controllers/dtos.APIError`：
```json
{
  "code": "SOME_CODE",
  "message": "human readable message",
  "meta": { "request_id": "..." }
}
```
- `request_id`：来自 `X-Request-Id`；若客户端未提供，服务端会生成并回填到 `meta.request_id`（但此时无法保证调用方“重试幂等”）。

### 5.1 Person JSON API（内部 API：`/person/api/*`）
#### 5.1.1 `POST /person/api/persons`
- **Request**：
```json
{
  "pernr": "000123",
  "display_name": "张三"
}
```
- **Response (201)**：
```json
{
  "person_uuid": "6f7a9a2d-0f17-4b35-b2fb-2c1f3e1db0a1",
  "pernr": "000123",
  "display_name": "张三",
  "status": "active",
  "created_at": "2025-12-22T11:00:00Z",
  "updated_at": "2025-12-22T11:00:00Z"
}
```
- **Error Codes**：
  - `409 Conflict`: pernr 已存在（`PERSON_PERNR_CONFLICT`）。
  - `422 Unprocessable Entity`: 字段为空/超长（`PERSON_VALIDATION_FAILED`）。

#### 5.1.2 `GET /person/api/persons:options?q=...&limit=20`
- **用途**：Org assignment form 的 typeahead/选择器。
- **Response (200)**：
```json
{
  "items": [
    { "person_uuid": "uuid", "pernr": "000123", "display_name": "张三" }
  ]
}
```
- **规则**：
  - `q` 同时匹配 `pernr` 前缀与 `display_name` 子串（M1 可先用 ILIKE；如需性能优化另起计划引入 `pg_trgm`）。

### 5.2 Person UI（UI：`/person/*`）
#### 5.2.1 `GET /person/persons`
- HTML 列表页：支持搜索、分页（M1 可只做简单分页）。
- HTMX：搜索框 `hx-get="/person/persons/table?q=..." hx-target="#persons-table"`

#### 5.2.2 `GET /person/persons/{person_uuid}`
- 详情页：
  - 基础信息（pernr/display_name/status）
  - “任职经历（任职记录）”区块：HTMX 拉取 Org timeline
    - `hx-get="/org/assignments?effective_date=YYYY-MM-DD&pernr={pernr}"`
    - 失败：展示 Unauthorized/空态，不出现 500

### 5.3 Org：任职记录读写（语义变化：subject_id=person_uuid）
#### 5.3.1 `GET /org/assignments?effective_date=...&pernr=...`
- **行为变化**：服务端不再 `NormalizedSubjectID(tenant_id, pernr)`；改为：
  1) 查询 `persons` 获取 `person_uuid`
  2) 用 `person_uuid` 查询任职记录时间线（保持缓存 key 以 `person_uuid` 为主）
- **错误码**：
  - `404 Not Found`: pernr 不存在（`ORG_PERSON_NOT_FOUND`）

### 5.4 Org：最小人事事件（新增语义 API）
> 语义 API 仅是“更易用的外壳”，底座仍落在 Org 的 assignment/position 时间线治理；该 API 必须写入 `org_personnel_events` 并投递 outbox。

- **幂等约定（M1 选定）**：
  - 客户端必须提供 `X-Request-Id`；服务端以 `org_personnel_events (tenant_id, request_id)` 唯一约束实现“同一 request 重试不重复写入”。
  - 若命中重复 `request_id`：返回 `200 OK`，响应体为已存在的 `personnel_event_id` 与其快照（并投递 outbox 的职责由实现决定：M1 建议不重复投递）。

#### 5.4.1 `POST /org/api/personnel-events/hire`
- **Request**：
```json
{
  "pernr": "000123",
  "org_node_id": "uuid",
  "effective_date": "2025-01-01",
  "allocated_fte": 1.0,
  "reason_code": "hire"
}
```
- **Response (201)**：
```json
{
  "personnel_event_id": "uuid",
  "event_type": "hire",
  "person_uuid": "uuid",
  "pernr": "000123",
  "effective_date": "2025-01-01",
  "reason_code": "hire"
}
```
- **规则**：
  - 必须存在 `persons(pernr)`；否则 404。
  - `org_node_id` 必填；`position_id` 可选（未提供则 auto-position）。
  - 创建 primary assignment（`assignment_type=primary,is_primary=true`）。
- **Error Codes**：
  - `404 Not Found`: person/org_node 不存在（`ORG_PERSON_NOT_FOUND` / `ORG_NODE_NOT_FOUND_AT_DATE`）
  - `409 Conflict`: primary assignment 重叠（`ORG_PRIMARY_CONFLICT`）
  - `400 Bad Request`: body 校验失败（`ORG_INVALID_BODY`）

#### 5.4.2 `POST /org/api/personnel-events/transfer`
- **Request**（最小）：
```json
{
  "pernr": "000123",
  "effective_date": "2025-02-01",
  "org_node_id": "uuid",
  "allocated_fte": 1.0,
  "reason_code": "transfer"
}
```
- **Response (201)**：
```json
{
  "personnel_event_id": "uuid",
  "event_type": "transfer",
  "person_uuid": "uuid",
  "pernr": "000123",
  "effective_date": "2025-02-01",
  "reason_code": "transfer"
}
```
- **规则**：
  - 查找 effective_date 当天生效的 primary assignment；不存在则 404。
  - 以 Org 既有“新增时间片（update）”策略切片（截断旧片段并写新片段）。

#### 5.4.3 `POST /org/api/personnel-events/termination`
- **Request**（最小）：
```json
{
  "pernr": "000123",
  "effective_date": "2025-03-01",
  "reason_code": "termination"
}
```
- **Response (201)**：
```json
{
  "personnel_event_id": "uuid",
  "event_type": "termination",
  "person_uuid": "uuid",
  "pernr": "000123",
  "effective_date": "2025-03-01",
  "reason_code": "termination"
}
```
- **规则**：
  - 查找 effective_date 当天生效的 primary assignment；不存在则 404。
  - 将该 primary assignment 的 `end_date` 截断为 `effective_date - 1 day`（按天闭区间；SSOT：DEV-PLAN-064）。

### 5.5 Integration Events（Outbox）
- **复用**：任职记录变更继续投递 `org.assignment.changed.v1`（对齐 `docs/dev-plans/022-org-placeholders-and-event-contracts.md`）。
- **新增**：`org.personnel_event.changed.v1`（SSOT：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`）
  - `entity_type=org_personnel_event`
  - `change_type=personnel_event.created`
  - `new_values` 最小字段：
    - `personnel_event_id`、`event_type`、`person_uuid`、`pernr`、`effective_date`、`reason_code`

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 pernr → person_uuid 解析（Org 内部）
**函数契约：**
- 输入：`tenant_id`, `pernr`
- 输出：`person_uuid` 或 `ORG_PERSON_NOT_FOUND`

**伪代码：**
1) `pernr = trim(pernr)`；空则 422。
2) 查询：`SELECT person_uuid FROM persons WHERE tenant_id=$1 AND pernr=$2`
3) 无行：返回 404（`ORG_PERSON_NOT_FOUND`）

### 6.2 Hire（入职）算法
1) 开启事务（沿用 Org 写路径事务入口；确保同一事务内写入任职记录 + outbox + org_personnel_events）。
2) Resolve `person_uuid`（6.1）。
3) 校验 `org_node_id` 存在且在 effective_date 有效（复用 Org 既有校验）。
4) 确定 `position_id`：
   - 若请求带 `position_id`：校验存在且有效。
   - 否则：用 `autoPositionID(tenant_id, org_node_id, person_uuid)` 生成 deterministic position_id；若 position 不存在则创建 `is_auto_created=true` 的 position 与 slice。
5) 调用“创建 assignment”底座：
   - `subject_type='person'`
   - `subject_id=person_uuid`
   - `pernr=pernr`
   - `assignment_type='primary'`, `is_primary=true`
   - `allocated_fte` 缺省 1.0
   - `effective_date` 按请求；`end_date=9999-12-31`
6) 插入 `org_personnel_events`（`tenant_id + request_id` 唯一，用于幂等重试）。
7) enqueue outbox：
   - `org.assignment.changed.v1`（assignment.created）
   - `org.personnel_event.changed.v1`（personnel_event.created）
8) 提交事务。

### 6.3 Transfer（调动）算法
1) 开启事务。
2) Resolve `person_uuid`。
3) 读取当前 primary assignment（as-of effective_date）并加锁（`SELECT ... FOR UPDATE`）。
4) 校验：新旧 org_node/position/allocated_fte 至少一项变化；否则 422。
5) 按 Org “新增时间片”规则：
   - 截断当前片段：`end_date = effective_date`
   - 插入新片段：effective_date 起的新 assignment（同 assignment_type/subject_id/pernr，更新 position/org_node/fte）
6) 插入 `org_personnel_events` + outbox 同 Hire。
7) 提交事务。

### 6.4 Termination（离职）算法
1) 开启事务。
2) Resolve `person_uuid`。
3) 读取当前 primary assignment（as-of effective_date）并加锁。
4) 将该 assignment 截断：`end_date = effective_date`（若 `effective_date <= assignment.effective_date`，按 Org 既有“rescind vs update”策略处理并返回清晰错误码）。
5) 插入 `org_personnel_events` + outbox。
6) 提交事务。

## 7. 安全与鉴权 (Security & Authz)
### 7.1 Authz 对象命名（避免漂移）
- **Person（选定）**：`person.persons`
  - Actions（选定）：`list` / `view` / `create` / `update` / `delete`
  - UI/HTTP 行为映射（M1）：
    - 列表页：`list`
    - 详情页：`view`
    - 新建：`create`
    - 编辑：`update`
- **Org 人事事件（M1 选定）**：
  - `/org/api/personnel-events/*` 作为“写入任职记录的语义入口”，鉴权复用 `org.assignments:assign`（避免额外对象导致策略漂移）
  - 若后续需要单独授权“仅写事件、不允许直接改任职记录”，再新增对象 `org.personnel_events`（另起计划）。

### 7.2 策略与门禁
- 新增策略片段：
  - `config/access/policies/person/persons.csv`
    - 最小内容（M1）：`p, role:core.superadmin, person.persons, *, global, allow`
- 移除 HRM 策略片段：
  - `config/access/policies/hrm/employees.csv`、`config/access/policies/hrm/positions.csv`
- 生成/校验：
  - 改策略片段后执行 `make authz-pack`（生成聚合文件，禁止手改聚合产物）
  - 必跑：`make authz-test && make authz-lint`

### 7.3 多租户隔离
- 所有 person 查询必须带 `tenant_id`。
- 若启用 RLS：对齐 `docs/dev-plans/019A-rls-tenant-isolation.md`，在事务内注入 `app.current_tenant`；本计划默认不新增 RLS 强制门槛（避免跨模块读路径在未完全 RLS 化前引入偶发失败）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- Outbox：`docs/dev-plans/017-transactional-outbox.md`
- Org 事件契约：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`
- 路由策略：`docs/dev-plans/018-routing-strategy.md`
- 术语：`docs/dev-plans/020-organization-lifecycle.md`

### 8.2 里程碑（建议拆分 PR）
1) [X] **PR-1：Person 模块骨架 + Schema + sqlc**
   - 新增：
     - `modules/person/module.go`、`modules/person/links.go`
     - `modules/person/domain/**`、`modules/person/services/**`、`modules/person/infrastructure/**`、`modules/person/presentation/**`
     - `modules/person/infrastructure/persistence/schema/person-schema.sql`
     - `modules/person/infrastructure/atlas/core_deps.sql`
     - `migrations/person/00001_person_baseline.sql`
   - Tooling/门禁对齐（必须在同一 PR 内完成，避免 CI 失配）：
     - `atlas.hcl`：`hrm_src` → `person_src`；`env dev/test/ci` 的 `src` 与 `migration.dir` 指向 `modules/person/**` 与 `migrations/person`
     - `scripts/db/run_goose.sh`：默认 `GOOSE_MIGRATIONS_DIR` 从 `migrations/hrm` 调整为 `migrations/person`
     - `Makefile`：
       - `make db plan/lint` 使用 `modules/person/.../person-schema.sql`
       - 将 `HRM_MIGRATIONS` 更名为 `PERSON_MIGRATIONS`（避免命名漂移），并确保 `PERSON_MIGRATIONS=1 make db migrate up` 走 `migrations/person`
     - `sqlc.yaml`：移除 `modules/hrm/...` 段，新增 `modules/person/...` 段
     - 新增 `scripts/db/export_person_schema.sh`（替代 `export_hrm_schema.sh`）
     - `.github/workflows/quality-gates.yml`：
       - `paths-filter`：`hrm-sqlc/hrm-atlas` → `person-sqlc/person-atlas`（或等价替换），并更新触发路径
       - Person Atlas plan/lint + goose smoke：环境变量改为 `PERSON_MIGRATIONS=1`
2) [X] **PR-2：一次性移除 HRM（employees）**
   - 删除：`modules/hrm/**`、`migrations/hrm/**`、`config/access/policies/hrm/**`
   - 更新引用：
     - `modules/load.go`：移除 `hrm.NewModule()` 与 `hrm.NavItems`，改为 `person.NewModule()` 与 `person.NavItems`
     - 所有 `/hrm/*` 路由、quicklinks、locale/templ 引用与文案
3) [X] **PR-3：Org subject_id 解析改为 person_uuid**
   - 退役 `modules/org/domain/subjectid` 派生路径
   - Org 写入/查询通过 `persons` 查询 `pernr -> person_uuid`（read-only 强校验）
   - 更新 org 测试与种子：
     - 需要 `person:{pernr}` 的测试在 DB 中补齐 `persons` 表与对应 pernr（可直接执行 `migrations/person/00001_person_baseline.sql` 的 Up SQL）
4) [X] **PR-4：最小人事事件 API + outbox**
   - 新增 `org_personnel_events` 表与 service/controller
   - 新增 topic `org.personnel_event.changed.v1`
5) [X] **PR-5：UI 打通**
   - Person 列表/详情页 + 嵌入 Org 任职经历时间线
   - i18n、导航与 routing allowlist：
     - `config/routing/allowlist.yaml` 增加 `/person`（ui）与 `/person/api`（internal_api）
6) [X] **PR-6：Readiness 记录**
  - `docs/dev-records/DEV-PLAN-061-READINESS.md`（命令/结果/时间戳）

## 9. 测试与验收标准 (Acceptance Criteria)
- **单元测试**：
  - Person：pernr 去空格、空值校验、pernr 冲突。
  - Org：pernr→person_uuid 解析错误码；入/转/离的关键失败路径（未知 pernr、重叠、无当前任职记录）。
- **集成测试（真实 DB）**：
  - org 033/025 等涉及 `person:{pernr}` 的用例：补齐 persons 种子后通过。
  - 事件：写入任职记录/人事事件后 outbox 行存在且 payload 可反序列化。
- **Lint/架构约束**：`make check lint` 通过，无 go-cleanarch 违规。
- **门禁**：按 2.1 触发器执行并通过；尤其是 atlas plan/lint + goose smoke + sqlc 生成物无漂移（`git status --short` 为空）。
- **可用性验收**：
  - 可创建 Person 并通过 pernr 在 Org 分配任职记录；
  - Person 详情页可查看任职经历时间线；
  - 入/转/离 API 可驱动任职记录变化并产出 outbox 事件。

## 10. 运维与监控 (Ops & Monitoring)
- **Feature Flag（可选）**：
  - `PERSON_MODULE_ROLLOUT=enabled|disabled`：便于在合并大范围删除时做短期开关（默认 enabled）。
- **结构化日志**：
  - 关键字段：`request_id`、`tenant_id`、`pernr`、`person_uuid`、`event_type`、`effective_date`、`change_type`。
- **回滚方案（早期）**：
  - 代码回滚：`git revert` 对应 PR。
  - 数据回滚：可使用 `make db reset`（本地）或在 CI/测试库中直接 drop 并重建；若进入生产阶段需改为“快照备份 + 受控 down”。
