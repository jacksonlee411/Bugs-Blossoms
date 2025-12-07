# DEV-PLAN-014：Casbin 模块改造（Core/HRM/Logging）

**状态**: 已完成（2025-12-07 01:05 UTC：全模块 enforce 验证与记录收尾）

## 背景
- DEV-PLAN-013 交付 Casbin 基础设施、policy 导出与 Feature Flag 之后，需要在 Core、HRM、Logging 三个模块率先落地授权改造，验证“多资源 RBAC + 基础 ABAC”在业务场景中的可行性。
- 当前模块仍依赖 `user.Can`、模板内联权限判断与松散的导航控制；未授权用户依旧可以访问大量页面并执行危险操作。
- 产品侧要求在最短周期内让核心模块具备统一的 Casbin 授权能力，并提供分批灰度、回滚剧本，避免一次性切换导致大面积停机。

## 前置依赖
- DEV-PLAN-012/013 交付的 `pkg/authz`、`config/access/{model.conf,policy.csv}`、`scripts/authz/export`、`scripts/authz/verify` 以及 `make authz-test`/`make authz-lint` 需保持可用；项目尚未投产，可在单机验证。
- DEV-PLAN-015A 已于 2025-12-04 完成，`policy_change_requests`、`/core/api/authz/*`、Authz.Debug、PolicyDraftService、bot/CLI 等能力均可直接复用；改造中出现的权限申请入口或调试需求一律对接 015A API，而非自建脚本。
- 若缺少最新旧权限映射，可临时运行 `scripts/authz/export`/`verify` 生成，无需审批。
- README/CONTRIBUTING/AGENTS 必须包含 Casbin 运维指引；若缺少，则在本计划中直接补齐。
- `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`、`docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 已建立并记录 2025-12-06/12-07 的 readiness 与灰度日志；后续进入 HRM/收尾阶段持续追加。
- **验收方法**：每次进入模块改造前运行 `make authz-test authz-lint && go test ./pkg/authz/...`，并在个人笔记或 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 简要记录结果（无需截图）。

## 现状检查（最新）
- DEV-PLAN-014A（Core 收尾）完成：controller/service/模板全面接入 `authz`，统一 Unauthorized 组件与 403 payload，shadow → enforce 灰度/回滚记录已写入 dev-records。
- DEV-PLAN-014B（HRM 改造）完成：controller/service/模板/导航/Quick Links/Spotlight 接入 `authz` + Unauthorized 占位，MissingPolicies 钩子到位，HRM enforce（无灰度）已记录。
- DEV-PLAN-014C（Logging 改造）完成：Logging 建立 `ensureAuthz` 入口、导航/Quick Links 过滤、403 JSON/Unauthorized 占位与审计链路，enforce 自测与记录补齐。
- DEV-PLAN-014D（公共层与 UI 接口）完成：403 契约、导航/Quick Links 过滤接口、Unauthorized 契约与文档示例补齐，验证记录已写入 dev-records。
- 2025-12-07 01:05 UTC 补充 enforce 自测：`AUTHZ_ENFORCE=1 go test ./modules/{core,hrm,logging}/...` 与 `go test ./pkg/authz/...` 全部通过并登记；DEV-PLAN-015A 已完成，可持续复用策略申请/调试接口。

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

## 进展同步（最新）
- Core 侧（014A）完成：controller/service/导航/Quick Links 全量切换 `authz`，模板统一使用 `pageCtx.CanAuthz` 与 Unauthorized 组件，shadow → enforce 灰度与回滚剧本已记录。
- HRM 侧（014B）完成：controller/service/模板/导航/Quick Links/Spotlight 接入 `authz` 与 Unauthorized，占位 UI 与 MissingPolicies 提供到位；readiness/灰度记录写入 dev-records，AGENTS 约束下无 shadow 灰度。
- Logging 侧（014C）完成：新增 Logging controller + authz helper、导航/Quick Links/Spotlight 按 `logging.logs:view` 过滤，403 JSON/Unauthorized 占位与审计链路落地，相关 readiness/灰度日志写入 dev-records。
- 公共层（014D）完成：差距表、统一 403 payload、Unauthorized 契约与导航/Quick Links 过滤接口已落主干，跨模块验证/文档回填/测试记录同步完成。

## 当前问题（最新）
- 无（014D 验证、文档与跨模块 go test 已完成；Core/HRM/Logging enforce 与回滚路径均在 dev-records 登记）。

## 后续待办
- 无（014D 验证、分批灰度/回滚登记与跨模块测试已完成）。

## 实施步骤
1. **[x] Core 模块改造（014A 完成）**
   - 控制器层：在 `modules/core/presentation/controllers` 中为用户/角色/组/上传的每个 handler 调用 `authz.Authorize`，并处理 403 视图/JSON。
   - 服务层：在 `modules/core/services` 中的写操作（创建/更新/删除）加入 guard，并确保后台作业使用统一 helper。
   - 测试：新增/更新 controller & service tests，覆盖授权通过/失败路径；使用 `pkg/authz` 的 mock 或 shadow Enforcer。
   - 导航：`pkg/types/navigation`、侧栏模板、Quick Links 改为由 controller 注入 `CanViewX` 布尔值。
2. **[x] HRM 模块改造（014B 完成）**
   - `EmployeeController` 全部 action 调用 `authz.Authorize`（`Employee.List`, `Employee.View`, `Employee.Create`, `Employee.Update`, `Employee.Delete` 等）。
   - 服务与后台作业（导入、批量更新）加入相同 guard；若涉及 domain/ABAC 条件，使用 `authz.CheckWithAttrs`。
   - Quick Links、导航、组件在渲染前检查权限，未授权时隐藏入口或展示统一 403 组件。
   - e2e/集成测试：编写具备/不具备 `Employee.*` 权限的用户场景，验证 UI/API 响应。
3. **[x] Logging 模块改造（014C 完成）**
   - 在 controller middleware 中统一调用 `authz.Authorize(..., Logs.View)`，所有 API/页面复用该逻辑。
   - 记录未授权访问到审计日志（subject/object/action/domain/tenant/IP），满足合规要求。
   - 模板与导航入口只有在授权成功时才渲染，未授权时显示空态描述与“申请权限”提示；在 DEV-PLAN-015 UI 完成前，本计划提供最小版 Unauthorized 组件。
4. **[x] 公共层与 UI 接口（014D 完成）**
   - 去除模板中的 `user.Can`，新增 `authz.ViewState` 结构，通过 controller 注入模板；所有 403 响应包含 `MissingPolicies`，供 015B 提供的组件消费。
   - 在 `pkg/middleware/sidebar`、`pkg/types/navigation` 中按照 Casbin 判定过滤导航项，并将可见性布尔值纳入 `authz.ViewState`。
   - **接口契约**（聚焦 015B 复用）：
     - `authz.ViewState` 至少包含 `Subject`, `Tenant`, `CanView*` 布尔值、`MissingPolicies` 列表以及 `SuggestDiff()` 等 helper，确保 015B 能直接渲染 Unauthorized/PolicyInspector。
     - 当 015B 组件尚未交付时，014 仅提供临时占位（如简单 403 文案），并通过按钮跳转到 015A 已提供的 `/core/api/authz/requests` 列表；一旦 015B 可用，替换为其模板而无需后端调整。
     - PolicyInspector 相关数据全部来自 `/core/api/authz/debug`，controller 只需注入请求参数与调用入口，严禁自建重复逻辑。
   - README/CONTRIBUTING/AGENTS 已补充 controller 注入、`authz.ViewState` 结构说明、403 JSON 示例及 `/core/api/authz/requests|debug` 调用示例。
   - `docs/dev-records` 中记录与 015B 联调的关键命令与结论，证明接口可复用。
5. **[x] 分批灰度、Parity 与回滚**
   - 仍按“模块 × 租户”规划启停，但早期可在本地/单租户验证通过后直接切换，不必维护复杂矩阵；Core/HRM/Logging enforce 记录已写入 dev-records。
   - 启用 `AUTHZ_ENFORCE` 前后各运行一次 `go run scripts/authz/verify/main.go --tenant <id>`；若出现 diff，立即排查修复，无需产出正式报告。
   - 由于缺乏真实流量，监控以自测为主：关注关键流程是否出现 403/500，日志是否异常即可。
   - 回滚采用“关闭 feature flag + git revert”即可；必要时运行 `scripts/authz/export` 生成旧策略。整理为简短操作说明（命令示例 + 检查点），无需额外脚本。

## 里程碑
- [x] M1：Core 模块全部控制器/服务切换到 `authz`，测试通过，模板不再直接依赖 `user.Can`。（014A 完成）
- [x] M2：HRM 模块完成授权改造并通过 e2e/集成测试，`AUTHZ_ENFORCE` 在至少一个租户上以 enforce 模式运行 48 小时无差异。（014B 完成，持续跟踪灰度记录）
- [x] M3：Logging 模块完成改造，分批灰度记录齐全，回滚演练完成。（014C 完成，持续监控/回滚阈值另行追踪）

## 交付物
- Core/HRM/Logging 模块中的授权改造代码、测试、模板更新。
- 供 015B 复用的 `authz.ViewState` 辅助结构、`MissingPolicies` 注入逻辑以及导航可见性布尔值；临时占位 UI 仅作兜底，最终 Unauthorized/PolicyInspector 由 015B 接管。
- README/CONTRIBUTING/AGENTS 更新、`docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`（readiness/parity 记录）、`docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`（启停日志与问题排查）、简化版回滚说明。
- 精简版分批灰度计划、`go run scripts/authz/verify` 的差异日志、feature flag 操作示例；与 015B 联调记录至少一次，展示“014 启用 + 015A API + 015B UI”闭环。
