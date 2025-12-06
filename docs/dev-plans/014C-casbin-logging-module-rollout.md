# DEV-PLAN-014C：Logging 模块 Casbin 改造细化计划

**状态**: 准备就绪（2025-12-06 00:02 UTC） — 已采纳评审意见与行业最佳实践（唯一入口/单一来源、租户隔离、集中审计降级）。

## 背景
- DEV-PLAN-014 要求 Core→HRM→Logging 依次完成 Casbin 改造。Core/HRM 已分别通过 014A/014B 建立 helper、ViewState、导航过滤与 shadow/enforce 流程，Logging 仍处空白。
- Logging 模块当前仅有权限常量、schema stub 与 locale，占位 `module.go` 还误报为 `"crm"`，无 controller/导航/服务层，`Logs.View` 未被任何入口使用，日志页面和 API 默认向所有登录用户暴露，缺少 403 与审计记录。
- Logging 表（`authentication_logs`、`action_logs`）已在迁移中创建，core session handler 在写入 authentication_logs，但缺少面向租户的查询/过滤/审计出口，也没有与 015A/015B 的 MissingPolicies/申请入口对齐。

## 前置依赖
- DEV-PLAN-012/013 基础设施 (`pkg/authz`、`make authz-test`/`authz-lint`、policy pack/export/verify 脚本) 需保持可用；014A 的 `authz.ViewState`、`ensureAuthz`、`authorizeCore`、导航过滤能力可直接复用到 Logging。
- Logging 数据面依赖现有 migration/schema（`modules/logging/infrastructure/persistence/schema/logging-schema.sql` + `migrations/**`），需要确认与数据库一致；request logging/Loki 采集链路由 `pkg/middleware/logging`、`pkg/commands/collect_logs` 提供，可按需复用。
- 015A 提供 `/core/api/authz/requests|debug`、PolicyDraftService 与 bot/CLI；015B（Unauthorized/PolicyInspector UI）尚未落地，需在本计划中暴露兼容数据结构与占位 UI。

## 前置准备清单（进入实施前完成并记录）
- [x] 重新执行并登记 readiness：`GOCACHE=/tmp/go-cache make authz-test`、`make authz-lint`、`go test ./pkg/authz/...` 已于 2025-12-06 00:02 UTC 完成，结果写入 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`。
- [x] 确认 Logging 表可用：使用运行中的 `bugs-blossoms-db-1`（localhost:5438）执行 `SELECT ... FROM information_schema.tables`，确认 `authentication_logs`/`action_logs` 均存在（2025-12-06 00:02 UTC）。
- [x] 验证运行基线：`AUTHZ_ENFORCE=0 DB_HOST=localhost DB_PORT=5438 DB_NAME=iota_erp DB_USER=postgres DB_PASSWORD=postgres REDIS_URL=localhost:6379 timeout 45s go run cmd/server/main.go` 成功监听 http://localhost，45s 超时后主动退出（用于验证缺省 Loki/日志配置不阻塞启动）。
- [x] 验证 policy 打包链路：`GOCACHE=/tmp/go-cache make authz-lint`（含 `make authz-pack`）在当前代码上生成聚合策略并通过 fixture parity（2025-12-06 00:02 UTC）。
- [x] 将本文档状态更新为“准备就绪/已批准”，附 readiness 命令时间戳（2025-12-06 00:02 UTC）。

## 范围
- `modules/logging/**` 的 DDD 结构（domain/entities & aggregates、infrastructure/persistence、services、presentation/controllers/templates/locales）、module 注册与导航/Quick Links。
- logging 相关的公共层适配：`pkg/middleware/sidebar`/`spotlight` authz 可见性、`pkg/commands/collect_logs`/`pkg/middleware/logging` 的审计钩子（仅限与授权相关的附加记录，不做全局重构）。
- `config/access/policies/logging/**` 片段、`config/access/policy.csv(.rev)` 及默认权限种子/fixtures。
- dev-records/README/CONTRIBUTING/AGENTS 中的 Logging 授权与调试说明（与 014/015A/015B 保持一致）。

## 目标
1. Logging 所有入口（页面、API、导航、Quick Links）统一通过 `pkg/authz` 判定 `logging.logs:view`（对象 `logging.logs` + 动作 `view`，映射 `Logs.View` 常量），未授权返回 403，附带 MissingPolicies + 审计日志；全链路强制租户隔离（查询/审计均按 `tenant_id` 过滤）。不保留 legacy 权限兜底，灰度/回滚仅依赖 `AUTHZ_ENFORCE` 与策略版本。
2. 建立最小可用的 Logging 展示层：列表/详情/过滤/导出接口遵循 DDD 分层，模板仅消费 `pageCtx.AuthzState()` 暴露的布尔值与 MissingPolicies，与 015B 协议兼容。
3. 服务/仓储层在进入存储前完成授权检查，审计/拒绝链路可验证（单测/e2e）；缺省 Loki/文件采集可按需 mock，确保无数据也能返回 403/空态。
4. 完成 Logging 专属灰度记录：`AUTHZ_ENFORCE` 针对 Logging 的启停命令、差异、回滚剧本写入 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`。

## 唯一性原则（最高约束）
- 同一功能仅保留一种实现路径：授权判定只走 `pkg/authz` helper，模板/UI 仅消费 `authz.ViewState`，不再保留并行/兼容版逻辑。
- 数据存储/查询保持单一来源：authentication_logs 与 action_logs 各司其职，不新增平行表或双写；租户隔离统一在 repo/service 层强制。
- 审计链路单点：未授权/关键操作先写集中结构化日志，按需异步落库，失败降级但不复制多套落点。
- 回滚与灰度只依赖 feature flag 与策略版本控制，避免为“向后兼容”额外维护多套代码分支。

## 提交流程
- 按“工作拆解”小节推进，每完成一个小节的主要实现与验证后立即推送一次 PR（小步提交便于审阅），必要时在 dev-records 补充对应命令/结果。

## 阶段划分（与 014 主计划对齐）
- M1（开发）：补齐 controller/service/repo + 模板/导航鉴权，默认 enforce 开发，可选一次 shadow 对比。
- M2（上线前准备）：完成最小 `make authz-test authz-lint`、`go test ./modules/logging/...`、`make authz-pack` readiness 并记录 dev-records。
- M3（灰度/发布）：按租户/环境切换 `AUTHZ_ENFORCE`，观察 ≥48h（如有），记录 diff/回滚，验证未授权绕过不可行。

## 工作拆解

### 决策澄清（固定选择）
- [x] 展现形态：提供双列表视图（Authentication Logs / Action Logs 两个 Tab），各自独立过滤（时间、用户、IP/UA、method/path），后端分表查询；不做单列表聚合以避免源混淆。
- [ ] Core 复用路径：core session handler 直接依赖 logging repo/service 写 `authentication_logs`，core 内不再维护平行仓储；logging 提供只读 service 给 controller/UI。
- [ ] action_logs 采集开关：默认关闭（配置可开），开启后通过 middleware 钩子写入；无数据环境允许空返回，不报错。
- [ ] 数据保留：默认保留 90 天，logging service 提供清理接口/定时任务钩子（可配置保留期）；纳入文档与验收。
- [ ] 审计日志格式：结构化日志固定字段 `subject, domain, object, action, tenant, ip, user_agent, request_id, trace_id, mode(enforce/shadow)`，logger 前缀 `authz.logging`；落库失败降级为日志告警。
- [ ] E2E/seed：提交 `logging.logs:view` 片段并跑 `authz-pack`；seed 账号至少两类（有/无 Logs.View），Playwright 用同一策略；policy 产物按 M2/M3 策略提交。
- [ ] 监控阈值：403 比例>5% 或 action_logs 写失败率>1% 或缺租户/用户拒绝占比>0.5% 触发告警/人工复核。
- [ ] UI 范围：首版仅 list/detail + 导出（如有），无创建/删除；Quick Link/导航指向 Authentication Logs 默认 Tab。

- [x] 新建 `modules/logging/presentation/controllers/authz_helpers.go`，复用 `authzutil.EnsureViewState` + 014A 的 ensureAuthz 模式，固定 capability `logging.logs:view`（对象 `logging.logs` + 动作 `view`，与 `Logs.View` 常量一致），仅走 `pkg/authz` 判定，不保留 legacy 兜底。
- [x] 建立 `LogsController`（列表/详情/导出/分页 API），入口第一步调用 helper，403 时返回统一消息（HX-Retarget 支持）、MissingPolicies、SuggestDiff；所有查询按上下文租户过滤，缺租户直接返回 403。
- [x] Controller props 注入 `authz.ViewState` 布尔值（例如 `CanViewLogs`）给模板/HTMX partial 使用；支持 query 参数过滤（用户、时间范围、IP、路径）并默认限定当前租户。
- [ ] 为 REST/HTMX/JSON 统一 handler，确保 403/200 序列化一致；记录 unauthorized 事件到审计（见“数据采集/审计”）。

### 2. 服务层、仓储与数据源
- [x] 依据 DDD 规范补全 domain/entities（AuthenticationLog/ActionLog + FindParams）、repository interface/impl（`modules/logging/infrastructure/persistence/*`），避免继续使用 core 占位 repo，必要时抽公共 mapper；所有查询/导出必须强制 `tenant_id` 条件。
- [x] 在 service 层新增 `authorizeLogging(ctx, action)` helper，所有读/导出操作调用 `authz.Authorize(ctx, subject, "logging.logs", "view", attrs...)`；未授权返回 `authz.ErrForbidden`，同时附带 MissingPolicies。
- [x] 数据源策略：`authentication_logs` 用于身份/登录审计，`action_logs` 用于请求/操作审计；仅分表查询，不提供聚合视图，内部始终区分表源，避免混用。
- [ ] Core 现有 session handler 写 `authentication_logs`：优先复用同一仓储（core handler 依赖 logging repo/service 或 logging 层提供只读 service，避免双写/双 schema）。
- [ ] action_logs 写入链路：在 `pkg/middleware/logging` 或单独 handler 中添加可选钩子（启用时将 request 摘要写入 action_logs），保持开关可配置且不影响现有日志输出。
- [ ] 单元测试（表驱动 + testify + mock repo）覆盖：①无用户/租户②无权限③有权限，拒绝时仓储不被调用；验证 attributes 透传（tenant、path、method）。

### 3. Presentation / 模板 / Locales
- [ ] 在 `modules/logging/presentation/templates/pages/logs/` 创建 list/detail/empty templat，全部用 `pageCtx.CanAuthz("logging.logs", "view")` 控制入口，禁止 `user.Can`；空态支持“无数据/无权限”两类。
- [x] 临时 Unauthorized 组件读取 `pageCtx.AuthzState().MissingPolicies` + `SuggestDiff`，提供跳转 `/core/api/authz/requests` 按钮；待 015B 可直接替换。
- [x] 补充 viewmodels/mappers 携带 `CanExport`, `CanInspect` 等布尔值，模板避免再做判定；HTMX partial 复用相同 props。
- [x] 更新 locales `{en,ru,uz,zh}.json`：新增 Logging 页面标题、筛选项、403 文案、申请权限按钮；完成后运行 `make check tr`。

### 4. 导航、Quick Links 与模块注册
- [x] 修复 `modules/logging/module.go` 的 `Name()` 返回值为 `"logging"`，在 Register 中挂载 controller/service/router/quick links（参考 core/hrm 模式）；embed schema/locale 保留。
- [x] 新建 `modules/logging/links.go`：为日志列表添加导航项（`AuthzObject: "logging.logs"`, `AuthzAction: "view"`），不再保留 legacy `Permissions`；在 `modules/NavLinks` 合并。
- [x] 在 `modules/logging/module.go` 或等价位置注册 Quick Link（“View Logs”），使用 `.RequireAuthz("logging.logs", "view")`，并在 spotlight/side bar 依赖 `authz.ViewState` 过滤。

### 5. 数据采集与审计对齐
- [ ] 定义 unauthorized 审计策略：当 `authz.Authorize` 返回 forbidden 时，先写集中结构化日志（subject/domain/object/action/tenant/IP/UA/request-id/trace-id），按需异步写 `action_logs`；写表失败降级为日志告警，不阻断请求。
- [ ] 与 session handler 的 authentication_logs 写入对齐字段/租户来源，必要时在 logging service 层增加批量导出/清理接口（保留期、分页上限）并文档化。
- [ ] 若启用 Loki/文件采集，提供可选 pipeline（只读模式、不要求生产可用），并在 README/CONTRIBUTING 说明如何 mock 或关闭。

### 6. 测试、E2E 与记录
- [ ] 单元测试：`go test ./modules/logging/...` 覆盖 controller/service/repo（含 403/成功路径），使用 testify mock tx/tenant 注入；生成模板后执行 `templ generate && make css` 确认工作区干净。
- [ ] E2E/Playwright：在 `e2e/tests/logs/**` 增加“有 Logs.View / 无 Logs.View”场景，校验导航隐藏、直接访问返回 403、MissingPolicies/申请入口显示；在 `pkg/commands/e2e/seed.go` 补充对应账号/策略。
- [ ] dev-records：将 readiness、策略 diff、AUTHZ_ENFORCE 启停命令写入 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 与 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`。

### 7. 灰度与回滚
- [ ] 在 `config/access/authz_flags.yaml`（或等效）声明 Logging 模块的 flag 分段，支持按租户/环境启停；shadow→enforce 切换前后运行 `go run scripts/authz/verify/main.go --tenant <id> --object logging.logs --action view`。
- [ ] 回滚策略：关闭 `AUTHZ_ENFORCE` + revert Logging 授权提交，必要时恢复 policy 产物；写入操作步骤（命令 + 预期日志）到 dev-records。
- [ ] 监控指标：请求 403 计数/延迟、action_logs 写入失败率、缺租户/用户的拒绝日志占比，灰度期间每日复核一次。

### 8. Policy 维护
- [x] 在 `config/access/policies/logging/` 定义 `logging.logs:view` 片段，与 `modules/logging/permissions` ID/名称一致；执行最小 `make authz-pack`（M2 可不提交产物，M3 提交正式产物）。
- [ ] `pkg/defaults` 权限种子与 fixtures 更新，确保 Playwright/e2e/seed 使用同一策略；必要时调整 `quality-gates` 期望；验收需校验模块名/导航/spotlight 入口挂载正确。

### 9. 与 015A/015B 的接口契约
- [ ] Controller/模板暴露 `pageCtx.AuthzState().MissingPolicies`、`SuggestDiff`、subject/domain（用于 PolicyInspector 参数），Forbidden 响应保持稳定字段，方便 015B 直接替换 UI。
- [x] Unauthorized 页/按钮跳转 `/core/api/authz/requests`，并在 props 中附上 object/action/domain 便于预填；若需要新增字段（如 log source），在本文档同步描述并与 015B 对齐。

## 交付物
- Logging 模块完整的 Casbin 接入（controller/service/repo/模板/导航/Quick Links）、修正后的 module 名称与注册。
- `config/access/policies/logging/**` 片段及必要的 policy 产物、authz-pack/verify 差异日志。
- 单元/E2E 测试与 dev-records（readiness、灰度、回滚）记录，README/CONTRIBUTING/AGENTS 中关于 Logging 授权/调试的增补。

## 验收标准
- M1：`go test ./modules/logging/...` 通过；`templ generate && make css`（如修改模板）/`make check tr`（如修改 locale）后 `git status --short` 干净；controller 403/成功路径均有覆盖。
- M2：`make authz-test authz-lint` + 最小 `make authz-pack` 成功；Forbidden 时带 MissingPolicies/申请入口；导航/Quick Links 按权限过滤。
- M3：如执行灰度，记录 shadow→enforce 命令/观察≥48h，未授权用户无法绕过 controller/service guard；回滚剧本可验证。
- 验收补充：core session handler 已切换到 logging 仓储（无平行仓储/双写），数据源严格分表查询无聚合视图。

## 与 014 主计划衔接节点
- **LOG-M1（控制器就绪）**：Logging controller/导航/Quick Links 全部接入 `ensureAuthz`，模板具备 403 占位，满足 014 步骤 3 的入口守卫要求。
- **LOG-M2（服务/模板就绪）**：service/repo 授权、模板/Unauthorized/申请入口齐备，`pageCtx.CanAuthz` 在 Logging 页面可用，可交付给 015B 接口。
- **LOG-M3（灰度完成）**：`AUTHZ_ENFORCE` 对 Logging enforce ≥48h（如执行）无异常，dev-records 记录完备，014 可进入收尾。
