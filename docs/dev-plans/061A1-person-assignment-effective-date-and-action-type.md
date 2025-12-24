# DEV-PLAN-061A1：任职（人员）生效日期 + 操作类型（雇用/调动/离职）详细设计

**状态**: 规划中（2025-12-23 23:20 UTC）

> 本计划是 `docs/dev-plans/061A-person-detail-hr-ux-improvements.md` 的后续：在“创建任职/编辑任职”的 UI 中补齐 HR 常用的“生效日期”和“操作类型（Hire/Transfer/Termination）”显式输入与契约。

## 1. 背景与上下文 (Context)
- **需求来源**: HR 用户反馈（061002：页面创建任职经历点击保存不成功；创建/编辑任职缺少“生效日期/操作类型”导致操作不确定）。
- **当前痛点**:
  - 任职表单当前将 `effective_date` 作为隐藏字段，并由外层页面（如人员详情页）“默认为今天”驱动，用户无法显式设置生效日期。
  - UI 未要求/展示“操作类型”（雇用/调动/离职），导致 HR 无法表达业务语义，也无法在后端形成可审计的人事事件。
  - 当选择较早日期时，会触发 Org 冻结窗口（freeze window）并返回 409，但 UI 缺少清晰的引导与可预见性。
- **业务价值**:
  - 让 HR 在一次操作里明确“何时生效 + 这次变更是什么性质”，同时沉淀到 `org_personnel_events`，形成可追溯审计链。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 在“创建任职/编辑任职”表单中增加可编辑的 `生效日期 (effective_date)` 输入（date）。
- [ ] 在表单中增加必填 `操作类型 (event_type)` 输入：`hire` / `transfer` / `termination`（UI 文案：雇用/调动/离职）。
- [ ] 语义收敛：
  - [ ] “创建任职”只支持 `event_type=hire`（雇用/入职任职）。
  - [ ] “变更任职”只支持 `event_type in (transfer, termination)`（调动/离职）。
- [ ] 前端根据 `event_type` 显示/校验必要字段（例如离职需要结束当前任职）。
- [ ] 后端在成功写入任职变更后，写入 `org_personnel_events`（`event_type` 与 `effective_date` 对齐），并补齐 `reason_code`（默认策略参考 `ReasonCodeMode`）。
- [ ] 对冻结窗口导致的 409 冲突，在 UI 给出可理解的错误信息，并在可行范围内提供预防性提示。

### 2.2 非目标 (Out of Scope)
- 不在本计划中实现完整的人事流程引擎（审批/回滚/批处理）。
- 不在本计划中引入新的“人事事件”对外 API（仅补齐现有 UI 与内部数据落库）。
- 不在本计划中扩展人员状态枚举（例如新增 `terminated`）；如需要，另起计划并附带迁移与全局影响评估。

## 2.3 工具链与门禁（SSOT 引用）
- **触发器清单（本计划命中项）**：
  - [ ] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] `.templ` / Tailwind（`make generate && make css`，并确保生成物提交）
  - [ ] 多语言 JSON（`make check tr`）
  - [ ] 路由治理（`make check routing`）
  - [ ] DB 迁移 / Schema（若调整事件落库/约束）
  - [ ] E2E（`make e2e ci`，覆盖创建/调动/离职三类）
  - [x] 文档门禁（`make check doc`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 总体策略
- “任职记录（org_assignments）”承担组织/职位的时间线事实。
- “人事事件（org_personnel_events）”承担 HR 语义与审计（雇用/调动/离职），并通过 `payload` 记录关键信息。

### 3.2 关键决策（ADR 摘要）
1) **操作类型落库使用 `org_personnel_events.event_type`（已存在约束）**
   - 选项 A：在 `org_assignments` 增加字段 `operation_type`。
   - 选项 B（选定）：使用 `org_personnel_events`（已有 `event_type` check：`hire/transfer/termination`），避免扩展任职事实表并保持审计语义独立。

2) **生效日期由表单显式输入，外层页面只提供默认值**
   - 当前：外层页面提供 `props.EffectiveDate`（通常为今天）并塞入隐藏字段。
   - 目标：表单显式展示/可编辑 `effective_date`，变更时联动刷新部门/职位搜索与冻结窗口提示。

3) **语义收敛：Hire=创建任职；Transfer/Termination=“变更任职”业务操作（不是 Patch）**
   - `hire`：创建新的 primary assignment（现有 `POST /org/assignments`）。
   - `transfer`：关闭旧 primary assignment（截断 end_date）并新建 primary assignment（两写，事务一致）。
   - `termination`：关闭当前 primary assignment（截断 end_date），并记录离职事件；本计划不强制联动 `persons.status`。

4) **冻结窗口 UX：默认生效日期 = max(today, cutoff) + date input min=cutoff**
   - 目的：把“061002 选早期日期保存失败”的不可预见错误变为“可预防”的输入约束与提示。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 既有表
- `org_assignments`：任职时间线（`effective_date`, `end_date`，并由 EXCLUDE 约束防重叠）。
- `org_personnel_events`：人事事件（**已存在** `event_type` check：`hire|transfer|termination`）。

### 4.2 `org_personnel_events.payload` 约定（新增契约）
`payload` 必须是 JSON object（由 check 约束保证）。建议字段：
```json
{
  "assignment_id": "uuid",
  "event_source": "ui",
  "org_node_id": "uuid",
  "position_id": "uuid",
  "previous_assignment_id": "uuid",
  "previous_org_node_id": "uuid",
  "previous_position_id": "uuid"
}
```
- `Hire`：填 `assignment_id/org_node_id/position_id`。
- `Transfer`：填 `assignment_id/org_node_id/position_id` + `previous_*`。
- `Termination`：至少填 `previous_assignment_id`，以及关闭动作的目标信息（可复用 `previous_*`）。

### 4.3 `reason_code` 策略
`org_personnel_events.reason_code` 不允许空字符串（check 约束）。本计划采用与 Org 现有逻辑一致的默认：
- 若 UI 未提供 reason code：按 `ReasonCodeMode` 的默认行为填充（shadow/默认 -> `legacy`）。

## 5. 接口契约 (API Contracts)
> 说明：本计划以 UI（HTMX）为主；如需新增 JSON API，另补充小节。

### 5.1 HTMX：创建任职（Hire）
> **收敛**：`POST /org/assignments` 仅允许 `event_type=hire`（若收到其他值，返回 422 并提示用户选择“调动/离职”的入口）。

- **Endpoint**: `POST /org/assignments`
- **Query**: `effective_date=YYYY-MM-DD`（仍保留，兼容现有路由与搜索联动；但以 Form Data 为准）
- **Source of Truth（必须避免歧义）**:
  - 如 Form Data 里有 `effective_date`，必须以 Form Data 为准；URL Query 仅用于“可分享/可回退”的页面状态。
  - 否则用户在表单里改了日期，但后端仍用 URL 上的旧日期，会导致“看起来保存不成功”。
- **Form Data（字段与校验）**:
  - `effective_date` (required, date)：用户可编辑，必须 `>= cutoff`（freeze enforce 时）
  - `event_type` (required, enum)：必须为 `hire`
  - `pernr` (required)
  - `org_node_id` (required)：UI 必选部门
  - `position_id` (required)：UI 必选职位（并按部门过滤）
  - `reason_code` (optional)：缺省按 `ReasonCodeMode` 填充
  - `reason_note` (optional)
- **Success (200 OK)**:
  - Body：返回 `<div id="org-assignment-form">...</div>`（替换表单区域）
  - OOB：返回 `<div id="org-assignments-timeline" hx-swap-oob="true">...</div>` 更新时间线
  - Header：
    - `HX-Push-Url`：
      - Org 页：`/org/assignments?effective_date=...&pernr=...`
      - Person 详情（include_summary=1）：清理 step（用 `HX-Replace-Url`，见 061A 实现）
- **Error**:
  - `409 Conflict`：冻结窗口/时间重叠等冲突（`ORG_FROZEN_WINDOW`, `ORG_OVERLAP` 等）
  - `422 Unprocessable Entity`：字段缺失/无效（例如缺少 `position_id`）

### 5.2 HTMX：变更任职（Transfer / Termination）
> **新增 endpoint（收敛为方案 A）**：用“业务事件”表达调动/离职，避免复用 patch 导致语义与实现不一致。

- **Endpoint**: `POST /org/assignments/{id}:transition`
- **Path Params**:
  - `id`: 被操作的“当前任职（primary）”的 assignment_id（UUID）
- **Form Data（字段与校验）**:
  - `effective_date` (required, date)：用户可编辑，必须 `>= cutoff`（freeze enforce 时）
  - `event_type` (required, enum)：必须为 `transfer` 或 `termination`
  - `pernr` (required)：只读展示但仍提交（便于错误回显/一致性校验）
  - `org_node_id`：
    - `transfer` 必填（新部门）
    - `termination` 不需要（隐藏/禁用）
  - `position_id`：
    - `transfer` 必填（新职位）
    - `termination` 不需要（隐藏/禁用）
  - `reason_code` / `reason_note`：同 5.1
- **Success (200 OK)**:
  - 同 5.1（替换表单 + OOB 更新时间线）
- **Error**:
  - `404 Not Found`: assignment 不存在
  - `409 Conflict`: 冻结窗口/时间线冲突
  - `422 Unprocessable Entity`: event_type 不允许/字段缺失/目标职位在该日期不存在等

> 行业常见约定：`Hire/Termination` 通常属于“雇佣关系（Employment）”生命周期；本系统当前以“任职（Assignment）+ 人事事件（Personnel Event）”表达，合理但需避免把“终止雇佣”误实现成“只结束某一条任职”。因此 termination 的后端语义建议为“结束该人员在生效日的所有有效任职”（见 6.3）。

### 5.3 兼容与保留：编辑（Correct / Patch）
- 现有 `POST /org/assignments/{id}:correct` 与 `PATCH /org/assignments/{id}` 保留，但 UI 文案改为：
  - `correct`：更正（管理员/特权用）
  - `patch`：不作为 HR “调动/离职”入口（避免误用）

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 雇用（Hire）
1. 校验冻结窗口：`effective_date >= cutoff`（mode=enforce 时）。
2. 创建 primary assignment：`effective_date` 起生效，`end_date=9999-12-31`。
3. 写入 `org_personnel_events(event_type=hire, effective_date=...)`。
4. `payload` 最小必填：`assignment_id/org_node_id/position_id/event_source`。

### 6.2 调动（Transfer）
1. 校验冻结窗口：`effective_date >= cutoff`。
2. 读取并锁定“被操作的 assignment_id”（即 UI 传入的 `{id}`），并校验其为 `primary` 且 pernr 匹配。
3. 将旧 assignment 的 `end_date` 截断到 `effective_date`（区间 `[old.effective_date, effective_date)`）。
4. 创建新 primary assignment（`effective_date` 起生效）。
5. 写入 `org_personnel_events(event_type=transfer, payload.previous_* + new *)`：
   - `payload.previous_assignment_id/previous_org_node_id/previous_position_id`
   - `payload.assignment_id/org_node_id/position_id`

### 6.3 离职（Termination）
1. 校验冻结窗口：`effective_date >= cutoff`。
2. 读取并锁定“被操作的 assignment_id”（即 UI 传入的 `{id}`），并校验其为 `primary` 且 pernr 匹配。
3. 将该人员在生效日仍有效的任职全部截断到 `effective_date`（建议至少包含：primary + 其他扩展任职类型）。
4. 写入 `org_personnel_events(event_type=termination, payload.previous_*)`：至少记录被操作的 primary assignment 作为 “anchor”，其余被关闭任职可追加到 `payload.terminated_assignment_ids`（数组）用于审计。
5. 本计划不强制联动 `persons.status`；如 HR 需要“离职=人员不可用”，另起计划做人员状态与权限联动。

### 6.4 时间语义与边界条件（行业通常做法）
> 采用 Postgres 时间区间的常见约定：任职区间是半开区间 `[effective_date, end_date)`，因此“调动/离职的生效日”是新状态开始的第一天，旧状态在该日 00:00 即失效。

- **禁止零长度区间**：由于 DB check `effective_date < end_date`，当 `effective_date == old.effective_date` 时无法通过“截断 end_date=effective_date”。
  - 处理建议：
    - 对 `transfer/termination`：若 `effective_date == current.effective_date`，提示用户使用“更正（correct）/撤销（rescind）”而不是“调动/离职”。
    - 对 `hire`：若当天错误录入，应走 `correct/rescind` 而非再次 hire。
- **有效范围校验**：`transfer/termination` 的 `effective_date` 必须满足 `current.effective_date < effective_date <= current.end_date`（上界按实现决定是否允许等于 end_date）。
- **重入/再雇用（rehire）**：若该人员已存在有效 primary assignment，则 `hire` 应返回冲突并引导走 `transfer`；若人员曾离职且当前无有效任职，则允许再次 `hire`。

## 7. 安全与鉴权 (Security & Authz)
- 页面能力：`org.assignments:assign` 仍为主门槛。
- 新增 transition endpoint：复用同一 object/action（`org.assignments:assign`），避免引入新的权限碎片；如后续需要拆分，再单独起计划调整 Casbin policy。

### 7.1 UI/HTMX 交互契约（可直接编码）
#### 7.1.1 表单组件改造点
- 文件：`modules/org/presentation/templates/components/orgui/assignments.templ`
  - 在 `AssignmentForm` 增加两个输入：
    - `effective_date`：`<input type="date" name="effective_date" ...>`（从 props 传默认值）
    - `event_type`：`<select name="event_type">`（或 radio group）
  - `event_type` 改变时：
    - `hire/transfer`：显示部门+职位
    - `termination`：隐藏/禁用部门+职位，并在提交时不发送 `org_node_id/position_id`
  - `effective_date` 改变时：
    - 更新部门/职位搜索 endpoint 的 effective_date 参数（保持与现有 `/org/nodes/search`、`/org/positions/search` 一致）
    - 更新 “冻结窗口提示文案”（显示 cutoff）

#### 7.1.2 人员详情页接入点
- 文件：`modules/person/presentation/templates/pages/persons/detail.templ`
  - 仍复用 `/org/assignments/form?...&include_summary=1`，但表单内展示/编辑 `effective_date`。
  - 默认 effective_date：`max(today, cutoff)`（由后端在 props 中传入）。

#### 7.1.3 冻结窗口（freeze window）前置约束（最小集）
- cutoff 的计算口径必须与后端一致（参见 `modules/org/services/freeze.go` 的 `computeFreezeCutoffUTC` 语义）。
- UI 行为：
  - date input：设置 `min=cutoff(YYYY-MM-DD)`
  - 默认值：`effective_date = max(today, cutoff)`
  - 提示文案：展示“系统冻结窗口：最早可选 {cutoff}”

#### 7.1.4 错误码到 UI 提示映射（最小集）
- `ORG_FROZEN_WINDOW`（409）：提示“生效日期早于系统冻结窗口，最早可选：{cutoff}”，并将 date input 的 `min` 设置为 `cutoff`。
- `ORG_OVERLAP`（409）：提示“该人员在该日期已有任职（时间线冲突）”，引导使用“更正/调动”而非重复创建。
- `ORG_POSITION_NOT_FOUND_AT_DATE`（422）：提示“该职位在生效日无效，请调整生效日期或选择其他职位”。
- `ORG_INVALID_BODY`（422/400）：提示“请补齐生效日期/操作类型/部门/职位”。
- `ORG_ASSIGNMENT_TYPE_DISABLED`（422）：提示“该任职类型未启用”。

#### 7.1.5 i18n 契约（最小集）
- 字段标签：
  - `Org.UI.Assignments.Fields.EffectiveDate`（生效日期）
  - `Org.UI.Assignments.Fields.EventType`（操作类型）
- 操作类型选项：
  - `Org.UI.Assignments.EventType.Hire`（雇用）
  - `Org.UI.Assignments.EventType.Transfer`（调动）
  - `Org.UI.Assignments.EventType.Termination`（离职）
- 冻结窗口提示：
  - `Org.UI.Assignments.Errors.FrozenWindow`（模板数据：`Cutoff`）

## 8. 依赖与里程碑 (Dependencies & Milestones)
- 依赖：`DEV-PLAN-061A`（任职表单已接入人员详情页）。
- 里程碑：
  1. [ ] UI：任职表单增加 `effective_date` + `event_type`，并完善联动与校验。
  2. [ ] 后端：创建/调动/离职的写入语义落地（含事务与审计事件 `org_personnel_events`）。
  3. [ ] i18n：新增 `Hire/Transfer/Termination` 文案与字段标签（中英）。
  4. [ ] 测试：E2E 覆盖三类事件；补充冻结窗口错误展示用例。
  5. [ ] 门禁：按触发器矩阵执行并记录。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] 创建任职：可设置生效日期；可选择操作类型；保存成功后时间线更新，且写入 `org_personnel_events`。
- [ ] 调动：选择调动后，旧任职在生效日截断，新任职建立；时间线无重叠；写入 `org_personnel_events(event_type=transfer)`。
- [ ] 离职：选择离职后，当前任职在生效日截断；写入 `org_personnel_events(event_type=termination)`。
- [ ] 冻结窗口：当 `effective_date < cutoff` 时，UI 可读地提示原因（至少展示 cutoff），并阻止/指导用户选择合理日期。
- [ ] `make check doc` 通过（本次文档变更门禁）。

## 10. 待决事项（需在实施前关闭）
1. **“编辑任职”到底对应什么语义？**
   - **已收敛**：编辑任职（面向 HR）= 触发业务事件（调动/离职），使用 `POST /org/assignments/{id}:transition`。
2. **离职是否需要联动人员状态？**
   - **已收敛**：本计划仅关闭任职并记录人事事件；不修改 `persons.status`。如需状态联动，另起计划并评估全局影响。
3. **冻结窗口在本地/测试环境的默认值**
   - **已收敛（实现口径）**：freeze 规则保持现状；UI 必须展示 cutoff 并限制最早可选日期（min=cutoff），默认生效日期取 `max(today, cutoff)`。
