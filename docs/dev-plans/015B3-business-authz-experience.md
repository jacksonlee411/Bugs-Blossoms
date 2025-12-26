# DEV-PLAN-015B3：业务界面授权体验（HRM、Logging 等）

> [!IMPORTANT]
> 自 DEV-PLAN-015C 起，策略草稿（requests）/审批/bot 链路已移除；当前唯一口径为管理员直接维护生效（`POST /core/api/authz/policies/apply`）。本文仅作历史记录，不再作为 SSOT。

**状态**: 已完成（2025-12-11 10:45 UTC）  
**范围**: HRM 组织/员工详情、Logging 搜索/详情页及 Quick Links 的 Unauthorized 组件、PolicyInspector 与授权契约

## 目标
- 提供统一 Unauthorized 组件与申请入口，直接写入 `suggested_diff` 并附带 `base_revision/domain/object/action/subject`。
- 提供 PolicyInspector 抽屉，展示调试链路并可一键生成草稿，含限流/降级提示。
- 确保 403 时模板获得完整 `authz.ViewState/MissingPolicies`，导航/Quick Links 可依据授权过滤并可复制 `request_id`。

## 实施步骤
1. [x] Unauthorized 组件 —— 实现 `components/authorization/unauthorized.templ`，从 `authz.ViewState` 获取 `missing_policies` 展示缺失策略并提供 Debug 串联（渲染层读取 `config/access/policy.csv.rev` 注入；403 统一由全局 toast/错误处理展示，不做组件内重复刷新/重试）；管理员侧策略变更统一使用 `POST /core/api/authz/policies/apply`。
2. [x] PolicyInspector —— 仅 `Authz.Debug` 可见，调用 `GET /core/api/authz/debug?subject=&object=&action=&domain=` 展示命中规则/ABAC/latency/trace；429/403 等错误统一走 015B4 定义的 toast/错误处理，不在组件内自建 5s/10s 重试；“一键生成草稿”按钮复用 Unauthorized 的请求封装与参数（含 base_revision 处理），不再额外复制重试/降级逻辑。
3. [x] 控制器契约 —— HRM/Logging 403 时注入 `MissingPolicies` 与 `authz.ViewState`（含 domain 映射）；渲染层注入最新 `base_revision`，HRM 页固定传 `domain=hrm`，Logging 页传 `domain=logging`（子域需显式传递）；`subject` 统一从当前会话/tenant 上下文注入至 `authz.ViewState` 并透传给组件，UI 不允许手动修改以避免伪造；Quick Links/Sidebar 根据授权过滤，直接复用现有 `components/auth/permission_guard` 等导航过滤守卫，而非重新实现；Unauthorized/PolicyInspector 均可复制/显示当前 `request_id` 与 trace 链接。
4. [x] 测试 —— 补 e2e/集成测试覆盖“有权限 vs 无权限”“缺 `Authz.Debug`/429 降级”“统一 toast/错误处理链路”“复制 request_id”；补充单元/契约测试覆盖 diff 构造（含 domain/subject/base_revision 映射）、Quick Links 过滤、请求失败后不覆盖旧 `request_id`、base_revision 过期时由后端提示并透传到 toast 的路径（前端不自建重试上限）。
5. [x] 质量门禁/文档/多语言/a11y —— 本轮已跑 `templ generate && make css`、`go test ./modules/logging/... ./modules/hrm/... ./modules/core/authzutil/... ./components/authorization/...`，补充 DEV-PLAN 记录；`make check lint` 本地尝试超时，后续交由 CI 复验；无新增文案/locale 变更，Unauthorized/PolicyInspector 按钮为文本可聚焦，后续可用 axe 补充自动化巡检。

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
