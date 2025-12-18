# DEV-PLAN-029：Org 闭包表与深层读取优化（Step 9）

**状态**: 规划中（2025-12-18 01:44 UTC）

## 0. 进度速记
- 本计划对应 `docs/dev-plans/020-organization-lifecycle.md` 的步骤 9：为“全树/深层级读取”提供可扩展的读模型，热点查询禁止递归 CTE，优先走闭包表/快照。
- 021 已落地 `org_edges.path (ltree)` 作为 M1 基线；029 在不改变写语义的前提下，新增读侧派生表与刷新任务，使读路径可平滑切换与一键回滚。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` → 步骤 9「优化深层级读取性能」。
- **当前痛点**：
  - 随节点数与深度增长，“子树/全树/祖先链/路径拍平”等深读若依赖递归 CTE，容易出现计划抖动与性能瓶颈（尤其在多 join（positions/assignments/roles）叠加时）。
  - 027 的性能预算覆盖的是 M1 树读（1k 节点）基线；但 M2+ 的“深读/报表/安全域继承”等场景会产生更重的读放大，需要独立的读模型与刷新机制。
  - 若缺少**幂等刷新 + 原子切换 + 回滚策略**，读模型的引入会在灰度阶段引入不可控的线上风险（数据过期/不一致/难回退）。
- **业务价值**：
  - 引入时态闭包表与 as-of 快照表，读侧查询可用索引命中、常数级 SQL 次数，并可按租户灰度启用。
  - 提供“可复现的 backfill/refresh 入口 + 原子切换 + 一键回滚”，使深读优化具备工程化可运维性。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 引入时态闭包表 `org_hierarchy_closure`（ancestor/descendant/depth + effective window），仅供查询（写侧不做同步级联更新）。
  - [ ] 引入 as-of 快照表 `org_hierarchy_snapshots`（按 `as_of_date` 索引），用于热点深读（全树/子树/路径拍平）走快照路径。
  - [ ] 提供幂等刷新任务：支持按 `tenant_id + hierarchy_type`（以及可选 `as_of_date`）重建 closure/snapshots，并写入 build 元数据。
  - [ ] 提供原子切换与回滚：通过 build pointer（active build）实现“新 build 就绪 → 秒级切换 → 可回退到上一个 build”。
  - [ ] 读路径可切换：通过 feature flag/配置在 `edges(path)` 与 `closure/snapshots` 间切换，并保留回退路径。
  - [ ] Readiness：准备 `docs/dev-records/DEV-PLAN-029-READINESS.md`，记录命令/耗时/结果与回滚演练。
- **非目标（本计划明确不做）**：
  - 不改变 024/025 的写语义（Insert/Correct/Rescind/冻结窗口等）。
  - 不交付可视化导出/高级报表 API（归属 033）。
  - 不实现长期压测体系、告警与值班流程（归属 034）。
  - 不引入 Asynq 等新队列系统（009A 指出 Asynq 尚未落地）；刷新任务先以 CLI/后台 worker 的最小形态落地。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划会引入 Org schema 迁移与 Go 代码（refresh/查询切换/测试），并可能引入新的路由/配置。命令与门禁以 SSOT 为准，本文只勾选触发器并给出引用。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] 迁移 / Schema（新增 `org_hierarchy_*` 表、索引、约束；必须走 Org Atlas+Goose）
  - [X] Go 代码（refresh 任务、repo 查询切换、测试/bench）
  - [X] 文档 / Readiness（新增 029 readiness record，更新 runbook/说明）
  - [ ] 路由治理（仅当新增 `/org/api/*` 的运维端点或更新 allowlist 时）
  - [ ] Authz（仅当新增运维端点暴露为 HTTP；默认用 CLI 不触发）
  - [ ] `.templ` / Tailwind（不涉及）
  - [ ] 多语言 JSON（不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

### 2.2 与其他子计划的边界（必须保持清晰）
- 021：写侧基线（`org_edges.path/depth`）与迁移工具链 SSOT。029 只追加读模型表，不改 021 的写约束/触发器语义。
- 026：Authz/403 payload/outbox/caching 口径。029 若新增任何 HTTP 运维端点，必须遵循 026 的 `ensureAuthz` 与 forbidden payload。
- 027：性能/灰度（含 query budget 思路）。029 的新读路径必须提供与 027 类似的“确定性守卫”（query budget / 结果一致性对照）。
- 028：继承/角色读侧会消费“祖先链/子树”能力；029 提供更高性能的底层查询支撑。
- 033：路径查询/报表会直接依赖闭包/快照表；033 不应自行实现递归热点查询。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  WriteAPI[/org/api/** write (024/025/026)/] --> DB[(Postgres org_* tables)]
  DB --> Outbox[(public.org_outbox)]

  Outbox --> Relay[pkg/outbox relay]
  Relay --> Dispatcher[Org Dispatcher]
  Dispatcher --> Dirty[Mark tenant dirty (db table or in-memory)]

  Worker[Org Closure/Snapshot Builder] -->|build| Closure[(org_hierarchy_closure)]
  Worker -->|build| Snap[(org_hierarchy_snapshots)]
  Worker --> Meta[(build meta: active build)]

  ReadAPI[/org/api/** read/] --> Repo[Org Read Repos]
  Repo -->|feature flag| ReadPath{edges vs closure vs snapshot}
  ReadPath --> DB
  ReadPath --> Closure
  ReadPath --> Snap
```

### 3.2 关键设计决策（ADR 摘要）
1. **写侧不做同步级联更新（选定）**
   - `org_edges` 仍是写侧事实源；闭包/快照是读侧派生表，通过 refresh 任务异步重建。
2. **闭包表 + 快照表双层（选定）**
   - `org_hierarchy_closure`：时态（有效期窗）闭包，支持任意 `effective_date` 的 as-of 查询。
   - `org_hierarchy_snapshots`：按 `as_of_date` 的快照（高命中、可控大小），用于热点深读（默认只构建“当天/当前”快照，按需扩展）。
3. **原子切换通过 build pointer（选定）**
   - 新 build 写入完成后再切换 active build；回滚仅需切回旧 build（不需要大规模 delete）。
4. **热点查询禁止递归 CTE（选定）**
   - 递归 CTE 仅允许在离线 build 任务中使用（生成 closure/snapshot）；在线 read path 不得使用递归。

## 4. 数据模型与约束 (Data Model & Constraints)
> **标准**：必须精确到字段类型、空值约束、索引策略及数据库级约束（check/exclude/fk）。
>
> 约定：Postgres 17；时间语义为 UTC 半开区间 `[effective_date, end_date)`；`hierarchy_type` M1 固定 `OrgUnit`（与 021 对齐）。

### 4.1 `org_hierarchy_closure_builds`（闭包 build 元数据）
**用途**：记录 closure build，并维护“每租户/层级一个 active build”指针，支持秒级切换与回滚。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `hierarchy_type` | `text` | `not null` + check | `'OrgUnit'` | 层级类型 |
| `build_id` | `uuid` | `not null` |  | build 标识 |
| `status` | `text` | `not null` + check | `'building'` | `building/ready/failed` |
| `is_active` | `boolean` | `not null` | `false` | 是否当前 active |
| `built_at` | `timestamptz` | `not null` | `now()` | 完成/写入时间（ready 时） |
| `source_request_id` | `text` | `null` |  | 可选：与 request_id/outbox 链路串联 |
| `notes` | `text` | `null` |  | 可选：失败原因/构建参数 |

**约束/索引（建议）**：
- `primary key (tenant_id, hierarchy_type, build_id)`
- `unique (tenant_id, hierarchy_type) where is_active`
- `check (status in ('building','ready','failed'))`

### 4.2 `org_hierarchy_closure`（时态闭包表）
**用途**：存储 ancestor→descendant 的可达关系与距离（depth），并带有效期窗（用于 as-of 查询）。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `hierarchy_type` | `text` | `not null` + check | `'OrgUnit'` | 层级类型 |
| `build_id` | `uuid` | `not null` |  | FK → `org_hierarchy_closure_builds.build_id` |
| `ancestor_node_id` | `uuid` | `not null` |  | 祖先节点 |
| `descendant_node_id` | `uuid` | `not null` |  | 后代节点 |
| `depth` | `int` | `not null` + check |  | 祖先到后代的距离（0=self） |
| `effective_date` | `timestamptz` | `not null` |  | 生效 |
| `end_date` | `timestamptz` | `not null` | `'9999-12-31'` | 失效 |

**约束/索引（建议）**：
- `check (effective_date < end_date)`
- `check (depth >= 0)`
- FK（tenant 隔离）：
  - `fk (tenant_id, ancestor_node_id) -> org_nodes (tenant_id, id) on delete restrict`
  - `fk (tenant_id, descendant_node_id) -> org_nodes (tenant_id, id) on delete restrict`
  - `fk (tenant_id, hierarchy_type, build_id) -> org_hierarchy_closure_builds (tenant_id, hierarchy_type, build_id) on delete cascade`
- 防重叠（同 build 下同一对 ancestor/descendant 的有效期窗不重叠）：
  - `exclude using gist (tenant_id with =, hierarchy_type with =, build_id with =, ancestor_node_id with =, descendant_node_id with =, tstzrange(effective_date, end_date, '[)') with &&)`
- 查询索引（表达式索引，建议）：
  - `gist (tenant_id, hierarchy_type, build_id, ancestor_node_id, tstzrange(effective_date, end_date, '[)'))`
  - `gist (tenant_id, hierarchy_type, build_id, descendant_node_id, tstzrange(effective_date, end_date, '[)'))`
  - `btree (tenant_id, hierarchy_type, build_id, ancestor_node_id, depth, descendant_node_id)`

### 4.3 `org_hierarchy_snapshot_builds`（快照 build 元数据）
**用途**：记录某租户在某个 `as_of_date` 的快照 build，并维护 active build。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `hierarchy_type` | `text` | `not null` + check | `'OrgUnit'` | 层级类型 |
| `as_of_date` | `date` | `not null` |  | 快照日期（UTC） |
| `build_id` | `uuid` | `not null` |  | build 标识 |
| `status` | `text` | `not null` + check | `'building'` | `building/ready/failed` |
| `is_active` | `boolean` | `not null` | `false` | 是否当前 active |
| `built_at` | `timestamptz` | `not null` | `now()` | 完成时间 |
| `source_request_id` | `text` | `null` |  | 链路串联 |
| `notes` | `text` | `null` |  | 失败原因/构建参数 |

**约束/索引（建议）**：
- `primary key (tenant_id, hierarchy_type, as_of_date, build_id)`
- `unique (tenant_id, hierarchy_type, as_of_date) where is_active`
- `check (status in ('building','ready','failed'))`

### 4.4 `org_hierarchy_snapshots`（as-of 快照闭包）
**用途**：存储某个 `as_of_date` 的 ancestor→descendant 快照，用于热点深读。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `hierarchy_type` | `text` | `not null` + check | `'OrgUnit'` | 层级类型 |
| `as_of_date` | `date` | `not null` |  | 快照日期 |
| `build_id` | `uuid` | `not null` |  | FK → `org_hierarchy_snapshot_builds.build_id` |
| `ancestor_node_id` | `uuid` | `not null` |  | 祖先 |
| `descendant_node_id` | `uuid` | `not null` |  | 后代 |
| `depth` | `int` | `not null` + check |  | 距离（0=self） |

**约束/索引（建议）**：
- `check (depth >= 0)`
- FK：
  - `fk (tenant_id, hierarchy_type, as_of_date, build_id) -> org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, build_id) on delete cascade`
  - `fk (tenant_id, ancestor_node_id) -> org_nodes (tenant_id, id) on delete restrict`
  - `fk (tenant_id, descendant_node_id) -> org_nodes (tenant_id, id) on delete restrict`
- `unique (tenant_id, hierarchy_type, as_of_date, build_id, ancestor_node_id, descendant_node_id)`
- 查询索引（建议）：
  - `btree (tenant_id, hierarchy_type, as_of_date, build_id, ancestor_node_id, depth, descendant_node_id)`
  - `btree (tenant_id, hierarchy_type, as_of_date, build_id, descendant_node_id, depth, ancestor_node_id)`

### 4.5 迁移策略
- **Up**：新增上述表与索引；对大表建议使用“先建表/索引 → backfill → 切换 flag”的灰度路径。
- **Down**：仅在确认无模块依赖后才允许 drop（生产环境通常只通过 feature flag 回滚，不依赖 drop）。

## 5. 接口契约 (API Contracts)
> 本计划默认不新增对外业务 API；其“接口契约”主要体现在：feature flag 的读路径选择、以及可重复执行的 refresh 工具入口。

### 5.1 Feature Flag（读路径切换口径）
> 目标：允许按租户灰度启用新读模型，并能一键回滚到 `org_edges` 基线。

- `ORG_DEEP_READ_ENABLED=true|false`（默认 `false`）：总开关；关闭时所有深读强制走 `org_edges` 基线。
- `ORG_DEEP_READ_BACKEND=edges|closure|snapshot`（默认 `edges`）：
  - `edges`：基线（021 的 `org_edges.path` 等）
  - `closure`：读时查 `org_hierarchy_closure`（按 `effective_date` as-of）
  - `snapshot`：读时查 `org_hierarchy_snapshots`（按 `as_of_date`）
- （可选）按租户 allowlist：复用 027 的 `ORG_ROLLOUT_MODE/ORG_ROLLOUT_TENANTS` 作为灰度条件（仅 allowlist 启用深读新后端）。

### 5.2 Refresh 工具入口（CLI / Make）
> 对外入口应稳定，避免“运维只能猜命令”。最终以 `Makefile` 为 SSOT；CLI 只是实现形态。

- `make org-closure-build`：为某 tenant 构建 closure build（默认 dry-run，`APPLY=1` 才写入并切换 active）。
- `make org-snapshot-build`：为某 tenant + as_of_date 构建 snapshot build（同上）。
- `make org-closure-activate`：将指定 build 激活为 active（支持回滚到上一 build）。
- `make org-closure-prune`：清理非 active 的旧 build（按保留策略）。

> 说明：若选择实现为 Go CLI（例如 `cmd/org-closure`），应输出 JSON 摘要（build_id、row_count、耗时、是否切换 active、错误信息），并将命令/输出记录到 Readiness。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 Snapshot build（推荐先落地：解决热点深读）
输入：`tenant_id, hierarchy_type, as_of_date`（UTC），以及 `org_edges` 的 as-of 视图。

1. 取 as-of edges：
   - `effective_date <= as_of_ts AND end_date > as_of_ts`（`as_of_ts = as_of_date at 00:00:00Z` 或统一的 `nowUTC` 截断规则）。
2. 生成 closure pairs（离线允许递归 CTE）：
   - base：self（depth=0）、parent→child（depth=1）
   - recursion：ancestor→descendant（depth+1）
3. 写入 `org_hierarchy_snapshots`（绑定 new `build_id`）。
4. 验证（最小）：
   - 对每个 node 至少存在 self row（depth=0）。
   - 对每条 edge 存在 depth=1 row。
5. 标记 build ready，并在事务内切换 active（可回滚）。

### 6.2 Temporal closure build（在需要“任意 effective_date as-of”时落地）
输入：`tenant_id, hierarchy_type`，以及全量 `org_edges` 时间片。

1. 生成 base rows（self + parent→child）并携带有效期窗 `[effective_date,end_date)`。
2. 递归扩展时对有效期做 intersection（祖先链上任一 edge 不存在则该窗不存在）。
3. 写入 `org_hierarchy_closure`（绑定 new `build_id`），并确保同 pair 的窗不重叠（EXCLUDE 兜底）。
4. 标记 build ready 并切换 active。

### 6.3 读路径查询（禁止递归 CTE）
以“子树/祖先链”两类查询为最小覆盖：

- **子树 descendants**（给定 `ancestor_node_id`）：
  - `snapshot`：按 `as_of_date` 找 active snapshot build → 查 snapshots where `ancestor_node_id=$id`
  - `closure`：按 `effective_date` 找 active closure build → 查 closure where `ancestor_node_id=$id AND tstzrange(effective_date,end_date,'[)') @> $as_of`
  - `edges`：基线（ltree 前缀查询）作为回退
- **祖先链 ancestors**（给定 `descendant_node_id`）：
  - 同上（按 descendant_id 方向索引）

> 要求：同一输入下三种 backend 返回的集合必须一致（排序可不同），差异需作为 bug 阻断。

### 6.4 幂等与并发控制
- **幂等**：
  - build 过程以 new `build_id` 写入；只有 ready 后才切换 active；失败 build 不影响现网读路径。
  - `APPLY=0` 时仅输出统计/计划（不落盘）。
- **并发控制**：
  - 同一 `tenant_id + hierarchy_type (+ as_of_date)` 的 build 应使用 advisory lock 或 DB row lock，避免并行构建互相覆盖。
- **清理策略**：
  - 保留最近 N 个 build（或保留最近 T 天），清理非 active build 的数据行（prune 命令）。

## 7. 安全与鉴权 (Security & Authz)
- **租户隔离**：所有 closure/snapshot 表必须包含 `tenant_id` 并在查询中强制过滤。
- **RLS（对齐 019A，若未来启用）**：
  - build 任务必须在事务内注入 `app.current_tenant`（fail-closed），避免跨租户写入。
  - 若采用“后台跨租户批处理”，必须使用专用 DB role 与审计策略，禁止应用角色使用 superuser/BYPASSRLS。
- **Authz**：
  - 默认用 CLI/后台 worker 执行 refresh，不暴露 HTTP。
  - 如后续新增运维端点（例如 `/org/api/closure:refresh`），必须要求 `Org.Admin`，并复用 026 的 forbidden payload。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/021-org-schema-and-constraints.md`：Org baseline 与 ltree path 存在（作为 edges 回退路径）。
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`：Org 迁移/门禁闭环（新增表必须走该工具链）。
  - `docs/dev-plans/027-org-performance-and-rollout.md`：query budget 与灰度/回滚口径参考。
  - `docs/dev-plans/033-org-visualization-and-reporting.md`：消费方（路径/报表）将依赖本计划产出。
- **里程碑**：
  1. [ ] 迁移落地：新增 closure/snapshot/build 表结构与索引（Org Atlas+Goose）
  2. [ ] snapshot build 工具落地（可对单租户/日期生成并切换 active）
  3. [ ] repo 查询切换（feature flag + 回退路径），并提供一致性对照测试
  4. [ ] temporal closure build（如确需支持任意 as-of 时刻）
  5. [ ] prune/回滚演练与 readiness 记录落盘

## 9. 测试与验收标准 (Acceptance Criteria)
- **正确性（一致性）**：
  - 对同一输入（tenant/hierarchy/as_of），`edges` 与 `snapshot/closure` 的结果集合一致（允许排序差异）。
  - 至少覆盖：balanced tree / deep chain / wide tree 三种结构（与 027 的数据 profile 可复用）。
- **性能（热点查询）**：
  - 深读查询不得使用递归 CTE（代码审查 + 测试约束）；SQL 次数为常数级。
  - 为关键查询添加 query budget 测试（类似 027 的“Query Count 守卫”思想）。
- **可运维性**：
  - build 任务幂等：重复执行不会破坏 active build；失败 build 不影响现网。
  - 回滚可演练：可将 active build 切回上一 build，并验证读路径恢复。
- **工程门禁**：
  - 命中触发器时按 `AGENTS.md` 执行门禁；迁移变更需通过 `make org plan && make org lint && make org migrate up`（并确保 `git status --short` 干净）。
  - 将命令/输出/耗时记录到 `docs/dev-records/DEV-PLAN-029-READINESS.md`。

## 10. 运维、回滚与降级 (Ops / Rollback)
- **快速回滚（优先）**：
  - 关闭 `ORG_DEEP_READ_ENABLED` 或将 `ORG_DEEP_READ_BACKEND=edges`，立即回退到 baseline。
- **版本回滚**：
  - 将 active build 指针切回上一 build（不需要 drop 表）。
- **数据清理**：
  - prune 非 active build（保留最近 N 个），避免表膨胀。
- **排障建议**：
  - 对比 `edges` 与 `snapshot/closure` 的差异：优先定位 edge slice 或 build 算法问题；差异视为阻断级 bug。
  - 记录 build 统计（row_count、最大 depth、耗时）用于容量评估与阈值告警（长期监控由 034 承接）。

## 交付物
- Org 迁移：`org_hierarchy_closure(_builds)`、`org_hierarchy_snapshots(_builds)`（及必要索引/约束）。
- snapshot/closure build 刷新任务（CLI/后台 worker）与 prune/activate 工具。
- repo 读路径切换（feature flag）与一致性/查询预算测试。
- `docs/dev-records/DEV-PLAN-029-READINESS.md`（在落地时填写）。  
