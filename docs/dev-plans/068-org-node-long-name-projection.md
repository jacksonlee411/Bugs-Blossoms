# DEV-PLAN-068：组织长名称投影（OrgNodeLongName Projection）详细设计

**状态**: 规划中（2025-12-28 08:01 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**：在 HRMS 中，几乎所有“列表/报表”都会引用组织信息（任职记录、员工花名册、薪资明细等）；仅展示“部门短名称”在同名部门、频繁 Move/Rename 的场景下歧义很大。
- **当前痛点**：
  - 组织长名称（root→self 的路径）属于高复用能力，但若每个页面各自实现，会产生 **SSOT 漂移**（拼接规则/兜底不一致）与 **性能风险**（按行调用 `GetNodePath` 形成隐式 N+1）。
  - 历史语义要求“as-of 正确”：部门名称会更名、上级部门会变化；同一任职切片在不同日可能对应不同路径描述。
- **业务价值**：抽出“组织长名称投影”作为共享读能力，让所有列表/报表在 **同一份契约** 下、以 **批量方式** 获得“当日快照”的长路径描述。

**相关计划/约束（SSOT）**
- 行级 as-of 语义（任职时间线 label）：`docs/dev-plans/063-assignment-timeline-org-labels-by-effective-slice.md`
- 任职经历列表新增长名称列（局部用例）：`docs/dev-plans/063A-assignments-timeline-org-long-name-column.md`
- 组织节点详情页长名称（拼接/兜底规则参考）：`docs/dev-plans/065-org-node-details-long-name.md`
- Valid Time day 粒度与迁移停止线：`docs/dev-plans/064-effective-date-day-granularity.md`、`docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`（064A 已完成：Org 已收敛为 date-only）

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 建立可复用的“组织长名称投影”能力：输入 `(tenant_id, org_node_id, as_of_day)`，输出 `org_node_long_name`（root→self，分隔符 ` / `）。
- [ ] 提供 **批量解析** 接口，确保列表/报表不会因“每行取路径”引入 N+1（查询次数与行数无关，或仅与 `distinct(as_of_day)` 有关）。
- [ ] 明确 **as-of 正确性**：长名称必须基于 `as_of_day` 查询当时的 `org_edges`（上级链）与 `org_node_slices`（名称切片），不得返回“当前最新路径”覆盖历史语义。
- [ ] 统一拼接/兜底规则（与 065 对齐）：`name` 为空回退 `code`，再回退 `id`。
- [ ] 对齐 064A（已完成）结论：Valid Time 对外只接受 `YYYY-MM-DD`（day）；本能力不引入/扩散 `effective_on/end_on`（且禁止重新引入双轨字段名与 `tstzrange` 口径）。

### 2.2 非目标 (Out of Scope)
- 不新增持久化字段（不在表上新增 `long_name` 存储列），仅在读时派生。
- 不要求“一条任职行里自动呈现多个时期的路径变化”（例如部门更名/上级变更导致同一任职有效期内部路径多次变化）。此类场景通过 **切换页面/报表的 `effective_date`（as-of day）** 查看该任职行在某一天对应的路径快照（见 §6.3 示例）。
- 不在本计划内把所有历史页面一次性迁移为新能力；本计划只定义 SSOT 能力与最小落地路径，迁移按页面/报表分批执行。

## 2.3 工具链与门禁（SSOT 引用）
> 目的：本文不复制命令清单；仅声明本计划命中的触发器，命令入口以 `AGENTS.md`/`Makefile` 为准。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码（见 `AGENTS.md`）
  - [ ] DB 读查询/可能涉及 schema（若引入 SQL function/view，则按 `AGENTS.md` 的 DB 门禁执行）
  - [X] 文档（本计划）：已执行 `make check doc`（docs gate: OK，2025-12-28 05:50 UTC；docs gate: no new files detected，2025-12-28 07:27 UTC；docs gate: no new files detected，2025-12-28 07:44 UTC；docs gate: no new files detected，2025-12-28 08:01 UTC）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
    A[任意列表/报表 Controller/Service] --> B[labelAsOfDay 选择(按行/按报表)]
    B --> C[pkg/orglabels: OrgNodeLongNameProjector]
    C --> D[(DB: org_edges + org_node_slices + org_nodes)]
    C --> E[map[org_node_id]long_name]
    E --> F[ViewModel/DTO]
    F --> G[templ/JSON 输出]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：SSOT 落点（选定：共享投影能力，而非页面各自实现）**
  - 选项 A：每个页面单独调用 `OrgService.GetNodePath(asOf)` 并拼接 → 规则易漂移，且按行调用会引入典型 N+1。
  - 选项 B（选定）：建立“组织长名称投影（projector/resolver）”作为共享读能力，集中拼接/兜底规则，并提供批量入口。
- **决策 2：计算时机（选定：读时派生，批量 hydrate）**
  - 选项 A：写时持久化 long_name → 写放大且祖先变更会级联影响大量后代，复杂度高。
  - 选项 B（选定）：读时派生；通过批量查询与组装避免 N+1。
- **决策 3：as-of 的来源（选定：对齐 063 的“行级 labelAsOfDay”）**
  - 默认使用“行起始日”作为该行的稳定快照；当页面/报表 `effective_date` 落在该行有效期内时，允许用页面 as-of 查看该行在任意一天的快照（但不拆分任职行）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 输入/输出模型（逻辑）
- 输入：`tenant_id` + `org_node_id` + `as_of_day`（UTC day，`YYYY-MM-DD`）。
- 输出：`org_node_long_name`（字符串）。

### 4.2 DB/Schema
- Phase 1（推荐最小落地）：不新增表/列，仅复用：
  - 组织关系：`org_edges`（Valid Time：`date effective_date/end_date` 闭区间；读时获取 `path/depth`）
  - 组织名称切片：`org_node_slices`（Valid Time：`date effective_date/end_date` 闭区间；读时获取 `name`）
  - 组织编码：`org_nodes.code`
- Phase 2（可选）：若 SQL 报表需要“可 join 的长名称”，再单独开计划引入 DB function/view（避免提前引入迁移与门禁负担）。

## 5. 接口契约 (Contracts)
### 5.1 Go 侧共享能力（建议 SSOT）
> 目的：让各模块只依赖 `pkg/**`，避免跨模块导入 `modules/org/**` 破坏 cleanarchguard。

- 新增包：`pkg/orglabels`（组织长名称投影 SSOT）
- **依赖约束（必须）**：
  - `pkg/orglabels` 仅依赖 `pkg/**` 与 DB 驱动（pgx），不得导入 `modules/org/**`（避免跨模块依赖与 cleanarchguard 风险）。
  - 查询通过 `composables.UseTx`（优先）或 `composables.UsePool` 获取连接，继承调用方事务边界（避免“看不见的二次连接”）。
- **对外接口（确定版）**：
  - `ResolveOrgNodeLongNamesAsOf(ctx, tenantID uuid.UUID, asOfDay time.Time, orgNodeIDs []uuid.UUID) (map[uuid.UUID]string, error)`：单 as-of day，**预算：1 query**。
  - `ResolveOrgNodeLongNames(ctx, tenantID uuid.UUID, queries []OrgNodeLongNameQuery) (map[OrgNodeLongNameKey]string, error)`：mixed as-of（pair-batch），**预算：1 query**（不得按 `queries` 循环，也不得按 `distinct(as_of_day)` 分组后逐组查询）。
  - `type OrgNodeLongNameQuery struct { OrgNodeID uuid.UUID; AsOfDay time.Time }`
  - `type OrgNodeLongNameKey struct { OrgNodeID uuid.UUID; AsOfDate string }`（`AsOfDate` 必须归一化为 `YYYY-MM-DD`）
- 失败路径：对单个节点缺失/路径缺失不 panic；由调用方决定“空值/—”兜底（UI 不应 500）。

### 5.2 拼接/兜底规则（SSOT）
对路径节点数组 `path.nodes[]`（root→self）逐段取值：
1) `name`（trim）非空则用；
2) 否则回退 `code`（trim）；
3) 否则回退 `id`（UUID string）；
4) 用 ` / ` 连接为最终长名称。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 labelAsOfDay 选择（对齐 063）
适用于“按任职行切片展示”的列表（如任职经历/时间线），定义：
- `pageAsOfDay`：页面 query 的 `effective_date` 归一化为 UTC day；
- `rowStartDay`：行的 `EffectiveDate` 归一化为 UTC day；
- `rowEndDay`：行的 `EndDate` 归一化为 UTC day（对外 `end_date`，day 闭区间右端点；open-ended 为 `9999-12-31`）。

规则（伪代码）：
```go
labelAsOfDay := rowStartDay
if !pageAsOfDay.Before(rowStartDay) && !pageAsOfDay.After(rowEndDay) {
    labelAsOfDay = pageAsOfDay
}
```

### 6.2 批量投影（避免 N+1）
对“同一 as-of day 的多个 org_node_id”，用 **一次查询** 获取每个节点的 root→self 路径段，并在 DB/Go 侧聚合为 long_name。

约束：不得对每行调用 `GetNodePath`（会导致每个节点至少 3 次 DB round-trip：`NodeExistsAt` + `ListAncestorsAsOf` + `ListOrgNodesAsOf`）。

对比（现有模式参考）：DEV-PLAN-063 已将 `/org/assignments` 时间线“部门/职位 label”做成 **repo SQL 联表一次取回**，且仅对“pageAsOf 命中行”做有限覆盖查询，避免随行数线性增长；068 的长名称投影必须沿用这一思路：**批量 hydrate**，不把路径解析隐藏在模板或逐行 service 调用里。

#### 6.2.1 单 as-of day（1 Query）
适用：报表/列表以“单一 `effective_date`（as-of）”渲染全表（例如花名册、薪资明细表某日快照）。

SQL 形状（示意，真实实现以 `pkg/orglabels` 为准）：
```sql
WITH target AS (
  SELECT e.child_node_id AS org_node_id, e.path
  FROM org_edges e
  WHERE e.tenant_id=$1
    AND e.hierarchy_type='OrgUnit'
    AND e.child_node_id = ANY($2::uuid[])
    AND e.effective_date <= $3::date
    AND e.end_date >= $3::date
),
ancestors AS (
  SELECT t.org_node_id, e.child_node_id AS ancestor_id, e.depth
  FROM target t
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.effective_date <= $3::date
   AND e.end_date >= $3::date
   AND e.path @> t.path
),
parts AS (
  SELECT a.org_node_id, a.depth,
         COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM ancestors a
  JOIN org_nodes n
    ON n.tenant_id=$1 AND n.id=a.ancestor_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id=$1 AND ns.org_node_id=a.ancestor_id
   AND ns.effective_date <= $3::date AND ns.end_date >= $3::date
)
SELECT org_node_id,
       string_agg(part, ' / ' ORDER BY depth ASC) AS long_name
FROM parts
GROUP BY org_node_id;
```

#### 6.2.2 mixed as-of（pair-batch，1 Query）
适用：同一页面/列表内，每行的 `labelAsOfDay` 可能不同（例如任职时间线：默认行起始日，且允许在“有效期内”用页面 `effective_date` 查看快照）。

约束：不得按 `distinct(as_of_day)` 分组逐组查询（在“每行一个起始日”的时间线场景会退化为近似 N 次查询）。

SQL 形状（示意）：
```sql
WITH input AS (
  SELECT *
  FROM unnest($2::uuid[], $3::date[]) AS q(org_node_id, as_of_day)
),
target AS (
  SELECT i.org_node_id, i.as_of_day, e.path
  FROM input i
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_day
   AND e.end_date >= i.as_of_day
),
ancestors AS (
  SELECT t.org_node_id, t.as_of_day, e.child_node_id AS ancestor_id, e.depth
  FROM target t
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.effective_date <= t.as_of_day
   AND e.end_date >= t.as_of_day
   AND e.path @> t.path
),
parts AS (
  SELECT a.org_node_id, a.as_of_day, a.depth,
         COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM ancestors a
  JOIN org_nodes n
    ON n.tenant_id=$1 AND n.id=a.ancestor_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id=$1 AND ns.org_node_id=a.ancestor_id
   AND ns.effective_date <= a.as_of_day AND ns.end_date >= a.as_of_day
)
SELECT org_node_id,
       as_of_day,
       string_agg(part, ' / ' ORDER BY depth ASC) AS long_name
FROM parts
GROUP BY org_node_id, as_of_day;
```

### 6.3 示例：不拆分任职行，只看快照
假设工号 `004` 有一条任职行：`2025-12-01 → 2025-12-31`（同一部门节点 `A`），且：
- `2025-12-15` 起部门 `A` 更名；
- `2025-12-20` 起 `A` 的上级部门调整（路径变化）。

则访问：
- `effective_date=2025-12-10`：该行显示 **2025-12-10** 的长路径快照（更名前/调整前）。
- `effective_date=2025-12-28`：同一行显示 **2025-12-28** 的长路径快照（更名后/调整后）。
- `effective_date=2026-01-10`（不在该行有效期内）：该行回退为按 **行起始日 2025-12-01** 的快照，避免历史行漂移为“最新路径”。

> 明确：本计划不实现“在同一任职行内自动呈现有效期内部的多次路径变化”；路径变化通过切换 `effective_date` 查看。

## 7. 安全与鉴权 (Security & Authz)
- 本能力为只读派生数据：不新增/修改 Casbin policy。
- 数据隔离：所有查询必须包含 `tenant_id`；不得跨租户拼装路径。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖（SSOT）**：
  - Valid Time day 语义与停止线：`docs/dev-plans/064-effective-date-day-granularity.md`、`docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`

### 8.0 实施前置条件（Readiness）
> 本节用于回答“现在是否能开始写代码”。任一项未满足时，应先补齐/澄清，再进入实现阶段（避免 Vibe Coding）。

1. [ ] **Contract First**：`DEV-PLAN-068`（本文件）已合并到 `main`，且包含：
   - date-only（对齐 064A）口径；
   - pair-batch（mixed as-of）单 query 约束；
   - query budget 数值（1 query）与留证方式；
   - 现有使用点盘点与改造策略（§8.1）。
2. [ ] **Schema/索引就绪**（无需新增迁移即可支撑 1-query 投影）：
   - `org_edges.path` 为 `ltree`，且存在 `org_edges_tenant_path_gist_idx`；
   - `org_edges.effective_date/end_date` 为 `date`，并按 day 闭区间查询；
   - `org_node_slices_tenant_node_effective_idx` 存在（as-of lookup 不退化为全表扫描）。
3. [ ] **接口输入已收紧**：Org Valid Time 输入仅接受 `YYYY-MM-DD`（064A 已完成），实现中不得回退为 RFC3339 兼容分支。
4. [ ] **性能门禁可执行**：确定 query budget test 的落点与复用数据种子（参考 `modules/org/services/org_057_reports_query_budget_test.go`），并能在 CI/本地可选跳过策略下稳定运行。
5. [ ] **影响面确认**：完成一次全仓盘点并更新 §8.1（如有新增）：
   - `rg -n "GetNodePath\\(" modules pkg` 的命中点与“是否需要改造”的结论一致；
   - 新增/改造的页面不得在 `.templ` 内逐行调用 service 取长路径。
- **里程碑（建议按 PR 切分）**：
  1. [ ] 引入 `pkg/orglabels`（批量解析 + 拼接/兜底 SSOT）。
  2. [ ] 为 1 个“高行数”用例补齐 query budget 测试，证明无 N+1（参考 `modules/org/services/org_057_reports_query_budget_test.go`）。
  3. [ ] 逐步迁移：优先替换 065（节点详情长名称）与 063A（任职经历长名称列）的 per-node `GetNodePath` 为批量 projector（如证明性能有风险）。
  4. [ ] 形成复用清单：在新报表/列表中禁止按行取路径，统一通过 projector。

### 8.1 现有使用点盘点与改造策略
> 目的：落地时避免“先实现新能力、旧代码继续各自为政”，同时避免无收益的重构。

**截至 2025-12-28：仓库内已使用组织长路径/长名称的入口**
- Org 节点详情面板（DEV-PLAN-065）：`modules/org/presentation/controllers/org_ui_controller.go` 的 `orgNodeLongNameFor` 调用 `OrgService.GetNodePath` 拼接单节点长名称（单节点、单次调用，不构成 N+1）。
- Org API（DEV-PLAN-033）：`GET /org/api/nodes/{id}:path` 提供路径查询（主要用于调试/交互），不直接提供 long_name。

**是否需要改造（结论）**
- 节点详情面板：不强制改造（单节点查询可接受）；但建议在 `pkg/orglabels` 落地后将拼接/兜底逻辑迁移为复用投影，减少重复与未来漂移。
- 任职经历列表“组织长名称”列（DEV-PLAN-063A）：若采用 per-row `GetNodePath` 将形成典型 N+1，必须改为使用 `pkg/orglabels` 批量投影（优先 `ResolveOrgNodeLongNames` 的 pair-batch），或等价的单 SQL hydrate。
- 未来新增列表/报表：禁止在 `.templ` 内逐行调用 service；必须在 controller/service 层批量 hydrate（并以 query budget test 留证）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **正确性（必须）**
  - [ ] 在“部门更名/上级调整”的样例数据下，不同 `effective_date` 看到的长路径不同且与当日一致（而非最新路径）。
  - [ ] 当 `effective_date` 不落在任职行有效期内，历史行回退为行起始日快照（避免全表漂移）。
  - [ ] 覆盖右闭边界：当 `effective_date == end_date`（day）时，该行仍应被视为“区间内”，并按该日渲染长路径快照。
- **性能（必须）**
  - [ ] 在 1000 节点规模（或等价高行数报表）下，解析长名称的 DB 查询次数为常数：`ResolveOrgNodeLongNamesAsOf` 为 1 query；`ResolveOrgNodeLongNames`（pair-batch）为 1 query；不得随行数或 `distinct(as_of_day)` 增长。

## 10. 运维与监控 (Ops & Monitoring)
- 不引入 Feature Flag/监控项（仓库级原则见 `AGENTS.md`）；以 query budget 测试与门禁保证性能退化可见。

## 11. DEV-PLAN-045 评审（Simple > Easy）
### 结构（解耦/边界）
- [x] SSOT 清晰：把“组织长名称”定义为共享投影能力，避免页面各自实现导致漂移。
- [x] 边界清晰：仅解决“长名称读取与批量化”，不引入写时派生/持久化写放大。
- [x] 依赖可预测：`pkg/orglabels` 不导入 `modules/org/**`，跨模块复用不会形成循环依赖或隐式跨层导入。

### 演化（规格/确定性）
- [x] 规格可执行：定义输入输出、拼接规则、labelAsOfDay 语义、以及“不拆分任职行”的明确非目标。
- [x] 契约确定：接口签名、key 结构与 query budget 已写死（`ResolveOrgNodeLongNamesAsOf`=1 query；`ResolveOrgNodeLongNames`(pair-batch)=1 query）。
- [x] mixed as-of 不退化：明确 pair-batch 单 query，并禁止按 `distinct(as_of_day)` 分组逐组查询。
- [ ] 若 pair-batch 在极端输入规模下不可接受，应先更新本计划（例如引入 DB function/view 并纳入迁移门禁），再改实现，避免即兴绕过预算。

### 认知（本质/偶然复杂度）
- [x] 本质复杂度明确：路径与名称都是 effective-dated，必须按 as-of day 取当时快照。
- [x] 偶然复杂度隔离：与 064A 的迁移期字段只通过停止线约束进入本计划，不把双轨列作为新对外概念扩散。
- [x] 对齐 064A：Org Valid Time 为 `date effective_date/end_date`（day 闭区间），Valid Time 输入只接受 `YYYY-MM-DD`。

### 维护（可理解/可解释）
- [x] 5 分钟可解释：确定 as-of → 批量取 path nodes → 按规则拼接 → hydrate 到 viewmodel/DTO。
- [x] 迁移口径明确：§8.1 盘点现有使用点与改造优先级（避免“新能力落地但旧逻辑继续漂移”）。

结论：通过（实现阶段必须用 query budget test 留证；且禁止在 `.templ` 内按行调用 service，所有长名称必须在 controller/service 层批量 hydrate）。 
