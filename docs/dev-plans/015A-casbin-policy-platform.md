# DEV-PLAN-015A：Casbin 策略平台（API、数据模型与 Bot 工作流）

**状态**: 进行中（2025-12-03 12:20）

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
1. [X] `make authz-test` / `make authz-lint` / `go test ./pkg/authz/... ./modules/core/...` —— 已在 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 登记（2025-01-15 11:05-11:10）。
2. [X] `make authz-pack` + `go run ./scripts/authz/verify --fixtures ...` —— 同步记录于 dev-records。
3. [X] `go run ./scripts/authz/export -dry-run` —— 以 `ALLOWED_ENV=production_export` 在本地执行，dry-run 成功（69 p / 4 g），阻塞已解除。
4. [ ] Git bot PAT 凭证与轮询方案待确认（完成后补 dev-records）。
5. [X] 数据库迁移链路验证 —— 本地执行 `make db migrate up`，成功生成 migration log，并写入 dev-records。

## 实施步骤（分阶段）

### 阶段 Alpha：Schema & Repository 就绪
1. [X] 迁移：`migrations/changes-1762000001.sql` 创建 `policy_change_requests` 表及索引（含 `bot_locked_at`、`base/applied_revision` 等），已通过 `make db migrate up`。
2. [X] Repository：在 `pkg/authz/persistence` 新增实体与仓储，提供 CRUD/分页/状态更新/锁管理，避免对 Core 模块的耦合。
3. [X] 单元测试：`go test ./pkg/authz/persistence` 通过，覆盖锁竞争、bot metadata、审批流转。
4. [X] 风险缓解：记录回滚命令与 `SKIP_MIGRATE=1` 导出说明（文中描述），并在 dev-records 标记迁移执行结果。

### 阶段 Beta：服务层与 REST API
1. [X] `PolicyDraftService`：定义 `DraftChange`/`PolicyDraft`，实现 diff 校验、幂等提交、审批流转、多租户过滤及审计事件。
2. [X] `base_policy_revision` provider：`make authz-pack` 生成 `policy.csv.rev`，`pkg/authz/version.Provider` 提供读取；README 记录操作方法。
3. [X] REST API：`/core/api/authz/policies|requests|debug` 及 `POST /requests/{id}/approve|reject|cancel|trigger-bot|revert`，附权限矩阵与速率限制。
4. [X] 测试与文档：补 `go test ./modules/core/services -run PolicyDraft` / `controllers -run Authz`，并在 README/AGENTS 提供 curl 示例与错误码说明。
   - 成果：Core 模块注册 `PolicyDraftService` + `AuthzAPIController`，新增 `Authz.*` 权限与多语言翻译，`config/access/policy.csv.rev` 纳入版本控制，`make authz-pack` 自动生成 revision 元数据；服务层/控制器均已有单元测试并归档调用示例。

## 最新进展（2025-12-03）

- 阶段 Beta 的服务层与 REST API 已并入 `feature/dev-plan-015a` 分支，`AuthzAPIController`、`PolicyDraftService` 相关 gofmt 已同步，`Code Quality & Formatting` 作业不再报错。
- 由于新增 “Log in with Google” 按钮，Playwright 登录脚本曾点击错误按钮导致 `/users` 用例停留在登录页，现已改为精准匹配 “Log in” 提交按钮修复 `tests/users/register.spec.ts`。
- `E2E Tests` pipeline 近期的红灯来自 `/__test__/reset` 超时与 `Resources.authorization` / `Permissions.Authz.*` 缺失翻译，现已通过延长 Playwright API/beforeAll 超时、补齐全部 locale 键值并在本地复跑 `pnpm exec playwright test tests/users/register.spec.ts` 验证；GitHub Actions 已重新变绿。

### 阶段 Gamma：Authz.Debug 与观测
1. [X] `pkg/authz` Inspector 输出 rule 链路、ABAC、latency，并封装 `Inspect` 结果。
2. [X] `/core/api/authz/debug` 返回 Inspector 数据、ABAC 属性与 latency，并写入审计日志。
3. [X] 监控：注册 Prometheus 指标 `authz_debug_requests_total/latency_seconds`，日志包含 request id/subject。
4. [X] dev-records：在 DEV-PLAN-012/015 文档中添加一次实际调用记录。
5. [X] 速率限制与红线监控：`/debug` 增加 `20 req/min/IP` 限流与属性过滤。

### 阶段 Delta：Bot/CLI & 自动化闭环
1. [X] `cmd/authzbot`/脚本：以 30 秒轮询 `policy_change_requests` 抢占 `bot_lock`，处理 base revision 校验、diff 应用、`make authz-pack && make authz-test`、PR 创建。
2. [X] 锁管理：`bot_lock` 字段提供 `scripts/authz/bot.sh --force-release <id>` 手动解锁与日志记录，暂不实现自动 TTL。
3. [X] 成功回写 `applied_policy_revision/snapshot`；`/revert` 端点依 snapshot 生成逆向草稿。
4. [X] CLI `scripts/authz/bot.sh` 支持 `run`/`force-release` 两种模式，文档列明环境变量（Git token、repo）。
5. [ ] Dev 验证：至少两次“草稿→bot→PR→状态更新”跑通并写入 dev-records。

#### Git Bot 凭证与轮询方案
1. [X] PAT 凭证链路 —— 创建 `@bb-authz-bot` GitHub 机器人账号，生成只具备 `repo`（contents/pull_requests）权限的 PAT，存入 1Password `DEV-AUTHZ`（命名 `AUTHZ_BOT_GIT_TOKEN`），由 `scripts/authz/bot.sh` 注入 `https://<token>@github.com/acme/Bugs-Blossoms.git`；同一脚本设置 `git config user.name/email`、branch 前缀（`AUTHZ_BOT_GIT_BRANCH_PREFIX` 默认 `authz/bot/`）、远端（`AUTHZ_BOT_GIT_REMOTE` 默认上述 HTTPS），并在推送前统一执行 `make authz-pack authz-test`，失败时写入 `policy_change_requests.error_log` 并释放锁。
2. [X] 轮询执行流 —— `cmd/authzbot` 每 30 秒扫描 `status IN (approved,failed)` 且 `error_log IS NULL` 的记录，通过 `AcquireBotLock` 控制并发，获取 diff、校验 base revision、更新文件、运行 `make authz-pack && make authz-test`，成功则推送并创建 PR，写回 `applied_policy_revision/snapshot/pr_link`；失败记录 `error_log` 并清空 `bot_lock`；提供 `scripts/authz/bot.sh force-release <id>` 执行 `UPDATE policy_change_requests SET bot_lock=NULL, bot_locked_at=NULL WHERE id=$1` 以支持人工干预；相关 SQL 与 env 示例需记录到 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md`。

### 阶段 Epsilon：文档、CI 与运维
1. [X] 文档：README/CONTRIBUTING/AGENTS 新增“策略草稿流程 / bot 操作 / 回滚脚本 / FAQ”章节。
2. [X] dev-records：维护 `request_id/pr_link/status/operator/log摘` 模板，每次 bot 运行都补齐。
3. [ ] CI：更新 `quality-gates`，对 authz 相关改动自动跑 `make authz-test authz-lint`、`go test ./pkg/authz/... ./modules/core/services ./modules/core/presentation/controllers`，若涉及模板/locale 同步跑 `templ generate && make css` / `make check tr`。
4. [ ] 运维：新增“手动 bot / revert”指南与缓存策略，减少 CI 耗时；确保 `git status --short` 在生成命令后干净。

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
