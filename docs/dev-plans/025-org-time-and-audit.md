# DEV-PLAN-025：Org 时间约束、冻结窗口与审计（Update / Correct / Rescind / ShiftBoundary）

**状态**: 已评审（2025-12-17 11:44 UTC）— 按 `docs/dev-plans/001-technical-design-template.md` 补齐可编码契约

## 0. 进度速记
- 本计划在 024 主链 CRUD 已可跑通的前提下，补齐 **有效期强校验 + 冻结窗口 + 审计落盘 + Correct/Rescind/ShiftBoundary**。
- **不在本计划**：Authz/outbox/snapshot/batch（见 [DEV-PLAN-026](026-org-api-authz-and-events.md)）、完整 UI（见 [DEV-PLAN-035](035-org-ui.md)）。

## 1. 背景与上下文 (Context)
- **需求来源**：[DEV-PLAN-020](020-organization-lifecycle.md) 步骤 5。
- **承接关系**：
  - [DEV-PLAN-021](021-org-schema-and-constraints.md)：核心表/约束（no-overlap、ltree 防环、root 规则）
  - [DEV-PLAN-022](022-org-placeholders-and-event-contracts.md)：integration events 契约（v1 字段冻结）
  - [DEV-PLAN-024](024-org-crud-mainline.md)：主链 CRUD（Update=Insert、MoveNode 子树重切片、AutoPosition）
- **目标**：把“主链能写”升级为“主链写得对、可追溯、可控回滚”。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **有效期语义强校验**：
  - [ ] 统一解析/规范化 `effective_date`（UTC，半开区间 `[start,end)`）。
  - [ ] 写入前校验：无重叠（DB 兜底）、关键时间线无空档（Service 保证）、跨表 valid-time 引用合法（Assignment 在 Position 有效窗内等）。
  - [ ] 对 OrgEdge/OrgNode 的结构写入保持“无环/无双亲/有 root edge”（DB + Service 组合）。
- [ ] **冻结窗口（可租户覆盖）**：默认“上月 + 3 天宽限期”，到期后拒绝修改更早月份的历史数据（可 per-tenant override）。
- [ ] **操作类型分流**（并写审计/事件）：
  - [ ] `Update`：Insert 语义（只提交 `effective_date`，系统计算 `end_date`）。
  - [ ] `Correct`：原位更正（不改时间字段；用于修正历史片段内容或结构）。
  - [ ] `Rescind`：撤销误创建（软撤销：节点/岗位通过 `status=rescinded`；结构/时间线相应收口；Assignment/Edge 的撤销以“截断/删除+审计”为准，详见 §6）。
  - [ ] `ShiftBoundary`：原子移动相邻片段交界线（不允许吞没/倒错）。
- [ ] **审计落盘**：引入 `org_audit_logs`，记录 `request_id/transaction_time/initiator_id/change_type/effective_window/old_values/new_values`，可按 request_id 串联。
- [ ] **事件字段齐全**：所有写操作产出的 payload 满足 022 v1 字段；投递闭环与重放由 026 负责。

### 2.2 非目标
- 不实现 workflow/草稿审批/并行版本/retro 回放。
- 不实现 outbox relay、缓存失效、snapshot/batch（026 负责）。
- 不调整 matrix/dotted 的写入策略（M1 仍默认关闭；见 024/026）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  API[Org Controllers (024/026)] --> S[Org Services (025)]
  S --> V[Validators]
  S --> R[Repositories]
  R --> DB[(org_* tables)]
  S --> A[Audit Writer]
  A --> AUD[(org_audit_logs)]
  S --> E[Event Payload Builder (022)]
  E -. outbox/relay in 026 .-> OB[(org_outbox)]
```

### 3.2 关键设计决策
1. **DB vs Service 分工（选定）**
   - DB 强约束：no-overlap（EXCLUDE）、唯一根、ltree 防环/双亲（见 021）。
   - Service 强约束：冻结窗口、无空档（仅对关键时间线）、跨表 valid-time 引用一致性、以及错误码稳定化。
2. **冻结窗口采用“按月结算 + 宽限期”的 cutoff（选定）**
   - 任何会影响 `affected_at`（见 §6.1）早于 `cutoff` 的写入都必须拒绝（enforce）或记录（shadow）。
3. **审计表为 SSOT（选定）**
   - 不在主表追加 `request_id/transaction_time` 等事务时间字段；以 `org_audit_logs` 作为事务时间线 SSOT（对齐 020 的“主表单时态”）。
4. **ShiftBoundary 的外部契约（选定）**
   - ShiftBoundary 仅改变时间边界（`effective_date/end_date`），不允许改变结构键（如 node_id/parent_id 等）。
   - integration event 的 `change_type` 不新增枚举：ShiftBoundary 统一映射为 `*.corrected`（对齐 022）。

## 4. 数据模型与约束 (Data Model & Constraints)
> 仅定义 025 新增的表；核心 org_* 表以 021 为准。

### 4.1 `org_settings`（租户级配置）
用于冻结窗口等 per-tenant override；若无记录，使用默认配置（`grace_days=3`、`mode=enforce`）。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `pk` + fk |  | FK → `tenants(id)` |
| `freeze_mode` | `text` | `not null` + check | `'enforce'` | `disabled/shadow/enforce` |
| `freeze_grace_days` | `int` | `not null` | `3` | 宽限期天数 |
| `created_at` | `timestamptz` | `not null` | `now()` |  |
| `updated_at` | `timestamptz` | `not null` | `now()` |  |

约束：
- `check (freeze_mode in ('disabled','shadow','enforce'))`
- `check (freeze_grace_days between 0 and 31)`

### 4.2 `org_audit_logs`（审计记录 SSOT）
| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` + fk |  | FK → `tenants(id)` |
| `id` | `uuid` | `pk` | `gen_random_uuid()` | 审计记录 id |
| `request_id` | `text` | `not null` |  | 与 `X-Request-ID` 对齐 |
| `transaction_time` | `timestamptz` | `not null` |  | 事务时间（M1 在事务内以 `time.Now().UTC()` 近似写入；要求同一 request 内一致） |
| `initiator_user_id` | `bigint` | `null` |  | FK → `users(id)`（脚本可为空） |
| `initiator_id` | `uuid` | `not null` |  | 事件/审计统一使用的用户 UUID（见 §7.1） |
| `change_type` | `text` | `not null` |  | 例如 `node.updated/assignment.rescinded`（对齐 022） |
| `entity_type` | `text` | `not null` + check |  | `org_node/org_edge/org_position/org_assignment` |
| `entity_id` | `uuid` | `not null` |  | 变更实体 id（口径见 §7.4） |
| `effective_date` | `timestamptz` | `not null` |  | 受影响有效期窗 start |
| `end_date` | `timestamptz` | `not null` |  | 受影响有效期窗 end |
| `old_values` | `jsonb` | `null` |  | 可省略（Create 可空） |
| `new_values` | `jsonb` | `not null` | `'{}'::jsonb` | 变更后快照（或被撤销实体快照） |
| `meta` | `jsonb` | `not null` | `'{}'::jsonb` | `{"operation":"ShiftBoundary","freeze_mode":"shadow",...}` |
| `created_at` | `timestamptz` | `not null` | `now()` |  |

约束/索引建议：
- `check (effective_date < end_date)`
- `check (entity_type in ('org_node','org_edge','org_position','org_assignment'))`
- `btree (tenant_id, transaction_time desc)`
- `btree (tenant_id, entity_type, entity_id, transaction_time desc)`
- `btree (tenant_id, request_id)`

### 4.3 迁移策略（Org migrations）
- 新增迁移：`migrations/org/00003_org_settings_and_audit.sql`（序号可按实际落地顺延，但必须位于 021 baseline 之后）。
- 本计划变更触发器：涉及 `migrations/org/**` 时，落地时需按仓库约束执行 `make db migrate up && make db seed`（见根 `AGENTS.md`）。

## 5. 接口契约 (API Contracts)
> 024 已定义 Update/Move 的最小写入口；本节补齐 025 引入的 Correct/Rescind/ShiftBoundary 契约（internal API，JSON-only）。

### 5.1 通用约定
- 时间参数：同 024（支持 `YYYY-MM-DD` 或 RFC3339；统一 UTC）。
- Authz：由 026 绑定 object/action；本计划只定义“需要 admin 的操作”：
  - `Correct/Rescind/ShiftBoundary` 统一要求 `org.* admin`（026 enforce）。
- 错误响应形状：复用 `modules/core/presentation/controllers/dtos.APIError`。

### 5.2 `POST /org/api/nodes/{id}:correct`
用于原位更正某个节点属性片段（不改时间字段）。

**Request**
```json
{
  "effective_date": "2025-02-01",
  "name": "Corrected Name",
  "i18n_names": { "en": "Corrected Name" },
  "status": "active",
  "display_order": 10,
  "manager_user_id": 123
}
```

**Rules**
- 必须提供 `effective_date`，用于定位“覆盖该时点”的 slice；该 slice 的 `effective_date/end_date` 不得被修改。
- 触发冻结窗口校验（见 §6.2）。

**Response 200**
```json
{
  "id": "uuid",
  "effective_window": { "effective_date": "2025-02-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_NOT_FOUND_AT_DATE`
- 409 `ORG_FROZEN_WINDOW`

### 5.3 `POST /org/api/nodes/{id}:rescind`
撤销误创建/误更新：从 `effective_date` 起将该节点标记为 `rescinded`（必要时同时结束边与相关派生数据）。

**Request**
```json
{ "effective_date": "2025-02-01", "reason": "created by mistake" }
```

**Rules（M1 安全口径）**
- 禁止 rescind root（422 `ORG_CANNOT_RESCIND_ROOT`）。
- 仅允许 rescind “叶子节点”（as-of `effective_date` 无子节点，且有效窗内无 Position/Assignment 依赖），否则 409 `ORG_NODE_NOT_EMPTY`。
- 若 `effective_date` 命中 slice 起点：可通过 Correct 直接把该 slice 的 `status` 改为 `rescinded`；否则按 Insert 语义插入 `status=rescinded` 新 slice 并截断旧 slice。

**Response 200**
```json
{
  "id": "uuid",
  "effective_window": { "effective_date": "2025-02-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" },
  "status": "rescinded"
}
```

**Errors**
- 422 `ORG_CANNOT_RESCIND_ROOT`
- 409 `ORG_NODE_NOT_EMPTY`
- 422 `ORG_NOT_FOUND_AT_DATE`
- 409 `ORG_FROZEN_WINDOW`

### 5.4 `POST /org/api/nodes/{id}:shift-boundary`
移动相邻节点属性片段的交界线（ShiftBoundary）。

**Request**
```json
{
  "target_effective_date": "2025-02-01",
  "new_effective_date": "2025-02-05"
}
```

**Rules**
- `target_effective_date` 必须等于目标 slice 的 `effective_date`（用于定位 `T`，见 §6.6）。
- `new_effective_date` 需满足：
  - `new_effective_date < T.end_date`（否则 422 `ORG_SHIFTBOUNDARY_INVERTED`）
  - 若存在前驱片段 `P`（满足 `P.end_date == target_effective_date`）：`new_effective_date > P.effective_date`（否则 422 `ORG_SHIFTBOUNDARY_SWALLOW`）
- 触发冻结窗口校验（见 §6.2，`affected_at=min(old_start,new_effective_date)`）。

**Response 200**
```json
{
  "id": "uuid",
  "shifted": { "target_effective_date": "2025-02-01T00:00:00Z", "new_effective_date": "2025-02-05T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_SHIFTBOUNDARY_INVERTED`
- 422 `ORG_SHIFTBOUNDARY_SWALLOW`
- 422 `ORG_NOT_FOUND_AT_DATE`
- 409 `ORG_FROZEN_WINDOW`

### 5.5 `POST /org/api/assignments/{id}:correct`
用于原位更正 assignment 片段的非时间字段（例如 pernr/subject_id/position_id 的修正）。

**Request**
```json
{
  "pernr": "000123",
  "position_id": "uuid",
  "subject_id": "uuid"
}
```

**Rules**
- 不允许修改 `effective_date/end_date`。
- `subject_id` 必须与 026 SSOT 映射一致，否则 422 `ORG_SUBJECT_MISMATCH`。

**Response 200**
```json
{
  "assignment_id": "uuid",
  "effective_window": { "effective_date": "2025-02-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_SUBJECT_MISMATCH`
- 422 `ORG_NOT_FOUND_AT_DATE`
- 409 `ORG_FROZEN_WINDOW`

### 5.6 `POST /org/api/assignments/{id}:rescind`
撤销 assignment：在 `effective_date` 处终止该 assignment 的有效窗（不插入新 assignment 片段）。

**Request**
```json
{ "effective_date": "2025-03-01", "reason": "mistake" }
```

**Rules**
- `effective_date` 必须满足 `assignment.effective_date < effective_date < assignment.end_date`，否则 422 `ORG_INVALID_RESCIND_DATE`。
- 实现为：更新该 assignment 的 `end_date = effective_date`（时间线收口），并写审计/事件 `assignment.rescinded`。

**Response 200**
```json
{
  "assignment_id": "uuid",
  "effective_window": { "effective_date": "2025-01-01T00:00:00Z", "end_date": "2025-03-01T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_INVALID_RESCIND_DATE`
- 422 `ORG_NOT_FOUND_AT_DATE`
- 409 `ORG_FROZEN_WINDOW`

### 5.7 `POST /org/api/nodes/{id}:correct-move`（结构 Correct：用于 MoveNode 边界修正）
当 `POST /org/api/nodes/{id}:move` 的 `effective_date` 等于当前 edge slice 起点时，需使用本接口（原位更正结构）。

**Request**
```json
{ "effective_date": "2025-03-01", "new_parent_id": "uuid" }
```

**Rules**
- `effective_date` 必须等于该节点在该时点的 edge slice 起点，否则 422 `ORG_USE_MOVE`。
- 该操作会改变结构（parent），必须对子树执行 edge 重切片以重算 `path/depth`（对齐 024 的 MoveNode 重切片思路）。

**Response 200**
```json
{
  "id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z"
}
```

**Errors**
- 422 `ORG_USE_MOVE`
- 422 `ORG_PARENT_NOT_FOUND`
- 422 `ORG_CANNOT_MOVE_ROOT`
- 409 `ORG_OVERLAP`
- 409 `ORG_FROZEN_WINDOW`

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 术语：`affected_at`
冻结窗口与审计都以“受影响的最早有效时间点”作为判定输入：
- Update(Insert)：`affected_at = effective_date`
- Correct：`affected_at = target_slice.effective_date`（被更正片段的起点）
- Rescind：
  - Node：`affected_at = effective_date`
  - Assignment：`affected_at = effective_date`（作为新 end_date）
- ShiftBoundary：`affected_at = min(old_start, new_effective_date)`
- Move（Insert）：`affected_at = effective_date`
- Correct-Move：`affected_at = effective_date`（结构在该起点被更正）

### 6.2 冻结窗口算法（M1，精确口径）
输入：`tenant_id`、`transaction_time=nowUTC`、`affected_at`。

1. 从 `org_settings` 读取：
   - `mode`：`disabled/shadow/enforce`
   - `grace_days`（默认 3）
2. 计算 `cutoff`（UTC 00:00）：
   - `month_start = date_trunc('month', transaction_time)`（00:00 UTC）
   - `grace_end = month_start + grace_days * 24h`
   - 若 `transaction_time < grace_end`：`cutoff = month_start - 1 month`（允许修改“上月”及之后）
   - 否则：`cutoff = month_start`（仅允许修改“本月”及之后）
3. 判定：
   - 若 `affected_at < cutoff`：
     - `mode=enforce`：拒绝 `ORG_FROZEN_WINDOW`（HTTP 409）
     - `mode=shadow`：允许写入，但在审计 `meta.freeze_violation=true` 并日志告警
     - `mode=disabled`：不检查

### 6.3 Update（Insert）算法（统一口径，对齐 025/024）
对任何“时间片表”的 Update 都遵循 Insert 语义：只接受 `effective_date=X`，系统计算 `end_date`。

1. `X = effective_date`
2. 定位当前片段 `S`：满足 `S.effective_date <= X < S.end_date` 的片段；不存在则拒绝 422 `ORG_NOT_FOUND_AT_DATE`（禁止在 gap 中 Update；需走 Create）。
3. 定位下一片段 `N`：满足 `N.effective_date > X` 的最小片段（同 key）。
4. `Y = N.effective_date`（若 N 存在）否则 `9999-12-31T00:00:00Z`
5. 同一事务内：
   - 若 `S.effective_date == X`：拒绝 422 `ORG_USE_CORRECT`（原位更正走 Correct/ShiftBoundary）
   - 将 `S.end_date` 截断为 `X`，插入新片段 `[X, Y)`
6. 并发：同 key 时间线必须 `SELECT ... FOR UPDATE`（至少锁 `S` 与 `N`），DB EXCLUDE 兜底重叠；冲突可重试（最多 1 次）。

### 6.4 Correct（原位更正）算法（M1）
目标：不改时间字段，仅修改非时间字段；必须写审计；必须经过冻结窗口。

1. 定位覆盖 `as_of` 的片段 `T` 并 `FOR UPDATE` 锁定。
2. 冻结窗口校验：`affected_at = T.effective_date`（见 §6.2）。
3. 应用 patch（拒绝任何 `effective_date/end_date` 字段）。
4. 写入并提交；生成审计/事件（见 §7.2/§7.3，`change_type=*.corrected`）。

### 6.5 Rescind（撤销）算法（M1）
#### 6.5.1 Node Rescind
1. 定位覆盖 `effective_date=X` 的 node slice `S` 并锁定。
2. 执行结构安全校验（M1 保守）：
   - as-of `X` 不得存在子节点（`org_edges` 中 `parent_node_id = node_id` 的有效 slice 存在则拒绝）。
   - as-of `X` 不得存在 `org_positions` 引用该 node。
   - `X` 之后到 `S.end_date` 不得存在 assignment 引用该 node（可通过 positions->assignments 间接查询；若实现成本高，M1 可仅拒绝“存在任意 position”）。
3. 冻结窗口校验：`affected_at = X`。
4. 变更：
   - 若 `S.effective_date == X`：Correct 将 `S.status='rescinded'`。
   - 否则：按 Insert 语义插入 `status='rescinded'` 新 slice，并截断旧 slice。
5. 结构收口（可选但推荐）：
   - 将该节点自身的 edge 在 `X` 截断（`end_date=X`），并产出 `edge.rescinded` 事件。

#### 6.5.2 Assignment Rescind
1. 读取并锁定 assignment `A`。
2. 冻结窗口校验：`affected_at = X`（X 为新 end_date）。
3. 校验：`A.effective_date < X < A.end_date`。
4. 更新 `A.end_date = X`；写审计/事件 `assignment.rescinded`。

### 6.6 ShiftBoundary（边界移动）算法（M1）
适用对象：`org_node_slices`（M1 必做）。`org_assignments/org_positions` 是否扩展由实现评审决定；`org_edges` 不在 M1 支持列表（避免与 021 的 edge 触发器“禁止改 effective_date”规则冲突）。

输入：`target_effective_date=old_start`、`new_effective_date`.

1. 锁定目标片段 `T`（`T.effective_date == old_start`）与前驱片段 `P`（满足 `P.end_date == old_start`），均 `FOR UPDATE`。
2. 基础校验：
   - `new_effective_date < T.end_date`（倒错拒绝：422 `ORG_SHIFTBOUNDARY_INVERTED`）
   - 若 `P` 存在：`new_effective_date > P.effective_date`（吞没拒绝：422 `ORG_SHIFTBOUNDARY_SWALLOW`）
3. 冻结窗口校验：`affected_at = min(old_start, new_effective_date)`
4. 写入顺序（避免触发 EXCLUDE）：
   - Move Earlier：先 `UPDATE P.end_date = new_effective_date`，再 `UPDATE T.effective_date = new_effective_date`
   - Move Later：先 `UPDATE T.effective_date = new_effective_date`，再 `UPDATE P.end_date = new_effective_date`
5. 审计：同一事务内写两条审计记录（针对 `P` 与 `T`），`request_id` 相同；integration event 统一使用 `node.corrected`（022 v1）并在 `meta.operation=ShiftBoundary` 标记。

## 7. 审计与事件 (Audit & Events)
### 7.1 request_id 与 initiator_id（统一口径）
- `request_id`：
  - HTTP 请求：优先使用 `REQUEST_ID_HEADER`（默认 `X-Request-ID`）；缺失则生成 UUID 字符串。
  - 脚本/批处理：调用方必须显式提供 request_id（或生成 UUID）。
- `transaction_time`：
  - 在一次写事务内应保持一致（M1：以 `time.Now().UTC()` 近似并复用到审计与事件 payload）。
- `initiator_id`：
  - UI/用户：使用 `modules/core/authzutil.NormalizedUserUUID(tenantID, user)` 映射得到稳定 UUID（对齐 024/026）。
  - 系统任务：使用固定 system subject（由 026 定义与授权）。

### 7.2 `change_type` 映射（对齐 022 v1）
- Update(Insert)：`node.updated` / `assignment.updated`
- Correct：`node.corrected` / `assignment.corrected`
- Rescind：`node.rescinded` / `assignment.rescinded`（如边被截断则同时 `edge.rescinded`）
- ShiftBoundary：对外 `node.corrected`（不新增枚举），在审计 `meta.operation=ShiftBoundary`

### 7.3 “仅成功提交后产出”原则
- 审计与事件 payload 必须在同一事务完成写入（审计表在事务内插入；event payload 可先构造，commit 后返回/写 outbox 由 026 接入）。
- 失败回滚不得写审计/不得产出 payload。

### 7.4 `entity_id` 口径（避免漂移）
- `entity_type=org_node`：`entity_id = org_node_id`（稳定 id）。
- `entity_type=org_position`：`entity_id = position_id`（稳定 id）。
- `entity_type=org_assignment`：`entity_id = assignment_id`（稳定 id）。
- `entity_type=org_edge`：`entity_id = org_edges.id`（edge slice id；`new_values` 同时包含 `parent_node_id/child_node_id`，对齐 022）。

## 8. 安全与鉴权 (Security & Authz)
- 025 定义的高权限操作：
  - `Correct/Rescind/ShiftBoundary/correct-move`：要求 `org.* admin`（026 enforce）。
- 冻结窗口可在 `shadow` 模式观察，不应通过“绕过校验”实现灰度（避免静默写脏数据）。
- RLS：所有访问启用 RLS 的 org 表必须在事务内注入 `app.current_tenant`（复用 `composables.ApplyTenantRLS`）。

## 9. 依赖与里程碑 (Dependencies & Milestones)
### 9.1 依赖
- 021：Org 表与触发器（no-overlap、ltree、root 规则）
- 022：事件契约（v1 字段与 change_type 枚举）
- 024：主链 CRUD（Update=Insert/MoveNode）
- 026：Authz/outbox（本计划只产出 payload，不投递）

### 9.2 里程碑（按提交时间填充）
1. [ ] 落地 `org_settings` 与 `org_audit_logs` 迁移
2. [ ] 冻结窗口实现（enforce/shadow/disabled）+ 测试
3. [ ] Update/Correct/Rescind/ShiftBoundary 服务实现 + 测试
4. [ ] 审计写入与事件 payload 生成 + 测试
5. [ ] Readiness 记录

## 10. 测试与验收标准 (Acceptance Criteria)
- **冻结窗口**：
  - grace 期内允许修改上月数据；过期拒绝；shadow 模式不拒绝但必须记录 `freeze_violation=true`。
- **Insert/Correct/ShiftBoundary**：
  - Insert 不允许显式 `end_date` 且不覆盖未来排程（end_date 自动衔接到下一片段）。
  - Correct 不修改时间字段；ShiftBoundary 保持 `P.end_date == T.effective_date` 且无 overlap/gap。
- **审计**：
  - 每次写入至少 1 条 `org_audit_logs`；ShiftBoundary 写 2 条且 `request_id` 相同。
  - 审计 `new_values` 满足 022 v1 的字段要求（按 entity_type 分支），并包含 `effective_date/end_date` 与操作 meta。
- **性能**：
  - 1k 节点数据集下：树查询 P99 < 200ms（复用 020 基线）；校验额外查询不应引入数量级退化（以基准记录为准）。

## 验证记录
- 落地后在 `docs/dev-records/DEV-PLAN-025-READINESS.md` 记录：
  - `go fmt ./... && go vet ./... && make check lint && make test`
  - 如涉及迁移：`make db migrate up && make db seed` 的输出摘要
  - 关键用例（冻结拒绝/Correct/ShiftBoundary/Rescind）执行日志与 request_id

## 风险与回滚/降级路径
- 冻结窗口误伤：先以 `shadow` 模式观察，再切 `enforce`；出现误伤可临时切 `disabled` 并保留审计告警。
- 校验性能退化：优先通过索引与查询改写解决；必要时对“无空档”校验提供开关降级（仅保留 DB no-overlap 兜底）。
- 回滚：代码回滚 +（演练环境）可使用 023 manifest 回滚；生产回滚需在 026/outbox 就绪后定义“事件与数据一致性”的回滚流程。

## 交付物
- 冻结窗口 + Correct/Rescind/ShiftBoundary 的服务实现与测试
- `org_settings`/`org_audit_logs` 迁移与 schema 约束
- readiness 记录：`docs/dev-records/DEV-PLAN-025-READINESS.md`
