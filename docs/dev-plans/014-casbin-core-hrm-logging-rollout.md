# DEV-PLAN-014：Casbin 模块改造（Core/HRM/Logging）

**状态**: 草拟中（2025-01-15 10:10）

## 背景
- DEV-PLAN-013 交付 Casbin 基础设施、policy 导出与 Feature Flag 之后，需要在 Core、HRM、Logging 三个模块率先落地授权改造，验证“多资源 RBAC + 基础 ABAC”在业务场景中的可行性。
- 当前模块仍依赖 `user.Can`、模板内联权限判断与松散的导航控制；未授权用户依旧可以访问大量页面并执行危险操作。
- 产品侧要求在最短周期内让核心模块具备统一的 Casbin 授权能力，并提供分批灰度、回滚剧本，避免一次性切换导致大面积停机。

## 前置依赖
- DEV-PLAN-012/013 交付的 `pkg/authz`、`config/access/{model.conf,policy.csv}`、`scripts/authz/export`、`scripts/authz/verify` 以及 `make authz-test`/`make authz-lint` 需保持可用；项目尚未投产，可在单机验证。
- DEV-PLAN-015A 已于 2025-12-04 完成，`policy_change_requests`、`/core/api/authz/*`、Authz.Debug、PolicyDraftService、bot/CLI 等能力均可直接复用；改造中出现的权限申请入口或调试需求一律对接 015A API，而非自建脚本。
- 若缺少最新旧权限映射，可临时运行 `scripts/authz/export`/`verify` 生成，无需审批。
- README/CONTRIBUTING/AGENTS 必须包含 Casbin 运维指引；若缺少，则在本计划中直接补齐。
- `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 尚未在仓库中创建；进入任一模块改造前先建该文件，并把每次执行 `make authz-test authz-lint && go test ./pkg/authz/...` 的结果登记进去，作为 readiness 记录。
- `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 同样缺失；在进入步骤 5 的分批灰度前创建并持续记录 `AUTHZ_ENFORCE` 启停命令、观测结果与回滚演练。
- **验收方法**：每次进入模块改造前运行 `make authz-test authz-lint && go test ./pkg/authz/...`，并在个人笔记或 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 简要记录结果（无需截图）。

## 现状检查（2025-01-15 14:45）
- 在 `feature/dev-plan-014` 分支上重新执行 `make authz-test` 与 `make authz-lint`，全部通过；待 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 建立后，将上述 readiness 命令与成功结论同步登记。
- DEV-PLAN-015A 的交付物已在仓库中（`docs/dev-plans/015A-casbin-policy-platform.md` 标记为 ✅ 完成），`policy_change_requests` schema、REST API、Authz.Debug 与 bot/CLI 可直接被 014 复用。
- README 与 AGENTS.md 已包含策略草稿 API、Bot 与运行指引；`docs/CONTRIBUTING.MD` 目前尚未覆盖 Casbin 运维，需在步骤 4 的文档更新中补齐。

## 目标
1. Core 模块（用户、角色、组、上传）全部控制器、服务、导航组件改用 `pkg/authz`，并补齐授权失败/成功路径的单元测试。
2. HRM 模块（员工列表/详情/创建/编辑/删除、批量导入）引入 `Employee.*` 权限校验，包括 controller、service、Quick Links、导航及 e2e 覆盖。
3. Logging 模块的页面、API、导航入口均基于 `Logs.View` Casbin 判定，未授权访问返回 403 + 审计日志。
4. 底层导航（sidebar、quick links）、模板渲染逻辑统一通过 controller 注入的布尔值或 helper 进行权限控制，杜绝在模板里直接调用 `user.Can`。
5. 通过 `AUTHZ_ENFORCE` Feature Flag 分批灰度 Core→HRM→Logging 模块，记录每个批次的启停命令、观测指标、回滚脚本。

## 风险
- 如果模块未完全覆盖授权点（例如 service 层或 background job 漏掉），可能导致权限绕过。
- HRM/Logging 页面在 HTMX/模板层面也需要同步更新，否则 UI 与 API 判定不一致。
- 与 legacy `user.Can` 并行时可能出现双重鉴权影响性能，需要监控请求延迟。
- 分批灰度需要严格的租户名单与回滚流程，否则容易造成租户体验差异和支持成本。
- 多模块同时迭代，团队协作难度大，需要明确的依赖与验收标准。

## 进展同步（2025-12-04 18:45）
- `feature/dev-plan-014` 分支已完成 Core Controller 层统一入口：新增 `modules/core/presentation/controllers/authz_helpers.go`，会自动创建 `authz.ViewState`、在 shadow 模式下回退 `user.Can` 判定，并向模板记录 `MissingPolicies`。
- 用户、角色、组、上传四个 Controller 全量改用 `ensureAuthz` + legacy 权限映射；`go test ./modules/core/presentation/controllers` 现已通过，确保 shadow flag 下依旧返回 403。
- Spotlight Quick Links / 侧边栏导航现已复用 `authz.ViewState`：`pkg/middleware/sidebar` 会按 `NavigationItem.AuthzObject/AuthzAction` 调 `authzutil.CheckCapability`，Quick Links 则在 `pkg/spotlight/items.go` 中做同样校验并支持链式 `.RequireAuthz()`。
- Users/Roles/Groups 列表页的“新建”入口与移动抽屉按钮现已通过 `pageCtx.CanAuthz("core.*", "create")` 直接读取 `authz.ViewState` 决定可见性，模板仅在掌握相应 capability 时渲染按钮/抽屉，杜绝无权用户的 UI 暴露。
- PageContext 结构体现已内建 `authz.ViewState` 持有者（`pkg/types/pagecontext.go`），`middleware.WithPageContext` 与 `authzutil.EnsureViewState` 会自动同步 state，使任何模板/组件均可调用 `pageCtx.CanAuthz(object, action)`；ExcelExportService 同步接入 `authorizeCore` 保护 `core.exports export` 动作，为链路导出/策略审核打好基础。

## 当前问题（2025-12-04 18:45）
- shadow 模式虽然通过 legacy 权限兜底，但 service 层仍然缺少 `authorizeCore` 全覆盖，后台 Job 也未接入，存在漏网风险。
- 模板与页面 ViewModel 依旧大量直接读取 `user.Can`，`authz.ViewState` 注入范围仅限 Controller/Nav/QuickLinks，UI 尚未消费 `MissingPolicies`。
- `go test ./modules/core/services/...` 暂未补充 Casbin 授权路径；Excel Export、Upload 等服务仍会因“用户不存在”报错，而非 403。

## 后续待办
- 在 Core service（user/role/group/upload）与后台任务中使用 `authorizeCore`（含 attributes）补齐写操作守卫，并新增对应单元测试。
- 将 `authz.ViewState` 注入到 Controller → 模板的 props，去除模板内的 `user.Can` 判断，并补充暂存 Unauthorized 组件及 `/core/api/authz/requests` 入口。
- 为 `ensureAuthz` 的 `MissingPolicies` 输出提供 UI 钩子，与 015A 的 `/core/api/authz/requests` 请求入口联动，记录 diff。
- 补录 readiness/灰度日志：下一次进入 HRM/Logging 改造前继续更新 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 与 `...014-CASBIN-ROLLING.md`。

## 实施步骤
1. **[ ] Core 模块改造**
   - 控制器层：在 `modules/core/presentation/controllers` 中为用户/角色/组/上传的每个 handler 调用 `authz.Authorize`，并处理 403 视图/JSON。
   - 服务层：在 `modules/core/services` 中的写操作（创建/更新/删除）加入 guard，并确保后台作业使用统一 helper。
   - 测试：新增/更新 controller & service tests，覆盖授权通过/失败路径；使用 `pkg/authz` 的 mock 或 shadow Enforcer。
   - 导航：`pkg/types/navigation`、侧栏模板、Quick Links 改为由 controller 注入 `CanViewX` 布尔值。
2. **[ ] HRM 模块改造**
   - `EmployeeController` 全部 action 调用 `authz.Authorize`（`Employee.List`, `Employee.View`, `Employee.Create`, `Employee.Update`, `Employee.Delete` 等）。
   - 服务与后台作业（导入、批量更新）加入相同 guard；若涉及 domain/ABAC 条件，使用 `authz.CheckWithAttrs`。
   - Quick Links、导航、组件在渲染前检查权限，未授权时隐藏入口或展示统一 403 组件。
   - e2e/集成测试：编写具备/不具备 `Employee.*` 权限的用户场景，验证 UI/API 响应。
3. **[ ] Logging 模块改造**
   - 在 controller middleware 中统一调用 `authz.Authorize(..., Logs.View)`，所有 API/页面复用该逻辑。
   - 记录未授权访问到审计日志（subject/object/action/domain/tenant/IP），满足合规要求。
   - 模板与导航入口只有在授权成功时才渲染，未授权时显示空态描述与“申请权限”提示；在 DEV-PLAN-015 UI 完成前，本计划提供最小版 Unauthorized 组件。
4. **[ ] 公共层与 UI 接口**
   - 去除模板中的 `user.Can`，新增 `authz.ViewState` 结构，通过 controller 注入模板；所有 403 响应包含 `MissingPolicies`，供 015B 提供的组件消费。
   - 在 `pkg/middleware/sidebar`、`pkg/types/navigation` 中按照 Casbin 判定过滤导航项，并将可见性布尔值纳入 `authz.ViewState`。
   - **接口契约**（聚焦 015B 复用）：
     - `authz.ViewState` 至少包含 `Subject`, `Tenant`, `CanView*` 布尔值、`MissingPolicies` 列表以及 `SuggestDiff()` 等 helper，确保 015B 能直接渲染 Unauthorized/PolicyInspector。
     - 当 015B 组件尚未交付时，014 仅提供临时占位（如简单 403 文案），并通过按钮跳转到 015A 已提供的 `/core/api/authz/requests` 列表；一旦 015B 可用，替换为其模板而无需后端调整。
     - PolicyInspector 相关数据全部来自 `/core/api/authz/debug`，controller 只需注入请求参数与调用入口，严禁自建重复逻辑。
   - README/CONTRIBUTING/AGENTS 补充 controller 注入、`authz.ViewState` 结构说明、如何调用 015A API 示例即可，不必附截图。
     - 现状缺口：README 与 AGENTS 已就绪，但 `docs/CONTRIBUTING.MD` 尚未包含 Casbin/策略草稿操作说明；本步骤中补写对应章节或引用 README，以完成“README/CONTRIBUTING/AGENTS 同步”要求。
   - `docs/dev-records` 中记录与 015B 联调的关键命令与结论，证明接口可复用。
5. **[ ] 分批灰度、Parity 与回滚**
   - 仍按“模块 × 租户”规划启停，但早期可在本地/单租户验证通过后直接切换，不必维护复杂矩阵。
   - 启用 `AUTHZ_ENFORCE` 前后各运行一次 `go run scripts/authz/verify/main.go --tenant <id>`；若出现 diff，立即排查修复，无需产出正式报告。
   - 由于缺乏真实流量，监控以自测为主：关注关键流程是否出现 403/500，日志是否异常即可。
   - 回滚采用“关闭 feature flag + git revert”即可；必要时运行 `scripts/authz/export` 生成旧策略。整理为简短操作说明（命令示例 + 检查点），无需额外脚本。

## 里程碑
- M1：Core 模块全部控制器/服务切换到 `authz`，测试通过，模板不再直接依赖 `user.Can`。
- M2：HRM 模块完成授权改造并通过 e2e/集成测试，`AUTHZ_ENFORCE` 在至少一个租户上以 enforce 模式运行 48 小时无差异。
- M3：Logging 模块完成改造，分批灰度记录齐全，回滚演练完成。

## 交付物
- Core/HRM/Logging 模块中的授权改造代码、测试、模板更新。
- 供 015B 复用的 `authz.ViewState` 辅助结构、`MissingPolicies` 注入逻辑以及导航可见性布尔值；临时占位 UI 仅作兜底，最终 Unauthorized/PolicyInspector 由 015B 接管。
- README/CONTRIBUTING/AGENTS 更新、`docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`（readiness/parity 记录）、`docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`（启停日志与问题排查）、简化版回滚说明。
- 精简版分批灰度计划、`go run scripts/authz/verify` 的差异日志、feature flag 操作示例；与 015B 联调记录至少一次，展示“014 启用 + 015A API + 015B UI”闭环。
