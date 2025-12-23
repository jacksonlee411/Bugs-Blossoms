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
- [ ] 在“创建任职/编辑任职”表单中增加必填 `操作类型 (event_type)` 输入：`hire` / `transfer` / `termination`（UI 文案：雇用/调动/离职）。
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

### 5.1 HTMX：创建任职（Hire / Transfer）
- **Endpoint**: `POST /org/assignments?effective_date=YYYY-MM-DD`
- **Form Data**:
  - `effective_date` (required, date, UI 可编辑)
  - `event_type` (required, enum: `hire|transfer|termination`)
  - `pernr` (required)
  - `org_node_id` (required unless `position_id` provided)
  - `position_id` (optional; 但 HR 体验上建议必选，并按部门过滤)
  - `reason_code` (optional)
  - `reason_note` (optional)
- **Success (200)**:
  - 返回表单片段（清空或切换到下一步）+ OOB 更新时间线。
- **Error**:
  - `409 Conflict`：冻结窗口/时间重叠等冲突（例如 `ORG_FROZEN_WINDOW`, `ORG_OVERLAP`）。
  - `422 Unprocessable Entity`：字段缺失/日期不存在等。

### 5.2 HTMX：编辑任职（Transfer / Termination）
> 本计划将“编辑任职”视为对现有任职的业务操作，而不是简单的字段编辑：
> - 调动：会“关闭旧任职 + 新建新任职”（两写，事务一致）。
> - 离职：会“关闭当前任职”（必要时联动人员状态，见待决）。

- **候选方案 A（倾向）**：新增 UI action endpoint（避免复用 `PATCH /org/assignments/{id}` 导致语义不清）
  - `POST /org/assignments/{id}:transition`
  - Form Data: `effective_date`, `event_type`, `org_node_id/position_id`（transfer 必填），`reason_*`
  - Success：更新时间线 + 写入 `org_personnel_events`

- **候选方案 B（兼容最小改动）**：复用现有 `PATCH /org/assignments/{id}` 与 `POST /org/assignments/{id}:rescind`
  - 不推荐：UI/后端语义分裂，且 transfer 需要“两写”无法自然表达。

> **决策**：在实施前需确定 A/B，默认采用 A。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 雇用（Hire）
1. 校验冻结窗口：`effective_date >= cutoff`（mode=enforce 时）。
2. 创建 primary assignment：`effective_date` 起生效，`end_date=9999-12-31`。
3. 写入 `org_personnel_events(event_type=hire, effective_date=...)`。

### 6.2 调动（Transfer）
1. 校验冻结窗口：`effective_date >= cutoff`。
2. 读取并锁定“当前 primary assignment”（as-of `effective_date` 或 “最新”策略，需实现约束）。
3. 将旧 assignment 的 `end_date` 截断到 `effective_date`（区间 `[old.effective_date, effective_date)`）。
4. 创建新 primary assignment（`effective_date` 起生效）。
5. 写入 `org_personnel_events(event_type=transfer, payload.previous_* + new *)`。

### 6.3 离职（Termination）
1. 校验冻结窗口：`effective_date >= cutoff`。
2. 读取并锁定“当前 primary assignment”。
3. 将当前 assignment 的 `end_date` 截断到 `effective_date`。
4. 写入 `org_personnel_events(event_type=termination, payload.previous_*)`。
5. **待决**：是否联动 `persons.status`（当前 DB check 仅允许 `active|inactive`）。建议先将人员置为 `inactive`，另起计划扩展 `terminated`。

## 7. 安全与鉴权 (Security & Authz)
- 页面能力：`org.assignments:assign` 仍为主门槛。
- 若新增 transition endpoint：复用同一 object/action 或新增 `org.assignments:transition`（需同步 authz 门禁与策略）。

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
   - 方案 A：编辑 = 触发业务事件（调动/离职），通过新 transition endpoint 实现（推荐）。
   - 方案 B：编辑 = patch 字段；调动/离职另做按钮（需更多 UI）。
2. **离职是否需要联动人员状态？**
   - 当前 `persons.status` 仅允许 `active|inactive`；若要 `terminated` 需迁移与全局调整。
3. **冻结窗口在本地/测试环境的默认值**
   - 当前缺省 `org_settings` 时默认 `FreezeMode=enforce`；是否对 HR 日常操作过严？是否需要在 seed 中写入 `shadow`？

