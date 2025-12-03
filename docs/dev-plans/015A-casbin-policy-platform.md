# DEV-PLAN-015A：Casbin 策略平台（API、数据模型与 Bot 工作流）

**状态**: 草拟中（2025-01-15 10:30）

## 背景
- DEV-PLAN-013/014 已在 Core/HRM/Logging 引入 `pkg/authz` 与 Feature Flag，但仍缺少官方策略变更通道；管理员只能手工修改 `config/access/policy.csv`，缺乏审计与回滚。
- DEV-PLAN-015 拟构建策略管理全链路。为解耦模块授权改造（014）与 UI 体验建设（015B），先交付“策略平台”底座（015A），确保 API、数据模型、bot/CLI、docs、测试齐备，供 014/015B 共用。
- 目标是提供稳定的 `policy_change_requests` 表、REST API、Authz.Debug、PolicyDraftService、UI→Git bot，使后续 UI/业务流程在不依赖手工脚本的情况下发起和追踪策略变更。

## 前置条件
- DEV-PLAN-013 输出的 `pkg/authz`、`config/access/{model.conf,policy.csv}`、`scripts/authz/export|verify` 可在本地/CI 执行；`make authz-test authz-lint` 通过。
- Core/HRM/Logging 控制器已具备 `authz.Authorize` 钩子与 `AUTHZ_ENFORCE` Feature Flag（来自 DEV-PLAN-014 的初始工作）。
- 数据库迁移通道（Goose/Atlas）可正常运行，允许新增 `policy_change_requests` 相关 schema。
- Git 托管平台（GitHub/GitLab）已配置 bot token/密钥，允许自动创建 PR。

## 目标
1. 设计并落地 `policy_change_requests` 数据模型、Repository、Service，支撑草稿/审批/审计。
2. 提供 `/core/api/authz/*` REST API（策略列表、草稿 CRUD、调试接口），并通过单元测试与 CLI 示例验证。
3. 实现 `PolicyDraftService` 状态机与 `Authz.Debug` 端点，确保 diff 校验、幂等性、审计日志完整。
4. 构建 UI→Git bot/CLI：监听草稿、生成分支、运行 `make authz-pack authz-test`、创建 PR、写回状态，并提供回滚操作。
5. 更新 README/AGENTS/docs，列出 API 契约、命令示例、SLA 及 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 的记录模板。

## 前置依赖验证
- **命令可用性**：在 `feature/dev-plan-015` 分支运行并记录以下命令的结果（时间、操作者、输出摘要）：
  - `make authz-test`, `make authz-lint`, `go test ./pkg/authz/...`、`go test ./modules/core/...`.
  - `scripts/authz/export`、`scripts/authz/verify`。
  - `make authz-pack`（需确认生成 `config/access/policy.csv` 和 `policy.csv.rev` 所需脚本已经就绪）。
- **环境准备**：
  - 数据库：确保 Goose/Atlas 可以创建共享表（本地/CI 均需记录一次成功执行）。
  - Git 凭证：为 bot/CLI 预置读写凭证（写入 `.env.example` 模板并在 `dev-records` 记录存放方式）。
  - 监听机制：确认数据库支持 LISTEN/NOTIFY 或准备轮询方案。
- **记录方式**：
  - 在 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 新增 “前置依赖验证” 表格，字段含 `日期/命令/环境/结果/备注`。
  - 每次验证更新本章节，注明最新执行者和结果（例如：“2025-01-16 @alice：make authz-test ✅”）。
- 所有前置项均成功并有记录后，方可进入阶段 Alpha；否则需补齐依赖或开出阻塞项。

## 实施步骤（分阶段）

### 阶段 Alpha：Schema & Repository 就绪
- Goose/Atlas migration：在共享目录（如 `migrations/shared`）创建 `policy_change_requests`，字段包含 `id UUID PK`, `status`, `requester_id`, `approver_id`, `tenant_id`, `subject`, `domain`, `action`, `object`, `reason`, `diff JSONB`, `base_policy_revision TEXT`, `applied_policy_revision TEXT`, `applied_policy_snapshot JSONB`, `pr_link`, `bot_job_id`, `bot_lock`, `bot_attempts`, `error_log`, `created_at`, `updated_at`, `reviewed_at`。索引 `(status, updated_at)`、`(tenant_id, status)`、`(bot_lock)`。
- `pkg/authz/persistence`：新增 repository（分页、过滤、锁定、状态更新、审计写入）与 mapper，避免依赖 Core 模块。
- 单元测试：覆盖 CRUD、锁定、审计字段。运行 `go test ./pkg/authz/persistence` 验证。
- 风险缓解：明确迁移脚本的回滚命令；若数据库环境不允许写操作，提供 `SKIP_MIGRATE=1` 导出 schema 步骤。

### 阶段 Beta：服务层与 REST API
- `PolicyDraftService`：
  - 定义 `DraftChange`、`PolicyDraft` 结构；实现 diff 校验、幂等 `SubmitDraft`、审批流转、base_revision 冲突检测、审计事件。
  - 支持多租户过滤，普通用户只能访问自己草稿。
- `base_policy_revision` 生成与同步：
  - `make authz-pack` 在生成 `config/access/policy.csv` 后，同步生成 `config/access/policy.csv.rev`（内容含 Git commit hash、生成时间、文件 hash）。
  - `pkg/authz` 新增 `version.Provider`，负责读取最新 `policy.csv.rev`；`PolicyDraftService` 在提交草稿时调用 provider，将 revision 写入 `base_policy_revision`。
  - Bot 在执行前同样通过 provider（或直接读取 Git HEAD）获取当前 revision，比对 `base_policy_revision`；不一致则返回 `409 Conflict` 并提示用户重新同步。
  - README/AGENTS 记录“如何查看/刷新 revision”，避免多人同时操作时产生误差。
- REST API（`/core/api/authz`）：根据权限矩阵实现 `GET/POST /requests`, `GET /requests/{id}`, `GET /policies`, `POST /requests/{id}/approve|reject|cancel|trigger-bot|revert`, `GET /debug`。
- 鉴权：`Authz.Request`、`Authz.Manage`、`Authz.BotOperator`、`Authz.Debug`；在 controller 层添加中间件日志，记录 subject/object/action/domain。
- README/AGENTS：新增“API 契约 + curl 示例 + 错误码”章节；标明常见失败原因（权限不足、base_revision 冲突、bot_lock 占用等）。
- 测试：`go test ./modules/core/services -run PolicyDraft`、`go test ./modules/core/presentation/controllers -run Authz`。
- 风险缓解：对暴力/高频调用设置速率限制（Nginx/Go middleware），防止滥用。

### 阶段 Gamma：Authz.Debug 与观测
- `pkg/authz` 增加 `Inspector`：输出命中的 rule id、policyType、domain、ABAC 属性、评估耗时。
- `/core/api/authz/debug` 将 Inspector 数据封装成响应；对 `Authz.Debug` 用户开放，并记录审计日志。
- 监控：为 API 增加 Prometheus/Tally 指标（调用数、latency、错误率）；log 中包含 request id、subject 等。
- `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 与 `DEV-PLAN-015-CASBIN-UI.md` 各记录一次 debug 调用示例（命令、输出摘要、结论）。
- 风险缓解：为 debug 接口设置速率限制、红线监控，防止泄露策略信息给非管理员。

### 阶段 Delta：Bot/CLI & 自动化闭环
- `cmd/authzbot`（或 `scripts/authz/bot`）：
  - 监听数据库（轮询或 LISTEN/NOTIFY），以 `SELECT ... FOR UPDATE`/`UPDATE ... WHERE bot_lock IS NULL RETURNING *` 占锁。
  - 在执行前验证 `current_policy_revision == base_policy_revision`，否则标记冲突。
  - 步骤：checkout 分支 → 应用 diff → 运行 `make authz-pack && make authz-test` → 创建 PR → 写入 `pr_link`/`status=merged`。
  - 失败处理：更新 `error_log`、`bot_attempts`，超阈值转 `failed`；`trigger-bot` 仅在 `bot_lock` 释放后可重试。
  - 回写：成功合并后记录 `applied_policy_revision`（PR merge commit）及 `applied_policy_snapshot`（最终 policy 行，用于 revert）。
  - 锁 TTL：`bot_lock` 字段记录占用者与时间戳，watchdog（cron 或 bot 内定时任务）每 5 分钟扫描超过 10 分钟且 `bot_attempts` 未变化的记录，自动清理锁、标记 `failed_timeout` 并写审计日志；管理员可通过 CLI `--force-release` 手动释放。
- CLI：`scripts/authz/bot.sh run --request <id>`，支持 `--force`（忽略锁，仅管理员）与 `--revert`；文档列出环境变量（Git token、repo、工作目录）。
- API：`POST /requests/{id}/trigger-bot`、`/revert` 调用 bot；返回当前锁状态/排队信息。
  - Revert：`/revert` 端点读取 `applied_policy_revision` + `applied_policy_snapshot`，与当前策略做差，生成逆向 diff 并创建新的 `policy_change_requests` 记录，同时引用原 request/pr link；若 snapshot 不存在则拒绝 revert 并提示手工处理。
- 验收：在 dev 环境跑通“草稿→bot→PR→状态更新”至少 2 次；失败日志可追溯。
- 风险缓解：在 bot 期间生成审计记录，避免 PR 未完成时 UI 误判；对 Git 操作设置超时和回滚机制。

### 阶段 Epsilon：文档、CI 与运维
- 文档：README/CONTRIBUTING/AGENTS 更新“策略草稿流程”“bot 操作”“回滚脚本”“常见 FAQ”；附命令示例、SLA、角色职责。
- dev-records：`DEV-PLAN-015-CASBIN-UI.md` 提供记录模板（`request_id/pr_link/status/operator/log摘`），每次变更需填写。
- CI：在 `quality-gates`/Makefile 中添加命令链：
  - `make authz-test authz-lint`
  - `go test ./pkg/authz/... ./modules/core/services ./modules/core/presentation/controllers`
  - 若触达模板/样式，执行 `templ generate && make css`；触达 locales 时运行 `make check tr`
  - 确保 `git status --short` 在生成命令后干净。
- 风险缓解：在 CI 中缓存工具依赖、缩短 bot 脚本执行时间；在 doc 中注明如何在故障时手动运行 bot 或 revert。

## 里程碑
- **M1**：`policy_change_requests` 表、repository、service、`POST/GET /requests` API 可用，具备最小草稿创建/查询能力。
- **M2**：Authz.Debug、审批/取消 API、PolicyDraftService 状态机、`GET /policies` 完成；CLI `curl` 示例可复现。
- **M3**：Bot/CLI 与 `trigger-bot/revert` 接口上线，README/AGENTS/record 文档更新，端到端流程（草稿→PR→合并→状态更新）在 dev 环境成功跑通 2 次以上。

## 交付物
- `policy_change_requests` schema、repository、mappers、服务层与 REST API。
- `pkg/authz` Inspector/Debug 能力及对应控制器。
- `cmd/authzbot`（或脚本）源码、配置模板、日志策略。
- README/AGENTS 更新、`docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 记录模板。
- 质量门槛脚本与示例命令。

## 验收标准
- 在本地运行 `curl -X POST /core/api/authz/requests` 可返回 `request_id`，并通过 `GET /core/api/authz/requests/{id}` 查看状态转换。
- `go test ./modules/core/... -run PolicyDraft`、`go test ./modules/core/presentation/controllers -run AuthzDebug` 全部通过。
- 任意草稿在 5 分钟内可被 bot 处理或 UI 提示可重试；bot 失败时 `error_log` 字段包含明确信息。
- README/AGENTS 包含完整 API/CLI 示例，`git status --short` 在执行 `templ generate && make css`（如需）和 `make authz-pack` 后保持干净。
