# DEV-PLAN-017：Transactional Outbox Readiness 记录

该记录用于 DEV-PLAN-017（`pkg/outbox`）实施过程中的可追溯验证：每次关键变更（接口契约、schema/查询口径、重试语义、并发策略、指标口径）都应在此记录执行命令与结果。

## 环境信息（填写）
- 日期（UTC）：2025-12-15
- 分支 / PR：feature/dev-plan-017（https://github.com/jacksonlee411/Bugs-Blossoms/pull/43）
- Git Revision：95968402c34794bd7acf2d0f2ee432bf068c384b
- 数据库：
  - Postgres 版本：
  - DSN/连接信息（脱敏后）：
- 运行模式：
  - 单实例 / 多实例：
  - relay：enabled/disabled：
  - cleaner：enabled/disabled：

## 门禁与命令记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-15 01:33 UTC | 本地 | `go fmt ./...` | 格式化无 diff | 无 diff | 通过 |
| 2025-12-15 01:33 UTC | 本地 | `go vet ./...` | 无 vet 报错 | 无输出 | 通过 |
| 2025-12-15 01:33 UTC | 本地 | `make check lint` | golangci-lint + cleanarchguard 通过 | 0 issues | 通过 |
| 2025-12-15 01:33 UTC | 本地 | `make test` | 测试全通过 | PASS | 通过 |
| 2025-12-15 01:33 UTC | 本地 | `go test ./pkg/outbox/...` | outbox 单测/集成测试通过 | 已由 `make test` 覆盖 | 通过 |
|  | 本地 | `OUTBOX_TEST_DSN=... go test -tags=integration ./pkg/outbox -run TestRelay_Integration` | 真实 PG 集成测试通过 | 未执行（待提供 DSN） | 未执行 |

> 如本计划引入/修改迁移（例如新增 `<module>_outbox` 表），请补充：
> - `make db migrate up` / `make db seed`（或模块对应迁移命令）
> - 关键表/索引存在性检查（SQL/截图/日志链接均可）

## 核心验收用例（勾选 + 记录证据）

### 1) 事务原子性（同 Tx 写入）
- [ ] 在同一事务内写入业务数据 + outbox，事务回滚后 outbox 无残留（附测试名/日志）
- [ ] 事务提交后 outbox 记录可被 relay claim（附 SQL/日志）

### 2) 并发与锁（SKIP LOCKED + lease）
- [ ] 两个 relay 并发 claim 同一表，不产生重复 claim（附测试名/日志）
- [ ] relay claim 后崩溃/不 ack，超过 `lock_ttl` 可被其他 relay 重新 claim（附测试名/日志）
- [ ] 默认单活（advisory lock）生效：同一表同一时刻仅一个 relay 工作（附日志/指标）

### 3) 重试与 dead-letter
- [ ] dispatch 失败会写入 `last_error` 并按 backoff 退避（附日志/SQL）
- [ ] 达到 `max_attempts` 后不再重试且记录可查询（附 SQL/指标）

### 4) Cleaner（保留策略）
- [ ] 仅清理 `published_at` 超过保留期的数据，不影响未发布记录（附测试名/SQL）

### 5) 指标与日志
- [ ] 指标：enqueue/dispatch/pending/dead 等关键指标可观测（附截图或输出片段）
- [ ] 日志字段包含 `table/topic/event_id/tenant_id/sequence/attempts`（附日志片段）
