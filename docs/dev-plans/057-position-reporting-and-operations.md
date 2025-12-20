# DEV-PLAN-057：Position 报表与运营（统计/空缺分析/质量守护）（对齐 051 阶段 E）

**状态**: 已完成（2025-12-21）

## 0. 进度速记
- 本计划对齐 050 §8.3/§8.4 的报表需求与 051 阶段 E（E0/E1/E2）：先冻结口径，再交付可查询/可导出的稳定 API；最后补齐质量守护与性能预算，确保“可重复对账、可回归”。
- 在线查询禁止递归 CTE：组织范围（含下级）必须复用 029 的 deep-read 后端（edges/closure/snapshot），并遵循 027 的 query budget 思路。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 业务需求：`docs/dev-plans/050-position-management-business-requirements.md`（§8.3 编制统计、§8.4 空缺分析、§9 权限边界）。
  - 实施蓝图：`docs/dev-plans/051-position-management-implementation-blueprint.md`（阶段 E：统计/空缺分析/运营守护）。
- **依赖链路（必须对齐）**：
  - `docs/dev-plans/052-position-contract-freeze-and-decisions.md`：生命周期/填充状态口径冻结、System/Managed 边界。
  - `docs/dev-plans/053-position-core-schema-service-api.md`：Position/Assignment 的 FTE/容量/时间线治理与稳定错误码（统计/空缺的 SSOT 输入）。
  - `docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`：Job Catalog 维度（按职级/分类拆分统计）与 Restrictions 对 vacancy 口径的影响（如纳入）。
  - `docs/dev-plans/029-org-closure-and-deep-read-optimization.md`：组织范围（含下级）查询底座与 feature flag（禁止递归）。
  - `docs/dev-plans/033-org-visualization-and-reporting.md`：`org_reporting` as-of 快照视图（BI/对账基线，可复用节点路径与属性）。
  - `docs/dev-plans/031-org-data-quality-and-fixes.md`：质量规则/报告/修复工具链（本计划扩展 staffing 规则时必须复用其契约）。
  - `docs/dev-plans/034-org-ops-monitoring-and-load.md`：metrics/health/load runner（本计划的查询预算、慢查询与回归基线需纳入）。
  - `docs/dev-plans/059-position-rollout-readiness-and-observability.md`：readiness/灰度/可观测收口（本计划必须提供可复现记录点）。
- **当前痛点**：
  - 缺少统一、可解释且可复现的统计口径：不同页面/导出/报表容易各算各的（尤其是“包含下级组织”“未来生效”“撤销/停用”边界）。
  - 组织范围统计天然重：若在线用递归/多次 SQL 循环，性能会随树深与节点数退化，难以作为长期运营能力。
  - 空缺分析若不冻结“vacancy 起点/终点/过滤条件”，会导致 time-to-fill 不可对账、不可用于运营决策。
- **业务价值**：
  - 为业务提供可对账的编制（capacity/occupied/available/fill rate）与空缺（vacancy aging/time-to-fill）能力，支持按组织范围与分类维度拆分。
  - 为后续看板化/BI 输出提供稳定数据契约与性能安全网（query budget/限流/质量巡检）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [x] **统计口径冻结（E0）**：容量/占用/可用/填充率以 FTE 为主，支持 scope=`self|subtree`，并明确撤销/停用/未来生效的计算口径（v1 不支持 `capacity_fte=0` 作为“冻结席位”语义；如需引入需先同步更新 052/053 合同与迁移约束）。
- [x] **统计查询/导出闭环（E1）**：提供稳定 JSON API（必要时补 CSV 导出），支持：
  - org scope（仅本组织/包含下级）
  - as-of 视角（默认 `nowUTC`）
  - 维度拆分（至少：按 `job_level`、按 `position_type`；若缺失则返回 `unknown` 组）
- [x] **空缺分析可用（E2）**：
  - vacancy aging：可按 org scope 输出“当前空缺列表/汇总”，并给出 vacancy_since/age（基础口径可解释、可回归）。
  - time-to-fill（基础版）：以“任职开始 = 填补”作为 SSOT 事件，输出指定时间窗内的 TTF 分布/均值（先聚焦 `capacity_fte=1` 的 Position，避免多席位歧义）。
- [x] **数据质量与性能守护**：扩展质量规则与最小性能预算：
  - 质量：超编、无效分类引用、限制冲突、异常 vacancy（例如 active 但长期 capacity=0）等。
  - 性能：禁止递归；为关键查询提供 query budget/分页上限/结果规模上限与稳定错误码。

### 2.2 非目标（Out of Scope）
- 不实现完整招聘流程与招聘事件（vacancy 仅以 Position/Assignment 的时间线推导）。
- 不在本计划内建设长期 BI ETL/调度与告警面板（可复用 033/034 的基线，但不作为硬交付）。
- 不在本计划内定义薪酬/预算口径（仅在 Position 上复用/透传字段，不进入统计口径核心）。

### 2.3 工具链与门禁（SSOT 引用）
> 本节只勾选触发器并引用 SSOT，避免复制命令矩阵导致 drift。

- **命中触发器（`[X]` 表示本计划预计涉及）**：
  - [X] Go 代码（reports service/repo、测试、（可选）CLI/导出）
  - [X] 路由治理（新增 `/org/api/reports/*`）
  - [X] Authz（新增/复用 reports 读权限；对齐 054）
  - [ ] DB 迁移 / Schema（若补索引/新增视图/表；需按 021A 走 Org Atlas+Goose）
  - [X] 文档（本计划更新）/（可选）Readiness 记录（对齐 059）
  - [ ] `.templ` / Tailwind、多语言 JSON（仅当新增看板 UI 时触发）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org 工具链：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
  - deep-read/报表/运维：`docs/dev-plans/029-org-closure-and-deep-read-optimization.md`、`docs/dev-plans/033-org-visualization-and-reporting.md`、`docs/dev-plans/034-org-ops-monitoring-and-load.md`

### 2.4 与其他子计划的边界（必须保持清晰）
- 052：冻结“哪些状态纳入统计/空缺”的口径；057 只消费冻结结果，不在报表层引入新口径。
- 053：SSOT 提供 Position/Assignment 的字段与写语义；057 只读统计，不改变写链路与错误码语义。
- 056：提供 Job Catalog/Profile/Restrictions 字段与启停治理；057 将其作为“维度/过滤条件”的只读输入。
- 029：提供 deep-read 后端与 build/回滚；057 在线查询必须复用，不允许自建递归查询。
- 031：质量报告/修复工具 SSOT；057 若新增 staffing 质量规则，必须复用 `org_quality_report.v1` 契约与默认 dry-run 安全网。
- 034：指标/health/压测 SSOT；057 只追加报表指标与必要的压测 profile，不另起一套 ops 暴露面。
- 059：readiness/灰度/可观测收口；057 的验证记录必须能被 059 汇总复跑。

### 2.5 并行实施说明（与 056/058）
- 本计划与 `docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`、`docs/dev-plans/058-assignment-management-enhancements.md` 采用并行实施；057 作为**只读报表层**，应对“上游能力尚未合并”的中间态保持兼容。
- **注意事项**：
  - **口径冻结优先**：统计/空缺/TTF 口径以 052/053/057 的冻结为 SSOT，尤其是占编 `occupied_fte` 仍仅统计 `assignment_type='primary'`；即便 058 放开 `matrix/dotted` 写入，057 默认也不得将其计入占编（如需变更必须回到 052/057 重新冻结并评审）。
  - **维度缺失降级**：当 056 的 Job Catalog/Profile 尚未完全落地或数据不齐时，维度拆分必须能降级为 `unknown`（或等价组），不得因维度表缺失/空值导致报表不可用。
  - **迁移/索引协同**：057 若需要补索引/视图，必须与 056 的 `migrations/org/**` 变更协同排序（避免时间戳冲突），并按 021A 工具链执行与回滚说明。
  - **能力开关与收口**：任何“include_system/性能后端选择/导出规模限制”等行为应可观测并可回退，最终由 059 的 readiness 记录统一验证与留痕。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.0 架构图（Mermaid）
```mermaid
flowchart TD
  Client[Client / BI / Export] --> API[Org API\n/org/api/reports/*]
  API --> SVC[Org Reports Service]

  SVC --> DeepRead[Deep-Read Repo\n(DEV-PLAN-029)]
  SVC --> PosRepo[Positions Repo\n(DEV-PLAN-053)]
  SVC --> AsgRepo[Assignments Repo\n(DEV-PLAN-053)]
  SVC --> JobRepo[Job Catalog Repo\n(DEV-PLAN-056)]

  DeepRead --> DB[(PostgreSQL 17)]
  PosRepo --> DB
  AsgRepo --> DB
  JobRepo --> DB
```

### 3.1 在线统计必须复用 deep-read 后端（选定）
- org scope=`subtree` 的“包含下级组织”必须通过 029 的 `OrgDeepReadBackendForTenant` 选择后端（优先 snapshot），禁止递归 CTE。
- API 响应必须包含 `source.deep_read_backend` 与（若为 snapshot）`source.snapshot_build_id`，便于对账与排障。

### 3.2 统计口径以“as-of 切片”作为 SSOT（选定）
- 所有统计均以 as-of 时间点 `t` 计算：
  - Position as-of：`effective_date <= t < end_date` 且生命周期状态属于“可统计集合”（以 052 映射为准）。
  - v1 默认“可统计集合”= `planned,active`；`inactive` 仅在显式请求（`lifecycle_statuses`）时纳入；`rescinded` 不进入报表口径。
  - Assignment as-of：`effective_date <= t < end_date`（并按 v1 的“计入占编”口径过滤；默认 `assignment_type='primary'`）。
- 统计输出默认聚焦 Managed Position（`org_positions.is_auto_created=false`）；System Position 仅在兼容期按配置（`include_system`）决定是否纳入（由 052/059 冻结策略决定）。

### 3.3 vacancy/time-to-fill 先做“可解释的基础版”（选定）
- vacancy aging：v1 以“当前 as-of 为空缺（occupied_fte=0 且 capacity_fte>0）”为入口，vacancy_since 采用可解释算法（见 §6.4）。
- time-to-fill：v1 仅覆盖 `capacity_fte=1` 的 Position，以“任职开始”作为填补事件；多席位 Position 的 TTF 不在 v1 承诺范围。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划尽量不新增持久化报表表；如性能达标需要补索引/视图，必须走 Org Atlas+Goose（021A）并给出回滚策略。

### 4.1 统计所需字段（依赖 053/056，作为 SSOT 输入）
以下字段名以“接口层契约”为准；若 053/056 采用不同命名，应以 053/056 的最终契约为 SSOT 并在此处同步更新：
- `org_positions`（稳定实体；053 SSOT）：
  - `tenant_id, id(position_id), code, is_auto_created`
- `org_position_slices`（as-of 切片；053 SSOT）：
  - `tenant_id, id(slice_id), position_id, org_node_id, lifecycle_status, position_type, capacity_fte, effective_date, end_date`
  - Job Catalog codes（来自 053/056 的契约输入）：`job_family_group_code/job_family_code/job_role_code/job_level_code`
- `org_assignments`（as-of 切片；053 SSOT）：
  - `tenant_id, id, position_id, assignment_type, effective_date, end_date`
  - `allocated_fte`（numeric/decimal；默认 1.0，允许 0.5 等；占编口径以 053/052 为准）

**Job Level 维度（报告侧派生）**：
- v1 报表维度 key 使用 `job_level_id`（uuid）以避免 code 复用导致歧义。
- `job_level_id` 的解析规则（对齐 056 的层级约束）：
  1. `org_job_family_groups`：`(tenant_id, code=job_family_group_code)`
  2. `org_job_families`：`(tenant_id, job_family_group_id, code=job_family_code)`
  3. `org_job_roles`：`(tenant_id, job_family_id, code=job_role_code)`
  4. `org_job_levels`：`(tenant_id, job_role_id, code=job_level_code)` → `job_level_id`
- 任一步骤无法解析（或字段缺失）则视为 `unknown`（用于 breakdown 分组与 items 展示）。

### 4.2 推荐索引（如性能基准不达标再落地）
- `org_position_slices`（对齐 053）：
  - `btree (tenant_id, position_id, effective_date)`
  - `btree (tenant_id, org_node_id, effective_date)`
  - （可选）对 scope 查询热点补充 covering index（需先用 034 的 load runner 证明收益）
- `org_assignments`：
  - `btree (tenant_id, position_id, effective_date)`
  - （可选）`gist (tenant_id, position_id, tstzrange(effective_date,end_date,'[)'))`

## 5. 接口契约 (API Contracts)
> 约定：内部 API 前缀 `/org/api`；JSON-only；Authz/403 payload 对齐 026；日期参数支持 `YYYY-MM-DD` 或 RFC3339（统一 UTC）。

### 5.1 `GET /org/api/reports/staffing:summary`（编制统计汇总）
**Query**
- `org_node_id`：可选（缺省租户 root）
- `effective_date`：可选（缺省 `nowUTC`）
- `scope`：可选，`self|subtree`（默认 `subtree`）
- `group_by`：可选，`none|job_level|position_type`（默认 `none`）
- `lifecycle_statuses`：可选（逗号分隔），默认 `planned,active`；允许 `planned|active|inactive`（`rescinded` 不允许进入报表口径）
- `include_system`：可选，默认 `false`（System Position 是否纳入；以 052/059 冻结策略为准）

**Response 200（v1）**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z",
  "scope": "subtree",
  "totals": {
    "positions_total": 120,
    "capacity_fte": 120.0,
    "occupied_fte": 87.5,
    "available_fte": 32.5,
    "fill_rate": 0.7292
  },
  "breakdown": [
    { "key": "unknown", "positions_total": 10, "capacity_fte": 10.0, "occupied_fte": 5.0, "available_fte": 5.0, "fill_rate": 0.5 }
  ],
  "source": { "deep_read_backend": "snapshot", "snapshot_build_id": "uuid" }
}
```

**Errors（稳定错误码）**
- 400 `ORG_INVALID_QUERY`
- 422 `ORG_NODE_NOT_FOUND_AT_DATE`（对齐 052 §6.3）
- 422 `ORG_REPORT_GROUP_BY_INVALID`
- 422 `ORG_REPORT_TOO_LARGE`（scope 子树规模超上限）
- 401/403：同 026

### 5.2 `GET /org/api/reports/staffing:vacancies`（空缺列表与 aging）
**Query**
- `org_node_id`：可选（缺省租户 root）
- `effective_date`：可选（缺省 `nowUTC`）
- `scope`：可选，`self|subtree`（默认 `subtree`）
- `lifecycle_statuses`：可选（逗号分隔），默认 `planned,active`
- `limit`：可选（默认 200，最大 2000）
- `cursor`：可选（基于 `position_id` 或 `position_code` 的游标）

**Response 200（v1）**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z",
  "scope": "subtree",
  "items": [
    {
      "position_id": "uuid",
      "position_code": "P-001",
      "org_node_id": "uuid",
      "capacity_fte": 1.0,
      "occupied_fte": 0.0,
      "vacancy_since": "2025-02-10T00:00:00Z",
      "vacancy_age_days": 20,
      "job_level_id": "uuid|null",
      "position_type": "unknown"
    }
  ],
  "next_cursor": null,
  "source": { "deep_read_backend": "snapshot", "snapshot_build_id": "uuid" }
}
```

**Errors**
- 同 §5.1，额外：
  - 422 `ORG_REPORT_LIMIT_TOO_LARGE`

### 5.3 `GET /org/api/reports/staffing:time-to-fill`（基础 TTF 报告）
> v1 只覆盖 `capacity_fte=1` 且能明确 vacancy→fill 的场景；作为“可解释基础口径”。

**Query**
- `org_node_id`：可选（缺省租户 root）
- `scope`：可选，`self|subtree`（默认 `subtree`）
- `from`：必填（YYYY-MM-DD，统计窗口起点）
- `to`：必填（YYYY-MM-DD，统计窗口终点）
- `group_by`：可选，`none|job_level|position_type`（默认 `none`）
- `lifecycle_statuses`：可选（逗号分隔），默认 `planned,active`

**Response 200（v1）**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "from": "2025-02-01",
  "to": "2025-03-01",
  "scope": "subtree",
  "summary": { "filled_count": 12, "avg_days": 18.2, "p50_days": 15, "p95_days": 45 },
  "breakdown": [{ "key": "unknown", "filled_count": 3, "avg_days": 10.0 }],
  "source": { "deep_read_backend": "snapshot", "snapshot_build_id": "uuid" }
}
```

### 5.4 `GET /org/api/reports/staffing:export`（导出，CSV/JSON）
> v1 目标是“可复现导出”，优先复用 §5.1/§5.2/§5.3 的计算口径；导出只改变序列化格式，不改变数据含义。

**Query**
- `kind`：必填，`summary|vacancies|time_to_fill`
- 其余 query：与对应的接口一致
- `format`：可选，`json|csv`（默认 `csv`）

**Response**
- `format=json`：同对应接口的 JSON payload
- `format=csv`：`text/csv; charset=utf-8`（首行 header；字段名以 v1 固化）

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 组织范围（subtree）节点集合（禁止递归）
输入：`tenant_id, hierarchy_type='OrgUnit', root_node_id, t`
1. 选择 deep-read backend（SSOT：029）。
2. backend=`snapshot`：
   - `as_of_date = date(t in UTC)`，获取 active build_id；
   - 取 descendant 列表：`SELECT descendant_node_id FROM org_hierarchy_snapshots WHERE tenant_id=? AND hierarchy_type=? AND as_of_date=? AND build_id=? AND ancestor_node_id=?`
3. backend=`edges`：使用 `org_edges.path` 与 as-of edge 切片获取 subtree（禁止递归）。
4. 返回 nodeset（用于 join positions）。

### 6.2 occupied_fte 计算（as-of）
对每个 Position slice：
- `occupied_fte = SUM(assignments.allocated_fte)` where assignment as-of 且 `assignment.position_id = position.position_id`，并满足 v1 计入占编口径（默认 `assignment_type='primary'`，对齐 052/053）。
- `available_fte = GREATEST(capacity_fte - occupied_fte, 0)`。
- `fill_rate = occupied_fte / capacity_fte`（v1 `capacity_fte > 0`；若遇到历史脏数据可返回 `null` 并纳入质量报告）。

### 6.3 汇总与拆分（group_by）
对 scope 内 positions 聚合：
- totals：sum capacity/occupied/available、count positions。
- group_by：
  - `job_level`：按派生的 `job_level_id` 分组（解析失败 → `unknown`）。
  - `position_type`：按 `position_type` 分组（空串/NULL → `unknown`）。

### 6.4 vacancy_since（基础口径，v1）
对“当前 vacant”（`capacity_fte>0` 且 `occupied_fte=0`）的 Position：
- `vacancy_since = COALESCE(last_primary_assignment_end, position_inception_date)`
- `last_primary_assignment_end` 定义为：同一 `position_id` 在 `t` 之前（`end_date <= t`）的最后一次“计入占编”的任职结束时间（取 `MAX(end_date)`；默认 `assignment_type='primary'`）。
- `position_inception_date` 定义为：该 `position_id` 的首个切片生效日（`MIN(org_position_slices.effective_date)`）。
  - 说明：使用 `position_id`（稳定实体）避免 Update 插入新切片导致 vacancy_since 被错误重置为当前切片 effective_date。

### 6.5 time-to-fill（基础版，v1）
统计窗口 `[from,to)` 内的“填补事件”：
- 仅统计 `capacity_fte=1` 的 Position。
- “填补事件”定义：某次“计入占编”的 Assignment 的 `effective_date` 落在窗口内，且该 Position 在此之前存在 vacancy（`last_primary_assignment_end` 存在且 `< effective_date`，或 Position 创建后首次任职）。
- `time_to_fill_days = effective_date - vacancy_since`（按天取整/向上取整需冻结，v1 选定：按 UTC date 差值）。

## 7. 安全与鉴权 (Security & Authz)
> 以 `docs/dev-plans/054-position-authz-policy-and-gates.md` 为 SSOT；本节只冻结 endpoint→object/action 映射。

| Endpoint | Object | Action |
| --- | --- | --- |
| `GET /org/api/reports/staffing:summary` | `org.position_reports` | `read` |
| `GET /org/api/reports/staffing:vacancies` | `org.position_reports` | `read` |
| `GET /org/api/reports/staffing:time-to-fill` | `org.position_reports` | `read` |
| `GET /org/api/reports/staffing:export` | `org.position_reports` | `read` |

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - 052：口径冻结与 System/Managed 策略。
  - 053：FTE/容量/assignment `allocated_fte` 字段与稳定错误码。
  - 029：deep-read 后端可用（至少 edges；推荐 snapshot）。
  - 034：metrics/health 基线可复用（用于观测与压测）。
- **里程碑（对齐 051 阶段 E）**：
  1. [x] E0：统计口径冻结（本文件评审通过；与 052/053/056 对齐）。
  2. [x] E1：`staffing:summary` API + 最小性能预算（含 query budget 测试）。
  3. [x] E2：`staffing:vacancies` + 基础 vacancy aging；`staffing:time-to-fill`（基础版）。
  4. [x] 质量守护：扩展 `org-data quality` 规则（复用 031 契约）并提供可复现报告。
  5. [x] Readiness：在 059 指定的记录中登记门禁/冒烟/性能摘要（命令/结果/时间戳）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **正确性**：
  - totals/breakdown 与手工 SQL 对账一致（含 subtree 口径）。
  - vacancy_since/time_to_fill 的算法在固定数据集下可复现。
- **性能与守卫**：
  - 在线查询无递归 CTE；关键路径具备 query budget（参考 027 思路，防 N+1/线性退化）。
  - `ORG_REPORT_TOO_LARGE` 等保护可触发且错误码稳定。
- **工程门禁**：
  - 触发的门禁按 `AGENTS.md` 执行；文档变更需通过 `make check doc`。

## 10. 运维与可观测性 (Ops & Monitoring)
- **保护阈值（v1，建议可配置）**：
  - `max_scope_nodes = 10000`（超过返回 `ORG_REPORT_TOO_LARGE`）
  - `max_time_range_days = 366`（超过返回 `ORG_INVALID_QUERY` 或专用错误码）
- **指标**（建议纳入 034 的 Prometheus 体系）：
  - `org_reports_staffing_requests_total{endpoint,result}`
  - `org_reports_staffing_latency_seconds{endpoint,result}`
  - `org_reports_staffing_too_large_total{endpoint}`
- **日志/审计**：
  - 报表拒绝（too large/invalid query）需输出结构化日志包含 `tenant_id, org_node_id, scope, effective_date, backend, error_code`，便于排障与回放。
