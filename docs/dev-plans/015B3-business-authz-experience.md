# DEV-PLAN-015B3：业务界面授权体验（HRM、Logging 等）

**状态**: 草拟中（2025-12-07 01:36 UTC）  
**范围**: HRM/Logging 等业务页面的 Unauthorized 组件、PolicyInspector 与授权契约

## 目标
- 提供统一 Unauthorized 组件与申请入口，直接写入 `suggested_diff`。
- 提供 PolicyInspector 抽屉，展示调试链路并可一键生成草稿。
- 确保 403 时模板获得完整 `authz.ViewState/MissingPolicies`，导航/Quick Links 可依据授权过滤。

## 实施步骤
1. [ ] Unauthorized 组件 —— 实现 `components/authorization/unauthorized.templ`，渲染缺失策略/参考文档/申请按钮；按钮 HTMX 调 `POST /core/api/authz/requests`。
2. [ ] PolicyInspector —— 仅 `Authz.Debug` 可见，调用 `GET /core/api/authz/debug?subject=&object=&action=&domain=`，展示命中规则、ABAC、latency，并支持“一键生成草稿”。
3. [ ] 控制器契约 —— HRM/Logging 403 时注入 `MissingPolicies`，Quick Links/Sidebar 基于 `authz.ViewState` 控制可见性。
4. [ ] 测试 —— 补 e2e/集成测试覆盖“有权限 vs 无权限”。

## 依赖
- 015A 调试/草稿 API 可用；各模块已集成 `authz.Authorize` 并能提供 `authz.ViewState`。

## 交付物
- Unauthorized/PolicyInspector 模板与辅助逻辑。
- HRM/Logging controller/template 改造、对应测试。

## 验收
- 未授权访问时显示统一组件并可成功发起草稿；持有 `Authz.Debug` 可查看并生成申请；导航/Quick Links 根据授权正确过滤。
