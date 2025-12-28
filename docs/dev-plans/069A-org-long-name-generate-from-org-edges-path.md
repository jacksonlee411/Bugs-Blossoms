# DEV-PLAN-069A：基于 `org_edges.path` 生成组织长路径名称（更轻量的读时派生）

**状态**: 草拟中（2025-12-28 14:34 UTC）

## 背景与动机

- DEV-PLAN-068 的“组织长名称投影”是**读时派生**，但祖先定位依赖 `org_edges.path`（ltree）并通过 `org_edges` 自连接 `e.path @> t.path` 找出祖先集合再 `string_agg` 拼接（落地于 `pkg/orglabels/org_node_long_name.go` 的 `mixedAsOfQuery`）。
- DEV-PLAN-069 给出问题根因与修复方向：当存在“回溯生效日”移动/纠正祖先节点的写入时，`org_edges.path/depth` 可能跨时间切片失真，从而导致长名称缺段；推荐的 A/C 方案把 `org_edges.path/depth` 作为**结构索引 SSOT** 来维护一致性（写入止血 + 存量治理）。

本附录回答一个具体问题：在 069 的 A/C 落地后，组织长路径名称是否可以更高效地通过 `org_edges.path` 生成，而不是继续沿用 068 的“动态（但依赖自连接）”方案？

## 结论摘要

1. **可以更高效**：当 `org_edges.path` 在给定 `as_of_day` 上可被视为 root→self 的节点序列（且跨时间切片一致）时，长名称可用“拆分 `target.path` 得到节点序列 → join `org_node_slices/org_nodes` 取 as-of 名称 → `string_agg`”生成；无需再通过 `org_edges` 自连接扫描祖先边集合。
2. **不等同于“写时物化 long_name”**：该方式仍是读时派生，只是把 SQL 形状从“祖先定位依赖 `org_edges`”改为“祖先列表由 `path` 直接提供”，以降低 join 与扫描规模。
3. **仍必须 join `org_node_slices`**：`path` 只编码节点身份（uuid key），不包含“当日名称切片”；因此依然需要按 `as_of_day` 连接 `org_node_slices`（并按 068 兜底规则 `name → code → id`）。
4. **UUID 转换应走最短路径**：在本仓库 Docker PostgreSQL 17 上验证：32 位 hex（无连字符）可直接 `::uuid`；因此应优先使用 `label::uuid`，避免 `substr`/`regexp_replace` 的额外 CPU 成本。

## 前置条件（Gate）

本方案是否可作为 SSOT 的前提，是 069 的 A/C 是否达成：

- `org_edges.path/depth` 在跨时间切片满足结构不变量（父 path 是子 path 的前缀；`child_path = parent_path || child_key`；`depth = nlevel(path)-1`）。
- 存量不一致已通过巡检/离线修复降到可依赖水平，且写入不再引入新不一致。

若未满足 Gate，则**任何**依赖 `path` 的长名称生成（无论 068 还是本文方案）都可能缺段或误导；此时只能作为临时 UI 兜底，不得成为 SSOT。

## 方案：Path-driven 长名称生成（SQL 形状草案）

### 核心思路

对每个输入 pair `(org_node_id, as_of_day)`：

1. 只做一次 `org_edges` join，拿到 `target.path`（与 068 相同）。
2. 将 `target.path::text` 按 `.` 拆成 label 序列（root→self），并保留顺序（`WITH ORDINALITY`）。
3. 把每个 label 直接 `::uuid` 转回 `uuid`（Postgres 支持 32 位 hex 输入），再按 `as_of_day` join `org_node_slices`/`org_nodes` 取名称并拼接。

### Mixed as-of（pair-batch）示意 SQL

> 说明：此处只给“形状”，真实落地需对齐 068 的入参类型与 `tenant_id`/`hierarchy_type` 过滤；且**不应**在本文阶段引入 schema 变更。

```sql
WITH input AS (
  SELECT *
  FROM unnest($2::uuid[], $3::date[]) AS q(org_node_id, as_of_day)
),
target AS (
  SELECT
    i.org_node_id,
    i.as_of_day,
    e.path
  FROM input i
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_day
   AND e.end_date >= i.as_of_day
),
	path_parts AS (
	  SELECT
	    t.org_node_id,
	    t.as_of_day,
	    p.ord,
	    p.key_text,
	    -- key_text 的来源是：replace(lower(uuid::text), '-', '')（见 org_edges 触发器）。
	    -- 注意：ltree label 不允许 '-'，因此存储为 32 位 hex；Postgres 允许其直接 ::uuid（无需插连字符）。
	    p.key_text::uuid AS node_id
	  FROM target t
	  CROSS JOIN LATERAL unnest(string_to_array(t.path::text, '.')) WITH ORDINALITY AS p(key_text, ord)
	),
parts AS (
  SELECT
    pp.org_node_id,
    pp.as_of_day,
    pp.ord,
    COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM path_parts pp
  JOIN org_nodes n
    ON n.tenant_id=$1 AND n.id=pp.node_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id=$1 AND ns.org_node_id=pp.node_id
   AND ns.effective_date <= pp.as_of_day AND ns.end_date >= pp.as_of_day
)
SELECT
  org_node_id,
  as_of_day::text AS as_of_date,
  string_agg(part, ' / ' ORDER BY ord ASC) AS long_name
FROM parts
GROUP BY org_node_id, as_of_day;
```

## 性能收益与风险评估（定性）

### 预期收益

- **避免祖先边扫描**：不再需要 `org_edges` 自连接（`e.path @> t.path`）来定位祖先集合；祖先列表由 `target.path` 直接给出。
- **行数更可控**：输出的中间行数约为 `sum(depth(node)+1)`；在层级深度相对稳定（通常远小于 `org_edges` 规模）时，更接近“按深度线性”的成本。

### 风险与注意事项

- **UUID 转换成本（基于本仓库 Docker PG17 的验证结论）**：
  - `label::uuid` 是最低成本路径：不需要“插连字符”，也不应使用正则。
  - 微基准（`jit=off`，1,000,000 行 `text→uuid`，顺序扫描临时表）结果：`label::uuid` 显著快于 “`substr` 插连字符再转”，也显著快于 “`regexp_replace` 再转”。因此本文方案应以 `label::uuid` 作为默认实现形态。
  - 是否成为端到端瓶颈取决于输入规模与执行计划（hash 批次、work_mem、join 形态等）；建议用 `EXPLAIN (ANALYZE, BUFFERS)` 同时跑 068 现有查询与本文查询做对比评估。
- **为何必须依赖 UUID join，而非组织编码（code）join**：
  - `org_edges.path` 的 label 域是“uuid 去掉 `-` 的文本 key”（由触发器写入），并不等于 `org_nodes.code`；要从 path 还原到“节点身份”，最自然的是先恢复 `uuid`。
  - `org_node_slices` 的关联键是 `(tenant_id, org_node_id uuid)`，按 `as_of_day` 取名称切片必须以 `org_node_id` join；即使最终展示需要 `code` 兜底，`code` 也只是展示字段，不是 slice 的主关联键。
  - 用 `uuid` 做结构索引与 join 能把“结构身份”与“展示字段（name/code）”解耦：rename/展示字段变化不应影响结构索引；把 code 引入结构链路会引入额外编码/兼容性与历史语义风险。
- **工程化建议（行业常见做法）**：
  - 可将 `label::uuid` 封装为 `IMMUTABLE STRICT` SQL 函数（例如 `uuid_from_ltree_label(text)`），用于统一实现与基准测试；未来若引入映射/派生读模型，也便于替换。
- **一致性语义更强依赖 path SSOT**：本方案更“相信 `path`”，不会因为祖先边表中存在不一致而被动截断；因此必须以 069 的 Gate 为前置，否则会把错误路径当作事实链路展示。
- **需要对齐 068 的兜底规则与空值策略**：例如某些节点在 as-of 当天无 slice 时，应回退 `code/id`，且调用方不得因缺失而 500。

## 验证记录（UUID 转换能力与成本）

> 目的：为本文“默认实现应使用 `label::uuid`”提供可复现的依据；详细基准以 DB 实测为准。

- 环境：本仓库 `compose.dev.yml` 的 `postgres:17`（示例版本：17.7），`jit=off`。
- 行为验证：`SELECT '0123456789abcdef0123456789abcdef'::uuid;` 可直接成功（证明 32 位 hex 可直接 cast）。
- 成本结论（微基准）：对 1,000,000 行 `text→uuid`，`label::uuid` 显著快于 “`substr` 插连字符再 cast”，也显著快于正则替换再 cast；因此应避免在热路径做字符串重写。

## 验收标准（与 068 对齐）

在 069 Gate 满足的前提下：

1. 对相同输入（tenant、pairs），本文方案生成的 `long_name` 与 068 当前 `pkg/orglabels` 查询结果一致（字面一致）。
2. 对 069 典型回归案例（“回溯生效日移动祖先 + 存在未来切片”），在修复后 long_name 不再缺段，且不会出现“上级显示”与“长名称链路”解释冲突。

## 后续实施步骤（若进入编码）

> 本附录仅为分析与契约记录；是否实施需在 069 Gate 接近完成时再决策。

1. [ ] 在 `pkg/orglabels` 增加 path-driven 查询作为替代实现，并加入对照测试（与现有查询结果做 diff）。
2. [ ] 评估是否需要封装 `uuid↔key_text` 的稳定函数；如需引入 schema/索引优化，另起 dev-plan 并按仓库规则获得确认后实施。
