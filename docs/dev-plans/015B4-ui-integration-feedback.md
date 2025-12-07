# DEV-PLAN-015B4：UI 串联与反馈

**状态**: 草拟中（2025-12-07 01:36 UTC）  
**范围**: 草稿提交/状态反馈、错误处理、bot 联动与 a11y

## 目标
- 为所有草稿操作提供即时反馈与跳转入口，串联请求状态与 bot 结果。
- 统一错误处理，避免静默失败。
- 确保新增组件满足可访问性（a11y）要求。

## 实施步骤
1. [ ] 反馈与跳转 —— 草稿操作返回 toast/HTMX snippet，包含 `request_id`、SLA 倒计时；提供“查看草稿”跳转 `/core/authz/requests/{id}`，展示 diff/审批状态/bot 日志。
2. [ ] bot 联动 —— PolicyInspector/Unauthorized 显示 bot 状态，`status=failed` 时展示“重试 bot”链接。
3. [ ] 错误处理 —— 后端 4xx/5xx 返回 `HX-Trigger: {"showErrorToast": {"message": "<具体原因>"}}`；表单校验失败用 422 + 带错误提示 partial；前端全局监听 `showErrorToast` 调用统一 toast。
4. [ ] 可访问性 —— 新增弹窗/抽屉/按钮需键盘全程操作（Tab/Shift+Tab/Enter/Space/Esc），语义标签与 ARIA 齐备（如 `role="dialog"`, `aria-modal`, `aria-labelledby`）；弹出时聚焦首元素，关闭时焦点回到触发按钮；使用 axe 等工具确保无严重 a11y 问题。

## 依赖
- 015A 的草稿/请求状态 API；015B1/015B2/015B3 的前端交互场景。

## 交付物
- 统一 toast 触发约定、跳转入口和 bot 状态联动逻辑。
- a11y 辅助脚本/说明，错误处理示例。

## 验收
- 草稿操作均有明确 toast/链接反馈，失败场景能提示具体原因；bot 失败场景可引导重试；核心页面通过键盘可用性检查且无严重 a11y 违规。
