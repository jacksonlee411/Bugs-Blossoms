# DEV-PLAN-014D：Logging 模块 Casbin 改造执行计划

**状态**: 规划中（2025-12-06 01:47 UTC） — 承接 014C 的准备就绪结论并拆解可执行 backlog。

## 背景
- DEV-PLAN-014C 已输出 Logging 模块 Casbin 改造的细化方案并标记为准备就绪，但实际开发/测试/灰度尚未落地。
- 014 主计划要求 Logging 与 Core/HRM 同步完成授权改造与启停记录，本计划在 014C 的基础上提供可执行切片与验收节点，避免与 015A/015B 界面联调时反复调整。
- 015A/015B 契约要求暴露 MissingPolicies/SuggestDiff/Unauthorized 占位，本计划确保 Logging 侧的 controller/模板都按该接口返回，便于后续 UI 直接替换。

## 目标
1. 完成 Logging 模块 controller/service/repo/模板/导航的 Casbin enforce，`logging.logs:view` 全链路守卫生效且单测覆盖。
2. 提供与 015A/015B 兼容的 Unauthorized 体验，MissingPolicies/SuggestDiff 与审计日志在无数据/缺权限环境也能稳定返回 403。
3. 产出策略片段/打包、启停记录与回滚预案，满足 014 主计划步骤 3/5 的验收并支撑后续 015B UI 落地。

## 前置依赖
- 014C readiness（`make authz-test`/`make authz-lint`/`go test ./pkg/authz/...`、Logging 表存在校验、baseline 启动成功）需保持有效；如有变更需在步骤 1 重新验证并更新 dev-records。
- 遵循 AGENTS/CLAUDE：Go 1.24.10、DDD 目录约束、模板修改需 `templ generate && make css`，locale 修改需 `make check tr`，策略生成用 `make authz-pack`，禁止使用 sed。
- Logging 以外的冻结模块（billing/crm/finance）不可改动；灰度按 014 主计划当前阶段“直接 enforce”执行，必要时另起 dev-plan 说明。
- 边界与约束：承接 014C 决策澄清与唯一性原则——双列表分查（authentication/action logs）、强制租户过滤、授权判定唯一走 `pkg/authz`、审计字段固定（subject/domain/object/action/tenant/ip/ua/request_id/trace_id/mode）、action_logs 写入可配置且失败降级为结构化日志告警，不新增平行仓储/双写。

## 实施步骤
1. [ ] Readiness 与记录 —— 复跑 `GOCACHE=/tmp/go-cache make authz-test authz-lint && go test ./pkg/authz/...`，确认 Logging schema 与本地 DB 一致、`authentication_logs`/`action_logs` 均存在且 baseline 启动正常，结果写入 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`，并在 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 建立 Logging 区段；预置 `scripts/authz/verify --tenant <id> --object logging.logs --action view` 的租户列表。
2. [ ] Controller/Helper 落地 —— 新建 `modules/logging/presentation/controllers/authz_helpers.go`（复用 ensureAuthz + EnsureViewState），实现 LogsController 的 list/detail/export/HTMX/REST 入口，首行授权 `logging.logs:view`，403 返回 MissingPolicies + HX-Retarget，携带 015A 申请入口参数，并记录结构化审计日志。
3. [ ] Service/Repository/审计 —— 按 DDD 补齐 domain/entities、persistence model/repo（强制 tenant_id），创建 `authorizeLogging` helper 覆盖 list/detail/export；action_logs 写入钩子与 session handler 复用同一仓储，默认关闭、可配置开启，失败降级为结构化日志告警，拒绝时不触达 repo。单测覆盖无用户/无 tenant/无权限/有权限，并验证审计字段完整与保留期/清理接口约束。
4. [ ] Presentation/模板/UI 占位 —— 在 logs 模板/viewmodel/mappers 中移除 `user.Can`，仅用 `pageCtx.CanAuthz("logging.logs", "view")`；新增 Unauthorized/空态占位，暴露 MissingPolicies/SuggestDiff 并传递 `/core/api/authz/requests|debug` 参数（对齐 015A/015B 契约），页面采用双 Tab/双列表（authentication/action logs）并保持强制租户过滤；更新 locales（含 zh/en/ru/uz），运行 `make check tr`。
5. [ ] 导航/Quick Links/注册 —— 修正 `modules/logging/module.go` 名称注册与 service/controller 挂载，新增 `modules/logging/links.go` 导航项与 Quick Link `.RequireAuthz("logging.logs", "view")`，验证 sidebar/spotlight 可见性过滤生效。
6. [ ] Policy/Seed/打包 —— 在 `config/access/policies/logging/` 增补 `logging.logs:view` 片段，更新默认权限/seed/e2e 账号矩阵；M2 运行最小 `make authz-pack` 验证（可不提交产物），M3 提交正式产物并确认 `.rev` 更新。
7. [ ] 测试与生成物 —— 执行 `go test ./modules/logging/...`，如模板/样式改动运行 `templ generate && make css`；必要时跑 `go vet ./...`。确认 `git status --short` 在生成后保持干净。
8. [ ] 灰度/回滚记录 —— 按主计划默认直接 Enforce；如需 Shadow→Enforce，请在执行前补充触发条件与观察窗口。记录 `AUTHZ_ENFORCE` 启停命令、`scripts/authz/verify --tenant <id> --object logging.logs --action view` 差异、回滚步骤到 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`，同步缺租户/缺权限 403 审计结果。
9. [ ] 文档同步 —— README/CONTRIBUTING/AGENTS（若缺）补充 Logging 授权/调试指引、Unauthorized 入口说明，并在 `docs/dev-records` 登记执行命令与观察结论。

## 里程碑
- LOG-D1（映射 014 LOG-M1）：完成步骤 1-3（controller/service/repo 守卫 + 单测），`make authz-test authz-lint` 通过。
- LOG-D2（映射 014 LOG-M2）：完成步骤 4-6（模板/UI/导航/策略/seed），输出 015B 可复用的 Unauthorized/MissingPolicies 数据与接口示例（含 `/core/api/authz/requests|debug` 参数），双 Tab/双列表 UI 就绪，`make check tr`/生成命令后工作区干净。
- LOG-D3（映射 014 LOG-M3）：完成步骤 7-8-9（测试汇总、authz-pack、启停/回滚记录、文档同步），留存 015B 接口契约样例与回滚记录，可进入后续 UI 联调。

## 验收标准
- `logging.logs:view` 授权在 controller/service 层均 enforced，拒绝路径附 MissingPolicies 与结构化审计日志；无权时 repo 未被调用，审计字段完整。
- 模板/导航/Quick Links 不再出现 `user.Can` 或 legacy 权限字段，全部读取 `pageCtx.AuthzState()`；双 Tab/双列表按租户过滤；Unauthorized 入口可跳转权限申请并携带 015A/015B 需要的参数与示例。
- `make authz-test authz-lint`、`go test ./modules/logging/...`、`make check tr`（如改 locale）、`templ generate && make css`（如改模板）通过后 `git status --short` 干净；M2 仅验证 policy 打包，M3 提交正式产物。
- `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 与 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md` 各自有 Logging readiness、启停/回滚记录与 shadow→enforce（如执行）条件/结果；必要的 README/CONTRIBUTING/AGENTS 增补已提交。
