# DEV-PLAN-070：Org 组织架构页增加“修改记录 / 删除记录”（Correct / DeleteSliceAndStitch）详细设计

**状态**: 已完成（PR #169 已合并至 `main`，merge commit `bc88cb60`，2025-12-29 21:08 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**:
  - Org UI（DEV-PLAN-035）已交付 `/org/nodes` 的组织树 + 节点面板（创建/编辑/Move，均为 Insert 时间片语义）。
  - 现状缺口：缺少“对既有记录做就地更正（Correct）”与“删除一条记录（DeleteSliceAndStitch）”的 UI 能力，导致历史录入错误只能靠 API/DB/运维介入或被迫用 Insert 语义绕行。
- **当前痛点**:
  - Insert 与 Correct 语义混用会造成时间线漂移（把“更正”误做成“从某天起变化”），并增加误操作概率。
  - 删除记录必须对齐 Auto-Stitch（删除后自动缝补时间轴、避免 gap），否则会触发 DB commit-time gap-free 门禁。
  - `org_edges` 写入会影响 `org_edges.path`（长路径 SSOT），需遵循 069B 的 preflight 与 prefix rewrite 限制。
- **业务价值**:
  - 提供符合 HR 有效期模型（Valid Time=DATE 日粒度）的“更正/撤销记录”操作，使组织架构维护更可控、可审计、可回溯（`org_audit_logs` + outbox events）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [x] 在 `/org/nodes`（组织架构页）为“记录（时间片）”补齐 v1 四个 UI 操作入口：
  - Node slices：**修改记录（Correct）**、**删除记录（DeleteSliceAndStitch）**
  - Edge slices（child 视角）：**修正移动（CorrectMoveNode）**、**撤销移动（DeleteEdgeSliceAndStitch）**
- [x] UI 语义清晰且独立：Insert / Correct / DeleteSliceAndStitch / Move / CorrectMove 分工明确，避免“同一按钮多语义”。
- [x] 权限拆分：Correct / DeleteSliceAndStitch / CorrectMove / DeleteEdgeSliceAndStitch 需要独立且更高权限（与 Insert 分离）。
- [x] 写入一致性与审计对齐：冻结窗口（freeze）、并发串行化（timeline lock）、审计日志（`org_audit_logs`）与事件发布（`*.v1`）全部由 Service 作为权威保障（UI 只做契约承载）。

### 2.2 非目标 (Out of Scope)
- 不实现“删除实体”（例如删除 `org_nodes` 本体）；本文的“删除记录”仅指删除某条时间片记录（对齐 DEV-PLAN-066 的定义）。
- 不在 v1 实现 Node/Edge 的 `Rescind`/`ShiftBoundary` UI（虽同属时间线操作，但语义与前置条件不同，建议另起计划）。
- Job Catalog 的任何改造/重构不在本计划范围内：另起计划承载。
- 不新增新的鉴权对象/动作语义：复用既有 `read/write/admin` action；不手工修改聚合文件（`config/access/policy.csv*` 必须由 `make authz-pack` 生成）。
- 不引入新的运维/监控开关。

## 2.3 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器/工具链”，不复制脚本细节；以 `AGENTS.md`/`Makefile`/CI 为准。

- 触发器清单（预计命中）：
  - [x] Go 代码（触发器矩阵：`AGENTS.md`）
  - [x] `.templ` / Tailwind（触发器矩阵：`AGENTS.md`）
  - [x] 多语言 JSON（`modules/org/presentation/locales/*.json`，触发器矩阵：`AGENTS.md`）
  - [x] Authz（`make authz-pack && make authz-test && make authz-lint`，触发器矩阵：`AGENTS.md`）
- SSOT：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 删除并缝补：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`
  - `org_edges.path` 一致性写入门禁：`docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  U[Browser / HTMX] --> C[OrgUIController (/org/*)]
  C --> T[templ pages/components]
  C --> S[OrgService]
  S --> R[OrgRepository]
  R --> PG[(Postgres)]
  PG --> TR[Triggers: gap_free + org_edges_set_path_depth_and_prevent_cycle]
  S --> AL[(org_audit_logs)]
  S --> OB[(outbox_events)]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：Correct/删除记录 与 Insert 权限分离**
  - 选项 A：新增 action（如 `correct/delete`）并在全仓库扩散。
  - 选项 B（选定）：复用既有 `read/write/admin`，将 Correct/删除记录/修正移动/撤销移动收敛到 `*.admin`。
  - 理由：最小化鉴权语义扩散，减少策略碎片与门禁面。
- **决策 2：`org_edges` 不提供“edge slice Correct”**
  - 选项 A：暴露 `CorrectEdgeSlice`（就地更正 edge 行）。
  - 选项 B（选定）：仅提供结构化入口 `CorrectMoveNode`（修正一次移动动作）+ `DeleteEdgeSliceAndStitch`（撤销一次移动）。
  - 理由：edge 的就地更正存在“结构级副作用”（子树 `org_edges.path` prefix rewrite + 069B preflight），用“修正移动/撤销移动”更符合用户心智与失败路径。
- **决策 3：日期单一权威**
  - 选项 A：同时维护 `effective_date`（页面 as-of）与 `target_effective_date`（目标记录）。
  - 选项 B（选定）：单一 `effective_date`；records 列表行的 `effective_date` 即目标记录起点，点击操作时以该日期打开表单并提交写入。
  - 理由：避免双权威导致 URL/表单/服务端含义漂移。
- **决策 4：UI 写入直接复用 `OrgService`**
  - 与现有 Org UI 一致：UI Controller 直接调用 `OrgService`（不绕 `/org/api`），保持同一事务/错误映射/HTMX 响应模式。

### 3.3 术语与不变量 (Definitions & Invariants)
- **记录（Record）**：指某个实体时间线（timeline key）上的一条有效期切片（`org_node_slices` 或 `org_edges` 的一行）。
- **Insert（编辑/创建）**：创建“从某天起生效”的新切片，不改动既有记录。
- **Correct（修改记录）**：对“某条既有切片”做就地更正，不创建新切片。
- **DeleteSliceAndStitch（删除记录）**：删除一条切片，并把其有效期并入相邻切片以保持时间轴无 gap（对齐 066 的 commit-time gap-free 门禁）。
- **Valid Time vs Audit/Tx Time**：Valid Time 使用 `date`（日粒度），审计时间使用 `timestamptz`（对齐 064）。

## 4. 数据模型与约束 (Data Model & Constraints)
> v1 不引入迁移；本节只标注与本计划直接相关的字段与约束，Schema 以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为权威。

### 4.1 `org_node_slices`（Node 时间线）
- 字段（关键子集）：
  - `org_node_id uuid`（timeline key）
  - `name varchar(255)`、`i18n_names jsonb`、`status text`
  - `effective_date date`、`end_date date`
- `status` 枚举（DB 约束为权威）：
  - 允许值：`active | retired | rescinded`
  - v1 UI 只提供：`active | retired`（“rescinded”仅由 `Rescind` 流程写入；本计划不提供手工选择入口）
  - 兼容性要求（阻断项）：现有 UI 表单若仍提交 `inactive` 将违反 DB check；本计划必须把 UI 的 “Inactive/停用” 选项收敛为 **value=`retired`**（文案可继续显示“停用/退役”，但写入值必须与 DB 一致）。
- 约束（关键子集）：
  - `effective_date <= end_date`
  - EXCLUDE：同一 `(tenant_id, org_node_id)` 的日期范围不得重叠
  - commit-time gap-free 约束触发器（066）：同一时间线不得有 gap，且最后一条必须以 `9999-12-31` 结尾（若该时间线存在记录）
- 索引（用于 records 列表）：
  - `org_node_slices_tenant_node_effective_idx (tenant_id, org_node_id, effective_date)`

### 4.2 `org_edges`（Edge 时间线 + 长路径 SSOT）
- 字段（关键子集）：
  - `hierarchy_type text`（v1 固定 `OrgUnit`）
  - `child_node_id uuid`（timeline key）、`parent_node_id uuid NULL`
  - `path ltree`、`depth int`
  - `effective_date date`、`end_date date`
- 约束（关键子集）：
  - EXCLUDE：同一 `(tenant_id, hierarchy_type, child_node_id)` 的日期范围不得重叠（single parent）
  - commit-time gap-free 约束触发器（066）
  - trigger：写入时自动设置 `path/depth` 并阻止环（cycle）
- 索引（用于 records 列表）：
  - `org_edges_tenant_child_effective_idx (tenant_id, child_node_id, effective_date)`

## 5. 接口契约 (UI Contracts / HTMX)
> v1 只新增/扩展 `/org/*` 的 UI 路由；写入业务语义以 `OrgService` 为权威。

### 5.1 页面与面板（现状复用 + 扩展 view）
- 页面：`GET /org/nodes?effective_date=YYYY-MM-DD&node_id=UUID`（现状）
- 面板：`GET /org/nodes/{id}?effective_date=YYYY-MM-DD[&view=...]`
  - `view` 扩展（v1）：
    - `view=`（默认）：详情 + records 列表
    - `view=edit`（现状）：Insert 编辑表单（`org.nodes write`）
    - `view=move`（现状）：Move 表单（`org.edges write`）
    - `view=correct`：Correct 表单（`org.nodes admin`）
    - `view=delete-slice`：DeleteNodeSliceAndStitch 表单（`org.nodes admin`）
    - `view=correct-move`：CorrectMove 表单（`org.edges admin`）
    - `view=delete-edge-slice`：DeleteEdgeSliceAndStitch 表单（`org.edges admin`）

#### 5.1.1 Node Panel UI 信息架构（v1）
- 节点面板（默认 view）分为三块：
  - **节点详情**（现状 `NodeDetails`，展示 long name/code/status/有效期窗口/parent hint）
  - **节点属性记录（Node slices records）**：展示 `org_node_slices` 时间线（倒序）
  - **上级关系记录（Edge slices records, child 视角）**：展示 `org_edges` 时间线（倒序）
- records 每行提供操作入口（按权限可见）：
  - 查看：切换页面 `effective_date` 并刷新面板/树
  - Node slices：Correct、DeleteSliceAndStitch
  - Edge slices：CorrectMove、DeleteEdgeSliceAndStitch

### 5.2 records 列表（读契约）
- Node records：该节点 `org_node_slices` 全量时间线，`effective_date DESC`
  - 必要字段：`slice_id, effective_date, end_date, name, status`
  - 行状态：`active_at_as_of = (slice.effective_date <= as_of <= slice.end_date)`（`as_of` 即页面 `effective_date`）
- Edge records（child 视角）：该节点作为 child 的 `org_edges` 全量时间线，`effective_date DESC`
  - 必要字段：`edge_id, effective_date, end_date, parent_node_id`
  - 展示字段：`parent_label_at_start`（以 `edge.effective_date` 解析 parent 的 name/code；`parent_node_id IS NULL` 显示 Root）
  - 行状态：`active_at_as_of = (edge.effective_date <= as_of <= edge.end_date)`

#### 5.2.1 records 行为细节（v1）
- 高亮：`active_at_as_of=true` 的行使用强调样式，帮助用户理解“当前视图日期下正在生效的是哪条记录”。
- 禁用/隐藏：
  - 若无 `org.nodes admin`：隐藏 Node records 的 Correct/Delete 入口。
  - 若无 `org.edges admin`：隐藏 Edge records 的 CorrectMove/Delete 入口。
  - Edge records 的“最早一条记录”禁止删除：UI 直接禁用按钮并提示原因（服务端仍会在误操作时返回 `ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`）。

### 5.3 写入路由（HTMX Form Data）
> 表单内 `effective_date` 必须为目标记录起点；UI 不维护 `target_effective_date`。

#### 5.3.1 Node Correct
- `POST /org/nodes/{id}:correct`
- Authz：`org.nodes admin`
- Form：
  - `effective_date`（Required, `YYYY-MM-DD`，只读展示 + hidden 提交）
  - `name`（Optional）
  - `status`（Optional，enum：`active | retired`；禁止提交 `inactive`）
  - `display_order`（Optional int）
  - `i18n_names`（Optional，JSON string）
- Success（200）：
  - `htmx.PushUrl("/org/nodes?effective_date=...&node_id=...")`
  - 返回：更新后的 node panel（含 records）+（OOB）更新树
- Errors：
  - `400 ORG_INVALID_BODY`：缺少必填字段
  - `404 ORG_NOT_FOUND`：目标切片不存在（该日期下无记录）
  - `409 ORG_FROZEN_WINDOW`：冻结窗口拒绝
  - `409 ORG_TIME_GAP`：仅当 DB 已处于不一致状态时可能出现（应视为数据异常；UI 仅需展示表单级错误）

#### 5.3.2 Node DeleteSliceAndStitch
- `POST /org/nodes/{id}:delete-slice`
- Authz：`org.nodes admin`
- Form：
  - `effective_date`（Required，目标 slice 起点）
  - `reason_code`（Required）
  - `reason_note`（Optional）
- Success（200）：同 5.3.1
- Errors：
  - `400 ORG_INVALID_BODY`：缺少 `reason_code`（取决于租户 settings 的 reason_code_mode）
  - `422 ORG_NOT_FOUND_AT_DATE`：目标切片不存在（必须为切片起点）
  - `409 ORG_FROZEN_WINDOW`：冻结窗口拒绝
  - `409 ORG_TIME_GAP`：DB gap-free 门禁拒绝（066）

#### 5.3.3 Edge CorrectMoveNode（修正移动）
- `POST /org/nodes/{id}:correct-move`
- Authz：`org.edges admin`
- Form：
  - `effective_date`（Required，目标 edge slice 起点）
  - `new_parent_id`（Required UUID）
- Success（200）：同 5.3.1
- Errors：
  - `422 ORG_USE_MOVE`：目标日期不是 edge slice 起点（应使用 Move Insert）

#### 5.3.4 Edge DeleteEdgeSliceAndStitch（撤销移动）
- `POST /org/nodes/{id}:delete-edge-slice`
- Authz：`org.edges admin`
- Form：
  - `effective_date`（Required，目标 edge slice 起点）
  - `reason_code`（Required）
  - `reason_note`（Optional）
- Success（200）：同 5.3.1
- Errors：
  - `400 ORG_INVALID_BODY`：缺少 `reason_code`（取决于租户 settings 的 reason_code_mode）
  - `422 ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`（066）
  - `422 ORG_PREFLIGHT_TOO_LARGE`（069B）
  - `409 ORG_FROZEN_WINDOW`：冻结窗口拒绝
  - `409 ORG_TIME_GAP`（066）

### 5.4 操作清单与 UI 覆盖矩阵（现状 + 本计划）
> 目的：把“已纳入 UI 的操作”与“本计划新增操作”统一列出来，作为后续 UI/鉴权/文案的共同事实源；细节契约以对应 dev-plan 为准（避免在本文重复复制）。

| 实体 | 操作（语义） | Service（示例） | UI（现状） | 本计划（070） |
| --- | --- | --- | --- | --- |
| Node slices | 创建（Insert） | `CreateNode` | ✅ `/org/nodes` | 不变 |
| Node slices | 编辑（Insert，新切片） | `UpdateNode` | ✅ `/org/nodes/{id}?view=edit` | 文案需明确为“编辑（Insert）” |
| Node slices | 修改记录（Correct，就地） | `CorrectNode` | ❌ | ✅ 新增 |
| Node slices | 删除记录（DeleteSliceAndStitch） | `DeleteNodeSliceAndStitch` | ❌ | ✅ 新增 |
| Node (lifecycle) | 撤销（Rescind） | `RescindNode` | ❌ | 不在 v1 |
| Node slices | 调整边界（ShiftBoundary） | `ShiftBoundaryNode` | ❌ | 不在 v1 |
| Edges | 变更上级（Move，Insert） | `MoveNode` | ✅ `/org/nodes/{id}?view=move` | 文案需明确为“变更上级（Insert）” |
| Edges | 修正移动（CorrectMove） | `CorrectMoveNode` | ❌ | ✅ 新增 |
| Edges | 撤销移动（DeleteSliceAndStitch） | `DeleteEdgeSliceAndStitch` | ❌ | ✅ 新增 |
| Assignments | 入职/创建（事件式写入） | `HirePersonnelEvent` | ✅ `/org/assignments` | 不变（仅列入清单） |
| Assignments | 转岗/离职（事件式写入） | `TransitionAssignment` | ✅ `/org/assignments/{id}:transition` | 不变（仅列入清单） |
| Positions | 创建/编辑（Insert） | `CreatePosition`/`UpdatePosition` | ✅ `/org/positions` | 不变（仅列入清单） |

### 5.5 操作语言清晰性与独立性（建议口径）
- Insert vs Correct：
  - “编辑（Insert，新切片）”只做“从某天起生效的新记录”，不会改动既有记录；用于“从某天起开始变化”。
  - “修改记录（Correct，就地更正）”只做“修正既有记录内容”，不会创建新记录；用于“历史录入错误修正”。
- DeleteSliceAndStitch vs Rescind：
  - “删除记录（DeleteSliceAndStitch）”是时间轴操作（删一条记录并缝补相邻记录），对齐 066。
  - “撤销（Rescind）”是生命周期语义（带前置条件校验），不等价于“删除记录”。
- Move vs CorrectMove：
  - “变更上级（Move，Insert）”用于“从某天起开始移动到新上级”（会创建新 edge slice）。
  - “修正移动（CorrectMove）”用于“更正一条既有移动记录”（就地更正 edge slice 起点的 parent）。
  - `ORG_USE_MOVE`/`ORG_USE_CORRECT_MOVE` 是服务端强制语义分流；UI 必须按 6.3 做互斥引导，避免用户在两个入口间试错。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 面板默认视图渲染（含 records）
1. 校验 `effective_date`（Valid Time=DATE）。
2. 读取并渲染：
   - `OrgService.GetNodeAsOf(node_id, as_of)` → NodeDetails
   - `OrgService.GetHierarchyAsOf("OrgUnit", as_of)` → 树（OOB）
   - `OrgService.ListNodeSlicesTimeline(node_id)` → Node records
   - `OrgService.ListEdgesTimelineAsChild("OrgUnit", node_id)` → Edge records
3. 计算每条 record 的 `active_at_as_of`，并在 UI 上高亮当前生效行。
4. `ensureOrgPageCapabilities` 需要覆盖 `org.nodes admin` 与 `org.edges admin`，以便在 templ 中用 `pageCtx.CanAuthz` 控制按钮可见性。

#### 6.1.1 建议新增的读方法（实现约束）
- Repository（`modules/org/infrastructure/persistence`）：
  - `ListNodeSlicesTimeline(ctx, tenantID, nodeID) -> []services.NodeSliceRow`（`ORDER BY effective_date DESC`）
  - `ListEdgesTimelineAsChild(ctx, tenantID, hierarchyType, childNodeID) -> []EdgeTimelineRow`（`ORDER BY effective_date DESC`，并返回 parent 在 `edge.effective_date` 当天的 name/code）
- Service（`modules/org/services`）：
  - `ListNodeSlicesTimeline(ctx, tenantID, nodeID) -> []services.NodeSliceRow`
  - `ListEdgesTimelineAsChild(ctx, tenantID, hierarchyType, childNodeID) -> []EdgeTimelineRow`
- 约束：
  - 读方法必须显式 `tenant_id` 过滤；不复用写侧 `Lock*` 方法（避免不必要的锁）。
  - **必须同步把上述方法加入 `modules/org/services/org_service.go` 的 `OrgRepository` interface，并在 `modules/org/infrastructure/persistence` 的实现中落地**，否则无法编译。

`EdgeTimelineRow`（建议定义在 `modules/org/services`，避免 UI 直接拼 SQL）：
- `edge_id uuid`
- `parent_node_id *uuid`
- `child_node_id uuid`
- `effective_date date`
- `end_date date`
- `parent_name_at_start *string`（`org_node_slices.name`）
- `parent_code *string`（`org_nodes.code`）

### 6.2 日期单一权威（UI 规则）
- records 列表中的每条记录行都以其 `effective_date` 作为：
  - 打开表单的日期（Correct/Delete/CorrectMove/DeleteEdge）
  - 提交写入的日期（Form `effective_date`）
  - 页面 URL 的日期（`/org/nodes?effective_date=...`）

### 6.3 Move 与 CorrectMove 的互斥引导（避免用户试错）
- 互斥错误来源：
  - `MoveNode` 在 `effective_date == movedEdge.EffectiveDate` 时返回 `422 ORG_USE_CORRECT_MOVE`
  - `CorrectMoveNode` 在 `effective_date` 非 edge slice 起点时返回 `422 ORG_USE_MOVE`
- v1 UX 策略（不引入新组件）：
  - 在 Move 提交收到 `ORG_USE_CORRECT_MOVE` 时：
    - 若用户具备 `org.edges admin`：直接返回 CorrectMove 表单（预填 `effective_date/new_parent_id`）
    - 否则：保留 Move 表单并提示“该操作需要更高权限（admin）进行修正移动”
  - 在 CorrectMove 提交收到 `ORG_USE_MOVE` 时：
    - 若用户具备 `org.edges write`：直接返回 Move 表单（预填 `effective_date/new_parent_id`）
    - 否则：提示“需要 write 权限进行变更上级（Insert）”

### 6.4 长路径（`org_edges.path`）在不同时间点的合理性保障
- SSOT：`org_edges.path` 是祖先链权威（069B）；长路径名称在读侧按 `effective_date`（as-of）从 `org_edges.path` + 各节点当日 `org_node_slices.name` 推导（`pkg/orglabels`）。
- Node Correct/Delete 只修改 `org_node_slices` 的字段或时间轴边界，不直接改 `org_edges.path`；长路径名称随 as-of 自然分段变化，属于预期。
- Edge CorrectMove/Delete 会触发“未来子树”的 `org_edges.path` 前缀重写（prefix rewrite）以维持任意 as-of 的一致性；写路径必须走 `OrgService.{CorrectMoveNode,DeleteEdgeSliceAndStitch}` 并遵循 069B preflight（超限返回 `ORG_PREFLIGHT_TOO_LARGE`）。

### 6.5 失败路径与用户体验（v1 必须覆盖）
- `409 ORG_TIME_GAP`：提示“时间线必须无 gap（删除/写入失败）”，建议刷新页面确认当前 records。
- `422 ORG_PREFLIGHT_TOO_LARGE`：提示影响范围过大（超过上限）导致在线拒绝；建议拆分日期/拆小子树/联系管理员走离线方案（对齐 069B 保护目标）。
- 冻结窗口拒绝：沿用现有错误码与展示策略（Service 权威），UI 展示为表单级错误。

## 7. 安全与鉴权 (Security & Authz)
### 7.1 权限拆分（v1 结论）
- Insert（现状，`write`）：
  - Node Create/Update：`org.nodes write`
  - Edge Move：`org.edges write`
- Correct/删除记录/修正移动/撤销移动（更高权限，`admin`）：
  - `org.nodes admin`：CorrectNode / DeleteNodeSliceAndStitch
  - `org.edges admin`：CorrectMoveNode / DeleteEdgeSliceAndStitch

### 7.2 角色分层（v1 建议）
- `role:org.orgchart.viewer`：`org.hierarchies read`
- `role:org.orgchart.editor`：viewer + `org.nodes write` + `org.edges write`
- `role:org.orgchart.admin`：editor + `org.nodes admin` + `org.edges admin`

### 7.3 UI 可见性与服务端拒绝
- records 行内按钮可见性用 `pageCtx.CanAuthz("org.nodes","admin")` / `pageCtx.CanAuthz("org.edges","admin")` 控制。
- 服务端仍强制鉴权（无论按钮是否隐藏），HTMX 场景下保持现有 403 响应风格。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- 删除并缝补语义与门禁：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`
- `org_edges.path` 一致性写入门禁与 preflight：`docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`

### 8.2 里程碑（实现顺序）
1. [x] records 读模型：repo/service 读方法 + viewmodels + templ 渲染（含 active_at_as_of 高亮）。
2. [x] 修复 Node `status` 枚举漂移：UI 表单选项收敛为 `active/retired`（禁止提交 `inactive`），并补齐对应 i18n 文案。
3. [x] Node Correct：新增 `view=correct` 表单与 `POST :correct` 写入。
4. [x] Node DeleteSliceAndStitch：新增 `view=delete-slice` 表单与 `POST :delete-slice` 写入（reason_code/note）。
5. [x] Edge CorrectMove：新增 `view=correct-move` 表单与 `POST :correct-move` 写入，并落地互斥引导（6.3）。
6. [x] Edge DeleteEdgeSliceAndStitch：新增 `view=delete-edge-slice` 表单与 `POST :delete-edge-slice` 写入（reason_code/note + 069B 错误展示）。
7. [x] Authz/UI：确保页面 capabilities 覆盖 `admin`，并对齐按钮可见性与服务端鉴权点。
8. [x] i18n：补齐 `modules/org/presentation/locales/{en,zh}.json` 文案（标题/按钮/错误提示/确认文案）。
9. [x] 测试与 Readiness：按 `AGENTS.md` 触发器执行与记录。

## 9. 测试与验收标准 (Acceptance Criteria)
- [x] records 列表满足 5.2 的读契约：倒序、可定位 active_at_as_of、Edge 行能显示 `parent_label_at_start`（或 Root）。
- [x] Node `status` 值与 DB 约束一致：UI 不再提交 `inactive`；选择“退役/停用”时写入值为 `retired`，并可在 records/详情中正确展示。
- [x] Node Correct：不创建新切片；Correct 后 records 列表仍存在同一条 `effective_date` 记录且字段变更可见。
- [x] Node DeleteSliceAndStitch：删除后时间轴仍满足 gap-free 门禁（066），records 列表能反映相邻切片 `end_date` 的变化。
- [x] Edge CorrectMove：修正移动后树结构与长路径名称在 as-of 上一致；互斥错误能按 6.3 给出可执行引导。
- [x] Edge DeleteEdgeSliceAndStitch：撤销移动后满足 066/069B 门禁；`ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`/`ORG_PREFLIGHT_TOO_LARGE` 有清晰错误展示。
- [x] 无对应权限时：按钮不可见 + 服务端拒绝（403/422 表现与现有一致）。
- [x] 本计划命中的门禁均通过（入口见 `AGENTS.md`），生成物提交齐全。

## 10. 运维与监控 (Ops & Monitoring)
- 不新增 Feature Flag：沿用现有 `OrgRolloutEnabledForTenant` 的租户级开启策略。
- 关键审计：所有写入由 Service 写入 `org_audit_logs`，并通过 outbox 事件投递；UI 不额外埋点。
- 回滚：
  - 代码回滚：按常规 PR revert。
  - 数据回滚：Correct/DeleteSliceAndStitch 属业务写入，需依赖审计日志做人工回溯（本计划不提供自动回滚脚本）。

## 11. 实施与合并记录 (Implementation & Merge Record)
- PR：https://github.com/jacksonlee411/Bugs-Blossoms/pull/169
- Merge commit：`bc88cb606662a6602807002aa60d1fe4dad80b7b`（`main`）
- 合并时间：2025-12-29 21:08 UTC
- 本地验证（触发器矩阵）：`make authz-pack && make generate && make css && go fmt ./... && go vet ./... && make check lint && make check tr && make test && make authz-test && make authz-lint`
- CI：Quality Gates 全绿（Code Quality & Formatting / Unit & Integration Tests / Routing Gates / E2E Tests）
