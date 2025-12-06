# DEV-PLAN-014D：Casbin 公共层与 UI 接口落地计划

**状态**: 规划中（2025-12-06 05:05 UTC） — 对应 DEV-PLAN-014 "公共层与 UI 接口" 子任务，聚焦 ViewState/UI 契约与文档/导航一致性。

## 背景
- DEV-PLAN-014 在步骤 4 要求统一公共层与 UI 接口：去除模板内 `user.Can`、通过 controller 注入 `authz.ViewState`、导航/Quick Links 依赖 Casbin，可为 015A/015B 的 Unauthorized/PolicyInspector 组件提供数据契约。
- Core/HRM 已在 014A/014B 落地部分能力（ViewState 注入、Unauthorized 占位、模板清理），Logging 尚未执行，跨模块的统一接口/文档/组件契约仍缺收敛。
- 015A 提供权限申请/调试 API，015B 将交付最终 Unauthorized/PolicyInspector UI，需要公共层保证字段、入口、导航可见性一致且无 legacy fallback。

## 目标
1. 公共层统一：`authz.ViewState` 在 Core/HRM/Logging controller 注入、导航/Quick Links/Sidebar/Spotlight 统一基于 `AuthzObject/Action` 过滤，移除模板中的 `user.Can` 与 legacy Permissions。
2. UI 契约完善：Unauthorized 占位与页面 props 暴露 MissingPolicies/SuggestDiff、subject/domain/object/action、/core/api/authz/requests|debug 参数，满足 015B 接口需求；HX/REST 403 payload 一致。
3. 文档与样例：README/CONTRIBUTING/AGENTS 补充公共层/模板接入说明；提供示例 payload/props，dev-records 记录关键命令与验证结果。

## 前置依赖
- 014A/014B readiness 已完成（authz.ViewState/ensureAuthz/authorizeCore/HRM helper 可复用）；014C 仅完成决策/部分 scaffold，需在本计划收敛 Logging。
- 遵循 AGENTS/CLAUDE：Go 1.24.10、DDD 分层、模板改动需 `templ generate && make css`，locale 改动跑 `make check tr`，策略改动跑 `make authz-pack`；禁止使用 sed。
- 现阶段不改 frozen 模块（billing/crm/finance）。

## 实施步骤
1. [x] 能力清单与差距梳理 —— 对 Core/HRM/Logging 的 controller/service/模板/导航/Quick Links 逐项盘点 `user.Can` 与 legacy Permissions；形成统一差距表（模块/文件/位置/遗留点/处理方式/状态），仅允许 Pending/Done；表格收录于 docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md 公共层小节，全部标记 Done 视为 UI-D1 完成。
2. [x] ViewState 注入一致化 —— 核验三模块 controller 入口均调用 `authzutil.EnsureViewState`/ensureAuthz；在 Logging 补齐 helper/中间件；确保 PageContext 持有 `AuthzState` 并向模板/组件透传。
3. [x] 模板/组件清理 —— 删除模板中的 `user.Can`/legacy 判断，统一使用 `pageCtx.CanAuthz(object, action)` 或 controller 注入的布尔 props；Logging 保持双 Tab 过滤（时间/用户/IP/UA/method/path，action_logs 可选 status）、空态/无权态一致；HTMX/REST 403 均渲染 Unauthorized 占位；运行 `templ generate && make css` 后确认工作区干净。
4. [x] Unauthorized 契约对齐 —— 为三模块的 Unauthorized props 定义统一结构（MissingPolicies、SuggestDiff、subject/domain/object/action、申请入口 URL、/core/api/authz/debug 参数），提供 015B 字段示例；统一 HX/REST 403 payload 字段名/结构，示例：`{\"error\":\"forbidden\",\"object\":\"<obj>\",\"action\":\"<act>\",\"subject\":\"<subj>\",\"domain\":\"<tenant>\",\"missing_policies\":[...],\"suggest_diff\":[...],\"request_url\":\"/core/api/authz/requests\",\"debug_url\":\"/core/api/authz/debug\"}`；更新 controller 403 响应与模板占位，确保 HX-Retarget 兼容；必要时在 pkg/templates/components 或模块 templates/components 复用。
5. [ ] 导航/Quick Links/Spotlight 统一过滤 —— 确保导航/Quick Links/Spotlight 数据源使用 `.RequireAuthz` 或 `AuthzObject/AuthzAction`，移除 legacy `.RequirePermissions`；验证 sidebar/spotlight 过滤在三模块一致，未授权用户不可见。
6. [ ] 文档与示例 —— 更新 README/CONTRIBUTING/AGENTS，补充公共层接入流程、Unauthorized 数据字段示例、403 JSON 示例（同上）、调用 `/core/api/authz/requests|debug` 示例链接；在 dev-records 记录 readiness 命令（`make authz-test authz-lint` 等）与契约验证结论。
7. [ ] 验证与记录 —— 运行 `go test ./pkg/authz/...` 及受影响模块包测试；若改模板/locale，执行 `templ generate && make css`、`make check tr`。在 docs/dev-records/DEV-PLAN-012-CASBIN-POC.md 与 014 rolling 中补充记录。

## 里程碑
- UI-D1：完成差距清单与 ViewState 注入一致化（步骤 1-2），三模块 controller 均具备 `AuthzState`。
- UI-D2：完成模板/组件清理、Unauthorized 契约对齐（步骤 3-4），HX/REST 403 响应一致。
- UI-D3：完成导航/Quick Links/Spotlight 过滤统一、文档与示例更新、测试与记录（步骤 5-7），可交付给 015B 联调。

## 提交要求
- 每完成一个阶段（UI-D1/UI-D2/UI-D3）推送远程一次，保持小步可审阅。
- 每次运行生成/校验命令（`templ generate && make css`、`make check tr`、`make authz-test authz-lint` 等）后记录结果，必要时同步 dev-records。
- 导航/Quick Links/Spotlight 过滤验证：使用有/无目标 capability 的账号各一次，验证可见性与直访 403 payload，结果写入 dev-records。

## 验收标准
- 模板/组件/导航/Quick Links 不再包含 `user.Can` 或 legacy Permissions 分支；均使用 `pageCtx.CanAuthz` 或 `.RequireAuthz`。
- Unauthorized 体验一致：403 响应与模板占位携带 MissingPolicies、SuggestDiff、subject/domain/object/action、authz requests/debug 参数，HX/REST 同步；未授权仓储不被调用。
- 三模块均通过相关测试；如修改模板/locale，`templ generate && make css`、`make check tr` 后 `git status --short` 干净；dev-records/README/CONTRIBUTING/AGENTS 更新完成。
