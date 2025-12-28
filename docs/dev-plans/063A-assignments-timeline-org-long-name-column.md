# DEV-PLAN-063A：任职经历列表新增“组织长名称”列（TDD）

**状态**: 草拟中（2025-12-28 04:00 UTC）

## 1. 背景与上下文 (Context)
- 入口页面：`/org/assignments?effective_date=2025-12-28&pernr=004`
- 当前“任职经历/时间线”表格仅展示“部门”（OrgNode label），在同名部门、跨层级组织、或频繁 Move/Rename 的场景下容易产生歧义。
- Org 模块已具备“节点路径查询”（root→node）能力，并在 DEV-PLAN-065 中将其用于详情面板的“组织长名称”展示；本计划将该能力复用到任职经历列表中。

**相关计划/约束**
- 行级 labelAsOf 语义（避免历史语义被页面 as-of 覆盖）：`docs/dev-plans/063-assignment-timeline-org-labels-by-effective-slice.md`
- Valid Time 按天闭区间：`docs/dev-plans/064-effective-date-day-granularity.md`
- `effective_on/end_on` 双轨停止线与收敛路径：`docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`
- “组织长名称”拼接规则与失败兜底：`docs/dev-plans/065-org-node-details-long-name.md`

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] 在 Org → Assignments 的任职经历列表中，在“部门”列后新增“组织长名称”列。
- [ ] 每行展示 root→部门 的路径拼接串（分隔符：` / `），并遵循 DEV-PLAN-063 的 `labelAsOf` 选择语义（默认行起始日；当页面 `effective_date` 落在该行有效期闭区间内时使用页面 `effective_date`）。
- [ ] 失败路径不阻断渲染：路径查询失败时不返回 500；该列展示 `—`（或空值兜底）。
- [ ] 不引入持久化字段（不新增 `long_name` 存储列），仅在读时派生。

### 2.2 非目标 (Out of Scope)
- 不新增/调整 DB 表结构与迁移。
- 不新增新的 HTTP API endpoint（复用 OrgService 的内部方法/已有能力）。
- 不改变任职时间线“行拆分/合并”规则（本计划只扩展展示列）。
- 不实现“同一条任职行内自动呈现有效期内部的组织路径变化”（例如部门更名/上级变更导致路径变化）；此类场景通过切换页面 `effective_date` 查看该任职行在指定日期的路径快照（见 §3.4）。

## 2.3 工具链与门禁（SSOT 引用）
> 命令细节以 SSOT 为准；本文只声明触发器。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码：见 `AGENTS.md`
  - [ ] `.templ` / Tailwind：见 `AGENTS.md`（生成物需提交）
  - [ ] 多语言 JSON：见 `AGENTS.md`
  - [X] 文档（本计划）：已执行 `make check doc`（docs gate: OK，2025-12-28 02:51 UTC；2025-12-28 03:20 UTC；2025-12-28 03:26 UTC；2025-12-28 04:00 UTC）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 方案概述 (Approach)
### 3.1 展示定义
- **列名**：`组织长名称`
- **列值含义**：以 root→self 的路径顺序拼接部门节点的 as-of 名称串（优先 name，其次 code，再次 id），分隔符为 ` / `。

### 3.2 数据来源与性能策略
- 数据来源：复用现有 `OrgService.GetNodePath(...)`（与 DEV-PLAN-065 同源），并在 UI 层按 as-of 拼接得到 long name。
- 性能策略（避免不必要放大）：
  - 以“请求内缓存（request-scope map）”去重 `GetNodePath` 调用，key 至少包含 `(org_node_id, labelAsOfDay)`；
  - 不引入跨请求的全局缓存（避免无 TTL/容量上限的内存与一致性问题）。

### 3.3 行级 as-of 规则（必须）
> 本节为 063A 的关键语义：确保“组织长名称”是**任职记录切片在当时的路径快照**，而不是“当前最新路径”。

- **`labelAsOfDay` 选择**（对齐 DEV-PLAN-063/064）：
  - 令 `pageAsOfDay` 为页面查询参数 `effective_date`（按 UTC day 归一化）。
  - 令 `rowStartDay` 为该任职行的 `EffectiveDate`（按 UTC day 归一化）。
  - 若 `pageAsOfDay` 落在该任职行的有效期闭区间内，则 `labelAsOfDay = pageAsOfDay`；否则 `labelAsOfDay = rowStartDay`。
- **组织长名称计算**（复用 DEV-PLAN-065 的拼接规则）：
  - 对每个任职行，调用 `OrgService.GetNodePath(..., org_node_id, asOf=labelAsOfDay)` 获取 root→node 的路径节点数组；
  - 将每个节点按 `name → code → id` 的回退规则取“显示名”，用 ` / ` 拼接为 `OrgNodeLongName`。
- **更名/上级变更的正确性来源**：
  - 因为 `GetNodePath(asOf)` 同时按 `asOf` 选择“当时的祖先链路”和“当时的节点名称切片”，所以部门更名与上级变更在不同 `labelAsOfDay` 下会得到不同长路径描述。
- **不拆分任职行（但可看不同日期快照）**：
  - 本计划不因部门更名/上级变更而拆分任职行；同一任职行在页面切换 `effective_date` 时，其“组织长名称”可随 `effective_date`（当且仅当 `effective_date` 落在该行有效期内）显示对应日期的路径快照；不要求在同一页面同时展示该行有效期内的多段路径变化。
- **请求内去重缓存（避免串味）**：
  - 仅做 request-scope 缓存；cache key 必须包含 `(org_node_id, labelAsOfDay)`，确保同一部门在不同日期下的路径不会互相覆盖。
- **失败兜底**：
  - 若路径查询失败或返回空路径，则 `OrgNodeLongName` 置空，由模板渲染为 `—`，且不影响页面其它列。
- **对齐 DEV-PLAN-064A 停止线（必须遵守）**：
  - 不在 Domain/Service/Presentation 引入/新增 `timestamptz` 作为 Valid Time 的输入/输出/判断依据；本计划对外只使用 `YYYY-MM-DD`（页面 `effective_date`）表达 Valid Time。
  - 在 064 阶段 D 合并前：本计划实现不得新增任何 `effective_on/end_on` 引用（运行时代码/SQL/Schema SSOT）；仅复用既有 `GetNodePath(asOf)` 与 day 口径 helper。
  - 在 064 阶段 D 合并后：本计划实现应天然满足“运行时代码中无 `effective_on/end_on` 残留”的验收要求（若依赖代码发生重命名/删列，应随 064D 同步调整）。

### 3.4 行级 as-of 语义示例（不拆分任职行）
假设工号 `004` 有一条任职记录行：
- 生效区间：`2025-12-01 → 2025-12-31`
- 部门（OrgNode）：节点 `A`

并且部门 `A` 在该区间内部发生了历史变化：
- `2025-12-15` 起部门更名（`name` 变化）
- `2025-12-20` 起上级部门调整（路径变化）

则该任职行的“组织长名称”显示为：
- 页面 `effective_date=2025-12-10`：该行显示 **2025-12-10** 当天的长路径（更名前/调整前）。
- 页面 `effective_date=2025-12-28`：同一条任职行仍是一行，但显示 **2025-12-28** 当天的长路径（更名后/调整后）。
- 页面 `effective_date=2026-01-10`（不落在该行区间内）：该行回退为按行起始日 **2025-12-01** 计算的长路径，用于稳定呈现历史行（避免所有行都跟随页面 as-of 漂移到“最新”）。

## 4. 契约（UI / ViewModel / i18n）
### 4.1 ViewModel 扩展
- 为任职时间线行结构增加只读字段：
  - `OrgNodeLongName string`：组织长名称（默认空字符串）。

### 4.2 模板渲染
- 在任职经历表格中插入新列：
  - 列位置：紧随“部门”列之后。
  - 值为空时显示：`—`。

### 4.3 多语言 keys
- 新增表头 i18n key：
  - `Org.UI.Assignments.Table.OrgNodeLongName`

## 5. 实施步骤 (Tasks)
1. [ ] i18n：新增 `Org.UI.Assignments.Table.OrgNodeLongName`（`modules/org/presentation/locales/en.json`/`zh.json`），并通过 `make check tr`。
2. [ ] ViewModel：扩展 `modules/org/presentation/viewmodels/assignment.go` 的 `OrgAssignmentRow`，并补齐 mapper 初始化。
3. [ ] Controller：在任职时间线构建/刷新路径中，为每行填充 `OrgNodeLongName`（复用 DEV-PLAN-063 的 labelAsOf 语义 + request-scope 去重）。
4. [ ] Templ：更新 `modules/org/presentation/templates/components/orgui/assignments.templ` 表格列，并执行 `make generate && make css` 确保生成物提交。
5. [ ] 验证与留证：按 DEV-PLAN-044 的口径在本地复现并截图（至少 1 张展示新增列与值）。

## 6. 验收标准 (Acceptance Criteria)
- [ ] 打开 `http://localhost:3200/org/assignments?effective_date=2025-12-28&pernr=004`：
  - “部门”列之后出现“组织长名称”列；
  - 每行展示对应部门的 root→self 路径串（以 ` / ` 分隔）。
- [ ] 切换页面 `effective_date`：
  - 当 `effective_date` 落在某一行的有效期闭区间内，该行“组织长名称”随 `effective_date` 变化（对齐 DEV-PLAN-063 的 current row 语义）。
  - 其余不落在 `effective_date` 的历史行保持各自“行起始日”对应的长路径（避免全表被渲染为同一条“最新路径”）。
- [ ] 准备一个“部门在任职有效期内部发生更名/上级变更”的样例：在不新增任职行的前提下，切换 `effective_date` 跨越变更日，能观察到同一任职行的“组织长名称”随之切换（路径快照语义）。
- [ ] 路径查询失败/缺失时页面不 500：该列显示 `—`，其他列正常渲染。

## 7. 回滚方案 (Rollback)
- [ ] 代码回滚：`git revert` 回滚本变更 PR/commit。
- [ ] 数据回滚：无数据变更。
