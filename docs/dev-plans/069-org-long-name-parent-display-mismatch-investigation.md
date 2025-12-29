# DEV-PLAN-069：Org 结构索引（`org_edges.path`）一致性修复详细设计

**状态**: 已完成（2025-12-29 03:27 UTC）— 已合并至 main（PR #157/#162）；Readiness：`docs/dev-records/DEV-PLAN-069-READINESS.md`

> 本计划以 `docs/dev-plans/001-technical-design-template.md` 为规范化模板：在保留专项调查证据的同时，把 “A + C（写入止血 + 存量治理）” 方案细化到可直接编码的技术详细设计（TDD）。

> 完成情况登记（以 main 为准）：
> - 写入止血（A）：`modules/org/services/org_service.go`、`modules/org/services/org_service_025.go`
> - 存量治理（C）脚本：`scripts/org/069_org_edges_path_inconsistency.sql`、`scripts/org/069_org_edges_path_inconsistency_count.sql`、`scripts/org/069_fix_org_edges_path_one_batch.sql`
> - 集成测试：`modules/org/services/org_069_edges_path_consistency_integration_test.go`

## 1. 背景与上下文 (Context)
- **需求来源**: Org UI 线上/本地问题复现（见“附录 A：本次问题数据库核对”）。
- **当前痛点**: 同一页面中，节点的“上级显示（ParentHint）”与“组织长名称（LongName）”出现不一致：上级显示 `丘比2`，但长名称为 `飞虫与鲜花 / AI治理办公室`，缺少中间祖先。
- **业务价值**: 保障 Org 树结构查询（祖先/子树/长名称/报表快照）的基础一致性，避免把错误长名称扩散到更多视图/报表（尤其与 DEV-PLAN-068 强耦合）。
- **问题复现**:
  - `http://localhost:3200/org/nodes?effective_date=2025-12-28&node_id=bc70deb0-ca70-4102-b7a6-2c90ac87bafa`
- **根因结论（已用 DB 真实数据验证）**:
  - 上级显示 `丘比2` 是真实数据：`org_node_slices.parent_hint` 与当日 `org_edges.parent_node_id` 一致指向 `93844c02-fd34-42a5-91fe-1a0d9c8e9658`。
  - 长名称缺段不是 UI 拼接 bug，而是 **`org_edges.path`（ltree materialized path）跨时间切片失真**：父节点的 `path` 已包含 `人力资源部`，但子节点的 `path` 未同步更新，导致长名称 SQL（依赖 `e.path @> t.path`）无法匹配到中间祖先，输出被截断（证据见附录 A）。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**:
  - [X] 把 `org_edges.path/depth` 明确为结构索引 SSOT，并补齐写路径的维护逻辑，使 Move/CorrectMove 不再引入新的跨切片 `path` 不一致。
  - [X] 提供离线巡检与修复方案，把既有不一致数据修复到可依赖状态（作为 068 的正确性 gate）。
  - [X] Move/CorrectMove 的“路径级联更新”必须受 preflight/预算控制，超限时返回稳定错误码（优先复用 `ORG_PREFLIGHT_TOO_LARGE`）。
  - [X] 对本 case（`bc70...`）在 `as-of=2025-12-28` 的长名称输出恢复为完整祖先链。
  - [X] 通过 `go fmt ./... && go vet ./... && make check lint && make test`（Go 触发器门禁）。
- **非目标 (Out of Scope)**:
  - 不把 long_name 持久化到写模型（不在 `org_edges/org_node_slices` 增加 long_name 字段）。
  - 不引入在线递归作为默认计算方式（读时递归仅可作为临时兜底，不纳入本计划主线）。
  - 不新增数据库表/大规模 Schema 改造（如需新增表/迁移，必须先获得手工确认）。

## 2.1 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器”，命令细节以 `AGENTS.md` / `Makefile` / CI 为准。

- **触发器清单（勾选本计划命中的项）**：
  - [x] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] `.templ` / Tailwind（`make generate && make css`）
  - [ ] 多语言 JSON（`make check tr`）
  - [ ] Authz（`make authz-test && make authz-lint`）
  - [ ] 路由治理（`make check routing`）
  - [ ] DB 迁移 / Schema（本计划不做；如发生变更，需切到对应门禁）
  - [ ] sqlc（本计划不涉及）
  - [ ] Outbox（仅复用既有写事件；不新增 outbox 类型）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`
  - 068 长名称投影：`docs/dev-plans/068-org-node-long-name-projection.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
    UI[Org UI / API] --> C[Org Controller]
    C --> S[OrgService Move/CorrectMove]
    S --> R[OrgRepository]
    R --> DB[(Postgres)]

    DB --> E[org_edges (ltree path)]
    DB --> NS[org_node_slices (name/parent_hint)]

    DB --> RN[org_reporting_nodes (snapshot, materialized)]
    DB --> HS[org_hierarchy_snapshots/closure (materialized)]

    UI --> LN[ResolveOrgNodeLongNames (dynamic)]
    LN --> DB
    LN --> E
    LN --> NS
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：把 `org_edges.path/depth` 定义为“结构索引 SSOT（物化路径）”**（不是长名称 SSOT）
  - 依据：长名称与祖先/子树查询都依赖 `ltree` 运算（`@>` / `<@`），一旦 path 失真，所有依赖方都会稳定地产生错误结果。
  - 结果：写路径必须维护 `path` 的跨切片一致性（本计划的 A）。

- **决策 2：推荐落地组合为 A + C（写入止血 + 存量治理）**
  - A：修复 Move/CorrectMove 写路径，避免继续制造不一致。
  - C：巡检并修复存量不一致，作为 068 的正确性 gate。
  - B（读时递归兜底）只允许作为临时容错，不纳入主线。

- **决策 3：采用“路径前缀替换（prefix rewrite）”作为核心原语**
  - 选项 A：在线递归沿 `parent_node_id` 重算每个后代的完整祖先链（慢、难控、易引入深度/超时问题）。
  - 选项 B：批量前缀替换（`new_prefix || subpath(old_path, ...)`）一次性覆盖子树内所有未来切片（SQL 原语简单、可控、无需递归）。
  - 选定：B。

- **决策 4：必须引入 preflight/预算并复用稳定错误码**
  - 复用：`ORG_PREFLIGHT_TOO_LARGE`（已在 Org 预检/变更请求体系中存在）。
  - 超限策略：拒绝在线写入，要求走离线修复/分批方案（C）。

- **决策 5：不在写模型中物化 long_name**
  - long_name 是展示派生值，依赖祖先链 + 各祖先在 as-of 当日的名称切片；任何祖先 rename/move 都会导致子树在一段时间窗内 long_name 大面积变化，写放大更大、更难控。
  - 若确实需要可 join 的“长名称”，优先使用派生读模型（如 `org_reporting_nodes.path_names[]` / `org_reporting` 视图）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 Schema 定义 (Atlas HCL / SQL)
> 本计划不改 Schema；以下仅列出与一致性约束相关的关键字段与数据库行为（以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为准）。

- **`org_edges`（有效期切片 + 物化路径）**
  - 核心字段：`tenant_id, hierarchy_type, parent_node_id, child_node_id, path (ltree), depth (int), effective_date (date), end_date (date)`
  - 约束：同一 `child_node_id` 的有效期切片不重叠（`EXCLUDE ... daterange(effective_date, end_date + 1, '[)')`）。
  - 触发器：
    - `org_edges_before_insert_set_path_depth`：仅在 `INSERT` 时计算 `path/depth`（并做环路检测）。
    - `org_edges_before_update_prevent_key_updates`：禁止更新键字段（允许更新 `path/depth`）。

- **`org_node_slices`（名称等有效期切片 + parent_hint）**
  - `parent_hint` 是 UI/展示层“上级提示”，不是结构索引；结构索引以 `org_edges` 为准。

- **`org_reporting_nodes`（物化报表快照，DEV-PLAN-033）**
  - 关键字段：`path_node_ids[] / path_codes[] / path_names[]`，属于 **派生读模型**，构建后不随写实时更新。

### 4.1.1 关键不变量（本计划要强制达成）
对任意一条 `org_edges` 行 `c`（`parent_node_id IS NOT NULL`），在 `as-of = c.effective_date` 上必须满足：

- `parent_path @> child_path`（父 path 必须是子 path 的前缀祖先）
- `child_path = parent_path || child_key`（最后一段必须是 child 的 key；且 `depth = nlevel(child_path)-1`）

其中 `child_key := replace(lower(child_node_id::text), '-', '')`（与 `org_edges_set_path_depth_and_prevent_cycle()` 一致）。

### 4.1.2 “物化 vs 动态”现状总结（用于解释认知冲突）
- **物化（持久化存储，不会自动重算）**：
  - `org_edges.path/depth`：写时（INSERT）计算；后续祖先链变化不会自动级联重算。
  - `org_hierarchy_closure/*snapshots*/org_reporting_nodes`：离线构建的派生读模型，构建后保持静态。
- **动态生成（读时计算，每次查询都会跑 SQL/拼接）**：
  - 组织长名称（LongName）：读时用 SQL `string_agg(...)` 拼接，但依赖 `org_edges.path` 做祖先匹配（并非沿 `parent_node_id` 在线递归）。

> 术语补充：仓库内不存在名为 `org_edgepath` 的表/视图；在讨论中若出现 “org_edgepath”，通常指代 `org_edges.path` 这一列（edge path）。

### 4.1.3 例子（来自本次调查数据）
- 节点：
  - `b625ee4e-b201-41a4-ae11-e4adf814f9e5`：飞虫与鲜花（根）
  - `cc8d5c7a-ab6b-457c-8afc-4bc62b0217f6`：人力资源部
  - `93844c02-fd34-42a5-91fe-1a0d9c8e9658`：丘比2
  - `bc70deb0-ca70-4102-b7a6-2c90ac87bafa`：AI治理办公室
- 正确的 `org_edges.path`（as-of=2025-12-28）应为：
  - `b625... . cc8d... . 93844... . bc70...`
- 当前错误的 `org_edges.path` 为：
  - `b625... . 93844... . bc70...`（缺少 `cc8d...`）
- 这会导致 long_name 查询只能匹配到根与自己，输出：`飞虫与鲜花 / AI治理办公室`（见附录 A）。

### 4.2 迁移策略
- 本计划不做 DDL 迁移/Schema 变更。
- 若后续发现必须新增表或引入触发器级联更新（写放大/锁风险更高），需单独起草并获得手工确认后再执行。

## 5. 接口契约 (API Contracts)
> 本计划不改变 URL/Method/Payload 结构；但可能新增/复用 `ORG_PREFLIGHT_TOO_LARGE` 作为 Move/CorrectMove 的失败路径（保持错误码稳定）。

### 5.1 JSON API: `POST /org/api/nodes/{id}:move`
- **Request**:
  ```json
  {
    "effective_date": "2025-12-10",
    "new_parent_id": "93844c02-fd34-42a5-91fe-1a0d9c8e9658"
  }
  ```
- **Response (200 OK)**:
  - `effective_window.effective_date/end_date`（与现有实现一致）。
- **Error Codes（与现有一致，补充本计划关注项）**:
  - `422 ORG_USE_CORRECT_MOVE`：当 `effective_date` 恰好是 edge slice start。
  - `422 ORG_PREFLIGHT_TOO_LARGE`：路径级联更新影响面超限（本计划新增触发点）。

### 5.2 JSON API: `POST /org/api/nodes/{id}:correct-move`
- **Request/Response**: 同 Move，语义为“在 slice start 上纠正 parent”。
- **Error Codes（补充）**:
  - `422 ORG_USE_MOVE`：当 `effective_date` 不是 edge slice start。
  - `422 ORG_PREFLIGHT_TOO_LARGE`：路径级联更新影响面超限（本计划新增触发点）。

### 5.3 UI/HTMX
- 本计划不改 UI 交互契约；UI 展示的 long_name 将因底层 `path` 一致性修复而“自然恢复正确”。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 方案 A：写时保证 `org_edges.path` 跨时间切片一致（止血）
#### A.1 适用范围（必须覆盖的写入口）
- `modules/org/services/org_service.go`：`MoveNode`
- `modules/org/services/org_service_025.go`：`CorrectMoveNode`

> 原则：凡是“改变某节点在某个 `effective_date=E` 起的祖先链”的写操作，都必须维护 `org_edges.path/depth` 的跨切片一致性。

#### A.2 关键观察：为什么可以用 `effective_date >= E` 过滤
Move/CorrectMove 的既有实现会把 **as-of=E 子树内、覆盖 E 的所有 edge 切片** 统一“截断旧 slice（end=E-1）并插入新 slice（start=E）”。

因此：对任意一个后代节点，只要它在 E 之后仍属于该子树，它在 `[E, +∞)` 上生效的 edge 行，其 `effective_date` 一定满足 `>= E`（要么就是新插入的 `E`，要么就是未来某天才开始的 slice）。

#### A.3 核心策略：对子树未来切片做“路径前缀替换”
定义：
- `E`：本次 Move/CorrectMove 的 `effective_date`
- `old_prefix`：移动前该节点在 `as-of=E` 的 `path`（`movedEdge.Path`）
- `new_prefix`：移动后该节点在 `as-of=E` 的 `path`（插入新 edge 后读取回来的 `path`）

目标：把所有仍使用 `old_prefix` 的后代 edge 行统一改成以 `new_prefix` 为前缀。

更新范围（必须同时满足）：
- `path <@ old_prefix`（位于旧子树）
- `effective_date >= E`（只修正从 E 起的未来语义）
- `nlevel(path) > nlevel(old_prefix)`（只更新后代节点，避免误改被移动节点自身）

替换规则：
- `new_path = new_prefix || subpath(old_path, nlevel(old_prefix))`
- `depth = nlevel(new_path) - 1`

#### A.4 preflight（预算）与失败策略
必须先评估影响行数，超限则拒绝写入（返回 `ORG_PREFLIGHT_TOO_LARGE`）。

**预算对象（必须明确）**：
- `affected_edges` 指 **本次 A.5 的 prefix rewrite 将要更新的 `org_edges` 行数（rows）**，不是“节点数”。
- 口径必须与 A.5 的 `WHERE` 条件 **完全一致**（包含 `nlevel(e.path) > nlevel(old_prefix)`），避免出现 “count/更新口径漂移”。

**为何以“行数”做预算（行业一般做法）**：
- 本项目是有效期切片模型：同一个节点在未来可能存在多个 edge slice，因此“节点数”会系统性低估真实写放大。
- 真正决定在线事务风险的是：更新行数 + 索引维护 + WAL/锁持有时间；行业里通常用“预计更新 rows/bytes”做在线预算，而不是概念层的节点数。

建议初始阈值：`maxEdgesPathRewrite = 5000`（与现有预检逻辑的 `maxSubtree=5000` 对齐；后续若需要可再抽成配置项）。

**阈值来源与校准原则（避免拍脑袋）**：
- 初值对齐 `maxSubtree=5000` 的动机：保持“在线可接受影响面”的数量级一致，先保守落地止血。
- 校准方式（建议在进入大租户前做一次）：用真实数据对典型 move 做 `EXPLAIN (ANALYZE, BUFFERS)`，观察 UPDATE 的耗时、锁等待、WAL 写入量；阈值以“可接受的在线事务时长/锁持有”反推。
- 是否可配置：本计划建议先用常量（更简单、更可预测）；若在不同环境/租户差异显著，可在后续迭代把阈值升级为配置项（避免在实现阶段引入额外配置面）。

建议 preflight SQL（与真实 UPDATE 同范围，避免 drift）：

```sql
SELECT count(*) AS affected_edges
FROM org_edges e
WHERE e.tenant_id=$1
  AND e.hierarchy_type=$2
  AND e.effective_date >= $3::date
  AND e.path <@ $4::ltree
	AND nlevel(e.path) > nlevel($4::ltree);
```

**超限时的处置路径（必须可执行）**：
- 行为：当 `affected_edges > maxEdgesPathRewrite`，直接返回 `422 ORG_PREFLIGHT_TOO_LARGE` 并回滚事务（不允许“部分更新”）。
- 可观测性：由于当前 API 错误结构的 `meta` 仅包含 `request_id`（见 `modules/org/presentation/controllers/org_api_controller.go`），为避免引入新契约，本计划建议在 `message` 里至少包含：`affected_edges`、`limit`、`effective_date`、`node_id`。
  - 示例（message 形状）：`subtree path rewrite impact is too large (affected_edges=12345, limit=5000, effective_date=2025-12-10, node_id=...)`
- 下一步（runbook）：
  1. **若是“大子树 move”导致超限**：拆分变更（把一次大 move 拆成多次较小 move），或在维护窗口执行（离线方式/提高离线预算），避免在线长事务。
  2. **若怀疑存在存量 path 不一致**：先执行 6.2 的 C.1 巡检并按 C.2 分批修复至可依赖水平（最好收敛到 0），再重试 move。
  3. **若启用了派生表**（`org_hierarchy_*` / `org_reporting_nodes`）：修复后执行 C.3 重建，避免下游继续读到旧快照。

#### A.5 路径前缀替换 UPDATE（原子步骤）
```sql
UPDATE org_edges e
SET
  path  = ($5::ltree) || subpath(e.path, nlevel($4::ltree)),
  depth = nlevel(($5::ltree) || subpath(e.path, nlevel($4::ltree))) - 1
WHERE e.tenant_id=$1
  AND e.hierarchy_type=$2
  AND e.effective_date >= $3::date
  AND e.path <@ $4::ltree
  AND nlevel(e.path) > nlevel($4::ltree);
```

说明：`org_edges_prevent_key_updates()` 只禁止更新键字段（`tenant_id/hierarchy_type/parent_node_id/child_node_id/effective_date`），允许更新 `path/depth`。

#### A.6 伪代码（放入 Move/CorrectMove 的事务内）
1. 开启事务（已存在）。
2. 锁定 moved edge（as-of=E），记录 `old_prefix = movedEdge.Path`。
3. 执行既有 Move/CorrectMove：插入新的 moved edge slice、并对 as-of=E 子树内覆盖 E 的 edge 做重切片。
4. 读取 moved edge 在 `effective_date=E` 上的新 `path`，作为 `new_prefix`。
5. `count = preflight(tenant, hierarchy, E, old_prefix)`（同 A.4）。
6. 若 `count > limit`：返回 `ORG_PREFLIGHT_TOO_LARGE`（事务回滚）。
7. 执行 A.5 的 UPDATE（同事务），更新 `count` 行或 0 行（若没有不一致）。
8. 提交事务。

#### A.7 代码落点（便于直接实现）
- Repository：在 `modules/org/infrastructure/persistence/org_crud_repository.go` 增加两个原语方法（同一事务内调用）
  - `CountDescendantEdgesNeedingPathRewriteFrom(ctx, tenantID, hierarchyType, fromDate, oldPrefix)`：对应 A.4 的 COUNT。
  - `RewriteDescendantEdgesPathPrefixFrom(ctx, tenantID, hierarchyType, fromDate, oldPrefix, newPrefix)`：对应 A.5 的 UPDATE。
- Service：
  - `modules/org/services/org_service.go` 的 `MoveNode`：在插入 moved edge + 子树重切片后，调用 `LockEdgeStartingAt(..., effectiveDate)` 读回 moved edge 的 `new_prefix`，然后执行 preflight + rewrite。
  - `modules/org/services/org_service_025.go` 的 `CorrectMoveNode`：同上。

### 6.2 方案 C：离线巡检 + 修复任务（存量治理）
#### C.1 一致性检测（巡检）
定位“父边 path 不是子边 path 前缀”的 edge 切片（把 `org_edges.path` 当 SSOT 的一致性检查）：

```sql
SELECT
  c.id AS child_edge_id,
  c.child_node_id,
  c.parent_node_id,
  c.effective_date,
  c.path::text AS child_path,
  p.path::text AS parent_path
FROM org_edges c
JOIN org_edges p
  ON p.tenant_id=c.tenant_id
 AND p.hierarchy_type=c.hierarchy_type
 AND p.child_node_id=c.parent_node_id
 AND p.effective_date <= c.effective_date
 AND p.end_date >= c.effective_date
WHERE c.tenant_id=$1
  AND c.hierarchy_type=$2
  AND c.parent_node_id IS NOT NULL
  AND NOT (p.path @> c.path)
ORDER BY c.effective_date DESC;
```

#### C.2 离线修复（回填）
修复原语：仍复用 A 的“前缀替换”，按不一致记录逐条收敛直到巡检为 0。

#### C.2.1 分批策略（行业一般做法，避免“大事务一次跑全表”）
- **分批单位（推荐默认）**：以一条不一致记录 `child_edge_id` 为一个 batch（修复该 edge 自身 + 对其后代做一次 prefix rewrite）。
- **每批预算（建议）**：对 batch 的 prefix rewrite 也先做一次 `affected_edges` 统计；可设置离线阈值（例如 `maxOfflineEdgesPathRewrite`），超限则要求维护窗口执行或进一步拆分（不要在日常在线窗口跑超大 UPDATE）。
- **并发/锁风险控制（建议）**：离线执行时建议设置 `lock_timeout/statement_timeout`（环境策略为准），失败则记录并重试/跳过，避免与在线写入互相阻塞。
- **拆分原则**：优先按“更小的子树根”分批（继续从 C.1 取下一条不一致记录）；若某条不一致 edge 的后代规模极大且不可拆，则必须走维护窗口/后台作业策略（而不是强行在线执行）。

#### C.2.2 baseline 记录（让治理可追踪、可复盘）
建议在每次离线执行前后记录以下最小信息（可写入对应的 readiness/运维记录）：
- `tenant_id/hierarchy_type`、执行时间窗（start/end）、`inconsistent_edges_before/after`（可用 C.1 的 `COUNT(*)` 版本）、本批 `child_edge_id`、本批 `affected_edges`、耗时、是否触发派生表重建、备注（异常/跳过原因）。

> 目标：把“离线修复”从一次性手工操作，变成可重复、可回放、可逐步收敛的治理流程。

单条修复流程：
1. 取一条不一致记录 `c`，并找到其父边 `p`（as-of=c.effective_date）。
2. 计算：
   - `E = c.effective_date`
   - `old_prefix = c.path`
   - `child_key = replace(lower(c.child_node_id::text), '-', '')`
   - `new_prefix = p.path || child_key::ltree`
3. 先修复该条 edge 自身：`UPDATE org_edges SET path=new_prefix, depth=nlevel(new_prefix)-1 WHERE tenant_id=? AND id=c.id;`
4. 再对其后代执行 A.5 的 prefix rewrite（输入 `old_prefix/new_prefix/E`），同步修复子树未来切片。
5. 重复执行直到 C.1 巡检为 0（建议按租户/日期/子树分批，避免一次性全表扫描）。

#### C.3 派生表重建（如启用）
若系统启用了 deep-read/报表快照（`org_hierarchy_*` / `org_reporting_nodes`），在修复后按其构建流程重建，确保 downstream 读模型对齐：
- `make org-closure-build` / `make org-snapshot-build`
- `make org-reporting-build`

## 7. 安全与鉴权 (Security & Authz)
- 本计划不新增/修改 Casbin 策略。
- Move/CorrectMove 仍通过既有鉴权：API 端 `orgEdgesAuthzObject` 的 `write` 权限（控制器已存在）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**:
  - 本计划是 DEV-PLAN-068 的正确性前置条件：068 的 long_name 计算同样依赖 `org_edges.path`。
  - 与 068 不存在目标冲突：069 修复的是结构索引底座一致性；068 的“读时派生 long_name”仍成立，但其正确性依赖 069 的不变量。
- **里程碑**:
  1. [X] 在 `MoveNode`/`CorrectMoveNode` 事务内落地 A（preflight + prefix rewrite）。
  2. [X] 新增/补齐集成测试：覆盖“回溯生效日移动祖先 + 后代未来切片 + long_name 不缺段”的回归。
  3. [X] 落地 C（巡检 SQL 固化 + 离线修复执行方式），记录 baseline 与修复进度。
  4. [X] （如启用）重建 `org_hierarchy_*` / `org_reporting_nodes` — N/A（本计划未把派生表重建作为 gate；需要时按各自构建流程执行）。
  5. [X] Readiness：按项目约定补充对应 readiness 记录。

## 9. 测试与验收标准 (Acceptance Criteria)
- **本 case 验收**（`as-of=2025-12-28`）：
  - `bc70deb0-ca70-4102-b7a6-2c90ac87bafa` 的 long_name 输出应为：`飞虫与鲜花 / 人力资源部 / 丘比2 / AI治理办公室`。
- **一致性验收（建议作为 gate）**：
  - 对目标租户运行 C.1 巡检返回 0 行；或在无法一次性修复为 0 时，至少确保 A 已止血且不再新增不一致（巡检趋势单调下降）。
- **回归测试建议（最小场景）**：
  1. 构造链路：`root -> cc8d(人力资源部) -> 93844(丘比2) -> bc70(AI治理办公室)`。
  2. 先对 `bc70` 执行 Move（effective=2025-12-10）到 `93844` 下，确保 `bc70` 的 edge slice 从该日开始生效。
  3. 再对 `93844` 执行回溯 Move/CorrectMove（effective=2025-12-06）到 `cc8d` 下。
  4. 验证：`as-of=2025-12-28` 时 `bc70` 的 `org_edges.path` 与 `93844` 的 `path` 满足前缀不变量，且 long_name 不缺段（附录 A 的 SQL 可直接复用）。
- **门禁**：
  - 触发器命中 Go：必须通过 `go fmt ./... && go vet ./... && make check lint && make test`。

## 10. 运维与监控 (Ops & Monitoring)
- 本项目仍处于早期阶段，不引入额外监控/开关。
- **回滚方案**:
  - 代码回滚：按正常流程回滚对应提交。
  - 数据回滚：本计划的数据修复属于“结构索引校正”；若出现误修复，优先用 C.1/C.2 继续收敛到不变量满足；必要时通过数据库备份/快照回滚（具体执行以运维/本地环境为准）。

---

## 附录 A：本次问题数据库核对（证据）
> 目的：保留“查询链路与真实数据”的可复现证据，方便复查与回归测试。

### A.1 调查环境
- DB：`postgres://postgres@localhost:5438/iota_erp?sslmode=disable`
- tenant：`00000000-0000-0000-0000-000000000001`
- as-of：`2025-12-28`
- 目标节点（AI治理办公室）：`bc70deb0-ca70-4102-b7a6-2c90ac87bafa`

### A.2 目标节点当日 slice（ParentHint）
```sql
SELECT id, org_node_id, name, parent_hint, effective_date, end_date
FROM org_node_slices
WHERE tenant_id='00000000-0000-0000-0000-000000000001'
  AND org_node_id='bc70deb0-ca70-4102-b7a6-2c90ac87bafa'
  AND effective_date <= '2025-12-28'::date
  AND end_date >= '2025-12-28'::date;
```
结果要点：`parent_hint = 93844c02-fd34-42a5-91fe-1a0d9c8e9658`（`name=丘比2`）。

### A.3 目标节点当日边关系（真实父级 + 错误 path）
```sql
SELECT id, parent_node_id, child_node_id, path::text, depth, effective_date, end_date
FROM org_edges
WHERE tenant_id='00000000-0000-0000-0000-000000000001'
  AND hierarchy_type='OrgUnit'
  AND child_node_id='bc70deb0-ca70-4102-b7a6-2c90ac87bafa'
  AND effective_date <= '2025-12-28'::date
  AND end_date >= '2025-12-28'::date;
```
结果要点：
- `parent_node_id = 93844...`（与 parent_hint 一致）
- `path = b625... . 93844... . bc70...`（缺少 `cc8d...`）

### A.4 父节点当日边关系（上级的 path 已包含中间祖先）
```sql
SELECT id, parent_node_id, child_node_id, path::text, depth, effective_date, end_date
FROM org_edges
WHERE tenant_id='00000000-0000-0000-0000-000000000001'
  AND hierarchy_type='OrgUnit'
  AND child_node_id='93844c02-fd34-42a5-91fe-1a0d9c8e9658'
  AND effective_date <= '2025-12-28'::date
  AND end_date >= '2025-12-28'::date;
```
结果要点：`path = b625... . cc8d... . 93844...`，与子节点不构成前缀关系。

### A.5 长名称 SQL（等价 `pkg/orglabels/org_node_long_name.go`）
> 依赖 `e.path @> t.path` 做祖先匹配，因此当 path 不一致时会缺段。

```sql
WITH input AS (
  SELECT 'bc70deb0-ca70-4102-b7a6-2c90ac87bafa'::uuid AS org_node_id,
         '2025-12-28'::date AS as_of_day
),
target AS (
  SELECT i.org_node_id, i.as_of_day, e.path, e.depth AS target_depth
  FROM input i
  JOIN org_edges e
    ON e.tenant_id='00000000-0000-0000-0000-000000000001'
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_day
   AND e.end_date >= i.as_of_day
),
ancestors AS (
  SELECT t.org_node_id, t.as_of_day,
         e.child_node_id AS ancestor_id,
         (t.target_depth - e.depth) AS rel_depth
  FROM target t
  JOIN org_edges e
    ON e.tenant_id='00000000-0000-0000-0000-000000000001'
   AND e.hierarchy_type='OrgUnit'
   AND e.effective_date <= t.as_of_day
   AND e.end_date >= t.as_of_day
   AND e.path @> t.path
),
parts AS (
  SELECT a.rel_depth,
         COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM ancestors a
  JOIN org_nodes n
    ON n.tenant_id='00000000-0000-0000-0000-000000000001' AND n.id=a.ancestor_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id='00000000-0000-0000-0000-000000000001' AND ns.org_node_id=a.ancestor_id
   AND ns.effective_date <= a.as_of_day AND ns.end_date >= a.as_of_day
)
SELECT string_agg(part, ' / ' ORDER BY rel_depth DESC) AS long_name
FROM parts;
```
实际结果：`飞虫与鲜花 / AI治理办公室`。

### A.6 审计日志：回溯生效日更新导致 path 失真
```sql
SELECT request_id, transaction_time, change_type, entity_id, effective_date,
       old_values->>'parent_node_id' AS old_parent,
       new_values->>'parent_node_id' AS new_parent,
       meta->>'operation' AS op
FROM org_audit_logs
WHERE tenant_id='00000000-0000-0000-0000-000000000001'
  AND entity_type='org_edge'
  AND entity_id IN (
    '32207122-ea2e-4614-ab24-4936e5491ae6', -- bc70 的 edge
    'ed139a27-4f2f-4412-80d3-a16a739197e4'  -- 93844 的 edge
  )
ORDER BY transaction_time DESC;
```
结果要点（UTC）：先移动 bc70（effective 2025-12-10），后回溯移动 93844（effective 2025-12-06），后者改变祖先链但不会自动重算既有子孙的 `path`。
