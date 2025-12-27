# DEV-PLAN-063：任职时间线部门/职位名称按时间切片渲染（TDD）

**状态**: 草拟中（2025-12-27 07:05 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**：本地开发环境验证发现（工号 `004` 任职时间线示例）。
- **当前痛点**：Org → Assignments 的任职时间线在 OrgNode/Position 存在历史切片（effective dating）时，会把所有行的“部门/职位名称”渲染为同一个（通常是最新）名称，覆盖历史语义。
- **业务价值**：让 HR 历史核对/审计可在同一页面对比多段任职区间时，看到对应时间切片的组织/职位名称，避免误判。

### 1.1 现象与复现 (Symptom & Repro)
以工号 `004` 为例（任职时间线展示）：

| 工号 | 生效区间 | 操作类型 | 部门 | 职位 |
| --- | --- | --- | --- | --- |
| 004 | 2025-12-01 → 2025-12-03 | 雇用 | AI治理办公室 (2) | 02 — 副总经理 |
| 004 | 2025-12-03 → 2025-12-08 | 调动 | AI治理办公室 (2) | 02 — 副总经理 |
| 004 | 2025-12-08 → 至今 | 调动 | AI治理办公室 (2) | 02 — 副总经理 |

当组织节点（或职位标题）在 2025-12-01 ~ 至今期间发生过重命名/切片更新时，预期上述三段在“部门/职位”列中应能体现当时的历史名称差异；但当前 UI 会将三段都渲染为最新名称。

**建议复现入口**：
- `http://localhost:3200/org/assignments?pernr=004&effective_date=2025-12-01`（或在页面输入 Pernr/切换 Effective Date）。
- 在同一 OrgNode/Position 上准备至少两段历史切片（重命名或 title 调整），确保 `004` 的任职区间覆盖这些时间点。

### 1.2 根因分析 (Root Cause)
- `modules/org/presentation/controllers/org_ui_controller.go` 在构建 `timeline.Rows` 时，使用了“页面级 effective_date（asOf）”为所有行计算 label：
  - `orgNodeLabelFor(..., asOf=effectiveDate)`
  - `positionLabelFor(..., asOf=effectiveDate)`
- 但上述 label helper 会进一步调用 `GetNodeAsOf` / `GetPosition(..., asOf=...)` 获取**该时间点**的切片；当页面 asOf 固定为“现在/最新”时，所有行都会命中最新切片，从而覆盖历史语义。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 任职时间线表格每一行的“部门/职位”显示为**该行任职记录时间切片**对应的组织/职位历史名称（而不是页面级 `effective_date` 的名称）。
  - [ ] 不改变任职时间线的行结构（不因组织重命名而拆分任职区间）。
  - [ ] 保持 UI 仍可基于页面 `effective_date` 正确识别“当前任职”（用于 summary/可编辑判断）。
  - [ ] 通过本计划命中的门禁（见 §2.1）。
- **非目标 (Out of Scope)**：
  - 不新增/调整数据库表结构与迁移。
  - 不重写任职时间线查询（`GetAssignments`/`AssignmentsToTimeline`）为“组织切片联表返回 name/code”。
  - 不在本计划内做 N+1 优化（如需将另开/升级方案，见 §3.2 决策 2）。

## 2.1 工具链与门禁（SSOT 引用）
> **目的**：避免在 dev-plan 里复制工具链细节导致 drift；本文只声明“本计划命中哪些触发器/工具链”，并给出可复现入口链接。

- **触发器清单（本计划命中）**：
  - [ ] 文档：`make check doc`
  - [ ] Go 代码（实施阶段）：`go fmt ./... && go vet ./... && make check lint && make test`
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
    A[Browser / HTMX] -->|GET /org/assignments?effective_date&pernr| B[OrgUIController.AssignmentsPage]
    B -->|GetAssignments| C[OrgService]
    C --> D[(DB)]
    B --> E[mappers.AssignmentsToTimeline]
    E --> F[timeline.Rows (EffectiveDate/EndDate/OrgNodeID/PositionID)]
    B --> G[labelAsOfForRow(row, pageAsOf)]
    B -->|OrgNodeLabel (labelAsOf)| H[OrgService.GetNodeAsOf]
    B -->|PositionLabel (labelAsOf)| I[OrgService.GetPosition]
    B --> J[orgui.AssignmentsTimeline (.templ)]
    J --> A
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：行级 labelAsOf 语义（选定）**
  - **选项 A（现状）**：所有行都用页面 `effective_date` 解析 label → 历史语义错误（全部变成最新）。
  - **选项 B**：所有行都用 `row.EffectiveDate` 解析 label → 历史语义正确，但 summary/当前行在页面 as-of 切换时不随之变化（语义不一致）。
  - **选项 C（选定）**：默认用 `row.EffectiveDate`；当页面 `asOf` 落在该行区间内时改用页面 `asOf`（与现有 “current row/canEdit” 判定一致）。
- **决策 2：落点选择（UI 层最小变更）**
  - 先在 `OrgUIController` 的 timeline 组装处修复（局部且可回滚）。
  - 性能/N+1 若出现可观测回归，再升级为“服务端批量解析 label”（联表/批量 API）作为后续计划（不在本计划实现）。
- **决策 3：失败路径不阻断页面渲染（选定）**
  - OrgNode label lookup 失败：fallback `nodeID.String()`。
  - Position label lookup 失败：fallback `row.PositionCode`，再 fallback `positionID.String()`。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划不引入 DB schema 变更；本节只固化“时间切片语义与边界”，避免实现阶段口径漂移。

- **时间区间语义**：
  - 任职行区间采用右开区间：`[row.EffectiveDate, row.EndDate)`。
  - open-ended 以 `9999-12-31` 表示上界（右开）。
- **effective_date 输入语义**：
  - `effective_date` 支持 `YYYY-MM-DD` 或 RFC3339；解析后统一为 `UTC`（见 controllers 包内 `parseEffectiveDate`）。
  - 页面 date input 提供的是 `YYYY-MM-DD`，因此 `pageAsOf` 为当日 `00:00:00Z`。
- **ViewModel（关键字段）**：`modules/org/presentation/viewmodels/assignment.go` 的 `OrgAssignmentRow`：
  - `OrgNodeID/PositionID/PositionCode`
  - `EffectiveDate/EndDate`（用于区间判断与 labelAsOf 选择）

## 5. 接口契约 (API Contracts)
### 5.1 页面路由：`GET /org/assignments`
- **Query 参数**：
  - `effective_date`：可选；无效时页面返回 `400` 并渲染错误提示，同时 fallback 为 `time.Now().UTC()` 渲染页面。
  - `pernr`：可选；为空时 timeline 为空（页面仍可访问）。
- **Response**：HTML 页面（或在 HTMX 场景返回 HTML partial，见 §5.2）。

### 5.2 HTMX 交互 (UI Partials)
- **切换 effective_date（日期输入框）**
  - Request：`GET /org/assignments?effective_date=YYYY-MM-DD&pernr=...`
  - HTMX：`hx-target=#org-assignments-page` + `hx-select=#org-assignments-page` + `hx-swap=outerHTML`
  - Contract：只替换 `#org-assignments-page`，避免“壳套壳”。
- **刷新 timeline（刷新按钮）**
  - Request：`GET /org/assignments?effective_date=...&pernr=...`（HTMX target 为 `org-assignments-timeline`）
  - Response：仅返回 `orgui.AssignmentsTimeline(...)` 对应的 HTML 片段（供 `hx-swap=innerHTML` 插入）。
- **timeline 输出语义契约（本计划新增/修正）**
  - 任职时间线每一行的 `OrgNodeLabel/PositionLabel` 必须按 §6 的 `labelAsOfForRow` 算法选择 `asOf` 再解析。
  - label lookup 失败必须按 §3.2 决策 3 fallback，且不得导致页面 500。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 行级 labelAsOf 选择算法（必须与 UI “current row/canEdit” 判定一致）
输入：
- `pageAsOf`：页面 `effective_date` 解析后的 UTC 时间。
- `rowStart = row.EffectiveDate.UTC()`
- `rowEnd = row.EndDate.UTC()`（右开；open-ended 为 `9999-12-31`）

输出：
- `labelAsOf`：用于解析 OrgNode/Position label 的时间点（UTC）。

伪代码：
```go
labelAsOf := rowStart
if !pageAsOf.Before(rowStart) && pageAsOf.Before(rowEnd) {
    labelAsOf = pageAsOf
}
return labelAsOf
```

### 6.2 timeline label 填充规则（实施必须覆盖所有 callsite）
对任职时间线 `timeline.Rows`：
1) `labelAsOf := labelAsOfForRow(row, pageAsOf)`
2) `row.OrgNodeLabel = orgNodeLabelFor(..., asOf=labelAsOf)`
3) `row.PositionLabel = positionLabelFor(..., asOf=labelAsOf, fallbackCode=strings.TrimSpace(row.PositionCode))`

**必须覆盖的 callsite（避免只修一处导致语义漂移）**：
- `OrgUIController.AssignmentsPage`（页面与 timeline partial）
- `OrgUIController.CreateAssignment` 成功后重建 timeline
- `OrgUIController.TransitionAssignment` 成功后重建 timeline
- `OrgUIController.UpdateAssignment` 成功后重建 timeline

## 7. 安全与鉴权 (Security & Authz)
- **页面访问**：`GET /org/assignments` 需通过 UI Authz：`org.assignments:read`；表单/写操作需 `org.assignments:assign`（实现中已存在，本计划不改变策略模型）。
- **数据隔离**：所有 label lookup 均通过 `tenantID` 调用 `OrgService`，不引入跨租户读取路径。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - Org rollout 对该 tenant 已开启（否则路由返回 404）。
  - 测试数据需具备：同一 OrgNode/Position 至少两段切片 + `004` 任职区间覆盖这些时间点。
- **里程碑**：
  1. [ ] 固化本 TDD（本文件）并通过 `make check doc`。
  2. [ ] 实现 §6 的 labelAsOfForRow + 替换所有 callsite。
  3. [ ] 增补最小回归测试（至少覆盖右开边界与 open-ended）。
  4. [ ] 本地验证（主路径 + HTMX 检查）并留证据（截图/trace/console 其一）。
  5. [ ] 按 SSOT 跑 Go 门禁并记录。

## 9. 测试与验收标准 (Acceptance Criteria)
> 对齐 `docs/dev-plans/044-frontend-ui-verification-playbook.md` 的“可复现路径 + 证据”要求；本变更不涉及 CSS/布局，但涉及“历史语义正确性”。

**主路径（必须可复现）**
- 在同一个 OrgNode/Position 上创建至少两段历史切片（重命名或 title 调整），并确保工号 `004` 的任职时间线覆盖这些时间点。
- 打开 `http://localhost:3200/org/assignments?pernr=004&effective_date=2025-12-01`：
  - 历史任职区间的“部门/职位”应显示对应时间切片的名称（不再全部等于最新名称）。
- 切换页面 `effective_date`（日期输入框触发 HTMX 更新）：
  - `effective_date` 落在某一行区间内时，该行 label 应随 `effective_date` 改变（其余行仍按各自起始时间渲染）。

**边界/失败模式（至少覆盖）**
- `effective_date == row.EffectiveDate`：该行按 `effective_date` 渲染（左闭）。
- `effective_date == row.EndDate`：该行不应被视为 current row（右开）。
- label lookup 失败：页面不应 500；OrgNode/Position 应回退到 ID/code（见 §3.2 决策 3）。

**HTMX 专项检查（如适用）**
- `effective_date` 变更时：`hx-target/#org-assignments-page` 与 `hx-select/#org-assignments-page` 只替换预期 fragment（避免“壳套壳”）。
- 异常态（4xx/5xx）仍能在可见区域呈现可理解的错误提示（不应 swap 成不可恢复空白）。

**测试建议（最小集合）**
- 单元测试：覆盖 `labelAsOfForRow` 的 4 个边界（before/start/inside/end）+ open-ended 行为。

**证据（最小集）**
- 至少 1 张截图：展示同一时间线多行出现不同 OrgNode/Position label（历史语义已修复）。
- 如出现争议或环境差异：补充 DevTools Network/Console 片段或 Playwright trace（二选一即可）。

## 10. 运维与监控 (Ops & Monitoring)
- **Feature Flag**：无新增开关；行为修正为默认行为。
- **可观测性**：不新增指标；如出现性能回归，优先用现有 request_id + DB/trace 手段定位是否由 per-row label lookup 放大导致。
- **回滚方案**：
  - 代码回滚：`git revert` 回滚该变更 PR/commit。
  - 数据回滚：无数据变更，不涉及 schema 回滚。

## 11. DEV-PLAN-044 评审（UI 验收可执行性）
- 结论：通过（实施阶段需按 §9 留下可复现证据，并做 HTMX 局部更新检查）。
- 风险提示：
  - 仍存在 N+1（每行 label lookup）；若 timeline 行数扩大或出现性能回归，升级为 §3.2 决策 2 的后续方案处理。

## 12. DEV-PLAN-045 评审（Simple > Easy）
### 结构（解耦/边界）
- [x] 变更局部：仅调整“时间线行 label 的 asOf 选择”，不引入新 service/新模型。
- [x] 单一权威表达：以 `labelAsOfForRow` 作为唯一入口，避免散落多处条件分支。

### 演化（规格/确定性）
- [x] 有对应 Spec（本 dev-plan），包含边界/失败模式/验收与回滚口径。
- [ ] 若实施阶段发现“必须下沉到 service 才能满足性能/一致性”，先更新本计划再改实现（避免 Vibe Coding）。

### 认知（本质/偶然复杂度）
- [x] 复杂逻辑对应明确不变量：历史行按历史切片显示；as-of 落在行区间内时按 as-of 显示（右开区间）。

### 维护（可理解/可解释）
- [x] 5 分钟可解释：选 `labelAsOf` → 计算 label → 渲染与 fallback。

结论：通过（建议：实现时抽出 `labelAsOfForRow` helper 并复用到所有 callsite，避免未来复制粘贴漂移）。
