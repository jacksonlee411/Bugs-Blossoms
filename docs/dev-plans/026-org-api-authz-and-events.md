# DEV-PLAN-026：Org API、Authz 与事件发布

**状态**: 规划中（2025-12-13 更新）

## 背景
- 对应 020 步骤 6，在主链 CRUD（024）与时间/审计（025）就绪后，需提供对外 REST API、统一鉴权（`pkg/authz`）以及事件发布闭环（Transactional Outbox + 可重放），并补齐缓存失效与对账接口，支撑 Authz/HRM 等下游订阅/纠偏。

## 目标
- API 覆盖节点/层级/岗位/分配的读写操作，返回值含租户隔离与有效期语义。
- 事件 `OrgChanged` / `OrgAssignmentChanged` 在写入成功后发布，并通过 Transactional Outbox 保证“数据与事件”原子一致。
- `make authz-test authz-lint authz-pack` 通过，策略片段提交。
- 所有入口接受 `effective_date` 参数（默认 `time.Now()`），响应/查询遵循时间线语义。
- 树/分配读写配套缓存键（含层级/tenant/effective_date）与事件驱动失效/重建策略。
- **对账与恢复**：提供 `GET /org/snapshot` 接口，允许下游系统（Authz/HRM）拉取指定时间点的全量状态，用于事件丢失后的纠偏。
- **批量事务**：提供 `POST /org/batch` 接口，支持在单事务内执行多条 Create/Update/Move 指令，保障组织架构调整（Reorg）的原子性。

## 范围与非目标
- 范围：对外 REST API、Authz 强制、策略片段、Outbox 事件闭环、缓存失效、snapshot/batch 两个系统性接口与配套测试/记录。
- 非目标：
  - 不实现 Org UI 体验完善（035 负责）；本计划仅保证 API 契约与错误返回稳定。
  - 不实现 change_requests/审批流/预检（030 负责）。
  - 不重新定义 022 的事件字段（026 只负责落地投递闭环与幂等/重放）。
  - 不改变 021 的核心表/约束定义（如需改动需回到 021/025 评审）。

## 与其他子计划的边界（必须保持清晰）
- 021：负责 schema/约束/迁移；026 只依赖其结构，不修改约束定义。
- 022：负责事件契约与占位表；026 复用事件契约，负责 outbox 投递闭环与消费者重放口径。
- 023：负责数据导入/回滚工具；026 提供 `POST /org/batch` 与 `GET /org/snapshot` 以支撑 023 的 `--backend api` 与纠偏链路。
- 024：负责主链 CRUD 的业务实现与最小页面；026 将其能力对外暴露为 API 并加上 Authz、outbox、缓存与对账。
- 025：负责冻结窗口、Correct/Update/Rescind/ShiftBoundary 的审计与强校验；026 负责把这些能力通过 API 暴露并返回稳定错误码/403 契约（冻结窗口细节仍以 025 为准）。
- 027：负责性能/读优化与 rollout；026 只给出缓存键与失效策略的 M1 基线，不承诺 M2 的闭包表/快照刷新。

## 依赖与里程碑
- 依赖：
  - 024：主链 service/repo/controller 骨架可用。
  - 025：Insert/Correct/Rescind/冻结窗口与审计（至少错误码与行为稳定）。
  - 022：事件结构体/Topic 命名与字段口径确定。
- 里程碑（按提交时间填充）：Authz 接入 -> outbox schema+relay -> API（含 snapshot/batch）-> 缓存失效 -> 测试与 readiness 记录。

## 设计决策
### 1. 路由与时间语义
- API 前缀：`/org`（对齐 020），所有读/写接口接受 `effective_date`（RFC3339 或 `YYYY-MM-DD`），缺省 `time.Now().UTC()`。
- 语义统一：查询一律按 as-of（`effective_date <= t < end_date`）读取有效片段；写入（Update）按 025 的 Insert 算法截断并插入新片段。

### 2. Authz（Casbin）接入与 403 契约
- 统一入口：controller 层使用 `modules/core/presentation/controllers.ensureAuthz`（或等价抽象）执行鉴权，并复用 `modules/core/authzutil.BuildForbiddenPayload` 输出统一 403 JSON/HTMX 契约（含 `missing_policies/suggest_diff/request_url/debug_url/base_revision/request_id`）。
- 对象命名：使用 `authz.ObjectName("org", "<resource>")`，例如：
  - `org.hierarchies`、`org.nodes`、`org.edges`、`org.positions`、`org.assignments`、`org.snapshot`、`org.batch`
- 动作口径（对齐 020 的能力命名，M1 保持粗粒度）：
  - `read`：所有 GET（树/分配/节点/岗位等）
  - `write`：节点/边/岗位的创建与更新（含 MoveNode）
  - `assign`：assignment 写入
  - `admin`：Correct/Rescind/ShiftBoundary、`/org/snapshot`、`/org/batch`
- 策略片段：新增 `config/access/policies/org/*.csv`，并执行 `make authz-pack` 生成 `config/access/policy.csv` 与 `config/access/policy.csv.rev`（禁止手改）。

### 3. 事件投递闭环（Transactional Outbox）
- 原子性要求：业务写入与 outbox 插入必须在同一数据库事务内提交（避免“数据已改但事件未发/事件已发但数据未改”）。
- 事件来源：复用 022 的 `OrgChangedEvent/OrgAssignmentChangedEvent`，Topic 仍按 `org.changed.v1/org.assignment.changed.v1`；`transaction_time` 取事务提交时间，`effective_window` 取变更的 valid time。
- 幂等与重放：outbox 以 `event_id` 为幂等键、`sequence` 为全局有序游标（M1 可先按 DB 自增/时间排序实现，027 再优化）；消费者必须幂等处理并允许重放。
- Relay：M1 可先做进程内 relay（轮询 outbox 未发布事件并 `Publish` 到应用内 `EventBus`），后续可替换为异步队列/外部总线（不改变 outbox 表与事件契约）。

### 4. Snapshot（对账/恢复）
- `GET /org/snapshot?effective_date=...&include=...` 返回指定时间点的全量状态（至少包含 `org_nodes/org_node_slices/org_edges/positions/org_assignments` 的 as-of 视图），用于事件丢失后的全量纠偏。
- 权限：默认要求 `org.* admin`；如需系统间调用，使用 system subject（由调用方注入）并单独策略放行。
- 性能口径：支持 `include=` 子集与分页（如有必要），避免一次性返回超大 payload；性能优化在 027。

### 5. Batch（单事务重组）
- `POST /org/batch` 在单事务内执行多条指令（Create/Update/Move/Assign），要么全部成功要么全部回滚；支持 `dry_run=true` 仅做校验与影响摘要（Impact 更完整版本在 030）。
- 权限：默认要求 `org.* admin`（M1 先保守）；后续可按指令类型降权。
- 事件策略：成功提交后按指令产生相应 outbox 事件；MoveNode 可能影响子树 path，M1 仅保证“边变更事件”可重放，子树级联事件策略在 027 评估。

### 6. 缓存键与失效
- 缓存键：至少覆盖树查询与按 subject 的分配查询，key 包含 `tenant_id/hierarchy_type/effective_date`（以及 subject id）。
- 失效策略：消费 outbox 事件后按 tenant 维度清理 org 缓存（M1 先粗粒度），必要时提供全量重建命令（调用 `/org/snapshot` 或重启应用）；精细化失效与读优化在 027。

## 任务清单与验收标准
1. [ ] API 路由与控制器：补齐 `/org/**` REST API（含 snapshot/batch），所有入口统一解析 `effective_date` 并强制 Session+tenant。验收：接口级测试覆盖租户隔离与默认 effective_date。
2. [ ] Authz 接入：为各 API 绑定 object+action（read/write/assign/admin），403 返回遵循统一 forbidden payload；新增 `config/access/policies/org/*.csv` 并运行 `make authz-test authz-lint authz-pack`。验收：门禁通过且 403 payload 含 `debug_url/base_revision` 等字段。
3. [ ] Outbox schema + repo：新增 outbox 表（或复用通用 outbox），写路径在同事务内插入 outbox 事件。验收：集成测试断言“写入成功必有 outbox 记录”，失败回滚无残留。
4. [ ] Relay 与重放：实现最小 relay（轮询未发布事件→发布到 EventBus→标记已发布），并保证重复消费幂等。验收：重复运行 relay 不产生重复副作用（以幂等键断言）。
5. [ ] Snapshot：实现 `/org/snapshot` 并支持 include 子集与必要分页；下游可用作全量纠偏。验收：在种子数据下可正确返回 as-of 视图且权限校验生效。
6. [ ] Batch：实现 `/org/batch`（含 dry_run），事务原子性与错误返回稳定。验收：测试覆盖全成功/中途失败回滚、dry-run 不落库、权限校验。
7. [ ] 缓存与失效：为树/分配读路径加缓存，订阅 outbox 事件触发失效；记录全量重建命令。验收：测试覆盖缓存命中/失效。
8. [ ] Readiness：执行 `make check lint`、`go test ./modules/org/...`、`make authz-test authz-lint authz-pack`；记录到 `docs/dev-records/DEV-PLAN-026-READINESS.md`。

## 验证记录
- 在 `docs/dev-records/DEV-PLAN-026-READINESS.md` 记录：
  - `make authz-test authz-lint authz-pack` 输出与 `git status --short` 干净确认
  - `/org/**` API 的关键用例（200/403、snapshot、batch dry-run/commit）
  - outbox/relay 的重放验证记录

## 风险与回滚/降级路径
- 策略风险：策略片段变更可能导致大面积 403；可先在 shadow 模式验证再切换 enforce（由 `config/access/authz_flags.yaml` 控制），并保留回滚 commit。
- Outbox 风险：relay 轮询可能带来 DB 压力；M1 先保守频率并提供开关（停用 relay 时仅保留 outbox 数据以便后续补发）。
- Snapshot 风险：全量响应可能过大；默认 require admin 且支持 include 子集/分页，性能优化在 027。
- Batch 风险：MoveNode 子树级联更新可能引发锁竞争；M1 限制 batch 指令规模并记录锁顺序，必要时拆分为多批次或改用离线窗口执行。

## 交付物
- `/org/**` REST API（含 `GET /org/snapshot`、`POST /org/batch`）与接口测试。
- `config/access/policies/org/*.csv` 策略片段与 authz 门禁通过记录。
- Outbox schema + relay + 事件重放验证。
- 缓存键与失效策略实现与记录。

## 交付物
- API/路由与权限片段。
- 事件发布实现。
- authz 门禁执行记录。
