# DEV-PLAN-026：Org API、Authz 与事件发布

**状态**: 规划中（2025-12-13 更新）

## 背景
- 对应 020 步骤 6，在主链 CRUD（024）与时间/审计（025）就绪后，需提供对外 REST API、统一鉴权（`pkg/authz`）以及事件发布闭环（Transactional Outbox + 可重放），并补齐缓存失效与对账接口，支撑 Authz/HRM 等下游订阅/纠偏。

## 目标
- API 覆盖节点/层级/岗位/分配的读写操作，返回值含租户隔离与有效期语义。
- 事件 Topics `org.changed.v1` / `org.assignment.changed.v1` 在写入成功后发布，并通过 Transactional Outbox 保证“数据与事件”原子一致。
- `make authz-test authz-lint authz-pack` 通过，策略片段提交。
- 所有入口接受 `effective_date` 参数（默认 `time.Now()`），响应/查询遵循时间线语义。
- 树/分配读写配套缓存键（含层级/tenant/effective_date）与事件驱动失效/重建策略。
- **对账与恢复**：提供 `GET /org/api/snapshot` 接口，允许下游系统（Authz/HRM）拉取指定时间点的全量状态，用于事件丢失后的纠偏。
- **批量事务**：提供 `POST /org/api/batch` 接口，支持在单事务内执行多条 Create/Update/Move 指令，保障组织架构调整（Reorg）的原子性。

## 范围与非目标
- 范围：对外 REST API、Authz 强制、策略片段、Outbox 事件闭环、缓存失效、snapshot/batch 两个系统性接口与配套测试/记录。
- 非目标：
  - 不实现 Org UI 体验完善（035 负责）；本计划仅保证 API 契约与错误返回稳定。
  - 不实现 `org_change_requests`/审批流/预检（030 负责）。
  - 不重新定义 022 的事件字段（026 只负责落地投递闭环与幂等/重放）。
  - 不改变 021 的核心表/约束定义（如需改动需回到 021/025 评审）。

## 与其他子计划的边界（必须保持清晰）
- 021：负责 schema/约束/迁移；026 只依赖其结构，不修改约束定义。
- 022：负责事件契约与占位表；026 复用事件契约，负责 outbox 投递闭环与消费者重放口径。
- 023：负责数据导入/回滚工具；026 提供 `POST /org/api/batch` 与 `GET /org/api/snapshot` 以支撑 023 的 `--backend api` 与纠偏链路。
- 024：负责主链 CRUD 的业务实现与最小页面；026 将其能力对外暴露为 API 并加上 Authz、outbox、缓存与对账。
- 025：负责冻结窗口、Correct/Update/Rescind/ShiftBoundary 的审计与强校验；026 负责把这些能力通过 API 暴露并返回稳定错误码/403 契约（冻结窗口细节仍以 025 为准）。
- 027：负责性能/读优化与 rollout；026 只给出缓存键与失效策略的 M1 基线，不承诺 M2 的闭包表/快照刷新。

## 依赖与里程碑
- 依赖：
  - 017：Transactional Outbox 工具链（`pkg/outbox` + 标准 schema/relay）。
  - 024：主链 service/repo/controller 骨架可用。
  - 025：Insert/Correct/Rescind/冻结窗口与审计（至少错误码与行为稳定）。
  - 022：事件结构体/Topic 命名与字段口径确定。
- 里程碑（按提交时间填充）：Authz 接入 -> outbox schema+relay -> API（含 snapshot/batch）-> 缓存失效 -> 测试与 readiness 记录。

## 设计决策
### 1. 路由与时间语义
- 内部 API 前缀：`/org/api`（对齐 `docs/dev-plans/018-routing-strategy.md` 的 `/{module}/api/*`），所有读/写接口接受 `effective_date`（RFC3339 或 `YYYY-MM-DD`），缺省 `time.Now().UTC()`。
- 路由登记：同步更新 `config/routing/allowlist.yaml`（`/org`=ui、`/org/api`=internal_api），并确保 `/org/api/*` 下 404/405/500 遵循 internal API 的 JSON-only 全局错误契约（对齐 018B）。
- 语义统一：查询一律按 as-of（`effective_date <= t < end_date`）读取有效片段；写入（Update）按 025 的 Insert 算法截断并插入新片段。

### 2. Authz（Casbin）接入与 403 契约
- 统一入口：controller 层使用 `modules/core/presentation/controllers.ensureAuthz`（或等价抽象）执行鉴权，并复用 `modules/core/authzutil.BuildForbiddenPayload` 输出统一 403 JSON/HTMX 契约（含 `missing_policies/suggest_diff/request_url/debug_url/base_revision/request_id`）。
- 对象命名：使用 `authz.ObjectName("org", "<resource>")`，例如：
  - `org.hierarchies`、`org.nodes`、`org.edges`、`org.positions`、`org.assignments`、`org.snapshot`、`org.batch`
- 动作口径（对齐 020 的能力命名，M1 保持粗粒度）：
  - `read`：所有 GET（树/分配/节点/岗位等）
  - `write`：节点/边/岗位的创建与更新（含 MoveNode）
  - `assign`：assignment 写入
  - `admin`：Correct/Rescind/ShiftBoundary、`/org/api/snapshot`、`/org/api/batch`
- 策略片段：新增 `config/access/policies/org/*.csv`，并执行 `make authz-pack` 生成 `config/access/policy.csv` 与 `config/access/policy.csv.rev`（禁止手改）。

### 3. 事件投递闭环（Transactional Outbox）
- 原子性要求：业务写入与 outbox 插入必须在同一数据库事务内提交（避免“数据已改但事件未发/事件已发但数据未改”）。
- 事件来源：复用 022 的事件契约（Topics `org.changed.v1/org.assignment.changed.v1`）；`transaction_time` 取事务提交时间，`effective_window` 取变更的 valid time。
- 工具链：复用 `DEV-PLAN-017` 的 `pkg/outbox`；本模块使用独立表 `org_outbox`（结构与索引对齐 017，便于未来演进为独立服务）。
- RLS 边界（对齐 019A）：PoC 阶段 `org_outbox` 不启用 RLS 以保证 relay 可跨租户 claim；如未来需要启用，必须走专用 DB role/连接池与审计，禁止通过放宽 policy 绕过隔离。
- 幂等与重放：outbox 以 `event_id` 为幂等键、`sequence` 为有序游标；消费者与应用内 handler 必须基于 `event_id` 幂等处理并允许重放。
- 顺序性：M1 relay 采用**单 worker**按 `sequence` 升序投递；多实例部署通过“仅开启一个 relay”或使用 `pg_try_advisory_lock` 保证同一时刻只有一个 relay；如未来并行化，按 `tenant_id` 分区并在分区内保持顺序（否则下游不得假设顺序）。
- Outbox 表（M1，示意字段）：
  - `id uuid pk`、`tenant_id uuid not null`、`topic text not null`、`event_id uuid not null unique`、`sequence bigserial`、`payload jsonb not null`、`created_at timestamptz default now()`、`published_at timestamptz null`、`attempts int default 0`、`available_at timestamptz default now()`、`locked_at timestamptz null`、`last_error text null`
  - 索引：`(published_at, sequence)`、`(tenant_id, published_at, sequence)`、`(available_at, sequence) where published_at is null`
- Relay：使用 `pkg/outbox` relay（短事务 claim/dispatch/ack；`FOR UPDATE SKIP LOCKED` + `available_at/locked_at` + 重试退避）。投递目标为 `outbox.Dispatcher`（需返回 `error`）；若继续投递到进程内 `pkg/eventbus`，需补充可返回错误的适配层（例如新增 `PublishE`/`PublishWithResult`），否则 outbox 无法判定失败并安全重试。

### 4. Snapshot（对账/恢复）
- `GET /org/api/snapshot?effective_date=...&include=...` 返回指定时间点的状态；默认仅返回树最小集（`org_nodes/org_node_slices/org_edges`），岗位/分配等通过 `include=` 显式请求，避免大 payload/超时。
- 权限：默认要求 `org.* admin`；如需系统间调用，使用 system subject（由调用方注入）并单独策略放行。
- 性能口径：支持 `include=` 子集与分页（如有必要），避免一次性返回超大 payload；性能优化在 027。
- 响应建议包含：`tenant_id/effective_date/generated_at`、各资源的数组与计数摘要（便于下游快速校验差异）。

### 5. Batch（单事务重组）
- `POST /org/api/batch` 在单事务内执行多条指令（Create/Update/Move/Assign），要么全部成功要么全部回滚；支持 `dry_run=true` 仅做校验与影响摘要（Impact 更完整版本在 030）。
- 规模限制：M1 对 `commands` 数量设上限（建议 50~100）；`MoveNode` 视为重型指令，可进一步限制 Move 条数或禁止在同一批次混入多次 Move，降低长事务/死锁风险。
- 权限：默认要求 `org.* admin`（M1 先保守）；入口 admin 已覆盖写能力，不再逐指令鉴权；如后续需降权，再在入口按 command 预检所需 object+action 并返回稳定错误。
- 事件策略：成功提交后按指令产生相应 outbox 事件；MoveNode 可能影响子树 path，M1 仅保证“边变更事件”可重放，子树级联事件策略在 027 评估。
- 请求体建议结构（示意）：
  - `effective_date`（全局默认）+ `commands[]`（每条含 `type` 与 `payload`），服务端校验通过后在同事务内执行并写 outbox。

### 6. 缓存键与失效
- 缓存键：至少覆盖树查询与按 subject 的分配查询，key 包含 `tenant_id/hierarchy_type/effective_date`（以及 subject id），建议使用 `pkg/repo.CacheKey("org", "<kind>", ...)` 生成。
- 写后即时失效：写事务提交后，本进程立即按 tenant 粒度清理本地缓存（避免等待 relay 轮询带来的读旧值体验）；跨实例仍以 outbox relay 驱动失效。
- 失效策略：消费 outbox 事件后按 tenant 维度清理 org 缓存（M1 先粗粒度），必要时提供全量重建命令（调用 `/org/api/snapshot` 或重启应用）；精细化失效与读优化在 027。

## 任务清单与验收标准
1. [ ] API 路由与控制器：补齐 `/org/api/**` REST API（含 snapshot/batch），所有入口统一解析 `effective_date` 并强制 Session+tenant。验收：接口级测试覆盖租户隔离与默认 effective_date。
2. [ ] Authz 接入：为各 API 绑定 object+action（read/write/assign/admin），403 返回遵循统一 forbidden payload；新增 `config/access/policies/org/*.csv` 并运行 `make authz-test authz-lint authz-pack`。验收：门禁通过且 403 payload 含 `debug_url/base_revision` 等字段。
3. [ ] Outbox schema + enqueue：创建 `org_outbox`（结构对齐 017），写路径在同事务内 enqueue outbox 事件（使用 `pkg/outbox`）。验收：集成测试断言“写入成功必有 outbox 记录”，失败回滚无残留。
4. [ ] Relay 与重放：接入 `pkg/outbox` relay（按 `sequence` 升序轮询未发布事件→投递到 `Dispatcher`→标记已发布），并保证多实例下不并发投递（开关或 advisory lock）与幂等重放。验收：并发跑两个 relay 不产生重复副作用（以 `event_id` 断言），且顺序性符合预期。
5. [ ] Snapshot：实现 `/org/api/snapshot`（默认最小集，`include=` 取子集）并支持必要分页；下游可用作全量纠偏。验收：在种子数据下可正确返回 as-of 视图且权限校验生效。
6. [ ] Batch：实现 `/org/api/batch`（含 dry_run），限制单次 commands 规模并保证事务原子性与错误返回稳定。验收：测试覆盖全成功/中途失败回滚、dry-run 不落库、权限校验与规模限制。
7. [ ] 缓存与失效：为树/分配读路径加缓存，订阅 outbox 事件触发失效；记录全量重建命令。验收：测试覆盖缓存命中/失效。
8. [ ] Readiness：执行 `make check lint`、`go test ./modules/org/...`、`make authz-test authz-lint authz-pack`；记录到 `docs/dev-records/DEV-PLAN-026-READINESS.md`。

## 验证记录
- 在 `docs/dev-records/DEV-PLAN-026-READINESS.md` 记录：
  - `make authz-test authz-lint authz-pack` 输出与 `git status --short` 干净确认
  - `/org/api/**` API 的关键用例（200/403、snapshot、batch dry-run/commit）
  - outbox/relay 的重放验证记录

## 风险与回滚/降级路径
- 策略风险：策略片段变更可能导致大面积 403；可先在 shadow 模式验证再切换 enforce（由 `config/access/authz_flags.yaml` 控制），并保留回滚 commit。
- Outbox 风险：relay 轮询可能带来 DB 压力；M1 先保守频率并提供开关（停用 relay 时仅保留 outbox 数据以便后续补发）。
- Snapshot 风险：全量响应可能过大；默认 require admin 且支持 include 子集/分页，性能优化在 027。
- Batch 风险：MoveNode 子树级联更新可能引发锁竞争；M1 限制 batch 指令规模/Move 数量并记录锁顺序，必要时拆分为多批次或改用离线窗口执行。

## 交付物
- `/org/api/**` REST API（含 `GET /org/api/snapshot`、`POST /org/api/batch`）与接口测试。
- `config/access/policies/org/*.csv` 策略片段与 authz 门禁通过记录。
- Outbox schema + relay + 事件重放验证。
- 缓存键与失效策略实现与记录。
