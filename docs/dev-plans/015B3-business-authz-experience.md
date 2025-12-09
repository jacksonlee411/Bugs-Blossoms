# DEV-PLAN-015B3：业务界面授权体验（HRM、Logging 等）

**状态**: 草拟中（2025-01-18 02:10 UTC）  
**范围**: HRM 组织/员工详情、Logging 搜索/详情页及 Quick Links 的 Unauthorized 组件、PolicyInspector 与授权契约

## 目标
- 提供统一 Unauthorized 组件与申请入口，直接写入 `suggested_diff` 并附带 `base_revision/domain/object/action/subject`。
- 提供 PolicyInspector 抽屉，展示调试链路并可一键生成草稿，含限流/降级提示。
- 确保 403 时模板获得完整 `authz.ViewState/MissingPolicies`，导航/Quick Links 可依据授权过滤并可复制 `request_id`。

## 实施步骤
1. [ ] Unauthorized 组件 —— 实现 `components/authorization/unauthorized.templ`，从 `authz.ViewState` 获取 `missing_policies` 构造 `suggested_diff`（g/p），携带 `subject/object/action/domain`、`base_revision`（渲染层读取 `config/access/policy.csv.rev` 注入，`AUTHZ_INVALID_REQUEST` 且提示 base_revision 过期时自动刷新片段以获取新 rev，刷新后再允许 1 次提交，仍失败则提示刷新页面）；HTMX 调 `POST /core/api/authz/requests`；展示/复制已有 `request_id`（提交成功后更新，失败不覆盖旧值）；失败时用黄条提示原因，提供“重试一次”和“复制 request_id/trace”按钮，重试最多 1 次，失败保留 diff。
2. [ ] PolicyInspector —— 仅 `Authz.Debug` 可见，调用 `GET /core/api/authz/debug?subject=&object=&action=&domain=` 展示命中规则/ABAC/latency/trace；处理 429 时提示“频率达上限，5s/10s 各重试一次后放弃”，403 时提示无权限；提供“一键生成草稿”按钮复用 Unauthorized 请求参数，并在 base_revision 过期时沿用 Unauthorized 的刷新逻辑。
3. [ ] 控制器契约 —— HRM/Logging 403 时注入 `MissingPolicies` 与 `authz.ViewState`（含 domain 映射）；渲染层注入最新 `base_revision`，HRM 页固定传 `domain=hrm`，Logging 页传 `domain=logging`（子域需显式传递）；`subject` 统一从当前会话/tenant 上下文注入至 `authz.ViewState` 并透传给组件，UI 不允许手动修改以避免伪造；Quick Links/Sidebar 根据授权过滤；Unauthorized/PolicyInspector 均可复制/显示当前 `request_id` 与 trace 链接。
4. [ ] 测试 —— 补 e2e/集成测试覆盖“有权限 vs 无权限”“缺 `Authz.Debug`/429 降级”“申请失败重试/复制 request_id”；补充单元/契约测试覆盖 diff 构造（含 domain/subject/base_revision 映射）、Quick Links 过滤、请求失败后不覆盖旧 `request_id`、base_revision 过期刷新路径与重试上限。
5. [ ] 质量门禁/文档/多语言/a11y —— 模板/Tailwind 改动后跑 `templ generate && make css`，文案改动跑 `make check tr`，合入前跑 `make check lint`；更新 README/AGENTS 示例与 locales，记录键盘可达/ARIA/axe 巡检结果。

## 依赖
- 015A 调试/草稿 API 可用；各模块已集成 `authz.Authorize` 并能提供 `authz.ViewState`。

## 交付物
- Unauthorized/PolicyInspector 模板与辅助逻辑。
- HRM/Logging controller/template 改造、对应测试。
- README/AGENTS 与多语言更新，a11y 巡检记录。
- 将迭代进展记录至 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md`（含命令/截图/差异）。

## 验收
- 未授权访问时显示统一组件并可成功发起草稿（含 `base_revision/domain/object/action/subject`，失败可重试/复制）；持有 `Authz.Debug` 可查看并生成申请，429/403 有降级提示；导航/Quick Links 根据授权正确过滤并可复制 `request_id`。
- 模板/文案改动通过 `templ generate && make css`、`make check tr`、`make check lint`，并留存 a11y 巡检记录。
- HRM/Logging 相关改动通过 `go test ./modules/hrm/... ./modules/logging/...` 与 `go vet ./modules/hrm/... ./modules/logging/...`。

## 里程碑
- M1：HRM/Logging 页三列只读视图 + Unauthorized 组件（含申请参数/失败提示）上线。
- M2：PolicyInspector 抽屉、限流降级及一键草稿，Quick Links/Sidebar 授权过滤全量启用。
- M3：跨 HRM/Logging 的 e2e 覆盖、文档/locales/a11y 记录补全并稳定运行两周。
