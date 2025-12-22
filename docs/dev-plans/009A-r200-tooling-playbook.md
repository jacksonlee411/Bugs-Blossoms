# DEV-PLAN-009A：R200 工具链落地现状与复用指引

**状态**: 已完成（2025-12-17 14:36 UTC）

> 本文是 `DEV-PLAN-009` 的“落地现状复盘 + 复用手册”：把 `DEV-PLAN-010~019`（及其子计划）中已引入/正在引入的工具链抽象为**可复用的标准路径**，用于后续新功能/新模块的建设，避免重复造轮子与架构漂移。
>
> 说明：本文会随仓库演进持续更新；其中 Outbox、多租户（Kratos/SSO/RLS 扩面）属于“能力在推进中”的工具链，复用时以对应 dev-plan/门禁为准。

## 1. 定位与边界

- **目标读者**：要新建模块/新功能、或要引入/扩展基础设施（DB/Authz/Outbox/Routing/Tenant）的研发同学。
- **单一事实源（SSOT）原则**：
  - 行为/命令以 `Makefile`、`.github/workflows/quality-gates.yml` 为准（CI 与本地对齐）。
  - 设计与契约以对应 `docs/dev-plans/*` 为准（本文只做索引与复用指引，避免复制细节导致 drift）。
- **不覆盖范围**：UI 细节、具体页面 IA、业务逻辑设计；这些以各自模块 dev-plan 为准。

## 2. 工具链总览（当前仓库已落地/在推进）

| 领域 | 目的 | 入口与落点（SSOT/资产） | 状态 |
| --- | --- | --- | --- |
| 数据访问（sqlc） | SQL-first、编译期类型安全、避免 ORM 运行期开销 | `docs/dev-plans/010-sqlc-baseline.md`、`sqlc.yaml`、`modules/person/infrastructure/sqlc/**`、`scripts/db/export_person_schema.sh`、`make sqlc-generate` | ✅ Person 已落地 |
| Schema/迁移（Atlas + Goose） | schema drift 可见、迁移链路可 lint/plan、Person 独立迁移闭环 | `docs/dev-plans/011A-atlas-goose-baseline-gapfix.md`、`atlas.hcl`、`migrations/person/**`、`scripts/db/run_goose.sh`、`make db plan/lint`、`PERSON_MIGRATIONS=1 make db migrate ...` | ✅ Person 已落地 |
| 授权（Casbin + 策略平台） | 统一 RBAC/ABAC、策略可审计/可回滚、UI→草稿→Bot→PR 闭环 | `docs/dev-plans/013-015*.md`、`pkg/authz/**`、`config/access/**`、`scripts/authz/**`、`cmd/authzbot/**`、`make authz-test/authz-lint/authz-pack` | ✅ 基础设施/首批模块已落地，UI 体验持续完善 |
| 可靠事件（Transactional Outbox） | 业务写入与事件入队同事务、可重试/可观测、避免 ad-hoc 异步 | `docs/dev-plans/017-transactional-outbox.md`、`pkg/outbox/**`、`docs/runbooks/transactional-outbox.md` | 🚧 M1 基础设施已落地 |
| 后台作业队列（Asynq） | 报表/导入/通知类长耗时任务的可靠执行与重试 | `docs/dev-plans/009-r200-tooling-alignment.md`（路线图） | ⏸️ 尚未落地（优先复用 Outbox；如需队列化再立项引入） |
| 路由治理（Routing Strategy + Gates） | UI/HTMX/API/Webhooks/Ops 命名空间与错误契约统一、门禁可阻断漂移 | `docs/dev-plans/018-routing-strategy.md`、`config/routing/allowlist.yaml`、`make check routing` | ✅ 已落地并纳入门禁 |
| 多租户工具链（RLS/Kratos/SSO/Tenant Console） | “认人-圈地-管事”纵深防御：Identity/RLS/Authz | `docs/dev-plans/019*.md`（含子计划）、`make db rls-role`、superadmin Tenant Console | 🚧 Console 已落地，RLS/Kratos/SSO 在推进 |

> 变更触发器与本地必跑命令矩阵以 `AGENTS.md` 为准；本文只强调“新增模块/功能应复用哪个工具链”。

## 2.1 门禁速查（最小版）

> 详细触发器矩阵以 `AGENTS.md` 为准；以下仅作为“新功能/新模块”改动时的最低自检入口。

- 任意 Go 代码：`go fmt ./... && go vet ./... && make check lint && make test`
- `.templ` / Tailwind：`make generate && make css`（生成物必须提交）
- 多语言 JSON：`make check tr`
- Authz：`make authz-test && make authz-lint`
- 路由/allowlist：`make check routing`
- Person sqlc：`scripts/db/export_person_schema.sh && make sqlc-generate && git status --short`
- Person Atlas/Goose：`make db plan && make db lint && PERSON_MIGRATIONS=1 make db migrate up`

## 3. 复用优先级：新增模块/功能怎么选（避免自建）

### 3.1 数据访问：优先 sqlc（SQL-first）

- **何时用**：新增/重构“复杂 SQL 查询、报表查询、写操作命令（事务内）”时。
- **复用路径**：
  - 参考 `DEV-PLAN-010` 的目录与生成策略：`modules/<module>/infrastructure/sqlc/<aggregate>/...`
  - 生成入口统一走 `Makefile`：`make sqlc-generate`（或 `make generate`）。
- **注意**：
  - `sqlc.yaml` 当前以 Person 为基线，扩展到新模块前应先对齐 CI 过滤器与代码审查口径（避免“生成物忘提交”与“冻结模块被误触发”）。
  - 修改 Person SQL/Schema 时严格按 `docs/runbooks/person-sqlc.md` 流程执行。

### 3.2 Schema/迁移：遵循“模块迁移链路”而非散落脚本

- **现状**：
  - 全仓库通用迁移入口：`make db migrate up|down|redo|status`（默认链路）。
  - Person 专用链路（Goose）：`PERSON_MIGRATIONS=1 make db migrate up|down|redo|status`（见 `DEV-PLAN-061`）。
- **Person/采用 Atlas+Goose 的模块**：
  - schema SSOT：`atlas.hcl`（`src` 指向 Person schema SQL 组合）。
  - drift/门禁入口：`make db plan`、`make db lint`（并在 CI 通过 `person-atlas` 过滤器触发）。
  - 若发现历史文档仍引用 `schema.hcl`：以 `DEV-PLAN-011A` 与仓库实际资产（`atlas.hcl`/`modules/person/infrastructure/atlas/core_deps.sql`/`modules/person/infrastructure/persistence/schema/person-schema.sql`）为准。
- **新增模块要不要“再搞一套”Atlas+Goose？**
  - 不建议“直接复制 HRM 方案”并在 Makefile/CI 里加第二套门禁；应先在 `docs/dev-plans/` 通过新计划明确并落地至少以下要素：
    - Makefile 入口（本地可复现）
    - CI changed-files 过滤器与门禁步骤（避免误触发/漏触发）
    - 受控目录（schema source、migrations 目录、执行脚本）
    - 回滚与既有环境接入策略（baseline/bootstrap）
    - 对应 runbook（便于协作与排障）

### 3.3 Authz：统一走 `pkg/authz` + `config/access` + Bot 工作流

- **新增业务能力时的标准动作**：
  1. **控制器/服务层鉴权**：调用 `pkg/authz`（不要在模板里直接做 ad-hoc 判定）。
  2. **策略与权限管理**：
     - 策略碎片：`config/access/policies/**`
     - 聚合产物：`config/access/policy.csv` + `config/access/policy.csv.rev`（只能通过 `make authz-pack` 生成，禁止手改）
  3. **门禁**：改动 Authz 相关内容必须跑 `make authz-test && make authz-lint`（CI 的 `authz` 过滤器会强制）。
  4. **变更通道**：需要可审计的策略变更时，优先走草稿 API/Bot（见 `DEV-PLAN-015A` 与 `docs/runbooks/authz-policy-draft-api.md`、`docs/runbooks/AUTHZ-BOT.md`）。

### 3.4 事件发布：跨模块/异步一致性优先 outbox

- **适用场景**：任何“业务写入后需要异步通知/下游处理”的场景（尤其跨模块）。
- **标准做法**：
  - 业务写入 + outbox enqueue 同一事务（`DEV-PLAN-017` 与 Org 相关计划明确要求）。
  - relay/cleaner 由 `pkg/outbox` 提供统一实现；运维/排障按 `docs/runbooks/transactional-outbox.md`。
- **反模式**：HTTP 请求中 `go func`、或各模块各自实现“队列表 + 轮询器”。

### 3.5 路由：命名空间与错误契约必须遵循 018 + 路由门禁

- **新增路由前先做分类**：UI / 内部 API / 对外 API / Webhooks / Ops / Dev-only / Test（`DEV-PLAN-018`）。
- **门禁**：路由相关变更（尤其 `/api/*`）必须跑 `make check routing`；如需例外必须登记 allowlist：`config/routing/allowlist.yaml`（并遵循 018 的迁移窗口流程要求）。

### 3.6 多租户：按“Kratos 认人 → RLS 圈地 → Casbin 管事”建设

- **数据模型最小要求**：业务表必须具备 `tenant_id`，并在服务/仓储层明确租户上下文来源。
- **RLS（已 PoC）**：
  - 启用/回滚策略见 `DEV-PLAN-019A`；当前 RLS PoC 表仍在推进中（旧 HRM `employees` 已在 061 中移除）。
  - 关键约束：事务内注入 `app.current_tenant`（fail-closed），且应用 DB 角色不能是 superuser/BYPASSRLS。
  - DB 角色入口：`make db rls-role`。
- **控制面（已落地）**：superadmin Tenant Console 见 `DEV-PLAN-019D`（跨租户配置仅允许在 superadmin server）。

## 4. 新建模块/新功能的“最短复用清单”（建议直接照做）

1. **按 DDD 分层建目录**：`modules/<module>/{domain,infrastructure,services,presentation}`（依赖约束见 `.gocleanarch.yml` 与 `DEV-PLAN-008`）。
2. **路由注册遵循 018**：先确定路由类别，再在模块 `module.go` 中注册到正确 namespace；必要时更新 `config/routing/allowlist.yaml` 并跑 `make check routing`。
3. **数据访问优先 sqlc**：新增 SQL 放在 `modules/<module>/infrastructure/sqlc/**`（如要扩展到非 Person，先补齐 dev-plan 与门禁/过滤器）。
4. **鉴权必须走 authz**：控制器/服务层调用 `pkg/authz`；策略走 `config/access/policies/**` + `make authz-pack`，并跑 `make authz-test && make authz-lint`。
5. **需要异步一致性就用 outbox**：复用 `pkg/outbox`，不要自建队列轮询器。
6. **按 AGENTS 触发器跑本地命令**：Go/templ/翻译/迁移/Authz/Outbox/Routing 各自有门禁入口，确保 CI 不因“漏跑生成/漏提交产物”失败。

## 5. 参考文档索引（从 009 到落地）

- 路线图（起点）：`docs/dev-plans/009-r200-tooling-alignment.md`
- sqlc：`docs/dev-plans/010-sqlc-baseline.md`
- Atlas+Goose（以 011A 为准）：`docs/dev-plans/011A-atlas-goose-baseline-gapfix.md`
- Casbin 基础设施/改造/UI：`docs/dev-plans/013-casbin-infrastructure-and-migration.md`、`docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`、`docs/dev-plans/015A-casbin-policy-platform.md`、`docs/dev-plans/015B-casbin-policy-ui-and-experience.md`
- Outbox：`docs/dev-plans/017-transactional-outbox.md`
- 路由策略：`docs/dev-plans/018-routing-strategy.md`、`docs/dev-plans/018B-routing-strategy-gates.md`
- 多租户：`docs/dev-plans/019-multi-tenant-toolchain.md`（含 019A/019B/019C/019D）
