# DEV-PLAN-016：Authz UI a11y 巡检清单（手工）

> 说明：本文件用于沉淀“可复用的巡检步骤 + 记录模板”。可选用浏览器 axe 插件做人工巡检；仓库也提供了 Playwright + axe 的 smoke 用例（`e2e/tests/a11y/authz-ui.spec.ts`）用于快速回归。

## 覆盖页面（建议）
- 角色策略矩阵：`/roles/{id}/policies`
- 用户权限板：`/users/{id}/policies`
- Unauthorized：任意受保护页面的 403 渲染（HTMX + 非 HTMX）
- PolicyInspector：具备 `Authz.Debug` 权限时的调试面板
- Requests Center：`/core/authz/requests`、`/core/authz/requests/{id}`

## 键盘可用性清单（Tab/Shift+Tab/Enter/Space/Esc）
1. **可达性**：页面主操作（Create/Save/Delete/Submit/Apply/Approve/Reject/Cancel/Retry/Undo）均可仅用键盘完成。
2. **焦点可见**：焦点态明显、不会被 sticky footer/overlay 遮挡；滚动容器内焦点移动正常。
3. **对话框/抽屉**：
   - 打开后焦点落在首个可交互元素；
   - `Esc` 关闭后焦点回到触发按钮；
   - Tab 不会逃逸到页面背后（若使用原生 `<dialog>`，确认 `aria-*`/focus trap 行为符合预期）。
4. **表格批量操作**：
   - 全选 checkbox 有 `aria-label`；
   - bulk remove 有二次确认；
   - Undo 可通过键盘触发，且触发后状态变化清晰。
5. **复制/提示类控件**：CopyButton 可聚焦、可触发且有可感知反馈（toast/文本变化）。

## axe smoke 清单（建议用浏览器 axe 插件）
对上述每个页面执行一次扫描，记录：
- Critical / Serious / Moderate / Minor 数量
- 关键问题摘要（如对话框缺少 label、按钮无可访问名称、颜色对比不足等）
- 修复建议（若可快速修复，直接在对应 PR 中补齐）

### 自动化 smoke（可选）
- 前置：启动 e2e dev（`make e2e dev`），并确保 `ENABLE_TEST_ENDPOINTS=true`
- 执行：`cd e2e && npx playwright test tests/a11y/authz-ui.spec.ts --reporter=list`
- 产物：Playwright report + `axe-*.json` attachments（用于复盘具体 violations）

## 巡检记录（填写）
- 日期：
- 版本（git rev）：
- 浏览器/系统：
- 覆盖页面：
- 结果摘要：
- 阻塞级别问题列表（若有）：

### 记录（待浏览器复验）
- 日期：2025-12-14
- 版本（git rev）：待填写
- 变更摘要：补齐 `<dialog>` 的 `aria-modal/aria-labelledby`、关闭按钮的 `aria-label`、关闭后焦点回退到触发元素；Undo 提示增加 `role="status"`/`aria-live`（需在浏览器跑 axe + 键盘清单确认无回归）。
