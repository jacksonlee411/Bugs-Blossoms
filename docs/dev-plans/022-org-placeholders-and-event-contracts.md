# DEV-PLAN-022：Org 占位表与事件契约

**状态**: 规划中（2025-12-13 更新）

## 背景
- 对应 020 步骤 2，需提前创建占位表（继承规则、角色、change_requests 草稿等）并定义 `OrgChanged` / `OrgAssignmentChanged` 事件契约。

## 目标
- 占位表结构就绪且不破坏 M1 范围。
- 事件 payload 定义覆盖 assignment_type、继承属性、change_type/initiator/entity_version/transaction_time/effective_window 及幂等键等扩展字段。
- 生成命令（sqlc/atlas）执行后工作区干净。

## 范围与非目标
- 范围：为组织域新增继承规则占位表、角色与角色分配表、变更草稿表，以及对应事件契约/服务占位与 sqlc 查询。
- 非目标：不实现继承/角色/草稿的业务逻辑、不发布事件到实际总线、不改前台界面或 API 体验（除草稿占位开关）。

## 依赖与里程碑
- 依赖：基于 DEV-PLAN-021 已落地的 `modules/org/infrastructure/atlas/schema.hcl` 版本生成迁移，保持同一租户模型与 org 节点约束。
- 里程碑（可按提交时间填充）：迁移草稿 -> 事件契约定义 -> sqlc 生成与占位 -> 验证记录完成。

## 设计决策
- 占位表
  - `org_attribute_inheritance_rules`：字段 `tenant_id uuid not null`, `hierarchy_type text`, `attribute_name text`, `can_override bool default false`, `inheritance_break_node_type text`, `effective_date timestamptz not null`, `end_date timestamptz not null default '9999-12-31'`, `created_at/updated_at timestamptz default now()`；约束 `check (effective_date < end_date)`、`exclude using gist (tenant_id with =, hierarchy_type with =, attribute_name with =, tstzrange(effective_date, end_date) with &&)`（同一属性规则按时间片不重叠），索引 `(tenant_id, hierarchy_type, attribute_name, effective_date)`。M1 仅结构占位。
  - `org_roles` 与 `org_role_assignments`：`org_roles` 含 `tenant_id`, `id uuid pk`, `code`, `name`, `description`, `created_at/updated_at`，约束 `unique(tenant_id, code)`，索引 `(tenant_id, name)`；`org_role_assignments` 含 `tenant_id`, `id uuid pk`, `role_id` (fk -> org_roles), `subject_id` (user_id), `org_node_id` (fk -> org_nodes), `effective_date timestamptz not null`, `end_date timestamptz not null default '9999-12-31'`, `created_at/updated_at`，约束 `check (effective_date < end_date)`、`exclude using gist (tenant_id with =, subject_id with =, org_node_id with =, role_id with =, tstzrange(effective_date, end_date) with &&)`，索引 `(tenant_id, org_node_id, effective_date)`、`(tenant_id, subject_id, effective_date)`。
    - 与 021 的 `org_node_slices.manager_user_id` 的关系：M1 阶段“负责人/直线经理”以 `manager_user_id` 为准（读性能优先且写链路最短），`org_role_assignments` 仅作为通用角色能力的结构占位，不作为 M1 的核心事实来源；当后续启用 `org_roles`（如 Manager/HRBP/Finance Controller）时再明确一致性策略（例如对 Manager 做单向映射/双写/读合并）。
  - `change_requests`：`tenant_id`, `id uuid pk`, `requester_id` (fk -> users), `subject_type` (text), `subject_id` (uuid), `status` (text), `payload_schema_version int not null default 1`, `payload` (jsonb), `notes` (text), `created_at/updated_at timestamptz default now()`；约束 `status` in (`draft/pending/approved/rejected/cancelled`)、`check (payload is not null)`，索引 `(tenant_id, requester_id, status)`、`(tenant_id, subject_type, subject_id)`；`payload` 必须与对应 API 的 JSON 结构一致（字段命名策略保持一致），并通过 `payload_schema_version` 支持向后兼容与迁移。
- 事件契约
  - 需包含版本/幂等信息，Topic 命名如 `org.changed.v1`、`org.assignment.changed.v1`，幂等策略 `event_id` + `sequence`，outbox 与事务一致。
  - `OrgChangedEvent` 示例：
    ```go
    // OrgChangedEvent 代表组织节点或边的变更。
    // 注意：TransactionTime 是事件记录/提交时间（transaction time），EffectiveWindow 才是变更的生效时间窗（valid time）。
    // 消费者必须按 EffectiveWindow 处理“未来生效”的变更，禁止将 TransactionTime 误当作生效时间。
    type OrgChangedEvent struct {
        EventID         uuid.UUID       `json:"event_id"`
        EventVersion    int             `json:"event_version"`
        TransactionTime time.Time       `json:"transaction_time"`
        TenantID        uuid.UUID       `json:"tenant_id"`
        ChangeType      string          `json:"change_type"`     // 如 "NodeCreated", "NodeUpdated"
        InitiatorID     uuid.UUID       `json:"initiator_id"`
        EntityVersion   int             `json:"entity_version"`
        EntityType      string          `json:"entity_type"`     // "OrgNode" 或 "OrgEdge"
        EntityID        uuid.UUID       `json:"entity_id"`
        EffectiveWindow EffectiveWindow `json:"effective_window"`
        OldValues       json.RawMessage `json:"old_values,omitempty"`
        NewValues       json.RawMessage `json:"new_values"`
        Sequence        int64           `json:"sequence"`
    }
    ```
  - `OrgAssignmentChangedEvent`：同上，包含 `AssignmentID`, `PositionID`, `SubjectID`, `AssignmentType`, `IsPrimary`，共享 `EventID/EventVersion/Sequence`。
  - `OldValues/NewValues`：必须使用与对应 API DTO 一致的 JSON 字段命名与语义（避免 snake_case/camelCase 混用）；`OldValues` 在 Create 场景可省略，其余变更建议提供以便审计与回放。
- 服务层集成点：API 支持 `?draft=true` 将变更写入 `change_requests`（状态 `draft`）不改主数据；写操作采用 Transactional Outbox，业务写入与 outbox 插入必须在同一数据库事务内提交，避免“数据已改但事件未发/事件已发但数据未改”的不一致；消费者需支持重放与幂等处理。
- sqlc 查询：`change_requests` 提供 `CreateChangeRequest` / `GetChangeRequest` / `ListChangeRequestsByRequester` / `ListChangeRequestsBySubject` / `UpdateChangeRequestStatus`；`org_roles`、`org_role_assignments` 提供基础 `List`/`Upsert`。

## 任务清单与验收标准
1. [ ] Schema 与迁移：在 `modules/org/infrastructure/atlas/schema.hcl` 补充上述表/索引/约束，`atlas migrate diff` 生成迁移并通过 `make db lint`，与 DEV-PLAN-021 结构不冲突。
2. [ ] 事件契约定义：在 `modules/org/domain/events` 定义 `OrgChangedEvent`、`OrgAssignmentChangedEvent`（含 `event_id/event_version/sequence`），并记录 Topic/幂等策略。
3. [ ] sqlc 查询与服务占位：新增 SQL 并运行 `make sqlc-generate`，服务层添加草稿保存与事件发布占位（可空实现），`git status --short` 保持干净。
4. [ ] 验证与记录：goose 上下行验证迁移；记录运行的命令与结果到 `docs/dev-records/DEV-PLAN-022-READINESS.md`。

## 验证记录
- 命令与结果请在执行后写入 `docs/dev-records/DEV-PLAN-022-READINESS.md`，确认 `make sqlc-generate`、`make db lint`、goose 上下行后工作区无额外 diff。

## 风险与回滚/降级路径
- 迁移风险：新表/索引需保持与现有 org 节点/租户约束一致，若冲突可使用 `GOOSE_STEPS=1 goose -dir migrations/org postgres "$DSN" down` 撤回最新步。
- 契约风险：事件字段冻结前避免下游依赖，若需调整通过事件版本号与 Topic 版本演进，不改已有版本。

## 交付物
- 占位表迁移与 schema 更新。
- 事件契约说明或类型定义。
- 验证记录（命令与结果）更新。
