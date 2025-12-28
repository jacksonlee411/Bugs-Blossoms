# DEV-PLAN-063A：任职经历列表新增“组织长名称”列（TDD）

**状态**: 草拟中（2025-12-28 04:13 UTC）
**对齐更新**：
- 2025-12-28：对齐 DEV-PLAN-064A（已完成）结论：Valid Time 对外仅 `YYYY-MM-DD`；区间语义为 day 粒度闭区间；本文不再使用/依赖 end-exclusive 口径。

## 1. 背景与上下文 (Context)
- **需求来源**：063A（Org → Assignments 任职经历列表增强）。
- **入口/复现**：`http://localhost:3200/org/assignments?effective_date=2025-12-28&pernr=004`
- **当前痛点**：任职经历表格仅展示“部门”（OrgNode label），在同名部门、跨层级组织、或频繁 Move/Rename 的场景下容易产生歧义（无法快速判断该部门处于哪条组织路径下）。
- **业务价值**：在不改变任职行切片的前提下，为每条任职记录提供“当时的组织长路径快照”，提升 HR 历史核对/审计可读性。

**相关计划/约束**
- 行级 labelAsOf 语义（避免历史语义被页面 as-of 覆盖）：`docs/dev-plans/063-assignment-timeline-org-labels-by-effective-slice.md`
- Valid Time 按天闭区间（SSOT）：`docs/dev-plans/064-effective-date-day-granularity.md`
- `effective_on/end_on` 双轨停止线与收敛路径（SSOT）：`docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`
- “组织长名称”拼接规则与失败兜底（参考）：`docs/dev-plans/065-org-node-details-long-name.md`
- 组织长名称投影（SSOT/复用/N+1 预算）：`docs/dev-plans/068-org-node-long-name-projection.md`

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 在 Org → Assignments 的任职经历列表中，在“部门”列后新增“组织长名称”列。
  - [ ] 每行展示 root→部门 的路径拼接串（分隔符：` / `），并遵循 DEV-PLAN-063 的行级 `labelAsOfDay` 语义（保证“历史切片显示历史路径”）。
  - [ ] 失败路径不阻断渲染：路径查询失败时不返回 500；该列展示 `—`（或空值兜底）。
  - [ ] 不引入持久化字段（不新增 `long_name` 存储列），仅在读时派生。
  - [ ] 对齐 DEV-PLAN-064A 停止线：不新增/扩散 `effective_on/end_on`；Valid Time 对外只使用 `YYYY-MM-DD`。
- **非目标 (Out of Scope)**：
  - 不新增/调整 DB 表结构与迁移。
  - 不新增新的 HTTP API endpoint（复用 OrgService 的既有能力）。
  - 不改变任职时间线“行拆分/合并”规则（本计划只扩展展示列）。
  - 不实现“同一条任职行内自动呈现有效期内部的组织路径变化”（例如部门更名/上级变更导致路径变化）；此类场景通过切换页面 `effective_date` 查看该任职行在指定日期的路径快照（见 §6.3）。

## 2.1 工具链与门禁（SSOT 引用）
> **目的**：避免在 dev-plan 里复制工具链细节导致 drift；本文只声明“本计划命中哪些触发器”，命令细节以 SSOT 为准。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码（见 `AGENTS.md`）
  - [ ] `.templ` / Tailwind（见 `AGENTS.md`；生成物需提交）
  - [ ] 多语言 JSON（见 `AGENTS.md`）
  - [X] 文档（本计划）：已执行 `make check doc`（docs gate: OK，2025-12-28 02:51 UTC；docs gate: no new files detected，2025-12-28 04:00 UTC；2025-12-28 04:13 UTC）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
    A[Browser / HTMX] -->|GET /org/assignments?effective_date&pernr| B[OrgUIController.AssignmentsPage]
    B -->|ListAssignmentsTimeline| C[OrgService]
    C --> D[OrgRepository]
    D --> E[(DB)]
    B --> F[mappers.AssignmentsToTimeline]
    F --> G[timeline.Rows]
    B --> H[labelAsOfDayForRow(row, pageAsOfDay)]
    B --> I[pkg/orglabels.ResolveOrgNodeLongNames (pair-batch)]
    I --> E
    B --> J[row.OrgNodeLongName]
    B --> K[orgui.AssignmentsTimeline (.templ)]
    K --> A
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：长路径获取策略（选定：Controller 侧批量投影，SSOT=DEV-PLAN-068）**
  - 选项 A：在 `.templ` 内逐行调用（不可控、难测、易形成隐式 N+1）。
  - 选项 B：在 controller 里对每行调用 `OrgService.GetNodePath(asOf)` + request-scope cache（仍是典型 N+1 形态；且难给出明确 query budget 上界）。
  - 选项 C（选定）：在 `OrgUIController` 内先收集 `(org_node_id, labelAsOfDay)` 对，调用 `pkg/orglabels.ResolveOrgNodeLongNames` 一次性批量 hydrate（query budget 常数；复用 SSOT 能力，避免重复实现）。
- **决策 2：行级 as-of 语义（选定：对齐 DEV-PLAN-063）**
  - 选项 A：全表使用页面 `effective_date` → 历史行会漂移为“最新路径”。
  - 选项 B：全表使用行 `EffectiveDate` → 历史语义稳定，但当前行在页面切换时不会反映当日路径变化。
  - 选项 C（选定）：当页面 `effective_date` 落在该行有效期内，用页面 `effective_date`；否则用行起始日（既保留历史稳定性，又支持“在有效期内查看当日快照”）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 ViewModel 扩展
- 在 `modules/org/presentation/viewmodels/assignment.go` 的 `OrgAssignmentRow` 增加字段：
  - `OrgNodeLongName string`：组织长名称（默认空字符串，仅展示用）。

### 4.2 DB / Schema
- 无 DB 变更、无迁移、无新索引/约束（长名称为读时派生；投影能力 SSOT 见 DEV-PLAN-068）。

### 4.3 Valid Time 表达（对齐 064/064A）
- 页面输入的 Valid Time 仅使用 `effective_date=YYYY-MM-DD`（day）。
- 不在运行时代码中新增/扩散 `effective_on/end_on` 引用；不将 Valid Time 回流为 `timestamptz` 语义（停止线见 `docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`）。

## 5. 接口契约 (UI Contracts)
### 5.1 页面：`GET /org/assignments`
- **Query**：
  - `effective_date`: `YYYY-MM-DD`
  - `pernr`: 字符串（示例：`004`）
- **Response**：
  - 返回 HTML（templ 渲染），任职经历表格新增一列“组织长名称”。

### 5.2 HTMX 局部更新
- 页面已存在基于 `effective_date` 变更与“刷新时间线”的局部更新；本计划不新增新 endpoint，仅确保返回的 timeine HTML 中包含新列与新值（含失败兜底 `—`）。

### 5.3 i18n
- 新增表头 key：`Org.UI.Assignments.Table.OrgNodeLongName`（`modules/org/presentation/locales/en.json`/`zh.json`）。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 `labelAsOfDay` 选择（对齐 DEV-PLAN-063）
定义（伪代码）：
```go
pageAsOfDay := normalizeValidTimeDayUTC(parseYYYYMMDD(query.effective_date))
rowStartDay := normalizeValidTimeDayUTC(row.EffectiveDate)
rowEndDay := normalizeValidTimeDayUTC(row.EndDate) // day 闭区间：inclusive end_date（对齐 064/064A）

labelAsOfDay := rowStartDay
if !pageAsOfDay.Before(rowStartDay) && !pageAsOfDay.After(rowEndDay) {
    labelAsOfDay = pageAsOfDay
}
```
注：若该任职行为 open-ended（例如 `end_date` 为 NULL / “至今”），则将 `rowEndDay` 视为无限远，`pageAsOfDay >= rowStartDay` 即命中“区间内”。

### 6.2 `OrgNodeLongName` 计算（批量投影；不做 per-row 查询）
```go
queries := []orglabels.OrgNodeLongNameQuery{}
for _, row := range timeline.Rows {
    if row.OrgNodeID == nil { continue }
    labelAsOfDay := labelAsOfDayForRow(row, pageAsOfDay) // DEV-PLAN-063
    queries = append(queries, orglabels.OrgNodeLongNameQuery{
        TenantID:  tenantID,
        OrgNodeID: *row.OrgNodeID,
        AsOfDay:   labelAsOfDay, // day-only
    })
}

names, err := orglabels.ResolveOrgNodeLongNames(ctx, queries)
if err != nil {
    // 失败兜底：不 500；该列展示 "—"
}

for i := range timeline.Rows {
    row := &timeline.Rows[i]
    if row.OrgNodeID == nil { continue }
    labelAsOfDay := labelAsOfDayForRow(*row, pageAsOfDay)
    key := orglabels.OrgNodeLongNameKey{TenantID: tenantID, OrgNodeID: *row.OrgNodeID, AsOfDay: labelAsOfDay}
    row.OrgNodeLongName = strings.TrimSpace(names[key])
}
```

### 6.3 示例（不拆分任职行，只看快照）
假设工号 `004` 有一条任职行：`2025-12-01 → 2025-12-31`，部门节点 `A`；且：
- `2025-12-15` 起部门更名（name 变化）
- `2025-12-20` 起上级部门调整（路径变化）

则访问：
- `effective_date=2025-12-10`：该行显示 **2025-12-10** 的路径快照（更名前/调整前）。
- `effective_date=2025-12-28`：同一行显示 **2025-12-28** 的路径快照（更名后/调整后）。
- `effective_date=2026-01-10`（不在该行有效期内）：该行回退为按 **行起始日 2025-12-01** 的路径快照，避免历史行漂移为“最新路径”。

## 7. 安全与鉴权 (Security & Authz)
- 不新增/修改 Casbin policy。
- 页面仍遵循既有 UI 权限：读取为 `org.assignments:read`；写操作为 `org.assignments:assign`（本计划只增加展示列）。
- 数据隔离：长名称投影查询必须带 `tenantID`（调用 `pkg/orglabels`）；不得引入跨租户读取路径。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - 组织长名称投影能力（DEV-PLAN-068）已落地并可复用：`pkg/orglabels`。
  - 行级 as-of 语义以 `docs/dev-plans/063-assignment-timeline-org-labels-by-effective-slice.md` 为准。
  - Valid Time 停止线以 `docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md` 为准。
- **里程碑**：
  1. [ ] i18n：新增 `Org.UI.Assignments.Table.OrgNodeLongName`，并通过 `make check tr`。
  2. [ ] ViewModel：扩展 `OrgAssignmentRow`，并补齐 mapper 初始化。
  3. [ ] Controller：hydrate `OrgNodeLongName`（行级 `labelAsOfDay` + `pkg/orglabels` 批量投影 + 失败兜底）。
  4. [ ] Templ：更新 `assignments.templ` 表头与单元格；执行 `make generate && make css` 并提交生成物。
  5. [ ] 验证与留证：按 DEV-PLAN-044 本地复现并截图（至少 1 张展示新增列与值）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **主路径（必须）**：
  - [ ] 打开 `http://localhost:3200/org/assignments?effective_date=2025-12-28&pernr=004`：
    - “部门”列之后出现“组织长名称”列；
    - 每行展示对应部门的 root→self 路径串（以 ` / ` 分隔）。
  - [ ] 切换页面 `effective_date`：
    - 当 `effective_date` 落在某一行有效期内，该行“组织长名称”随 `effective_date` 变化；
    - 其余历史行保持各自“行起始日”对应的路径快照（避免全表变成同一条“最新路径”）。
- **边界/失败路径（至少覆盖）**：
  - [ ] 路径查询失败/缺失时页面不 500：该列显示 `—`，其他列正常渲染。
  - [ ] 准备一个“部门在任职有效期内部发生更名/上级变更”的样例：在不新增任职行的前提下，切换 `effective_date` 跨越变更日，观察同一行的路径快照切换。

## 10. 运维与监控 (Ops & Monitoring)
- 不引入 Feature Flag/灰度/监控项（仓库级原则见 `AGENTS.md`）。
- **回滚方案**：
  - 代码回滚：`git revert` 回滚对应变更。
  - 数据回滚：无数据变更。

## 11. DEV-PLAN-045 评审（Simple > Easy）
### 结构（解耦/边界）
- [x] 变更局部：仅新增“组织长名称”展示列与对应 hydrate，不引入新 service/新 API/新持久化字段。
- [x] 边界清晰：hydrate 仍集中在 `OrgUIController`，但长名称解析复用 `pkg/orglabels`（DEV-PLAN-068，SSOT），避免 per-row service 调用与模板内隐式查询。
- [x] 单一权威表达：Valid Time 对外只使用 `effective_date=YYYY-MM-DD`；对齐 064A 停止线，不新增/扩散 `effective_on/end_on`。

### 演化（规格/确定性）
- [x] Spec 可执行：已按 001 模板补齐架构图/ADR/算法/契约/验收标准，实施不需要再“对话式补丁”。
- [x] 性能策略前置：明确采用 `pkg/orglabels` 的 pair-batch 单 SQL 投影，避免 N+1；如后续需要更强约束（例如 query budget 证明/trace 证据），应先更新本计划再改实现。

### 认知（本质/偶然复杂度）
- [x] 复杂度对应明确不变量：每条任职行展示“当时路径快照”，并允许通过切换页面 `effective_date` 在有效期内查看指定日期快照（但不拆分任职行）。
- [x] 偶然复杂度隔离：与 064/064A 的迁移期字段/语义仅通过“停止线”约束进入本计划，不把双轨映射规则扩散为新概念。

### 维护（可理解/可解释）
- [x] 5 分钟可解释：计算 `labelAsOfDay` → 批量投影 `ResolveOrgNodeLongNames` → 渲染/兜底；失败路径明确且不影响其他列。

结论：通过（建议：实现时抽出 `labelAsOfDayForRow` 小 helper 并复用，避免未来复制粘贴漂移）。 
