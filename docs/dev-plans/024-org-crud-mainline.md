# DEV-PLAN-024：Org 主链 CRUD（Person → Position → Org）

**状态**: 已完成（已合并至 main；Readiness：`docs/dev-records/DEV-PLAN-024-READINESS.md`）
**对齐更新**：
- 2025-12-27：对齐 DEV-PLAN-064：Valid Time（`effective_date/end_date`）按天（`YYYY-MM-DD`）闭区间语义；示例与算法不再使用 RFC3339 timestamp 表达生效日。

## 0. 进度速记
- 本计划只交付 **主链 CRUD + 自动创建空壳 Position** 的最小可用路径（可写可查），用于支撑后续 025/026/035 的落地与联调。
- **不在本计划**：时间/审计/冻结窗口/Correct/Rescind/ShiftBoundary（见 [DEV-PLAN-025](025-org-time-and-audit.md)）、Authz/outbox/snapshot/batch（见 [DEV-PLAN-026](026-org-api-authz-and-events.md)）、完整 UI（见 [DEV-PLAN-035](035-org-ui.md)）。
- 关键难点（需在 024 明确算法）：**MoveNode 的子树级联 edge 重切片**，保证 `org_edges.path/depth` 始终正确（对齐 [DEV-PLAN-021](021-org-schema-and-constraints.md) §6.2）。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` 的步骤 4（主链 CRUD）。
- **当前痛点**：
  - 仅有 021 的 schema/约束与 022 的事件契约，缺少“可执行的写路径”，无法验证有效期语义、ltree path 触发器、以及 Person→Position→Org 的业务主链。
  - 023 提供导入/回滚工具，但没有稳定 CRUD 主链，导入数据无法被业务端读写验证。
- **业务价值**：
  - 提供最短闭环：可创建/查询组织树、可给人员分配组织（经由 Position），让 025/026 能在真实写链路上加固（冻结窗口/审计/outbox/鉴权）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [x] 新增 `modules/org`（DDD 分层）并完成模块注册（`module.go/links.go`），通过 cleanarchguard。
- [x] 主链写入能力（M1）：
  - [x] Node：Create + Update(Insert)（仅属性时间片）+ MoveNode（边时间片 + 子树级联重切片）。
  - [x] Assignment：Create + Update(Insert)，并支持“未提供 `position_id` 时自动创建空壳 Position”。
- [x] 主链读取能力（M1）：树概览（as-of）与按人员的 assignment 时间线查询。
- [x] 写入成功后生成 022 的 integration event payload（**不投递**；outbox/relay 在 026 统一落地）。
- [x] 全链路 Session + tenant 强校验；兼容 `RLS_ENFORCE=enforce`（事务内注入 `app.current_tenant`，对齐 019A）。
- [x] Readiness：至少通过 `go fmt ./... && go vet ./... && make check lint && make test`，并记录到 `docs/dev-records/DEV-PLAN-024-READINESS.md`。

### 2.2 非目标（本计划明确不做）
- 不交付 Correct/Rescind/ShiftBoundary/冻结窗口与审计（025 负责；024 仅保留入口与错误码占位）。
- 不接入 `pkg/authz`、不产出策略片段、不落地 outbox/relay/对账与缓存失效（026 负责）。
- 不开放 matrix/dotted 写入（M1 默认拒绝；未来开关与权限在 026/后续计划定义）。
- 不交付完整 Org UI（035 负责）；024 仅提供最小页面用于本地验证。

### 2.3 与子计划边界（必须保持清晰）
- 021：schema/约束/触发器 SSOT；024 不修改约束定义，仅按约束实现 CRUD 写路径。
- 022：事件契约 SSOT；024 仅“生成 payload”，不负责 outbox/投递闭环。
- 023：导入/回滚工具；024 不做批量导入。
- 025：时间/审计/冻结窗口与强校验；024 的 Insert 写语义必须与 025 一致（允许先不实现冻结/审计，但不能背离语义）。
- 026：Authz/outbox/snapshot/batch；024 的 `/org/api/*` 形态与错误结构需为 026 预留稳定落点。
- 035：完整 UI/HTMX 交互；024 只做最小 UI 验证页。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  UI[HTMX/UI (024 minimal)] --> C[Org Controllers]
  API[JSON /org/api/* (024 subset)] --> C
  C --> S[Org Services]
  S --> R[Org Repositories]
  R --> DB[(Postgres org_* tables)]
  S --> EP[Event Payload Builder (022)]
  EP -. outbox/relay in 026 .-> OB[(org_outbox)]
```

### 3.2 关键设计决策（ADR 摘要）
1. **时间语义与写入模型（选定）**
   - Update 一律采用 **Insert 语义**（新增时间片 + 截断旧片段），对齐 025 的算法；`PATCH` 请求**禁止显式提交 `end_date`**。
   - 需要“原位修改”或“边界移动”的场景一律走 025（Correct/ShiftBoundary），024 仅占位。
2. **MoveNode 必须对子树重切片（选定）**
   - 由于 `org_edges.path/depth` 为 materialized path 且由触发器在插入时计算，MoveNode 若只重写目标节点的 edge，会导致子节点的 `path` 失真。
   - 因此 MoveNode 在 `effective_date=X` 需要对**整个子树**执行 edge “重切片”（同 parent/child，但让 trigger 重新计算 path）。
3. **`subject_id` 映射 SSOT（选定）**
   - `org_assignments.subject_id` 的生成/校验必须复用 [DEV-PLAN-026](026-org-api-authz-and-events.md) 的 SSOT；024 的写路径不得自定义另一套算法。
4. **最小 API 先稳定契约（选定）**
   - 024 实现 `/org/api/*` 的最小子集；026 在此基础上加 Authz/outbox，不应重写请求/响应形态。

## 4. 数据模型与约束 (Data Model & Constraints)
> Schema/约束的 SSOT 为 [DEV-PLAN-021](021-org-schema-and-constraints.md)。本节仅列出 024 依赖的关键不变式。

- **租户隔离**：所有写/读必须携带 `tenant_id` 过滤；启用 RLS 时在事务内注入 `app.current_tenant`（见 019A）。
- **root 规则**：
  - 单租户唯一 root：`org_nodes.is_root=true` 的节点仅能存在一个（DB partial unique）。
  - root 必须有对应的 root edge（`org_edges.parent_node_id IS NULL` 且 `child_node_id=root_id`），以保证树查询与后续 path 计算。
- **`parent_hint` 一致性**：`org_node_slices.parent_hint` 必须与同一 `effective_date` 的 as-of edge parent 一致（Service 校验并在 MoveNode 时更新）。
- **edge/path 规则**：`org_edges.path/depth` 由触发器维护；插入顺序必须 “parent before child”（同一时点按 depth 升序）。
- **主链唯一性**：
  - `org_nodes.code`：租户内唯一（DB unique）。
  - `org_assignments`：同主体同窗仅一个 `primary`（DB exclude where primary）。

## 5. 接口契约 (API Contracts)
> 本节定义 024 需要实现的最小 API 合同（JSON-only，internal API）；403/Authz 行为在 026 加固，但本节的 payload/错误码需保持稳定。

### 5.1 通用约定
- 路由前缀：`/org/api/*`（对齐 018 路由策略）。
- 时间参数：
  - `effective_date`：Valid Time，一律使用 `YYYY-MM-DD`（兼容期允许 RFC3339，但会归一化为 UTC date 并回显为 `YYYY-MM-DD`；SSOT：DEV-PLAN-064）。
  - 缺省 `effective_date`：服务端使用 `todayUTC`（UTC date）。
- 错误响应（JSON）统一形状（复用 `modules/core/presentation/controllers/dtos.APIError`）：
  ```json
  { "code": "ORG_INVALID_BODY", "message": "…", "meta": { "request_id": "…" } }
  ```
- 本计划不实现 Authz：在 026 前，API 仅依赖 Session+tenant；Authz 拒绝（403）与 forbidden payload 由 026 统一落地。

### 5.2 `GET /org/api/hierarchies`
**Query**
- `type`：必填，M1 仅允许 `OrgUnit`
- `effective_date`：可选

**Response 200**
```json
{
  "tenant_id": "uuid",
  "hierarchy_type": "OrgUnit",
  "effective_date": "2025-01-01",
  "nodes": [
    {
      "id": "uuid",
      "code": "PA0001-ORGEH",
      "name": "Root",
      "parent_id": null,
      "depth": 0,
      "display_order": 0,
      "status": "active"
    }
  ]
}
```

**Errors**
- 400 `ORG_INVALID_QUERY`：`type/effective_date` 非法
- 401 `ORG_NO_SESSION`：无 Session
- 400 `ORG_NO_TENANT`：无 tenant

### 5.3 `POST /org/api/nodes`
**Request**
```json
{
  "code": "D001",
  "name": "Engineering",
  "parent_id": "uuid|null",
  "effective_date": "2025-01-01",
  "i18n_names": { "en": "Engineering", "zh": "工程" },
  "status": "active",
  "display_order": 0,
  "legal_entity_id": "uuid|null",
  "company_code": "text|null",
  "location_id": "uuid|null",
  "manager_user_id": 123,
  "manager_email": "manager@example.com"
}
```

**Rules**
- `code/name/effective_date` 必填。
- `parent_id=null` 表示创建 root；若租户已存在 root 则拒绝。
- `manager_user_id` 与 `manager_email` 同时存在时，以 `manager_user_id` 为准；若只提供 `manager_email`，需能解析到 `users.id`（找不到则 422）。

**Response 201**
```json
{
  "id": "uuid",
  "code": "D001",
  "effective_window": { "effective_date": "2025-01-01", "end_date": "9999-12-31" }
}
```

**Errors**
- 409 `ORG_CODE_CONFLICT`
- 422 `ORG_PARENT_NOT_FOUND`
- 422 `ORG_MANAGER_NOT_FOUND`
- 409 `ORG_OVERLAP`：违反 DB exclude（重叠/双亲等），或触发器拒绝（成环）

### 5.4 `PATCH /org/api/nodes/{id}`（Update = Insert 时间片）
**Request**
```json
{
  "effective_date": "2025-02-01",
  "name": "Engineering (Renamed)",
  "i18n_names": { "en": "Engineering" },
  "status": "active",
  "display_order": 10,
  "manager_user_id": 321,
  "legal_entity_id": "uuid|null",
  "company_code": "text|null",
  "location_id": "uuid|null"
}
```

**Rules**
- 必须提供 `effective_date`；禁止提交 `end_date`。
- **不允许**在 024 通过本接口修改 `code`、或在 `effective_date` 处做原位修正；若 `effective_date` 等于当前片段 `effective_date`，返回 422 `ORG_USE_CORRECT`（由 025 提供 correct/shiftboundary）。

**Response 200**
- 返回 `effective_window`（本次新增片段的 `[effective_date, end_date]`（按天闭区间））。

### 5.5 `POST /org/api/nodes/{id}:move`
**Request**
```json
{
  "effective_date": "2025-03-01",
  "new_parent_id": "uuid"
}
```

**Rules**
- 不允许移动 root（422 `ORG_CANNOT_MOVE_ROOT`）。
- 需要执行子树 edge 重切片（见 §6.4）。
- `effective_date` 等于当前 edge slice 起点时不支持 Insert Move：返回 422 `ORG_USE_CORRECT_MOVE`（使用 025 的 `POST /org/api/nodes/{id}:correct-move`）。

**Response 200**
- 返回移动后该节点在 `effective_date` 的 `effective_window`。

### 5.6 `GET /org/api/assignments`
**Query**
- `subject`：必填，格式 `person:{pernr}`
- `effective_date`：可选（若提供，仅返回 as-of 视图；不提供则返回全量时间线）

**Response 200**
```json
{
  "tenant_id": "uuid",
  "subject": "person:000123",
  "assignments": [
    {
      "id": "uuid",
      "position_id": "uuid",
      "org_node_id": "uuid",
      "assignment_type": "primary",
      "effective_date": "2025-01-01",
      "end_date": "9999-12-31"
    }
  ]
}
```

### 5.7 `POST /org/api/assignments`
**Request**
```json
{
  "pernr": "000123",
  "effective_date": "2025-01-01",
  "reason_code": "assign",
  "assignment_type": "primary",
  "position_id": "uuid|null",
  "org_node_id": "uuid|null",
  "subject_id": "uuid|null"
}
```

**Rules**
- `pernr/effective_date/reason_code` 必填。
- `assignment_type` 缺省 `primary`；M1 仅允许写入 `primary`（否则 422 `ORG_ASSIGNMENT_TYPE_DISABLED`）。
- 必须满足二选一：
  - 显式提供 `position_id`
  - 或提供 `org_node_id` 以触发自动创建空壳 Position（见 §6.5）
- `subject_id` 若提供则必须与 026 SSOT 映射一致，否则 422 `ORG_SUBJECT_MISMATCH`。
- `reason_code` 用于审计治理：写入 `org_audit_logs.meta.reason_code`（对齐 052）；兼容期允许后端填充 `legacy`，进入强制后缺失则 400 `ORG_INVALID_BODY`。

**Response 201**
```json
{
  "assignment_id": "uuid",
  "position_id": "uuid",
  "subject_id": "uuid",
  "effective_window": { "effective_date": "2025-01-01", "end_date": "9999-12-31" }
}
```

**Errors**
- 400 `ORG_INVALID_BODY`：缺少 `effective_date/reason_code` 等必填
- 409 `ORG_PRIMARY_CONFLICT`：违反 primary 唯一（DB exclude）
- 422 `ORG_NODE_NOT_FOUND_AT_DATE`：`org_node_id` 在 `effective_date` 不存在（自动创建路径）
- 422 `ORG_POSITION_NOT_FOUND_AT_DATE`：`position_id` 在 `effective_date` 不存在

### 5.8 `PATCH /org/api/assignments/{id}`（Update = Insert 时间片）
**Request**
```json
{
  "effective_date": "2025-02-01",
  "reason_code": "transfer",
  "position_id": "uuid|null",
  "org_node_id": "uuid|null"
}
```

**Rules**
- 必须提供 `effective_date` 与 `reason_code`；禁止提交 `end_date`。
- 允许“改岗位/改组织归属”通过 Insert 新片段实现；若未提供 `position_id`，必须提供 `org_node_id` 以自动创建新的空壳 Position。
- `effective_date` 等于当前片段起点时返回 422 `ORG_USE_CORRECT`（由 025 提供）。

**Response 200**
```json
{
  "assignment_id": "uuid",
  "position_id": "uuid",
  "effective_window": { "effective_date": "2025-02-01", "end_date": "9999-12-31" }
}
```

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 通用：Insert 写语义（对齐 025）
- 024 的 Update（Insert）必须遵守 025 的“截断旧片段 + 插入新片段”语义；本计划可暂不实现冻结窗口/审计，但不得允许“显式 end_date 写入”或“覆盖未来排程”。

### 6.2 CreateNode（root/child）
1. 开启事务；`ctx = composables.WithTenantID(ctx, tenantID)`；调用 `composables.ApplyTenantRLS(ctx, tx)`。
2. 若 `parent_id is null`：
   - 断言租户不存在 root（`org_nodes where tenant_id and is_root`）。
   - 插入 `org_nodes(is_root=true,type='OrgUnit',code=...)`。
   - 插入首个 `org_node_slices(parent_hint=null,...)`。
   - 插入 root edge：`org_edges(parent_node_id=null, child_node_id=root_id, effective_date=..., end_date=9999...)`（触发器写 `path/depth`）。
3. 若 `parent_id not null`：
   - 校验 `parent_id` 在 `effective_date` 的 as-of 视图存在（通过查 `org_edges` 或 `org_nodes + org_edges`）。
   - 插入 `org_nodes(is_root=false,...)`。
   - 插入 `org_node_slices(parent_hint=parent_id,...)`。
   - 插入 edge：`org_edges(parent_node_id=parent_id, child_node_id=new_id, ...)`（触发器拒绝成环并计算 `path/depth`）。

### 6.3 UpdateNode（Insert 时间片）
- 输入：`org_node_id` + `effective_date=X` + 若干可变属性字段。
- 规则：若当前 slice `S.effective_date == X`，拒绝（要求走 025 Correct/ShiftBoundary）。
- 在同一事务内：
  1. 锁定并读取覆盖 `X` 的当前 slice `S`（`FOR UPDATE`）。
  2. 计算新片段结束 `Y`（复用 025 Insert 规则：下一片段 `effective_date - 1 day`；若不存在则 `9999-12-31`）。
  3. 将 `S.end_date` 截断为 `X - 1 day`，插入新 slice `[X,Y]`（含更新后的属性字段；未提供的字段从 `S` 继承）。

### 6.4 MoveNode（Insert edge + 子树重切片）
- 输入：`org_node_id` + `new_parent_id` + `effective_date=X`。
- 规则：
  - `org_node_id` 不能是 root。
  - 若当前 edge slice `E.effective_date == X`，拒绝（要求走 025 `POST /org/api/nodes/{id}:correct-move` 原位更正结构）。
  - `new_parent_id` 在 `X` 必须存在；触发器会兜底拒绝“把节点挂到自己子孙下”的成环。

**算法（同一事务内）**：
1. 读取并锁定在 `X` 的 as-of subtree：
   - 读出 `moved_path`：`SELECT path FROM org_edges WHERE tenant_id=? AND child_node_id=? AND effective_date<=X AND X<=end_date FOR UPDATE`
   - 列出 subtree edges：`SELECT parent_node_id, child_node_id, depth, end_date FROM org_edges WHERE tenant_id=? AND effective_date<=X AND X<=end_date AND path <@ moved_path ORDER BY depth ASC FOR UPDATE`
2. 先处理被移动节点自身：
   - 将其当前 edge `E` 的 `end_date` 更新为 `X`（`E.effective_date < X` 前置已保证）。
   - 插入新 edge：`(parent_node_id=new_parent_id, child_node_id=org_node_id, effective_date=X, end_date=E.end_date_original)`。
   - 同时对该节点插入新的 `org_node_slices` 片段以更新 `parent_hint=new_parent_id`（按 §6.3 Insert）。
3. 再处理子树其它节点（排除 moved node）：
   - 按 `depth ASC` 遍历每条 edge（保证 parent 在前）：
     - 将该节点当前 edge 的 `end_date` 更新为 `X`。
     - 插入新 edge：`(parent_node_id=old_parent_node_id, child_node_id=child, effective_date=X, end_date=old_end_date)`。
   - 目的：不改变父子关系，只让 trigger 重新计算 `path/depth`，使其继承新的祖先路径。

### 6.5 CreateAssignment（含自动空壳 Position）
- 输入：`pernr`、`effective_date`、(`position_id` 或 `org_node_id`)、可选 `subject_id`。
- 步骤（同一事务内）：
  1. 计算/校验 `subject_id`（SSOT：026）。
  2. 若提供 `position_id`：校验 position 在 `effective_date` 存在且 `tenant_id` 匹配。
  3. 否则（自动创建）：
     - 若 `ENABLE_ORG_AUTO_POSITIONS=false`：拒绝并返回 422 `ORG_AUTO_POSITION_DISABLED`。
     - 校验 `org_node_id` 在 `effective_date` 存在。
     - 生成 deterministic Position：
       - 约定常量：`auto_position_namespace = uuid.MustParse("2ee72897-775c-49eb-94a2-1d6b9e157701")`
       - `payload = fmt.Sprintf("%s:%s:%s:%s", tenant_id, org_node_id, "person", subject_id)`
       - `auto_position_uuid = uuid.NewSHA1(auto_position_namespace, []byte(payload))`
       - `code = "AUTO-" + strings.ToUpper(strings.ReplaceAll(auto_position_uuid.String(), "-", "")[:16])`（长度 21，字符集 `[0-9A-F-]`）
       - `is_auto_created=true`，`org_node_id=...`，`effective_date=...`，`end_date=9999...`
     - 通过 `org_positions` 的 (tenant_id, code, window) exclude + 事务重试/读回实现并发去重。
  4. 插入 `org_assignments`（M1 固定 `assignment_type=primary,is_primary=true`），DB exclude 兜底 primary 唯一与时间重叠。

### 6.6 事件 payload 生成（对齐 022；不投递）
- 024 的职责：在“事务成功提交”后，为每次写入生成 022 定义的 v1 payload（用于后续 outbox enqueue/测试/日志），失败回滚不得生成。
- 字段口径（v1）：
  - `event_id`：每条 payload 生成新 UUID
  - `event_version=1`
  - `request_id`：优先取 `REQUEST_ID_HEADER`（默认 `X-Request-ID`）请求头；若缺失则生成 UUID 字符串
  - `tenant_id`：当前租户
  - `transaction_time`：`time.Now().UTC()`（024 先以提交后的 wall-clock 近似；026/outbox 落地后以 outbox 写入时间为准）
  - `initiator_id`：使用 `modules/core/authzutil.NormalizedUserUUID(tenantID, user)`（把 `users.id` 映射为稳定 UUID）
  - `entity_version=0`（M1 固定）
  - `effective_window`：本次变更影响的 `[effective_date,end_date]`（按天闭区间，与写入的时间片一致）
- change_type 与产出策略（M1）：
  - Node Create：产出 `node.created`（entity_type=`org_node`, entity_id=`org_nodes.id`）以及 `edge.created`（entity_type=`org_edge`, entity_id=`org_edges.id`）
  - Node Update(Insert)：产出 `node.updated`
  - MoveNode：只产出 **一条** `edge.updated`（针对被移动节点在 `effective_date` 的新 edge slice）；子树重切片不逐条产出事件，消费侧按 tenant 粗粒度失效并可通过 `/org/api/snapshot`（026）纠偏
  - Assignment Create/Update：分别产出 `assignment.created` / `assignment.updated`（entity_type=`org_assignment`, entity_id=`org_assignments.id`）

## 7. 安全与鉴权 (Security & Authz)
- **Session+tenant**：024 的所有写/读入口必须要求 Session 与 tenant 上下文；缺失时返回稳定错误（401/400）。
- **RLS 兼容**：写路径必须在事务内调用 `composables.ApplyTenantRLS`；当 `RLS_ENFORCE=enforce` 时，缺 tenant 必须 fail-fast（对齐 019A）。
- **Authz**：026 负责为 `/org/api/*` 加 Casbin 鉴权与统一 403 forbidden payload；024 不自行发明另一套 403 结构。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- [DEV-PLAN-021](021-org-schema-and-constraints.md)：表/约束/ltree trigger（尤其 root edge 与 MoveNode 子树重切片的约束基础）。
- [DEV-PLAN-022](022-org-placeholders-and-event-contracts.md)：事件 payload 字段口径（024 仅生成 payload）。
- [DEV-PLAN-025](025-org-time-and-audit.md)：Insert 算法/冻结窗口/Correct/ShiftBoundary（024 必须预留并对齐）。
- [DEV-PLAN-026](026-org-api-authz-and-events.md)：`subject_id` 映射 SSOT、Authz/outbox/snapshot/batch。

### 8.2 里程碑（按提交时间填充）
1. [x] 模块骨架 + 注册（module/links/router allowlist）
2. [x] Repo 层（as-of 读 + 最小写入）
3. [x] Service 层：Create/Insert/MoveNode/Assignment+AutoPosition
4. [x] Controller：`/org/api/*` 最小子集 + 最小验证页
5. [x] 测试与 readiness 记录

## 9. 测试与验收标准 (Acceptance Criteria)
- **功能验收（必须）**：
  - Node Create：root/child 可创建，root edge 存在；树查询可返回正确 depth。
  - MoveNode：移动后子树所有节点的 `path/depth` 在 `effective_date` 处正确（至少覆盖 3 层子树）。
  - Assignment Create：不传 `position_id` 时自动创建 Position 且并发下不重复；primary 唯一冲突返回稳定错误。
  - 事件 payload：上述写入路径生成的 payload 满足 022 的 v1 字段要求，且仅在事务成功后生成。
- **测试**：
  - `go test ./modules/org/...`：覆盖 Insert 写语义、MoveNode 子树重切片、AutoPosition 并发去重、事件 payload 构造。
  - controller 测试覆盖：无 Session/无 tenant/参数非法/租户隔离。
- **Readiness（按仓库门禁）**：在 `docs/dev-records/DEV-PLAN-024-READINESS.md` 记录：
  - [x] `go fmt ./...`
  - [x] `go vet ./...`
  - [x] `make check lint`
  - [x] `make test`
  - [x] 如涉及 `.templ`/Tailwind：`make generate && make css` 且 `git status --short` 干净
  - [x] 如涉及路由 allowlist：`make check routing`

## 10. 运维、回滚与降级 (Ops / Rollback)
- **降级开关**：
  - `ENABLE_ORG_AUTO_POSITIONS=true|false`（默认 `true`）：关闭后不再允许通过 `org_node_id` 自动创建空壳 Position（要求显式 `position_id`）。
  - `ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES=true|false`（默认 `false`）：关闭时拒绝写入 `matrix/dotted`（M1 默认）。
- **回滚**：
  - 业务数据回滚：种子/演练环境优先使用 [DEV-PLAN-023](023-org-import-rollback-and-readiness.md) 的 manifest 回滚；生产回滚策略在 026（outbox/审计）就绪后再定义。
  - 代码回滚：回滚到上一个稳定版本（遵循常规 git revert）。

## 交付物
- `modules/org` 主链 CRUD（domain/service/repo/controller）与测试。
- `/org/api/*` 最小子集（JSON-only）与最小验证页。
- Readiness 记录：`docs/dev-records/DEV-PLAN-024-READINESS.md`。
