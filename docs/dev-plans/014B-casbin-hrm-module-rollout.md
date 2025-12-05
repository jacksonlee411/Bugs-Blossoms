# DEV-PLAN-014B：HRM 模块 Casbin 改造细化计划

**状态**: 进行中（2025-12-05 18:30）

## 背景
- DEV-PLAN-014 主计划要求 HRM 模块与 Core/Logging 一起完成 Casbin 授权改造，并通过 `AUTHZ_ENFORCE` 灰度。Core 已在 DEV-PLAN-014A 中完成服务层兜底、模板注入等基础工作，HRM 需要在此基线之上复制能力。
- HRM 目前仍依赖 legacy `user.Can`、Quick Links 无鉴权、服务层/批量导入缺少 `authz.Authorize`。Controller 直接导出数据，没有 403/Unauthorized 体验，也没有 `MissingPolicies` 供 015B UI 使用。
- 本子计划聚焦 HRM，目标是输出与 014A 相同颗粒度的 TODO 与验收标准，确保 HRM 在 014 主计划第二阶段内具备可发布能力。

## 前置依赖
- DEV-PLAN-012/013 建立的 `pkg/authz`、`make authz-test`/`make authz-lint`、`scripts/authz/*` 已可用，并在进入本计划前重新执行一遍 readiness 命令并记录到 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`。
- DEV-PLAN-014A 已完成 `authz.ViewState` 注入、`ensureAuthz`/`authorizeCore` helper，可作为 HRM Controller/Service 实现模板；缺少的 helper 需在 HRM 模块内复制并与 core/authzutil 对齐。
- HRM SQLC、Atlas、schema 导出流程（DEV-PLAN-010/011）必须保持可运行，以便在授权改造期间新增字段/索引时可随时生成迁移。

## 前置准备清单（进入实施前需完成并记录）
- [ ] 重新执行并登记 012/013 readiness：`make authz-test authz-lint`、`go test ./pkg/authz/...`，结果写入 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`。
- [ ] 确认 HRM 基线可测：`go test ./modules/hrm/...`。
- [ ] 验证 SQLC/Atlas 链路：`make sqlc-generate`（完成后确保 `git status --short` 干净），可选 `make db plan HRM_MIGRATIONS=1` 以确认 Atlas diff 无报错。
- [ ] 验证策略打包：`make authz-pack` 能正常生成 `config/access/policy.csv`/`.rev` 且无意外 diff。
- [ ] 验证 shadow 运行：`AUTHZ_ENFORCE=0 make run`（或等效 dev 启动）可启动，日志无 authz 报错。
- [ ] 更新本文档状态为“准备就绪/已批准”，补充 readiness 命令时间戳及结果。

## 范围
- `modules/hrm/presentation/controllers/employee_controller.go` 及未来 HRM 控制器。
- `modules/hrm/services/**`（包含 Employee/Position + 后续批处理/导入 Job），确保所有可写路径在进入仓储前完成授权。
- `modules/hrm/presentation/templates/**`、`viewmodels`、`mappers`、`locales`。
- HRM 导航（`modules/hrm/links.go`）、Quick Links（`modules/hrm/module.go`）与 Spotlight 入口。
- e2e/Playwright `e2e/tests/employees/**` 与任何 HRM 相关集成测试。
- HRM 相关 docs/dev-records、README/CONTRIBUTING 补充说明。
- `config/access/policies/**`、`config/access/policy.csv(.rev)` 及 `make authz-pack` 产物，覆盖 `hrm.employees:*`（以及 Position/未来聚合）的 policy 片段。
- e2e/seed 数据中 HRM 权限矩阵（含“具备全部权限”与“无 HRM 权限”两类账号），确保授权体验可在自测/CI 中复现。

## 目标
1. HRM 控制器、服务、导航、Quick Links 全面接入 Casbin，legacy 权限仅在 shadow 模式兜底，并具备统一 403 响应与 MissingPolicies 注入。
2. 模板/组件完全依赖 `pageCtx.CanAuthz` + `authz.ViewState` 控制可见性，去除 `user.Can`/硬编码入口；未授权场景提供临时 Unauthorized 体验，与 015B 契约兼容。
3. 服务层、批量导入/导出等写操作在进入 Repository 前执行 `authz.Authorize`，并提供单元/E2E 测试覆盖“允许/拒绝/缺少用户”三类场景。
4. 完成 HRM 专属分批灰度记录：`AUTHZ_ENFORCE` 在 HRM 上的启停命令、diff、回滚全部登记到 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`。

## 阶段划分与对齐（与 014 主计划）
- M1（开发阶段）：默认 enforce 开发，必要时做一次 shadow 对比；交付 controller guard、模板清理、服务层 authorize + 单测。
- M2（上线前准备）：补齐 `make authz-test authz-lint`、SQLC/Atlas/最小 authz-pack readiness，并记录到 dev-records readiness 章节。
- M3（灰度/发布）：如需灰度，执行 shadow→enforce 对比并观察 ≥48h，补全 dev-records 灰度记录、Playwright + 无权限账号矩阵、正式 authz-pack。

## 工作拆解

### 1. 控制器与中间件
- [x] 复制 Core `ensureAuthz` 模式到 HRM（建议新建 `modules/hrm/presentation/controllers/authz_helpers.go`），依赖 `authzutil` + `pkg/authz` 统一 subject/domain 计算，legacy fallback 复用 `modules/hrm/permissions`。
- [x] 为 EmployeeController 的 `List/GetNew/GetEdit/Create/Update/Delete` 定义能力矩阵（建议命名 `hrm.employees:list|view|create|update|delete`），controller 入口第一个逻辑即调用 helper；403 时返回统一消息并设置 HX-Retarget。
- [x] 为 HRM Router 中的 Quick Links、Nav、PageContext 注入 `authz.ViewState`（参照 Core `middleware.NavItems` + `authzutil.EnsureViewState`），确保模板 `pageCtx.CanAuthz` 可用。
- [x] Controller 返回的 props 需包含所有与权限相关的布尔值（如 `CanCreate`, `CanDelete`），便于 HTMX partial 在无权限时隐藏按钮。

### 2. 服务层 & 异步任务
- [x] 在 `modules/hrm/services` 创建 `authorizeHRM` helper，行为与 `authorizeCore` 相同；所有写操作（Create/Update/Delete、批量导入、事件消费）进入 Repository 前必须调用，必要时传入 `authz.Attributes`（例如导入批次 ID）。
- [x] EmployeeService：Create/Update/Delete 加 guard，`Count/GetPaginated` 可选 shadow 只读；触发 `employee.NewCreatedEvent` 等事件前若授权失败需返回 `authz.ErrForbidden`。
- [x] PositionService 以及未来 HRM 服务的 Create/Update/Delete 同样复用 capability（建议 `hrm.positions:{create|update|delete}`），通过表驱动测试验证拒绝时不会触发仓储/事件。
- [x] 若存在批处理或 future job（导入 CSV、同步第三方数据、工资计算等），需定义 system subject（例如 `system:hrm.job`），并在 docs/dev-records 中登记策略写法。（当前无批处理需求，标记为 N/A）
- [x] 为每个操作新增/更新单测：使用 testify + mock repository，模拟上下文无用户/无权限/有权限；拒绝时断言仓储未被调用。

### 3. Presentation / 模板 / 本地化
- [x] 在 `modules/hrm/presentation/templates/pages/employees/*.templ` 去除 `user.Can`，全部改为 `if pageCtx.CanAuthz("hrm.employees", "create") { ... }`。按钮/链接（新建、保存、删除、批量操作）全部受控。
- [x] 提供临时 Unauthorized 组件（可直接借用 Core 的 403 文案），props 接收 `pageCtx.AuthzState().MissingPolicies`，展示“申请权限”按钮跳转 `/core/api/authz/requests`，为 015B 升级留钩子。
- [x] 更新 viewmodels/mappers 以携带 `CanEdit`, `CanDelete` 等布尔字段，避免模板内直接发起权限判断。
- [x] `modules/hrm/presentation/locales/{en,ru,uz}.json` 增加 Casbin 相关文案（Denied 标题、申请按钮、调试提示）；完成后运行 `make check tr`。

### 4. 导航 & Quick Links
- [x] `modules/hrm/links.go`：为 `HRMLink`/`EmployeesLink` 补充 `AuthzObject`（`"hrm.employees"`）和动作（`"list"`/`"view"`），并清理 `Permissions` 字段；若新增 Position/Dashboard，再按同样模式扩展。
- [x] `modules/hrm/module.go` Quick Link 通过 `.RequireAuthz("hrm.employees", "create")`，未授权用户不再看到“新建员工”入口，保留 `.RequirePermissions` 作为 shadow fallback。
- [x] 若 HRM 未来新增 Dashboard/Reports，需在本计划输出的 checklist 中说明如何声明 `AuthzObject` 以及 Quick Link/Navigation 的层级策略：建议使用对象前缀 `hrm.dashboard`/`hrm.reports`，导航父项沿用 HRM，子项声明 `AuthzAction`=`list|view`，Quick Link 通过 `.RequireAuthz` 对应能力，禁用 legacy `Permissions`。

### 5. 测试、E2E 与记录
- [x] M1：以单元/少量集成覆盖授权通过/拒绝路径：`go test ./modules/hrm/...`，模板改动后执行 `templ generate && make css` 并确认 `git status --short` 干净。
- [x] M3：扩充 `e2e/tests/employees/employees.spec.ts`、在 `pkg/commands/e2e/seed.go` 增加缺权账号并对齐策略（新增 nohrm@example.com + 403 覆盖）。
- [x] M2：`make authz-test authz-lint && go test ./pkg/authz/... ./modules/hrm/...` 结果记录到 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`。
- [ ] M3：`docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 记录 shadow/enforce 切换命令与 diff。
- [x] CI/Actions：PR #92 lint/action 失败已排查。问题一：templ fmt 生成物未提交（已在 d3e36640/010489ae 修复）；问题二：golangci-lint nilnil 检查指向 HRM service mocks 返回 nil 值+nil err（已在 a63c3b99/da9bb6f6 修复）。已触发 run #96 复验。

### 6. 灰度与回滚
- [ ] M1：开发阶段默认直接 enforce，必要时用 `AUTHZ_ENFORCE=0` 做一次 shadow 对比。
- [ ] M3：如进入灰度/上线，执行 shadow→enforce，对 enforce 观察 ≥48h，记录命令与差异。
- [ ] 回滚简化：关闭 flag 或 revert 授权提交；若已进入 M3，再补充 `policy.csv` 恢复步骤与记录。

### 7. Policy 维护（新增）
- [x] 在 `config/access/policies/hrm/`（或现有片段目录）定义 `hrm.employees:*`、`hrm.positions:*` 等 capability 对应的 policy 行，命名遵循现有“模块.资源.动作”语义（新增 hrm/positions.csv）。
- [x] M2：执行最小 `make authz-pack` 产出供其他模块/脚本复用，可不提交产物，但需确认无报错；如提交，PR 贴关键 diff。（已执行并更新 policy.csv/.rev）
- [ ] M3：正式 `make authz-pack` 并提交产物；若 E2E/演示租户需要快速赋权，补充 `scripts/authz/verify`/`go test ./pkg/authz/...` 中的 fixture，避免策略 drift。

## 与 015B 的接口契约
- 014B 交付：controller 403 props 与模板暴露 `pageCtx.AuthzState().MissingPolicies`/`SuggestDiff`，临时 Unauthorized UI 占位，按钮跳转权限申请接口；字段/触发点保持稳定。
- 015B 交付：最终 Unauthorized UI/交互与权限申请流程，基于上述数据结构编写验收测试（含有/无权限场景）。
- 若需新增字段/事件，由 014B 在此文档补充并与 015B 确认。

## 交付物
- HRM 控制器/服务授权 helper、能力矩阵、403 体验。
- 更新后的 `modules/hrm/presentation/templates`、`viewmodels`、`mappers`、`locales`、Quick Links/导航配置。
- Go 单元测试通过；上线前补 Playwright 覆盖、`make authz-test authz-lint` 记录与 `scripts/authz/verify` 差异日志。
- README/CONTRIBUTING/AGENTS 追加 HRM 授权调试与 e2e 运行说明；进入发布阶段时再补 `docs/dev-records` 的 readiness/灰度条目。

## 验收标准
- M1（开发）：`go test ./modules/hrm/...`、`templ generate && make css`、`make check tr`（如修改 locale）通过，`git status --short` 在生成命令后保持干净。
- M2（上线前准备）：`make authz-test authz-lint` 通过；最小 `make authz-pack` 可执行（如提交则 diff 明确）。
- M3（灰度/发布）：Playwright 覆盖有/无权限场景；shadow→enforce 记录（如执行）；HRM enforce ≥48h 观察无异常；未授权用户无法通过直接 POST 绕过 controller guard（service 层测试覆盖）。
- HRM 模板/组件不再包含 `user.Can` 或硬编码权限逻辑，所有入口均由 `pageCtx.CanAuthz` 或 controller props 控制；403 页面展示 MissingPolicies 与申请入口。
- 进入发布/灰度阶段时，`docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`、`...014-CASBIN-ROLLING.md` 补充对应记录；README/CONTRIBUTING/AGENTS 更新完成。

## 与 014 主计划衔接节点
- **HRM-M1（控制器就绪）**：EmployeeController 等入口全部调用 HRM 版 `ensureAuthz`，并在模板展示 403，满足 014 步骤 2 前两 bullet。
- **HRM-M2（服务/模板就绪）**：服务层授权、模板/Quick Links/UI 交互全部落地，`pageCtx.CanAuthz` 在 HRM 页面普及，可通知 015B 接入 Unauthorized 组件。
- **HRM-M3（灰度完成）**：`AUTHZ_ENFORCE` 在 HRM enforce ≥48 小时无 diff，dev-records 记录完备，主计划可继续推进 Logging 阶段。

> 备注：本计划与 DEV-PLAN-015B 紧密协作。模板需提前暴露 `MissingPolicies`、`SuggestDiff()` 数据结构，确保 015B 可以无缝替换临时 Unauthorized 组件。
