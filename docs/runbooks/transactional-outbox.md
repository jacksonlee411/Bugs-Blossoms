# Transactional Outbox（DEV-PLAN-017）Runbook

本手册用于 **启用/运维/排障** `pkg/outbox`（Transactional Outbox）工具链：relay（投递）、cleaner（清理）、Prometheus 指标与常用排障 SQL。

契约与实现细节见：
- 计划（SSOT）：`docs/dev-plans/017-transactional-outbox.md`
- Readiness 记录：`docs/dev-records/DEV-PLAN-017-READINESS.md`
- 示例环境变量：`.env.example`

## 1. 启用方式（配置项）

outbox 后台任务通过服务启动时的环境变量控制（见 `.env.example`）。

- **是否启用 relay**：`OUTBOX_RELAY_ENABLED=true|false`
- **要投递的表**：`OUTBOX_RELAY_TABLES=public.org_outbox,public.hrm_outbox`
  - 为空时不会启动任何 relay（即使 `OUTBOX_RELAY_ENABLED=true`）。
- **是否单活**：`OUTBOX_RELAY_SINGLE_ACTIVE=true|false`
  - `true` 时使用 Postgres advisory lock 做“每表单活”，并会为每张 outbox 表长期占用 1 条连接（见 5）。
- **cleaner**：
  - `OUTBOX_CLEANER_ENABLED=true|false`
  - `OUTBOX_CLEANER_TABLES=...`（为空时默认跟随 `OUTBOX_RELAY_TABLES`）
  - `OUTBOX_CLEANER_RETENTION=168h`（清理已发布记录的保留期）
  - `OUTBOX_CLEANER_DEAD_RETENTION=0|168h`（默认 0：不清理 dead；设置后按 `created_at` 清理）

## 2. 指标与观测（Prometheus + 日志）

### 2.1 启用 Prometheus 抓取端点

- `PROMETHEUS_METRICS_ENABLED=true`
- `PROMETHEUS_METRICS_PATH=/debug/prometheus`（默认值）

### 2.2 核心指标（建议口径）

outbox 指标命名与维度对齐 `docs/dev-plans/017-transactional-outbox.md` 的 10.2 节；重点关注：
- `outbox_pending{table}`
- `outbox_dispatch_total{table,topic,result}`
- `outbox_dead_total{table,topic}`
- `outbox_relay_leader{table}`（单活模式）

### 2.3 日志字段

服务端默认使用 `component=outbox` 日志域，常见字段包含 `table/topic/event_id/tenant_id/sequence/attempts`（以实际实现为准）。

## 3. 排障 SQL（积压 / dead / 重放）

> outbox 为 **at-least-once**；任何重放都可能产生重复副作用。必须确保消费者按 `event_id` 幂等处理。

将 `<module>_outbox` 替换为实际表名。

### 3.1 查询积压

```sql
SELECT sequence, event_id, topic, tenant_id, attempts, available_at, locked_at, last_error
  FROM <module>_outbox
 WHERE published_at IS NULL
 ORDER BY available_at, sequence
 LIMIT 100;
```

### 3.2 查询 dead（达到上限仍未发布）

```sql
SELECT sequence, event_id, topic, tenant_id, attempts, available_at, last_error
  FROM <module>_outbox
 WHERE published_at IS NULL AND attempts >= $1
 ORDER BY sequence
 LIMIT 100;
```

### 3.3 重置并重放单条（谨慎）

```sql
UPDATE <module>_outbox
   SET attempts = 0,
       available_at = now(),
       locked_at = NULL,
       last_error = NULL
 WHERE event_id = $1;
```

## 4. 集成测试（真实 Postgres）

`pkg/outbox` 提供 `integration` build tag 的集成测试；未配置 DSN 时会自动 skip。

```bash
OUTBOX_TEST_DSN='postgres://postgres:postgres@localhost:5438/iota_erp?sslmode=disable' \
  go test -tags=integration ./pkg/outbox -run TestRelay_Integration
```

将执行结果登记到 `docs/dev-records/DEV-PLAN-017-READINESS.md`。

## 5. 资源与安全注意事项

- 单活模式（`OUTBOX_RELAY_SINGLE_ACTIVE=true`）会为每张 outbox 表长期持有连接：部署时需预留连接池容量，避免与业务查询争用导致级联超时。
- 不建议提供“全表清空/批量重置”的便捷命令；如需脚本化重放/重置，必须强校验环境（仅本地/开发）并要求二次确认（详见 `docs/dev-plans/017-transactional-outbox.md` 的 10.4 节安全要求）。
