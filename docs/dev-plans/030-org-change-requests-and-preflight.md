# DEV-PLAN-030：Org 变更请求与预检（Step 10）

**状态**: 规划中（2025-12-18 02:01 UTC）

## 0. 进度速记
- 本计划对应 `docs/dev-plans/020-organization-lifecycle.md` 的步骤 10：在 M1 CRUD/API/审计/outbox 基线稳定后，引入“变更请求（draft/submit）+ 影响预检（preflight）”能力。
- `org_change_requests` 的表结构与 payload 形状 SSOT 以 `docs/dev-plans/022-org-placeholders-and-event-contracts.md` 为准；本计划补齐 API、权限、预检输出与测试口径。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` → 步骤 10「开发变更请求流程」。
- **当前痛点**：
  - 目前 Org 的写入口（024/025/026）可直接修改主数据，但缺少“草稿化/可审阅的变更描述”，难以在灰度/生产场景下做审阅与演练。
  - 缺少预检（Pre-flight）能力：在提交变更前无法回答“会影响哪些员工/岗位/节点/事件”，只能先改再回滚，风险高且成本大。
  - 变更请求与预检若无明确权限边界与租户隔离守卫，容易暴露敏感数据或导致跨租户扫描。
- **业务价值**：
  - 用 `org_change_requests` 承载“变更草稿 → 提交”的最小闭环（workflow 未启用时不触发审批/路由），降低变更引入的风险与沟通成本。
  - 预检输出“影响摘要”，为后续审批/窗口冻结/回滚演练提供可追溯输入。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 提供 `org_change_requests` 的最小 API：创建草稿、更新草稿、读取、提交、取消（不执行变更）。
  - [ ] 提供 Pre-flight API：对草稿（或 payload）执行结构校验 + 业务校验 + 影响分析，输出稳定 JSON 合同。
  - [ ] workflow 未启用时：提交仅改变 `status` 并落盘审计信息，不触发任何外部路由/执行主数据写入/不写 outbox。
  - [ ] 测试：覆盖无权限/有权限/租户隔离（含“跨租户读取/提交必须失败”）。
  - [ ] Readiness：新增/更新 `docs/dev-records/DEV-PLAN-030-READINESS.md`，记录门禁命令与结果。
- **非目标（本计划明确不做）**：
  - 不实现审批/排程/生效（Approve/Schedule/Activate）完整流程（后续阶段另立 dev-plan；workflow 模块启用前不做）。
  - 不实现“执行草稿变更”（apply）入口；实际写入仍通过 026 的 `/org/api/batch` 等已冻结契约完成。
  - 不重写 024/025/026 的写语义与错误码（本计划只复用并做预检分析）。
  - 不建设影响分析的长期报告/可视化（归属 033/034）。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划会新增 Org API、可能新增 Authz 策略片段与路由登记，并新增 Go 代码与测试；命令与门禁以 SSOT 为准，本文不复制矩阵。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（service/repo/controller、预检分析、测试）
  - [X] 路由治理（新增 `/org/api/change-requests*` / `/org/api/preflight`）
  - [X] Authz（新增 object/action 映射与 `config/access/policies/org/*.csv` 策略片段）
  - [X] 文档 / Readiness（新增 030 readiness record）
  - [ ] 迁移 / Schema（仅当 022 的占位表尚未落地或需补索引/约束时；若触发需走 Org Atlas+Goose）
  - [ ] `.templ` / Tailwind（不涉及）
  - [ ] 多语言 JSON（不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

### 2.2 与其他子计划的边界（必须保持清晰）
- 022：`org_change_requests` 表结构与 `payload` 形状 SSOT；030 不应定义与 022 冲突的字段/语义。
- 024/025：业务写语义与稳定错误码 SSOT；预检必须复用相同校验逻辑，避免“预检通过但执行失败”。
- 026：Authz/403 payload/outbox/caching 与 `/org/api/batch` 合同 SSOT；030 的 payload 必须复用 026 batch 的 `commands[]` 形状。
- 018：路由治理与 JSON-only 错误契约；新 `/org/api/*` 路由必须通过 routing gate。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Client[UI/CLI] --> API[/org/api/change-requests & preflight/]
  API --> Authz[ensureAuthz (026)]
  API --> CR[ChangeRequestService]
  CR --> Repo[(org_change_requests repo)]
  Repo --> DB[(Postgres)]
  API --> PF[PreflightService]
  PF --> Repos[Org read/write services (024/025/026)]
  PF --> Impact[Impact Analyzer]
  Repos --> DB
```

### 3.2 关键设计决策（ADR 摘要）
1. **payload 复用 batch 合同（选定）**
   - `org_change_requests.payload` 必须复用 026 的 `/org/api/batch` request 结构（022 已选定），避免“草稿 payload 与执行 payload 漂移”。
2. **草稿与提交只落盘，不执行（选定）**
   - workflow 未启用阶段：任何草稿/提交都不得写主数据表、不得写 outbox、不得发布事件。
3. **幂等键使用 `(tenant_id, request_id)`（选定）**
   - `request_id` 来自 `X-Request-Id`（或服务端生成）；用于草稿重复提交幂等化与链路串联（对齐 022）。
4. **预检使用“可复用校验管线 + 影响分析”两段式（选定）**
   - 第一段：复用 batch/单条接口的结构与业务校验（确保与执行一致）。
   - 第二段：计算影响摘要（受影响节点/岗位/人员/事件数量等），并对输出规模做限流与裁剪。

## 4. 数据模型与约束 (Data Model & Constraints)
> `org_change_requests` 表结构 SSOT：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`。

### 4.1 `org_change_requests`（草稿/提交）
- 关键字段（v1）：
  - `tenant_id uuid`
  - `id uuid`
  - `request_id text`（与 `X-Request-Id` 对齐；`unique (tenant_id, request_id)`）
  - `requester_id uuid`（发起人标识；语义对齐 022/026/015A：**user uuid**，不依赖 `users.id` 的 int8）
  - `status text`：`draft/submitted/approved/rejected/cancelled`（本计划只实现 `draft/submitted/cancelled` 的状态机；其余为占位）
  - `payload_schema_version int`（固定 `1`）
  - `payload jsonb`（复用 026 batch request）
  - `notes text`
  - `created_at/updated_at timestamptz`

### 4.2 状态机（M2 最小版）
- `draft` → `submitted`（提交）
- `draft` → `cancelled`（取消）
- `submitted` → `cancelled`（取消；允许撤回）
- 任何非 `draft` 的记录禁止修改 `payload`（避免“提交后内容漂移”）。

## 5. 接口契约 (API Contracts)
> 约定：内部 API 前缀 `/org/api`；JSON-only；Authz/403 payload 对齐 026。

### 5.1 `POST /org/api/change-requests`（创建/幂等 upsert 草稿）
**Request**
```json
{
  "notes": "why we need this change",
  "payload": {
    "effective_date": "2025-03-01",
    "commands": [
      { "type": "node.update", "payload": { "id": "uuid", "effective_date": "2025-03-01", "name": "New Name" } }
    ]
  }
}
```

**Rules**
- `payload` 必填；其形状与合法性以 026 batch 为 SSOT（未知 command/type 视为 422）。
- 幂等：以 `(tenant_id, request_id)` 唯一约束实现“重复创建草稿”的幂等（022 建议）。
- 仅允许写入 `status=draft`；不得写主数据表/不得写 outbox。

**Response 201/200**
```json
{
  "id": "uuid",
  "request_id": "uuid",
  "status": "draft",
  "created_at": "2025-12-18T02:01:00Z",
  "updated_at": "2025-12-18T02:01:00Z"
}
```

**Errors**
- 401 `ORG_NO_SESSION`
- 400 `ORG_NO_TENANT`
- 403 Forbidden（对齐 026 forbidden payload）
- 422 `ORG_CHANGE_REQUEST_INVALID_BODY`

### 5.2 `PATCH /org/api/change-requests/{id}`（更新草稿）
**Request**
```json
{
  "notes": "updated notes",
  "payload": {
    "effective_date": "2025-03-01",
    "commands": [
      { "type": "node.update", "payload": { "id": "uuid", "effective_date": "2025-03-01", "name": "New Name" } }
    ]
  }
}
```

**Rules**
- 仅允许 `status=draft` 更新；否则 409 `ORG_CHANGE_REQUEST_NOT_DRAFT`。

### 5.3 `GET /org/api/change-requests`（列表）
**Query**
- `status`：可选（`draft|submitted|cancelled`）
- `limit`：可选（默认 50，最大 200）
- `cursor`：可选（v1：`updated_at:<rfc3339>:id:<uuid>`）

**Response 200**
```json
{
  "total": 1,
  "items": [
    {
      "id": "uuid",
      "request_id": "uuid",
      "status": "draft",
      "requester_id": "uuid",
      "updated_at": "2025-12-18T02:01:00Z"
    }
  ],
  "next_cursor": null
}
```

### 5.4 `GET /org/api/change-requests/{id}`（详情）
**Response 200**
```json
{
  "id": "uuid",
  "tenant_id": "uuid",
  "request_id": "uuid",
  "requester_id": "uuid",
  "status": "draft",
  "payload_schema_version": 1,
  "payload": { "effective_date": "2025-03-01", "commands": [] },
  "notes": "…",
  "created_at": "2025-12-18T02:01:00Z",
  "updated_at": "2025-12-18T02:01:00Z"
}
```

### 5.5 `POST /org/api/change-requests/{id}:submit`（提交）
**Rules**
- 仅允许从 `draft` 提交；提交后 `payload` 不可再修改。
- workflow 未启用阶段：提交仅更新状态并落盘；不得触发执行/路由/outbox。

### 5.6 `POST /org/api/change-requests/{id}:cancel`（取消/撤回）
**Rules**
- 允许 `draft/submitted` 取消，目标状态为 `cancelled`。

### 5.7 `POST /org/api/preflight`（影响预检）
> 预检用于“在不执行写入”的前提下输出影响摘要；其输入 payload 与 batch 一致，避免双合同。

**Request**（与 026 batch 对齐）
```json
{
  "effective_date": "2025-03-01",
  "commands": [
    { "type": "node.update", "payload": { "id": "uuid", "effective_date": "2025-03-01", "name": "New Name" } }
  ]
}
```

**Response 200**
```json
{
  "effective_date": "2025-03-01T00:00:00Z",
  "commands_count": 1,
  "impact": {
    "org_nodes": { "create": 0, "update": 1, "move": 0, "rescind": 0 },
    "org_assignments": { "create": 0, "update": 0, "rescind": 0 },
    "events": { "org.changed.v1": 1, "org.assignment.changed.v1": 0 },
    "affected": {
      "org_node_ids_count": 1,
      "org_node_ids_sample": ["uuid"]
    }
  },
  "warnings": []
}
```

**Errors**
- 401/400/403：同 026
- 409/422：复用 024/025/026 的稳定错误码，并在 `meta` 中附带 `command_index/command_type`
- 422 `ORG_PREFLIGHT_TOO_LARGE`：影响面过大（例如 move 子树过大导致输出/计算超限）

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 草稿写入（幂等）
1. 从 ctx 解析 `tenant_id`、`requester_id`、`request_id`（`X-Request-Id`；缺省生成）。
2. 结构校验 `payload`（对齐 batch：commands 长度、type 合法、payload 必填字段）。
3. `insert … on conflict (tenant_id, request_id) do update`：
   - 仅在 `status=draft` 时允许覆盖 `payload/notes`；
   - 非 draft 冲突时返回 409 `ORG_CHANGE_REQUEST_IMMUTABLE`。

### 6.2 Preflight（校验 + 影响分析）
1. 复用 batch 的解析与“effective_date 注入”规则（见 026）。
2. 校验阶段：
   - 复用 024/025 的 service 校验逻辑（保证与执行一致）。
3. 影响分析阶段（最小版）：
   - 对每个 command 统计影响计数（node/edge/assignment 等）。
   - 对涉及 move 的 command：使用 `org_edges.path` 的 as-of 子树查询估算影响面（node_ids_count），并对超限返回 `ORG_PREFLIGHT_TOO_LARGE`。
   - `events`：按 022 的 topic 规则估算（M2 先按 command 类型映射计数；不要求生成完整 payload）。
4. 输出：返回“计数 + sample ids（截断）+ warnings”的稳定 JSON。

## 7. 安全与鉴权 (Security & Authz)
- **租户隔离**：
  - `org_change_requests` 的所有 CRUD 必须以 `tenant_id` 过滤；禁止通过 id 跨租户读取。
  - preflight 禁止做“全库扫描”；必须在 tenant 范围内计算影响面。
- **Authz（对齐 026 的 ensureAuthz）**：
  - 建议 object/action（M2，选定）：
    - `org.change_requests`：`read/write/admin`
    - `org.preflight`：`admin`（预检可能暴露影响面与人员信息，先收敛到 admin）
  - 403 payload：统一复用 026 的 forbidden payload。
- **PII 最小化**：
  - preflight 输出默认只返回 `*_count` 与 `*_sample`（受限长度）；如需输出人员列表必须明确增加分页/limit，并受更严格权限保护（后续阶段再扩展）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/022-org-placeholders-and-event-contracts.md`：`org_change_requests` 表结构与 payload 形状 SSOT。
  - `docs/dev-plans/024-org-crud-mainline.md`：稳定错误码与命令语义（node/assignment）。
  - `docs/dev-plans/025-org-time-and-audit.md`：冻结窗口与 Correct/Rescind/ShiftBoundary 的校验口径（预检必须复用）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403 payload/batch 合同（payload 复用）。
- **里程碑**：
  1. [ ] `org_change_requests` 迁移落地（若 022 尚未落地）
  2. [ ] change-requests API（create/update/get/list/submit/cancel）
  3. [ ] preflight API（校验 + 影响摘要）
  4. [ ] Authz 策略片段与门禁通过
  5. [ ] 测试与 readiness 记录落盘

## 9. 测试与验收标准 (Acceptance Criteria)
- **权限**：
  - 无权限访问 change-requests/preflight 返回 403（forbidden payload 对齐 026）。
- **租户隔离**：
  - 跨租户读取/提交同一 change_request_id 必须失败（404 或 403，口径固定并测试覆盖）。
- **预检一致性**：
  - 同一 payload 在 `preflight` 与未来 `batch(dry_run)` 的校验结果一致（错误码/定位一致）。
- **工程门禁**：
  - 按 `AGENTS.md` 命中触发器执行门禁；并将命令/输出记录到 `docs/dev-records/DEV-PLAN-030-READINESS.md`。

## 10. 运维、回滚与降级 (Ops / Rollback)
- feature flag（建议）：
  - `ORG_CHANGE_REQUESTS_ENABLED=true|false`（默认 `false`）：关闭时直接禁用 change-requests API。
  - `ORG_PREFLIGHT_ENABLED=true|false`（默认 `false`）：关闭时禁用 preflight API（或返回明确错误）。
- 回滚策略：
  - 若仅功能问题：直接关闭 feature flag。
  - 若迁移已落地且需回滚：按 Org Atlas+Goose 工具链执行 down（生产优先通过 flag 回滚，避免 drop 表）。

## 交付物
- `org_change_requests` 草稿/提交 API（含权限/租户隔离）。
- `POST /org/api/preflight`（影响摘要）与测试。
- Authz 策略片段与门禁通过（如涉及 `config/access/**`）。
- `docs/dev-records/DEV-PLAN-030-READINESS.md`（在落地时填写）。  
