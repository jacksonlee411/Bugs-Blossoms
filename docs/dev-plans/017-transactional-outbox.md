# DEV-PLAN-017：事务性发件箱 (Transactional Outbox) 工具链

**状态**: 规划中（2025-12-14 09:04 UTC）

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
  - [ ] 交付通用 `pkg/outbox`（Publisher/Relay/Cleaner），仅依赖 `pgx/v5` 与现有 `pkg/repo.Tx` 抽象。
  - [ ] 标准化 outbox 表结构与查询口径：支持“按模块独立表，结构统一”（如 `org_outbox`、`hrm_outbox`）。
  - [ ] 明确投递语义：**At-least-once**；以 `event_id` 为幂等键，允许重复投递，消费者必须幂等。
  - [ ] 明确 dispatcher 契约（必须返回 `error`），并给出与 `pkg/eventbus` 的集成决策（见 5.2）。
  - [ ] 提供 Prometheus 指标、日志字段与运维开关建议（见 10）。
  - [ ] 通过 `go fmt ./... && go vet ./... && make check lint && make test`。
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
  C -->|Dispatch(topic,payload)| D[Dispatcher (error boundary)]
  D --> E[In-proc Handlers / EventBus Adapter / External Bus]
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：自研轻量级 `pkg/outbox`（不引入 Watermill/Debezium）**
  - 原因：项目已使用 `pgx`/自定义 repo 抽象；Watermill 引入的抽象层较重且 schema/tenant 约束不匹配；Debezium 运维复杂度过高。
- **决策 2：每模块独立 outbox 表，结构统一**
  - 原因：减少跨模块耦合，便于未来拆分与独立扩容；同时保证指标与运维口径一致。
- **决策 3：默认单活 relay（advisory lock）**
  - 原因：降低并发竞争与乱序风险；M1 不要求全局顺序，但默认单活可避免“同一 topic/tenant 的并发重放”扩大副作用面。
  - 并行化：可在后续按需开放多 worker，但必须在文档中声明“下游不得假设顺序”，并补齐压测与去重策略。
- **决策 4：Dispatcher 是 outbox 的稳定错误边界；与 `pkg/eventbus` 解耦**
  - 017 的 outbox 不依赖 `pkg/eventbus`；模块可以选择：
    - 直接实现 `Dispatcher`（推荐，handlers 返回 `error`，便于 retry）
    - 或使用 `pkg/eventbus` 作为内部广播，但需通过 5.2 的适配层把 panic/handler error 显式化为 `error`，否则 relay 无法安全重试。

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

type Publisher interface {
	Enqueue(ctx context.Context, tx repo.Tx, table string, msg Message) (sequence int64, err error)
}

type Dispatcher interface {
	Dispatch(ctx context.Context, topic string, payload json.RawMessage) error
}
```

**错误语义（必须明确）**：
- `Enqueue`：
  - 若 `event_id` 冲突（幂等重复 enqueue），必须返回“可识别的幂等结果”（建议：返回已存在记录的 `sequence`，或返回 `ErrDuplicateEventID` 并由调用方决定忽略/读回；M1 建议“冲突读回并返回 sequence”）。
- `Dispatch`：
  - 返回 `nil` 表示该消息可 ack。
  - 返回 `error` 表示该消息需 retry（或最终进入 dead 状态，见 6.2）。

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
- **在 Org 的落地方式（示意）**：
  - `org.Dispatcher`：按 topic 反序列化 payload -> 调用 `EventBusWithError.PublishE(&OrgChangedEvent{...})`，将 handler 错误与 panic 显式上抛，交由 outbox retry。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 Enqueue（事务内写入）
1. 业务 service 打开事务。
2. 在同一 `tx` 内完成业务写入。
3. 在同一 `tx` 内调用 `Publisher.Enqueue(..., table="<module>_outbox", msg)` 写入 outbox。
4. 提交事务：确保“业务写入 + outbox 入库”原子一致。

### 6.2 Relay（claim/dispatch/ack，短事务 + lease）
> 目标：多实例安全、可重试、可恢复；允许重复投递（At-least-once）。

- **默认参数（M1）**：
  - `batch_size=100`
  - `poll_interval=1s`
  - `lock_ttl=60s`
  - `max_attempts=25`
  - `backoff(attempts)=min(1s*2^(attempts-1), 60s) + jitter(0~200ms)`

#### claim（抢占，短事务）
- 条件（必须包含）：
  - `published_at IS NULL`
  - `available_at <= now()`
  - `attempts < max_attempts`
  - `(locked_at IS NULL OR locked_at < now() - lock_ttl)`
- SQL（语义示意，具体参数化以实现为准）：
  - `SELECT ... ORDER BY sequence LIMIT $batch_size FOR UPDATE SKIP LOCKED`
  - 同事务内 `UPDATE ... SET locked_at=now(), attempts=attempts+1 WHERE id IN (...)`

#### dispatch（事务外）
- 对每条记录调用 `Dispatcher.Dispatch(topic,payload)`。
- 记录 `dispatch_latency` 与结果（success/failure）。

#### ack / nack（短事务）
- 成功（ack）：
  - `published_at=now(), locked_at=NULL, last_error=NULL`
- 失败（nack）：
  - `locked_at=NULL, last_error=<err string>, available_at=now()+backoff(attempts)`
- 达到 `max_attempts`：
  - relay 不再 claim（通过 `attempts < max_attempts` 条件自然停止），记录将长期保留用于排障/补发。

#### 多实例（单活）
- 默认使用 `pg_try_advisory_lock` 实现“每表单活 relay”：
  - lock key 建议：`hashtext('outbox:' || table_name)`
  - 未拿到锁的实例：不执行 claim（sleep 进入下一 tick），避免并发重放扩大副作用面。

## 7. 安全与鉴权 (Security & Authz)
- outbox 为服务端内部机制，不对外暴露任何写入/ack 接口。
- **租户隔离**：
  - 表结构包含 `tenant_id`，用于审计与排障（并非 access control）。
  - relay 处理全租户消息，但日志/指标必须包含 `tenant_id`（或允许配置脱敏）。
- **敏感信息**：
  - `payload` 可能包含敏感字段；`last_error` 不得写入完整 payload，只记录错误摘要（避免泄露）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `pkg/repo`：`Tx` 抽象（`Query/Exec/SendBatch` 等）。
  - `pkg/composables`：事务/连接池注入模式（便于业务侧在同 tx enqueue）。
  - Prometheus client：指标实现参考 `pkg/authz/metrics.go`。
  - `pkg/eventbus`：仅当模块选择 eventbus 作为 dispatcher 落点时需要（见 5.2）。
- **里程碑**：
  1. [ ] `pkg/outbox`：Publisher + Relay + Cleaner（含 Options 与默认值）。
  2. [ ] `pkg/eventbus`：补齐 `PublishE` 与 `ErrNoSubscribers`（仅新增能力，不破坏原 Publish 用法）。
  3. [ ] 集成测试：原子性、并发 claim、lease 过期可恢复、重试退避、dead-letter 行为。
  4. [ ] 文档与示例：接入指南与 `docs/dev-records/DEV-PLAN-017-READINESS.md` 完整记录。

## 9. 测试与验收标准 (Acceptance Criteria)
- **单元测试**：
  - backoff/jitter 逻辑稳定可预测（可注入随机源）。
  - `PublishE`：无订阅者/handler panic/handler 返回 error 的行为符合 5.2。
- **集成测试（真实 PG）**：
  - 事务原子性：业务回滚时 outbox 不产生残留。
  - 并发 claim：两个 worker 并发拉取同一表，不产生重复 claim（SKIP LOCKED 生效）。
  - lease 恢复：worker claim 后崩溃/不 ack，超过 `lock_ttl` 后可被其他 worker 重新 claim。
  - 重试与 dead：失败会退避重试；达到 `max_attempts` 后不再重试但记录可查询。
  - Cleaner：仅清理 `published_at < now()-retention` 的记录，不影响未发布与近期发布。
- **门禁**：
  - `go fmt ./... && go vet ./... && make check lint && make test` 全通过。
- **Readiness**：
  - 将命令、耗时、结果记录到 `docs/dev-records/DEV-PLAN-017-READINESS.md`。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 配置开关（建议）
- `OUTBOX_RELAY_ENABLED=true|false`（默认 true）
- `OUTBOX_RELAY_POLL_INTERVAL=1s`
- `OUTBOX_RELAY_BATCH_SIZE=100`
- `OUTBOX_RELAY_LOCK_TTL=60s`
- `OUTBOX_RELAY_MAX_ATTEMPTS=25`
- `OUTBOX_CLEANER_ENABLED=true|false`（默认 true）
- `OUTBOX_CLEANER_RETENTION=168h`（默认 7 天）

### 10.2 指标（Prometheus）
建议命名风格参考 `pkg/authz/metrics.go`：
- Counter：
  - `outbox_enqueue_total{table,topic}`
  - `outbox_dispatch_total{table,topic,result}`（result=success|failure）
  - `outbox_dead_total{table,topic}`（attempts 达到上限）
- Histogram：
  - `outbox_dispatch_latency_seconds{table,topic,result}`
- Gauge：
  - `outbox_pending_count{table}`（`published_at IS NULL`）
  - `outbox_locked_count{table}`（`locked_at IS NOT NULL AND published_at IS NULL`）

### 10.3 回滚/降级
- 代码级：关闭 `OUTBOX_RELAY_ENABLED`，停止投递但保留 outbox 数据，便于后续补发。
- 数据级：不建议删除未发布数据；若必须清理，需按表/tenant/time window 明确范围并保留审计记录。
