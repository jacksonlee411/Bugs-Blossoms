# DEV-PLAN-071：Docker 中 PostgreSQL CPU 偏高调查与建议

**状态**: 草拟中（2025-12-29 07:33 UTC）

## 背景
本地开发环境中，Docker 里的 PostgreSQL（`postgres:17`）出现持续偏高的 CPU 使用率，影响本机其它任务与测试稳定性。

## 目标与非目标
- **目标**
  - [ ] 明确 CPU 偏高的主要归因与证据链（可复现、可验证）。
  - [ ] 给出优先级明确的缓解/修复建议（含安全注意事项）。
  - [ ] 给出可验收的“恢复健康”指标。
- **非目标**
  - 不在本 DEV-PLAN 中直接提交“清库/删库”的破坏性操作（需人工确认后另行执行/落地）。
  - 不在本 DEV-PLAN 中做全面数据库参数调优（仅提出与现象强相关的建议）。

## 调查范围与环境
- 容器：`iota-sdk-dev-db-1`（Image：`postgres:17`，端口：`5438->5432`）
- 说明：该容器由本仓库 compose/devhub 编排启动；具体运行参数以 `compose.dev.yml` 为准。

## 复核记录（2025-12-29）
> 目的：验证“071 关键发现”是否属实，并补齐可追溯证据与更精确的根因链路。

1. **容器确实以 `log_statement=all` 启动（非推测）**
   - `docker inspect iota-sdk-dev-db-1` 显示 Cmd/Args 为 `postgres -c log_statement=all ...`。
   - 容器内 `SHOW log_statement;` 返回 `all`。

2. **CPU 热点进程确实是 `autovacuum launcher`，且是“持续消耗”而非瞬时抖动**
   - `docker top` 显示最高 CPU 进程为 `postgres: autovacuum launcher`。
   - 容器内 `ps` 采样：`autovacuum launcher` 运行约 9 小时累计 CPU 时间约 1 小时（平均约 11% 单核）。
   - `pg_stat_activity` 显示 `backend_type='autovacuum launcher'`，`wait_event=AutovacuumMain`。
   - 采样时刻未观测到活跃的 autovacuum worker（`pg_stat_activity` 里 `autovacuum:%` 为 0；`pg_stat_progress_vacuum` 为空）。

3. **数据库数量异常属实，且命名与测试框架默认策略一致**
   - `SELECT count(*) FROM pg_database;` 返回 `6551`。
   - `test%` 前缀库数量 `6534`（抽样库名形如：`testaichatconfigrepository_delete_49493`）。
   - 该命名模式与 `pkg/itf/context.go` 的默认 dbName 策略一致：`fmt.Sprintf("%s_%d", tb.Name(), os.Getpid())`。

## 因果验证：清理测试库后 CPU 恢复（2025-12-29）
> 目的：验证“CPU 偏高”与“测试库数量爆炸”之间是否存在强相关的因果（至少在本地环境中可复现）。

- **清理前（2025-12-29 07:54 UTC）**
  - `pg_database`：`db_total=6551`，`test%/t_%=6545`
  - `docker stats iota-sdk-dev-db-1`：CPU 约 `20%`
  - 备注：`docker top` 的 `%CPU` 更像进程生命周期平均值，清理后的即时观测以 `docker stats`/容器内 `top` 为准。

- **执行动作**
  - 批量执行 `DROP DATABASE IF EXISTS ... WITH (FORCE)`，目标范围仅限 `test%` 与 `t_%`（共 `6545` 个数据库），分批 `LIMIT 200` 直到剩余为 0。

- **清理后（2025-12-29 08:11 UTC）**
  - `pg_database`：`db_total=6`，`test%/t_%=0`（剩余：`iota_erp`、`iota_erp_e2e`、`org_dev`、`postgres`、`template0`、`template1`）
  - `docker stats iota-sdk-dev-db-1`：CPU 约 `0%`（恢复到空闲水平）
  - 容器内 `top` 采样：CPU 空闲 `~98%`，`autovacuum launcher` 即时 CPU 约 `0%`

## 落地变更（2025-12-29）
- [X] **测试库自动清理**：为 `itf` 测试上下文增加 `tb.Cleanup` 阶段的 `DROP DATABASE ... WITH (FORCE)`（仅对 `test*` / `t_*` 命名空间生效），避免跨运行批次累积（实现：`pkg/itf/context.go`、`pkg/itf/utils.go`）。
  - 可选保留：设置环境变量 `ITF_KEEP_DATABASES=1` 可跳过自动删除（用于本地排障时保留现场）。
- [X] **开发 compose 默认不打全量 SQL**：`compose.dev.yml` 移除 `log_statement=all` 默认配置，降低日志开销与误用风险。

## 关键发现（Evidence）
1. **CPU 主要消耗来自 autovacuum launcher（后台维护）而非前台业务查询**
   - 通过 `docker top` / 容器内 `top` 观察到高 CPU 进程为 `postgres: autovacuum launcher`。
   - 同时 `pg_stat_activity` 未显示长时间运行的活跃 SQL（采样时刻）。

2. **集群内数据库数量异常（绝大多数为测试库残留）**
   - `pg_database` 统计：总库数约 `6551`。
   - 其中 `test%` 前缀库约 `6534`（基本可判定为测试/集成测试创建后未清理的临时库）。
   - 影响：即使单库很小，autovacuum/统计/系统表扫描等后台维护也会因“库数量爆炸”产生持续 CPU 开销。

3. **dev compose 启用了全量 SQL 日志（会在有流量时放大 CPU/IO）**
   - `compose.dev.yml` 中 PostgreSQL 以 `-c log_statement=all` 启动。
   - 影响：一旦有请求/测试跑起来，SQL 字符串格式化与日志写入（stderr + docker logging driver）会显著增加 CPU（与查询频率/语句长度正相关）。

## 归因更新（Root Causes，复核后）
1. **测试框架默认“每个 test 创建一个新数据库”，且只关闭连接不删除数据库**
   - `pkg/itf/context.go` 默认 dbName 含 `os.Getpid()`：同一测试在不同 `go test` 进程/不同运行批次会产生不同数据库名，导致“跨运行批次”持续累积。
   - `pkg/itf/itf.go` 的 `itf.Setup(...)` 与 `pkg/itf/suite.go` 的 `itf.HTTP(...)` 均会走到 `NewTestContext().Build(tb)`，因此该策略会覆盖大量集成测试用例。
   - `pkg/itf/context.go` 的 `tb.Cleanup(...)` 仅做 `tx.Rollback` 与 `pool.Close()`，未对数据库执行 `DROP DATABASE`。
   - `pkg/itf/utils.go` 的 `CreateDB(...)` 负责 `DROP DATABASE IF EXISTS <same-name>` 再 `CREATE DATABASE <name>`；但当 name 带 pid 时，对“上一次运行的旧 pid 库”不会命中，因此旧库保留并逐步堆积。

2. **`log_statement=all` 作为默认长期配置会放大 CPU/IO（但不是“无流量也高 CPU”的唯一解释）**
   - 该配置更适合短时排障；与测试/业务高频查询叠加时会进一步推高 CPU。

## 解决建议（按优先级）
### P0：止血（当天可做）
1. **清理残留测试库（需人工确认后执行）**
   - 建议策略：仅删除符合“明确前缀/命名规则”的临时库（如 `test%`、`t_%`），避免误删。
   - 风险：破坏性操作；需先确认是否有需要保留用于复现的库白名单。

2. **将 `log_statement=all` 改为按需开启**
   - 建议默认关闭（`none`），排障时通过临时覆盖方式开启；或改为只记录慢查询（例如使用 `log_min_duration_statement`）。
   - 具体修改点：`compose.dev.yml`（保持与 SSOT 一致，避免在多处复制启动参数）。

### P1：根治（需要改代码/流程）
1. **为测试创建的临时数据库建立“强一致清理”机制**
   - 在测试入口统一注册 `tb.Cleanup(...)` 执行 `DROP DATABASE ... WITH (FORCE)`（在确保连接池/事务已关闭之后），避免跨运行批次累积。
   - 对并发/崩溃场景：提供可重复执行的“残留清理”子命令/脚本（只清理临时命名空间），并在本地常用工作流或 CI 里显式调用。

2. **减少“每个测试一个数据库”的必要性**
   - 优先选择：单库多 schema / 每次测试重置 schema（在可接受隔离前提下），避免创建海量数据库对象。
   - 若必须创建数据库：限制并行度/配额，或使用固定数量的数据库池复用。

### P2：观测与阈值（防复发）
1. **为本地开发增加“数据库数量阈值”自检**
   - 例如在 `make preflight` 或测试入口前增加自检：`pg_database` 总数/临时库数量超过阈值则提示清理。

## 验收标准（Acceptance Criteria）
- [ ] 在“无业务请求、无测试运行”的稳定态，`iota-sdk-dev-db-1` 的 CPU 长期处于低位（例如 < 5%，以本机/负载为准）。
- [ ] `pg_database` 中临时测试库数量回落到可控范围（例如 < 50），且不会随日常开发持续增长。
- [ ] 默认开发配置不再开启 `log_statement=all`；需要排障时有明确可复制的开启方式与回滚方式。

## 关联与参考（SSOT）
- 本地开发编排：`devhub.yml`、`compose.dev.yml`
- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
