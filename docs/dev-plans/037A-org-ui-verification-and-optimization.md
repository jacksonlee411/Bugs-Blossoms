# DEV-PLAN-037A：Org UI 可视化验收与交互验证与优化实施方案（基于 DEV-PLAN-044）

**状态**: 草拟中（2025-12-24 15:43 UTC）

> 本文档定位：按 `docs/dev-plans/044-frontend-ui-verification-playbook.md` 的规范，对 Org 模块 UI（`/org/nodes`、`/org/positions`、`/org/assignments`）进行验证，并将“发现问题 → 解决方案（可编码）→ 测试与验收标准”以 `docs/dev-plans/001-technical-design-template.md` 的技术规格粒度落地。
>
> 关联：
> - Org UI 契约（M1/M2/M3）：`docs/dev-plans/035-org-ui.md`
> - Org UI IA/侧栏集成：`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
> - Org UI 交互问题审计（已完成）：`docs/dev-plans/037-org-ui-ux-audit.md`
> - UI 验收 Playbook（依据）：`docs/dev-plans/044-frontend-ui-verification-playbook.md`

## 1. 背景与上下文 (Context)
- **需求来源**：本次对 Org UI 的 044 验证结果（见“9. 验收标准 / 验证记录”）。
- **当前痛点**（来自 037A 验证与代码审阅）：
  - P1：`/org/nodes` 的父节点信息以 UUID 展示，缺少可读 label 与可回溯导航。
  - P1：页面 as-of（query）与表单写入生效日期（body）同名 `effective_date`，存在语义不清与 UI 状态不同步风险。
  - P1：`/org/assignments` 的 Unauthorized（read 维度）缺少 E2E 覆盖。
  - P2：`/org/positions` 多处直接展示 `LifecycleStatus/StaffingState` 代码值，与 filters 的 i18n label 不一致。
  - P2：`/org/assignments` “刷新”动作为次要但靠近主操作，缺少语义提示。
- **业务价值**：降低 HR/组织管理员在“有效期维护、节点层级理解、职位状态识别、权限边界”上的认知成本，提升可用性与可验证性（证据可复现）。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] `ParentHint/ParentID` 统一显示为可读 label（`Name (Code)`），并提供“回到父节点”的可点击导航（NodePanel + Tree 同步）。
  - [ ] 明确 `effective_date` 的**契约与优先级**：写操作以 **form body** 的 `effective_date` 为准；成功后同步页面 as-of 控件与 URL（避免“URL 已变但页面仍显示旧日期”）。
  - [ ] 补齐 `/org/assignments` Unauthorized E2E 覆盖（与 nodes/positions 对齐）。
  - [ ] Positions 的 `LifecycleStatus/StaffingState` 在 list/detail/timeline 统一用 i18n label 展示（未知值回退到原始 code）。
  - [ ] 通过相关门禁并保证 E2E 可复现（本地/CI）。

- **非目标（Out of Scope）**：
  - 不考虑多浏览器适配：以 Chrome/Chromium 为准（见 044）。
  - 不开展故障注入与性能采样：044 的 4.6/4.7 暂不纳入本计划门槛。
  - 不引入 DB 迁移/Schema 变更；不改变 Org 领域服务的核心数据契约（仅修正 UI/Controller 行为与展示）。

## 2.1 工具链与门禁（SSOT 引用）
> 命令细节以 `AGENTS.md`/`Makefile`/CI 为准；本文只声明本计划命中触发器与复现入口。

- **触发器清单（本计划命中项）**：
  - [ ] Go 代码（Org UI controller / mapper / viewmodel）
  - [ ] `.templ` / Tailwind（Org UI templates / components）
  - [ ] 多语言 JSON（若新增/补齐 i18n keys）
  - [ ] Authz（不改政策文件；仅补 E2E 覆盖与 UI gating 验证）
  - [ ] DB 迁移 / Schema（不涉及）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - E2E（Playwright）说明：`e2e/README.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图（交互链路）
```mermaid
graph TD
  A[Browser + HTMX] -->|GET/POST/PATCH /org/*| B[OrgUIController]
  B --> C[org templates (*.templ)]
  C -->|HTML fragments / swap-oob / HX-Push-Url| A
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：父节点信息必须可读且可导航**
  - 选项 A：继续展示 UUID（不选）
  - 选项 B（选定）：展示 `Name (Code)`，点击后触发 NodePanel（并 OOB 刷新 tree 选中态）
- **决策 2：effective_date 的契约用“来源优先级 + UI 同步”解决**
  - 选项 A：重命名 query 参数为 `as_of`（更清晰但改动大，暂不选）
  - 选项 B（选定）：保持 query 参数不变，但在 controller 明确：写操作优先读取 body 的 `effective_date`；成功后用 OOB/片段刷新同步页面日期控件与 as-of 文案
- **决策 3：状态字段统一 i18n label**
  - 选项 A：展示 code（不选）
  - 选项 B（选定）：映射到 filters 已存在的 i18n keys；未知值回退到 code（避免空白）
- **决策 4：补齐 assignments Unauthorized 的 E2E 覆盖**
  - 选项 A：依赖人工点 UI（不选）
  - 选项 B（选定）：在 `e2e/tests/org/org-ui.spec.ts` 增加用例，与 nodes/positions 同结构断言

## 4. 数据模型与约束 (Data Model & Constraints)
- 本计划不引入 DB 迁移与 Schema 变更。
- 数据只读展示的改动（label 映射）不改变后端存储与 API payload，仅改变 UI 展示层。

## 5. 接口契约 (API Contracts)
> 重点是 **UI/HTMX 契约**：URL、参数来源、响应包含哪些 fragment/OOB、以及错误码在 UI 上如何呈现。

### 5.1 effective_date 契约（as-of vs 写入）
- 页面 as-of（浏览上下文）：来自 URL query `effective_date=YYYY-MM-DD`，用于渲染树/列表/面板的“当前视图”。
- 写操作生效日期：来自 form body `effective_date=YYYY-MM-DD`（Node/Assignment/Position 的 create/edit/transition）。
- **优先级（必须实现）**：写操作 handler 读取 `effective_date` 时，必须优先使用 `r.PostFormValue("effective_date")`（或等价逻辑），query 仅作为兜底或浏览上下文。
- **成功后同步（必须实现）**：当写操作的 `effective_date` 与页面当前 as-of 不一致时：
  - 仍以写入日期作为新的 as-of：设置 `HX-Push-Url` 到 `effective_date=<写入日期>`；并同步更新页面顶部日期控件与 as-of 文案（避免“URL 变了但页面显示旧日期”）。

### 5.2 `/org/nodes`（组织结构）
- `GET /org/nodes?effective_date=YYYY-MM-DD&node_id=<uuid?>`
  - 返回：包含 `#org-nodes-page` 的完整页面响应（HTMX 通过 `hx-select` 抽取该 fragment）。
- `GET /org/nodes/{id}?effective_date=YYYY-MM-DD&view=<edit|move?>`
  - 返回：`#org-node-panel` 的 innerHTML（NodeDetails/NodeForm），并在默认 view 下 OOB 刷新 `#org-tree`（选中态一致）。
- `POST /org/nodes?effective_date=<as-of>`
  - Body（FormData）：`code,name,status,display_order,parent_id?,i18n_names?,effective_date=<写入日期>`
  - Success（200）：
    - Header：`HX-Push-Url: /org/nodes?effective_date=<写入日期>&node_id=<newID>`
    - Body：返回 NodeDetails（写入日期 as-of）+ OOB 刷新 `#org-tree`；并 OOB 刷新页面顶部日期控件/文案（见 6.2）。
  - Error：
    - 422：返回 NodeForm（保留用户输入与 field errors，可见且可恢复）。
    - 401/403：返回 Unauthorized（不得 swap 成空白；必须可见）。

### 5.3 `/org/assignments`（任职/分配）
- `GET /org/assignments?effective_date=YYYY-MM-DD&pernr=<string?>`
  - 返回：包含 `#org-assignments-page` 的页面响应。
- `POST /org/assignments?effective_date=<as-of>`（Hire）/ `POST /org/assignments/{id}:transition?effective_date=<as-of>`（Transfer/Termination）
  - Body：`effective_date=<写入日期>` + 其他字段
  - Success：`HX-Push-Url: /org/assignments?effective_date=<写入日期>&pernr=<pernr>` + OOB 刷新 `#org-assignments-timeline`（已存在）+ 同步页面顶部日期控件/文案（见 6.2）。
  - Unauthorized：read/assign 两类都必须有可理解提示；其中 read 维度需有 E2E 覆盖（见 9）。

### 5.4 `/org/positions`（职位）
- `GET /org/positions?effective_date=YYYY-MM-DD&node_id=<uuid?>&position_id=<uuid?>`
  - 返回：包含 `#org-positions-page` 的页面响应。
- `PATCH /org/positions/{id}?effective_date=<as-of>` / `POST /org/positions?effective_date=<as-of>`
  - Body：`effective_date=<写入日期>` + 其他字段
  - Success：`HX-Push-Url` 指向 `effective_date=<写入日期>` 的 positions 页面（保持 node_id/position_id），并同步顶部日期控件/文案；panel/list 以写入日期重算。

## 6. 核心逻辑与实现细节 (Business Logic & Implementation Notes)
### 6.1 P1：ParentHint/ParentID 显示为 label + 可回溯导航（nodes）
- 目标 UI：Parent 显示 `Name (Code)`；点击后切换到父节点（NodePanel 更新 + Tree 选中态更新）。
- 影响文件（建议）：
  - `modules/org/presentation/templates/components/orgui/node_details.templ`
  - `modules/org/presentation/templates/components/orgui/node_forms.templ`
  - `modules/org/presentation/viewmodels/node.go`
  - `modules/org/presentation/controllers/org_ui_controller.go`（计算 label 与 link 行为）
- 设计：
  - 为 NodeDetails 的 parent 区块增加 clickable 行为：
    - `hx-get="/org/nodes/<parentID>?effective_date=<as-of>"`
    - `hx-target="#org-node-panel"`，`hx-swap="innerHTML"`
    - `hx-push-url="/org/nodes?effective_date=<as-of>&node_id=<parentID>"`（触发树选中态一致）
  - label 计算：优先用当前 hierarchy 列表映射（已在 nodes 页/NodePanel 中获取）；找不到再回退 `OrgUIController.orgNodeLabelFor(...)`。
- 实现细节（可直接编码）：
  - ViewModel：
    - 在 `modules/org/presentation/viewmodels/node.go` 的 `OrgNodeDetails` 增加字段：`ParentLabel string`（默认空字符串）。
  - Controller：
    - `NodesPage`：在拿到 `nodes`（hierarchy）与 `selected`（node details）后，若 `selected.ParentHint != nil`：
      - 优先从 `nodes` 列表按 ID 映射 label（`Name (Code)`）；写入 `selected.ParentLabel`。
      - 若映射失败，回退 `c.orgNodeLabelFor(r, tenantID, *selected.ParentHint, effectiveDate)`。
    - `writeNodePanelWithOOBTree`：同样在渲染前补齐 `details.ParentLabel`（确保“通过树点击节点”也能看到 label + 可点击导航）。
    - `NewNodeForm`：当 query `parent_id` 存在时，计算 `ParentLabel` 并下发给表单（见下）。
  - Templates：
    - `modules/org/presentation/templates/components/orgui/node_details.templ`：
      - 当 `props.Node.ParentHint != nil` 时优先展示 `props.Node.ParentLabel`；为空则回退展示 UUID（仅兜底，不作为常态）。
      - 将 parent label 渲染为 `<button type="button">`（或 `<a>`），并按上面的 `hx-*` 约定跳转到父节点。
    - `modules/org/presentation/templates/components/orgui/node_forms.templ`：
      - 在 `NodeFormProps` 增加 `ParentLabel string`，当 `props.ParentID != nil` 时展示 `ParentLabel`；UUID 仅作为次要信息（可省略或缩小显示）。
- 验收点：nodes 页 node details 与 create-child 表单均不再暴露 UUID 作为唯一信息（UUID 可作为次要信息隐藏/折叠）。

### 6.2 P1：effective_date 优先级 + 页面顶部日期控件同步
- 目标：当用户在表单内填写未来日期并提交成功后，页面顶部的 `#effective-date` 与 as-of 文案必须同步为新日期；URL、tree、panel 三者一致。
- 影响文件（建议）：
  - `modules/org/presentation/controllers/org_ui_controller.go`：把写操作的 effective_date 读取改为显式优先 `r.PostFormValue("effective_date")`。
  - `modules/org/presentation/templates/pages/org/nodes.templ`、`modules/org/presentation/templates/pages/org/assignments.templ`、`modules/org/presentation/templates/pages/org/positions.templ`
    - 为顶部日期区域增加稳定锚点（例如 `id="org-*-header"`），便于 OOB 同步。
- 设计（选定）：写操作成功响应中追加 OOB 同步片段
  - 在 nodes/assignments/positions 页面顶部日期控件外层容器加 id（例如 `org-nodes-header`/`org-assignments-header`/`org-positions-header`）。
  - Controller 在成功写入后：
    - `HX-Push-Url` 指向 `effective_date=<写入日期>`
    - 响应 body 除原有 panel/tree/timeline 更新外，追加 `hx-swap-oob="true"` 的 header 容器（以写入日期渲染），确保 UI 文案与控件值同步。
- 实现细节（可直接编码）：
  - Controller（显式优先级）：
    - 新增 helper（建议放在 `org_ui_controller.go`）：
      - `effectiveDateFromWriteForm(r *http.Request) (time.Time, error)`：读取 `strings.TrimSpace(r.PostFormValue("effective_date"))` 并 `parseEffectiveDate`；为空/非法返回 error。
      - `effectiveDateFromQuery(r *http.Request) (time.Time, error)`：读取 `strings.TrimSpace(r.URL.Query().Get("effective_date"))` 并 `parseEffectiveDate`（GET 页面与局部刷新使用）。
    - 将以下写操作统一改为使用 `effectiveDateFromWriteForm`（避免与 query 多值混淆）：
      - Nodes：`CreateNode` / `UpdateNode` / `MoveNode`
      - Assignments：`CreateAssignment` / `UpdateAssignment` / `TransitionAssignment`
      - Positions：`CreatePosition` / `UpdatePosition`
  - Templates（避免同页多处 `name="effective_date"` 导致 hx-include 误包含）：
    - 将页面顶部日期选择器的 `hx-include` 从 `[name='effective_date']` 收敛为 `#effective-date`（只包含顶部控件自身）：
      - Nodes：`modules/org/presentation/templates/pages/org/nodes.templ`：`hx-include="#effective-date"`
      - Assignments：`modules/org/presentation/templates/pages/org/assignments.templ`：`hx-include="#org-pernr, #effective-date"`
      - Positions：`modules/org/presentation/templates/pages/org/positions.templ`：`hx-include="#org-positions-filters, #effective-date"`
    - 对引用 `TreeProps.Include` 的页面（positions）同步改为包含 `#effective-date`，避免把表单内的 `effective_date` 一并带上。
  - Header OOB 同步：
    - 在每个页面顶部 header 容器加 id（例如 `id="org-nodes-header"`），并将“as-of 文案 + date input”都放在该容器内，确保 OOB 替换后 UI 一致。
    - 在写操作成功的响应中追加 `hx-swap-oob="true"` 的 header 容器渲染（推荐抽取为可复用 templ 组件，避免 controller 侧复制 HTML 属性）。
- 失败模式与回退：
  - 422：返回的 form 必须保留用户填写的 `effective_date`（写入日期），并展示 field error；不得回退成页面旧日期。
  - 401/403：不得 swap 成空白；必须显示 Unauthorized 区块（对齐 044/037）。

### 6.3 P2：Positions 状态字段统一 i18n label（positions）
- 影响文件（建议）：`modules/org/presentation/templates/components/orgui/positions.templ`
- 设计：
  - 新增映射 helper：把 `planned/active/inactive/rescinded` 映射到 `Org.UI.Positions.Lifecycle.*`；把 `empty/partially_filled/filled` 映射到 `Org.UI.Positions.Staffing.*`。
  - 映射表（按 code → i18n key）：
    - Lifecycle：
      - `planned` → `Org.UI.Positions.Lifecycle.Planned`
      - `active` → `Org.UI.Positions.Lifecycle.Active`
      - `inactive` → `Org.UI.Positions.Lifecycle.Inactive`
      - `rescinded` → `Org.UI.Positions.Lifecycle.Rescinded`
    - Staffing：
      - `empty` → `Org.UI.Positions.Staffing.Empty`
      - `partially_filled` → `Org.UI.Positions.Staffing.PartiallyFilled`
      - `filled` → `Org.UI.Positions.Staffing.Filled`
  - 在以下位置统一使用 label：
    - PositionsList 表格列
    - PositionDetails 概览
    - PositionTimeline 列表项
  - 未知值：回退展示原始 code（避免空白）。

### 6.4 P2：Assignments “刷新”动作语义强化（assignments）
- 设计建议（不改变契约）：
  - 给刷新按钮加说明文案或 icon（例如“刷新时间线”），并保持为 secondary；避免与“Create/Save”主动作混淆。

### 6.5 补齐失败模式覆盖（对齐 044）
- 非法输入：必填缺失（code/name/pernr）、无效日期、`i18n_names` 非法 JSON（422 且错误可见）
- 冲突：重复 code（409/422 且提示明确）
- 权限：assignments 的 read/assign 边界（按钮 gating + 页面 Unauthorized）

## 7. 安全与鉴权 (Security & Authz)
- 本计划不修改 Casbin policy 聚合文件；仅要求 UI 与 E2E 验证 read/assign/write 的边界一致。
- 新增的“父节点可点击回溯”属于 read 行为：必须经过现有 `ensureOrgAuthzUI(..., orgHierarchiesAuthzObject, "read")` 保护；无权时返回 Unauthorized。
- 证据产物脱敏：不在仓库提交 HAR/trace；在 PR/Readiness 引用 CI artifact 时注意去除 cookie/个人信息。

## 8. 依赖与里程碑 (Dependencies & Milestones)
1. [ ] Nodes：父节点 label + 导航（UI + controller 计算）
2. [ ] effective_date：写入优先级显式化 + header OOB 同步（nodes/assignments/positions）
3. [ ] Positions：状态字段 i18n label 统一
4. [ ] E2E：补齐 `/org/assignments` Unauthorized 用例；补齐 “future-dated 写入后 header/URL/树一致” 用例
5. [ ] Readiness：记录执行命令与结果（必要时迁移到 `docs/dev-records/`）

## 9. 测试与验收标准 (Acceptance Criteria)
### 9.1 自动化验证记录（已执行）
- [X] 2025-12-24：运行 `e2e/tests/org/org-ui.spec.ts`（Chromium，headless）结果 4 passed。

### 9.2 必须新增/更新的自动化用例
- [ ] `e2e/tests/org/org-ui.spec.ts` 增加：无 Org 权限账号访问 `/org/assignments` 返回 Unauthorized（与 nodes/positions 断言一致）。
- [ ] `e2e/tests/org/org-ui.spec.ts` 增加：在 NodeForm/AssignmentForm/PositionForm 中把 `effective_date` 改为 future date 提交成功后：
  - URL 包含 `effective_date=<future>`
  - 页面顶部 `#effective-date` 值为 `<future>`
  - 树/面板/时间线与 `<future>` 一致（不出现“URL 已变但页面仍显示旧日期”）

### 9.3 验收清单（完成定义）
- [ ] ParentHint/ParentID 不再以 UUID 作为唯一展示，且可一键回到父节点。
- [ ] 写操作 effective_date 以 body 为准，成功后页面日期控件与文案同步，HTMX 不出现空白 swap。
- [ ] Positions 的 Lifecycle/Staffing 在 list/detail/timeline 均展示为 i18n label。
- [ ] E2E 覆盖补齐并通过；`make check doc`、相关 lint/test 门禁可通过。

## 10. 运维与监控 (Ops & Monitoring)
- 本计划不新增 runtime 依赖与监控指标；如需排障，使用现有 request-id 关联与 Playwright trace/录像（不入库）。
