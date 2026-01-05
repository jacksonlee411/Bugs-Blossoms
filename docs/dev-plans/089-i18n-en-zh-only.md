# DEV-PLAN-089：V4 多语言（仅 en/zh：UI 选择 + Accept-Language + 用户语言）

**状态**: 草拟中（2026-01-05 07:48 UTC）

## 1. 背景与上下文 (Context)

`DEV-PLAN-077`～`DEV-PLAN-086` 明确了 HRMS v4 的 Greenfield 路线：DB=Projection Kernel（权威）、Go=Command Facade（编排）、One Door Policy（唯一写入口），以及 AHA UI Shell（Astro + HTMX + Alpine）。

为保证 v4 的全局 UX 一致性与可维护性，需要将“界面语言（UI language）”作为系统级能力收敛并冻结契约：
- 系统仅暴露并支持 `en` / `zh` 两种语言，不扩展更多语言。
- 多语言只覆盖 **界面文案与交互提示**（导航、按钮、表单标签、错误提示等）。
- 多语言 **不涉及业务数据**：例如创建部门/组织单元时，只维护单一 `name` 文本；不支持“部门名称的多语言版本”之类的数据结构。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标
- [ ] **语言白名单**：系统对外仅支持 `en`/`zh`；任何输入（用户资料 / `Accept-Language` / UI 选择）最终都必须落在该集合内。
- [ ] **统一 locale 解析**：HTTP/HTMX/WS 等入口复用同一套“whitelist match + fallback”策略（避免因入口不同导致语言漂移）。
- [ ] **UI 可选语言**：所有语言选择控件仅展示 `en`/`zh`（对齐 `DEV-PLAN-086`：Topbar 提供快速切换入口）。
- [ ] **用户资料语言**：用户资料持久化字段只允许 `en`/`zh`；UI 切换后写回用户资料并立即生效。
- [ ] **未登录可切换**：未登录页面也可切换语言；通过 cookie 记忆偏好（仍只允许 `en`/`zh`）。
- [ ] **语言写入口唯一**：语言切换与写回逻辑收敛到单一写入口（见 `POST /ui/language`），避免在多个 controller/service 中各自实现导致漂移。
- [ ] **浏览器语言支持**：当用户未设置语言或未登录时，支持通过 `Accept-Language` 自动匹配到 `en` 或 `zh`。
- [ ] **门禁可验证**：翻译键一致性与使用校验门禁通过（入口以 `Makefile` 的 `make check tr` 为准）。

### 2.2 非目标（明确不做）
- 不扩展更多语言；不提供“可插拔语言包/动态语言列表/按租户启用语言”等设计空间。
- 不对业务数据做 i18n（不引入 MultiLang 字段、翻译表、`name_i18n` 之类的 API/Schema）。
- 不在本计划内保证翻译内容质量（语义/措辞/排版），仅保证“行为确定 + 不崩溃 + 门禁可通过”。

## 2.3 工具链与门禁（SSOT 引用）
> 目的：避免在 dev-plan 里复制脚本细节导致 drift；本文只声明“会命中哪些触发器”，命令以 SSOT 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`（`make check tr` / `make check doc` 等）
- CI 门禁：`.github/workflows/quality-gates.yml`

本计划实施时通常会命中：
- [ ] Go 代码（按 `AGENTS.md`）
- [ ] 多语言资源（`modules/**/presentation/locales/**` → `make check tr`）
- [ ] `.templ` / UI 资源（按 `AGENTS.md`）
- [X] 文档（本计划 → `make check doc`）

## 3. 架构与关键决策 (Architecture & Decisions)

### 3.1 语言集合 SSOT（冻结为 en/zh）
**选定**：运行期允许语言集合固定为 `["en","zh"]`，并在以下层级形成一致约束：
- 应用层：Application 默认支持语言固定为 `en/zh`（不允许“空值=全量语言”的回退行为）。
- 用户层：用户资料 `ui_language` 仅允许 `en`/`zh`。
- 资源层：仅维护 `en`/`zh` locale 文件；新增文案必须同步补齐两种语言。

### 3.2 locale 解析顺序（HTTP）
**选定**：locale 的输入来源按优先级为：
1. 用户资料语言（已登录且可取到 user）
2. 语言偏好 cookie（默认 cookie key：`ui_language`；仅 `en|zh`）
3. `Accept-Language` header（未登录且无 cookie，或用户语言不可用时）
4. 默认回退：`en`（对齐 `DEV-PLAN-042` 的回退口径：`docs/dev-plans/042-remove-ru-uz-locales.md`）

**输出规则（强制）**：
- 始终对候选语言执行 whitelist matcher：`Match([en, zh], candidates...)`，保证最终 locale 只能是 `en` 或 `zh`。
- 任一来源出现“不可解析/不在白名单/空值”，都不得导致报错或落入非 `en/zh`；必须进入回退路径并最终落到 `en` 或 `zh`。

### 3.3 UI 语言切换（V4 Shell）
对齐 `DEV-PLAN-086` 的 Shell 约束：语言是“动态上下文”，不得在 Astro 壳里固化。

**选定**：在 Topbar 提供快速切换入口，并将“切换语言”收敛为一个小而明确的写入口：
- 仅允许写入 `en` 或 `zh`
- 写入成功后触发整页刷新（避免局部 swap 导致页面出现两种语言混杂）

### 3.4 JS locale（仅 en/zh）
**选定**：页面输出应满足：
- `<html lang="en|zh">` 与服务端最终 locale 一致；
- JS/前端时间格式化仅使用 `en-US` / `zh-CN`（或等价映射），不引入/不暴露更多 locale 选项。

### 3.5 明确不做业务数据多语言
**选定**：所有业务领域对象的“名称/描述”等字段均为单一文本（`text/varchar`），不引入多语言结构。

示例（仅作约束说明，不绑定具体 API 路径）：
- OrgUnit（部门/组织单元）：`name` 为单一文本；不接受 `name_i18n` / `name_translations`。
- Job Catalog（职类/职级/职位模板）：`name` 为单一文本；不维护多语言名称。
- Position/Assignment：同理。

说明：
- 用户录入的内容是什么语言，就以该内容为准；系统不试图“翻译业务数据”。
- 若未来确有“多语言业务数据”的需求，必须另立 dev-plan（并明确数据模型、写入口与回填策略），不得在本计划中顺手扩展。

### 3.6 翻译资源与 key 组织（对齐门禁）
**选定**：翻译资源按模块就近维护，路径固定为 `modules/{module}/presentation/locales/{en,zh}.json`（或该模块既有格式），并通过 `make check tr` 校验：
- keys 必须在 `en` 与 `zh` 同步存在；
- 代码/模板引用的 key 必须在允许语言集合中可被 localize（避免运行期 panic）。

**key 命名建议（避免冲突）**：
- v4 新模块按 083 的模块划分做命名空间前缀：
  - `OrgUnit.*` / `JobCatalog.*` / `Staffing.*` / `Person.*`
- UI Shell（086）的公共文案以 `UI.*` 前缀收敛（例如 `UI.Topbar.Language`）。

约束：
- 避免动态拼接 key（会削弱 `check_tr_usage` 的 fail-fast 能力）；如必须动态，需在实现期明确可枚举范围并配套测试。

## 4. 数据模型与约束 (Data Model & Constraints)

### 4.1 用户资料语言（User Profile Language）
**选定**：用户资料保存 UI 语言，且值域固定为 `en|zh`。

最小约束：
- Domain 层：语言为枚举类型（仅 `en/zh`）。
- Persistence 层：字段非空，并建议增加 DB 约束（可选但推荐）：`CHECK (ui_language IN ('en','zh'))`，避免脏数据绕过应用校验。

### 4.2 无业务数据多语言
本计划不引入 MultiLang 字段/表/结构；所有业务数据仍以单字段保存。

## 5. 接口契约 (API / UI Contracts)

### 5.1 locale 的外部输入
- **用户资料语言**：若已登录，优先使用用户资料 `ui_language`（并通过 whitelist matcher 固化到 `en/zh`）。
- **语言偏好 cookie**：若存在 `ui_language=en|zh`，则在未登录/无用户资料场景下优先于 `Accept-Language`。
- **Accept-Language**：若未登录或用户资料不可用，解析 `Accept-Language` 并在 `[en,zh]` 范围内匹配。
- **默认回退**：`en`。

### 5.2 UI 语言切换
**选定**：提供一个专用写入口用于切换语言（服务端校验 + 刷新整页）：
- `POST /ui/language`
  - Request（Form）：`language=en|zh`
  - 行为：
    - 校验输入必须为 `en|zh`
    - 写入语言偏好 cookie（总是）
    - 若已登录：同时写入用户资料 `ui_language`（作为跨设备偏好 SSOT），并与 cookie 保持一致
    - 返回 `204` 并携带 `HX-Refresh: true`（或 303 Redirect 回 `Referer`），确保整页语言一致

> 备注：该端点是“语言 One Door”，避免在多个 controller 里各自实现一套写回逻辑导致漂移。

### 5.3 响应约定
- HTML 页面应输出 `<html lang="en|zh">` 与当前 locale 一致。
- 允许（可选）输出 `Content-Language: en|zh`，用于调试与缓存策略（如未来引入 CDN/缓存）。

## 6. 实施步骤（建议顺序）
1. [ ] 语言白名单与默认回退：确保运行期允许集合固定为 `en/zh`，默认回退为 `en`（与系统行为一致）。
2. [ ] locale 解析收敛：HTTP middleware 与 WS 广播等入口复用 whitelist matcher（避免任何路径落入非 `en/zh`）。
3. [ ] 用户资料语言：在用户资料更新入口中强制校验 `en|zh`；（可选）增加 DB check constraint。
4. [ ] UI Shell 集成（对齐 086）：Topbar 增加语言切换入口，并接入 `POST /ui/language`。
5. [ ] 翻译资源补齐：为 v4 UI Shell 与 v4 模块新增的文案补齐 `en/zh` keys，并通过 `make check tr`。

### 6.1 实现落点 checklist（避免实现期分叉）
- [ ] HTTP：统一 locale 解析函数包含 cookie 分支（用户资料 → cookie → `Accept-Language` → `en`），并在末端做 whitelist match。
- [ ] WS：对用户资料语言做 whitelist match（不得直接信任 DB 值），保证连接上下文 locale 只可能是 `en|zh`。
- [ ] UI：Topbar 与登录页（未登录）共用 `POST /ui/language`；切换后强制整页刷新（禁止局部 swap 混语言）。
- [ ] 翻译门禁：任何新增 UI 文案必须同时补齐 `en/zh`；确保 `make check tr` 可作为 fail-fast。

## 7. 测试与验收标准 (Acceptance Criteria)

### 7.1 门禁
- [ ] `make check doc` 通过（本计划文档新增/更新）。
- [ ] 若涉及 locales 变更：`make check tr` 通过。
- [ ] 若涉及 Go 代码：按 `AGENTS.md` 的 Go 门禁执行并通过。

### 7.2 行为验收（必须覆盖）
- [ ] 用户资料 `ui_language=zh`：页面与导航文案以中文渲染。
- [ ] 用户资料 `ui_language=en`：页面与导航文案以英文渲染。
- [ ] 未登录：通过 UI 切换到 `zh` 后写入 `ui_language=zh` cookie，刷新后页面以中文渲染。
- [ ] 未登录：通过 UI 切换到 `en` 后写入 `ui_language=en` cookie，刷新后页面以英文渲染。
- [ ] 未登录且 `Accept-Language=zh`：页面以中文渲染。
- [ ] 未登录且 `Accept-Language=ru`（或任意非 `en/zh`）：页面可正常渲染且最终回退为 `en`。
- [ ] 存在脏值（必须 fail-closed 到白名单）：用户资料 `ui_language` 或 cookie 不在 `en|zh`（或不可解析）时，页面可正常渲染且最终回退为 `en` 或匹配到 `zh`（不得报错/不得落入第三语言）。
- [ ] 缺少 `Accept-Language` 或解析失败：页面可正常渲染且最终回退为 `en`。
- [ ] 创建/编辑业务对象时不出现任何“多语言字段输入/返回”（例如无 `name_i18n`）。
- [ ] 运行期不出现因缺少翻译 key 导致的 panic（例如 `PageContext.T()` 的 missing key）；应由 `make check tr` 在 CI 前置拦截。

## 8. 交付物
- [ ] `docs/dev-plans/089-i18n-en-zh-only.md`（本文）
- [ ] 实施 PR：语言切换入口 + locale 解析 + v4 UI shell i18n 集成（对齐 077-086）
