# DEV-PLAN-015B2：Core 用户管理 UI

> [!IMPORTANT]
> 自 DEV-PLAN-015C 起，策略草稿（requests）/审批/bot 链路已移除；当前唯一口径为管理员直接维护生效（`POST /core/api/authz/policies/apply`）。本文仅作历史记录，不再作为 SSOT。

**状态**: 进行中（M1/M2 完成，M3 待启动，2025-12-11）  
**范围**: `modules/core` 用户详情页的权限可视化、草稿/申请闭环与分页

## 目标
- 将 Permissions Tab 拆为继承角色、直接策略、Domain Overrides 三列，明确来源与状态（含 request_id）。
- 支持批量新增/撤销策略，展示审批/部署状态并可一键发起申请，未授权时降级只读。
- 接入 PolicyInspector/Unauthorized 组件，提供缺失策略提示与申请入口。
- 复用 Session 暂存与分页，保证多次操作与刷新后的状态一致。

## 实施步骤
1. [x] 三列视图 —— 权限板（Policy Board）三列已接入并合并 main，展示 request_id、来源和状态，读写路径按 015A 规范。
2. [x] 变更与状态 —— 各列“添加”对话框生成 `g/p` diff；支持批量撤销与 domain 筛选，操作落地草稿并触发申请入口。
3. [x] 授权、申请与测试 —— Tab 顶部统一“提交草稿/申请权限”按钮；各列“添加”仅收集 diff；接入 PolicyInspector/Unauthorized，共有/无权限、申请触发与 request_id 展示测试补齐。
4. [x] 暂存/分页 —— 复用 `POST/DELETE /core/api/authz/policies/stage` 与分页接口，Tab 切换保留暂存，刷新后差异不丢。
5. [x] 文档、翻译与可访问性 —— 补 README/AGENTS 示例，更新 locales（运行 `make check tr`），确保对话框/抽屉具备 ARIA/键盘可达性，并用 axe+键盘巡检记录结果。
6. [x] 立即生效反馈 —— UI 在 apply 成功后展示 toast（含 revision/added/removed）；当遇到 `409 base_revision` 冲突时提示刷新重试，并保留 `request_id` 便于排障。

## 依赖
- 015A API/`/core/api/authz/policies|requests`；015B1 暂存/分页实现可直接复用。

## 交付物
- 用户权限页模板/partials，支持 HTMX 交互与 PolicyInspector/Unauthorized 组件。
- 暂存与分页逻辑接入、相关测试，覆盖申请按钮与 request_id 展示。
- 文档与多语言更新，可访问性校验记录。

## 验收
- 三列均可分页浏览并区分来源/状态；有权限可暂存并提交草稿，刷新后暂存保留；无权限只读且可发起申请。
- 创建草稿后 5 分钟内可见状态更新；UI 轮询并显示 request_id，与 015A 日志对应，超时有黄条兜底与重试/日志链接。
- PolicyInspector/Unauthorized 组件可用，申请入口可触发；对话框/抽屉支持键盘导航与必要 ARIA。
- README/AGENTS 示例与 locales 更新完成，`make check tr`/相关测试通过。

## 里程碑与方案
- M1：交付三列视图 + Unauthorized 组件（✅ 已合并 main，修复关联 E2E 用例）。
- M2：上线草稿/申请入口、request_id 展示与分页/暂存（✅ 已完成，等待长稳观察）。
- M3：验证 UI→bot→状态回写闭环运行 ≥2 周，并完成文档/翻译/a11y 记录（未启动）。

## 最新进展（2025-12-09）
- 暂存草稿生成 g/p JSONPatch，支持批量撤销与域筛选，Tab 顶部统一提交/申请入口。
- apply 后不再轮询；通过 toast + `request_id` 串联日志；Stage 文案完成多语言。
- Policy Board 继续复用暂存/分页，刷新后暂存保持；README/AGENTS 示例与 locales 已更新（`make check tr` 通过）；用户详情对话框/抽屉完成键盘巡检与 axe smoke，未发现阻塞级别的 a11y 问题。
