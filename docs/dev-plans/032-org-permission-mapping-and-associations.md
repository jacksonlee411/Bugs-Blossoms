# DEV-PLAN-032：Org 权限映射与业务关联（Step 12）

**状态**: 已评审（2025-12-18 12:00 UTC）— 按 `docs/dev-plans/001-technical-design-template.md` 补齐可编码契约

## 0. 进度速记
- 本计划交付“可有效期化的映射表 + 只读权限预览接口”，用于支撑后续 workflow/报表/下游集成的 scope 计算与对账。
- 本计划 **不生成 Casbin 策略**：仅提供映射与预览（preview），策略变更通道仍以 `docs/dev-plans/015A-casbin-policy-platform.md` 为 SSOT。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` **步骤 12：权限映射与业务关联**。
- **关键动机**：
  - Org 主链（024/025/026）解决了“树/岗位/分配”的时间线一致性，但缺少“组织维度的权限 scope”与“组织对业务对象的关联”，导致后续安全域/审批路由/报表只能各自维护一套映射并产生漂移。
  - 需要一个 **可 as-of 查询、可继承计算、可解释来源（source node）** 的映射底座，并提供 preview 接口作为排障与 Readiness 的基线。
- **与仓库现状对齐**：
  - 仓库已 Hard Delete `modules/finance`（见 `docs/dev-plans/040-remove-finance-billing-crm.md` 与根 `AGENTS.md`）；因此本计划的“成本中心/预算科目”等关联只做 **外部 SOR 的 key 链接**，不新增 finance 侧 schema/代码耦合。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 落地 `org_security_group_mappings`：组织节点 ↔ 安全组映射（有效期 + “是否对子树继承”标记），并可按 as-of 查询。
  - [ ] 提供 **权限预览（read-only）** API：给定 `org_node_id + effective_date`，返回该节点“生效的安全组集合（含继承来源）”。
  - [ ] 落地 `org_links`：组织节点 ↔ 业务对象关联（项目/成本中心/预算科目等，使用外部 key；有效期化）。
  - [ ] 提供 `org_links` 的最小查询 API：支持按 `org_node_id` 或按 `object_type+object_key` 反查。
  - [ ] 支持 `dry-run` 的写入路径：本计划新增的写入指令必须可在 026 batch 中 dry-run（用于预检/演练），并可被 030 change-request payload 复用。
  - [ ] 通过本计划命中的 CI 门禁，并在 Readiness 中记录命令与结果（见 §9）。
- **非目标 (Out of Scope)**：
  - 不改动 Casbin `model.conf` 与现有 `policy.csv` 语义，不在本计划内引入“按 org scope 的强制鉴权”。
  - 不实现 workflow 路由、domain policy 生成、Authz policy draft 自动产出（后续计划承接）。
  - 不落地“跨模块强 FK”到外部业务表（例如项目/成本中心主数据表）；仅存外部 key 并由应用层校验（可选）。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划会新增 Org 迁移/Go 代码/API/Authz 策略片段；命令细节以 SSOT 为准，本文不复制门禁矩阵。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（repo/service/controller、preview 算法、测试）
  - [X] 路由治理（新增 `/org/api/security-group-mappings` / `/org/api/links` / `/org/api/permission-preview`）
  - [X] Authz（新增 endpoint → object/action 映射与策略片段）
  - [X] 迁移 / Schema（新增 `migrations/org/**` 与 schema 变更；必须按 021A 的 Org Atlas+Goose 工具链执行）
  - [X] 文档 / Readiness（新增 `DEV-PLAN-032-READINESS`）
  - [ ] `.templ` / Tailwind、多语言 JSON（本计划不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

### 2.2 与其他子计划的边界（必须保持清晰）
- 021：Org schema/约束 SSOT；032 仅新增两张表与对应索引/约束，不改动主链表语义。
- 026：Authz/403 payload/outbox/batch/snapshot SSOT；032 的写入必须以“单条接口 + batch 复用”形态落地，并复用 026 的 batch dry-run。
- 028：继承/角色读侧；032 的 “scope 继承”只针对 security group mapping 的子树继承，不替代 028 的继承解析与 role-assignment 语义。
- 030：change-requests/preflight；032 新增的写入指令必须可被 030 的 payload 复用（结构对齐 026 batch）。
- 031：数据质量与修复；032 的新表需在 031 的质量检查中按需补规则（例如 key 规范化），但 032 不定义质量规则 SSOT（以 031 为准）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Client[UI/CLI/Integrations] --> API[/org/api/*/]
  API --> Authz[ensureAuthz + ForbiddenPayload (026)]
  API --> S[Org Mapping Service]
  S --> Repo[Repo]
  Repo --> DB[(Postgres)]

  DB --> T1[(org_security_group_mappings)]
  DB --> T2[(org_links)]

  API --> Preview[Permission Preview]
  Preview --> DB
```

### 3.2 关键设计决策（ADR 摘要）
1. **有效期化（选定）**
   - `org_security_group_mappings` 与 `org_links` 均使用 `[effective_date, end_date)` 半开区间（UTC）并用 EXCLUDE 防重叠，保持与 Org 主链一致。
2. **继承标记用“对子树生效”表达（选定）**
   - 每条 security group mapping 持有 `applies_to_subtree`；preview 通过 `org_edges.path`（ltree）在 as-of 视图上计算祖先/子树关系，并返回 `source_org_node_id` 便于解释。
3. **执行面不生成策略（选定）**
   - v1 仅交付 mapping 与 preview；Authz 策略变更仍走 015A policy draft/bot；避免在 Org 内自建策略写通道造成 drift。
4. **batch 复用（选定）**
   - 为保证 preflight/change-request/dry-run 能复用同一合同，本计划新增的写操作同时提供单条接口与 batch command type（见 §5.5）。

## 4. 数据模型与约束 (Data Model & Constraints)
> 约定：PostgreSQL 17；UTC；半开区间 `[effective_date, end_date)`；EXCLUDE 依赖 `btree_gist`（由 021 baseline 已启用）。

### 4.1 `org_security_group_mappings`
**用途**：把“安全组（security group）”绑定到 OrgNode，并声明是否对子树继承；用于权限 scope 预览与后续下游订阅。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `id` | `uuid` | `pk` | `gen_random_uuid()` | mapping slice id |
| `org_node_id` | `uuid` | `not null` |  | FK → `org_nodes`（tenant 隔离） |
| `security_group_key` | `text` | `not null` |  | 安全组标识（v1 为字符串 key，不做跨模块 FK） |
| `applies_to_subtree` | `boolean` | `not null` | `true` | `true`=对子树继承；`false`=仅本节点 |
| `effective_date` | `timestamptz` | `not null` |  | 生效时间 |
| `end_date` | `timestamptz` | `not null` | `'9999-12-31'` | 失效时间 |
| `created_at` | `timestamptz` | `not null` | `now()` |  |
| `updated_at` | `timestamptz` | `not null` | `now()` |  |

**约束/索引（v1 选定）**
- `check (effective_date < end_date)`
- `check (char_length(trim(security_group_key)) > 0)`
- FK（tenant 隔离）：
  - `fk (tenant_id, org_node_id) -> org_nodes (tenant_id, id) on delete restrict`
- no-overlap（同 node + 同 key 的时间片不重叠）：
  - `exclude using gist (tenant_id with =, org_node_id with =, security_group_key with =, tstzrange(effective_date, end_date, '[)') with &&)`
- 索引：
  - `btree (tenant_id, org_node_id, effective_date)`
  - `btree (tenant_id, security_group_key, effective_date)`

### 4.2 `org_links`
**用途**：把 OrgNode 与外部业务对象（project/cost_center/…）建立多对多关联；支持有效期化与反查。

| 列 | 类型 | 约束 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `tenant_id` | `uuid` | `not null` |  | 租户 |
| `id` | `uuid` | `pk` | `gen_random_uuid()` | link slice id |
| `org_node_id` | `uuid` | `not null` |  | FK → `org_nodes`（tenant 隔离） |
| `object_type` | `text` | `not null` + check |  | v1：`project/cost_center/budget_item/custom` |
| `object_key` | `text` | `not null` |  | 外部对象 key（例如 project_code/cost_center_code） |
| `link_type` | `text` | `not null` + check |  | v1：`owns/uses/reports_to/custom` |
| `metadata` | `jsonb` | `not null` | `'{}'` | 扩展信息（必须是 object） |
| `effective_date` | `timestamptz` | `not null` |  |  |
| `end_date` | `timestamptz` | `not null` | `'9999-12-31'` |  |
| `created_at` | `timestamptz` | `not null` | `now()` |  |
| `updated_at` | `timestamptz` | `not null` | `now()` |  |

**约束/索引（v1 选定）**
- `check (effective_date < end_date)`
- `check (char_length(trim(object_key)) > 0)`
- `check (jsonb_typeof(metadata) = 'object')`
- `check (object_type in ('project','cost_center','budget_item','custom'))`
- `check (link_type in ('owns','uses','reports_to','custom'))`
- FK（tenant 隔离）：
  - `fk (tenant_id, org_node_id) -> org_nodes (tenant_id, id) on delete restrict`
- no-overlap（同 node + 同对象 + 同 link_type 的时间片不重叠）：
  - `exclude using gist (tenant_id with =, org_node_id with =, object_type with =, object_key with =, link_type with =, tstzrange(effective_date, end_date, '[)') with &&)`
- 索引：
  - `btree (tenant_id, org_node_id, effective_date)`
  - `btree (tenant_id, object_type, object_key, effective_date)`

### 4.3 迁移策略（Org Atlas+Goose）
- schema 源 SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`
- 新增迁移（示例命名）：`migrations/org/0000x_org_security_group_mappings_and_links.sql`
- 生成/lint/应用命令入口与门禁：见 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`（本文不复制）。

## 5. 接口契约 (API Contracts)
> 约定：内部 API 前缀 `/org/api`；JSON-only；Authz/403 payload 对齐 026；时间参数支持 `YYYY-MM-DD` 或 RFC3339，统一 UTC。

### 5.1 通用错误码（复用）
- 400 `ORG_INVALID_QUERY`
- 422 `ORG_INVALID_BODY`
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden（body 为 026 `ForbiddenPayload`）

### 5.2 `GET /org/api/security-group-mappings`
用于查询 mapping（支持 as-of 或返回全时间线）。

**Query**
- `org_node_id`：可选
- `security_group_key`：可选
- `effective_date`：可选（若提供则返回 as-of 视图；不提供则返回全量时间线）
- `limit`：可选，默认 `200`，最大 `1000`
- `cursor`：可选

**Response 200**
```json
{
  "tenant_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z|null",
  "items": [
    {
      "id": "uuid",
      "org_node_id": "uuid",
      "security_group_key": "wd:HRBP",
      "applies_to_subtree": true,
      "effective_window": { "effective_date": "2025-03-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
    }
  ],
  "next_cursor": null
}
```

### 5.3 `POST /org/api/security-group-mappings`
创建 mapping slice（Insert 语义）。

**Request**
```json
{
  "org_node_id": "uuid",
  "security_group_key": "wd:HRBP",
  "applies_to_subtree": true,
  "effective_date": "2025-03-01"
}
```

**Rules**
- `org_node_id/security_group_key/effective_date` 必填；禁止提交 `end_date`（由系统默认 `9999-12-31`；调整边界走 rescind/再次创建）。
- `security_group_key`：`TrimSpace` 后不得为空；v1 不强制格式，但建议使用可读前缀（如 `wd:`/`ext:`）。

**Response 201**
```json
{
  "id": "uuid",
  "effective_window": { "effective_date": "2025-03-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_NODE_NOT_FOUND_AT_DATE`：`org_node_id` 在 `effective_date` 不存在
- 409 `ORG_OVERLAP`：违反 no-overlap EXCLUDE

### 5.4 `POST /org/api/security-group-mappings/{id}:rescind`
撤销 mapping：在 `effective_date` 处终止有效窗。

**Request**
```json
{ "effective_date": "2025-04-01", "reason": "mapping removed" }
```

**Rules**
- `effective_date` 必须满足 `mapping.effective_date < effective_date < mapping.end_date`，否则 422 `ORG_INVALID_RESCIND_DATE`（复用 025 口径）。

### 5.5 batch 扩展（对齐 026）
> 为支持 030 change-request payload 复用，本计划新增 command type（并提供对应单条接口作为 SSOT）。

| type | 对应单条接口（SSOT） |
| --- | --- |
| `security_group_mapping.create` | `POST /org/api/security-group-mappings` |
| `security_group_mapping.rescind` | `POST /org/api/security-group-mappings/{id}:rescind` |
| `link.create` | `POST /org/api/links` |
| `link.rescind` | `POST /org/api/links/{id}:rescind` |

约定：
- `payload` 字段必须与单条接口 request body 一致；path 参数用 `payload.id` 传入（与 026 既有约定一致）。

### 5.6 `GET /org/api/links`
用于查询 link（支持 as-of 或返回全时间线）。

**Query**
- `org_node_id`：可选
- `object_type`：可选（`project/cost_center/budget_item/custom`）
- `object_key`：可选
- `effective_date`：可选（若提供则返回 as-of 视图；不提供则返回全量时间线）
- `limit`：可选，默认 `200`，最大 `1000`
- `cursor`：可选

**Response 200**
```json
{
  "tenant_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z|null",
  "items": [
    {
      "id": "uuid",
      "org_node_id": "uuid",
      "object_type": "cost_center",
      "object_key": "CC-100",
      "link_type": "uses",
      "metadata": {},
      "effective_window": { "effective_date": "2025-03-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
    }
  ],
  "next_cursor": null
}
```

### 5.7 `POST /org/api/links`
创建 link slice（Insert 语义）。

**Request**
```json
{
  "org_node_id": "uuid",
  "object_type": "cost_center",
  "object_key": "CC-100",
  "link_type": "uses",
  "metadata": {}
}
```

**Rules**
- `org_node_id/object_type/object_key/link_type` 必填；`metadata` 缺省 `{}`；禁止提交 `end_date`。
- `object_key`：`TrimSpace` 后不得为空；不自动改大小写。
- v1 仅允许 `object_type`：`project/cost_center/budget_item/custom`；仅允许 `link_type`：`owns/uses/reports_to/custom`。

**Response 201**
```json
{
  "id": "uuid",
  "effective_window": { "effective_date": "2025-03-01T00:00:00Z", "end_date": "9999-12-31T00:00:00Z" }
}
```

**Errors**
- 422 `ORG_NODE_NOT_FOUND_AT_DATE`
- 409 `ORG_OVERLAP`

### 5.8 `POST /org/api/links/{id}:rescind`
撤销 link：在 `effective_date` 处终止有效窗。

**Request**
```json
{ "effective_date": "2025-04-01", "reason": "link removed" }
```

**Rules**
- `effective_date` 必须满足 `link.effective_date < effective_date < link.end_date`，否则 422 `ORG_INVALID_RESCIND_DATE`。

### 5.9 `GET /org/api/permission-preview`
> 只读预览：给定 org node + as-of，返回该节点“生效的安全组集合（含继承来源）”与 as-of 的业务对象关联；用于排障/对账/Readiness。

**Query**
- `org_node_id`：必填
- `effective_date`：可选（缺省 `nowUTC`）
- `include`：可选，逗号分隔；允许值：
  - `security_groups`（默认）
  - `links`（默认）
- `limit_links`：可选，默认 `200`，最大 `1000`（仅对 links 生效）

**Response 200**
```json
{
  "tenant_id": "uuid",
  "org_node_id": "uuid",
  "effective_date": "2025-03-01T00:00:00Z",
  "security_groups": [
    {
      "security_group_key": "wd:HRBP",
      "applies_to_subtree": true,
      "source_org_node_id": "uuid",
      "source_depth": 3
    }
  ],
  "links": [
    {
      "object_type": "cost_center",
      "object_key": "CC-100",
      "link_type": "uses",
      "source": { "link_id": "uuid", "org_node_id": "uuid" }
    }
  ],
  "warnings": []
}
```

**Errors**
- 404 `ORG_NODE_NOT_FOUND_AT_DATE`
- 422 `ORG_INVALID_QUERY`
- 401/400/403：同 026

### 5.10 Authz 映射（MVP 固化）
> object 使用 `authz.ObjectName("org", "<resource>")`，action 使用 `read|admin`（026 口径）；v1 先收敛到 admin，避免泄露组织安全域。

| Endpoint | Object | Action |
| --- | --- | --- |
| `GET /org/api/security-group-mappings` | `org.security_group_mappings` | `admin` |
| `POST /org/api/security-group-mappings` | `org.security_group_mappings` | `admin` |
| `POST /org/api/security-group-mappings/{id}:rescind` | `org.security_group_mappings` | `admin` |
| `GET /org/api/links` | `org.links` | `admin` |
| `POST /org/api/links` | `org.links` | `admin` |
| `POST /org/api/links/{id}:rescind` | `org.links` | `admin` |
| `GET /org/api/permission-preview` | `org.permission_preview` | `admin` |

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 as-of 选择（通用）
- 任何 as-of 查询以 `effective_date <= t AND t < end_date` 取片段。
- `YYYY-MM-DD` 解释为 `00:00:00Z`（对齐 024/026）。

### 6.2 Permission Preview：安全组继承计算（v1）
输入：`tenant_id, org_node_id, t`

1. 取目标节点在 as-of 的路径：
   - `SELECT path, depth FROM org_edges WHERE tenant_id=? AND child_node_id=? AND effective_date<=t AND t<end_date`
   - 若不存在：404 `ORG_NODE_NOT_FOUND_AT_DATE`
2. 计算“对该节点生效”的 mapping：
   - 候选条件：
     - mapping 本身在 as-of 生效：`m.effective_date<=t AND t<m.end_date`
     - mapping 绑定节点在 as-of 存在：join `org_edges` as-of 获取 `m_node_path/m_node_depth`
   - 生效规则（v1）：
     - 若 `m.applies_to_subtree=true`：当且仅当 `target_path <@ m_node_path`（目标在其子树内）时生效
     - 若 `m.applies_to_subtree=false`：仅当 `m.org_node_id == target_node_id` 时生效
3. 输出时为每条 mapping 计算 `source_depth = m_node_depth`，并返回 `source_org_node_id` 作为可解释来源。
4. 去重策略（v1）：
   - 允许同一 `security_group_key` 同时来自多个 source（例如一个 key 被多个祖先重复配置）；preview **不去重**，而是完整返回，交由管理员排障（后续如需去重必须定义优先级规则）。

### 6.3 Permission Preview：links as-of 汇总（v1）
- 直接查询 `org_links` 在 as-of 生效的片段，并按 `limit_links` 截断：
  - 超限时返回 `warnings += ["links_truncated"]`，并提供 `links_count`（可选字段；v1 允许后续追加）。

### 6.4 写入语义（v1）
- `POST /security-group-mappings` / `POST /links`：
  - 预检：`org_node_id` 在 `effective_date` 必须存在（通过查 `org_edges` as-of 或 `org_nodes + org_edges` 组合）。
  - 写入：插入新 slice；DB EXCLUDE 拒绝重叠。
- `:rescind`：
  - 复用 025 的 rescind 判定：`start < effective_date < end`，并执行 `UPDATE ... SET end_date = effective_date`。

### 6.5 batch 扩展执行（对齐 026）
- 在 026 batch 的“单事务、多指令”框架内新增两类 handler：
  - `security_group_mapping.create/rescind`
  - `link.create/rescind`
- `dry_run=true`：在同一事务内执行后回滚；`events_enqueued=0`。
- 任一 command 失败：回滚并返回错误码，并在 `meta` 中携带 `command_index/command_type`（026 SSOT）。

## 7. 安全与鉴权 (Security & Authz)
- **Authz**：
  - 所有新端点必须使用 026 的 `ensureAuthz` 与 403 forbidden payload（不可自定义 403 形状）。
  - v1 先收敛到 `admin` 动作（见 §5.10），后续如需开放 read-only 给更多角色需另起评审并更新策略片段。
- **租户隔离**：
  - 所有查询/写入必须带 `tenant_id`；Repository 层不得提供“按 id 不带 tenant 读取”的方法。
- **RLS 兼容**：
  - 当 `RLS_ENFORCE=enforce` 时，事务内必须调用 `composables.ApplyTenantRLS`（对齐 019A/023/026）。
- **PII 最小化**：
  - 本计划的数据模型不存储人员姓名/email 等；`security_group_key/object_key` 视为业务标识，不视为 PII。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/021-org-schema-and-constraints.md`：ltree path/depth 与 as-of 查询基础。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403/batch 语义与 dry-run 框架。
  - `docs/dev-plans/030-org-change-requests-and-preflight.md`：payload 复用与预检链路（可选但推荐在 readiness 中演练）。
  - `docs/dev-plans/031-org-data-quality-and-fixes.md`：数据质量门槛（后续可增加对新表的质量规则）。
- **里程碑**：
  1. [ ] 迁移落地（新增两张表 + 索引/约束）并通过 Org Atlas+Goose 门禁。
  2. [ ] CRUD 最小接口（create/rescind/list）与 batch 扩展（dry-run）。
  3. [ ] `GET /org/api/permission-preview`（含继承来源）与测试守卫（租户隔离 + authz）。
  4. [ ] `docs/dev-records/DEV-PLAN-032-READINESS.md` 补齐（命令、输出摘要、演练结论）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **DB 约束**：
  - 对同一 `(tenant_id, org_node_id, security_group_key)` 插入重叠时间片必须被 EXCLUDE 拒绝。
  - 对同一 `(tenant_id, org_node_id, object_type, object_key, link_type)` 插入重叠时间片必须被 EXCLUDE 拒绝。
- **预览正确性**：
  - 同一节点上配置 `applies_to_subtree=false` 的 mapping 不应影响子节点预览。
  - 祖先节点配置 `applies_to_subtree=true` 的 mapping 应出现在子孙节点预览中，且 `source_org_node_id` 指向祖先节点。
- **工程门禁**：
  - Go 代码：按 `AGENTS.md` 执行 gofmt/vet/lint/test 并通过。
  - 迁移：按 021A 的 Org Atlas+Goose 工具链执行 plan/lint/migrate 并通过。
  - 文档：`make check doc` 通过。
- **Readiness 记录**：
  - 新增 `docs/dev-records/DEV-PLAN-032-READINESS.md`，记录：
    - 迁移命令与输出摘要
    - 最小 API 调用（create/list/preview/rescind）与返回示例（敏感信息打码）
    - batch dry-run 演练（含 meta.command_index/type 的失败示例至少 1 条）

## 10. 运维与回滚 (Ops & Rollback)
- **Feature Flags（契约）**：
  - `ORG_SECURITY_GROUP_MAPPINGS_ENABLED=true|false`（默认 `false`）：关闭时拒绝 mapping 写接口与 batch 对应 command（读接口/preview 可保留给 admin 用于排障，或一并关闭需在实现中明确）。
  - `ORG_LINKS_ENABLED=true|false`（默认 `false`）：同上。
  - `ORG_PERMISSION_PREVIEW_ENABLED=true|false`（默认 `false`）：关闭时 `GET /org/api/permission-preview` 返回 404 或 403（实现选定其一并写测试守卫）。
- **回滚**：
  - 数据层：优先用 `:rescind` 在有效期上终止错误配置；必要时追加新的 slice 覆盖未来生效（避免历史回写）。
  - Schema 层：通过 goose/atlas 回滚迁移（对齐 021A），禁止手工修补导致 drift。

## 11. 交付物 (Deliverables)
- 迁移：`org_security_group_mappings` + `org_links`（含约束与索引）。
- API：mapping/link 的 create/list/rescind + batch 扩展 + permission preview。
- Authz：策略片段与门禁记录（对齐 026/015A）。
- 文档：`docs/dev-records/DEV-PLAN-032-READINESS.md`。
