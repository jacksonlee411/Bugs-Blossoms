# DEV-PLAN-055：Position UI（Org UI 集成 + 本地化）（对齐 051 阶段 C-UI）

**状态**: 草拟中（2025-12-20 05:06 UTC）

## 0. 评审结论（已采纳）
- **IA 选型**：沿用 Org UI 既有“左树右面板 + 顶部 Subnav Tabs”模式，在 `/org/positions` 增加第三个 Tab（不新增全局 Sidebar 子项，避免入口重复与信息架构分叉）。
- **复用优先**：参数化 `orgui.Tree` 的“点击请求 URL + push-url 目标 + hx-target”，避免复制树组件（保持 035 的节点页行为不变）。
- **分页策略**：采用 cursor-based（“加载更多/下一页”）而非 offset；排序固定为 `code ASC, id ASC`（缺省 code 时回退到 id），确保稳定与可复现。
- **System/Managed 边界**：默认 `show_system=0` 隐藏 system/auto-created；允许在 UI 里 toggle 展示（仅影响展示，system 永远只读且不暴露治理操作入口）。
- **写入范围**：v1 UI 交付 Create/Update/Transfer（业务写）；Correct/Rescind/ShiftBoundary 作为强治理能力 v1 不做 UI 表单（由 API/脚本/后续计划承接），但 UI 必须提供清晰的错误引导与权限申请入口（403 契约一致）。
- **占编联动**：v1 UI 以“占编摘要展示 + 跳转/联动 Assignments 页”为主；如 053 已提供 position 视角的占用写入口，则在详情页提供“快速分配（可选）”。
- **测试策略**：补齐 Positions UI 的 403 UX（HTMX/Full page）与最小 happy path（读 + create/update）e2e；必要的 controller 单测锁定 403 契约与关键 partial 输出。

## 1. 背景与上下文 (Context)
- **需求来源**：[DEV-PLAN-050](050-position-management-business-requirements.md)（列表/详情/时间线、有效期治理操作、占编与空缺提示、统计入口、权限边界）；[DEV-PLAN-051](051-position-management-implementation-blueprint.md) 阶段 C-UI。
- **现状**：
  - Org UI（`/org/nodes`、`/org/assignments`）已由 [DEV-PLAN-035](035-org-ui.md) 落地，并在 [DEV-PLAN-035A](035A-org-ui-ia-and-sidebar-integration.md) 明确了页面壳与侧栏集成契约。
  - 仓库内 Position 目前主要用于“Assignment 自动创建空壳 Position”的主链（System Position），并不满足 050 的“业务可管理 Position”闭环。
- **目标用户**：具备 Org/Staffing 权限的 HR 管理员/组织管理员（租户用户）。
- **技术约束**：
  - UI 采用 server-rendered `templ` + `HTMX`（对齐 035），不引入 SPA。
  - 交互优先复用 `pkg/htmx` 与 `components/` 组件；禁止手改 `.templ` 生成物，必须通过工具生成并提交。
  - 路由分层对齐 [DEV-PLAN-018](018-routing-strategy.md)：UI 在 `/org/*`；JSON-only 内部 API 在 `/org/api/*`（UI 控制器可直接调用 service，不通过 HTTP 调用自身 API）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **Org UI 集成**：在 Org UI 的导航与子导航（Tabs）中加入 Position 入口，并能从组织树定位到“某组织单元”的 Position 列表与详情。
- [ ] **读体验优先**：列表/详情/时间线（as-of 视角）可用；字段展示与口径对齐 052/053 的 v1 合同（避免 UI 与 API 漂移）。
- [ ] **最小写入闭环（可演示）**：
  - [ ] Position：创建、Update（新增版本）、Transfer（组织转移，若为单独命令）、维护 reports-to（若纳入 v1）。
  - [ ] 强治理（admin）：v1 不交付 Correct/Rescind/ShiftBoundary 的 UI 表单；但当服务端提示“需强治理”或返回 403/409 时，UI 必须提供一致的错误解释与后续动作指引（申请 admin 权限/改用 Correct 等）。
  - [ ] Assignment：最小占用/释放闭环（用于演示 `occupied_fte`/填充状态派生与“超编阻断”的错误反馈）。
- [ ] **权限驱动 UI**：按钮显隐与服务端鉴权一致；无权限时 UX 可理解且可申请（对齐 035/054/026 的 403 契约）。
- [ ] **本地化与门禁对齐**：新增文案具备 locales；`.templ`/Tailwind 生成物齐全且提交；按触发器矩阵通过本地门禁。

### 2.2 非目标（Out of Scope）
- 不交付完整统计/看板/空缺分析页面（见 [DEV-PLAN-057](057-position-reporting-and-operations.md)）。
- 不交付 Job Catalog / Job Profile / Position Restrictions 的维护 UI（见 [DEV-PLAN-056](056-job-catalog-profile-and-position-restrictions.md)）。
- 不在本计划内引入 OrgNode 范围级 ABAC/行级权限（Casbin attrs 或服务层 scope），本阶段以 coarse-grained object/action 为主（见 054）。
- 不在本计划内做复杂的“批处理/预检/草稿审批/What-if/Impact UI”（对齐 Org 的 030 轨道）。

## 2.3 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器/工具链”，避免复制命令细节导致 drift；具体命令以 `AGENTS.md`/`Makefile`/`.github/workflows/quality-gates.yml` 为准。

- **触发器清单（勾选本计划命中的项）**：
  - [ ] Go 代码（新增/扩展 UI controller、viewmodels、mappers）
  - [ ] `.templ` / Tailwind（新增页面与组件，需生成物提交）
  - [ ] 多语言 JSON（Org UI 文案 keys）
  - [ ] Authz（按钮显隐/403 契约复用；若变更策略碎片则命中 054 流程）
  - [ ] 路由治理（通常不命中：`/org` 前缀已在 allowlist；若新增顶级前缀才命中）
  - [ ] 文档（本计划）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org UI 契约（HTMX/403/页面壳）：`docs/dev-plans/035-org-ui.md`
  - Org UI IA/侧栏集成：`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
  - 路由策略：`docs/dev-plans/018-routing-strategy.md`
  - Position Core（服务/API）：`docs/dev-plans/053-position-core-schema-service-api.md`
  - Authz（object/action 与门禁）：`docs/dev-plans/054-position-authz-policy-and-gates.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  B[Browser] -->|HTMX requests| C[Org UI Controller (/org/*)]
  C -->|render| T[templ templates/components]
  C -->|call| S[Org Staffing Services (053)]
  S --> R[Repositories]
  R --> DB[(org_positions/org_assignments + audit)]
  C --> A[Authz (pkg/authz + casbin)]
```

### 3.2 关键设计决策（必须对齐 035/054/053）
1. **页面壳与路由落点（选定）**
   - 页面最外层统一使用 `layouts.Authenticated`（对齐 035A）。
   - Position UI 路由统一落在 `/org/positions`（仍在 `/org` 前缀下，符合 allowlist 与模块聚合方式）。
2. **状态驻留在 URL（选定）**
   - `effective_date`、`node_id`、`position_id`、filters 通过 query string 表达，并通过 `hx-push-url` / `htmx.PushUrl` 固化，保证可刷新/可分享/可回退。
3. **master-detail + 局部刷新（选定）**
   - 复用 Org 树作为“组织上下文锚点”，右侧为 Position 管理区（列表 + 详情 + 时间线），HTMX 局部刷新为主。
4. **System vs Managed 的 UI 边界（选定，具体口径以 052 冻结）**
   - 默认只展示 Managed；System/auto-created 仅在显式开关 `show_system=1` 时展示；System 永远只读且不暴露治理操作入口。
5. **错误与 403 体验（选定）**
   - 403 统一复用 `layouts.WriteAuthzForbiddenResponse`（对齐 035），使 Full page/HTMX/JSON 的拒绝体验一致。
   - 表单校验类错误：422 返回带错误信息的表单片段；冲突/冻结窗口/重叠等治理错误：409 返回表单片段并展示错误码/信息（错误码 SSOT 以 053/025 为准）。
6. **Tree 组件复用（选定）**
   - `orgui.Tree` 需要支持可配置的“点击请求 URL（hx-get）/push-url/target 容器”，以便在 `/org/nodes` 与 `/org/positions` 复用同一树组件。
7. **分页（选定：cursor-based）**
   - cursor 视为 opaque 字符串；UI 只负责透传 `cursor` 并渲染“下一页/加载更多”按钮，不在前端拼装 cursor。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本节定义 UI 层“要展示/要编辑”的字段口径与边界；字段最终清单以 052/053 冻结的 v1 合同为准，但 UI 必须以此为约束，避免实现中发散。

### 4.1 Position（列表行）最小字段集（v1）
- 标识：`position_id (uuid)`、`code (string)`、`title (string|null)`
- 归属：`org_node_id (uuid)`、`org_node_label (string)`（用于列表显示与面包屑）
- 生命周期：`lifecycle_status (enum)`（对齐 052 的映射；UI 使用 badge 展示）
- 有效期窗：`effective_date/end_date (time)`（UTC，`YYYY-MM-DD` 展示）
- 类型标记：`is_system/is_auto_created (bool)`（用于 Managed/System 分流）
- 占编摘要（若 053 v1 已交付）：
  - `capacity_fte (decimal)`、`occupied_fte (decimal)`、`available_fte (decimal)`
  - `staffing_state (enum)`：EMPTY/PARTIALLY_FILLED/FILLED（可选 `vacant` 标签）

### 4.2 Position（详情）最小字段集（v1）
- 列表行字段 +：
  - `reports_to_position_id (uuid|null)`（若纳入 v1）
  - `job_catalog_path/job_profile/restrictions` 等摘要（若 056 已交付则展示；未交付则隐藏或显示“未启用”）
- 审计摘要（来源以 025/053 为准）：
  - `last_changed_at/last_changed_by`（用于详情页“最近变更”）
  - `reason_code`（若纳入 v1 且可读取）
  - `request_id`（用于排障/串联审计）

### 4.3 时间线（timeline）字段
- 每个版本最小信息：`effective_window + lifecycle_status + org_node + reason + operator + transaction_time`
- 规则：时间线只读视图必须与 as-of 查询一致；UI 不自行拼接时间片逻辑。

### 4.4 表单字段与校验边界（对齐 050 的“必填”，最终以 052/053 为准）
- Create/Update 表单必须能表达（至少）：
  - `org_node_id`（默认为当前选中节点）
  - `code/title`（或其替代字段，以 053 v1 为准）
  - `effective_date`（默认页面 effective_date；Update 允许未来）
  - `capacity_fte`（若 v1 已交付；否则作为后续增强项隐藏）
  - `reason_code + reason_text`（若 v1 已纳入；否则以 audit meta 代替）
- Correct/Rescind/ShiftBoundary（强治理）：v1 不做 UI 表单；但冻结窗口/冲突/权限不足等治理拒绝必须输出可理解的错误信息，并给出建议动作（申请 `admin`、改用 Correct 等）。

## 5. 页面信息架构与路由 (IA & Routes)
### 5.1 导航与入口（对齐 035A）
- **全局 Sidebar（选定）**：保持单入口 `OrgLink -> /org/nodes`（对齐现有实现），避免出现“两个 Org 入口”的信息架构分叉。
- **Subnav Tabs（选定）**：在 `orgui.Subnav` 增加第三个 Tab：
  - Structure：`/org/nodes`
  - Assignments：`/org/assignments`
  - Positions：`/org/positions`
  - Positions Tab 可见性：`pageCtx.CanAuthz("org.positions","read")` 为真才展示（对齐 054 object/action）；并在 `/org/nodes`/`/org/assignments` 页面预加载 `org.positions read` capability，避免“有权限但 Tab 不出现”。

### 5.2 页面布局（建议）
> 对齐既有 `/org/nodes` 的“左树右面板”布局，降低认知成本。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Header: Title + effective_date picker（hx-get + hx-push-url）                │
│ Tabs: Structure / Assignments / Positions                                    │
└─────────────────────────────────────────────────────────────────────────────┘
┌───────────────────────────────┬─────────────────────────────────────────────┐
│ 左侧：Org Tree（node_id 锚点） │ 右侧：Positions 面板                         │
│ - 选择节点 -> 刷新列表         │ - Filters + Create                           │
│ - hx-push-url 固化 node_id     │ - 列表（分页）                               │
│                               │ - 详情（含时间线与操作区）                    │
└───────────────────────────────┴─────────────────────────────────────────────┘
```

### 5.3 URL 状态（v1）
- 页面：`GET /org/positions`
- Query（建议最小集合）：
  - `effective_date=YYYY-MM-DD`（必需；缺省为 UTC today）
  - `node_id=<uuid>`（可选；选中组织节点）
  - `position_id=<uuid>`（可选；选中职位）
  - `q=<string>`（可选；关键词：title/code）
  - `include_descendants=0|1`（可选；默认 1）
  - `lifecycle_status=<enum>`（可选；对齐 052/053 的状态映射）
  - `staffing_state=<enum>`（可选；EMPTY/PARTIALLY_FILLED/FILLED + 可选 VACANT 标签）
  - `show_system=0|1`（可选；默认 0）
  - `cursor=<opaque>`（可选；分页游标，具体实现可按 053 的 list contract）

## 6. 交互契约 (HTMX Contracts)
> UI 路由在 `/org/*`（HTML/HTMX）；内部 JSON API 在 `/org/api/*`（若 UI 选择直接调用 service，可不经由 `/org/api`，但口径与错误码需对齐 053）。

### 6.1 页面与 partial（建议）
- `GET /org/positions?effective_date=...`：PositionsPage（全页）
- `GET /org/positions/panel?effective_date=...&node_id=...&...`：仅渲染右侧面板（用于树选择/筛选的局部刷新）
- `GET /org/positions/list?effective_date=...&node_id=...&...`：列表区（table/list）
- `GET /org/positions/{id}?effective_date=...`：详情区（基本信息 + 占编摘要 + 操作入口）
- `GET /org/positions/{id}/timeline?effective_date=...`：时间线区（版本列表）

### 6.2 写入（建议）
> 写入契约字段以 053 v1 为准；本计划只固化“交互形态与刷新策略”。

- `GET /org/positions/new?effective_date=...&node_id=...`：创建表单（HTMX 片段）
- `POST /org/positions?effective_date=...`：创建（成功后刷新列表 + 详情 + push-url）
- `GET /org/positions/{id}/edit?effective_date=...`：编辑表单（HTMX 片段）
- `PATCH /org/positions/{id}?effective_date=...`：Update（Insert 时间片语义，成功后刷新）
- `POST /org/positions/{id}:transfer?effective_date=...`：组织转移（若 053 选择独立命令；否则用 Update 表单承载）
- `POST /org/positions/{id}:correct|:rescind|:shift-boundary?effective_date=...`：强治理（v1 不做 UI 表单；如 053 已提供端点，v1 仅保证 403/409 错误展示与排障引导一致）

### 6.3 局部刷新与 OOB（建议）
- 树选中节点：`hx-get="/org/positions/panel?...&node_id=..."` 更新右侧面板；同时 `hx-push-url` 固化 `node_id`。
- 列表点击行：`hx-get="/org/positions/{id}?effective_date=..."` 更新详情区，并 `hx-push-url` 固化 `position_id`（保留 filters）。
- 写入成功：服务端使用 `htmx.PushUrl` 更新 URL（包含 `node_id`/`position_id`），并用 `hx-swap-oob="true"` 同步刷新：
  - 列表区（新行/状态变化/占编变化）
  - 详情区（最新快照）
  - 时间线区（若当前选中 position）

## 7. 核心交互逻辑 (Business Logic & UX Flows)
### 7.1 组织视角 → 职位列表
1. 用户进入 `/org/positions?effective_date=...`。
2. 选择组织树节点：
   - 右侧加载该节点（可选含下级）下的 Position 列表。
   - 列表默认只展示 Managed（`show_system=0`）。

### 7.2 列表过滤与分页
- 过滤项变更（q/status/staffing_state/include_descendants/show_system）触发 `hx-get` 刷新列表（必要时也刷新详情区为空状态）。
- 分页采用 cursor 并保持 filters 不丢失；点击“下一页/加载更多”仅刷新列表区。

### 7.3 详情与时间线
- 点击 Position 行加载详情区（基本字段 + 占编摘要 + actions）。
- 时间线区默认展示“版本列表”，并允许切换：
  - “按 as-of 日期展示当前版本”
  - “查看所有版本 + 变更原因/操作者/变更时间”（来源审计，口径对齐 025/053）

### 7.4 写入：Create / Update / Transfer（最小可演示）
- Create：从列表页点击“创建职位”，表单默认带入 `org_node_id=node_id` 与 `effective_date`。
- Update：在详情页点击“更新”，以 Insert 语义创建新版本（字段可变性与必填项对齐 052/053）。
- Transfer：若为独立命令，表单仅改组织归属；否则作为 Update 表单中的一个字段（均需强制记录 reason）。

### 7.5 强治理：Correct / Rescind / ShiftBoundary（admin）
- v1 UI 不提供强治理表单入口（避免在 UI 端引入高风险写操作与额外交互复杂度）；但必须满足：
  - API 返回的 403/409 能在 UI 中被清晰展示（含 `request_id`，并指向 `/core/api/authz/requests` 与 `/core/api/authz/debug`）。
  - 当服务层提示“需改用 Correct/ShiftBoundary”时，UI 给出明确引导（对齐 025 的 `USE_CORRECT` 类错误口径）。

### 7.6 占编（Assignment）最小闭环（用于演示派生口径）
- 详情页展示占编摘要：`capacity_fte / occupied_fte / available_fte / staffing_state`（字段口径以 052/053 为准）。
- v1 优先复用既有 Assignments 页能力（`/org/assignments`），在 Position 详情页提供：
  - 只读占编摘要（如 053 已提供该口径）
  - “查看该人员分配时间线/跳转到 Assignments 页”的联动入口
  -（可选）快速分配：输入 pernr（以及可选 fte/日期，取决于 053 v1 合同）并写入成功后刷新摘要
- 超编/冲突必须被阻断，并以可理解的错误提示返回（409/422，错误码 SSOT 以 053 为准）。

## 8. 安全与鉴权 (Security & Authz)
### 8.1 鉴权入口（对齐 035）
- Controller 统一使用 `ensureOrgAuthzUI`（或等价封装）做服务端强制鉴权；鉴权失败统一 `layouts.WriteAuthzForbiddenResponse`。
- Template 中使用 `pageCtx.CanAuthz(object, action)` 控制按钮/链接可见性（但不可替代服务端鉴权）。

### 8.2 object/action（对齐 054）
- 页面与读能力：`org.positions read`
- Position 写入：
  - Create/Update/Transfer：`org.positions write`
  - Correct/Rescind/ShiftBoundary：`org.positions admin`
- 占编（Assignment）：
  - 读：`org.assignments read`
  - 写（占用/释放）：`org.assignments assign`
  - 强治理：`org.assignments admin`
- 若引入统计入口：`org.position_reports read`（具体页面由 057 落地）

## 9. 本地化（i18n）与文案约定
- 文案 keys 归属：`modules/org/presentation/locales/{en,zh,ru,uz}.json`
- 建议命名空间：
  - `Org.UI.Positions.*`（列表/详情/时间线/表单/filters）
  - 与既有 `Org.UI.Shared.*`、`Org.UI.Tabs.*` 复用，避免重复 keys。
- 触发翻译门禁时（新增 keys）：按 `AGENTS.md` 跑 `make check tr`。

## 10. 测试与验收标准 (Acceptance Criteria)
- **导航与状态**：
  - `/org/positions` 可从侧栏/Tab 进入；`effective_date/node_id/filters` 可通过 URL 复现。
  - 切换 `effective_date` 会刷新列表与详情口径（as-of 一致）。
- **权限**：
  - 无 `org.positions read`：入口不可见；直接访问返回 403（Full page/HTMX 体验一致）。
  - 无 `write/admin`：对应按钮隐藏/禁用且服务端也会 403。
- **核心闭环（最小演示）**：
  - Create/Update（及 Transfer 若单独命令）可用；写入成功后列表与详情即时刷新。
  - 占编最小闭环可演示 `occupied_fte` 变化与超编阻断。
- **E2E（建议最小集）**：
  - 未授权访问 `/org/positions`：403 UX 与 035 保持一致（Full page + HTMX）。
  - 有 `org.positions read`：可见 Positions Tab，且能按 `effective_date/node_id` 查看列表与详情。
  - 有 `org.positions write`：可 Create/Update（happy path），并能看到 409/422 的错误反馈与 request_id。
- **门禁**：
  - `.templ`/Tailwind：生成物提交且 `git status --short` 干净。
  -（若命中）`make check tr`、`make check lint`、`make test` 等对齐触发器矩阵。

## 11. 运维与监控 (Ops & Monitoring)
- **开关/灰度**：建议复用 Org 已有 rollout 机制（例如 `OrgRolloutEnabledForTenant`）控制页面可见性；写入能力的灰度/只读策略与回滚路径以 [DEV-PLAN-059](059-position-rollout-readiness-and-observability.md) 为准。
- **可观测性**：
  - UI 必须在错误提示中保留 `request_id`（或提供复制入口），便于串联审计/日志排障（对齐 025/059 的口径）。
  - 关键治理拒绝（冻结窗口、超编、时间线冲突）应能从 UI 反馈定位到“缺失权限/冲突原因/建议操作”（例如改用 Correct、申请 admin 权限）。

## 12. 依赖与里程碑 (Dependencies & Milestones)
### 12.1 依赖
- [DEV-PLAN-052](052-position-contract-freeze-and-decisions.md)：System/Managed 边界、字段可变性矩阵、强治理边界冻结。
- [DEV-PLAN-053](053-position-core-schema-service-api.md)：Position/Assignment v1 服务能力与错误码口径稳定。
- [DEV-PLAN-054](054-position-authz-policy-and-gates.md)：`org.positions` 等 object/action 与策略碎片落地（至少 superadmin 回归可用）。
-（可选）[DEV-PLAN-056](056-job-catalog-profile-and-position-restrictions.md)：若 UI 需要展示/编辑 Catalog/Profile/Restrictions 的字段或摘要。

### 12.2 里程碑（建议）
1. [ ] IA 与路由契约冻结（§5-§6）
2. [ ] Positions Page（树 + 列表 + 详情骨架）可用（读）
3. [ ] Create/Update 最小写入闭环可演示
4. [ ] 强治理错误引导与 403/错误反馈一致（无表单）
5. [ ] 本地化 keys 补齐 + 生成物/门禁通过

## 13. 实施步骤
1. [ ] 扩展 Org UI 导航与 Tab（对齐 035A）
2. [ ] 新增 `/org/positions` 页面与右侧面板骨架（复用树与布局）
3. [ ] Position 列表（filters + 分页）与详情/时间线 partial
4. [ ] Create/Update/Transfer 表单与写入后刷新（HTMX + OOB）
5. [ ] Assignment 占编最小闭环（展示 + 最小写入）
6. [ ] locales 补齐 + 生成物提交（`.templ`/Tailwind）
7. [ ] 验收与门禁记录（执行时填写；结果登记到 059 的 readiness 约定）

## 14. 交付物
- Org UI 中的 Position 页面（树上下文 + 列表/详情/时间线）与最小写入闭环（含占编演示）。
- 本地化 locales keys 与必要生成物（确保 CI 不因生成漂移失败）。
