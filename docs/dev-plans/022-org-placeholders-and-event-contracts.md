# DEV-PLAN-022：Org 占位表与事件契约

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 2，需提前创建占位表（继承规则、角色、change_requests 草稿等）并定义 `OrgChanged` / `OrgAssignmentChanged` 事件契约。

## 目标
- 占位表结构就绪且不破坏 M1 范围。
- 事件 payload 定义覆盖 assignment_type、继承属性、change_type/initiator/version/timestamp 及幂等键等扩展字段。
- 生成命令（sqlc/atlas）执行后工作区干净。

## 实施步骤
### 待补充的实施细节
为了让开发人员能更顺畅地实施，建议在方案中进一步明确以下技术细节：

1.  **占位表的详细 Schema 设计**
    *   **`org_attribute_inheritance_rules`**: 配置属性继承策略。
        *   **字段**: `tenant_id uuid not null`, `hierarchy_type text`, `attribute_name text`, `can_override bool default false`, `inheritance_break_node_type text`, `effective_date timestamptz not null`, `end_date timestamptz not null default '9999-12-31'`, `created_at/updated_at timestamptz default now()`。
        *   **约束/索引**: `check (effective_date < end_date)`；`unique (tenant_id, attribute_name, hierarchy_type)`；索引 `(tenant_id, hierarchy_type, effective_date)`。
        *   **说明**: M1 仅结构占位，继承解析逻辑后续实现。
    *   **`org_roles` 与 `org_role_assignments`**: 角色定义与分配。
        *   **`org_roles` 字段**: `tenant_id`, `id uuid pk`, `code`, `name`, `description`, `created_at/updated_at`; 约束：`unique(tenant_id, code)`，索引 `(tenant_id, name)`。
        *   **`org_role_assignments` 字段**: `tenant_id`, `id uuid pk`, `role_id` (fk -> org_roles), `subject_id` (user_id), `org_node_id` (fk -> org_nodes), `effective_date timestamptz not null`, `end_date timestamptz not null default '9999-12-31'`, `created_at/updated_at`。
        *   **约束/索引**: `check (effective_date < end_date)`；`exclude using gist (tenant_id with =, subject_id with =, org_node_id with =, role_id with =, tstzrange(effective_date, end_date) with &&)`；索引 `(tenant_id, org_node_id, effective_date)`、`(tenant_id, subject_id, effective_date)`。
    *   **`change_requests`**: 存储变更草稿。
        *   **字段**: `tenant_id`, `id uuid pk`, `requester_id` (fk -> users), `subject_type` (text), `subject_id` (uuid), `status` (text)，`payload` (jsonb), `notes` (text), `version int default 1`, `created_at/updated_at timestamptz default now()`。
        *   **约束与索引**: `status` check（`draft/pending/approved/rejected/cancelled`）；索引 `(tenant_id, requester_id, status)`、`(tenant_id, subject_type, subject_id)`；`check (payload is not null)`。
        *   **`payload` 结构**: 与实体 API 请求体一致，便于直接应用变更。

2.  **事件契约的具体 Go 结构与字段**
    *   事件需包含版本/幂等信息，方便下游兼容与去重。
    *   **`OrgChangedEvent` 结构示例**:
        ```go
        // OrgChangedEvent 代表组织节点或边的变更
        type OrgChangedEvent struct {
            EventID         uuid.UUID       `json:"event_id"`        // 幂等键
            EventVersion    int             `json:"event_version"`   // 事件 schema 版本
            Timestamp       time.Time       `json:"timestamp"`       // 事件发生时间
            TenantID        uuid.UUID       `json:"tenant_id"`
            ChangeType      string          `json:"change_type"`     // 如 "NodeCreated", "NodeUpdated"
            InitiatorID     uuid.UUID       `json:"initiator_id"`    // 操作发起人
            EntityVersion   int             `json:"entity_version"`  // 实体的版本号
            EntityType      string          `json:"entity_type"`     // "OrgNode" 或 "OrgEdge"
            EntityID        uuid.UUID       `json:"entity_id"`
            EffectiveWindow EffectiveWindow `json:"effective_window"`
            OldValues       json.RawMessage `json:"old_values,omitempty"`
            NewValues       json.RawMessage `json:"new_values"`
            Sequence        int64           `json:"sequence"`        // 可选顺序号/offset
        }
        ```
    *   **`OrgAssignmentChangedEvent` 结构示例**: 同上，包含 `AssignmentID`, `PositionID`, `SubjectID`, `AssignmentType`, `IsPrimary`，共享 `EventID/EventVersion/Sequence`。
    *   **说明**: 事件契约需明确 Topic/Subject 命名（如 `org.changed.v1`、`org.assignment.changed.v1`），并约定重放/幂等策略由 `event_id` + `sequence` 控制，outbox 实现需保证与事务一致。

3.  **服务层集成点**
    *   草稿保存：API 支持 `?draft=true`，服务层将变更写入 `change_requests`（状态 `draft`），不改主数据，校验租户与 payload schema。
    *   事件发布：写操作在事务内生成事件，提交前写入 outbox（`EventPublisher` 接口），`event_id` 作为幂等键，消费者需支持重放。

4.  **sqlc 查询与生成**
    *   为 `change_requests` 提供基础查询：
        *   `-- name: CreateChangeRequest :one`
        *   `-- name: GetChangeRequest :one`
        *   `-- name: ListChangeRequestsByRequester :many`
        *   `-- name: ListChangeRequestsBySubject :many`
        *   `-- name: UpdateChangeRequestStatus :one`
    *   `org_roles`、`org_role_assignments` 可先提供 `List`/`Upsert` 查询。

### 实施步骤（细化后）
1. [ ] **Schema 描述与迁移**: 在 `modules/org/infrastructure/atlas/schema.hcl` 中按上述设计添加占位表（租户键/有效期/EXCLUDE/索引）。运行 `atlas migrate diff` 生成迁移脚本，确保与 `DEV-PLAN-021` 兼容。
2. [ ] **事件契约定义**: 在 `modules/org/domain/events` 中定义 `OrgChangedEvent`、`OrgAssignmentChangedEvent`，包含 `event_id/event_version/sequence` 元数据与文档说明。
3. [ ] **sqlc 查询与服务层占位**: 为 `change_requests`/`org_roles`/`org_role_assignments` 创建 SQL 并运行 `make sqlc-generate`；服务层添加草稿保存与事件发布占位（可空实现）。
4. [ ] **验证与记录**: 运行 `make db lint`、goose 上下行；生成后确认 `make sqlc-generate`/其他命令执行后 `git status --short` 干净；在 `docs/dev-records/DEV-PLAN-022-READINESS.md` 记录命令与结果。

## 交付物
- 占位表迁移与 schema 更新。
- 事件契约说明或类型定义。
