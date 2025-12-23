# DEV-PLAN-042：移除 ru/uz 多语言，仅保留 en/zh

**状态**: 规划中（2025-12-23 04:22 UTC）

## 1. 背景与上下文 (Context)
当前仓库支持多语言（含 `ru`/`uz`/`en`/`zh`），但项目需求仅保留 `en` 与 `zh`。继续维护 `ru/uz` 会带来：
- 翻译键一致性门禁成本（`make check tr`）与变更阻力
- UI 语言选择项与用户资料语言字段的“无效选项”问题
- Bundle 体积与维护面增大

近期已出现过“缺少通用翻译 key 导致页面 panic”的真实事故：`pageCtx.T("View")` 在 `en` locale 下缺失会直接触发 panic（详见修复提交 `fix(i18n): add core View label`）。该类问题在语言集合越大时越难以持续维护。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 目标 (Goals)
- [ ] 系统仅暴露并支持 `en`、`zh` 两种语言（UI 选择、Accept-Language 解析、用户资料语言等）。
- [ ] 移除 `ru`、`uz` 的 locale 资源与语言枚举；并保证 `make check tr` 可通过。
- [ ] 默认回退策略固定且可验证：任何非 `en/zh` 的输入最终回退到 `en`。
- [ ] CI 门禁通过（Go / tr / doc）。

### 2.2 非目标 (Non-Goals)
- 不对既有 `ru/uz` 用户语言数据做数据库迁移/清洗（如需要，另起数据迁移计划）。
- 不扩展更多语言，也不补全翻译内容质量（仅做语言集合裁剪与行为一致性）。

## 2.3 工具链与门禁（SSOT 引用）
> 目的：避免在 dev-plan 复制脚本细节导致漂移；本文只声明触发器与验收入口。

- 本计划命中触发器：
  - [ ] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] 多语言 JSON（`make check tr`）
  - [ ] 文档新增/整理（`make check doc`）
- 建议新增门禁（本计划内落地）：
  - [ ] **翻译使用校验门禁（Fail Fast）**：若代码/模板引用了任意翻译 key，但在运行期允许语言集合（`en/zh`）里缺失，则门禁失败。
    - 背景：`PageContext.T()` 使用 `MustLocalize`，缺 key 会直接 panic；门禁应在 CI 阶段提前拦截。
    - 形态：新增 CLI 命令（例如 `command check_tr_usage`）并纳入 `make check tr` / CI。
- SSOT：
  - 触发器矩阵：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)

### 3.1 关键决策（评审结论）
1) **语言集合的单一事实源（SSOT）**
- 现状问题：`pkg/intl/intl.go` 维护 `allSupportedLanguages`，但运行期的“允许语言集合”实际由 `application.ApplicationOptions.SupportedLanguages` 决定；而当前大多数 entrypoint 未显式设置该值，导致默认“全量语言”。
- 决策：将“运行期允许语言集合”收敛为 **Application 级别的 whitelist**，默认值固定为 `["en","zh"]`（不再依赖 `pkg/intl` 的“全量默认”）。

2) **回退策略**
- 决策：默认回退到 `en`（当用户资料/请求头/任何输入为非 `en/zh` 时）。

3) **用户资料语言的处理方式**
- 现状问题：`pkg/middleware/i18n.go` 当前优先使用 `user.UILanguage()`，且不校验是否在支持集合内；如果用户资料为 `ru/uz`，即使系统已裁剪 ru/uz locales，也会创建 `ru/uz` localizer，最终出现缺失 key 或直接报错。
- 决策：**中间件层必须强制将用户语言也纳入 whitelist match**；若不在支持集合内，回退到 `en`（或匹配 Accept-Language，再回退）。

### 3.2 替代方案（未选）
- 仅删除 `ru/uz` JSON 文件，不改中间件：风险高（用户资料为 `ru/uz` 时仍会创建 `ru/uz` localizer）。
- 仅在 UI 层隐藏 `ru/uz`：不够（API/Accept-Language/已有用户语言仍可能触发 ru/uz）。

## 4. 数据模型与约束 (Data Model & Constraints)
本计划不引入新表/字段。涉及字段仅为既有的用户 UI 语言字段（例如 `user.UILanguage()` 的取值）。

约束（运行期约束，不要求 DB 约束）：
- 允许语言集合固定为 `en`/`zh`。
- 若用户资料语言为 `ru/uz`：运行期渲染强制回退到 `en`；不强制写回 DB（除非后续单独做数据清洗）。

## 5. 接口契约 (API Contracts)

### 5.1 语言选择的外部契约
- **UI 可选语言**：仅显示 `en` / `zh`。
- **Accept-Language**：
  - 输入：任意 RFC 语言标签（例如 `ru-RU,ru;q=0.9,en;q=0.8`）
  - 处理：仅在支持集合 `[en, zh]` 内进行 matcher；否则回退 `en`。
- **用户资料语言**：
  - 输入：用户资料中已有的语言值（可能为 `ru/uz`）
  - 处理：同样按支持集合 matcher；不在集合内则回退 `en`。

### 5.2 兼容性说明
- 之前可选择 `ru/uz` 的用户，在升级后将默认以 `en` 显示（除非显式改成 `zh`）。
- 该行为变化属于“可见行为变更”，需要在 release note / PR 描述中声明。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)

### 6.1 统一的 locale 解析算法（建议实现）
输入来源按优先级：
1. user profile language（如果存在）
2. `Accept-Language` header（如果存在且可解析）
3. default（`en`）

输出规则：
- 始终通过 `language.NewMatcher([en, zh]).Match(...)` 输出结果（保证不落入 ru/uz）。

伪代码：
```
supported = [en, zh]
tags = []
if userLang exists: tags = append(tags, userLang)
if acceptLanguage parse ok: tags = append(tags, parsed...)
if len(tags)==0: return en
return matcher.Match(tags...).Tag
```

## 7. 安全与鉴权 (Security & Authz)
不涉及鉴权策略变更。注意事项：
- 不允许通过 query/header 注入非 `en/zh` 导致 panic（必须通过 matcher 限制）。

## 8. 依赖与里程碑 (Dependencies & Milestones)

### 8.1 依赖
- 无外部依赖。
- 需要确认是否存在 `ru/uz` 的强依赖（例如前端固定写死 `ru` 文案 key、或外部集成依赖语言码）。如存在，需在实施前列出并达成替代方案。

### 8.2 里程碑（实施拆分）
1. [ ] 代码层：将 application 的 `SupportedLanguages` 默认固定为 `en/zh`（覆盖 server / superadmin / CLI common 等 entrypoints）
2. [ ] 代码层：调整 `pkg/middleware/i18n.go`，确保 user language 也走 whitelist matcher
3. [ ] 资源层：删除所有 `modules/**/presentation/locales/ru.json` 与 `modules/**/presentation/locales/uz.json`
4. [ ] UI 层：语言选择组件与账号设置仅展示 `en/zh`；保存时拒绝/回退 `ru/uz`
5. [ ] 门禁与回归：
   - [ ] `make check tr`（翻译键一致性）
   - [ ] `check_tr_usage`（本计划新增：引用 key 不可缺失）
   - [ ] `make check doc` + `make check lint` + `make test`

## 9. 测试与验收标准 (Acceptance Criteria)

### 9.1 门禁
- [ ] `make check tr` 通过（仅对 `en/zh` 做键一致性校验）
- [ ] `check_tr_usage` 通过（仅对 `en/zh` 校验：被引用的 key 必须存在）
- [ ] `make check doc` 通过
- [ ] `make check lint` 通过
- [ ] `make test` 通过

### 9.2 行为验收（必须覆盖）
- [ ] 用户资料 `UILanguage=ru` 时页面可正常渲染（最终 locale= `en`）
- [ ] `Accept-Language=ru` 时页面可正常渲染（最终 locale= `en`）
- [ ] `Accept-Language=zh` 时页面可正常渲染（最终 locale= `zh`）
- [ ] UI 语言选择不再出现 `ru/uz`
- [ ] 运行期无 `message "<key>" not found` 的 panic（可通过访问关键页面 + 观察日志验证）

### 9.3 check_tr_usage 约束（建议实现细节）
- **输入**：扫描仓库中 `.templ` 与 `.go` 的“翻译调用点”，提取 string literal 形式的 message id（例如 `pageCtx.T("View")`、`intl.MustT(ctx,"Login.Errors.PasswordInvalid")`）。
- **校验**：对运行期允许语言集合（目标为 `en/zh`）逐个 `Localize`，若任意语言缺失则失败，并输出：`missing_key + locale + source_file:line`。
- **限制**：对动态 key（例如 `fmt.Sprintf("X.%s", v)`）可先跳过或要求迁移为常量 key；实现时需在文档与输出中明确策略，避免误报/漏报。

## 10. 运维与回滚 (Ops & Rollback)

### 10.1 运维注意事项
- 该变更会影响“已有用户的语言展示”，建议在上线说明中提示：`ru/uz` 将回退为 `en`，可在账号设置中切换为 `zh`。

### 10.2 回滚
- 代码回滚：对该计划的 PR 使用 `git revert` 回滚提交。
- 资源回滚：恢复 `ru/uz` locales 文件 + 将支持语言集合恢复为包含 `ru/uz`。

## 交付物
- [ ] `docs/dev-plans/042-remove-ru-uz-locales.md`（本文）
- [ ] 实施 PR（代码 + 资源删除）
- [ ] （如需要）Readiness：`docs/dev-records/DEV-PLAN-042-READINESS.md`
