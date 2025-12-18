# DEV-PLAN-028：Org 属性继承解析与角色读侧占位（Step 8）

**状态**: 规划中（2025-12-18 01:21 UTC）

## 0. 进度速记
- 本计划对应 `docs/dev-plans/020-organization-lifecycle.md` 阶段 2 / 步骤 8：在 026（API/Authz/Outbox）与 027（性能/灰度）稳定后，补齐“读侧增强能力”。
- 占位表 SSOT：继承规则/角色/角色分配的表结构与字段语义以 `docs/dev-plans/022-org-placeholders-and-event-contracts.md` 为准；028 只落地读侧解析/查询与缓存策略。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` → 阶段 2 / 步骤 8「实现属性继承与角色查询」。
- **当前痛点**：
  - `org_node_slices` 已包含 `legal_entity_id/company_code/location_id/manager_user_id` 等显式属性，但缺少统一的“继承解析值（resolved）”口径；下游若各自计算会产生漂移。
  - `org_attribute_inheritance_rules` / `org_roles` / `org_role_assignments` 在 022 已定义（占位表），但缺少可复用的查询/API，无法支撑后续安全域/权限预览/报表等读取需求。
  - `org_assignments.assignment_type` 已预留 `matrix/dotted`，需要明确“只读边界”与测试守卫，避免写链路提前开放导致数据语义失控。
- **业务价值**：
  - 固化继承解析算法、缓存与失效策略，形成仓库内 SSOT，可复用到 UI/报表/后续策略生成。
  - 提供角色读侧占位入口，为 M2+ 的角色管理/安全域继承/策略草稿平台铺路。
  - 明确矩阵/虚线的只读承诺，降低灰度后引入不受控写入的风险。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 落地 `OrgInheritanceResolver`：在给定 `tenant_id + hierarchy_type + effective_date` 下，为 OrgNode 计算 `resolved_attributes`（与显式 `attributes` 同时可得）。
  - [ ] 为 `GET /org/api/hierarchies` 增加可选 `include=resolved_attributes`（向后兼容；未指定时响应保持 024 既有形态）。
  - [ ] 提供角色读侧占位 API：`GET /org/api/roles` 与 `GET /org/api/role-assignments`（只读），并支持 `include_inherited=true`（按祖先继承，返回 `source_org_node_id`）。
  - [ ] 矩阵/虚线只读：读接口应可返回 `assignment_type in (primary,matrix,dotted)`；写接口继续拒绝 `assignment_type != primary`（对齐 024 的 422 `ORG_ASSIGNMENT_TYPE_DISABLED`）。
  - [ ] Readiness：新增/更新 `docs/dev-records/DEV-PLAN-028-READINESS.md`，记录门禁命令与结果。
- **非目标（本计划明确不做）**：
  - 不提供继承规则/角色/角色分配的写 API（仅通过迁移/seed 或后续 dev-plan 承接）。
  - 不实现安全域/策略生成与继承（020 的 M3+）。
  - 不引入闭包表/物化视图等深层读优化（归属 029）。
  - 不改变 024/025 的写语义（Correct/Insert/Rescind/冻结窗口等）。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划涉及新增/修改 Go 代码与路由/API；如需补充迁移或 Authz 策略片段亦会触发对应门禁。命令细节以 SSOT 为准，本文不复制矩阵。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（resolver/service、API handler、测试）
  - [X] 路由治理（新增 `/org/api/roles` / `/org/api/role-assignments` 等）
  - [X] Authz（新增 endpoint → object/action 映射与策略片段）
  - [X] 文档 / Readiness（新增 028 readiness record）
  - [ ] 迁移 / Schema（仅当 022 的占位表需补索引/约束时才触发；若触发需走 Org Atlas+Goose）
  - [ ] `.templ` / Tailwind（不涉及）
  - [ ] 多语言 JSON（不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

### 2.2 与其他子计划的边界（必须保持清晰）
- 022：占位表与字段契约 SSOT（继承规则/角色/角色分配）。028 只消费其结构；若占位表尚未落地，应先完成 022 对应的迁移/门禁。
- 024：hierarchies/assignments 的基础读写契约。028 仅对 `GET /org/api/hierarchies` 做向后兼容的可选扩展，并确保 assignments 读侧能返回 matrix/dotted。
- 026：Authz/403 payload/outbox/caching 口径。028 的新接口必须遵循 026 的 `ensureAuthz` 与 forbidden payload。
- 027：性能/灰度与 query budget 思路。028 的继承解析必须避免 N+1，并给出可测试的 query budget。
- 029：闭包表/深层读优化。028 不引入额外读模型，继承解析只基于现有 `org_edges` 路径与 as-of slice。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Client[UI/Integrations] --> API[/org/api/**/]
  API --> Authz[ensureAuthz (026)]
  API --> Resolver[OrgInheritanceResolver]
  API --> RoleRead[Role Read APIs]
  Resolver --> Cache[(tenant cache)]
  Resolver --> Repo[Org Read Repos]
  RoleRead --> Repo
  Repo --> DB[(Postgres org_* tables)]
```

### 3.2 关键设计决策（ADR 摘要）
1. **继承解析采用“树顶向下”单次遍历（选定）**
   - 一次性取 as-of 的 `org_edges` 与 `org_node_slices`，在内存中构建树并自顶向下计算 `resolved_attributes`，避免 per-node SQL（N+1）。
2. **`can_override` 语义（选定）**
   - `can_override=true`：子节点显式值优先（nearest wins）。
   - `can_override=false`：祖先值优先（root-first wins）；子节点即使存在显式值也不应覆盖祖先（并在后续写 API 中逐步加入校验，避免产生“无效显式值”）。
3. **缓存以“租户粒度失效”作为 M2 最小可用策略（选定）**
   - 继承解析结果受节点切片/边关系/规则表影响，变更会影响子树；M2 先采用 tenant 粗粒度失效，避免复杂局部失效带来正确性风险。
4. **角色读侧支持 `include_inherited=true`（选定）**
   - `org_role_assignments` 绑定在某个 `org_node_id` 上，读侧按祖先链汇总（union）并返回 `source_org_node_id`，为“角色作用域=子树”提供可解释输出。
5. **矩阵/虚线只读守卫（选定）**
   - 读侧允许返回 `assignment_type=matrix/dotted`；写侧继续由 024 的校验拒绝（422 `ORG_ASSIGNMENT_TYPE_DISABLED`），并通过测试防回退。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划不新增表；表结构与约束 SSOT 见 `docs/dev-plans/021-org-schema-and-constraints.md` 与 `docs/dev-plans/022-org-placeholders-and-event-contracts.md`。

- **继承输入**：
  - `org_attribute_inheritance_rules`（022）：按 `effective_date <= t < end_date` 取 as-of 规则；仅处理 `hierarchy_type=OrgUnit`。
  - `org_node_slices`（Org baseline）：显式属性来源：
    - `legal_entity_id uuid|null`
    - `company_code text|null`
    - `location_id uuid|null`
    - `manager_user_id bigint|null`
- **树结构**：
  - `org_edges`（Org baseline）：按 as-of 取节点父子关系（`path/depth` 可用于祖先链查询）。
- **角色读侧**：
  - `org_roles`（022）：角色字典（只读）。
  - `org_role_assignments`（022）：按 as-of 取分配记录；`subject_type` 的 `group` 仍为占位（本期可不实现）。

- **属性白名单（v1，选定）**：
  - 028 的解析器仅支持：`legal_entity_id/company_code/location_id/manager_user_id`。
  - 若规则表出现其它 `attribute_name`：
    - **hierarchies include=resolved_attributes**：忽略未知属性（不阻断树读）。
    - **调试端点（见 5.2）**：当显式请求未知属性时返回 400 `ORG_UNKNOWN_ATTRIBUTE`。

## 5. 接口契约 (API Contracts)
> 约定：路由前缀 `/org/api`；Session/tenant/authz/403 payload 对齐 026。

### 5.1 `GET /org/api/hierarchies`（扩展：可选返回继承解析）
**Query（继承 024，并新增）**
- `type`：必填，`OrgUnit`
- `effective_date`：可选（缺省 `nowUTC`）
- `include`：可选，逗号分隔；允许值：
  - `resolved_attributes`：在每个 node 上返回 `attributes` + `resolved_attributes`

**Response 200（当 include 含 resolved_attributes）**
```json
{
  "tenant_id": "uuid",
  "hierarchy_type": "OrgUnit",
  "effective_date": "2025-01-01T00:00:00Z",
  "nodes": [
    {
      "id": "uuid",
      "code": "D001",
      "name": "Engineering",
      "parent_id": "uuid|null",
      "depth": 1,
      "display_order": 0,
      "status": "active",
      "attributes": {
        "legal_entity_id": null,
        "company_code": null,
        "location_id": "uuid",
        "manager_user_id": null
      },
      "resolved_attributes": {
        "legal_entity_id": "uuid",
        "company_code": "ACME",
        "location_id": "uuid",
        "manager_user_id": 123
      }
    }
  ]
}
```

**Errors**
- 400 `ORG_INVALID_QUERY`：`type/effective_date/include` 非法
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden（对齐 026 的 forbidden payload）

### 5.2 `GET /org/api/nodes/{id}:resolved-attributes`（调试/验收用）
> 用于灰度/Readiness 中快速定位“某个解析值来自哪个祖先节点”，避免只能通过全树 dump 排查。

**Query**
- `effective_date`：可选（缺省 `nowUTC`）
- `attributes`：可选，逗号分隔；缺省表示使用 as-of 规则表中出现的属性白名单子集。

**Response 200**
```json
{
  "tenant_id": "uuid",
  "hierarchy_type": "OrgUnit",
  "org_node_id": "uuid",
  "effective_date": "2025-01-01T00:00:00Z",
  "attributes": { "company_code": null, "location_id": "uuid" },
  "resolved_attributes": { "company_code": "ACME", "location_id": "uuid" },
  "resolved_sources": { "company_code": "uuid", "location_id": "uuid" }
}
```

**Errors**
- 404 `ORG_NODE_NOT_FOUND_AT_DATE`
- 400 `ORG_UNKNOWN_ATTRIBUTE`
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden

### 5.3 `GET /org/api/roles`（只读）
**Response 200**
```json
{
  "tenant_id": "uuid",
  "roles": [
    { "id": "uuid", "code": "HRBP", "name": "HR Business Partner", "description": "…", "is_system": true }
  ]
}
```

### 5.4 `GET /org/api/role-assignments`（只读，支持祖先继承汇总）
**Query**
- `org_node_id`：必填
- `effective_date`：可选（缺省 `nowUTC`）
- `include_inherited`：可选，默认 `false`
- `role`：可选（role code）
- `subject`：可选，格式 `user:<uuid>`（`group:<uuid>` 预留但本期不保证返回）

**Response 200**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "effective_date": "2025-01-01T00:00:00Z",
  "include_inherited": true,
  "items": [
    {
      "assignment_id": "uuid",
      "role_id": "uuid",
      "role_code": "HRBP",
      "subject": "user:uuid",
      "source_org_node_id": "uuid",
      "effective_window": { "effective_date": "2025-01-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
    }
  ]
}
```

**Errors**
- 400 `ORG_INVALID_QUERY`
- 404 `ORG_NODE_NOT_FOUND_AT_DATE`
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden

### 5.5 矩阵/虚线（assignment_type）只读承诺
- 读：`GET /org/api/assignments`（024）返回的 `assignment_type` 必须允许出现 `primary/matrix/dotted`（如数据存在，不应过滤）。
- 写：`POST/PATCH /org/api/assignments`（024）继续只允许 `primary`；其它类型返回 422 `ORG_ASSIGNMENT_TYPE_DISABLED`。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 继承解析（resolved_attributes）
对单个 `effective_date` 的一次树解析，目标是 **O(N)** 计算并且 SQL 查询数为常数级。

1. 取 as-of 规则：`org_attribute_inheritance_rules`（tenant+hierarchy_type，`effective_date <= t < end_date`）。
2. 取 as-of 树：`org_edges`（`hierarchy_type`，`effective_date <= t < end_date`），构建 parent→children。
3. 取 as-of 切片：`org_node_slices`（对节点集合，`effective_date <= t < end_date`），得到显式属性值。
4. 自顶向下遍历（按 root→leaf）：
   - 对每个 `attribute_name`：
     - `can_override=true`：`resolved = explicit != null ? explicit : parentResolved`
     - `can_override=false`：`resolved = parentResolved != null ? parentResolved : explicit`
   - `resolved_sources`：
     - 若 `resolved` 来自 parent：继承 parent 的 source
     - 若 `resolved` 来自 self explicit：source=self
5. 输出 `resolved_attributes` 与 `resolved_sources`。

### 6.2 角色继承汇总（include_inherited）
当 `include_inherited=true`：
1. 获取 `org_node_id` 的 as-of `path`（`org_edges` 中该节点的 row）。
2. 基于 ltree 前缀查询祖先集合（包含 self 与 root）。
3. 查询 as-of `org_role_assignments` where `org_node_id in ancestors`（可按 `role/subject` 过滤）。
4. 输出 `items[]`，并在每条 item 上标注 `source_org_node_id`（即 assignment 所在节点）。

### 6.3 缓存与失效（最小正确策略）
- **缓存 key（建议）**：
  - 继承解析：`tenant_id + hierarchy_type + effective_date(date) + include(resolved_attributes)`（可按日粒度缓存，避免无限 key）。
  - 角色查询：`tenant_id + org_node_id + effective_date(date) + include_inherited + filters`
- **失效触发（tenant 粗粒度）**：
  - 任何写入导致 `org.changed.v1` / `org.assignment.changed.v1` 投递成功后：清理该 tenant 的 inheritance/roles cache。
  - 若未来新增规则/角色写接口：写事务提交后清理该 tenant cache。
- **回退策略**：通过 feature flag 关闭（见 10.1），回退到“仅显式属性、不返回 resolved”与“roles endpoints disabled”。

## 7. 安全与鉴权 (Security & Authz)
- **租户隔离**：所有查询必须显式以 `tenant_id` 过滤；effective_date 必须使用 as-of 语义。
- **Authz（对齐 026）**：新接口必须用 `ensureAuthz`；403 返回 forbidden payload。
  - 建议映射（M2，选定）：
    - `GET /org/api/hierarchies`：`org.hierarchies` + `read`
    - `GET /org/api/nodes/{id}:resolved-attributes`：`org.hierarchies` + `read`
    - `GET /org/api/roles`：`org.roles` + `admin`
    - `GET /org/api/role-assignments`：`org.role_assignments` + `admin`

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/021-org-schema-and-constraints.md`：Org baseline（`org_nodes/org_node_slices/org_edges/...`）可用。
  - `docs/dev-plans/022-org-placeholders-and-event-contracts.md`：继承规则/角色/角色分配表已落地（若未落地，先完成迁移并通过 Org 工具链门禁）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403 payload/路由规范可复用。
  - `docs/dev-plans/027-org-performance-and-rollout.md`：query budget 思路与灰度口径可复用。
- **里程碑**：
  1. [ ] Resolver 与树读扩展（include=resolved_attributes）落地
  2. [ ] node resolved-attributes 调试端点落地
  3. [ ] roles/role-assignments 只读端点落地
  4. [ ] cache + tenant 失效策略落地
  5. [ ] 测试与门禁通过 + readiness 记录落盘

## 9. 测试与验收标准 (Acceptance Criteria)
- **继承解析正确性**：
  - 覆盖 `can_override=true/false`、多层级、祖先/子节点同时有值、全空、规则按时间窗切换等用例。
  - `resolved_sources` 能定位到期望的 `org_node_id`。
- **性能与查询预算**：
  - 继承解析不得出现 N+1（全树解析 SQL 查询数为常数级；roles include_inherited 查询数亦为常数级）。
- **矩阵/虚线只读守卫**：
  - 读接口能返回 `assignment_type=matrix/dotted`（如存在）；写接口继续返回 422 `ORG_ASSIGNMENT_TYPE_DISABLED`。
- **工程门禁**：
  - 按 `AGENTS.md` 命中触发器执行本地门禁；并在 `docs/dev-records/DEV-PLAN-028-READINESS.md` 记录命令、结果与时间戳。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 Feature Flag（建议）
- `ORG_INHERITANCE_ENABLED=true|false`（默认 `false`）：关闭时不计算/不返回 `resolved_attributes`。
- `ORG_ROLE_READ_ENABLED=true|false`（默认 `false`）：关闭时 `/org/api/roles` 与 `/org/api/role-assignments` 返回 disabled（实现自定，但需一致）。
- （可选）按租户 allowlist：复用 027 的 `ORG_ROLLOUT_MODE/ORG_ROLLOUT_TENANTS`，仅对灰度租户开启上述能力。

### 10.2 观测与排障
- 日志：在 resolver/roles 查询中输出 `tenant_id/org_node_id/effective_date` 与 cache hit/miss。
- 指标（可选）：inheritance cache 命中率、roles 查询耗时 P95、返回条数。

## 交付物
- `OrgInheritanceResolver` + 缓存与失效策略（实现与测试）。
- 继承解析 API 扩展与调试端点。
- `roles/role-assignments` 只读 API 与 Authz 映射。
- `docs/dev-records/DEV-PLAN-028-READINESS.md` 记录。
