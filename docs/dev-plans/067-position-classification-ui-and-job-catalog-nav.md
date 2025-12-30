# DEV-PLAN-067：Position 分类字段 UI 贯通 + Job Catalog 维护入口（页内分组导航）

**状态**: 已完成（2025-12-28 11:45 UTC）
**对齐更新**：
- 2025-12-29：职位体系与 UI 已由 `DEV-PLAN-072` 对标 Workday 重新收口：Position 不再写入 `job_family_group_code/job_family_code/job_role_code`，改为仅选 `job_profile_id`（职位模板）+ 可选 `job_level_code`；`Job Role` 页签与相关写入口退场。本文保留为历史记录，不再作为实现 SSOT。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 业务需求：`docs/dev-plans/050-position-management-business-requirements.md`（职位类型/雇佣类型/Job Catalog 四级分类）。
  - 决策冻结：`docs/dev-plans/052-position-contract-freeze-and-decisions.md`（字段/合同冻结；Managed vs System）。
  - Position v1：`docs/dev-plans/053-position-core-schema-service-api.md`、`docs/dev-plans/053A-position-contract-fields-pass-through.md`。
  - UI IA：`docs/dev-plans/055-position-ui-org-integration.md`。
  - 主数据/校验：`docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`。
- **当前状态（实现盘点摘要）**：
  - Schema 已包含 `org_position_slices.position_type/employment_type/job_*_code/job_profile_id/cost_center_code`（见 `modules/org/infrastructure/persistence/schema/org-schema.sql`）。
  - Service & API 已可写入/更新上述字段，并在 Managed Position 上强制非空（`modules/org/services/org_service_053.go:21`、`modules/org/presentation/controllers/org_api_controller.go:2100`）。
  - Job Catalog/Profile 的主数据表与维护 API 已存在（`modules/org/presentation/controllers/org_api_controller.go:85`、`modules/org/presentation/controllers/org_api_controller.go:101`），并支持 `disabled/shadow/enforce` 校验灰度（`modules/org/infrastructure/persistence/schema/org-schema.sql:676`、`modules/org/services/position_catalog_validation_056.go:53`）。
  - Org UI 的 Positions 页面尚未展示/可编辑这些分类字段：Create 仍硬编码写入默认值（`modules/org/presentation/controllers/org_ui_controller.go:739`），Position 详情 viewmodel 也未承载这些字段（`modules/org/presentation/viewmodels/position.go:9`）。
- **问题**：
  - 用户侧无法在职位详情维护 `position_type/employment_type/Job Catalog 四级分类`，导致数据长期处于“占位/默认值”状态。
  - Job Catalog 维护目前仅有 API，缺少 UI 入口与“页内分组导航”；无法支撑业务侧自助配置/启停治理。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [X] 在 Org → Positions 的详情与编辑表单中增加并贯通以下字段（Create/Update）：
  - `position_type`（职位类型）
  - `employment_type`（雇佣类型/全职兼职等）
  - `job_family_group_code/job_family_code/job_role_code/job_level_code`（Job Catalog 四级分类）
- [X] 在 Org 模块新增 “Job Catalog” 的 UI 维护入口，并提供“页内分组导航”切换四级分类：
  - Family Group / Family / Role / Level 的查询、创建、编辑、启停（对齐 056 的表结构与 API 能力）。
- [X] 导航优化：将“组织架构 / 职位管理 / Job Catalog”调整为左侧导航栏 `组织与职位`（`NavigationLinks.Org`）下的二级菜单，并移除 `/org/nodes`、`/org/positions` 的页内 Tab 语义。
- [X] 不修改 052/053 已冻结的 Position v1 合同字段名与错误码语义，仅补齐 UI 层履约与展示。
- [X] 权限与边界对齐 054/056：读写入口分别受 `org.job_catalog` / `org.job_profiles` 的 `read/admin` 约束。

### 2.2 非目标（Out of Scope）
- 不在本计划内引入 `position_type/employment_type` 的主数据表与强约束（仍作为文本字段）；如需治理/枚举/映射，另立 dev-plan。
- 不在本计划内贯通 `cost_center_code` / `job_profile_id` 的 UI 写入口（055/056 需要时另立增量计划）。
- 不在本计划内交付报表（057）与 Restrictions 管理 UI（056 已定义，若需要 UI 另立增量计划）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划实施时将命中 `.templ`/多语言/Go 代码，必要时命中 Authz；命令入口以 SSOT 为准，避免在本文复制矩阵细节：`AGENTS.md`、`Makefile`、`.github/workflows/quality-gates.yml`。

- [X] Go 代码 —— `2025-12-28 11:45 UTC` 通过 `go fmt ./... && go vet ./... && make check lint && make test`
- [X] `.templ` / Tailwind —— `2025-12-28 11:45 UTC` 通过 `make generate && make css`（生成物已纳入变更集）
- [X] 多语言 JSON —— `2025-12-28 11:45 UTC` 通过 `make check tr`
- [X] Authz —— 未命中（无策略碎片/对象能力变更），因此无需执行 `make authz-test && make authz-lint`
- [X] 路由治理 —— `2025-12-28 11:45 UTC` 通过 `make check routing`
- [X] 文档 —— `2025-12-28 11:45 UTC` 通过 `make check doc`

## 3. 现状盘点（落地实现情况）
### 3.1 Position 分类字段（Schema / Service / API）
- Schema：`modules/org/infrastructure/persistence/schema/org-schema.sql`（字段已存在）。
- Service：Create/Update 均接收并落盘；Managed Position 强制非空（`modules/org/services/org_service_053.go:50`、`modules/org/services/org_service_053.go:654`）。
- API：
  - JSON API Create 必填字段包含 `position_type/employment_type/job_*_code`（`modules/org/presentation/controllers/org_api_controller.go:2100`）。
  - `GET /org/api/positions` 的响应中已包含这些字段（`modules/org/presentation/controllers/org_api_controller.go:2033`）。

### 3.2 Job Catalog 主数据与校验
- 主数据表 + 维护 API 已实现（056）：`/org/api/job-catalog/*`、`/org/api/job-profiles`（`modules/org/presentation/controllers/org_api_controller.go:85`、`modules/org/presentation/controllers/org_api_controller.go:101`）。
- 校验灰度：租户级 `org_settings.position_catalog_validation_mode`（`disabled/shadow/enforce`；`modules/org/infrastructure/persistence/schema/org-schema.sql:676`）。
- 校验语义：按四级 code 解析 → 主数据存在/启用/层级自洽；Profile 与 Role/Level 映射冲突拒绝（`modules/org/services/position_catalog_validation_056.go:13`）。

### 3.3 Org UI（缺口）
- Positions 页面目前不展示/可编辑分类字段；Create 直接写入默认值（`modules/org/presentation/controllers/org_ui_controller.go:739`）。
- 详情 viewmodel 不承载分类字段，导致 UI 层即使后端可读也无法展示（`modules/org/presentation/viewmodels/position.go:9`）。

## 4. 关键决策：Job Catalog 四级信息是否需要时间约束？
### 4.1 选项
1. **仅标签（现状，SCD Type 1）**：四级表仅 `code/name/is_active`，不做 effective dating；Position slice 记录 code，随时间可变更分类路径。
2. **有效期主数据（SCD Type 2）**：四级表增加 `effective_date/end_date`（date，闭区间）与 no-overlap 约束；写入口校验与 UI 展示按 as-of date 解析“当时有效”的标签。
3. **混合（仅启停有效期）**：为 `is_active` 增加生效日/停用日语义，但 name 仍为 SCD1。

### 4.2 行业实践与取舍（摘要）
- 有效期（effective dating）的收益主要在“历史口径必须可追溯/可复现”的场景：历史报表按当时名称/层级展示、审计追责、跨系统对账等。
- 但其成本会显著放大：查询/校验/编辑都需要 as-of 语义，并引入数据修复与回滚策略（尤其是层级关系也需要随时间变化时）。

### 4.3 选定（本计划口径）
- **更新为：倾向选项 2（有效期主数据，SCD Type 2）**，但建议分两阶段落地，避免 067 的 UI 贯通被“主数据有效期化”拖大：
  - Phase 1（本 DEV-PLAN-067）：沿用现有 schema（SCD1 标签化）先把 UI 贯通与维护入口补齐；并在产品/报表侧明确限制：历史 slices 展示的 `name/层级` 可能随主数据变更而“漂移”。
  - Phase 2（后续增量 dev-plan）：若需要“历史口径可复现（按 as-of 展示名称/启停/层级）”，为四级表引入 `effective_date/end_date`（date，闭区间）与 as-of date 解析，并扩展 UI 支持生效日维护（Valid Time 口径对齐 `DEV-PLAN-064A`：date-only、闭区间、no-overlap 统一）。
- 依据：Workday / SAP SuccessFactors / PeopleSoft 在“岗位/职位相关基础数据/主数据”上普遍存在 effective dating 机制（见 4.4）。

### 4.4 主流产品调研（证据与结论）
> 说明：本节仅记录“可公开获取的一手证据”（官方文档/官方 API Reference），用于判断“Job Catalog 四级是否需要有效期语义”。

#### 4.4.1 Workday（公开 Workday API 文档）
- Job Family Group：`Put_Job_Family_Group` 的请求数据包含 `Effective_Date` 与 `Inactive`（且含“最早有效期”校验文案）。参考：
  - https://community.workday.com/sites/default/files/file-hosting/productionapi/Human_Resources/v45.1/Put_Job_Family_Group.html
- Job Family：`Put_Job_Family` 的请求数据包含 `Effective_Date`（并含“最早有效期”校验文案）。参考：
  - https://community.workday.com/sites/default/files/file-hosting/productionapi/Human_Resources/v45.1/Put_Job_Family.html
- Job Profile：`Submit_Job_Profile`（Manage Job Profile BP）包含 `Effective_Date`（“Select the date you want the Job Profile change to take effect”）。参考：
  - https://community.workday.com/sites/default/files/file-hosting/productionapi/Human_Resources/v45.1/Submit_Job_Profile.html
- 读取语义：`Get_Job_Profiles` 的 Response Filter 支持 `As_Of_Effective_Date`。参考：
  - https://community.workday.com/sites/default/files/file-hosting/productionapi/Human_Resources/v45.1/Get_Job_Profiles.html

#### 4.4.2 SAP（SuccessFactors / HCM，一手证据）
- SuccessFactors OData V2 的 Foundation Object（例如 `FOJobCode`、`FOJobFunction`）属于 effective-dated entity：
  - `FOJobCode`：`Business Keys: externalCode + startDate`，且标记 `Effective-date:true`；示例响应包含 `startDate/endDate`。参考：
    - https://help.sap.com/docs/successfactors-platform/sap-successfactors-api-reference-guide-odata-v2/fojobcode
  - `FOJobFunction`：`Business Keys: externalCode + startDate`，且标记 `Effective-date:true`；示例响应包含 `startDate/endDate`。参考：
    - https://help.sap.com/docs/successfactors-platform/sap-successfactors-api-reference-guide-odata-v2/fojobfunction
- Effective Dating 机制（平台级说明）：
  - 记录在 `startDate~endDate` 区间有效；插入新记录会自动将上一条 `endDate` 置为“新记录 startDate 的前一天”，新记录默认 `endDate=12-31-9999`；查询支持 `asOfDate/fromDate/toDate`。参考：
    - https://help.sap.com/docs/successfactors-platform/sap-successfactors-api-reference-guide-odata-v2/effective-dating

#### 4.4.3 PeopleSoft（一手证据）
- PeopleSoft PeopleTools（Applications User’s Guide）将 effective dating 作为核心机制：允许存储历史/未来数据；effective-dated rows 分为 `Current/History/Future`；并通过页面动作 `Update/Display / Include History / Correct History` 控制可见与可编辑范围。参考：
  - https://docs.oracle.com/cd/G41075_01/pt862pbr3/eng/pt/tupa/UsingEffectiveDates-0714e5.html

### 4.5 架构与实现决策（Phase 1）
```mermaid
graph TD
    UI[Browser (HTMX)] --> OrgUI[OrgUIController (/org/*)]
    OrgUI --> Svc[OrgService]
    Svc --> Repo[Org Repository]
    Repo --> DB[(PostgreSQL)]
    API[External/CLI] --> OrgAPI[OrgAPIController (/org/api/*)]
    OrgAPI --> Svc
```

- UI 侧不“反向调用” JSON API：Org UI controller 直接复用 `modules/org/services/*` 与 repo，确保业务校验与错误码语义与 API 一致（052/053/056）。
- 下拉/搜索选择统一使用 HTMX 返回 `<option>` 列表：
  - 选择组件：`components/base/combobox.templ`（`hx-get` → `hx-target`=隐藏 `<select>` → `hx-swap=innerHTML`）。
  - 级联依赖处理：复用 `modules/org/presentation/templates/components/orgui/assignments.templ` 的 Trigger 模式（`hx-include` + 父级变化时清空/禁用子级）。
- Job Catalog 的时间语义：本计划 Phase 1 仍按现状 SCD1（仅 `code/name/is_active`）；如未来引入有效期主数据，Valid Time 字段与 no-overlap 口径对齐 `DEV-PLAN-064A`（date-only、闭区间、`daterange(effective_date, end_date + 1, '[)')`）。

### 4.6 数据模型与约束（Phase 1，Schema SSOT）
> Schema SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`（本文仅摘录与 067 直接相关的字段/约束，避免漂移）。

- Position slices：`org_position_slices`
  - 分类字段：`position_type text NULL`、`employment_type text NULL`、`job_family_group_code varchar(64) NULL`、`job_family_code varchar(64) NULL`、`job_role_code varchar(64) NULL`、`job_level_code varchar(64) NULL`、（可选）`job_profile_id uuid NULL`、`cost_center_code varchar(64) NULL`。
  - Valid Time：`effective_date date NOT NULL`、`end_date date NOT NULL DEFAULT 9999-12-31`；no-overlap：`EXCLUDE ... daterange(effective_date, end_date + 1, '[)') WITH &&`。
  - 业务约束（Service）：Managed Position 必须非空（即使列允许 NULL）；System/AutoCreated 允许为空（见 053/052）。
- Job Catalog 主数据（SCD1）：
  - `org_job_family_groups`：`code varchar(64) NOT NULL`、`name text NOT NULL`、`is_active boolean NOT NULL DEFAULT true`（tenant 内 `code` 唯一）。
  - `org_job_families`：`job_family_group_id uuid NOT NULL` + `code/name/is_active`（tenant+group+code 唯一）。
  - `org_job_roles`：`job_family_id uuid NOT NULL` + `code/name/is_active`（tenant+family+code 唯一）。
  - `org_job_levels`：`job_role_id uuid NOT NULL` + `code/name/display_order/is_active`（tenant+role+code 唯一）。
- Phase 1 不新增/修改 schema 与迁移；仅补齐 UI 履约与展示。

## 5. 方案：在职位详情页面可维护分类属性
### 5.1 UI 展示（Read）
- Position 详情页增加“分类/属性”分组，展示（至少）：
  - `position_type`、`employment_type`
  - Job Catalog 四级 code（可选：展示 `code — name`；若无法解析则标记为“缺失/停用”）
- 数据来源：
  - as-of Position 当前切片：`c.org.GetPosition(...)` 已能返回相关字段（`modules/org/presentation/controllers/org_ui_controller.go:891`）。
  - Job Catalog name 的解析：复用 Org Service/Repo，新增只读解析（单次查询返回四级 `code/name/is_active`），用于详情展示与 Edit 表单预填；避免 UI 侧 N+1 与重复查询。
    - 位置：建议落在 `modules/org/services/job_catalog_056.go`（Service）与对应 repo（SQL join 一次查齐）。
    - 失败展示：缺失/停用时仍展示 code，并附加状态提示（不在 UI 侧“猜测”校验口径）。

### 5.2 UI 维护（Create/Update）
- 扩展 `orgui.PositionForm` 表单字段，贯通到 `OrgUIController.CreatePosition/UpdatePosition`：
  - `position_type`、`employment_type`：v1 先做自由文本或最小枚举选择（不引入主数据 SSOT）；Create 默认沿用现有值（`regular/full_time`），但允许用户覆盖。
  - Job Catalog 四级：采用级联选择（Family Group → Family → Role → Level）。
    - 使用 `base.Combobox`（Searchable）+ UI options endpoints（返回 `<option>` 列表）：
      - `/org/job-catalog/options/family-groups`（value=code）
      - `/org/job-catalog/options/families`（value=code；依赖 `job_family_group_code`）
      - `/org/job-catalog/options/roles`（value=code；依赖 `job_family_group_code/job_family_code`）
      - `/org/job-catalog/options/levels`（value=code；依赖 `job_family_group_code/job_family_code/job_role_code`）
    - 级联联动：为下级 Combobox 提供 Trigger（`hx-include` 选择上级 `<select>`；上级变更时清空/禁用下级），实现方式参考 `modules/org/presentation/templates/components/orgui/assignments.templ` 的 Position Combobox。
    - Create 默认不再写入占位值（当前实现写入 `UNSPECIFIED`）；初始为“未选择”，提交时必须选择四级 code（以保证 `position_catalog_validation_mode=enforce` 时不被拒绝）。
  -（可选）`job_profile_id`：如纳入同表单，使用独立 combobox（value=uuid），并在后端沿用 056 的冲突校验与错误码语义。
- 错误处理：
  - 复用服务端错误码（如 `ORG_JOB_CATALOG_INACTIVE_OR_MISSING`、`ORG_JOB_CATALOG_INVALID_HIERARCHY`、`ORG_JOB_PROFILE_CONFLICT`）并在表单上展示；必要时扩展 `mapServiceErrorToForm` 将部分错误映射到字段级 `Errors[...]`（避免仅显示泛化 FormError）。
  - 字段错误映射（最小可用，Phase 1）：
    - `ORG_JOB_CATALOG_INACTIVE_OR_MISSING` / `ORG_JOB_CATALOG_INVALID_HIERARCHY` → `job_family_group_code/job_family_code/job_role_code/job_level_code`
    - `ORG_JOB_PROFILE_NOT_FOUND` / `ORG_JOB_PROFILE_INACTIVE` / `ORG_JOB_PROFILE_CONFLICT` → `job_profile_id`（如本期纳入）
    - `ORG_INVALID_BODY`（缺少必填）→ 对应缺失字段（优先在 controller 做表单校验，减少无谓 roundtrip）
  - 重要前提：Positions 表单需要读取 Job Catalog（用于下拉与 `code — name` 展示）；本计划要求具备 `org.positions write` 的角色同时具备 `org.job_catalog read`（必要时通过策略碎片补齐，见 7.7 与 9）。

### 5.3 路由与契约（UI）
> 说明：以下为 Org UI 层契约（HTML/HTMX）。JSON API 契约保持不变（056 已定义）。

- Positions（既有路由，扩展表单字段）：
  - `GET /org/positions?effective_date=YYYY-MM-DD`：页面。
  - `GET /org/positions/{id}?effective_date=YYYY-MM-DD&node_id=...`：详情局部刷新（HTMX）。
  - `GET /org/positions/new?effective_date=YYYY-MM-DD&node_id=...`：Create 表单局部刷新。
  - `POST /org/positions?effective_date=YYYY-MM-DD`：Create（HTMX），Form Data（新增字段名建议与 slice 字段一致：`position_type`、`employment_type`、`job_family_group_code`、`job_family_code`、`job_role_code`、`job_level_code`、（可选）`job_profile_id`、`cost_center_code`）。
  - `GET /org/positions/{id}/edit?effective_date=YYYY-MM-DD`：Edit 表单局部刷新（需预填上述字段）。
  - `PATCH /org/positions/{id}?effective_date=YYYY-MM-DD`：Update（HTMX），Form Data 同上（UpdateInput 使用指针，允许按需更新）。
- Job Catalog options（新增，供 Position 表单级联选择）：
  - `GET /org/job-catalog/options/family-groups?q=...` → `<option value="CODE">CODE — Name</option>`
  - `GET /org/job-catalog/options/families?q=...&job_family_group_code=...`
  - `GET /org/job-catalog/options/roles?q=...&job_family_group_code=...&job_family_code=...`
  - `GET /org/job-catalog/options/levels?q=...&job_family_group_code=...&job_family_code=...&job_role_code=...`
  - 约束：
    - 父级缺失时返回空列表（200）。
    - 默认仅返回 `is_active=true` 项，避免 UI 选择到停用项；支持 `include_inactive=1`（仅在 Job Catalog 管理 UI 场景使用）。
    - 上限 50 条，按 `code ASC`（同 code 再按 `name ASC`）。
    - Edit 表单的“当前选中项”由服务端直接渲染 `<option selected>`，不依赖 options endpoint 做回显。

## 6. 方案：Job Catalog 维护入口与“页内分组导航”
### 6.1 IA（信息架构）
- **左侧导航栏（全局 IA）**：将目前“组织架构 / 职位”等页内 Tab，调整为左侧导航栏 `NavigationLinks.Org`（中文显示为“组织与职位”）下的二级菜单：
  - `组织架构`（`NavigationLinks.OrgStructure`）→ `GET /org/nodes`（read gate：`org.hierarchies read`）
  - `职位管理`（`NavigationLinks.OrgPositions`）→ `GET /org/positions`（read gate：`org.positions read`）
  - `Job Catalog`（`NavigationLinks.JobCatalog`）→ `GET /org/job-catalog`（read gate：`org.job_catalog read`）
  - 顶层 `组织与职位` 作为分组（AccordionGroup）：用于折叠/展开与聚合权限，不承担“点击跳转”。当某用户仅有 1 个可见子项时，侧栏会按既有逻辑将分组折叠为单个链接（`pkg/middleware/sidebar.go:42`）。
- **移除页内 Tab 语义**：`/org/nodes` 与 `/org/positions` 页面不再通过 `orgui.Subnav` 在页内做“Tab 切换”，避免把不同对象误导为同一对象的不同属性；页面顶部仅保留标题 + `effective_date`（as-of）切换器。
- **任职入口不在本计划内调整**：`/org/assignments` 的导航入口继续由 People 模块的 `NavigationLinks.JobData` 承载（`modules/person/links.go`）。
- **Job Catalog 页内导航**：在 `Job Catalog` 页面内部提供“页内分组导航”（侧边栏或次级 Tab）切换四级对象：
  - Family Groups / Families / Roles / Levels
  - （可选）Job Profiles 入口（权限对象不同：`org.job_profiles`），如需同页纳入则必须明确边界与提示。
  - 页面 URL 建议：`GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=<family-groups|families|roles|levels>&...`（Phase 1：Job Catalog 数据不随 `effective_date` 变化；该参数仅用于 Org UI 上下文一致性与导航回链）。

### 6.2 CRUD/启停行为
- UI 直接复用 056 的 Service/Repo（无需新增后端契约），并以 HTMX 表单与局部刷新实现：
  - List（含 `is_active` 显示）
  - Create / Update（`code/name/is_active`；Job Level 另含 `display_order`）
  - 权限：Read 用户只读；Admin 用户可创建/编辑/启停（UI 展示与后端 enforce 双重保证）。
- 层级导航建议：
  - Families 列表需要选定/过滤 `job_family_group_code`
  - Roles 列表需要选定/过滤 `job_family_group_code` + `job_family_code`
  - Levels 列表需要选定/过滤 `job_family_group_code` + `job_family_code` + `job_role_code`

### 6.3 路由与契约（UI）
- 页面：
  - `GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=family-groups`
  - `GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=families&job_family_group_code=CODE`
  - `GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=roles&job_family_group_code=CODE&job_family_code=CODE`
  - `GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=levels&job_family_group_code=CODE&job_family_code=CODE&job_role_code=CODE`
  -（可选）`GET /org/job-catalog?...&edit_id=UUID`
- 父级筛选 Combobox（options endpoints；value=CODE）：
  - `GET /org/job-catalog/family-groups/options?effective_date=YYYY-MM-DD&q=...`
  - `GET /org/job-catalog/families/options?effective_date=YYYY-MM-DD&q=...&job_family_group_code=CODE`
  - `GET /org/job-catalog/roles/options?effective_date=YYYY-MM-DD&q=...&job_family_group_code=CODE&job_family_code=CODE`
  - `GET /org/job-catalog/levels/options?effective_date=YYYY-MM-DD&q=...&job_family_group_code=CODE&job_family_code=CODE&job_role_code=CODE`
  - 约束：默认仅返回 `is_active=true`；支持 `include_inactive=1`（仅 Job Catalog 管理 UI 场景使用）。
- CRUD（新增，HTMX；均需 `org.job_catalog admin`）：
  - `POST /org/job-catalog/family-groups`
  - `PATCH /org/job-catalog/family-groups/{id}`
  - `POST /org/job-catalog/families`（需要 `job_family_group_code`）
  - `PATCH /org/job-catalog/families/{id}`
  - `POST /org/job-catalog/roles`（需要 `job_family_group_code` + `job_family_code`）
  - `PATCH /org/job-catalog/roles/{id}`
  - `POST /org/job-catalog/levels`（需要 `job_family_group_code` + `job_family_code` + `job_role_code`）
  - `PATCH /org/job-catalog/levels/{id}`

### 6.4 页面布局（UI Layout）
#### 6.4.1 全局导航（左侧）
- 一级：`组织与职位`（现有 `NavigationLinks.Org`）
  - 二级：`组织架构`（/org/nodes）
  - 二级：`职位管理`（/org/positions）
  - 二级：`Job Catalog`（/org/job-catalog）
  - 注：当用户仅有 1 个可见子项权限时，侧栏可能直接显示该子链接而不显示分组（既有行为）。

#### 6.4.2 页面：组织架构（`/org/nodes`）
- Header：
  - 标题：组织架构
  - 右侧：`effective_date` 日期选择（as-of，上下文保持与现有一致）
- Body（双栏）：
  - 左栏：组织树（Tree）
  - 右栏：节点详情/编辑面板（Node panel）

#### 6.4.3 页面：职位管理（`/org/positions`）
- Header：
  - 标题：职位管理
  - 右侧：`effective_date` 日期选择（as-of）
- Body（双栏）：
  - 左栏：组织树（Tree，用于选择部门/组织节点）
  - 右栏：职位面板（Positions panel）
    - 列表区：职位列表 + 过滤/分页
    - 详情区：职位详情（新增“分类/属性”分组：`position_type/employment_type/job_*_code`…）
    - 时间线区：职位时间线（timeline）
    - Create/Edit 表单：在现有字段组基础上新增“分类”字段组（四级级联 Combobox）

#### 6.4.4 页面：Job Catalog（`/org/job-catalog`）
- Header：
  - 标题：Job Catalog
  - 右侧：保留 `effective_date`（仅用于上下文/回链；Phase 1 主数据不随 as-of 变化）
- Body（建议三段式）：
  - 页内分组导航：Family Groups / Families / Roles / Levels
  - 列表区：当前分组的条目列表（code/name/is_active；Levels 额外 display_order）
  - 侧栏/弹层：Create/Edit 表单（admin 可见；read 用户仅浏览）

### 6.5 i18n 文案清单（必须对齐 `make check tr`）
> 仅列出本计划新增/调整的 key；具体字符串以中英文一致性为准。

- Sidebar 二级菜单（新增，见 `modules/org/links.go`）：
  - `NavigationLinks.OrgStructure`：组织架构 / Org structure
  - `NavigationLinks.OrgPositions`：职位管理 / Positions
  - `NavigationLinks.JobCatalog`：Job Catalog / Job Catalog
- 页面标题（建议调整，避免“菜单名 ≠ 页面标题”）：
  - `Org.UI.Nodes.Title`、`Org.UI.Nodes.MetaTitle`：组织架构 / Org structure
  - `Org.UI.Positions.Title`、`Org.UI.Positions.MetaTitle`：职位管理 / Positions
  - `Org.UI.JobCatalog.Title`、`Org.UI.JobCatalog.MetaTitle`：Job Catalog / Job Catalog
- Positions 字段（新增，供详情/表单）：
  - `Org.UI.Positions.Fields.PositionType`
  - `Org.UI.Positions.Fields.EmploymentType`
  - `Org.UI.Positions.Fields.JobFamilyGroupCode`
  - `Org.UI.Positions.Fields.JobFamilyCode`
  - `Org.UI.Positions.Fields.JobRoleCode`
  - `Org.UI.Positions.Fields.JobLevelCode`
  -（可选）`Org.UI.Positions.Fields.JobProfileID`、`Org.UI.Positions.Fields.CostCenterCode`
- Job Catalog 页内分组导航（新增）：
  - `Org.UI.JobCatalog.Tabs.FamilyGroups`
  - `Org.UI.JobCatalog.Tabs.Families`
  - `Org.UI.JobCatalog.Tabs.Roles`
  - `Org.UI.JobCatalog.Tabs.Levels`
- Job Catalog 字段（新增，供列表/表单）：
  - `Org.UI.JobCatalog.Fields.Code`
  - `Org.UI.JobCatalog.Fields.Name`
  - `Org.UI.JobCatalog.Fields.IsActive`
  - `Org.UI.JobCatalog.Fields.DisplayOrder`

## 7. 实施步骤（里程碑）
1. [X] 文档冻结：补齐路由/参数/字段/鉴权/测试清单，并与 055/056/059 交叉引用确认不冲突。
2. [X] UI（导航 IA）：将 `NavigationLinks.Org`（组织与职位）拆为二级菜单（组织架构/职位管理/Job Catalog），并移除 `/org/nodes`、`/org/positions` 页内 `orgui.Subnav`。
3. [X] UI（Positions Read）：扩展 viewmodel + 详情页展示分类字段；补齐 Job Catalog label 解析（只读）。
4. [X] UI（Positions Write）：扩展 Create/Edit 表单字段与级联选择；贯通 Create/Update controller → service。
5. [X] UI（Job Catalog）：新增 `/org/job-catalog` 页面、页内分组导航、父级筛选与四级 CRUD/启停能力；read/admin 分离。
6. [X] i18n：补齐新增导航/字段/页面文案到 `modules/org/presentation/locales/*.json`，并通过 `make check tr`。
7. [X] Authz：复用既有 `org.job_catalog`/`org.positions` 对象与策略能力；本计划无新增策略碎片。
8. [X] E2E：扩展 `e2e/tests/org/org-ui.spec.ts` 覆盖导航 IA、Job Catalog UI 与 Position 分类字段贯通；`2025-12-28 11:45 UTC` 通过 `cd e2e && npx playwright test`（全量）。
9. [X] 门禁：`2025-12-28 11:45 UTC` 通过（Go/templ/css/i18n/routing/doc/e2e）；并确认 `make generate && make css` 后生成物已纳入变更集。

## 8. 验收标准 (Acceptance Criteria)
- [X] 左侧导航栏信息架构满足以下其一：
  - 若用户可见子项 ≥ 2：存在 `组织与职位` 分组，且包含二级菜单 `组织架构` / `职位管理` / `Job Catalog`（按权限显示子集）。
  - 若用户可见子项 = 1：侧栏可直接展示该子链接（既有折叠逻辑），不强制显示分组。
  - 且 `GET /org/nodes`、`GET /org/positions` 不再使用页内 Tab 切换。
- [X] Positions 详情页可查看并编辑 `position_type/employment_type/job_*_code`，保存后刷新详情与时间线一致。
- [X] Job Catalog UI 可维护四级分类（查询/创建/编辑/启停），并能支撑 Positions 表单的级联选择。
- [X] 当 Job Catalog/Profile 校验处于 `enforce` 时，UI 能给出可理解的拒绝提示（不吞错误、不漂移错误码语义）。
- [X] 所有门禁通过（Go lint/test、templ 生成、css、多语言、doc 检查等按触发器矩阵执行）。

## 9. 安全与鉴权 (Security & Authz)
- Org UI：
  - Positions：沿用既有 `org.positions read/write`（`modules/org/presentation/controllers/org_ui_controller.go:87`）。
  - Job Catalog：新增 UI 路由需分别校验 `org.job_catalog read/admin`（对象已存在：`modules/org/presentation/controllers/authz_helpers.go:22`；API 侧口径：`modules/org/presentation/controllers/org_api_controller_056_job_catalog.go:21`、`modules/org/presentation/controllers/org_api_controller_056_job_catalog.go:44`）。
  - Job Catalog options/search endpoints：读取主数据，按 `org.job_catalog read` 保护；因此需通过策略确保“可写职位分类字段”的角色具备 `org.job_catalog read`（最小只读）。
- UI 展示策略：
  - read 用户可见页内分组导航与列表；admin 用户可见 Create/Edit/启停控件。
  - 后端必须再次 enforce（UI 只负责体验，不作为安全边界）。

## 10. 测试计划 (Test Plan)
- E2E（Playwright）：
  - [X] 导航：管理员可在左侧导航栏看到 `组织与职位` 分组与二级菜单（组织架构/职位管理/Job Catalog），点击可正确跳转且高亮状态正确；只读用户仅可见其有权限的入口（必要时折叠为单链接）。
  - [X] 管理员：进入 `/org/job-catalog` 创建最小四级路径（group→family→role→level），再到 `/org/positions` 创建/编辑职位并选择对应四级 code，验证详情页展示 `code — name`。
  - [X] 只读用户：可浏览 Job Catalog 列表但不可创建/编辑；直接请求 admin 路由返回 403。
- 回归：
  - [X] `org.positions` 既有 CRUD/搜索/时间线功能不回退（通过全量 e2e 覆盖与回归验证）。

## 11. 运维与监控 (Ops & Monitoring)
- 本计划不新增运行时监控/开关；排障继续使用现有 request-id 关联与 Playwright trace（如需）。

## 12. UI 验证记录（按 DEV-PLAN-044）
> 交付口径：不是“我看过了”，而是“我能复现并证明”。本章节给出可复现脚本、证据落点与按页面的评估结论（P0/P1/P2）。

### 12.1 可复现入口（脚本化）
- 证据脚本：`e2e/tests/org/dev-plan-067-ui-verification.spec.ts`
  - 覆盖 3 个 viewport：`mobile`（390×844）、`laptop`（1366×768）、`desktop`（1920×1080）
  - 主路径：`/org/nodes`、`/org/positions`（Create position 抽屉 + Job Catalog 四级级联选择）、`/org/job-catalog`（四级切换）
  - 关键边界：无权限用户访问 `/org/job-catalog` 显示 `Permission required`
  - 关键交互：在 Position Create 抽屉里验证 Org node combobox 选中 chip 不重复渲染（避免 HTMX swap 后 Alpine double-init 导致重复）
- 证据产物目录（不入库）：`tmp/ui-verification/dev-plan-067/latest/`
  - 注意：Playwright 默认会清理 `e2e/test-results/`，因此证据需落到仓库 `tmp/`（已在 `.gitignore` 排除）。
- 本地复现命令：`cd e2e && npx playwright test tests/org/dev-plan-067-ui-verification.spec.ts --workers=1 --reporter=list`

### 12.2 页面评估报告（P0/P1/P2）
#### 12.2.1 `/org/job-catalog`（Job Catalog）
- 主路径：四级页内分组导航切换（Family Groups / Families / Roles / Levels），Admin 可 Create/Edit，Read 只读。
- 布局评估：
  - P0：无
  - P1：无
  - P2：移动端下页内分组导航占用首屏高度较多（在 Levels 视图还叠加 3 个父级过滤器），首屏需要滚动才能看到列表区；可考虑在移动端把过滤器折叠为可展开区域或吸顶折叠。
- 交互评估：
  - P0（已修复）：`/org/job-catalog` 曾出现 500（nil pointer），原因是模板里对 `props.Edit*` 指针的非惰性求值（Go 先求值参数），已改为 nil-safe helper（见 `modules/org/presentation/templates/pages/org/job_catalog.templ`）。
  - P1：无
  - P2：无
- 证据：`tmp/ui-verification/dev-plan-067/latest/` 下以 `org-job-catalog-*` 命名的截图（含 unauthorized 证据）。

#### 12.2.2 `/org/positions`（职位管理）
- 主路径：选择组织节点 → 打开 Create/Edit 抽屉 → 维护 `position_type/employment_type/job_*_code` → 保存。
- 布局评估：
  - P0：无
  - P1：无
  - P2：移动端过滤器区较长，首屏信息密度偏高（但不影响主路径）；可考虑移动端将过滤器折叠或改为“筛选”抽屉。
- 交互评估：
  - P0：无
  - P1：无
  - P2（已修复）：Position Create 抽屉中的 Org node combobox 在 HTMX swap 后出现选中 chip 重复渲染（Remove 按钮重复）；根因是手动 `Alpine.initTree()` 与 Alpine v3 MutationObserver 重复初始化导致重复克隆，已将 `modules/core/presentation/assets/js/lib/htmx-alpine-init.js` 调整为 no-op 以避免 double-init。
- 证据：`tmp/ui-verification/dev-plan-067/latest/org-positions*` 截图。

#### 12.2.3 `/org/nodes`（组织架构）
- 主路径：Tree + Details 双栏；可创建节点并在右侧查看详情。
- 布局评估：
  - P0：无
  - P1：无
  - P2：移动端双栏纵向堆叠时，Tree 与 Details 卡片高度都较大，首屏有效信息较少；可考虑 Details 在移动端默认折叠或延迟渲染（选中节点后再展示）。
- 交互评估：
  - P0：无
  - P1：无
  - P2：无
- 证据：`tmp/ui-verification/dev-plan-067/latest/org-nodes*` 截图。
