# DEV-PLAN-076：Org 模块现状研究与 v4（Transactional Event Sourcing + Synchronous Projection）差异评估

**状态**: 草拟中（2026-01-04 01:36 UTC）

## 0. 进度速记
- [X] 盘点 Org 模块：核心表（schema/migrations）、写路径（service/repo）、读路径（deep-read/long_name/snapshot/reporting）。
- [X] 汇总相关方案文档（DEV-PLAN-020L/021/022/024/025/026/029/066/068/069B 等）作为事实源入口。
- [X] 按 “SoT / Read Model / 投射引擎 / 并发锁 / 重放与纠偏 / 索引与性能” 与 v4 方案逐项对照差异。
- [ ] 决策：是否要向 v4 形态演进（需要明确目标：审计/回放能力 vs 写放大/复杂度/运维成本）。
- [ ] 若推进“事件溯源 SSOT + 同步投射”，需另开实施子计划（建议 076A），并在新增数据库表前获得手工确认（仓库红线）。

## 1. 背景与上下文 (Context)
你提供的 v4 方案将组织架构模块定义为 **“事务性事件溯源 + 同步投射架构”**：一次事务内写入事件日志（SoT）并同步更新读模型（Read Model），写后强一致可读，且可随时重放重建读模型。

本计划的目的：基于仓库内 Org 模块既有实现与 SSOT 文档，给出一份可追溯的 **现状评估** 与 **差异清单**，并提出可选的演进路径（不在本计划内实施代码/DB 变更）。

## 2. 调研范围与事实源 (Scope & Sources)
### 2.1 代码与 schema（事实源）
- Schema SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`
- Org migrations：`migrations/org/*`
- 核心写入口（示例）：`modules/org/services/org_service.go`、`modules/org/services/org_service_025.go`、`modules/org/services/org_service_066.go`
- 读侧：`modules/org/infrastructure/persistence/org_repository.go`、`modules/org/infrastructure/persistence/org_deep_read_repository.go`
- 长名称投影：`pkg/orglabels/org_node_long_name.go`
- 快照输出（纠偏）：`modules/org/infrastructure/persistence/org_snapshot_repository.go`
- outbox dispatcher 与缓存失效：`modules/org/infrastructure/outbox/dispatcher.go`、`modules/org/handlers/outbox_events_handler.go`

### 2.2 方案/契约文档（SSOT）
本计划不复制门禁矩阵与命令入口，统一引用：
- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- Org 功能目录：`docs/dev-plans/020L-org-feature-catalog.md`
- 核心 schema/约束：`docs/dev-plans/021-org-schema-and-constraints.md`
- 事件契约（integration events）：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`
- CRUD 主链：`docs/dev-plans/024-org-crud-mainline.md`
- 时间治理与审计：`docs/dev-plans/025-org-time-and-audit.md`
- API/Authz/outbox/snapshot/batch：`docs/dev-plans/026-org-api-authz-and-events.md`
- 深读优化（closure/snapshot）：`docs/dev-plans/029-org-closure-and-deep-read-optimization.md`
- 删除自动缝补与一致性：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`、`docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`
- 长名称投影：`docs/dev-plans/068-org-node-long-name-projection.md`

## 3. Org 模块现状（As-Is）架构摘要
### 3.1 数据模型：状态切片 + 结构索引（当前 SoT）
当前 Org 的“真相”主要由以下表承载（简化表达）：
- `org_nodes`：稳定标识（code/is_root/type）。
- `org_node_slices`：节点属性时间片（Valid Time：`date effective_date/end_date`）。
- `org_edges`：层级关系时间片，包含 `path ltree` 与 `depth`；`path/depth` 在 DB trigger 中计算并兜底拒绝成环。

配套表：
- `org_audit_logs`：审计日志（用于追踪变更与排障，不是可重放 SoT）。
- `org_outbox`：事务内 enqueue 的 integration events（topic=`*.v1`），由 outbox relay 投递。

派生读模型（可 build/activate/回滚）：
- `org_hierarchy_closure*` / `org_hierarchy_snapshot*`：深读优化（029）。
- `org_reporting_nodes` 等：报表/导出侧读模型（033/057 等）。

### 3.2 写路径：Go 事务内编排（状态 + 审计 + outbox）
整体写入模式可概括为：
1) Go service 开启事务并注入 tenant + 启用 RLS；
2) 通过 repo 执行状态表写入（切片拆分/截断/新增）；
3) 写 `org_audit_logs` 记录 old/new；
4) 事务内 enqueue `org_outbox` 事件（integration events），提交后由 relay 投递；
5) 写后做 tenant 粒度缓存失效（commit 时 + outbox 消费时）。

复杂写（Move/CorrectMove/066 删除缝补）在 Go 层显式执行：
- 子树切片重建（truncate + insert）；
- `org_edges.path` 前缀 rewrite（避免跨切片失真，069/069B/066 相关）；
- preflight 预算（例如 `maxEdgesPathRewrite`）控制在线写放大。

### 3.3 并发控制：行锁为主，局部 advisory lock
- 大多数写入口使用 `SELECT ... FOR UPDATE` 锁定目标切片与子树相关行，减少并发写冲突。
- 部分“时间线级写”与“build 任务”使用 `pg_advisory_xact_lock(hashtext(lock_key))` 做互斥（例如 066 的 gap-free/auto-stitch 与 029 build）。
- 写入口整体不是统一的 “try advisory lock fail-fast” 模式。

### 3.4 读路径：基线 edges(path) + 可切换深读 + 投影工具
- 基线树读取：join `org_node_slices` + `org_edges`（as-of day）。
- 深读（祖先/子树）：支持 `edges/closure/snapshot` 多后端切换（029）。
- 组织长名称：`pkg/orglabels` 以 `org_edges.path` 为祖先链权威，SQL 内拆 path 并按 as-of join `org_node_slices`，实现 batch 1-query（068/069A）。
- 纠偏快照：`/org/api/snapshot`（026）以 as-of 输出节点/边/岗位/任职的状态流，供下游对账与补偿。

## 4. v4 参考方案摘要（你提供的目标形态）
v4 的关键点（仅提炼差异对照所需信息）：
- SoT：`org_events` 只增不改，记录业务意图（CREATE/MOVE/RENAME/DISABLE + payload）。
- Read Model：`org_unit_versions` 由事件同步投射得到（`node_path ltree` + `validity daterange` + no-overlap exclude + GiST 索引）。
- 投射引擎：DB 存储过程（提交事件 → 触发 move/rename 等切片逻辑与子树级联）。
- 并发安全：Advisory Lock 串行化同一棵树/同一节点变更。
- 灾备/重放：可 `TRUNCATE` 读模型表后按事件序重放重建。

## 5. 差异对照（Gap Analysis）
| 维度 | v4（目标） | 当前 Org（现状） | 差异与影响 |
| --- | --- | --- | --- |
| 真理之源（SoT） | `org_events` append-only | `org_nodes/org_node_slices/org_edges` 状态切片为 SoT；`org_audit_logs` 为审计；`org_outbox` 为集成事件 | 缺少可重放的“事件流 SSOT”；核心状态难以从日志确定性重建 |
| 写入入口 | `submit_org_event()` 单入口 | 多写入口在 Go service 中实现（Create/Update/Move/Correct/Rescind/066 等） | 写语义分散，投射逻辑主要在 Go；难做到 DB 级单点重放/幂等投射 |
| 同步投射 | DB 函数内同步计算读模型 | DB 有 trigger/constraint 兜底；大部分“切片分裂/子树级联/path rewrite”在 Go 编排 | 复杂度在应用层；锁持有与网络往返更难收敛 |
| 读模型表 | 单表 `org_unit_versions`（时空联合索引） | 读时 join `org_edges` + `org_node_slices`；另有 closure/snapshot/reporting 派生表（可切换/回滚） | 读模型不是单表；性能路径更多依赖派生 build 与 feature flag |
| 路径/索引策略 | `GiST(node_path, validity)` + 生成列 `path_ids` | `org_edges` 有 `gist(tenant_id, path)`；多数 as-of 使用 `effective_date/end_date` 比较；长名称 SQL 运行时拆 path | 与 v4 的“时空联合索引+预计算”不同，部分查询形状可能更吃 CPU |
| 并发互斥 | try/advisory lock 串行化关键写 | 行锁为主；advisory lock 仅覆盖部分时间线写与 build | fail-fast/统一互斥策略不一致，极端并发下更依赖行锁范围 |
| 重放与灾备 | 事件驱动可重建 | closure/snapshot/reporting 可重建；核心状态表不可“按事件流重放重建” | 灾备能力更偏“build 派生表 + snapshot/batch 纠偏”，不是纯事件溯源 |
| 多租户隔离 | 需另行设计 | tenant_id + RLS fail-closed 已融入基础设施 | v4 若落地，必须把事件表/锁 key/重放工具全量纳入租户隔离设计 |

## 6. 评估结论
1) 当前实现已经具备 v4 追求的部分结果（强约束的 Valid Time、ltree 路径、写后强一致可读、可回滚的派生读模型、outbox 一致性）。
2) 但关键范式不同：当前是“状态切片 SoT”，v4 是“事件流 SoT”。因此 v4 的核心能力（确定性重放、投射幂等、以事件为唯一写入口）在现状中不成立。
3) 若目标是“读性能 + 一致性 + 可纠偏”，现状通过 `org_edges.path` + 029 closure/snapshot + 026 snapshot/batch 已能覆盖大部分需求；若目标是“任意时刻可全量重放重建（包括核心状态）”，则需要引入事件溯源 SSOT 并改变写模型形态。

## 7. 推荐演进路径（Options）
> 本节只给出可选路线与前置条件；不在本计划内实施。

### Option 0：保持现状范式，做定点增强（低风险）
- 目标：在不引入 event-sourcing SSOT 的情况下，提升热点查询形状与并发稳定性。
- 方向示例：
  - 将热点 as-of 查询逐步收敛为 range 运算（复用 `daterange(effective_date, end_date + 1, '[)')` 语义与 GiST 能力），减少口径漂移。
  - 评估为 Move/CorrectMove 等写入口引入统一的“时间线级 advisory lock（try-lock 可选）”以降低并发死锁与锁风暴风险。
  - 对长名称/祖先链热点场景，评估是否需要进一步降低 SQL 端 path 解析成本（前提：不引入不必要的写放大）。

### Option 1：保持“状态切片 SoT”，但把复杂写逻辑收敛到 DB（中风险）
- 目标：更接近 v4 的“DB 内投射”，但不引入 `org_events` 作为 SoT。
- 做法：为 Move/CorrectMove/066 等高复杂写提供 DB 函数包装（单入口、单事务、明确锁顺序），Go 仅负责鉴权/参数校验/调用。
- 风险：需要严格的可观测与回滚策略；同时要避免与既有 Go 算法出现双实现漂移。

### Option 2：引入事件溯源 SSOT + 同步投射读模型（高风险/高成本，最接近 v4）
- 目标：对齐 v4 的“可重放重建”与“单入口投射”。
- 必要前置：
  - 另开实施子计划（建议 `DEV-PLAN-076A`）明确：事件模型、幂等与顺序、迁移路径、回放工具、与 tenant/RLS/锁 key 的约束。
  - **新增数据库表前必须获得手工确认**（仓库红线），并明确与现有表的共存/切换/回滚策略（至少双写或灰度）。
- 风险提示：这会改变 Org 模块的基础范式与运维手册，影响面远大于单点优化。

## 8. 验证与门禁（SSOT 引用）
- 本计划为文档变更：按 `AGENTS.md` 的文档门禁执行 `make check doc`。
- 若后续任一选项进入实施：按 `AGENTS.md` 的“变更触发器矩阵”选择必跑项（Go/迁移/templ/authz 等）。

