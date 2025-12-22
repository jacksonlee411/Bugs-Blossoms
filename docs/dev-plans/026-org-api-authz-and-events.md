# DEV-PLAN-026：Org API、Authz 与事件发布（Outbox / Snapshot / Batch）

**状态**: 已完成（已合并至 main；Readiness：`docs/dev-records/DEV-PLAN-026-READINESS.md`）

## 0. 进度速记
- 024/025 定义主链 CRUD 与时间/审计写语义；026 在其契约上补齐“对外 API + Authz + outbox/relay + 缓存 + snapshot/batch”闭环。
- 022 是事件字段 SSOT；026 只负责“事务内 enqueue + relay 投递”，不重写 Topic/字段。
- 017 是 outbox 工具链与表结构 SSOT；026 只落地 `public.org_outbox` 并把它纳入 relay/cleaner。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` 步骤 6（API/Authz/事件闭环）。
- **当前痛点**：
  - 024/025 的写能力已定义但缺少“稳定对外入口 + 权限边界”，无法安全支撑 UI/脚本/集成。
  - 022 已冻结事件契约但缺少 outbox 事务入库与 relay 投递，无法保证“数据与事件一致”。
  - 缓存与对账缺失，下游（Authz/HRM）在事件丢失/延迟时无法自愈。
- **业务价值**：
  - `/org/api/**` 成为单一对外写入口（含审计/冻结窗口），并用 Casbin 固化权限口径。
  - outbox+relay 提供 at-least-once 的可靠投递，可重放、可观测。
  - snapshot/batch 提供“全量纠偏”与“原子重组”，降低 Reorg 风险与演练成本。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [x] **API 交付与冻结**：实现 `/org/api/**`，其中 CRUD/Correct 等接口的请求/响应/错误码以 024/025 为 SSOT；本计划补齐 Authz/403/outbox/caching/snapshot/batch。
- [x] **Authz**：
  - [x] 端点 → object/action 映射固化（见 §7.1），并用 `ensureAuthz` 统一校验。
  - [x] 403 返回 `modules/core/authzutil.ForbiddenPayload`（见 §5.2）。
  - [x] 提交策略片段 `config/access/policies/org/*.csv`，并通过 `make authz-test authz-lint authz-pack`。
- [x] **Outbox 原子一致**：
  - [x] 落地 `public.org_outbox`（结构/索引对齐 017）。
  - [x] 所有写事务在同一 DB tx 内 enqueue 022 事件 payload（topic=`org.changed.v1`/`org.assignment.changed.v1`）。
  - [x] relay/cleaner 通过 `OUTBOX_RELAY_TABLES/OUTBOX_CLEANER_TABLES` 启用，并支持单活与重试（对齐 017）。
- [x] **Snapshot（纠偏）**：实现 `GET /org/api/snapshot`，支持 as-of、include 子集与 cursor 分页（见 §5.4）。
- [x] **Batch（原子重组）**：实现 `POST /org/api/batch`，支持 dry-run、规模限制、失败定位（见 §5.5）。
- [x] **缓存（M1 基线）**：树查询与分配查询提供进程内缓存；写后即时失效 + outbox 事件驱动失效（tenant 粒度）。
- [x] **Readiness（按仓库门禁）**：在 `docs/dev-records/DEV-PLAN-026-READINESS.md` 记录：
  - `go fmt ./... && go vet ./... && make check lint && make test`
  - `make authz-test authz-lint authz-pack` 且 `git status --short` 干净
  - 如修改 `config/routing/allowlist.yaml`：`make check routing`
  - 如落地迁移：按仓库门禁执行 `make db migrate up && make db seed`（Org 迁移工具链以 021 为准）

### 2.2 非目标（本计划明确不做）
- 不交付 UI（035）/审批与预检（030）/性能读优化与 rollout（027）。
- 不改变 021 的 schema/约束定义；不改变 022 的 Topic/字段（如需演进必须先更新 022 并发布 v2）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Client[UI/CLI/Integrations] --> API[/org/api/**/]
  API --> Authz[ensureAuthz + ForbiddenPayload]
  API --> S[Org Services (024/025)]
  S --> R[Org Repos]
  R --> DB[(Postgres org_* tables)]
  S --> Pub[pkg/outbox Publisher]
  Pub --> O[(public.org_outbox)]
  O --> Relay[pkg/outbox Relay]
  Relay --> Disp[Org Dispatcher]
  Disp --> EB[pkg/eventbus.PublishE]
  EB --> Cache[Org Cache Invalidate (tenant)]
```

### 3.2 关键设计决策（ADR 摘要）
1. **路由与时间语义（选定）**
   - 内部 API 前缀：`/org/api`（对齐 `docs/dev-plans/018-routing-strategy.md`）。
   - 读请求按 as-of：`effective_date <= t < end_date`；写请求按 025 的 Insert/Correct/Rescind/ShiftBoundary 语义执行。
2. **Authz 粗粒度动作（选定）**
   - 动作固定为 `read/write/assign/admin`（M1）；避免接口级“动作爆炸”。
   - Assignment 写入只要求 `assign`，即使触发自动创建空壳 Position（见 035 的“权限悖论”约束）。
3. **Outbox 原子一致（选定）**
   - `org_outbox` 与业务写入必须同一事务提交；依赖 017 的 `pkg/outbox` 作为标准工具链。
4. **Relay 单活与顺序（选定）**
   - M1 使用 017 的 session-level advisory lock 保证每表单活；按 `sequence` 递增投递；下游不得假设全局顺序。
5. **Snapshot 用于纠偏（选定）**
   - Snapshot 返回“as-of 状态流”，用于事件丢失/重放/对账；通过 `include` 限制体积，并提供 cursor 分页。
6. **Batch M1 先保守（选定）**
   - M1 仅对 `org.* admin` 开放；限制 commands 数量与 Move 次数；失败返回第一处失败并带 `command_index`。

## 4. 数据模型与约束 (Data Model & Constraints)
> 026 新增的 DB 合同仅包含 outbox 表；Org 核心表以 021 为准。

### 4.1 `public.org_outbox`（对齐 017 的标准表结构）
```sql
CREATE TABLE org_outbox (
  id           UUID        NOT NULL DEFAULT gen_random_uuid(),
  tenant_id    UUID        NOT NULL,
  topic        TEXT        NOT NULL,
  payload      JSONB       NOT NULL,
  event_id     UUID        NOT NULL,
  sequence     BIGSERIAL   NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NULL,
  attempts     INT         NOT NULL DEFAULT 0,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at    TIMESTAMPTZ NULL,
  last_error   TEXT        NULL,

  CONSTRAINT org_outbox_pkey PRIMARY KEY (id),
  CONSTRAINT org_outbox_event_id_key UNIQUE (event_id),
  CONSTRAINT org_outbox_attempts_nonnegative CHECK (attempts >= 0)
);

CREATE INDEX org_outbox_pending_by_available
  ON org_outbox (available_at, sequence)
  WHERE published_at IS NULL;

CREATE INDEX org_outbox_published_by_time
  ON org_outbox (published_at, sequence)
  WHERE published_at IS NOT NULL;

CREATE INDEX org_outbox_tenant_published
  ON org_outbox (tenant_id, published_at, sequence);
```

**RLS 口径（M1）**：
- `org_outbox` **不启用 RLS**（对齐 017/019A）：relay 需要跨租户 claim；如未来启用必须使用专用 DB role 与审计，不得用放宽 policy 绕过隔离。

### 4.2 迁移策略
- 新增迁移：`migrations/org/00004_org_outbox.sql`（序号以 021 baseline 为准；必须位于 025 的 settings/audit 之后）。
- **Up**：创建表 + 索引 + 约束；如缺 `gen_random_uuid()` 则确保 `pgcrypto` 可用。
- **Down**：删除表（生产通常不建议执行破坏性 down，需先确认无需补发）。

## 5. 接口契约 (API Contracts)
> 本节只定义 026 新增的 snapshot/batch 与 Authz/403 合同；CRUD/Correct 端点形状以 024/025 为 SSOT。

### 5.1 通用约定
- 路由前缀：`/org/api/*`。
- 时间参数：
  - `effective_date` 支持 `YYYY-MM-DD` 或 RFC3339；`YYYY-MM-DD` 解释为 `00:00:00Z`。
  - 缺省 `effective_date`：服务端使用 `time.Now().UTC()`。
- 错误响应（JSON）统一形状（复用 `modules/core/presentation/controllers/dtos.APIError`）：
  ```json
  { "code": "ORG_INVALID_QUERY", "message": "...", "meta": { "request_id": "..." } }
  ```

### 5.2 403 Forbidden（统一契约）
- 所有 `/org/api/**` 端点在鉴权失败时返回 `403`，body 为 `modules/core/authzutil.ForbiddenPayload`：
  ```json
  {
    "error": "forbidden",
    "message": "Forbidden: org.nodes write. 如需申请权限，请访问 /core/api/authz/requests。",
    "object": "org.nodes",
    "action": "write",
    "subject": "tenant:...:user:...",
    "domain": "tenant_uuid",
    "missing_policies": [{"domain":"...","object":"org.nodes","action":"write"}],
    "suggest_diff": [],
    "request_url": "/core/api/authz/requests",
    "debug_url": "/core/api/authz/debug?...",
    "base_revision": "policy.csv.rev",
    "request_id": "X-Request-ID"
  }
  ```

### 5.3 `/org/api/**` Authz 映射（M1 固化）
> object 使用 `authz.ObjectName("org", "<resource>")`，即 `org.<resource>`；action 使用 `read/write/assign/admin`。

| Endpoint | Object | Action |
| --- | --- | --- |
| `GET /org/api/hierarchies`（024） | `org.hierarchies` | `read` |
| `POST /org/api/nodes`（024） | `org.nodes` | `write` |
| `PATCH /org/api/nodes/{id}`（024） | `org.nodes` | `write` |
| `POST /org/api/nodes/{id}:move`（024） | `org.edges` | `write` |
| `POST /org/api/nodes/{id}:correct`（025） | `org.nodes` | `admin` |
| `POST /org/api/nodes/{id}:rescind`（025） | `org.nodes` | `admin` |
| `POST /org/api/nodes/{id}:shift-boundary`（025） | `org.nodes` | `admin` |
| `POST /org/api/nodes/{id}:correct-move`（025） | `org.edges` | `admin` |
| `GET /org/api/positions`（053） | `org.positions` | `read` |
| `GET /org/api/positions/{id}`（053） | `org.positions` | `read` |
| `GET /org/api/positions/{id}/timeline`（053） | `org.positions` | `read` |
| `POST /org/api/positions`（053） | `org.positions` | `write` |
| `PATCH /org/api/positions/{id}`（053） | `org.positions` | `write` |
| `POST /org/api/positions/{id}:correct`（053） | `org.positions` | `admin` |
| `POST /org/api/positions/{id}:rescind`（053） | `org.positions` | `admin` |
| `POST /org/api/positions/{id}:shift-boundary`（053） | `org.positions` | `admin` |
| `GET /org/api/positions/{id}/restrictions`（056） | `org.position_restrictions` | `read` |
| `POST /org/api/positions/{id}:set-restrictions`（056） | `org.position_restrictions` | `admin` |
| `GET /org/api/job-catalog/*`（056） | `org.job_catalog` | `read` |
| `POST/PATCH /org/api/job-catalog/*`（056） | `org.job_catalog` | `admin` |
| `GET /org/api/job-profiles`（056） | `org.job_profiles` | `read` |
| `POST/PATCH /org/api/job-profiles*`（056） | `org.job_profiles` | `admin` |
| `GET /org/api/assignments`（024） | `org.assignments` | `read` |
| `POST /org/api/assignments`（024） | `org.assignments` | `assign` |
| `PATCH /org/api/assignments/{id}`（024） | `org.assignments` | `assign` |
| `POST /org/api/assignments/{id}:correct`（025） | `org.assignments` | `admin` |
| `POST /org/api/assignments/{id}:rescind`（025） | `org.assignments` | `admin` |
| `GET /org/api/snapshot`（本计划） | `org.snapshot` | `admin` |
| `POST /org/api/batch`（本计划） | `org.batch` | `admin` |

### 5.4 `GET /org/api/snapshot`
用于下游纠偏：返回指定 `effective_date` 的 as-of 全量状态（支持 include 与分页）。

**Query**
- `effective_date`：可选，缺省 `nowUTC`
- `include`：可选，逗号分隔，默认 `nodes,edges`
  - 允许值：`nodes|edges|positions|assignments`
- `limit`：可选，默认 `2000`，最大 `10000`
- `cursor`：可选，上一页返回的 `next_cursor`

**Response 200**
```json
{
  "tenant_id": "uuid",
  "effective_date": "2025-01-01T00:00:00Z",
  "generated_at": "2025-01-01T12:00:00Z",
  "includes": ["nodes", "edges"],
  "limit": 2000,
  "items": [
    {
      "entity_type": "org_node",
      "entity_id": "uuid",
      "new_values": {
        "org_node_id": "uuid",
        "code": "D001",
        "name": "Engineering",
        "status": "active",
        "parent_node_id": null,
        "effective_date": "2025-01-01T00:00:00Z",
        "end_date": "9999-12-31T00:00:00Z"
      }
    }
  ],
  "next_cursor": "org_node:uuid"
}
```

**Cursor 约定（v1）**
- `cursor/next_cursor` 形如 `<entity_type>:<uuid>`。
- 固定排序（M1）：`org_node` → `org_edge` → `org_position` → `org_assignment`，同类型内按 `entity_id` 升序。
- `include` 会裁剪类型集合；排序仍按上述顺序在子集上执行。

**Errors**
- 400 `ORG_INVALID_QUERY`：`effective_date/include/limit/cursor` 非法
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden（见 §5.2）

### 5.5 `POST /org/api/batch`
在单事务内执行多条写指令：要么全部成功，要么全部回滚；支持 `dry_run`。

**Request**
```json
{
  "dry_run": true,
  "effective_date": "2025-03-01",
  "commands": [
    {
      "type": "node.create",
      "payload": { "code": "D001", "name": "Engineering", "parent_id": null }
    },
    {
      "type": "node.move",
      "payload": { "id": "uuid", "new_parent_id": "uuid" }
    }
  ]
}
```

**Command Types（M1）**
> `payload` 字段与对应单条接口的 request body **一致**；path 参数用 `payload.id` 传入。

| type | 对应单条接口（SSOT） |
| --- | --- |
| `node.create` | `POST /org/api/nodes`（024） |
| `node.update` | `PATCH /org/api/nodes/{id}`（024） |
| `node.move` | `POST /org/api/nodes/{id}:move`（024） |
| `node.correct` | `POST /org/api/nodes/{id}:correct`（025） |
| `node.rescind` | `POST /org/api/nodes/{id}:rescind`（025） |
| `node.shift_boundary` | `POST /org/api/nodes/{id}:shift-boundary`（025） |
| `node.correct_move` | `POST /org/api/nodes/{id}:correct-move`（025） |
| `assignment.create` | `POST /org/api/assignments`（024） |
| `assignment.update` | `PATCH /org/api/assignments/{id}`（024） |
| `assignment.correct` | `POST /org/api/assignments/{id}:correct`（025） |
| `assignment.rescind` | `POST /org/api/assignments/{id}:rescind`（025） |

**Rules**
- `commands`：必填，长度 `1..100`（超过返回 422 `ORG_BATCH_TOO_LARGE`）。
- `dry_run=true`：只做校验与影响摘要，不落库、不写 outbox（实现可用“事务执行后回滚”确保一致性）。
- `effective_date`：作为默认值注入到 `payload` 中缺失的 `effective_date` 字段（不会覆盖显式提供的值）。
- `node.move` 与 `node.correct_move` 视为重型指令：M1 最多允许 10 条（超过返回 422 `ORG_BATCH_TOO_MANY_MOVES`）。

**Response 200**
```json
{
  "dry_run": false,
  "results": [
    { "index": 0, "type": "node.create", "ok": true, "result": { "id": "uuid" } },
    { "index": 1, "type": "node.move", "ok": true, "result": { "id": "uuid" } }
  ],
  "events_enqueued": 3
}
```

**Errors**
- 422 `ORG_BATCH_INVALID_BODY`
- 422 `ORG_BATCH_TOO_LARGE`
- 422 `ORG_BATCH_TOO_MANY_MOVES`
- 422 `ORG_BATCH_INVALID_COMMAND`：未知 `type` / payload 字段缺失
- 409/422：来自单条接口的稳定错误码（024/025 SSOT），并在 `meta` 中附带：
  - `command_index`（int）
  - `command_type`（string）
- 401/400/403：同 §5.4

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 事务内 enqueue（Outbox 原子一致）
1. controller 解析 `effective_date`、`request_id`、tenant/session 并完成 Authz。
2. service 开启事务（并注入 RLS：`composables.ApplyTenantRLS(ctx, tx)`）。
3. 执行业务写入（024/025）。
4. 在 **同一事务内** 对每条变更事件：
   - 生成 `event_id`（UUID）
   - 选择 `topic`（022：`org.changed.v1` / `org.assignment.changed.v1`）
   - 构造 022 v1 payload（必须 snake_case）
   - `outbox.Publisher.Enqueue(ctx, tx, pgx.Identifier{"public","org_outbox"}, msg)`
5. 提交事务；回滚时 outbox 行随事务回滚，无残留。

### 6.2 Relay/Dispatcher（重试与重放）
- relay：复用 017 的 `pkg/outbox`（`OUTBOX_RELAY_SINGLE_ACTIVE=true` 默认单活；按 `sequence` 递增 claim/dispatch/ack）。
- dispatcher：
  - 以 `msg.Meta.Topic` 选择事件类型；
  - 将 `msg.Payload` 反序列化为 022 对应结构体；
  - 调用 `pkg/eventbus.PublishE(meta, &Event)`，并将 handler error/panic 转为 `error` 供 outbox retry。

### 6.3 Snapshot（as-of 状态流）
1. 解析 `effective_date/include/limit/cursor`；Authz=admin。
2. 以固定顺序遍历 entity_type（受 include 裁剪）并分页读取：
   - `org_node`：读取 as-of node slice + as-of parent（来自 edge as-of）
   - `org_edge`：读取 as-of edge slice（每 child 一条）
   - `org_position`：读取 as-of position（含 org_node_id）
   - `org_assignment`：读取 as-of assignment（含 pernr/subject_id/position_id）
3. 拼装 `items[]` 与 `next_cursor`；若不足一页则 `next_cursor=null`。

### 6.4 Batch（单事务、多指令）
1. 解析请求，做结构校验与规模限制；Authz=admin。
2. 将全局 `effective_date` 注入到缺失字段的 command payload。
3. 开启事务并顺序执行 commands：
   - 每条 command 调用与其对应的 service 方法（与单条接口同源）。
   - 任一失败：回滚并返回错误码（携带 `command_index/type`）。
4. `dry_run=true`：执行后显式回滚，`events_enqueued=0`。

### 6.5 缓存与失效（M1）
- 缓存对象：进程内、tenant-scoped（避免全局 `Clear()` 放大影响）。
- key：`repo.CacheKey("org", kind, tenantID, effectiveDate, extra...)`。
- 失效：
  - 写后即时：写事务 commit 后本进程立即失效该 tenant（保证读己一致）。
  - 跨实例：订阅 outbox 事件，在 handler 中按 tenant 失效（tenant 粗粒度）。

## 7. 安全与鉴权 (Security & Authz)
### 7.1 Casbin object/action（SSOT）
- object：`org.hierarchies|org.nodes|org.edges|org.positions|org.assignments|org.position_restrictions|org.job_catalog|org.job_profiles|org.snapshot|org.batch`
- action：`read|write|assign|admin`（见 §5.3 映射表）

### 7.2 403 Payload（SSOT）
- 统一复用 `modules/core/authzutil.BuildForbiddenPayload`（见 §5.2）。

### 7.3 Subject 标识与映射（SSOT）
> 本节是 `org_assignments.subject_id` 的单一事实源（SSOT）。UI/CLI/API 不得各自实现不同映射规则，避免漂移。

> ⚠️ 重要：本仓库已采纳 **方案 B**（`subject_id = person_uuid`）。历史版本的“确定性派生 subject_id（UUIDv5/sha1）”口径作废，不再作为 SSOT。

- 术语：
  - `subject_type`：M1 固定为 `person`（DB 约束见 021）。
  - `pernr`：人员可读标识（string），允许前导零；写入与解析前必须 `TrimSpace`，其余保持原样（区分大小写）。
  - `subject_id`：**`person_uuid`**（UUID），来源于 Person SOR（`persons.person_uuid`）。
- 映射规则（选定）：
  - 写入任职记录时：服务端以 `tenant_id + pernr` 查询 Person SOR 得到 `person_uuid`，并将其写入 `org_assignments.subject_id`。
  - 解析 SQL（示意）：
    - `SELECT person_uuid FROM persons WHERE tenant_id=$1 AND pernr=$2`
- 契约：
  - 仅提供 `pernr` 时：服务端 resolve `person_uuid` 后落库（`subject_id=person_uuid`）。
  - 同时提供 `subject_id` 与 `pernr`：必须校验 `subject_id == resolved(person_uuid)`，不一致返回 422 `ORG_SUBJECT_MISMATCH`。
  - `pernr` 无法解析到 Person：返回 404 `ORG_PERSON_NOT_FOUND`（由 Org 服务层返回，保持 `/org/api/*` 错误码前缀一致）。
- 代码落点（必须复用，避免跨模块 Go 依赖）：
  - Org service 层不得直接依赖 `modules/person` 包；resolve 逻辑应落在 Org repo 接口中（例如 `ResolvePersonUUIDByPernr(ctx, tenantID, pernr)`），由 `modules/org/infrastructure/persistence` 通过 SQL 查询 `persons` 表实现。

### 7.4 租户隔离与 RLS
- `/org/api/**` 必须强制 Session + tenant；当 `RLS_ENFORCE=enforce` 时事务内必须调用 `composables.ApplyTenantRLS`（对齐 019A）。
- `org_outbox` M1 不启用 RLS（见 §4.1）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- [DEV-PLAN-017](017-transactional-outbox.md)：`pkg/outbox` + 标准表结构/relay/metrics。
- [DEV-PLAN-021](021-org-schema-and-constraints.md)：Org schema/迁移工具链（含 `migrations/org` 目录约定）。
- [DEV-PLAN-022](022-org-placeholders-and-event-contracts.md)：Topic/字段口径 SSOT。
- [DEV-PLAN-024](024-org-crud-mainline.md)：主链 CRUD 与最小 API 形状。
- [DEV-PLAN-025](025-org-time-and-audit.md)：冻结窗口/Correct/Rescind/ShiftBoundary/correct-move 与错误码口径。
- [DEV-PLAN-019A](019A-rls-tenant-isolation.md)：RLS 注入与隔离约定。

### 8.2 里程碑（按提交时间填充）
1. [x] Authz：object/action 映射落地 + 策略片段 + 门禁
2. [x] `org_outbox` 迁移 + 写事务内 enqueue（覆盖所有写路径）
3. [x] relay/dispatcher 接入 + 缓存失效 handler
4. [x] `/org/api/snapshot`（include + cursor）
5. [x] `/org/api/batch`（dry_run + 规模限制 + 失败定位）
6. [x] 测试与 readiness 记录

## 9. 测试与验收标准 (Acceptance Criteria)
- **Authz**：
  - 403 返回 `ForbiddenPayload`，包含 `missing_policies/suggest_diff/debug_url/base_revision/request_id`。
  - `make authz-test authz-lint authz-pack` 通过且生成物提交。
- **Outbox 原子一致**：
  - 成功写入必有 `org_outbox` 行；事务回滚无 outbox 残留。
  - relay 重放不会产生不可恢复的副作用（消费者以 `event_id` 幂等）。
- **Snapshot**：
  - 默认 `include=nodes,edges`；include 扩展能返回 positions/assignments。
  - cursor 分页可覆盖 >1 页，且顺序稳定。
- **Batch**：
  - 全成功：全部写入并 enqueue 事件。
  - 中途失败：全回滚；错误 `meta` 包含 `command_index/type`。
  - dry_run：不落库、不写 outbox。
- **Readiness**：输出记录落盘到 `docs/dev-records/DEV-PLAN-026-READINESS.md`（命令与结果对齐门禁矩阵）。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 Feature Flag / 环境变量（对齐 017）
- Outbox：
  - `OUTBOX_RELAY_ENABLED=true|false`
  - `OUTBOX_RELAY_TABLES=public.org_outbox`
  - `OUTBOX_RELAY_SINGLE_ACTIVE=true|false`（默认 true）
  - `OUTBOX_CLEANER_ENABLED=true|false`
  - `OUTBOX_CLEANER_TABLES=public.org_outbox`（为空则跟随 relay tables）
- Authz：
  - `AUTHZ_MODE=shadow|enforce`（默认 shadow；切换需谨慎）

### 10.2 关键指标（对齐 017）
- `outbox_pending{table}` / `outbox_locked{table}`
- `outbox_dispatch_total{table,topic,result}`
- `outbox_dispatch_latency_seconds{table,topic,result}`
- `outbox_dead_total{table,topic}`

### 10.3 回滚与降级
- Authz 策略误伤：优先保持 `AUTHZ_MODE=shadow` 观察，再切 `enforce`；紧急情况下回滚策略片段 commit 或临时切回 shadow。
- Outbox 压力/故障：关闭 `OUTBOX_RELAY_ENABLED`（保留 `org_outbox` 数据以便后续补发）；排障入口见 `docs/runbooks/transactional-outbox.md`。
- Snapshot/Batch 压力：维持 admin-only；必要时通过路由/feature flag 临时关闭入口（实现需提供开关）。

## 交付物
- `/org/api/**`：完成 024/025 SSOT 端点的 Authz/outbox/caching 接入，并新增：
  - `GET /org/api/snapshot`
  - `POST /org/api/batch`
- `migrations/org/00004_org_outbox.sql`（或等价迁移）与相关测试/记录。
- `config/access/policies/org/*.csv` 策略片段与 `authz-pack` 生成物。
- Readiness 记录：`docs/dev-records/DEV-PLAN-026-READINESS.md`。
