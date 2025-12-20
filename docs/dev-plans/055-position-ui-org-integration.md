# DEV-PLAN-055：Position UI（Org UI 集成 + 本地化）（对齐 051 阶段 C-UI）

**状态**: 草拟中（2025-12-20 01:46 UTC）

## 1. 背景
- 050 要求面向业务的可操作界面：列表/详情/时间线、有效期治理操作（Update/Correct/Rescind/转移）、占编与空缺提示、以及基本统计入口。
- 本仓库 UI 约束：交互优先复用 `pkg/htmx` 与 `components/`；`.templ` 生成物需由工具生成并提交，避免 CI 失败。

## 2. 工具链与门禁（SSOT 引用）
- UI 与生成物：见 `AGENTS.md`（`.templ`/Tailwind 触发：`make generate && make css`，且 `git status --short` 需为空）。
- 多语言：如触发 `modules/**/presentation/locales/**/*.json`，需对齐 `AGENTS.md` 的翻译门禁（`make check tr`）。
- 路由/交互约束：见 `docs/dev-plans/018-routing-strategy.md` 与现有 Org UI 计划（`docs/dev-plans/035-org-ui.md`、`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`）。

## 3. 目标（完成标准）
- [ ] 在 Org UI（树/侧栏）中集成 Position 入口：可定位到组织单元的 Position 列表与详情。
- [ ] UI 支持核心闭环操作的最小演示：创建/变更/纠错/撤销/转移/占编（联调 053 API + 054 Authz）。
- [ ] 时间线体验可用：版本列表可展示原因/操作者/变更时间（来源审计），并可按 as-of 视角查看。
- [ ] 本地化与门禁可通过：新增文案具备 locales，生成物齐全且提交。

## 4. 依赖与并行
- 依赖：`DEV-PLAN-052`（IA 与合同边界）；`DEV-PLAN-053`（API）；`DEV-PLAN-054`（Authz）。
- 可并行：在 API 合同冻结后可先做 IA/交互原型；数据/权限联调在 053/054 就绪后收口。

## 5. 实施步骤
1. [ ] IA 与导航集成
   - 在 Org 树/侧栏集成 Position 入口与面包屑（对齐 035/035A 的导航口径）。
   - 明确“默认只展示 Managed Position；System Position 仅用于兼容期”的 UI 策略（对齐 052 决策）。
2. [ ] 列表页（读为先）
   - 过滤维度：组织范围（含下级）、生命周期/填充状态、类型/分类、关键词；支持分页。
   - 列表展示：容量/占用（FTE）、空缺提示（VACANT/部分填充）、状态徽标与 as-of 视角提示。
3. [ ] 详情页与时间线
   - 详情：核心字段、归属组织单元、reports-to、容量/占用、限制摘要（如已落地）。
   - 时间线：版本列表、变更原因/操作者/时间；支持切换 as-of 日期查看。
4. [ ] 操作与联调（写入闭环）
   - Position：创建/更新/纠错/撤销/转移/维护汇报链（按 053 API 合同）。
   - Assignment：占用/释放（FTE），并在 UI 上即时反映填充状态派生与超编阻断错误。
5. [ ] 本地化与生成物
   - 补齐 locales 文案；如涉及 `.templ`/Tailwind 变更，确保生成物提交并通过门禁。
6. [ ] 验收与门禁记录（执行时填写）
   - 按 `AGENTS.md` 触发器矩阵跑对应门禁，并将命令/结果/时间戳登记到收口计划指定的 readiness 记录。

## 6. 交付物
- Org UI 中的 Position 管理页面（列表/详情/时间线/占编提示）与最小写入闭环演示。
- 多语言 locales 与必要生成物（确保 CI 不因生成漂移失败）。

