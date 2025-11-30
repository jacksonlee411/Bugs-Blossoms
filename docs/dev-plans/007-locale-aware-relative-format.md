# DEV-PLAN-007：相对时间显示跟随用户语言

**状态**: 规划中（2025-11-30 17:10）

## 背景
网页上所有使用 `x-data="relativeformat"` 或 `dateFns` 的时间组件，当前都在 `modules/core/presentation/assets/js/alpine.js` 中硬编码 `locale = "ru"`。因此即使用户界面、翻译或 `Accept-Language` 为英语/中文，员工列表等位置仍显示俄语（如“1 секунду назад”）。后端已经具备基于用户 UI 语言或 `Accept-Language` 解析 locale 的中间件，但前端未消费该信息。

## 目标
- 在渲染 HTML 时向前端暴露当前 locale（优先用户设置，其次浏览器语言，最后默认英语）。
- Alpine 的 `relativeFormat` 与 `dateFns` 使用动态 locale，而非硬编码 `"ru"`。
- 相对时间组件在切换语言后自动刷新显示；提供快速回归测试（手动或自动）确保员工列表/用户列表受控。

## 风险
- 需要保证 locale 字符串与 `Intl.RelativeTimeFormat`、`Intl.DateTimeFormat` 兼容。
- Javascript 端需要在 HTMX 局部刷新后同样可用（locale 来源不能只依赖初次加载）。
- 可能涉及模板或 layout 层，需注意不要在每个页面重复注入脚本。

## 实施步骤
1. [ ] **Locale 注入机制**  
   - 在 layout 模板或全局 `<body>` 上通过 `data-locale="{{.Locale}}"` 等方式输出当前语言（来自 `intl.WithLocale`）。  
   - 若已经有 `intl.LocaleFromContext` 的 helper，可复用以避免重复解析。

2. [ ] **前端逻辑改造**  
   - 在 `alpine.js` 中暴露一个 `window.__sdkLocale`（由模板写入）或从 `document.documentElement.lang` 读取，作为 `relativeFormat`、`dateFns` 的默认 locale。  
   - 提供 fallback（例如 `navigator.language` 或 `"en"`）以免数据缺失。

3. [ ] **HTMX/动态更新兼容**  
   - 确保 `relativeformat` 组件在 HTMX 局部刷新时不会丢失 locale（读取全局变量/DOM 属性即可）。  
   - 如有必要，监听自定义事件（`htmx:afterSwap`）以重新执行 Alpine 初始化。

4. [ ] **验证与文档**  
   - 在 HRM 员工列表、Core 用户列表、Finance 支付列表中分别创建条目，确认不同语言（中文、英文、俄文）的相对时间显示一致。  
   - 更新 `README` 或 `docs/dev-plans/003` 的“默认账号 + 登录测试”步骤，提醒检查相对时间 locale。  
   - 可选：新增 Cypress/Playwright 断言或截图，确保 `innerText` 不含俄语字符串。

## 里程碑
- M1：前端可读取后端 locale 并成功在 `relativeFormat/dateFns` 中生效。
- M2：关键页面验证通过，并记录 QA 步骤（例如如何在浏览器切换 Accept-Language/用户语言）。

## 交付物
- 更新后的 `modules/core/presentation/assets/js/alpine.js`（动态 locale）。  
- 在布局模板/中间件中传递 locale 的代码与文档。  
- 验证记录（截图或 QA 说明），证明员工/用户列表的“更新时间”文字会随语言变化。
