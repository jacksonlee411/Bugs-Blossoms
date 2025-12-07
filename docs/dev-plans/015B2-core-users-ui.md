# DEV-PLAN-015B2：Core 用户管理 UI

**状态**: 草拟中（2025-12-07 01:36 UTC）  
**范围**: `modules/core` 用户详情页的权限可视化、草稿操作与分页

## 目标
- 将 Permissions Tab 拆为继承角色、直接策略、Domain Overrides 三列，明确来源与状态。
- 支持批量新增/撤销策略，展示审批/部署状态，未授权时降级只读。
- 复用 Session 暂存与分页，保证多次操作与刷新后的状态一致。

## 实施步骤
1. [ ] 三列视图 —— `GET /core/api/authz/policies?subject=user:<id>` 填充三列，展示 `Awaiting PR/Pending Deploy/Active` 状态（来自 `GET /core/api/authz/requests`）。
2. [ ] 变更与状态 —— 各列“添加”对话框生成 `g/p` diff；支持批量撤销与 domain 筛选，操作落地草稿。
3. [ ] 权限与测试 —— 授权失败显示只读视图；补 controller/viewmodel/templ 测试覆盖有/无权限。
4. [ ] 暂存/分页 —— 复用 `POST/DELETE /core/api/authz/policies/stage` 与分页接口，Tab 切换保留暂存，刷新后差异不丢。

## 依赖
- 015A API/`/core/api/authz/policies|requests`；015B1 暂存/分页实现可直接复用。

## 交付物
- 用户权限页模板/partials，支持 HTMX 交互。
- 暂存与分页逻辑接入、相关测试。

## 验收
- 三列均可分页浏览并区分来源/状态；有权限可暂存并提交草稿，刷新后暂存保留；无权限只读且可发起申请。
