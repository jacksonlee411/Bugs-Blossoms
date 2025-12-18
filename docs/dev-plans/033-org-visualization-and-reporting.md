# DEV-PLAN-033：Org 可视化与高级报告（Step 13）

**状态**: 已评审（2025-12-18 12:00 UTC）— 按 `docs/dev-plans/001-technical-design-template.md` 补齐可编码契约

## 0. 进度速记
- 本计划交付三类能力：**组织图导出（JSON）**、**节点/人员路径查询**、以及面向 BI 的 **`org_reporting`（as-of 快照）**。
- 热点查询禁止递归 CTE：在线查询必须复用 029 的 deep-read 后端（edges/closure/snapshot），递归仅允许出现在离线 build 刷新任务里。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` **步骤 13：可视化与高级报告**。
- **依赖链路**：
  - `docs/dev-plans/029-org-closure-and-deep-read-optimization.md`：闭包/快照表与 build pointer（deep read 的性能底座）。
  - `docs/dev-plans/032-org-permission-mapping-and-associations.md`：security group mapping 与 `org_links`（报告维度之一）。
  - `docs/dev-plans/024-org-crud-mainline.md` / `docs/dev-plans/025-org-time-and-audit.md`：节点/岗位/分配的写语义与稳定错误码（路径查询依赖其 as-of 语义）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403 payload、snapshot/batch 基线（本计划新增 API 必须遵循）。
- **当前痛点**：
  - 缺少“可直接用于可视化/导出”的稳定接口，外部只能拼装 `GET /org/api/hierarchies` 或 `GET /org/api/snapshot`，难以稳定对接。
  - “节点路径/人员路径”属于高频排障与报表需求，若用递归查询或 N+1 join，性能容易随树深/节点数退化。
  - BI 场景需要“按 as-of 日期可重复对账”的快照视图，避免线上表结构/事件延迟导致报表漂移。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] **组织图导出（JSON）**：提供 `GET /org/api/hierarchies:export`（见 §5.1），支持：
    - as-of（`effective_date`）
    - subtree（`root_node_id` + `max_depth`）
    - 可选 include（security groups / links）
  - [ ] **节点路径查询**：提供 `GET /org/api/nodes/{id}:path`（见 §5.2），返回 root→node 的路径数组（含来源与 depth），并具备稳定错误码。
  - [ ] **人员路径查询**：提供 `GET /org/api/reports/person-path`（见 §5.3），基于 primary assignment as-of 定位 org node 并返回路径。
  - [ ] **BI 视图/快照（SSOT）**：定义 `org_reporting`（见 §4），按 `as_of_date` 索引，面向 BI/对账消费：
    - 每节点一行（路径拍平、属性、security groups、links 的可选展开）
    - 只读取 active build（与 029 snapshot builds 对齐）
  - [ ] **性能与可复现**：为路径查询与导出提供 query budget/分页限制，避免 O(N) 级 SQL roundtrip。
  - [ ] Readiness：新增 `docs/dev-records/DEV-PLAN-033-READINESS.md`（实现阶段落盘），记录门禁命令与输出摘要。
- **非目标 (Out of Scope)**：
  - 不实现 PNG/SVG 服务端渲染（MVP 仅提供 JSON；SVG/PNG 由前端/外部工具消费 JSON 后生成）。
  - 不建设长期 BI ETL/调度/告警（归属 034 运维治理）；本计划仅定义快照/视图与可重复构建入口。
  - 不在本计划内引入“按 org scope 自动生成 Casbin 策略/草稿”（策略通道以 015A 为 SSOT）。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划会新增 Go 代码、路由/API、可能新增 Org 迁移/视图；命令与门禁以 SSOT 为准，本文不复制矩阵。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（API handler、report builder、测试/bench）
  - [X] 路由治理（新增 `/org/api/hierarchies:export`、`/org/api/nodes/{id}:path`、`/org/api/reports/person-path` 等）
  - [X] Authz（新增 endpoint → object/action 映射与策略片段）
  - [X] 迁移 / Schema（如落地 `org_reporting` 快照表/视图；必须走 Org Atlas+Goose）
  - [X] 文档 / Readiness（新增 033 readiness record）
  - [ ] `.templ` / Tailwind、多语言 JSON（本计划不涉及 UI 资产；如后续加 UI 则需补门禁）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

### 2.2 与其他子计划的边界（必须保持清晰）
- 029：deep-read 表/build/pointer SSOT；033 在线查询禁止递归 CTE，必须复用 029 的 backend 选择。
- 032：security group mapping 与 org links SSOT；033 的 include/reporting 只读消费，不改变其写语义。
- 026：Authz/403 payload 与 snapshot/batch SSOT；033 的新端点必须复用 `ensureAuthz` 与 forbidden payload。
- 035：Org UI（M1）负责交互体验；033 只提供导出/报告/路径 API，UI 接入可另起子计划或在 035 承接。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Client[UI/CLI/BI] --> API[/org/api/**/]
  API --> Authz[ensureAuthz (026)]
  API --> ReportSvc[Org Reporting Service]
  ReportSvc --> ReadBackend{deep read backend}
  ReadBackend -->|edges| Edges[(org_edges.path)]
  ReadBackend -->|closure/snapshot| Snap[(org_hierarchy_* 029)]
  ReportSvc --> Map[(org_security_group_mappings 032)]
  ReportSvc --> Links[(org_links 032)]
  ReportSvc --> BI[(org_reporting 033)]
```

### 3.2 关键设计决策（ADR 摘要）
1. **MVP 仅 JSON 导出（选定）**
   - 组织图导出以 JSON 为唯一输出；SVG/PNG 属“表示层渲染”问题，避免引入 Graphviz/Headless 浏览器等重依赖。
2. **路径查询基于 deep-read 后端（选定）**
   - 在线路径查询优先使用 029 的 snapshot/closure（如启用），否则 fallback 到 021 的 `org_edges.path`（ltree）；禁止在线递归。
3. **BI 视图只读 active build（选定）**
   - `org_reporting` 只暴露 active snapshot build 的结果，保证报表可重复对账并可通过 build pointer 回滚。
4. **契约优先（选定）**
   - API 返回稳定字段（id/code/name/depth/path），错误码复用既有口径；对大结果集强制 limit/cursor，避免一次性全量 dump。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本节定义 BI 面向的 `org_reporting` SSOT。deep-read 表与 build pointer 以 029 为 SSOT；security group / links 以 032 为 SSOT。

### 4.1 `org_reporting_nodes`（快照表，建议落地）
**用途**：每节点一行的“路径拍平 + 属性”快照，供 BI/对账使用；按 `as_of_date` 可索引查询。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `hierarchy_type` | `text` | `not null` + check | `'OrgUnit'` | 层级类型 |
| `as_of_date` | `date` | `not null` |  | 快照日期（UTC） |
| `build_id` | `uuid` | `not null` |  | FK → 029 `org_hierarchy_snapshot_builds` |
| `org_node_id` | `uuid` | `not null` |  | 节点 |
| `code` | `varchar(64)` | `not null` |  | `org_nodes.code` |
| `name` | `text` | `not null` |  | as-of `org_node_slices.name` |
| `status` | `text` | `not null` |  | as-of `org_node_slices.status` |
| `parent_node_id` | `uuid` | `null` |  | root 为 null |
| `depth` | `int` | `not null` + check |  | root=0 |
| `path_node_ids` | `uuid[]` | `not null` |  | root→self |
| `path_codes` | `text[]` | `not null` |  | root→self |
| `path_names` | `text[]` | `not null` |  | root→self |
| `attributes` | `jsonb` | `not null` | `'{}'` | 节点显式属性（v1 keys：`legal_entity_id/company_code/location_id/manager_user_id`） |
| `security_group_keys` | `text[]` | `not null` | `'{}'` | 解析后的安全组 key（可选维度；来自 032） |
| `links` | `jsonb` | `not null` | `'[]'` | 业务对象关联（v1 item：`{object_type,object_key,link_type}`；可选维度；来自 032） |
| `created_at` | `timestamptz` | `not null` | `now()` |  |

**约束/索引（建议）**
- `primary key (tenant_id, hierarchy_type, as_of_date, build_id, org_node_id)`
- FK：
  - `fk (tenant_id, hierarchy_type, as_of_date, build_id) -> org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, build_id) on delete cascade`
  - `fk (tenant_id, org_node_id) -> org_nodes (tenant_id, id) on delete restrict`
- `check (depth >= 0)`
- `check (jsonb_typeof(attributes) = 'object')`
- `check (jsonb_typeof(links) = 'array')`
- 索引：
  - `btree (tenant_id, hierarchy_type, as_of_date, org_node_id)`
  - `btree (tenant_id, hierarchy_type, as_of_date, code)`

### 4.2 `org_reporting`（BI 视图，SSOT）
**用途**：只暴露 active build 的节点快照，BI 无需理解 build pointer 细节。

**视图定义（示意）**
```sql
CREATE VIEW org_reporting AS
SELECT r.*
FROM org_reporting_nodes r
JOIN org_hierarchy_snapshot_builds b
  ON b.tenant_id = r.tenant_id
 AND b.hierarchy_type = r.hierarchy_type
 AND b.as_of_date = r.as_of_date
 AND b.build_id = r.build_id
WHERE b.is_active = TRUE AND b.status = 'ready';
```

### 4.3 迁移策略
- schema 源 SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`
- 新增迁移（示例）：`migrations/org/0000x_org_reporting_nodes.sql`
- 工具链与门禁：按 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md` 执行（本文不复制命令清单）。

## 5. 接口契约 (API Contracts)
> 约定：内部 API 前缀 `/org/api`；JSON-only；Authz/403 payload 对齐 026；时间参数支持 `YYYY-MM-DD` 或 RFC3339（统一 UTC）。

### 5.1 `GET /org/api/hierarchies:export`（组织图导出，JSON）
**Query**
- `type`：必填，`OrgUnit`
- `effective_date`：可选（缺省 `nowUTC`）
- `root_node_id`：可选（缺省为租户 root）
- `max_depth`：可选（缺省不限；最大建议 20，超过返回 422 `ORG_EXPORT_TOO_DEEP`）
- `include`：可选，逗号分隔；允许值：
  - `nodes`（默认）
  - `edges`（可选；返回 `parent_node_id/child_node_id` 边集合）
  - `security_groups`（需要 032；返回每节点 resolved keys）
  - `links`（需要 032；返回每节点 links 摘要）
- `limit`：可选，默认 `2000`，最大 `10000`
- `cursor`：可选

**Response 200（v1）**
```json
{
  "tenant_id": "uuid",
  "hierarchy_type": "OrgUnit",
  "effective_date": "2025-03-01T00:00:00Z",
  "root_node_id": "uuid",
  "includes": ["nodes", "edges"],
  "limit": 2000,
  "nodes": [
    { "id": "uuid", "parent_id": "uuid|null", "code": "D001", "name": "Engineering", "depth": 3, "status": "active" }
  ],
  "edges": [
    { "child_node_id": "uuid", "parent_node_id": "uuid|null" }
  ],
  "next_cursor": null
}
```

**Errors**
- 400 `ORG_INVALID_QUERY`
- 422 `ORG_EXPORT_TOO_DEEP`
- 422 `ORG_EXPORT_TOO_LARGE`（root 子树规模超过上限时）
- 401/400/403：同 026

### 5.2 `GET /org/api/nodes/{id}:path`（节点路径）
**Query**
- `effective_date`：可选（缺省 `nowUTC`）
- `format`：可选，默认 `nodes`；允许值：
  - `nodes`：仅返回 `path.nodes[]`
  - `nodes_with_sources`：返回 `path.nodes[]` + `source`（用于解释来自哪个 ancestor）

**Response 200**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z",
  "path": {
    "nodes": [
      { "id": "uuid", "code": "ROOT", "name": "Company", "depth": 0 },
      { "id": "uuid", "code": "D001", "name": "Engineering", "depth": 1 }
    ]
  }
}
```

**Errors**
- 404 `ORG_NODE_NOT_FOUND_AT_DATE`
- 401/400/403：同 026

### 5.3 `GET /org/api/reports/person-path`（人员路径）
**Query**
- `subject`：必填，格式 `person:{pernr}`（对齐 024 `GET /org/api/assignments`）
- `effective_date`：可选（缺省 `nowUTC`）

**Response 200**
```json
{
  "tenant_id": "uuid",
  "subject": "person:000123",
  "effective_date": "2025-03-01T00:00:00Z",
  "assignment": { "assignment_id": "uuid", "position_id": "uuid", "org_node_id": "uuid" },
  "path": {
    "nodes": [
      { "id": "uuid", "code": "ROOT", "name": "Company", "depth": 0 },
      { "id": "uuid", "code": "D001", "name": "Engineering", "depth": 1 }
    ]
  }
}
```

**Errors**
- 404 `ORG_ASSIGNMENT_NOT_FOUND_AT_DATE`
- 422 `ORG_SUBJECT_INVALID`（subject 格式非法）
- 401/400/403：同 026

### 5.4 Authz 映射（MVP 固化）
> v1 复用既有 object/action，避免引入新的策略碎片；如后续需要细分，再单独评审并更新 026 映射与策略片段。

| Endpoint | Object | Action |
| --- | --- | --- |
| `GET /org/api/hierarchies:export` | `org.hierarchies` | `read` |
| `GET /org/api/nodes/{id}:path` | `org.hierarchies` | `read` |
| `GET /org/api/reports/person-path` | `org.assignments` | `read` |

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 节点路径查询（在线，禁止递归）
输入：`tenant_id, org_node_id, t`

1. 选择 deep-read 后端（SSOT：029 `ORG_DEEP_READ_ENABLED/ORG_DEEP_READ_BACKEND`）。
2. 若 backend=`snapshot`：
   - `as_of_date = date(t in UTC)`，查 active snapshot build；
   - 查询祖先链：
     - `SELECT ancestor_node_id, depth FROM org_hierarchy_snapshots WHERE tenant_id=? AND as_of_date=? AND build_id=? AND descendant_node_id=? ORDER BY depth DESC`
   - join `org_nodes` 与 as-of `org_node_slices` 获取 `code/name`。
3. 若 backend=`closure`：按 029 的时态闭包表在 `t` 的 as-of 视图查询祖先链（同样 order by depth desc）。
4. 若 backend=`edges`：使用 `org_edges.path`：
   - 取节点 as-of 的 `path`；
   - 将 path label 解析为 node_id 列表（ltree label → uuid），再按顺序 join 获取 `code/name`。
5. 返回 `path.nodes[]`；若任一环节无法定位节点 → 404 `ORG_NODE_NOT_FOUND_AT_DATE`。

### 6.2 人员路径查询（在线）
输入：`tenant_id, subject=person:{pernr}, t`

1. 查询 primary assignment as-of：
   - `org_assignments where tenant_id=? and pernr=? and assignment_type='primary' and effective_date<=t and t<end_date`
   - 若无 → 404 `ORG_ASSIGNMENT_NOT_FOUND_AT_DATE`
2. 查询 position as-of → 得到 `org_node_id`：
   - `org_positions where tenant_id=? and id=$position_id and effective_date<=t and t<end_date`
3. 复用 6.1 的节点路径查询。

### 6.3 `org_reporting_nodes` build（离线，允许递归）
输入：`tenant_id, hierarchy_type, as_of_date, build_id`

1. 前置：要求 029 的 snapshot build 已 `ready` 且可被激活（或已激活）。
2. 对每个 `org_node_id` 生成一行：
   - path_ids：从 snapshot 表取 `ancestor_node_id`（order by `depth desc`）聚合为数组；
   - `parent_node_id`：取 path_ids 倒数第二个（root 为 null）；
   - `depth`：`array_length(path_ids)-1`；
   - attributes：从 as-of `org_node_slices` 取显式属性拼为 json object；
   - security_group_keys：复用 032 的 preview 算法在批量模式下计算（如未启用可留空数组）；
   - links：取 `org_links` as-of 的 link 摘要（如未启用可留空数组）。
3. 写入 `org_reporting_nodes`（绑定 build_id）；失败应保持幂等可重跑（可先删后写或写入新 build_id）。

## 7. 安全与鉴权 (Security & Authz)
- **Authz**：所有新端点必须复用 026 的 `ensureAuthz` 与 forbidden payload（禁止自定义 403 形状）。
- **租户隔离**：所有查询必须强制 `tenant_id` 过滤；禁止出现“按 id 全局读”的 repo 方法。
- **PII 最小化**：
  - person-path 返回 `subject=person:{pernr}` 与 assignment/position/node id，不返回姓名/email。
  - BI `org_reporting` 默认不输出用户信息；如未来需要“人员列表”，必须分页且提升权限门槛并另行评审。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/029-org-closure-and-deep-read-optimization.md`：snapshot/closure/build pointer 可用（至少 snapshot ready/active）。
  - `docs/dev-plans/032-org-permission-mapping-and-associations.md`：security groups / links（如启用 include 与 reporting 维度）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403 payload/路由治理口径。
- **里程碑**：
  1. [ ] 路由与控制器：`hierarchies:export`、`nodes:{id}:path`、`reports/person-path`。
  2. [ ] 路径查询实现：按 029 后端切换，无递归、无 N+1。
  3. [ ] BI：`org_reporting_nodes` + `org_reporting` view（迁移 + build 入口）。
  4. [ ] Readiness：新增 `docs/dev-records/DEV-PLAN-033-READINESS.md`。

## 9. 测试与验收标准 (Acceptance Criteria)
- **功能**：
  - 导出在给定 `root_node_id` 下返回正确 subtree；`max_depth` 生效且越界返回稳定错误码。
  - 节点路径与人员路径在 as-of 下返回一致的 root→leaf 顺序。
- **性能守卫**：
  - 路径查询 SQL roundtrip 次数为常数级（建议 ≤ 5），且不随节点数增长。
  - 导出必须分页；单次响应不得超过 `limit`，并提供 `next_cursor`。
- **工程门禁**：
  - 文档：`make check doc` 通过。
  - 如落地 Go/迁移/Authz：按 `AGENTS.md` 触发器矩阵执行并通过。

## 10. 运维与回滚 (Ops & Rollback)
- **Feature Flags（契约，复用 029）**：
  - deep read 后端以 029 的 `ORG_DEEP_READ_*` 为准；033 不新增“另一套后端选择开关”。
- **回滚**：
  - API 行为回滚：关闭相关 feature flag 或切回 `ORG_DEEP_READ_BACKEND=edges`（性能降级但正确性优先）。
  - 报表回滚：通过 029 snapshot build pointer 回滚到上一 build（`org_reporting` 将自动指向 active build）。

## 11. 交付物 (Deliverables)
- API：组织图导出（JSON）、节点路径、人员路径。
- BI：`org_reporting_nodes` + `org_reporting`（active build 视图）。
- 测试与 Readiness：`docs/dev-records/DEV-PLAN-033-READINESS.md`（实现阶段落盘）。
