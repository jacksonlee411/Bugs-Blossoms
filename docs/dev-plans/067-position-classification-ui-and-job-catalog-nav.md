# DEV-PLAN-067：Position 分类字段 UI 贯通 + Job Catalog 维护入口（二级目录）

**状态**: 规划中（2025-12-28 00:30 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 业务需求：`docs/dev-plans/050-position-management-business-requirements.md`（职位类型/雇佣类型/Job Catalog 四级分类）。
  - 决策冻结：`docs/dev-plans/052-position-contract-freeze-and-decisions.md`（字段/合同冻结；Managed vs System）。
  - Position v1：`docs/dev-plans/053-position-core-schema-service-api.md`、`docs/dev-plans/053A-position-contract-fields-pass-through.md`。
  - UI IA：`docs/dev-plans/055-position-ui-org-integration.md`。
  - 主数据/校验：`docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`。
- **当前状态（实现盘点摘要）**：
  - Schema 已包含 `org_position_slices.position_type/employment_type/job_*_code/job_profile_id/cost_center_code`（`modules/org/infrastructure/persistence/schema/org-schema.sql:380`）。
  - Service & API 已可写入/更新上述字段，并在 Managed Position 上强制非空（`modules/org/services/org_service_053.go:21`、`modules/org/presentation/controllers/org_api_controller.go:2100`）。
  - Job Catalog/Profile 的主数据表与维护 API 已存在（`modules/org/presentation/controllers/org_api_controller.go:85`、`modules/org/presentation/controllers/org_api_controller.go:101`），并支持 `disabled/shadow/enforce` 校验灰度（`modules/org/infrastructure/persistence/schema/org-schema.sql:676`、`modules/org/services/position_catalog_validation_056.go:53`）。
  - Org UI 的 Positions 页面尚未展示/可编辑这些分类字段：Create 仍硬编码写入默认值（`modules/org/presentation/controllers/org_ui_controller.go:739`），Position 详情 viewmodel 也未承载这些字段（`modules/org/presentation/viewmodels/position.go:9`）。
- **问题**：
  - 用户侧无法在职位详情维护 `position_type/employment_type/Job Catalog 四级分类`，导致数据长期处于“占位/默认值”状态。
  - Job Catalog 维护目前仅有 API，缺少 UI 入口与“二级目录”导航；无法支撑业务侧自助配置/启停治理。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] 在 Org → Positions 的详情与编辑表单中增加并贯通以下字段（Create/Update）：
  - `position_type`（职位类型）
  - `employment_type`（雇佣类型/全职兼职等）
  - `job_family_group_code/job_family_code/job_role_code/job_level_code`（Job Catalog 四级分类）
  -（同 055 v1 可选字段，按需一起贯通）`cost_center_code`、`job_profile_id`
- [ ] 在 Org 模块新增 “Job Catalog” 的 UI 维护入口，并提供“二级目录”切换四级分类：
  - Family Group / Family / Role / Level 的查询、创建、编辑、启停（对齐 056 的表结构与 API 能力）。
- [ ] 不修改 052/053 已冻结的 Position v1 合同字段名与错误码语义，仅补齐 UI 层履约与展示。
- [ ] 权限与边界对齐 054/056：读写入口分别受 `org.job_catalog` / `org.job_profiles` 的 `read/admin` 约束。

### 2.2 非目标（Out of Scope）
- 不在本计划内引入 `position_type/employment_type` 的主数据表与强约束（仍作为文本字段）；如需治理/枚举/映射，另立 dev-plan。
- 不在本计划内交付报表（057）与 Restrictions 管理 UI（056 已定义，若需要 UI 另立增量计划）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划实施时将命中 `.templ`/多语言/Go 代码，必要时命中 Authz；命令入口以 SSOT 为准，避免在本文复制矩阵细节：`AGENTS.md`、`Makefile`、`.github/workflows/quality-gates.yml`。

- [ ] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
- [ ] `.templ` / Tailwind（`make generate && make css`，并确保生成物提交）
- [ ] 多语言 JSON（`make check tr`）
- [ ] Authz（若新增 UI 路由/对象能力或策略碎片：`make authz-test && make authz-lint`）
- [ ] 文档（`make check doc`）

## 3. 现状盘点（落地实现情况）
### 3.1 Position 分类字段（Schema / Service / API）
- Schema：`modules/org/infrastructure/persistence/schema/org-schema.sql:380`（字段已存在）。
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
2. **有效期主数据（SCD Type 2）**：四级表增加 `effective_on/end_on`（date）与时间不重叠约束；写入口校验与 UI 展示按 as-of 解析“当时有效”的标签。
3. **混合（仅启停有效期）**：为 `is_active` 增加生效日/停用日语义，但 name 仍为 SCD1。

### 4.2 行业实践与取舍（摘要）
- 有效期（effective dating）的收益主要在“历史口径必须可追溯/可复现”的场景：历史报表按当时名称/层级展示、审计追责、跨系统对账等。
- 但其成本会显著放大：查询/校验/编辑都需要 as-of 语义，并引入数据修复与回滚策略（尤其是层级关系也需要随时间变化时）。

### 4.3 选定（本计划口径）
- **更新为：倾向选项 2（有效期主数据，SCD Type 2）**，但建议分两阶段落地，避免 067 的 UI 贯通被“主数据有效期化”拖大：
  - Phase 1（本 DEV-PLAN-067）：沿用现有 schema（SCD1 标签化）先把 UI 贯通与维护入口补齐；并在产品/报表侧明确限制：历史 slices 展示的 `name/层级` 可能随主数据变更而“漂移”。
  - Phase 2（后续增量 dev-plan）：若需要“历史口径可复现（按 as-of 展示名称/启停/层级）”，为四级表引入 `effective_on/end_on` 与 as-of 解析，并扩展 UI 支持生效日维护（day granularity 对齐 `DEV-PLAN-064`）。
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

## 5. 方案：在职位详情页面可维护分类属性
### 5.1 UI 展示（Read）
- Position 详情页增加“分类/属性”分组，展示（至少）：
  - `position_type`、`employment_type`
  - Job Catalog 四级 code（可选：展示 `code — name`；若无法解析则标记为“缺失/停用”）
- 数据来源：
  - as-of Position 当前切片：`c.org.GetPosition(...)` 已能返回相关字段（`modules/org/presentation/controllers/org_ui_controller.go:891`）。
  - Job Catalog name 的解析：复用 Org Service/Repo（必要时补一个“按 code 解析并返回 name”的只读方法，避免 UI 侧 N+1 与重复查询）。

### 5.2 UI 维护（Create/Update）
- 扩展 `orgui.PositionForm` 表单字段，贯通到 `OrgUIController.CreatePosition/UpdatePosition`：
  - `position_type`、`employment_type`：v1 先做自由文本或最小枚举选择（不引入主数据 SSOT）。
  - Job Catalog 四级：采用级联选择（Family Group → Family → Role → Level）。
    - 复用现有 API `GET /org/api/job-catalog/*`；UI 表单建议通过 HTMX/Combobox 提供可搜索选择，避免一次性加载过大列表。
- 错误处理：
  - 复用服务端错误码（如 `ORG_JOB_CATALOG_INACTIVE_OR_MISSING`、`ORG_JOB_PROFILE_CONFLICT`）并在表单上就近展示，避免 UI 自行推断口径。

## 6. 方案：Job Catalog 维护入口与“二级目录”
### 6.1 IA（信息架构）
- 在 Org 子导航（`orgui.Subnav`）新增 Tab：`Job Catalog`（可见性：具备 `org.job_catalog read`）。
- Job Catalog 页面内部提供“二级目录”（侧边栏或次级 Tab）切换四级对象：
  - Family Groups / Families / Roles / Levels
  - （可选）Job Profiles 入口（权限对象不同：`org.job_profiles`），如需同页纳入则必须明确边界与提示。

### 6.2 CRUD/启停行为
- 复用 056 的 JSON API（无需新增后端契约），UI 以 HTMX 表单与局部刷新实现：
  - List（含 is_active 显示与过滤）
  - Create / Update（name、is_active；Job Level 另含 `display_order`）
- 层级导航建议：
  - Families 列表需要选定/过滤 `job_family_group_id`
  - Roles 列表需要选定/过滤 `job_family_id`
  - Levels 列表需要选定/过滤 `job_role_id`

## 7. 实施步骤（里程碑）
1. [ ] 文档冻结：补齐本计划的路由/权限/字段验收清单，并与 055/056/059 交叉引用确认不冲突。
2. [ ] UI：扩展 Position viewmodel + 详情展示（Read），确保 as-of 一致性。
3. [ ] UI：扩展 Position Create/Edit 表单字段，并贯通 Create/Update controller 到 service（Write）。
4. [ ] UI：新增 Job Catalog 页面（含二级目录）与四级 CRUD/启停能力；权限按 read/admin 分离。
5. [ ] 测试与门禁：按 `AGENTS.md` 触发器矩阵执行；如新增 `.templ`/多语言，确保生成物提交且 `git status --short` 为空。

## 8. 验收标准 (Acceptance Criteria)
- [ ] Positions 详情页可查看并编辑 `position_type/employment_type/job_*_code`，保存后刷新详情与时间线一致。
- [ ] Job Catalog UI 可维护四级分类（查询/创建/编辑/启停），并能支撑 Positions 表单的级联选择。
- [ ] 当 Job Catalog/Profile 校验处于 `enforce` 时，UI 能给出可理解的拒绝提示（不吞错误、不漂移错误码语义）。
- [ ] 所有门禁通过（Go lint/test、templ 生成、css、多语言、doc 检查等按触发器矩阵执行）。
