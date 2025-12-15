# DEV-PLAN-017：事务性发件箱 (Transactional Outbox) 工具链

**状态**: 进行中（2025-12-15 01:34 UTC — M1 基础设施落地完成）

## 1. 背景与上下文 (Context)
- **需求来源**：
  - `docs/dev-plans/009-r200-tooling-alignment.md`（事件可靠投递基础设施诉求，outbox 作为标准模式）。
  - `docs/dev-plans/020-organization-lifecycle.md`：步骤 6 明确 Org 需 “业务写入 + outbox 入库同一事务”，并在事务提交后由 relay 投递。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Org 将率先落地 `org_outbox` + relay，要求 dispatcher 返回 `error` 以支持安全重试与重放。
- **当前痛点**：
  - 现有 `pkg/eventbus` 为进程内发布：无 topic 概念、无可返回 `error` 的投递边界，不适合作为 outbox relay 的稳定投递契约。
  - 若各模块各自实现 outbox 表结构与 relay，会导致：重复代码、监控分散、重试/幂等语义不一致，且难以形成统一运维手册。
- **业务价值**：
  - 将“可靠投递、可重试、可追溯”的事件发布能力沉淀为通用基础设施，支撑 Org/后续模块按统一方式对外发布变更事件，降低跨模块集成风险。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [X] 交付通用 `pkg/outbox`（Publisher/Relay/Cleaner），仅依赖 `pgx/v5` 与现有 `pkg/repo.Tx` 抽象。
  - [ ] 标准化 outbox 表结构与查询口径：支持“按模块独立表，结构统一”（如 `org_outbox`、`hrm_outbox`）。
  - [X] 明确投递语义：**At-least-once**；以 `event_id` 为幂等键，允许重复投递，消费者必须幂等。
  - [X] 明确 dispatcher 契约（必须返回 `error`），并给出与 `pkg/eventbus` 的集成决策（见 5.2）。
  - [X] 提供 Prometheus 指标、日志字段与运维开关建议（见 10）。
  - [X] 通过 `go fmt ./... && go vet ./... && make check lint && make test`。
- **非目标 (M1)**：
  - 不引入 Debezium/Kafka/Connect 等 CDC 基础设施。
  - 不承诺 Exactly-once；不在 relay 内做“跨消费者”的去重副作用。
  - 不强制所有模块共用同一张 outbox 表（仍支持 `<module>_outbox`）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  A[Business Tx] -->|same tx enqueue| B[(<module>_outbox)]
  B --> C[Relay: claim/dispatch/ack]
  C -->|Dispatch(msg)| D[Dispatcher (error boundary)]
  D --> E[In-proc Handlers / EventBus Adapter / External Bus]
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：自研轻量级 `pkg/outbox`（不引入 Watermill/Debezium）**
  - 原因：项目已使用 `pgx`/自定义 repo 抽象；Watermill 引入的抽象层较重且 schema/tenant 约束不匹配；Debezium 运维复杂度过高。
- **决策 2：每模块独立 outbox 表，结构统一**
  - 原因：减少跨模块耦合，便于未来拆分与独立扩容；同时保证指标与运维口径一致。
- **决策 3：默认单活 relay（advisory lock）**
  - 原因：降低并发竞争与乱序风险；M1 不要求全局顺序，但默认单活可避免“同一 topic/tenant 的并发重放”扩大副作用面。
  - 注意：单活 **不保证顺序**，也 **不消除重复投递**（仍为 at-least-once）；其价值主要在于降低并发重放的副作用面与排障复杂度。
  - 并行化：可在后续按需开放多 worker，但必须在文档中声明“下游不得假设顺序”，并补齐压测与去重策略。
- **决策 4：Dispatcher 是 outbox 的稳定错误边界；与 `pkg/eventbus` 解耦**
  - 017 的 outbox 不依赖 `pkg/eventbus`；模块可以选择：
    - 直接实现 `Dispatcher`（推荐，handlers 返回 `error`，便于 retry）
    - 或使用 `pkg/eventbus` 作为内部广播，但需通过 5.2 的适配层把 panic/handler error 显式化为 `error`，否则 relay 无法安全重试。
  - **补充：Dispatcher 入参必须包含元数据（至少 `event_id/tenant_id/sequence`）**
    - 原因：outbox 的幂等键是 `event_id`；仅传 `topic/payload` 会导致跨进程投递无法稳定设置 message key / headers，进程内 handler 也无法可靠去重与追踪。

## 4. 数据模型与约束 (Data Model & Constraints)
> 标准：必须精确到字段类型、空值约束、索引策略及数据库级约束。

### 4.1 标准表结构（SQL，作为权威口径）
> 表名建议：`<module>_outbox`（例如 `org_outbox`）。字段含义与索引口径必须一致，便于统一 relay/monitoring。

```sql
CREATE TABLE <module>_outbox (
  id           UUID        NOT NULL DEFAULT gen_random_uuid(),
  tenant_id    UUID        NOT NULL,
  topic        TEXT        NOT NULL,
  payload      JSONB       NOT NULL,
  event_id     UUID        NOT NULL, -- 幂等键（业务侧生成，消费者以此去重）
  sequence     BIGSERIAL   NOT NULL, -- 有序游标（用于 claim 顺序与游标化排障）
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NULL,
  attempts     INT         NOT NULL DEFAULT 0,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at    TIMESTAMPTZ NULL,
  last_error   TEXT        NULL,

  CONSTRAINT <module>_outbox_pkey PRIMARY KEY (id),
  CONSTRAINT <module>_outbox_event_id_key UNIQUE (event_id),
  CONSTRAINT <module>_outbox_attempts_nonnegative CHECK (attempts >= 0)
);

-- pending claim：按可用时间 + sequence 取最早事件
CREATE INDEX <module>_outbox_pending_by_available
  ON <module>_outbox (available_at, sequence)
  WHERE published_at IS NULL;

-- cleaner：按 published_at 清理历史记录
CREATE INDEX <module>_outbox_published_by_time
  ON <module>_outbox (published_at, sequence)
  WHERE published_at IS NOT NULL;

-- 观测/排障：按 tenant 定位积压
CREATE INDEX <module>_outbox_tenant_published
  ON <module>_outbox (tenant_id, published_at, sequence);
```

**说明（避免误解）**：
- `UNIQUE (event_id)` 仅保证“outbox 入库幂等 / 避免重复 enqueue”；端到端幂等必须由 relay/dispatcher/消费者 **基于 `event_id` 去重**。
- `sequence` 主要用于游标化排障与定位积压区间，不应作为业务强顺序依赖（顺序语义见 6.2）。

### 4.2 迁移策略 (Migration Strategy)
- **Up**：
  - 创建 `<module>_outbox` 表与以上索引/约束。
  - 若数据库未启用 `gen_random_uuid()`，需确保 `pgcrypto` 可用（可在迁移中 `CREATE EXTENSION IF NOT EXISTS pgcrypto;`）。
- **Down**：
  - 删除 `<module>_outbox` 表（及索引/约束）。
  - 生产环境通常不建议执行破坏性 down；若执行需先确认 outbox 记录已归档/不再用于补发。
- **落地路径（对齐 Atlas+Goose 约定）**：
  - 每个模块把 `<module>_outbox` 纳入自身 schema（例如 Org 的 `modules/org/infrastructure/atlas/schema.hcl`），并通过 atlas diff 生成对应模块的 goose 迁移；命令与目录以该模块 dev-plan 为准（例如 `docs/dev-plans/021-org-schema-and-constraints.md` 的迁移目录约定）。

## 5. 接口契约 (API Contracts)
> outbox 是库级契约，本节用 Go API 作为“契约”。所有接口必须能表达失败并触发重试。

### 5.1 `pkg/outbox` 核心类型与接口（契约）
```go
// Message is the unit stored in <module>_outbox.
type Message struct {
	TenantID uuid.UUID
	Topic    string
	EventID  uuid.UUID
	Payload  json.RawMessage
}

// Meta is the stable dispatch metadata (idempotency + tracing + ops).
type Meta struct {
	Table    pgx.Identifier // e.g. pgx.Identifier{"public", "org_outbox"}
	TenantID uuid.UUID
	Topic    string
	EventID  uuid.UUID
	Sequence int64
	Attempts int

	// Optional tracing context (W3C Trace Context). See 5.4.
	TraceParent string
	TraceState  string
}

// DispatchedMessage is the unit delivered by Relay to Dispatcher.
type DispatchedMessage struct {
	Meta    Meta
	Payload json.RawMessage
}

type Publisher interface {
	Enqueue(ctx context.Context, tx repo.Tx, table pgx.Identifier, msg Message) (sequence int64, err error)
}

type Dispatcher interface {
	Dispatch(ctx context.Context, msg DispatchedMessage) error
}
```

**错误语义（必须明确）**：
- `Enqueue`：
  - 若 `event_id` 冲突（幂等重复 enqueue），必须返回“可识别的幂等结果”（M1：单语句冲突读回并返回 `sequence`，例如 `INSERT ... ON CONFLICT (event_id) DO UPDATE SET event_id = EXCLUDED.event_id RETURNING sequence;`）。
  - `table` 禁止直接字符串拼接；实现必须通过 `table.Sanitize()` 生成安全标识符（避免 identifier 注入与 schema 兼容问题）。
- `Dispatch`：
  - 返回 `nil` 表示该消息可 ack。
  - 返回 `error` 表示该消息需 retry（或最终进入 dead 状态，见 6.2）。
  - `error` 必须可用于排障：实现需在日志/metrics 中带上 `table/topic/event_id/tenant_id/sequence/attempts`（但 metrics 不建议包含 `tenant_id`，见 7/10）。
  - 对外消息总线适配器建议使用 `event_id` 作为 message key，并把 `tenant_id/topic/sequence` 写入 headers/attributes（payload 保持业务 JSON，避免重复与泄露）；若启用 tracing，透传 `traceparent/tracestate`。

### 5.2 Dispatcher 与 `pkg/eventbus` 的集成决策（M1 必须定死）
**决策**：Outbox 的 dispatcher 作为“错误边界”必须返回 `error`；若模块选择继续使用 `pkg/eventbus` 做进程内广播，必须提供一个“可返回 error 的适配层”。

- **推荐方案（M1）**：为 `pkg/eventbus` 增加一个可选能力接口（不强制所有调用方迁移）：
  - 新增接口：
    - `type EventBusWithError interface { eventbus.EventBus; PublishE(args ...any) error }`
  - `publisherImpl` 实现 `PublishE`：
    - 对所有匹配的 subscriber 逐个执行：
      - 捕获 panic → 视为 `error`
      - 若 handler 返回单个 `error` 且非 nil → 视为 `error`
    - 多个错误用 `errors.Join` 汇总。
    - 若无任何匹配 subscriber：返回 `ErrNoSubscribers`（作为配置错误，避免 outbox 静默吞消息）。
    - 若 handler 返回值不为“0 个”或“1 个 error”：返回 `ErrInvalidHandlerReturn`（fail-fast，避免 outbox 假成功）。
  - 错误类型建议使用 `pkg/serrors` 定义稳定 code（例如 `EVENTBUS_NO_SUBSCRIBERS`、`EVENTBUS_INVALID_HANDLER_RETURN`），并允许 wrap 具体原因用于排障。
- **在 Org 的落地方式（示意）**：
  - `org.Dispatcher`：按 `msg.Meta.Topic` 反序列化 `msg.Payload` -> 调用 `EventBusWithError.PublishE(&msg.Meta, &OrgChangedEvent{...})`，将 handler 错误与 panic 显式上抛，交由 outbox retry。
  - outbox 事件的进程内 handler 如需幂等/追踪，应订阅 `func(meta *outbox.Meta, e *OrgChangedEvent) error`（或在事件结构体内显式包含 `EventID`），禁止“只发业务事件不带 event_id”。

### 5.3 Topic 约定与版本策略（M1 必须定死）
> topic 是 outbox 的“稳定路由键”。它既用于进程内分发（dispatcher/handlers），也用于未来对外总线适配（Kafka/SQS/HTTP 等）。

- **命名规则**：
  - 全小写，使用 `.` 分隔层级：`<module>.<aggregate>.<event>.vN`
  - 允许字符集建议限定为：`[a-z0-9.-]`（禁止空格与大写），长度建议 `< 128`。
- **版本策略**：
  - **breaking change**（字段语义变化/删除字段/同字段类型变化/无法向后兼容）必须 bump `vN`（例如 `...v1` -> `...v2`）。
  - **non-breaking change**（新增可选字段）保持版本不变。
  - 版本迁移期允许同时发布 `v1` 与 `v2`，直至所有消费者迁移完成；relay 与 outbox 不负责跨版本兼容。
- **示例（Org）**：
  - `org.organization.created.v1`
  - `org.organization.updated.v1`
  - `org.organization.deleted.v1`

### 5.4 Tracing 传递（可选增强 / M2）
> M1 的 outbox 仍可先不做链路追踪；但为了后续能串起“业务事务 -> outbox -> relay -> handler/外部总线”的完整链路，建议预留 tracing 扩展点。

- **标准**：使用 W3C Trace Context（`traceparent`/`tracestate`），避免自定义 TraceID/SpanID 语义漂移。
- **持久化方式（推荐）**：为 `<module>_outbox` 增加可选列（M2）：
  ```sql
  ALTER TABLE <module>_outbox
    ADD COLUMN traceparent TEXT NULL,
    ADD COLUMN tracestate  TEXT NULL;
  ```
  - outbox 不要求为 tracing 字段建索引。
  - `traceparent/tracestate` 为空表示“无可继承链路”。
- **透传语义**：
  - Enqueue：从 `ctx` 提取当前 trace context，写入 outbox 行（若列存在）。
  - Dispatch：relay 从行读取 `traceparent/tracestate` 并填入 `Meta.TraceParent/TraceState`；是否创建 span 由 dispatcher/上层决定（避免 `pkg/outbox` 引入额外依赖）。
  - External bus：若投递到外部消息系统，建议把 `traceparent/tracestate` 写入 headers/attributes。

### 5.5 Payload 与 JSON 约定（建议）
> outbox 把 `payload` 视为“opaque JSON”，不做重写；事件 schema 的约束与演进由 topic 版本管理（见 5.3）。

- **建议**：
  - 生产者/消费者统一使用稳定 JSON 语义（推荐 Go 标准库 `encoding/json`），避免同字段在不同库下出现不一致编码。
  - `time.Time` 等时间字段建议使用 UTC 的 RFC3339Nano 字符串表示，避免时区歧义。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 Enqueue（事务内写入）
1. 业务 service 打开事务。
2. 在同一 `tx` 内完成业务写入。
3. 在同一 `tx` 内调用 `Publisher.Enqueue(..., table=pgx.Identifier{"<schema>", "<module>_outbox"}, msg)` 写入 outbox（默认 schema 可用 `public`）。
4. 提交事务：确保“业务写入 + outbox 入库”原子一致。

### 6.2 Relay（claim/dispatch/ack，短事务 + lease）
> 目标：多实例安全、可重试、可恢复；允许重复投递（At-least-once）。

- **默认参数（M1）**：
  - `batch_size=100`
  - `poll_interval=1s`
  - `lock_ttl=60s`
  - `max_attempts=25`
  - `backoff(attempts)=min(1s*2^(attempts-1), 60s) + jitter(0~200ms)`

#### 顺序语义（必须声明）
- relay **不提供顺序保证**（包括单活模式）：
  - claim 仅保证“本轮取数”按 `available_at, sequence` 排序；一旦发生失败/backoff，后续消息可能先于失败消息被投递（重排）。
  - 若关闭单活并启用并发 worker，则更不保证顺序。
  - `sequence` 的主要用途是：游标化排障、定位积压区间与人工补发，不可作为业务强顺序依赖。

#### claim（抢占，短事务）
- 条件（必须包含）：
  - `published_at IS NULL`
  - `available_at <= now()`
  - `attempts < max_attempts`
  - `(locked_at IS NULL OR locked_at < now() - lock_ttl)`
- SQL（语义示意，具体参数化以实现为准）：
  - `SELECT ... ORDER BY available_at, sequence LIMIT $batch_size FOR UPDATE SKIP LOCKED`
  - 同事务内 `UPDATE ... SET locked_at=now(), attempts=attempts+1 WHERE id IN (...)`

#### dispatch（事务外）
- 对每条记录调用 `Dispatcher.Dispatch(ctx, msg)`（msg 包含 `Meta + Payload`）。
- 单条消息失败 **不得中断** 本轮 batch；必须继续处理剩余消息，并对每条消息分别执行 ack/nack（避免 head-of-line blocking）。
- 记录 `dispatch_latency` 与结果（success/failure）。

#### ack / nack（短事务）
- 成功（ack）：
  - `published_at=now(), locked_at=NULL, last_error=NULL`
- 失败（nack）：
  - 若 `attempts < max_attempts`：`locked_at=NULL, last_error=<err string>, available_at=now()+backoff(attempts)`
  - 若 `attempts >= max_attempts`：进入 **dead** 状态（不再自动重试），`locked_at=NULL, last_error=<err string>, available_at=now()`
- 达到 `max_attempts`：
  - relay 不再 claim（通过 `attempts < max_attempts` 条件自然停止），记录将长期保留用于排障/补发。
  - 指标：当消息首次进入 dead（`attempts >= max_attempts` 且本次 dispatch 失败）时，必须递增 `outbox_dead_total{table,topic}`。

#### 多实例（单活）
- 默认使用 **session-level** `pg_try_advisory_lock` 实现“每表单活 relay”（覆盖 claim + dispatch 全过程，确保同一时刻仅一个实例在投递该表）：
  - lock key 建议：由 Go 侧生成稳定 `int64`（例如 `fnv64a("outbox:"+table)`），并使用 `pg_try_advisory_lock($1::bigint)`；避免依赖 DB 内部 hash 且降低冲突风险。
  - 实现约束：session lock 必须绑定到同一条连接；relay 需要从 `pgxpool` 长期 `Acquire` 连接并持有到退出（每张 outbox 表 1 条连接）。
  - 未拿到锁的实例：不执行 claim/dispatch（sleep 进入下一 tick），作为 standby。
  - 可配置降级：允许关闭单活（`OUTBOX_RELAY_SINGLE_ACTIVE=false`）仅依赖 `SKIP LOCKED` 做并发安全；此时必须在文档中声明“下游不得假设顺序”，并补齐压测与幂等策略。

## 7. 安全与鉴权 (Security & Authz)
- outbox 为服务端内部机制，不对外暴露任何写入/ack 接口。
- **租户隔离**：
  - 表结构包含 `tenant_id`，用于审计与排障（并非 access control）。
  - relay 处理全租户消息；日志必须包含 `tenant_id`（允许配置脱敏），指标避免 `tenant_id`（防止高基数）。
- **敏感信息**：
  - `payload` 可能包含敏感字段；`last_error` 不得写入完整 payload，只记录错误摘要（避免泄露）。
  - `last_error` 必须截断（建议 2KB 以内）并去除明显敏感信息；日志同理。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `pkg/repo`：`Tx` 抽象（`Query/Exec/SendBatch` 等）。
  - `pkg/composables`：事务/连接池注入模式（便于业务侧在同 tx enqueue）。
  - `pkg/serrors`：用于定义可识别的稳定错误 code（outbox/eventbus 的配置/契约错误）。
  - Prometheus client：指标实现参考 `pkg/authz/metrics.go`。
  - `pkg/eventbus`：仅当模块选择 eventbus 作为 dispatcher 落点时需要（见 5.2）。
- **里程碑**：
  1. [X] `pkg/outbox`：Publisher + Relay + Cleaner（含 Options 与默认值）。
  2. [X] `pkg/eventbus`：补齐 `PublishE` 与 `ErrNoSubscribers/ErrInvalidHandlerReturn`（仅新增能力，不破坏原 Publish 用法）。
  3. [ ] 集成测试：原子性、并发 claim、lease 过期可恢复、重试退避、dead-letter 行为（已补齐可选 `integration` 用例，待配置真实 PG 运行）。
  4. [ ] 文档与示例：接入指南与 `docs/dev-records/DEV-PLAN-017-READINESS.md` 完整记录（已更新 readiness 模板，待补“模块接入指南/示例”）。
  5. [ ] （可选 / M2）Tracing：持久化/透传 `traceparent/tracestate`（见 5.4）。
  6. [ ] （可选）开发调试脚本：封装 10.4 SQL（必须含安全保护，见 10.4）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **单元测试**：
  - backoff/jitter 逻辑稳定可预测（可注入随机源）。
  - `PublishE`：无订阅者/handler panic/handler 返回 error 的行为符合 5.2。
  - `PublishE`：handler 返回值非法时返回 `ErrInvalidHandlerReturn`（避免假成功）。
- **集成测试（真实 PG）**：
  - 事务原子性：业务回滚时 outbox 不产生残留。
  - 并发 claim：两个 worker 并发拉取同一表，不产生重复 claim（SKIP LOCKED 生效）。
  - lease 恢复：worker claim 后崩溃/不 ack，超过 `lock_ttl` 后可被其他 worker 重新 claim。
  - 重试与 dead：失败会退避重试；达到 `max_attempts` 后不再重试但记录可查询。
  - 毒丸消息：单条消息持续失败（panic / 超时 / 返回 error）不会阻塞后续消息投递；达到 `max_attempts` 后进入 dead，后续消息仍持续被投递（无 head-of-line blocking）。
  - Tracing（可选 / M2）：若 outbox 表包含 `traceparent/tracestate` 并在 enqueue 时写入，relay dispatch 时 `Meta.TraceParent/TraceState` 必须原样透传。
  - Cleaner：默认仅清理 `published_at < now()-retention` 的记录，不影响未发布与近期发布；若启用 dead retention，则仅额外清理“dead 且超过 dead_retention”的记录。
  - 单活：两实例同表运行时，同一时刻仅一个实例会执行 dispatch（advisory lock 生效）。
  - 安全：`last_error` 截断/脱敏生效，不会写入 payload。
- **门禁**：
  - `go fmt ./... && go vet ./... && make check lint && make test` 全通过。
- **Readiness**：
  - 将命令、耗时、结果记录到 `docs/dev-records/DEV-PLAN-017-READINESS.md`。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 配置开关（建议）
- `OUTBOX_RELAY_ENABLED=true|false`（默认 true）
- `OUTBOX_RELAY_TABLES=public.org_outbox,public.hrm_outbox`（默认空：不启动任何 relay）
- `OUTBOX_RELAY_SINGLE_ACTIVE=true|false`（默认 true）
- `OUTBOX_RELAY_POLL_INTERVAL=1s`
- `OUTBOX_RELAY_BATCH_SIZE=100`
- `OUTBOX_RELAY_LOCK_TTL=60s`
- `OUTBOX_RELAY_MAX_ATTEMPTS=25`
- `OUTBOX_RELAY_DISPATCH_TIMEOUT=30s`（默认 30s）
- `OUTBOX_LAST_ERROR_MAX_BYTES=2048`（默认 2048）
- `OUTBOX_CLEANER_ENABLED=true|false`（默认 true）
- `OUTBOX_CLEANER_TABLES=public.org_outbox,public.hrm_outbox`（默认空：跟随 `OUTBOX_RELAY_TABLES`）
- `OUTBOX_CLEANER_INTERVAL=1m`（默认 1m）
- `OUTBOX_CLEANER_RETENTION=168h`（默认 7 天）
- `OUTBOX_CLEANER_DEAD_RETENTION=0|168h`（默认 0：不清理 dead；设置后按 `created_at` 清理 dead）

### 10.2 指标（Prometheus）
建议命名风格参考 `pkg/authz/metrics.go`：
- Counter：
  - `outbox_enqueue_total{table,topic}`
  - `outbox_dispatch_total{table,topic,result}`（result=success|failure）
  - `outbox_dead_total{table,topic}`（消息首次进入 dead）
- Histogram：
  - `outbox_dispatch_latency_seconds{table,topic,result}`
- Gauge：
  - `outbox_pending{table}`（`published_at IS NULL`）
  - `outbox_locked{table}`（`locked_at IS NOT NULL AND published_at IS NULL`）
  - `outbox_relay_leader{table}`（单活：当前实例是否持有表级 leader 锁，1/0）
> 指标 labels 必须保持低基数：仅允许 `{table,topic,result}` 等维度，禁止加入 `tenant_id/event_id/sequence`。

**暴露方式（实现约定）**：
- 若启用 `PROMETHEUS_METRICS_ENABLED=true`，服务将暴露 `PROMETHEUS_METRICS_PATH`（默认 `/debug/prometheus`）用于 Prometheus 抓取。

### 10.3 回滚/降级
- 代码级：关闭 `OUTBOX_RELAY_ENABLED`，停止投递但保留 outbox 数据，便于后续补发。
- 数据级：不建议删除未发布数据；若必须清理，需按表/tenant/time window 明确范围并保留审计记录。

### 10.4 排障与重放（Runbook 口径，M1 必须写清）
> outbox 默认为 at-least-once；任何重放都可能产生重复副作用，必须确保消费者幂等（以 `event_id` 为键）。

- **查询积压**：
  ```sql
  SELECT sequence, event_id, topic, tenant_id, attempts, available_at, locked_at, last_error
    FROM <module>_outbox
   WHERE published_at IS NULL
   ORDER BY available_at, sequence
   LIMIT 100;
  ```
- **查询 dead（达到上限仍未发布）**：
  ```sql
  SELECT sequence, event_id, topic, tenant_id, attempts, available_at, last_error
    FROM <module>_outbox
   WHERE published_at IS NULL AND attempts >= $1
   ORDER BY sequence
   LIMIT 100;
  ```
- **重置并重放单条（谨慎）**：
  ```sql
  UPDATE <module>_outbox
     SET attempts = 0,
         available_at = now(),
         locked_at = NULL,
         last_error = NULL
   WHERE event_id = $1;
  ```

- **开发调试便捷命令（可选）**：
  - 可将 10.4 的 SQL 封装为 `scripts/db/outbox_*` 或 `make db outbox-*` 目标，提升本地调试效率。
  - 安全要求（必须满足，否则禁止提供该命令）：
    - 仅允许在本地/开发环境执行（强校验环境变量或 DSN host，避免误伤生产/预发）。
    - 必须显式传入目标表与 `event_id`（禁止“全表清空/批量重置”的默认行为）。
    - 执行前输出将要变更的 SQL 与影响行数预览，且要求二次确认（例如 `CONFIRM=1`）。

### 10.5 单活资源与可用性（M1 必须写清）
- **连接资源**：
  - session-level advisory lock 绑定连接；当 `OUTBOX_RELAY_SINGLE_ACTIVE=true` 时，relay 需要对每张 outbox 表长期持有 1 条连接（standby 实例也可能在竞争锁时短暂占用连接）。
  - 需要在部署说明中预留连接池容量（避免 relay 抢占业务查询连接导致级联超时）。
- **故障切换语义**：
  - leader 进程崩溃或连接断开时锁会释放，其他实例可在下一 tick 接管。
  - 若 leader 进程“卡死但连接不断开”，则锁不会自动释放；必须依赖应用健康检查/超时控制触发重启（建议：dispatch 侧对外部 IO 设置 deadline，并以 `outbox_relay_leader` + pending/locked 指标监控卡死）。

## 11. 实施进度（登记）
- [X] 落地 `pkg/outbox`（Publisher/Relay/Cleaner + metrics + 表名解析）
- [X] 落地 `pkg/eventbus.PublishE`（panic/handler error 显式化，支持 outbox retry）
- [X] Server 接入：通过 `OUTBOX_RELAY_TABLES/OUTBOX_CLEANER_TABLES` 启动后台 relay/cleaner
- [X] Metrics 接入：可选暴露 Prometheus 抓取端点（`PROMETHEUS_METRICS_ENABLED`）
- [X] 门禁：`go fmt ./... && go vet ./... && make check lint && make test`
- [X] PR：`feature/dev-plan-017` -> `main`（https://github.com/jacksonlee411/Bugs-Blossoms/pull/43，`95968402`）
- [ ] 待落地：选择首个模块创建 `<module>_outbox` 迁移，并在业务事务内 `Enqueue`（契约落地样例）
- [ ] 待验证：配置 `OUTBOX_TEST_DSN` 运行 `integration` 集成测试，并把结果登记到 `docs/dev-records/DEV-PLAN-017-READINESS.md`
