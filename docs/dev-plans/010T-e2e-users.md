# DEV-PLAN-010T：HRM 用户注册 E2E 专项

**状态**: 跟踪中（2025-12-02 11:00）

## 背景
- DEV-PLAN-010 将 HRM 仓储迁移至 sqlc 后，CI 重新启用了 E2E Playwright 测试。
- 最新 run（workflow `Quality Gates`, run id 19840542217）中 `tests/users/register.spec.ts` 三个场景全部失败，导致 pipeline 标红。
- 为避免 HRM sqlc 计划被 E2E 阻断，需要单独列出问题、范围与修复路径。

## 失败摘要
| 场景 | 报错 | 说明 |
| --- | --- | --- |
| creates a user and displays changes in users table | `locator.click` 等待 `button[x-ref="trigger"]` 超时 | UI 中 `select[name="RoleIDs"]` 的外层按钮未渲染或被重构 |
| edits a user and displays changes in users table | 同上 | 复用相同组件 |
| newly created user should see tabs in the sidebar | `page.waitForURL` 超时（始终停留在 `/login`） | 登录流程提交后被重定向回 `/login`，可能与角色/权限初始化失败有关 |

## 初步分析
1. **角色下拉结构变化**：HRM 可能改用纯 `<select>` 或不同的 Trigger 元素。需要下载 CI 附件（screenshot/video）确认 DOM；Playwright 可能要改用 `selectOption` 或新的 `data-testid`。
2. **登录未跳转**：`fixtures/auth.ts` 依赖的用户/租户可能在 seed 中不存在，或登录后因为权限校验失败被重定向 `/login`。需在本地复现并查看 server 日志。

## 行动计划
1. **收集证据**：
   - 下载 `test-results/**/video.webm` 与 screenshot，确认 `RoleIDs` 控件的 HTML。
   - 查看 `logs/app.log` 中 `/login` 请求是否返回 500 / 302。
2. **更新测试或 UI**：
   - 若 DOM 变化 → 调整 `register.spec.ts` 中的 locator（改 `await roleSelect.selectOption(...)` 等）。
   - 若 DOM 未变化但加载失败 → 排查模板/Alpine，必要时在组件上加 `data-testid`。
3. **校验登录逻辑**：在本地运行 `make e2e test`，如果依然留在 `/login`，检查 `fixtures/auth.ts` 使用的邮箱/密码与 seed 是否匹配。
4. **复测并记录**：修复完成后 rerun CI，并在本文件追加结论。

## 角色分工
- QA/前端：调整 Playwright 脚本 & 页面标记
- HRM 后端：若登录依赖的权限/角色缺失，负责补种 seed

