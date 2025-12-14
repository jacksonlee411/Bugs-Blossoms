# DEV-PLAN-016：权限管理模块页面 UI 设计优化建议（覆盖 DEV-PLAN-012~015）

**状态**: 草拟中（2025-12-12 16:00 UTC）

## 背景与进展
- DEV-PLAN-012~015 已定义并落地 Casbin 授权底座、模块改造与公共层 UI 契约；其中 DEV-PLAN-015A/015B 交付了策略平台与 UI 工作流（角色/用户策略管理、Unauthorized/PolicyInspector、草稿提交与反馈）。
- 当前 UI 已可跑通闭环，但在信息架构、交互一致性、误操作防护、i18n/a11y 与可理解性上仍有可预期的体验成本。
- 本文档将 012~015 的 UI 需求与现有实现进行对照，沉淀“调查发现 + UI 优化建议”，作为后续迭代的设计/开发 backlog。

## 调研输入与范围
### 文档输入
- `docs/dev-plans/012-casbin-core-hrm-logging.md`
- `docs/dev-plans/013-casbin-infrastructure-and-migration.md`
- `docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`
- `docs/dev-plans/014D-casbin-public-layer-ui-interface.md`
- `docs/dev-plans/015-casbin-policy-ui-and-workflow.md`
- `docs/dev-plans/015A-casbin-policy-platform.md`
- `docs/dev-plans/015B-casbin-policy-ui-and-experience.md`
- `docs/dev-plans/015B1-core-roles-ui.md`
- `docs/dev-plans/015B2-core-users-ui.md`
- `docs/dev-plans/015B3-business-authz-experience.md`
- `docs/dev-plans/015B4-ui-integration-feedback.md`

### 页面/组件范围（覆盖 012-015 所需 UI 面）
- 角色策略矩阵：`modules/core/presentation/templates/pages/roles/policy_matrix.templ`
- 用户权限板：`modules/core/presentation/templates/pages/users/policy_board.templ`
- Unauthorized 组件：`components/authorization/unauthorized.templ`
- PolicyInspector 组件：`components/authorization/policy_inspector.templ`
- 草稿/请求页面（Requests Center）：`/core/authz/requests`、`/core/authz/requests/{id}`（若缺失，本计划补齐以保证 `view_url` 不断链）
- 业务页面接入（HRM/Logging）：`modules/hrm/presentation/templates/**`、`modules/logging/presentation/templates/**`
- 导航/Quick Links/Spotlight 可见性：`pkg/middleware/sidebar.go`、`pkg/spotlight/items.go`、`modules/*/links.go`

### 非目标
- 不展开后端 API/状态机的实现细节（以 015A/015B 的契约为准），但需要在 UI 侧明确并验收 014D/015B4 定义的契约与 fallback 行为。
- 不触碰冻结模块：`modules/billing`、`modules/crm`、`modules/finance`。

## 调查发现（主要问题）
### 1) 心智模型不统一：Stage / Request / Bot / 生效
- 角色页/用户页/Unauthorized 都存在“暂存（stage）→提交草稿（request）→轮询状态→bot/PR→生效”的链路，但各处呈现粒度不同：
  - 角色页更偏“表格操作 + 颜色标记”，缺少提交前的变更摘要与确认（`modules/core/presentation/templates/pages/roles/policy_matrix.templ:90`）。
  - 用户页有“轮询/超时”提示，但状态文案与 i18n 处理不一致（`modules/core/presentation/templates/pages/users/policy_board.templ:205`）。
  - Unauthorized/PolicyInspector 把 diff 直接作为 JSON textarea 暴露，信息密度高且不利于非技术用户理解（`components/authorization/unauthorized.templ:55`）。

### 2) p / g 两类策略语义在 UI 上被混用
- 角色新增规则抽屉允许选择 `type=g`，但表单仍要求 Object/Action/Effect（更像 p 规则），容易产生无效或误导性输入（`modules/core/presentation/templates/pages/roles/policy_matrix.templ:198`）。
- 用户“继承列”按通用策略表格展示（Type/Subject/Domain/Object/Action/Effect），对“角色继承（g）”的真实含义不够直观，难以回答“从哪个角色继承的？”。

### 3) 术语与本地化不一致（尤其是状态/提示文案）
- 用户页状态映射为硬编码英文（`modules/core/presentation/templates/pages/users/policy_board.templ:205`），且部分 UI 直接拼接 “Subject/Domain”（`modules/core/presentation/templates/pages/users/policy_board.templ:110`）。
- 角色矩阵存在硬编码中文/英文混杂（如 “Policy Matrix/Prev/Next/Domain”等），与全局 i18n 体系不对齐（`modules/core/presentation/templates/pages/roles/policy_matrix.templ:71`、`:158`、`:434`）。
- Unauthorized/PolicyInspector 组件通过 dataset 注入了部分状态 label，但 JS 仍有硬编码中文错误兜底文本与固定 “SLA xx:yy” 形式（`components/authorization/unauthorized.templ:413`、`:447`）。

### 4) 关键操作缺少“提交前预览 + 原因输入 + 风险提示”
- 角色页“提交草稿”不收 reason，且默认 object/action 为空，容易导致“提交了但不知道提交了什么”的体验（`modules/core/presentation/templates/pages/roles/policy_matrix.templ:90`）。
- bulk remove / 删除类动作缺少清晰的语义分层与二次确认，表格里的 “Delete” 同时承担“撤销暂存/暂存删除”的概念，易误触。

### 5) 可用性细节：复制、筛选、聚合视图不足
- Unauthorized 的 “复制 request_id” 无明确成功/失败反馈，且无降级提示（`components/authorization/unauthorized.templ:85`）。
- 策略表格缺少“按资源聚合”“仅看变更/仅看暂存”等视图切换，导致在策略量大时难以定位差异。

### 6) 业务页面的细粒度授权 UX 尚未形成统一模式
- DEV-PLAN-012 期望在 HRM/Logging 等业务页面不仅有 403 空态，还应具备“可见的权限状态提示、关键按钮禁用与申请入口”等细粒度体验（`docs/dev-plans/012-casbin-core-hrm-logging.md:73`）。
- 当前实现仍偏向“直访 403 空态”，缺少对“部分有权限（可 view 不可 create/delete）”场景的统一设计与可复用组件清单；需要将其纳入本计划的落地与验收（见 P0-6、E）。

### 7) 导航/Quick Links/Spotlight 的授权过滤与验证清单缺失
- DEV-PLAN-014D 明确要求导航/Quick Links/Spotlight 统一过滤并验证可见性（`docs/dev-plans/014D-casbin-public-layer-ui-interface.md:25`）；当前缺少可执行的回归清单/验证，容易形成“页面已 403，但入口仍可见/反之”的割裂体验（见 P0-7、F）。

### 8) HTMX/REST 403 契约与非 HTMX fallback 需要作为强制验收项
- 014D/015B4 对 403 payload、HX-Trigger 错误通道、非 HTMX 场景（redirect/flash/标准错误页）有明确约定（`docs/dev-plans/014D-casbin-public-layer-ui-interface.md:24`、`docs/dev-plans/015B4-ui-integration-feedback.md:14`）。
- UI 改造必须把“契约对齐与回归验证”作为强制验收项，避免 HTMX/REST 分叉或静默失败（见 P0-8、H）。

## 优化目标
1. 让管理员能快速回答三个问题：我能做什么？缺什么？怎么补齐并追踪到生效？
2. 在所有入口统一“工作流可见性”：变更摘要、提交确认、状态追踪、错误可解释。
3. 降低误操作风险：类型语义明确、批量操作可撤销/可确认、危险变更有提示。
4. i18n/a11y 与组件一致性对齐 015B 的验收标准（键盘可达、必要 ARIA、文案一致）。

## 统一术语与数据模型（规范）
为避免各页面各说各话，本计划统一以下术语/数据结构，作为 UI 文案、i18n key、组件 props 与验收的共同基线：

### 1) 工作流术语（统一口径）
- Stage（暂存）：页面内“未提交”的变更集合（可清空/撤销/导出）；用于生成 Draft/Request，不代表已进入审批或 bot 流程。
- Request（草稿/请求）：通过 `POST /core/api/authz/requests` 创建的持久化记录（返回 `request_id`、`view_url`、SLA、可选 `retry_token`）；状态由 `GET /core/api/authz/requests/{id}` 或 `GET /core/api/authz/requests` 轮询获得。
- Bot/PR：bot 消费请求并创建/更新 PR；PR 合并后策略进入下一次加载/刷新周期。
- 生效（Effective）：`policy.csv`/`.rev` 已更新且服务端完成 reload；UI 统一将其表达为“已生效/已合并”，并提供可追踪入口（PR 链接/请求详情）。

### 2) 策略类型与 diff 结构（对齐现有契约）
- `p`（权限规则）：`[subject, object, action, domain, effect]`，对应 `policy.csv` 行：`p, <sub>, <obj>, <act>, <dom>, <eft>`。
- `g`（成员/继承关系）：`[subject, role, domain]`，对应 `policy.csv` 行：`g, <sub>, <role>, <dom>`。
- Diff 使用 JSON Patch（RFC6902），UI 侧优先通过“暂存→提交”生成 diff，避免用户手写 index：
  - 新增 `p`：`{"op":"add","path":"/p/-","value":["role:reporting","core.users","read","global","allow"]}`
  - 新增 `g`：`{"op":"add","path":"/g/-","value":["tenant:<id>:user:<id>","role:core.viewer","<tenant-domain>"]}`

### 3) Request 状态（统一枚举）
终态：`merged/failed/rejected/canceled`；非终态：`draft/pending_review/approved`。所有页面统一从 i18n 渲染状态与 SLA，不在 JS 内硬编码状态文案。

### 4) Domain 显示（UI 约定）
- UI 上应优先显示可读的“租户名称”而非仅显示 ID。
- 需确认后端 `ViewState` 或 API 是否已注入 ID 到 Name 的映射；若无，需作为依赖项补充。

## 优化建议（按模块/页面）
### A. 全局信息架构（建议优先级：P0）
- 引入“Authz 工作区（Workspace）”统一容器（可作为 Core 级组件）：
  - **P0 阶段定义**：仅作为**当前上下文（Subject+Domain）的提交栏**（类似购物车结算条），固定在页面底部或顶部。避免设计为跨页面的全局暂存，以降低后端 Session 管理复杂度。
  - **布局建议**：推荐使用 Sticky Footer（底部吸附），并确保主内容区域预留足够的 `padding-bottom` 防止遮挡。
  - 功能：展示当前 subject/domain、暂存数量、变更摘要（+N/-M、涉及资源/动作）、提交按钮、清空按钮、导出 diff（建议支持颜色编码的可视化 Diff）、最近 request 状态与跳转链接。
- 将“查看草稿/请求详情”的入口固定化：所有提交成功后的 toast 与页面内状态区都应包含 view_url（`/core/authz/requests/{id}`），并在无 JS/非 HTMX 场景保持可用；若该页面尚未落地，优先补齐最小只读详情页以避免断链（见 P0-2、G）。

### B. 角色页（Policy Matrix）（建议优先级：P0-P1）
- “矩阵化”默认视图：按资源聚合（行=Object，列=Action），单元格展示 allow/deny/空；原始规则表格放到“高级/原始规则”。
- **空状态引导**：当角色无策略时，提供“快速开始”模板（如“复制已有角色权限”或“从预设模板导入”），降低初始配置难度。
- p/g 表单分流：
  - `type=p`：编辑该角色的权限规则（Object/Action/Effect/Domain）；危险项（deny、通配符、跨域）在提交前必须显式提示。
  - `type=g`：仅用于“角色继承/绑定角色”（当前角色继承另一个角色），表单字段为“父角色/域”，不再要求填写 Object/Action/Effect；建议增加循环依赖检测提示（如 A->B->A），前端即时阻断或警告。
- 提交草稿前加确认对话框：展示变更摘要、reason 输入（必填/默认可编辑）、危险项提示（deny、通配符、跨域）。

### C. 用户页（Policy Board）（建议优先级：P0-P1）
- 继承列改为“角色视图”而不是通用策略表格：以角色为一级条目，展开查看该角色带来的策略；支持“添加/移除角色”专用交互。
- 增加 “Effective 权限（汇总）”区域：合并 allow/deny 后按模块分组展示，并能展开来源链路（来源：角色/直授/override）。
  - **依赖确认**：`015A` 的 `/debug` 接口仅支持单点检查。实现汇总视图可能需要后端补充 `GetImplicitPermissions` 或类似聚合 API；若暂不支持，P0 阶段可仅展示“显式分配的角色与策略”，将“计算后的最终权限”作为 P1。
  - **交互建议**：采用“列表+抽屉”模式。列表仅展示最终结果（允许/拒绝），点击条目后在侧边抽屉展示归因链路（Trace），避免列表过载。
- 状态与 SLA 全面 i18n 化：移除硬编码 `authzStatusLabels` 文案，改为从后端/模板注入 i18n key 或 dataset label。

### D. Unauthorized / PolicyInspector（建议优先级：P0）
- diff 默认结构化展示：Add/Remove 分组列表 + “复制 JSON/展开原文”开关；减少 textarea 作为主展示。
- 复制动作补齐反馈：复制 request_id / 复制 debug 链接 / 复制 diff 都应有 toast（成功/失败）与 fallback。
- SLA 展示格式 i18n 化：不要在 JS 内拼接固定英文/中文前缀，改用 i18n key + 统一格式化函数。
- PolicyInspector 更贴合“抽屉”：统一使用 Drawer 组件并补齐快捷动作（复制 matched policy、复制 attributes JSON）。

### E. 业务页面（HRM/Logging 等）的细粒度授权体验（建议优先级：P0-P1）
- 除 403 空态外，补齐“部分授权”场景的统一交互：
  - 关键按钮（新建/导入/删除等）在无权限时禁用，并提供一致的 tooltip/inline hint + 一键申请入口（对齐 012 的“按钮禁用 + 申请权限”预期）。
  - **组件化建议**：
    - 对于 **按钮/链接**：推荐直接在组件 Props 中传递 `Disabled: !pageCtx.CanAuthz(...)`，并复用 Button 组件内部的 Tooltip 逻辑，而非包裹外层组件（避免 SSR 无法修改子元素属性的问题）。
    - 对于 **区块/面板**：提供 `components.AuthzGuard(allowed bool, fallback templ.Component)`，无权时渲染 fallback（如“申请权限”占位符）或隐藏。
  - 页面头部提供轻量的“权限状态提示”（例如：当前页所需 object/action、是否允许、所属 domain），帮助定位“为什么按钮灰了/为什么被拒绝”。
- 将该能力做成可复用组件/模式：业务页面只需要声明 object/action/domain 与 reason 模板，其余交给 Unauthorized/请求封装处理。

### F. 导航/Quick Links/Spotlight 的可见性一致性（建议优先级：P0）
- 建立 UI 回归清单：同一 object/action 的入口默认“无权限即隐藏”，直访则 403 统一反馈；如需“置灰+可申请”，仅在页面内关键按钮场景使用，避免导航入口的割裂体验（对齐 014D）。
- 为“入口不可见”提供可诊断路径（仅管理员可见）：例如在 PolicyInspector/Debug 中可查询该入口对应的 object/action，避免只能靠猜。

### G. 草稿中心 / 请求详情页（Requests Center）（建议优先级：P0-P1）
- P0：补齐 `/core/authz/requests/{id}` 最小只读详情页，确保 `view_url` 不断链（支持非 HTMX/无 JS）。至少包含：状态时间线、diff（结构化 + 原始 JSON）、request_id 复制、PR 链接、bot 错误日志。
- P1：完善列表/详情的管理能力：过滤（我的/待审核/状态）、分页、revert、审核动作（基于权限显示）、失败重试 bot（冷却提示）。

### H. UI 契约与 fallback（对齐 014D/015B4）（建议优先级：P0）
- HTMX/REST 一致性：403 payload 字段、HX-Trigger 错误通道、base_revision 过期提示与刷新 CTA 行为保持一致。
- 非 HTMX 场景：表单提交/跳转路径仍能获得可理解的反馈（flash/标准错误页），并提供 request_id/view_url 的可追踪入口。
- 将“契约对齐”作为 UI 改造验收的一部分（不只靠后端测试）。

### I. 一致性与可访问性（建议优先级：P0-P2）
- 统一字段展示：主体/域/资源/动作/效果的显示与复制规则一致（展示 i18n 名称 + 原始 key）。
- 键盘与 ARIA：抽屉/对话框按 015B 的 a11y 规范补齐焦点管理与 `aria-*`，并为批量操作按钮提供明确的 aria-label。

## 建议优先级与落地清单（Backlog）
### P0（优先做：减少误操作 + 统一体验）
1. [x] 引入统一 Authz Workspace（Sticky Footer/Header）：作为当前上下文的提交栏，展示暂存数量与提交入口；支持提交前预览 + reason 必填 + 危险项提示。（已交付：角色矩阵/用户权限页统一提交栏 + reason 必填 + 提交前预览 + 危险项提示 + 二次确认）
2. [x] 补齐 `/core/authz/requests/{id}` 请求详情页（最小只读即可），确保 `view_url` 不断链；非 HTMX 也可访问（对齐 015B4）。
3. [x] p/g 语义分离：角色页 `type=g` 仅做“角色继承/绑定角色”；用户页以“用户↔角色”视图呈现继承与变更；表单不再要求无意义字段。（已完成：g 规则不再要求填写/提交无意义的 Action/Effect；后端对 g 默认补齐 `action="*"`/`effect="allow"`；角色矩阵/用户权限页的 g 展示语义化并支持跳转到角色策略）
4. [x] i18n 清理：移除硬编码状态与提示文案（用户页状态映射、角色矩阵标题/分页、Unauthorized JS 兜底文案）。（已完成：关键交互与状态均走 i18n；allow/deny 展示统一映射到 i18n 文案）
5. [x] diff 默认结构化渲染 + 复制反馈（Unauthorized/PolicyInspector/Requests 详情页）。
6. [x] 业务页面“部分授权”统一模式：按钮禁用 + 申请入口 + 权限状态提示（对齐 012 的 HRM/Logging 体验）。（已交付：HRM Employees 的 create/update/delete 场景；Logging 无权限直访统一 Unauthorized）
7. [x] 导航/Quick Links/Spotlight：默认无权限隐藏 + 直访 403 统一反馈；补齐回归清单与验证（对齐 014D）。
8. [x] HTMX/REST 403 契约与非 HTMX fallback 回归验证（对齐 014D/015B4）。（已补齐：JSON 合同、HTMX headers、HTML fallback 渲染的回归测试）

### P1（体验增强：更好理解与排障）
1. [x] 增加 Effective 权限汇总视图 + 来源链路（用户页）。（已完成：按 domain/object/action 聚合；展示直配 + 角色继承链；支持跳转到角色策略矩阵）
2. [x] 草稿中心（Requests Center）列表页完善：过滤/分页/我的与待审核视图 + 详情页补齐审核动作与运维入口（PR/bot/revert）。（已交付：过滤/分页/全部&我发起&待审核（按权限可见）；详情页支持结构化 diff + 审核/运维动作）

### P2（质量与规模化）
1. [x] 资源/动作字典化选择（Object/Action selector），减少拼写与无效 diff。（已完成：角色矩阵/用户权限页暂存抽屉提供 datalist 建议项）
2. [x] 批量操作二次确认/撤销能力（尤其是 bulk remove）。（已完成：用户权限页 bulk remove 增加确认弹窗 + Undo 撤销能力，支持批量删除暂存条目）
3. [ ] 为核心页面补充 axe smoke + 键盘巡检记录（对齐 015B 验收）。（已提供巡检清单模板：`docs/dev-records/DEV-PLAN-016-A11Y-CHECKLIST.md`；待补齐实际巡检结果）

## 验收标准（建议）
- 角色/用户/业务页的授权申请入口：均可在 UI 内清晰看到“将要变更什么 + 为什么 + 预计多久生效 + 去哪看状态”。
- `type=g` 的交互不再要求用户填写 Object/Action，页面语义明确（继承/绑定/成员关系）。
- 所有状态/提示/SLA 文案均走 i18n：`make check tr` 通过；不再存在硬编码英文状态标签。
- 复制类动作有可见反馈；diff 默认可读（结构化），JSON 原文作为高级选项。
- 业务页面的关键操作在无权限时禁用且可申请；导航/Quick Links/Spotlight 的入口可见性与直访 403 行为一致。
- HTMX/REST/非 HTMX 的 403/错误反馈与契约一致；`view_url=/core/authz/requests/{id}` 可直达请求详情页并可追踪。
- 模板/样式修改后：`templ generate && make css` 生成结果干净（`git status --short` 无差异）。

## 参考
- DEV-PLAN-015（母计划）：`docs/dev-plans/015-casbin-policy-ui-and-workflow.md`
- DEV-PLAN-015B（UI/体验）：`docs/dev-plans/015B-casbin-policy-ui-and-experience.md`
- 015B1/015B2/015B3/015B4 子计划同目录文件

## 实施记录（2025-12-13 更新）
### 已完成 (P0 阶段)
1. **核心组件开发**：
   - `AuthzGuard` (templ): 实现了细粒度授权控制组件，支持 fallback。
   - `AuthzWorkspace` (templ): 实现了 Sticky Footer 布局的暂存区组件，并补齐提交前预览、危险项提示与二次确认。
2. **页面与控制器**：
   - `RequestDetail` (templ): 请求详情页模板已实现，并支持结构化 diff + 原始 JSON 展示。
   - `AuthzRequestController`: 已接入 `PolicyDraftService.Get`，通过 `/core/authz/requests/{id}` 渲染只读请求详情。
   - `AuthzRequestController_test`: 通过集成测试验证从 `/core/api/authz/requests` 创建草稿后，`view_url` 能正确渲染详情页。
3. **Requests Center 列表页与入口**：
   - 新增 `/core/authz/requests` 列表页（状态过滤、分页跳转、全部/我发起/待审核快捷视图），并与详情页打通。
   - 侧边栏（Administration）与 Spotlight/Quick Links 补齐入口，默认按 `AuthzRequestsRead`/authz capability 隐藏。
4. **体验与一致性**：
   - 角色矩阵/用户权限页接入 `AuthzWorkspace`，统一暂存数量与提交入口，并要求 reason 输入。
   - 403 payload 的 object/action 对齐为 `core.authz/read`（避免无效 missing policy），并补齐相关 i18n 文案与 toast 文案。
   - `type=g` 在角色矩阵/用户权限页进行语义化展示（Role/ParentRole，action=*，effect=allow）。
   - Unauthorized/PolicyInspector 的 diff 默认结构化渲染，并提供 raw diff + 复制能力。
   - 业务页面（HRM Employees）在无 create/update/delete 权限时，提供禁用态 + 申请入口 + 提示（partial authorization）。
   - CopyButton 文案收敛并纳入 i18n。
5. **CI 修复**：
   - 修复 `templ fmt .`/`go fmt` 在 CI 的格式化差异（确保 quality-gates 通过）。
6. **质量门禁回归**：
   - 已跑 `make check lint`、`make authz-test`、`make authz-lint`、`make check tr`、`go vet ./...` 与相关 `go test`。
   - 补齐 ensureAuthz 的 JSON/HTMX/HTML fallback 403 契约回归测试。
7. **P0-3 用户页 Effective 权限汇总闭环**：
   - 用户权限页新增 Effective 汇总视图（按 domain/object/action 聚合）并展示来源链路（用户直配 + 角色继承链）。
   - g 继承项支持直达对应角色策略矩阵（从角色名/ID 解析到 `/roles/{id}/policies`），避免仅靠搜索定位。
8. **P0-4 文案与 i18n 收敛**：
   - 清理残留硬编码英文/组件级文案，并补齐四语种 locale key（含表单/错误页/布尔值/Effect 文案等）。

### 下一步计划
1. **a11y 复验与记录（P2）**：补齐核心页面 axe smoke + 键盘巡检记录（对齐 015B 验收），并沉淀为可复用的回归清单。
2. **语义深化（P1）**：继续推进 role->permission explainability（更完整的来源解释/排障能力、对象/动作字典化进一步收敛）。

#### P0-8：403 契约与 fallback 回归验证清单（建议）
- **REST(JSON)**：对任意受保护 endpoint，带 `Accept: application/json` 时返回 `403` 且 `Content-Type: application/json`；payload 至少包含 `error/object/action/subject/domain/missing_policies/suggest_diff/request_url/debug_url/base_revision/request_id`（其中 `base_revision/request_id` 允许为空但推荐透出）。
- **HTMX(HTML)**：对页面类 endpoint，带 `Hx-Request: true` 且非 JSON Accept 时返回 `403`，并设置 `Hx-Retarget: body`、`Hx-Reswap: innerHTML`；body 渲染 `components/authorization/unauthorized.templ`（至少包含 `data-authz-container` 与 `data-request-url="/core/api/authz/requests"`）。
- **非 HTMX(HTML)**：对页面类 endpoint，非 JSON Accept 时返回 `403` 并渲染 `unauthorized.templ`；若缺失 PageContext（极端 fallback），允许退化为 `http.Error` 的纯文本 `Forbidden:` 信息，但需确保不会 200/静默失败。
- **Request ID/Revision**：设置 `X-Request-ID` 时，JSON payload 的 `request_id` 与 UI 展示应一致；`base_revision` 需与 `config/access/policy.csv.rev` 保持同步（过期时由 API 通过 `X-Authz-Base-Revision`/meta 提示刷新）。
- **自动化覆盖建议**：至少覆盖 Core/HRM/Logging 三模块的 `ensure*Authz`（JSON/HTMX/HTML fallback）与关键 API（`/core/api/authz/requests` 的 `AUTHZ_INVALID_REQUEST`、`Hx-Trigger: showErrorToast/notify`）。
