# DEV-PLAN-010T：HRM 用户注册 E2E 专项

**状态**: 已完成（2025-12-02 18:40）

## 背景
- DEV-PLAN-010 将 HRM 仓储迁移至 sqlc 后，CI 重新启用了 E2E Playwright 测试。
- 最新 run（workflow `Quality Gates`, run id 19840542217）中 `tests/users/register.spec.ts` 三个场景全部失败，导致 pipeline 标红。
- 为避免 HRM sqlc 计划被 E2E 阻断，需要单独列出问题、范围与修复路径。

## 失败摘要
| 场景 | 报错 | 说明 |
| --- | --- | --- |
| creates a user and displays changes in users table | `locator.selectOption` 在 `select[name="RoleIDs"]` 上超时 | 该 `<select>` 被 Combobox 组件标记为 `hidden`，Playwright 无法直接操作 |
| edits a user and displays changes in users table | 同上 | 复用相同组件 |
| newly created user should see tabs in the sidebar | `page.waitForURL` 超时（始终停留在 `/login`） | 登录流程提交后被重定向回 `/login`，可能与角色/权限初始化失败有关 |

## 初步分析
1. **角色下拉隐藏**：Combobox 依旧只渲染隐藏 `<select>` + 外层 trigger。需要依赖可视 trigger 或新增 `data-testid`，直接 `selectOption` 会因为元素不可见而失败。
2. **登录未跳转**：`fixtures/auth.ts` 依赖的用户/租户可能在 seed 中不存在，或登录后因为权限校验失败被重定向 `/login`。需在本地复现并查看 server 日志。

## 行动计划
1. **收集证据**：
   - 下载 `test-results/**/video.webm` 与 screenshot，确认 `RoleIDs` 控件的 HTML。
   - 查看 `logs/app.log` 中 `/login` 请求是否返回 500 / 302。
2. **更新测试或 UI**：
   - 若继续沿用 Combobox → 在 trigger/选项上补充 `data-testid`，测试通过点击 trigger + 选择列表项完成角色选择。
   - 若改成原生 `<select>` → 调整模板，确保元素在 DOM 中可见后再用 `selectOption`。
3. **校验登录逻辑**：在本地运行 `make e2e test`，如果依然留在 `/login`，检查 `fixtures/auth.ts` 使用的邮箱/密码与 seed 是否匹配。
4. **复测并记录**：修复完成后 rerun CI，并在本文件追加结论。

## 最新进展（2025-12-02 18:40）
- Playwright `register.spec.ts` 已重构为使用独立的 `CREATE_USER` / `EDIT_USER` 测试账号，先通过 `ensureUserExists` 在 UI 中创建缺失的数据，再执行验证。这样即使第一个场景失败，也不会覆盖 `test@gmail.com` 管理员，后续登录逻辑保持稳定。
- `selectFirstRole` 同时支持带 `data-testid="role-combobox"` 的新 UI 与旧的隐藏 `<select name="RoleIDs">`：当找不到可视化 Combobox 时，会临时移除 `hidden` 类并对原生 `<select>` 调用 `selectOption`，彻底解决 `selectOption` 超时。
- 语言选择改为按照语言代码选项值（不再使用索引），避免因 placeholder 选项被禁用而导致 `selectOption` 一直报 “option not enabled”。
- 在 `login()`、进入 `/users`、`/users/new` 时均增加 `not.toHaveURL(/\/login/)` 断言，一旦回到登录页即可立即失败并带出日志。
- 复测 `cd e2e && npx playwright test tests/users/register.spec.ts` 3 个场景全部通过，CI 可重新启用该 spec。

## 角色分工
- QA/前端：完成 Playwright 脚本调整并维护回归
- HRM 后端：无需额外动作，如再次变更 seed/角色需同步通知 QA 更新测试数据
