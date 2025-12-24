# DEV-PLAN-037：Org 模块页面交互问题调查与改进建议

**状态**: 草拟中（2025-12-24 11:08 UTC）

> 本文档定位：对当前 Org UI（`/org/nodes`、`/org/positions`、`/org/assignments`）的可用性问题做调查与根因分析，并给出可执行的改进建议与验收标准。
>
> SSOT：Org UI 的既有可编码契约以 `docs/dev-plans/035-org-ui.md` 为准；IA/侧栏集成参考 `docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`。

## 1. 背景与上下文 (Context)
- 当前 Org UI 已交付 M1/M2/M3 主链（见 `DEV-PLAN-035`），但在实际使用中出现多处影响操作的 UX 问题，导致 HR/组织管理员无法顺畅完成“组织结构维护 + 任职分配”。
- 用户反馈的主要问题集中在两类：
  1) **有效期（effective dating）输入/切换体验**：创建时无法输入生效日期；切换生效日期会错误嵌套渲染整页。
  2) **表单可用性与视觉对比**：i18n 字段要求手写 JSON；Tab 与日期框在浅色主题下不可读。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 明确每个问题的**现象 → 根因 → 影响面**（以代码证据为准，避免“猜 UI”）。
- [ ] 给出**可落地的修复路径**（优先最小改动、可渐进发布）。
- [ ] 明确“人员分配”页面在 Org vs Person 的归属与关系，减少信息架构混乱。

### 2.2 非目标
- 不在本计划内直接引入新的 Org 业务能力/数据契约（例如新增复杂审批流、批量调整等）。
- 不强制做大规模 UI 重构；优先修复阻塞性可用性问题（能操作、能看清、不会嵌套错乱）。

### 2.3 工具链与门禁（SSOT 引用）
- 触发器矩阵与本地必跑：`AGENTS.md`
- `.templ`/Tailwind：`make generate && make css`，并确保生成物提交（SSOT：`AGENTS.md`）
- Go：`go fmt ./... && go vet ./... && make check lint && make test`（SSOT：`AGENTS.md`）
- 文档门禁：`make check doc`（SSOT：`AGENTS.md`）

## 3. 调查范围 (Scope)
### 3.1 页面/入口
- Org：
  - `/org/nodes`（组织结构）
  - `/org/positions`（职位）
  - `/org/assignments`（人员分配/任职）
- Person：
  - `modules/person/presentation/templates/pages/persons/detail.templ` 中嵌入的“任职时间线/创建任职”区域（调用 Org 的 `/org/assignments*` UI endpoints）

### 3.2 关键概念（对齐 035）
- `effective_date`：as-of 查询点（以及写入 insert slice 的生效日期）；当前 UI 通过 query 参数 `effective_date=YYYY-MM-DD` 传递。
- `i18n_names`：节点多语言名称（当前 UI 让用户输入 JSON 字符串）。

## 4. 问题发现与根因分析

### 4.1 创建新部门时没有地方填写生效日期
**现象**
- “新建部门/OrgNode”表单没有可编辑的生效日期字段，用户无法指定该部门从哪一天开始生效。

**代码证据**
- `modules/org/presentation/templates/components/orgui/node_forms.templ`：表单仅包含隐藏字段 `<input type="hidden" name="effective_date" value={ props.EffectiveDate }/>`，没有可编辑的 date 控件。

**当前行为推断**
- 创建/编辑的生效日期默认使用“页面右上角 effective_date”作为写入生效日期；用户若要创建 future-dated slice，必须先切换页面 effective_date（但该切换目前存在嵌套渲染问题，见 4.3）。

**不合理点**
- “查询 as-of 日期” 与 “写入生效日期”被耦合到同一个全局输入，且缺少显式提示，用户心智模型容易误解（尤其是 HR 场景需要明确“本次变更从哪天生效”）。

### 4.2 i18n 名称要求手写 JSON，格式对用户不友好
**现象**
- 节点表单要求填写“多语言名称（JSON）”，普通业务用户难以理解 JSON 格式、容易输错（引号/逗号/花括号等）。

**代码证据**
- `modules/org/presentation/templates/components/orgui/node_forms.templ`：`i18n_names` 使用 `textarea` + JSON placeholder（`"{\"en\":\"Name\"}"`）。

**不合理点**
- UX 不符合“表单输入应可视化”的常识；且该字段往往是“可选增强”，不应阻塞主要流程。

### 4.3 切换右上角“生效日期”会嵌套生成重复页面（含左右导航栏）
**现象**
- 在 Org 页面点击右上角 `effective_date` 选择新日期后，页面内容被重复嵌套渲染（包括左右导航栏/布局壳），越点越深。

**根因（代码级）**
- Org 页面 `effective_date` 输入使用 HTMX 把 **整页响应** swap 到页面内某个 div 中：
  - `modules/org/presentation/templates/pages/org/nodes.templ`：
    - `hx-get="/org/nodes"`
    - `hx-target="#org-nodes-page"`
    - `hx-swap="outerHTML"`
  - `modules/org/presentation/templates/pages/org/assignments.templ` / `positions.templ` 同模式。
- 但后端 handler 始终渲染“带 Authenticated layout 的整页模板”：
  - `modules/org/presentation/controllers/org_ui_controller.go`：
    - `NodesPage` → `orgtemplates.NodesPage(...)`
    - `AssignmentsPage` → `orgtemplates.AssignmentsPage(...)`
    - `PositionsPage` → `orgtemplates.PositionsPage(...)`

因此：HTMX 将包含 `layouts.Authenticated(...)` 的完整页面 HTML 注入到 `#org-*-page` div 中，造成“壳套壳”的嵌套。

**不合理点**
- 该问题属于“交互阻断级 bug”：一旦用户频繁切换日期，DOM 结构失控，后续交互/样式/性能都会异常。

### 4.4 非活动 Tab 背景黑色导致看不见内容
**现象**
- Org 页面的二级 Tab（结构/分配/职位）中，非活动 Tab 背景为黑色且文字对比不足，导致不可读。

**代码证据**
- `modules/org/presentation/templates/components/orgui/subnav.templ`：非 active 使用 `bg-surface-200 text-200`。
- `modules/core/presentation/assets/css/main.css`（浅色主题 `:root`）：
  - `--clr-surface-200: var(--black);`
  - `--clr-text-200: var(--gray-600);`（深色文本）

**根因（设计令牌误用）**
- `surface-200` 在浅色主题里等于黑色（似乎用于侧栏/暗面板），但 Org Tab 在主内容区仍复用该 token，且文字使用深色 token → 对比失败。

### 4.5 生效日期输入框背景黑色导致看不见内容
**现象**
- Org 页面的右上角 `effective_date` date input 背景黑色，且文字/placeholder 对比不足，导致不可读。

**代码证据**
- `modules/org/presentation/templates/pages/org/nodes.templ` / `assignments.templ` / `positions.templ`：
  - `class="... bg-surface-200 ... text-100"`
- `modules/core/presentation/assets/css/main.css`（浅色主题 `:root`）：
  - `--clr-surface-200: var(--black);`
  - `--clr-text-100: var(--black);`

=> 直接造成“黑底黑字”。

### 4.6 “人员分配”页面出现在 Org 是否合理？与 Person 模块的分配是什么关系？
**现状（代码事实）**
- Org 模块存在独立页面：`/org/assignments`（`modules/org/presentation/controllers/org_ui_controller.go: AssignmentsPage` + `modules/org/presentation/templates/pages/org/assignments.templ`）。
- Person 模块的人员详情页中，已嵌入“任职时间线/创建任职”区域：
  - `modules/person/presentation/templates/pages/persons/detail.templ` 会通过 HTMX 调用 Org 的 UI endpoints：
    - `GET /org/assignments/form?...&include_summary=1`
    - `GET /org/assignments?...&include_summary=1`

**关系结论**
- Person 模块并没有实现一套“独立的分配领域”；当前“分配/任职”本质上属于 Org 域（Assignment 连接 person ↔ org_node/position）。
- Person 详情页只是“以人作为上下文”的入口，把 Org Assignment 的 UI 以组件/partial 的方式嵌入（更贴近 HR 的日常操作：从人出发做任职管理）。
- `/org/assignments` 则是“以组织/岗位作为工作台”的入口，适合：
  - 快速按 `pernr` 查询并维护任职
  - 进行组织侧联调/回归（与 Org 节点/职位同处一个模块）

**潜在不合理点（信息架构）**
- 两个入口并存但缺少“明确的导航语义/互链”，容易让用户困惑：
  - “我应该在 Org 里分配，还是在 Person 里分配？”
  - 两边是否同一数据？是否会出现不一致？

## 5. 改进建议（可执行方案）

### 5.1 生效日期：拆分“查询 as-of”与“写入生效”
建议方案（择一或组合）：
- 方案 A（最小改动）：在“新建/编辑节点”表单内新增可编辑 `effective_date` 字段，默认值为页面 effective_date，并在字段旁提示其语义（“本次变更从该日期起生效”）。
- 方案 B（次优但成本低）：保持表单不新增字段，但在表单顶部显式说明“本次创建/编辑将使用页面右上角生效日期”，并提供“修改生效日期”锚点跳转/聚焦到右上角日期控件。

推荐：方案 A（HR 心智更清晰；避免“全局日期”隐式耦合导致误操作）。

### 5.2 i18n_names：从 JSON 文本改为可视化多语言输入
建议方案：
- 复用现有 `components/multilang/form_input.templ` 的交互模型（多行 locale/value + hidden JSON），为 Org Node 的 `i18n_names` 提供可视化输入。
- 约定：`name` 仍为主显示名；`i18n_names` 可选补充，若为空则不阻塞创建。

### 5.3 修复 effective_date 切换导致“壳套壳”的 HTMX 行为
建议方案（优先最小改动）：
- 方案 A（推荐）：在 Org 页面的 date input 上加 `hx-select`，只从整页响应中抽取 `#org-*-page` 片段进行 swap。
  - 例如在 `nodes.templ` 上追加：`hx-select="#org-nodes-page"`
- 方案 B：后端检测 HTMX 请求（`HX-Request`）时改为渲染“无 layout 的 page partial”（新建 `NodesPagePartial` 等模板），避免返回整页。

推荐：方案 A（无需改 controller/render 结构，风险低；后续可再做方案 B 收敛架构）。

### 5.4 修复 Tab 与日期输入在浅色主题的可读性
建议方案：
- 方案 A（推荐）：Org 主内容区避免使用 `bg-surface-200`（该 token 在浅色主题里是黑色，偏向侧栏/暗面板）；改用 `bg-surface-300` 或 `bg-surface-100`（白/浅灰底），并匹配 `text-100/text-200`。
  - 目标文件：`modules/org/presentation/templates/components/orgui/subnav.templ`、`modules/org/presentation/templates/pages/org/*.templ`
- 方案 B：调整设计 token：把浅色主题的 `--clr-surface-200` 改为浅色（例如 `--gray-50/100`），并检查对侧栏/Spotlight 等既有暗面板的影响。

推荐：方案 A（改动面更可控；避免全局 token 改动引发连锁 UI 变化）。

### 5.5 “人员分配”在 Org vs Person：澄清定位 + 增加互链
建议：
- 明确定位（文案/导航）：`/org/assignments` 属于“组织侧工作台”，Person 详情页为“人员侧入口”。
- 在 Person 详情的任职区域增加“打开完整分配页”的链接（携带 `pernr` + `effective_date`），在 Org 分配页也提供“打开人员详情”的链接（携带 `pernr`），形成双向可达。
- 如未来要做侧栏二级导航（见 035A §6.2/§6.3），可将“人员分配”作为 Org 的子项展示，避免入口隐藏导致用户只能靠偶然发现。

## 6. 验收标准 (Acceptance Criteria)
- 生效日期：
  - [ ] 新建/编辑节点时，用户可明确输入/确认“本次变更生效日期”，且不会被隐藏的全局状态误导。
- i18n：
  - [ ] `i18n_names` 不再要求手写 JSON；普通用户可通过多语言输入控件完成填写，且输入错误不会破坏整页提交体验。
- HTMX：
  - [ ] 切换 Org 页面的 `effective_date` 不会产生 layout 嵌套；多次切换后 DOM 结构保持稳定。
- 样式：
  - [ ] 浅色主题下，非活动 Tab 与 `effective_date` 输入框可读（对比度足够），不出现黑底黑字。
- IA：
  - [ ] 文档/页面上明确 “Org 分配页” 与 “Person 详情分配区”是同一套数据/能力的不同入口，并提供互链。

## 7. 待办清单（实施任务草案）
1. [ ] 为 Org 三个页面的 date input 增加 `hx-select`（或切到 partial 渲染），并补充回归用例。
2. [ ] 调整 Org subnav 与 effective_date 输入的背景/文字 token 使用（浅色主题可读）。
3. [ ] 为 Org Node 表单增加可编辑的“生效日期”字段（并明确语义文案）。
4. [ ] 将 `i18n_names` 替换为可视化多语言输入（复用/抽取组件）。
5. [ ] 补充 Person ↔ Org 分配页互链与定位说明（文案/i18n keys）。

## 8. 附录：关键证据索引
- Org 页面 effective date（HTMX swap 全页）：  
  - `modules/org/presentation/templates/pages/org/nodes.templ`  
  - `modules/org/presentation/templates/pages/org/assignments.templ`  
  - `modules/org/presentation/templates/pages/org/positions.templ`
- Org 页面 handler（始终渲染带 layout 的整页）：  
  - `modules/org/presentation/controllers/org_ui_controller.go`（`NodesPage` / `AssignmentsPage` / `PositionsPage`）
- Org Node 表单（无可编辑 effective date + i18n JSON textarea）：  
  - `modules/org/presentation/templates/components/orgui/node_forms.templ`
- Org Subnav（非 active tab 使用 `bg-surface-200 text-200`）：  
  - `modules/org/presentation/templates/components/orgui/subnav.templ`
- 设计 token（浅色主题 surface/text 定义）：  
  - `modules/core/presentation/assets/css/main.css`
- Person 详情页嵌入 Org 分配 UI：  
  - `modules/person/presentation/templates/pages/persons/detail.templ`

