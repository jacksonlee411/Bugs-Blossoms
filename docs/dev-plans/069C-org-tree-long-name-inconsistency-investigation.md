# DEV-PLAN-069C：调查“组织架构树”与“组织长名称（long_name）”不一致

**状态**：已收口（2025-12-29 13:45 UTC）— 结论已固化，相关实现已合并到 main（PR #165、#166）

> 目标定位：这是“调查/证据固化”文档，用于复现、定位并归因“同一 as-of（effective_date）下，组织架构树（parent 显示/树路径）与组织长名称（long_name）不一致”的问题；并给出可执行的修复选项与验收口径。实现计划应落到后续 DEV-PLAN（或对既有 069/069A 的补充 PR）。

## 1. 背景与上下文 (Context)

- Org UI 在 `effective_date=YYYY-MM-DD` 视角下渲染：
  - 左侧树/上级显示：通常来自 as-of 的层级读取（例如 `org_edges.parent_node_id` 或树查询结果）。
  - 节点长名称（root→self）：通过 `pkg/orglabels.ResolveOrgNodeLongNames*`（读时派生）。
- 当两者在同一 as-of 下不一致时，会造成：
  - 用户认知冲突：页面同时展示两个相互矛盾的“组织上下文”。
  - 报表/导出可信度下降：长名称被作为“路径语义”复用，但可能是错误链路。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] 给出可复现路径：明确 “哪个 tenant / 哪个节点 / 哪个 effective_date / 哪个页面” 能稳定复现不一致。
- [ ] 固化证据：用可复现 SQL/日志证明“不一致来自数据或读模型”，而不是 UI 拼接差异。
- [ ] 归因到最小不变量破坏点：例如 `org_edges.path/depth` 与 `parent_node_id`/切片写入之间的漂移，或读路径选取的 as-of 口径不一致。
- [ ] 给出修复选项与决策点：在线止血（写路径）、存量修复（离线/批处理）、读时兜底（仅临时）等。
- [ ] 定义“关闭条件”（见 §10）：完成调查时 reviewer 能复述根因与验证方案，并能驱动后续实现计划落地。

### 2.2 非目标（Out of Scope）

- 不在本文件内直接落地代码修复；实现应由单独 PR 或对应 dev-plan 承载。
- 不新增数据库表；如后续修复需要 DDL/迁移，必须按仓库规则获得手工确认并另起计划/PR。

### 2.3 工具链与门禁（SSOT 引用）

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- 本地开发服务编排与端口：`devhub.yml`（默认 web `:3200`，db `:5438`）

## 3. 现象定义（Symptom）

在同一 `effective_date = D` 下，出现以下任一情况即视为命中：

- **树视角**：节点详情里的“上级显示（parent label）”指向 `P1`（或树路径显示为 `... / P1 / Self`）。
- **长名称视角**：long_name 解析结果为 `... / P2 / Self`，且 `P2 != P1`；或 long_name 缺段（例如缺少中间祖先），导致与树路径解释冲突。

需要在调查中明确“树视角”的数据来源与口径（见 §6），否则容易把“读路径口径差异”误判为“数据不一致”。

## 4. 相关计划与依赖 (Related)

- DEV-PLAN-069：`org_edges.path/depth` 一致性修复（把 path/depth 定义为结构索引 SSOT）
- DEV-PLAN-069A：基于 `org_edges.path` 的 path-driven long_name（读时派生优化）
- DEV-PLAN-068：组织长名称投影（SSOT 能力与契约）

> 注意：如果 069 的 Gate 未达成，则 long_name 与树路径的“不一致”可能是稳定的、可重复的；但其根因很可能是 `path/depth` 失真，而不是 068/069A 的拼接逻辑。

## 5. 复现入口（UI/HTTP）

记录以下信息（复现时必须填写，避免“对话式漂移”）：

- 环境：本地 / staging / prod（若非本地，需说明数据快照来源与脱敏方案）
- tenant_id：
- effective_date（as-of day）：
- node_id：
- 页面 URL（建议）：`/org/nodes?effective_date=YYYY-MM-DD&node_id=<uuid>`
- 屏幕证据：截图路径（建议放 `docs/assets/069c/`，本计划阶段可先不提交图片）

### 5.1 已登记复现样本（持续追加）

#### 样本 1：丘比2 的“树位置 / 上级显示 / 任职路径”互相矛盾

- 环境：本地（web `:3200`）
- effective_date（as-of day）：`2025-12-28`
- node_id：`93844c02-fd34-42a5-91fe-1a0d9c8e9658`
- 复现入口：
  - 节点详情：`http://localhost:3200/org/nodes?effective_date=2025-12-28&node_id=93844c02-fd34-42a5-91fe-1a0d9c8e9658`
  - 任职列表（用于对照组织路径）：`http://localhost:3200/org/assignments?effective_date=2025-12-28&pernr=004`
- 观察到的不一致（同一 as-of 下）：
  - 组织架构树：`丘比2` 显示在 `财务部` 之下。
  - 组织详情：上级部门却显示为 `人力资源部`。
  - 任职列表（`pernr=004`）：存在记录 `004  2025-12-10 → 至今  调动  AI治理办公室 (2)  飞虫与鲜花 / 丘比2 / AI治理办公室  02 — 副总经理`（其路径语义又与上述“树位置/上级显示”存在冲突点，需进一步用 SQL 固化对照）。
- 待补信息：
  - tenant_id（以及是否存在多租户/多层级类型混淆）
  - 截图证据（如需要，放 `docs/assets/069c/`）

**样本 1：已固化 DB 证据（本地 `iota_erp`，as-of=`2025-12-28`）**

- tenant_id：`00000000-0000-0000-0000-000000000001`
- 关键节点（按当前 slice 名称）：
  - Root：`飞虫与鲜花`（`b625ee4e-b201-41a4-ae11-e4adf814f9e5`）
  - `人力资源部`（`cc8d5c7a-ab6b-457c-8afc-4bc62b0217f6`）
  - `财务部`（`efa34bf5-2fcf-49fd-a5dd-8a1a267b1011`）
  - `丘比2`（`93844c02-fd34-42a5-91fe-1a0d9c8e9658`）
  - `AI治理办公室`（`bc70deb0-ca70-4102-b7a6-2c90ac87bafa`）
- as-of=`2025-12-28` 下的真实结构（`org_edges`）：
  - `丘比2` 的 `parent_node_id = 人力资源部`（path=`飞虫与鲜花 / 人力资源部 / 丘比2`，depth=2）
  - `AI治理办公室` 的 `parent_node_id = 丘比2`（但其 path=`飞虫与鲜花 / 丘比2 / AI治理办公室`，depth=2；缺失 `人力资源部`，且不满足 `child.depth = parent.depth + 1`）
- 直接导致的 UI 侧“矛盾”：
  - **任职路径**：`/org/assignments` 展示的 long_name 来自 `org_edges.path` 解析，因此会显示 `飞虫与鲜花 / 丘比2 / AI治理办公室`（缺段）。
  - **树位置错觉**：当前树组件是“按 depth 缩进的扁平列表”，而非按 `parent_id` 真实分组；同时层级查询按 `depth ASC, name ASC` 排序（非 pre-order），在本样本中 `财务部` 恰好是 depth=1 的最后一项，导致后续 depth=2 节点（含 `丘比2`）在视觉上被“挂到财务部下面”。
- 时间线证据（`org_audit_logs`）：
  - `2025-12-28 10:13:13Z`：`AI治理办公室`（`bc70...`）执行 `Move`，effective_date=`2025-12-10`，从 Root 移到 `丘比2`。
  - `2025-12-28 10:28:51Z`：`丘比2`（`9384...`）执行 `Move`，effective_date=`2025-12-06`，从 Root 移到 `人力资源部`（这是一次“回溯生效日”的上级变更）。
  - 结论：在“先移动后代、再回溯移动祖先”的序列下，`AI治理办公室` 的未来切片 `org_edges.path/depth` 未被级联重写，导致 long_name 与树/上级展示口径出现稳定冲突。

## 6. 数据来源与口径对齐（关键）

调查前必须先回答：在 as-of=D 下，页面中以下字段分别来自哪里？

### 6.1 最小映射表（以当前实现为准，避免“对话式漂移”）

| 视角/字段 | 当前入口（UI/代码） | 数据来源（表/字段） | as-of 口径 | 备注（常见误判点） |
| --- | --- | --- | --- | --- |
| 树列表（左侧） | `OrgService.GetHierarchyAsOf` → `OrgRepository.ListHierarchyAsOf*` → `mappers.HierarchyToTree` | `org_edges.parent_node_id` + `org_edges.depth` + `org_node_slices.display_order/name` | `effective_date <= D AND end_date >= D`（以 `D::date` 过滤） | Tree 仍是“按 depth 缩进的扁平列表”，但 **`HierarchyToTree` 已按 `ParentID` 做 pre-order 输出**（同一 parent 下按 `display_order/name` 排序），避免“树位置错觉”。 |
| 节点详情：上级显示（ParentLabel） | `getNodeDetails` → `GetNodeAsOf`（node slice） + `GetHierarchyAsOf`（edges parent）→ `details.ParentLabel` | 优先使用 `org_edges.parent_node_id`（同 as-of），仅在缺失时 fallback `org_node_slices.parent_hint` | `effective_date <= D AND end_date >= D`（以 `D::date` 过滤） | 与树对齐：上级展示默认以 edges 的真实 parent 为准，降低 H3 噪声。 |
| 节点详情：long_name | `orglabels.ResolveOrgNodeLongNamesAsOf` | 依赖 `org_edges.path`（祖先定位/解析）+ `org_node_slices.name`（祖先命名切片） | 应与页面 effective_date 同步 | path 失真会产生“稳定但错误”的缺段/错段（H1）；若后续切换 069A 形状，对 069 Gate 的依赖更强。 |

> 注：树列表底层 SQL 仍可能按 `depth ASC, display_order ASC, name ASC` 返回（非 pre-order），但 UI 侧已按 `ParentID` 重新排序；真实结构仍以 `org_edges.parent_node_id`（同 as-of）为准。

### 6.2 需要明确的“口径分叉点”（先排除，再谈一致性）

- 树/上级显示：
  - 口径 A：来自 `org_edges`（as-of=D 的 `parent_node_id`）
  - 口径 B：来自 `org_node_slices.parent_hint`（展示提示字段，可能滞后）
  - 口径 C：来自 `GetHierarchyAsOf` 等“树查询”的输出（其内部可能选择 path 或 edges 读策略）
- long_name：
  - `pkg/orglabels.ResolveOrgNodeLongNamesAsOf/ResolveOrgNodeLongNames`（其 SQL 是否采用 069A path-driven 形状）

若树视角与 long_name 视角的“as-of 口径”不同（例如一个用 `effective_date`，另一个用系统当前时间/或另一字段），则首先应视为“读路径口径 bug”，而不是数据一致性问题。

## 7. 假设列表（Hypotheses）

按优先级从高到低（每条必须能被 SQL/日志证伪）：

1. **H1：`org_edges.path/depth` 跨切片失真**  
   典型触发：回溯生效日 Move/CorrectMove 导致子树后代的未来切片 path 未被级联重写；long_name 依赖 path 做祖先定位或拆分，从而出现缺段/错段。
2. **H2：树读取策略与 long_name 读取策略不一致**  
   例如树使用 `OrgReadStrategy=edges`，而 long_name 走 `path`；或两者对“有效期包含”使用了不同闭区间/半开区间口径。
3. **H3：`org_node_slices.parent_hint` 与 `org_edges.parent_node_id` 不一致**  
   UI 优先使用 parent_hint 展示，但 long_name 走 edges/path，造成展示不一致（此时 long_name 可能是对的，也可能不是）。
4. **H4：多棵树/多层级类型混淆**  
   `hierarchy_type` 过滤不一致（例如某处缺失 `'OrgUnit'` 限制），导致祖先链落在不同层级域。

## 8. SQL 证据收集（可复制执行）

> 说明：以下 SQL 用于调查与证据固化；执行前用实际值替换 `:tenant_id/:as_of/:node_id`。

### 8.1 as-of 当天的边切片（目标节点）

```sql
SELECT
  tenant_id,
  hierarchy_type,
  parent_node_id,
  child_node_id,
  effective_date,
  end_date,
  depth,
  path::text AS path_text
FROM org_edges
WHERE tenant_id = :tenant_id
  AND hierarchy_type = 'OrgUnit'
  AND child_node_id = :node_id
  AND effective_date <= :as_of::date
  AND end_date >= :as_of::date
ORDER BY effective_date DESC
LIMIT 5;
```

### 8.2 对照：parent_hint（展示提示）与 node slice（名称）

```sql
SELECT
  ns.tenant_id,
  ns.org_node_id,
  ns.parent_hint,
  ns.name,
  ns.effective_date,
  ns.end_date
FROM org_node_slices ns
WHERE ns.tenant_id = :tenant_id
  AND ns.org_node_id = :node_id
  AND ns.effective_date <= :as_of::date
  AND ns.end_date >= :as_of::date
ORDER BY ns.effective_date DESC
LIMIT 5;
```

### 8.3 审计时间线：定位 Move/CorrectMove/删除/缝补 的因果顺序（`org_audit_logs`）

> 说明：`org_edge` 的审计 `entity_id` 是 **edge_id**，不是 `child_node_id`；因此按 node 追踪时需从 `old_values/new_values` 里筛 `child_node_id`。

```sql
-- 方式 A：已知 request_id（最推荐，完整还原一次写事务）
SELECT
  transaction_time,
  change_type,
  meta->>'operation' AS operation,
  entity_type,
  entity_id,
  effective_date,
  end_date,
  old_values,
  new_values,
  meta
FROM org_audit_logs
WHERE tenant_id = :tenant_id
  AND request_id = :request_id
ORDER BY transaction_time ASC;

-- 方式 B：按 node_id 追踪 edge 变更（Move/CorrectMove/删除等）
SELECT
  transaction_time,
  change_type,
  meta->>'operation' AS operation,
  entity_id AS edge_id,
  effective_date,
  end_date,
  old_values->>'parent_node_id' AS old_parent_node_id,
  new_values->>'parent_node_id' AS new_parent_node_id,
  COALESCE(new_values->>'child_node_id', old_values->>'child_node_id') AS child_node_id,
  request_id
FROM org_audit_logs
WHERE tenant_id = :tenant_id
  AND entity_type = 'org_edge'
  AND (
    new_values->>'child_node_id' = :node_id::text
    OR old_values->>'child_node_id' = :node_id::text
  )
ORDER BY transaction_time DESC
LIMIT 200;
```

### 8.4 结构不变量：父 path 应为子 path 的前缀（同 as-of）

```sql
WITH target AS (
  SELECT *
  FROM org_edges
  WHERE tenant_id = :tenant_id
    AND hierarchy_type='OrgUnit'
    AND child_node_id=:node_id
    AND effective_date <= :as_of::date
    AND end_date >= :as_of::date
  ORDER BY effective_date DESC
  LIMIT 1
),
parent AS (
  SELECT *
  FROM org_edges e
  JOIN target t ON t.parent_node_id = e.child_node_id
  WHERE e.tenant_id = :tenant_id
    AND e.hierarchy_type='OrgUnit'
    AND e.effective_date <= :as_of::date
    AND e.end_date >= :as_of::date
  ORDER BY e.effective_date DESC
  LIMIT 1
)
SELECT
  t.child_node_id AS node_id,
  t.parent_node_id AS parent_id,
  t.path::text AS child_path,
  p.path::text AS parent_path,
  CASE WHEN p.path IS NULL THEN NULL ELSE (p.path @> t.path) END AS parent_is_prefix,
  CASE WHEN p.path IS NULL THEN NULL ELSE (t.depth = p.depth + 1) END AS parent_child_depth_ok,
  (t.depth = nlevel(t.path)-1) AS child_depth_ok,
  (p.depth = nlevel(p.path)-1) AS parent_depth_ok
FROM target t
LEFT JOIN parent p ON true;
```

### 8.5 子树内潜在不一致扫描（只读调查版）

> 用目标节点 as-of 的 `path` 作为子树根，扫描同一天内满足 `path <@ root_path` 的边切片，查找 `depth != nlevel(path)-1` 或 “parent_path 不是 child_path 前缀” 的候选。

```sql
WITH root AS (
  SELECT e.path
  FROM org_edges e
  WHERE e.tenant_id = :tenant_id
    AND e.hierarchy_type='OrgUnit'
    AND e.child_node_id=:node_id
    AND e.effective_date <= :as_of::date
    AND e.end_date >= :as_of::date
  ORDER BY e.effective_date DESC
  LIMIT 1
),
subtree AS (
  SELECT e.*
  FROM org_edges e, root r
  WHERE e.tenant_id = :tenant_id
    AND e.hierarchy_type='OrgUnit'
    AND e.effective_date <= :as_of::date
    AND e.end_date >= :as_of::date
    AND e.path <@ r.path
)
SELECT
  c.child_node_id,
  c.parent_node_id,
  c.effective_date,
  c.end_date,
  c.depth,
  nlevel(c.path)-1 AS depth_calc,
  c.path::text AS path_text,
  p.path::text AS parent_path_text,
  CASE WHEN p.path IS NULL THEN NULL ELSE (p.path @> c.path) END AS parent_is_prefix,
  CASE WHEN p.path IS NULL THEN NULL ELSE (c.depth = p.depth + 1) END AS parent_child_depth_ok
FROM subtree c
LEFT JOIN org_edges p
  ON p.tenant_id = c.tenant_id
  AND p.hierarchy_type = c.hierarchy_type
  AND p.child_node_id = c.parent_node_id
  AND p.effective_date <= :as_of::date
  AND p.end_date >= :as_of::date
WHERE c.parent_node_id IS NOT NULL
  AND (
    c.depth <> nlevel(c.path)-1
    OR p.path IS NULL
    OR NOT (p.path @> c.path)
    OR c.depth <> p.depth + 1
  )
ORDER BY effective_date DESC
LIMIT 200;
```

## 9. 预期结论形态（输出要求）

调查结束时，必须产出以下最小集合（否则不允许“关闭”）：

- 根因归类：命中 §7 的哪条假设（可多选），以及证据点。
- 最小反例：一个能复现的不一致样本（tenant_id/node_id/as_of）。
- 口径对齐结论：补齐 §6.1 的“字段→来源”映射，并注明树查询当前使用的 `OrgReadStrategy`。
- 关键证据快照：至少包含 §8.1/8.2/8.4 的输出摘要；若涉及写入顺序/回溯生效日，补齐 §8.3 的审计时间线摘要。
- 修复建议：对应 069 的 A/C（或补充计划），并明确是否需要读时兜底（仅临时）。
- 风险评估：影响面（哪些页面/报表受 long_name 错误影响）与回滚策略。

## 10. 关闭条件（Exit Criteria）

- [X] 至少 1 个复现样本被记录（见 §5 “样本 1”），并已给出最小反例的 tenant/node/as-of 与审计线索。
- [X] 已明确“不一致”包含数据不变量破坏（H1）+ UI 视觉误判（树位置错觉），且已用 §6 + §8 的证据支撑。
- [X] 树位置错觉已收敛：树列表输出改为按 `ParentID` pre-order（同 parent 下按 `display_order/name`），避免把“视觉位置”误当真实 parent（PR #166）。
- [X] 已产出并合并可执行落地路径：
  - 写入止血/防复发：069 + 069B（PR #165；并已在 main）
  - 读口径对齐 + 树 pre-order：PR #166
  - 存量巡检/收敛：对 rollout tenant（`000...001`）执行 `scripts/org/069_org_edges_path_inconsistency_count.sql` / §8.4 类不变量扫描均为 0（本地 `iota_erp` 基线）
