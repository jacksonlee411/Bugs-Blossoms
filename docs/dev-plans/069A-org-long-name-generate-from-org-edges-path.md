# DEV-PLAN-069A：基于 `org_edges.path` 生成组织长路径名称（更轻量的读时派生）

**状态**: 已实现（2025-12-29 03:27 UTC）— 已合并至 main（PR #159）；Readiness：`docs/dev-records/DEV-PLAN-069A-READINESS.md`

> 完成情况登记（以 main 为准）：
> - 默认查询已为 path-driven（拆 `target.path` 再 join slices）：`pkg/orglabels/org_node_long_name.go`
> - 对照一致性测试：`modules/org/services/org_069A_long_name_path_driven_equivalence_test.go`

## 1. 背景与上下文 (Context)
- **需求来源**:
  - DEV-PLAN-068：Org long_name 读时派生（当前实现位于 `pkg/orglabels/org_node_long_name.go` 的 `mixedAsOfQuery`）。
  - DEV-PLAN-069：`org_edges.path/depth`（ltree materialized path）跨有效期切片失真会导致 long_name 缺段；A/C 方案把 `org_edges.path/depth` 定义为结构索引 SSOT（写入止血 + 存量治理）。
- **当前痛点**:
  - 068 的 long_name 生成需要 `org_edges` 自连接（`e.path @> t.path`）定位祖先集合；在批量查询/数据规模增长时 join 与扫描开销更难控。
  - 当 `org_edges.path` 失真时（DEV-PLAN-069 的问题），任何依赖 `path` 的祖先定位都会给出“稳定但错误”的结果；因此 069A 的可行性强依赖 069 Gate。
- **业务价值**:
  - 在 069 Gate 成立后，用 `target.path` 直接提供祖先序列，避免祖先边扫描，使 long_name 的中间行数更接近 `sum(depth(node)+1)` 的线性成本，降低读路径负载与抖动风险。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**:
  - [X] 在 **不改变对外契约**（调用方式/返回语义）的前提下，优化 long_name 生成 SQL 形状：用 `target.path` 提供祖先序列，避免 `org_edges` 自连接扫描祖先集合。
  - [X] 将改动边界收敛到 `pkg/orglabels.ResolveOrgNodeLongNames`：不扩散到 Controller/Service/UI。
  - [X] 提供“新旧实现结果一致”的对照验证（自动化测试），并给出可回滚路径。
  - [X] 明确本计划命中的工具链/门禁触发器，并按 SSOT 执行与引用（见“工具链与门禁（SSOT 引用）”）。
- **非目标 (Out of Scope)**:
  - 不把 long_name 写时物化到写模型（不新增 long_name 字段/表）。
  - 不引入 schema 变更（DDL/迁移/新增索引/新增 SQL 函数）。若后续确需 DDL（例如引入 `uuid_from_ltree_label(text)`），必须另起 dev-plan，并按仓库规则获得确认后再实施。
  - 不绕过 069 Gate：在 069 的 A/C 未达成前，本方案不得成为 SSOT，仅可作为“临时 UI 兜底候选”（且需要明确容错策略与限制条件）。

## 2.1 工具链与门禁（SSOT 引用）
> 本文只声明“本计划命中哪些触发器/工具链”，命令细节以 `AGENTS.md` / `Makefile` / CI 为准。

- **触发器清单（勾选本计划命中的项）**：
  - [x] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] `.templ` / Tailwind（`make generate && make css`）
  - [ ] 多语言 JSON（`make check tr`）
  - [ ] Authz（`make authz-test && make authz-lint`）
  - [ ] 路由治理（`make check routing`）
  - [ ] DB 迁移 / Schema（本计划不做；如需 DDL，必须另起 dev-plan 并先获得手工确认）
  - [ ] sqlc（本计划不涉及）
  - [ ] Outbox（本计划不涉及）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`
  - 068 long_name：`docs/dev-plans/068-org-node-long-name-projection.md`
  - 069 path SSOT：`docs/dev-plans/069-org-long-name-parent-display-mismatch-investigation.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
    Caller[Org UI / Service / Reporting] --> L[ResolveOrgNodeLongNames]
    L --> DB[(Postgres)]
    DB --> E[org_edges (path/depth)]
    DB --> N[org_nodes (code)]
    DB --> NS[org_node_slices (name, valid-time)]
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：采用 path-driven 祖先展开（选定）**
  - **选项 A（现状）**：`org_edges` 自连接，用 `e.path @> t.path` 扫描祖先集合；优点：不需要解析 `path::text`；缺点：祖先定位需要扫描祖先边集合，join 形状更重。
  - **选项 B（选定）**：从 `target.path` 拆分出 root→self label 序列，顺序 join `org_node_slices/org_nodes` 再 `string_agg`；优点：中间行数更可控；缺点：需要 `text→uuid` cast，并更强依赖 path SSOT 不变量。
- **决策 2：不引入 DDL（选定）**
  - 不新增函数/索引/表，避免把一次查询形状优化升级为 schema 变更；若后续确需 DDL，另起 dev-plan。
- **决策 3：严格模式 vs 容错模式**
  - **严格模式（默认倾向，作为 SSOT 语义）**：`label::uuid` cast 失败直接报错，逼迫数据回到 069 Gate 不变量。
  - **容错模式（仅允许临时 UI 兜底）**：跳过非法 label，避免 500，但 long_name 可能缺段，不能当作事实链路。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划不改 Schema，但必须明确实现依赖的字段与不变量（否则实现会在“容错/兼容”上即兴发明复杂度）。

### 4.1 依赖的表与字段（只列关键项）
- `org_edges`
  - `tenant_id uuid`：所有查询必须包含 `tenant_id = $1`。
  - `hierarchy_type text`：本计划只针对 `OrgUnit`。
  - `child_node_id uuid`
  - `effective_date date` / `end_date date`：valid-time 切片（day 粒度）。
  - `path ltree`：materialized path（root→self），label 由 `replace(lower(child_node_id::text), '-', '')` 写入（见 `modules/org/infrastructure/persistence/schema/org-schema.sql` 的触发器）。
  - `depth int`：`nlevel(path)-1`。
- `org_nodes`
  - `id uuid` / `tenant_id uuid` / `code text`（展示兜底字段）。
- `org_node_slices`
  - `tenant_id uuid` / `org_node_id uuid`
  - `effective_date date` / `end_date date`
  - `name text`（展示字段，可能为空/空白）

### 4.2 Gate（不变量）
- 对任意 `(tenant_id, org_node_id, as_of_day)`，若存在生效边切片，则 `org_edges.path` 必须代表 root→self 的节点序列，且跨切片满足前缀关系与 `depth = nlevel(path)-1`。
- label 必须是 32 位 hex（无 `-`），可直接 `::uuid`。

### 4.3 迁移策略
- 本计划不包含 DDL/迁移。

## 5. 接口契约 (API Contracts)
> 本计划不改变 HTTP/UI 契约；唯一可观察变化应是性能与（在 069 Gate 成立后的）正确性更稳。

### 5.1 Go API: `pkg/orglabels.ResolveOrgNodeLongNames`
- **输入**：
  - `tenantID uuid.UUID`（必填，`uuid.Nil` 直接返回空 map）
  - `queries []OrgNodeLongNameQuery`：每项包含 `OrgNodeID` 与 `AsOfDay`
- **输出**：
  - `map[OrgNodeLongNameKey]string`：每个输入 pair 都有 key；若 SQL 未返回该 pair 的 row，则 value 为 `""`（对齐现状）。
- **错误**：
  - DB 查询错误向上冒泡；在“严格模式”下，若 `path` label 不能 cast 为 uuid，会导致查询失败（见 6.3）。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 现状基线（DEV-PLAN-068）
- 读取 target edge（child 在 as-of 的 `path` 与 `depth`），再用 `org_edges` 自连接扫描祖先集合（`e.path @> t.path`），并以相对深度排序 `string_agg`。

### 6.2 Path-driven 查询形状（选定方案）
对每个输入 pair `(org_node_id, as_of_day)`：
1. join `org_edges` 获取 `target.path`（与 068 相同过滤：`tenant_id`、`hierarchy_type`、有效期包含 as-of）。
2. 将 `target.path::text` 按 `.` 拆成 label 序列（root→self），用 `WITH ORDINALITY` 保序。
3. 将 label `::uuid` 还原为 `org_node_id`，再按 as-of join `org_node_slices` / `org_nodes`，按 `name → code → id` 兜底生成每段名称并拼接。

为何必须依赖 UUID join（而不是 `code` join）：
- `org_edges.path` 的 label 是 “uuid 去掉 `-` 的 32 位 hex”，它表达的是“结构身份”，不是业务编码；用 `code` 会把展示字段引入结构链路，带来 rename/历史语义与兼容性复杂度。
- `org_node_slices` 的主关联键是 `(tenant_id, org_node_id uuid)`，要按 `as_of_day` 取名称切片必须以 `org_node_id` join；`code` 只是展示兜底字段，不是 slice 的结构键。

**Mixed as-of（pair-batch）示意 SQL（保持与现状一致的 `$3::text[]` 入参）**：
```sql
WITH input AS (
  SELECT *
  FROM unnest($2::uuid[], $3::text[]) AS q(org_node_id, as_of_date)
),
target AS (
  SELECT
    i.org_node_id,
    i.as_of_date::date AS as_of_day,
    e.path
  FROM input i
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_date::date
   AND e.end_date >= i.as_of_date::date
),
path_parts AS (
  SELECT
    t.org_node_id,
    t.as_of_day,
    p.ord,
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

### 6.3 契约与失败路径（必须与 068 对齐）
- **输入契约**：`(tenant_id, org_node_id[], as_of_date[])`；as-of 仍以 `YYYY-MM-DD` 字符串传入，SQL 内部做 `::date` cast（保持现状）。
- **输出契约**：
  - 若存在匹配的 `org_edges`（target）与祖先节点信息，则返回 `long_name`（按 `name → code → id` 兜底规则拼接，分隔符 ` / `）。
  - 若 target 不存在（as-of 当天无 edge slice），则返回空字符串（对齐现状：缺失 key 填 `""`）。
- **关键失败路径：`label::uuid` cast**：
  - Gate 成立时，label 来源于 `replace(lower(uuid::text), '-', '')`，应始终可 `::uuid`。
  - 若出现存量/写入错误导致 label 非法，`label::uuid` 会使查询失败并向上冒泡。
  - **默认策略（严格模式）**：保留失败，让错误暴露，依赖 069 的巡检/修复把数据拉回不变量；本方案仅在 Gate 达成后切换为默认实现。
  - **仅兜底候选（容错模式）**：如确需避免 500，可引入过滤逻辑跳过非法 label，但输出可能缺段；该策略需要单独写清“启用条件/适用范围”，且不得成为 SSOT。

### 6.4 性能收益与风险（定性）
- **预期收益**：
  - 避免 `org_edges` 自连接扫描祖先集合；祖先列表由 `target.path` 直接给出。
  - 中间行数约为 `sum(depth(node)+1)`，更接近“按深度线性”的成本。
- **风险与注意事项**：
  - `label::uuid` cast 成本：建议用 `EXPLAIN (ANALYZE, BUFFERS)` 同时跑 068 与本方案对比评估；避免 `substr`/`regexp_replace`。
  - 本方案更“相信 path”，因此更强依赖 069 Gate；Gate 未满足时，错误会更像“事实链路”。

## 7. 安全与鉴权 (Security & Authz)
- 不引入新的 Authz 面；所有 SQL 必须包含 `tenant_id=$1`，并固定 `hierarchy_type='OrgUnit'`。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - DEV-PLAN-069 的 A/C：`org_edges.path/depth` 作为结构索引 SSOT 的不变量成立（含存量治理）。
  - Postgres `ltree` 已启用（现状已在 org schema 中声明）。
- **里程碑**（进入编码后）：
  1. [X] 在 `pkg/orglabels` 增加 path-driven 查询实现，并替换默认实现；旧查询作为基线仅保留在对照测试中。
  2. [X] 增加对照测试：同一组输入 pairs 同时跑新旧两套查询，断言输出字面一致（覆盖：正常链路、无 edge slice 为空串、无 name 时 `code/id` 兜底）。
  3. [ ] 做一次最小化性能对比：对同一批输入记录 `EXPLAIN (ANALYZE, BUFFERS)`，确认收益在典型数据集上成立。
  4. [X] 决策切换方式：已替换为默认查询（PR #159）；回滚路径为 revert 对应提交。

## 9. 测试与验收标准 (Acceptance Criteria)
- **正确性（对照一致）**：对相同输入（tenant、pairs），本方案生成的 `long_name` 与 068 现有实现字面一致。
- **回归案例**：覆盖 069 典型回归（“回溯生效日移动祖先 + 存在未来切片”）的数据集，修复后 long_name 不再缺段，且不出现“上级显示”与“长名称链路”解释冲突。
- **边界处理**：
  - as-of 当天找不到 edge slice：返回空字符串（不报错）。
  - slice name 为空/空白：回退 `code`，再回退 `id`。
- **门禁**：进入编码后必须通过 Go 触发器门禁（见 `AGENTS.md`）。

## 10. 运维与监控 (Ops & Monitoring)
- 不引入运行期开关（项目早期阶段避免过度运维）。
- **回滚方案**：
  - 代码回滚：切回 068 的原查询实现（保留旧 SQL 常量/分支以便回滚，不依赖 schema 回滚）。
  - 数据回滚：本计划不改数据，无数据回滚需求（数据治理属于 DEV-PLAN-069）。

## 附录：验证记录（UUID 转换能力与成本）
- 环境：本仓库 `compose.dev.yml` 的 `postgres:17`（示例版本：17.7），`jit=off`。
- 行为验证：`SELECT '0123456789abcdef0123456789abcdef'::uuid;` 可直接成功（证明 32 位 hex 可直接 cast）。
- 成本结论（微基准）：对 1,000,000 行 `text→uuid`，`label::uuid` 显著快于 “`substr` 插连字符再 cast”，也显著快于正则替换再 cast；因此应避免在热路径做字符串重写。
