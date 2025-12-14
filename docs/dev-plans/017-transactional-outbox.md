# DEV-PLAN-017：事务性发件箱 (Transactional Outbox) 工具链

**状态**: 规划中（2025-01-15 15:00 UTC）

## 背景
- **DEV-PLAN-009** 明确了 ERP 系统需引入 Transactional Outbox 模式，以解决“数据库写入与事件发布”的原子性问题，保证最终一致性。
- **DEV-PLAN-026** 计划在 Org 模块率先落地 `org_outbox` + relay 作为首个试点；若各模块各自实现表结构与 relay，将导致代码重复、监控分散且行为不一致。
- 现有 `pkg/eventbus` 为进程内发布（无 topic、无返回错误），不适合作为 outbox relay 的稳定投递契约；outbox 需要一个可返回 `error` 的投递边界，由应用侧适配到 `eventbus` 或外部总线。

## 目标
- 交付通用的 `pkg/outbox` 库，包含：
  - **Publisher**：在同一数据库事务内写入 outbox 记录（与业务写入同 Tx）。
  - **Relay**：以短事务方式抢占 pending 事件并投递到 `Dispatcher`，支持重试与幂等。
  - **Cleaner**：按保留策略清理已发布历史事件（失败事件保留供排障/补偿）。
- 标准化 outbox 表结构与查询口径（与 `DEV-PLAN-026` 对齐），支持“按模块独立表，结构统一”。
- 明确语义：**At-least-once** 投递；以 `event_id` 为幂等键，消费者必须幂等。

## 非目标（M1）
- 不引入 Debezium/Kafka/Connect 等 CDC 基础设施。
- 不承诺 Exactly-once；不在 relay 内做“跨消费者”的去重副作用。
- 不强制所有模块共用同一张 outbox 表（仍支持 `<module>_outbox`）。

## 技术选型建议

### 选项 A：Watermill (SQL Outbox)
- **优点**: 社区成熟，功能丰富，支持多种 Pub/Sub 后端（Kafka, AMQP, Go Channel），标准化程度高。
- **缺点**: 引入较多抽象层和依赖；默认 Schema 可能不完全符合项目规范（如 UUID/Tenant 隔离）；对 `pgx` 的原生事务集成可能需要适配器。

### 选项 B：Debezium (CDC)
- **优点**: 对应用代码无侵入，基于 WAL 日志，性能极高。
- **缺点**: 运维复杂度高（需 Kafka/Connect），不适合当前“模块化单体 + Docker Compose”的轻量级开发环境。

### 选项 C：自研轻量级 `pkg/outbox` (推荐)
- **优点**:
  - **极致轻量**: 仅依赖 `pgx/v5`，无额外第三方包袱。
  - **Postgres 深度优化**: 直接利用 `FOR UPDATE SKIP LOCKED` 实现高并发、无锁竞争的 Relay，适合多实例部署。
  - **架构契合**: 可直接复用现有的 `pkg/repo.Tx` + `pkg/composables.InTx` 事务模式，符合 Clean Architecture。
  - **可控性**: Schema 可完全按需定制（如包含 `tenant_id`, `trace_id`）。
- **决策**: 采用 **选项 C**。以 `DEV-PLAN-026` 的落地需求作为首个对齐场景，沉淀为通用库。

## 范围
- **核心库 (`pkg/outbox`)**：
  - `Publisher`：`Enqueue(ctx, tx, table, msg)`（基于现有 `pkg/repo.Tx` 抽象，不引入新的 `pkg/db` 包）。
  - `Relay`：claim/dispatch/ack（`FOR UPDATE SKIP LOCKED` + lease），投递目标为 `Dispatcher`（返回 `error`）。
  - `Cleaner`：仅清理已发布且超过保留期的历史事件。
- **Schema 规范**：提供 Atlas HCL 模板与索引建议，模块独立表、结构统一。
- **监控指标**：Prometheus 指标（建议 `outbox_enqueue_total/outbox_relay_dispatch_total/outbox_relay_latency_seconds/outbox_pending_count` 等）。

## 依赖与里程碑
- **依赖**:
  - `pkg/repo`: 提供统一的 `Tx` 抽象。
  - `pkg/composables`: 提供 Tx/Pool 的上下文注入与事务工具（如 `InTx`）。
  - Prometheus client（可参考 `pkg/authz/metrics.go` 的命名风格）。
- **里程碑**:
  - M1: `pkg/outbox` 核心代码（Publisher + Relay）完成。
  - M2: 集成测试（基于现有 ITF/PG 环境）验证原子性、并发抢占与重试语义（允许重复）。
  - M3: 文档与示例代码，优先指导 Org 模块（026）接入。

## 详细设计

### 1. 幂等与顺序
- 语义：**At-least-once** 投递，允许重复；消费者必须以 `event_id` 幂等处理。
- `event_id`：业务幂等键，DB 层 `unique`；重复 enqueue 应通过“冲突即忽略/读回”实现幂等。
- `sequence`：数据库生成的有序游标；顺序只在“单 worker / 单分区”条件下可被假设。

### 2. 接口边界（避免绑定 `pkg/eventbus`）
- 数据库写入使用现有抽象 `pkg/repo.Tx`。
- relay 只依赖可返回错误的 `Dispatcher`；是否投递到 `eventbus`、HTTP webhook、消息队列由应用侧适配。

```go
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

### 3. 标准表结构 (Atlas HCL)
建议各模块独立建表（如 `org_outbox`, `hrm_outbox`）以保持微服务演进能力，但结构统一（与 `DEV-PLAN-026` 对齐）：
```hcl
table "module_outbox" {
  column "id" { type = uuid, default = sql("gen_random_uuid()") }
  column "tenant_id" { type = uuid }
  column "topic" { type = text }
  column "payload" { type = jsonb }
  column "event_id" { type = uuid } // 业务幂等键
  column "sequence" { type = bigserial }
  column "created_at" { type = timestamptz, default = sql("now()") }
  column "published_at" { type = timestamptz, null = true }
  column "attempts" { type = int, default = 0 }
  column "available_at" { type = timestamptz, default = sql("now()") }
  column "last_error" { type = text, null = true }
  column "locked_at" { type = timestamptz, null = true }
  
  primary_key { columns = [column.id] }
}
```
- 约束：必须有 `unique(event_id)`；推荐 `tenant_id/topic/payload/event_id/sequence` 为 `not null`（与业务语义一致）。
- 索引建议：`(published_at, sequence)`、`(tenant_id, published_at, sequence)`、`(available_at, sequence) where published_at is null`。

### 4. Relay 机制（短事务 + lease）
- **轮询**：`time.Ticker` 驱动，batch 拉取。
- **claim（抢占）**：短事务内拉取并标记：
  - 条件：`published_at is null AND available_at <= now() AND (locked_at is null OR locked_at < now()-lock_ttl)`
  - `ORDER BY sequence LIMIT $batch_size FOR UPDATE SKIP LOCKED`
  - 更新：`locked_at=now(), attempts=attempts+1`
- **dispatch（投递）**：事务外调用 `Dispatcher.Dispatch`。
- **ack（确认）**：
  - 成功：`published_at=now(), locked_at=null, last_error=null`
  - 失败：`locked_at=null, last_error=$err, available_at=now()+backoff(attempts)`；超过 `max_attempts` 时停止重试并保留记录（可由运维/脚本处理）。
- **多实例**：为保证顺序性与避免多 relay 并发，可用 `pg_try_advisory_lock` 做“单活 relay”；若放开并行化，则必须声明“下游不得假设全局顺序”。

## 实施步骤
1. [ ] **核心库开发**: 创建 `pkg/outbox`，实现 `Publisher` 接口与基于 `pgx` 的存储实现。
2. [ ] **Relay 实现**: 编写基于 `SKIP LOCKED` 的 Relay Worker，支持优雅停止和错误重试。
3. [ ] **Schema 定义**: 在 `pkg/outbox/schema` 或文档中提供标准 Atlas HCL 模板。
4. [ ] **测试验证**: 编写集成测试，模拟高并发下的 enqueue 与 relay 抢占，确保不丢失、允许重复（At-least-once）且可重放。
5. [ ] **文档**: 更新 `README.md` 和 `docs/dev-records`，提供“如何为新模块添加 Outbox”的指南（含幂等消费与排障/补发说明）。

## 交付物
- `pkg/outbox` Go 包。
- 标准 Outbox Schema 定义。
- 集成测试套件。
- 接入指南文档。
