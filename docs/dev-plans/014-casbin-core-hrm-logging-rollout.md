# DEV-PLAN-014：Casbin 模块改造（Core/HRM/Logging）

**状态**: 草拟中（2025-01-15 10:10）

## 背景
- DEV-PLAN-013 交付 Casbin 基础设施、policy 导出与 Feature Flag 之后，需要在 Core、HRM、Logging 三个模块率先落地授权改造，验证“多资源 RBAC + 基础 ABAC”在业务场景中的可行性。
- 当前模块仍依赖 `user.Can`、模板内联权限判断与松散的导航控制；未授权用户依旧可以访问大量页面并执行危险操作。
- 产品侧要求在最短周期内让核心模块具备统一的 Casbin 授权能力，并提供分批灰度、回滚剧本，避免一次性切换导致大面积停机。

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
   - 模板与导航入口只有在授权成功时才渲染，未授权时显示空态描述与“申请权限”链接（由 DEV-PLAN-015 提供 UI）。
4. **[ ] 公共层更新**
   - 去除模板中的 `user.Can`，新增 `authz.ViewState` 结构，通过 controller 注入模板。
   - 在 `pkg/middleware/sidebar`、`pkg/types/navigation` 中按照 Casbin 判定过滤导航项。
   - 提供统一的 `UnauthorizedComponent` 模板，供 Core/HRM/Logging 复用。
5. **[ ] 分批灰度与回滚**
   - 拟定“模块 × 租户”灰度矩阵：例如先在 internal tenant 对 Core 强制启用，再推广到 beta 租户，完成后切 HRM、Logging。
   - 使用 `AUTHZ_ENFORCE` 控制器或租户配置表实现按租户开关，并在 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 记录启停日志。
   - 准备回滚脚本：关闭 flag、恢复 `user.Can` 逻辑（保留在代码中但受 flag 控制）、恢复旧导航渲染。

## 里程碑
- M1：Core 模块全部控制器/服务切换到 `authz`，测试通过，模板不再直接依赖 `user.Can`。
- M2：HRM 模块完成授权改造并通过 e2e/集成测试，`AUTHZ_ENFORCE` 在至少一个租户上以 enforce 模式运行 48 小时无差异。
- M3：Logging 模块完成改造，分批灰度记录齐全，回滚演练完成。

## 交付物
- Core/HRM/Logging 模块中的授权改造代码、测试、模板更新。
- 统一的导航/Unauthorized 组件、`authz.ViewState` 辅助结构。
- 分批灰度计划、`docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`、回滚 runbook。
