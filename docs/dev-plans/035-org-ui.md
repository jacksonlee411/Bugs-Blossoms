# DEV-PLAN-035：组织机构模块 M1 前端界面（templ + HTMX）

**状态**: 规划中（2025-12-14 23:54 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**: [DEV-PLAN-020](020-organization-lifecycle.md) 的步骤 6A。
- **目标用户**: HR 管理员/组织管理员（具备 `Org.Read/Org.Write/Org.Assign` 权限的租户用户）。
- **现状与痛点**:
  - Org 模块的主链能力（Person → Position → Org）与 API/Authz/事件闭环分别由 [DEV-PLAN-024](024-org-crud-mainline.md)、[DEV-PLAN-026](026-org-api-authz-and-events.md) 交付。
  - 目前缺少“可用且可操作”的管理界面，导致组织主数据仍需通过脚本或临时页面维护，无法支撑日常 HR 操作与验证。
- **技术约束**:
  - UI 采用 server-rendered `templ` + `HTMX`（项目既有栈），不引入 SPA。
  - Authz 403 体验与能力暴露遵循 [DEV-PLAN-014D](014D-casbin-public-layer-ui-interface.md)/[DEV-PLAN-015](015-casbin-policy-ui-and-workflow.md) 以及既有 `components/authorization/unauthorized.templ`。
- **关键术语**:
  - `effective_date`: “as-of” 查询点；M1 UI 统一以 `YYYY-MM-DD`（UTC）作为输入格式，并在服务层转换为 `time.Time`。
  - `pernr`: 人员编号（`string`）。M1 临时采用 `HRM employees.id`（十进制字符串）作为 `pernr`，后续可替换为真实工号并通过迁移/映射兼容。
  - `subject_id`: Org 内部主体标识（`uuid`）。服务端通过确定性映射从 `(tenant_id, subject_type, pernr)` 生成，前端不生成 `subject_id`。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **组织树视图**：交付可交互的树形视图，展示 `OrgUnit` 单树，并可随 `effective_date` 切换刷新。
- [ ] **节点管理**：交付 OrgNode 的“查看 + 创建 + 编辑（Insert 时间片语义）”表单与详情面板。
- [ ] **职位与分配管理（M1 主链）**：
  - [ ] 在分配时间线/详情中展示 Position 信息（只读；字段以 schema/API 为准）。
  - [ ] 交付 Assignment 的“创建 + 编辑（Insert 时间片语义） + 时间线查看”，并覆盖后端“自动创建空壳 Position”的交互闭环。
- [ ] **授权驱动 UI**：所有页面与按钮的可见性、可操作性均受 `Org.Read/Org.Write/Org.Assign` 控制；无权访问统一走 403 契约。
- [ ] **HTMX 局部刷新体验**：树、详情、表单提交使用局部刷新；提交成功后自动更新相关区域（树/详情/时间线）。
- [ ] **门禁对齐**：`.templ`/Tailwind 变更后执行 `make generate && make css` 并保证生成物已提交（`git status --short` 干净）。

### 2.2 非目标 (Out of Scope)
- 不实现审批流/草稿/预检/What-if/Impact UI（见 [DEV-PLAN-030](030-org-change-requests-and-preflight.md) 与 [DEV-PLAN-020](020-organization-lifecycle.md) 后续阶段）。
- 不实现多层级树（Company/Cost/Custom）与矩阵关系的写入体验（M1 仅 OrgUnit 单树）。
- 不提供独立的 Position CRUD 页面/表单（M1 仅做展示；如需管理 Position 另起计划或在后续里程碑扩展）。
- 不在 M1 实现复杂的“人员全文检索/组织批量拖拽调整”；人员选择器优先满足“可用的分页选择”，检索增强可后续立项。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  B[Browser] -->|HTMX requests| O[Org Presentation Controllers]
  O -->|render| T[templ templates/components]
  O -->|call| S[Org Services]
  S --> R[Org Repositories]
  O -->|optional lookup| H[HRM Employee Service]
  O --> A[Authz (pkg/authz + casbin)]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：UI 采用 server-rendered `templ` + `HTMX`（选定）**
  - 选项 A：SPA（React/Vue）。缺点：引入新栈与构建链路，Authz/403 契约复用成本高。
  - 选项 B：纯服务端全页跳转。缺点：交互割裂，树/表单体验差。
  - 选项 C（选定）：`templ` + `HTMX`。优点：对齐现有项目实践，可用局部刷新实现“接近 SPA”的体验。
- **决策 2：业务逻辑单一来源**
  - UI 控制器不重复实现写路径逻辑；复用 Org Service（与 [DEV-PLAN-024](024-org-crud-mainline.md)/[DEV-PLAN-026](026-org-api-authz-and-events.md) 同源），保证时间线语义与错误码一致。
- **决策 3：`effective_date` URL 化**
  - 统一使用 query 参数 `effective_date=YYYY-MM-DD`；在写操作成功后，通过响应头 `Hx-Push-Url` 将浏览器 URL 更新为对应的页面路由（例如 `/org/nodes?effective_date=...`），避免 URL 停留在 `POST/PATCH` 路径且保证“写后可见”。
- **决策 4：局部刷新策略**
  - 表单提交成功后，优先使用 OOB（out-of-band swap）同步刷新组织树与相关面板；`HX-Trigger` 仅用于 toast/轻量事件通知（避免前端再发额外请求）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 URL 与查询参数约束
- `effective_date`: `YYYY-MM-DD`（UTC，`time.DateOnly`），缺省为“今天（UTC）”。
  - **读（as-of）**：以 URL query 的 `effective_date` 作为查询点。
  - **写（Insert）**：以表单字段 `effective_date` 作为写入时间片起点；若表单未提供则默认使用 URL query 的 `effective_date`。
- `type`: 固定 `OrgUnit`（M1）；其它类型在后续阶段启用时再扩展。
- `subject`: `person:{id}`（与 [DEV-PLAN-024](024-org-crud-mainline.md) 对齐），M1 支持两种 `id`：
  - `person:{pernr}`：`pernr` 为租户内稳定的人员编号（`string`）。**M1 规范用法**。
  - `person:{uuid}`：当 `id` 可解析为 UUID 时，服务端可直接视为 `subject_id`（便于 API 客户端/后续演进）。

### 4.2 ViewModel（建议最小集）
> 说明：字段名仅用于消除歧义；最终以实现时的 Go 类型为准。

- `OrgPageVM`:
  - `EffectiveDate string`（`YYYY-MM-DD`）
  - `Tree []OrgTreeNodeVM`
  - `SelectedNodeID *uuid.UUID`
- `OrgTreeNodeVM`:
  - `ID uuid.UUID`
  - `Code string`
  - `Name string`（展示名，来自 slice 的 as-of 视图）
  - `Depth int`
  - `HasChildren bool`
  - `IsSelected bool`
  - `CanView bool` / `CanWrite bool`
- `OrgNodeFormVM`:
  - `ID *uuid.UUID`（new 时为空）
  - `Code string`
  - `Name string`
  - `I18nNames map[string]string`（可选）
  - `ParentNodeID *uuid.UUID`
  - `LegalEntityID *uuid.UUID`（或 `CompanyCode string`，以 schema 为准）
  - `LocationID *uuid.UUID`
  - `DisplayOrder int`
  - `EffectiveDate string`
  - `Errors map[string]string`
- `AssignmentTimelineVM`:
  - `Subject string`（`person:{id}`）
  - `PERNR string`（用于表单回显与可读展示）
  - `Items []AssignmentItemVM`
  - `Errors map[string]string`
- `AssignmentItemVM`:
  - `ID uuid.UUID`（或 schema 对应主键类型）
  - `OrgNodeID uuid.UUID`
  - `OrgNodeName string`
  - `PositionID uuid.UUID`
  - `PositionTitle string`
  - `AssignmentType string`
  - `IsPrimary bool`
  - `EffectiveDate string`（`YYYY-MM-DD`）
  - `EndDate string`（`YYYY-MM-DD`）

### 4.3 表单校验与错误展示
- DTO 校验失败：返回 422，并渲染带 `Errors` 的表单片段（对齐现有 `composables.UseForm(...).Ok(...)` 模式）。
- 业务规则失败（如时间重叠/冻结窗口）：错误映射为可读提示并在表单顶部展示；错误码与语义以 [DEV-PLAN-025](025-org-time-and-audit.md)/[DEV-PLAN-026](026-org-api-authz-and-events.md) 为准。

### 4.4 Person 标识与确定性映射（M1 契约）
- **输入（UI）**：以 `pernr`（string）作为用户可输入/可选择的人员标识；用于 URL `subject=person:{pernr}` 与表单字段 `pernr`。
- **存储（DB）**：`org_assignments.subject_id` 为 `uuid not null`（见 [DEV-PLAN-021](021-org-schema-and-constraints.md)），服务端必须在写入时生成或校验该值。
- **映射算法（服务端）**：`subject_id = UUIDv5_SHA1(namespace, tenant_id + ":" + subject_type + ":" + pernr)`。
  - `subject_type`：M1 固定为 `person`。
  - `namespace`：使用项目内固定常量（需与导入工具 [DEV-PLAN-023](023-org-import-rollback-and-readiness.md) 保持一致）。
- **校验规则**：
  - 若请求中同时包含 `subject_id` 与 `pernr`，服务端必须校验二者一致（不一致返回 422）。
  - 前端不生成 `subject_id`，仅传 `pernr`（或 `subject`），避免多端算法漂移。

## 5. 接口契约 (API Contracts)
> 标准：需要同时定义页面路由、HTMX partial 的 URL/Method/参数与错误行为；Authz 失败需返回统一 403 契约并渲染 Unauthorized 组件。
>
> 约定：UI 路由（`/org/*`）主要服务 HTML/HTMX；显式 `Accept: application/json` 时可返回 JSON 用于 E2E/诊断，但正式内部 API 应调用 `/org/api/*`（见 DEV-PLAN-026）。

### 5.0 内容协商规则（优先级）
> 对齐 `docs/dev-plans/018-routing-strategy.md` 的 5.1：显式 JSON（`Accept`）优先于 HTMX（`Hx-Request`）。

1. **显式 JSON**：若 `Accept` 包含 `application/json`，返回 JSON（用于 E2E/诊断；避免 JSON 请求误拿 HTML）。
2. **HTMX Partial**：若 `Hx-Request: true`，返回 HTML（partial/OOB）。
3. **默认页面**：其它情况返回 HTML（full page）。

说明：若同时满足 `Accept: application/json` 与 `Hx-Request: true`，以 **JSON 优先**；常规 HTMX 请求通常不会显式请求 JSON。

> 说明：403 Forbidden 的 payload/组件输出遵循项目统一 authz 契约（参考 `components/authorization/unauthorized.templ` 与 `modules/core/authzutil` 的 forbidden payload）。

### 5.1 页面路由（Full Page）
- `GET /org/nodes?effective_date=YYYY-MM-DD`
  - 200：返回包含“组织树 + 详情面板 + 生效日期选择器”的页面。
  - 403：返回 Unauthorized 页面（见 `components/authorization/unauthorized.templ`）。
- `GET /org/assignments?effective_date=YYYY-MM-DD&subject=person:{id}`（可选：独立页）
  - 200：返回“人员选择 + 分配时间线”的页面。

### 5.2 HTMX Partial（HTML）
#### 5.2.1 组织树
- `GET /org/hierarchies?type=OrgUnit&effective_date=YYYY-MM-DD`
  - 200：返回树组件 HTML（可包含 `id="org-tree"` 容器）。
  - 400：参数非法（如 `effective_date/type` 无法解析）。
  - 403：返回 Unauthorized 片段，并设置 `Hx-Retarget: body`、`Hx-Reswap: innerHTML`（对齐项目现有 403 HTMX 契约）。

#### 5.2.2 节点详情与表单
- `GET /org/nodes/{id}?effective_date=YYYY-MM-DD`
  - 200：返回节点详情片段（右侧面板）。
  - 404：节点不存在（返回空状态或 404 片段，二选一但需一致）。
- `GET /org/nodes/new?effective_date=YYYY-MM-DD&parent_id={uuid}`（可选）
  - 200：返回“新建节点表单”片段。
- `POST /org/nodes?effective_date=YYYY-MM-DD`（创建）
  - Form（示意）：`code,name,parent_id,legal_entity_id,location_id,display_order,effective_date`
  - 200：返回详情片段 +（OOB）刷新树；并设置 `Hx-Push-Url: /org/nodes?effective_date=<effective_date>`（以写入的 `effective_date` 为准）。
  - 403：无 `org.* write` 权限。
  - 422：返回带错误的表单片段。
  - 409：写入冲突（如 code 重复、时间线重叠、冻结窗口拒绝等；以 [DEV-PLAN-025](025-org-time-and-audit.md)/[DEV-PLAN-026](026-org-api-authz-and-events.md) 的错误码为准）。
- `PATCH /org/nodes/{id}?effective_date=YYYY-MM-DD`（更新；Insert 时间片语义）
  - Form（示意）：与创建类似，附带 slice 相关字段。
  - 成功/失败行为同创建。
  - 404：节点不存在。

#### 5.2.3 分配时间线与写入
- `GET /org/assignments?subject=person:{id}&effective_date=YYYY-MM-DD`
  - 200：返回该 subject 的分配时间线片段。
  - 400：参数非法（如 `effective_date/subject` 无法解析）。
  - 403：无 `org.* read` 权限。
- `POST /org/assignments?effective_date=YYYY-MM-DD`
  - Form（示意）：`pernr,org_node_id,position_id(optional),assignment_type(optional),effective_date`
  - 200：返回更新后的时间线片段；并设置 `Hx-Push-Url: /org/assignments?effective_date=<effective_date>&subject=person:<pernr>`（以写入的 `effective_date/pernr` 为准）。
  - 403：无 `org.* assign` 权限。
  - 422：返回带错误的表单片段。
  - 409：写入冲突（如时间线重叠/冻结窗口/主分配唯一性冲突等；以 [DEV-PLAN-025](025-org-time-and-audit.md)/[DEV-PLAN-026](026-org-api-authz-and-events.md) 的错误码为准）。
- `PATCH /org/assignments/{id}?effective_date=YYYY-MM-DD`（更新；Insert 时间片语义）
  - 成功/失败行为同创建。
  - 404：分配不存在。

## 6. 核心交互逻辑 (Business Logic & UX Flows)
### 6.1 树 → 详情联动
1. 用户点击树节点：HTMX `hx-get="/org/nodes/{id}?effective_date=..."` 更新右侧面板。
2. 右侧面板提供“编辑”入口：加载编辑表单片段。

### 6.2 表单提交后的同步刷新
1. 用户提交节点/分配表单：HTMX `POST`/`PATCH ...`。
2. 服务端成功后：
   - 返回新的详情/时间线片段；
   - 通过 OOB 刷新组织树与相关面板，避免前端额外请求（保持选中节点高亮与滚动位置尽量稳定）。

### 6.3 生效日期切换
1. 用户调整 `effective_date`：
   - 通过 `hx-get="/org/nodes"` + `hx-push-url="true"` 触发局部刷新（替换页面内容容器），并将 `effective_date` 固化到 URL。
2. 触发树与当前面板刷新（若当前有选中节点，则按新 `effective_date` 重取其 as-of 视图）。

## 7. 安全与鉴权 (Security & Authz)
- **鉴权入口**: 控制器层统一执行鉴权（对齐 [DEV-PLAN-026](026-org-api-authz-and-events.md) 的 object/action 口径），并调用统一 403 输出（HTML/HTMX）。
- **UI 可见性**: 模板中使用 `pageCtx.CanAuthz(object, action)` 控制按钮/链接展示，但不可替代服务端强制鉴权。
- **最小权限集（M1）**:
  - `org.* read`：允许查看树/节点详情/分配时间线。
  - `org.* write`：允许创建/更新组织节点（以及必要的结构写入，例如移动节点导致的边更新）。
  - `org.* assign`：允许创建/更新分配；当 `position_id` 为空时，允许服务端在同一事务内自动创建空壳 Position（不应额外要求 `org.* write`，避免“可分配但不能分配”的权限悖论）。
- **租户隔离**: 所有读写必须基于 Session/tenant 上下文；UI 不允许通过 query/path 访问其它租户数据。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 前置依赖
- [ ] [DEV-PLAN-026](026-org-api-authz-and-events.md)：`/org/**` API/Authz/时间线语义与错误码稳定。
- [ ] [DEV-PLAN-014D](014D-casbin-public-layer-ui-interface.md)/[DEV-PLAN-015](015-casbin-policy-ui-and-workflow.md)：Unauthorized 组件/权限申请体验可复用。
- [ ] `templ`/Tailwind 工具链可用：`make generate && make css` 可稳定运行且生成物可提交。

### 8.2 实施步骤（任务清单）
1. [ ] **目录与路由骨架**
   - [ ] 新增 `modules/org/presentation/controllers|templates|viewmodels|mappers`（对齐 DDD/cleanarch）。
   - [ ] 注册路由与导航：至少 `/org/nodes`（可选 `/org/assignments`）。
2. [ ] **页面布局与生效日期选择器**
   - [ ] 在 Org 页面布局中提供 `effective_date` 选择器（默认 UTC today），并能驱动刷新。
3. [ ] **组织树组件**
   - [ ] 实现树组件渲染（递归/分层/折叠），空状态/错误态/加载态可用。
   - [ ] 点击节点可加载详情面板并保持选中态。
4. [ ] **节点详情与表单**
   - [ ] 新建/编辑表单：字段覆盖 [DEV-PLAN-020](020-organization-lifecycle.md) 的 M1 必填字段（以 schema 为准）。
   - [ ] 提交成功刷新树与面板；校验失败返回 422 并展示错误。
5. [ ] **职位与分配**
   - [ ] 分配表单以 `pernr` 为输入（手动输入为默认路径），并能展示 `subject=person:{pernr}` 的分配时间线。
   - [ ] （可选增强）人员选择器：用可搜索组件（例如 `components/base/combobox`）按姓名/邮箱查询 HRM 员工并回填 `pernr`；若无 HRM 权限或查询失败则回退到手动输入。
   - [ ] 覆盖“自动创建空壳 Position”的交互（后端逻辑由 [DEV-PLAN-024](024-org-crud-mainline.md) 定义）。
6. [ ] **授权与 403 体验**
   - [ ] 模板内按 capability 控制可见性。
   - [ ] 控制器内强制鉴权并输出统一 403（页面与 HTMX）。
7. [ ] **测试**
   - [ ] 单元测试：`viewmodels/mappers` 基本映射与边界条件。
   - [ ] E2E：新增 `e2e/tests/org/` 场景覆盖（见 9.2）。

### 8.3 里程碑定义
- [ ] **M1**：组织树（只读）+ `effective_date` 切换可用。
- [ ] **M2**：节点/职位/分配的核心写入路径可用（创建/编辑 + 时间线查看）。
- [ ] **M3**：Authz/403 体验完成 + E2E 覆盖 + 门禁与 readiness 记录齐全。

### 8.4 交付物
- `modules/org/presentation/**`：controllers/templates/viewmodels/mappers。
- `modules/org/module.go`（及必要的 `links.go`）：路由与导航注册。
- `e2e/tests/org/**`：Playwright 用例。
- `docs/dev-records/`：关键交互与 readiness 命令记录（如需截图，放入 `docs/assets/` 并在记录中引用）。

## 9. 测试与验收标准 (Acceptance Criteria)
### 9.1 验收标准
- Org 页面在本地环境可用：树可浏览，节点/分配可按 `effective_date` 查询并完成主链操作。
- `subject=person:{pernr}` 可用：以 `pernr` 查询时间线与创建分配行为一致；服务端按 4.4 的规则确定性映射到 `subject_id`。
- 权限正确：
  - 无 `Org.Read`：访问 `/org/nodes` 返回 403（页面与 HTMX 都符合统一契约）。
  - 无 `Org.Write`：创建/编辑入口不可见且服务端拒绝写入。
  - 无 `Org.Assign`：分配入口不可见且服务端拒绝写入。
  - 仅有 `Org.Assign`（无 `Org.Write`）时：仍可创建分配并触发后端自动创建 Position。
- 生成物与门禁一致：
  - `make generate && make css` 后 `git status --short` 干净（无漏提交生成文件）。

### 9.2 E2E 覆盖（最小集）
- [ ] 管理员登录后可查看组织树。
- [ ] 创建新 OrgNode，树中可见且详情正确。
- [ ] 编辑 OrgNode（Insert 语义），在不同 `effective_date` 下可看到正确 as-of 视图。
- [ ] 为某 `person:{pernr}` 创建 Assignment（默认走手动输入 `pernr` 路径），并在时间线中可见。
- [ ] 无 `Org.Write` 账户：UI 不展示创建入口，且直接请求写接口返回 403。
- [ ] 无 `Org.Assign` 账户：UI 不展示分配入口，且直接请求分配写接口返回 403。
- [ ] 仅 `Org.Assign`（无 `Org.Write`）账户：创建分配成功（包含自动创建 Position 的路径）。
- [ ] 无 `Org.Read` 账户：直接访问 `/org/nodes` 返回 403 Unauthorized 页面。

### 9.3 Readiness（执行后在此记录）
> 执行后将 `[ ]` 改为 `[X]`，并补充时间戳、结果与必要链接。

- [ ] `make generate && make css` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败（附摘要）
- [ ] `git status --short` —— （YYYY-MM-DD HH:MM UTC）结果：必须为空
- [ ] `go fmt ./... && go vet ./...` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败
- [ ] `make check lint && make test` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败
- [ ] `cd e2e && pnpm test tests/org` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败

## 10. 运维与监控 (Ops & Monitoring)
- **Feature Flag（可选）**：如需灰度，引入 `ENABLE_ORG_UI`（默认关闭），逐租户开启。
- **关键日志**：在写操作路径记录 `request_id/tenant_id/effective_date/object/action`，便于排查时间线与权限问题。
- **回滚策略**：UI 级别可通过 Feature Flag 关闭入口；后端数据回滚遵循 [DEV-PLAN-023](023-org-import-rollback-and-readiness.md)/[DEV-PLAN-025](025-org-time-and-audit.md) 的脚本与审计口径。
