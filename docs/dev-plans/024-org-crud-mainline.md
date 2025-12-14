# DEV-PLAN-024：Org 主链 CRUD（Person→Position→Org）

**状态**: 规划中（2025-12-13 更新）

## 背景
- 对应 020 步骤 4，承接 021（schema 与约束）与 022（占位表/事件契约）、023（导入/回滚），交付 Person→Position→Org 单树的核心 CRUD（可写可查），包含“创建 Assignment 时自动创建空壳 Position”的主链能力。
- 024 的定位是**把主链写路径跑通**并给 025（时间/审计）与 026（API/Authz/事件闭环）提供清晰落点；不在本计划内承诺审批流、继承解析与完整 UI 体验。

## 目标
- Person→Position→Org 单树 CRUD 可用且通过租户/Session 校验。
- 自动创建一对一空壳 Position 的逻辑生效。
- M1 仅开放 primary assignment 写入，matrix/dotted 保持只读占位或特性开关关闭。
- 全链路遵守 DDD 分层/cleanarch 约束，禁止跨层耦合。
- 所有接口/服务接受 `effective_date` 参数，缺省按 `time.Now()` 处理。

## 范围与非目标
- 范围：`OrgNode`/`OrgEdge`/`Position`/`OrgAssignment` 主链 CRUD（单树），自动创建一对一空壳 Position，Session+租户强校验，最小 REST/HTMX 控制器与最小页面（便于本地验证）。
- 非目标：不开放 matrix/dotted 写入（默认关闭）、不实现草稿/审批/预检/仿真/retro、不实现继承解析/角色业务逻辑、不实现缓存策略与对账接口。
- 与子计划边界（必须保持清晰）：
  - 021：仅负责 schema/约束/迁移；024 不改约束定义。
  - 022：仅负责占位表与事件**契约定义**；024 复用契约类型，不新增自定义字段。
  - 023：仅负责 CSV 导入/回滚工具；024 不做批量导入。
  - 025：负责冻结窗口、Correct/Update/Rescind 的审计与时间线强校验（含 ShiftBoundary）；024 仅实现主链 CRUD 的基础写路径并预留扩展点。
  - 026：负责 `pkg/authz` 接入与策略片段、事件投递闭环（outbox/对账/缓存失效）、`/org/snapshot` 与 `/org/batch`；024 不在本计划内落地这些增强能力。
  - 035：负责完整 Org UI；024 仅提供最小可用页面用于开发验证。

## 依赖与里程碑
- 依赖：
  - 021：核心表/约束可用（至少 `org_nodes/org_node_slices/org_edges/positions/org_assignments` 迁移完成）。
  - 022：`OrgChangedEvent/OrgAssignmentChangedEvent` 契约类型可引用（字段对齐以 022 为准）。
  - 023：仅用于本地准备数据与回滚（不影响 024 代码结构）。
- 里程碑（按提交时间填充）：模块骨架 -> repo/service CRUD -> controller/DTO/mapper -> 自动 Position -> 最小页面 -> 测试与 readiness 记录。

## 设计决策
### 1. 模块结构（对齐 DDD/cleanarch）
- 目录骨架（M1）：
  - `modules/org/domain/aggregates`：`orgnode/orgedge/position/assignment`（主链聚合）。
  - `modules/org/infrastructure/persistence`：sqlc 查询 + repository 实现（强制 tenant 过滤）。
  - `modules/org/services`：业务编排（默认写入语义、自动 Position、校验调用）。
  - `modules/org/presentation/controllers`：REST/HTMX handlers（仅做 session+tenant 校验与 DTO 解析；Authz 接入在 026）。
  - `modules/org/presentation/templates`：最小页面（开发验证用途，完整 UI 在 035）。

### 2. 数据模型映射（对齐 021）
- `org_nodes`：稳定标识（被 `org_edges/positions` 外键引用），字段含 `code/is_root/type`。
- `org_node_slices`：节点属性时间片（`name/i18n_names/status/manager_user_id/... + effective_date/end_date`），同节点区间不重叠（EXCLUDE）。
- `org_edges`：父子关系时间片（`parent_node_id/child_node_id/path/depth + effective_date/end_date`），同 child 区间不重叠（EXCLUDE），`path/depth` 由触发器维护。
- `positions`：绑定 `org_node_id` 的岗位时间片（`code/title/status/is_auto_created + effective_date/end_date`）。
- `org_assignments`：人员分配时间片（`position_id/subject_type/subject_id/pernr/assignment_type/is_primary + effective_date/end_date`），primary 唯一与重叠由约束兜底。

### 3. API 入口（对齐 020；Authz/批量/对账在 026）
- 读：
  - `GET /org/hierarchies?type=OrgUnit&effective_date=`：树概览（M1 仅 OrgUnit）。
  - `GET /org/assignments?subject=person:{id}&effective_date=`：人员分配时间线。
- 写（M1 主链）：
  - `POST /org/nodes`：创建节点（写入 `org_nodes` + 首个 `org_node_slices` + `org_edges`（非 root））。
  - `PATCH /org/nodes/{id}`：按 Insert 语义新增时间片（end_date 自动计算；细节在 025）。
  - `POST /org/assignments`：创建 primary assignment；可不传 `position_id` 触发自动创建空壳 Position。
  - `PATCH /org/assignments/{id}`：按 Insert 语义更新（M1 可先仅支持字段子集，复杂分支在 025）。
- 预留但不在 024 完整落地：
  - `POST /org/nodes/{id}:correct`、`POST /org/nodes/{id}:rescind` 等高权限写入（行为审计与冻结窗口在 025）。
  - `GET /org/snapshot`、`POST /org/batch`（在 026）。

### 4. 写入语义与校验（基础路径在 024；强约束/审计在 025）
- Update（Insert）：请求仅提交 `effective_date`（缺省 `time.Now()`），服务层按 025 的 Insert 算法计算 `end_date` 并在同事务内完成“截断旧片段 + 插入新片段”。
- Correct/Rescind：024 仅预留 API/Service 入口与权限占位；审计字段、冻结窗口与 ShiftBoundary 在 025 落地。
- 校验分层：
  - DB 兜底：EXCLUDE/unique/trigger（021）。
  - Service 预检：tenant 过滤、外键存在性、`parent_hint` 与 edge 一致性、primary 唯一的业务口径、错误码稳定化（冻结窗口与无空档强校验在 025）。
- 移动节点（MoveNode）：父节点变更是独立语义，禁止直接 `UPDATE parent_node_id`；必须通过“失效旧边 + 创建新边”（同一 `effective_date`）实现。由于 `ltree path` 的子树级联更新是重型操作，MoveNode 需单独的 Service 方法与更严格的锁顺序；并发/冻结窗口/审计口径在 025 加固，性能基线与优化在 027。

### 5. 自动创建空壳 Position（M1 必做）
- 触发条件：创建 Assignment 且未提供 `position_id`（或提供 `org_node_id` 替代）。
- 行为：在同事务内创建 `positions(is_auto_created=true)` 并继续创建 assignment；默认一人一岗仅作为写链路便捷，不视为编制体系。
- 命名与元数据：
  - `code`：使用保留前缀 `AUTO-` + 确定性短 hash（建议基于 `tenant_id/org_node_id/subject_type/subject_id`），用于并发去重与审计定位；M1 不要求对外可读。
  - `title`：可为固定模板（如 `Member of {OrgNodeCode}`）或为空；不要求随 OrgNode 改名自动同步，UI 展示优先使用 OrgNode slice 的 `name`。
- 幂等与并发：高并发下通过“确定性 code + 事务内冲突处理（重试/读回）”避免重复创建；统一的幂等键（`Idempotency-Key`/`request_id`）在 025/026 固化。
- 数据治理（后续）：频繁调动可能遗留无引用的 `is_auto_created=true` Position，M1 不做自动清理；治理策略（例如清理无 assignment 的空壳、或统一 Retire）在 031（数据质量）或后续运维计划中落地。

### 6. 事件生成与发布（对齐 022；投递闭环在 026）
- 024 的交付口径：在写入成功后生成 `OrgChangedEvent/OrgAssignmentChangedEvent` 并通过应用内 `EventBus` 发布（字段对齐 022；`transaction_time=now()`、`initiator_id` 来自 Session；`entity_version/sequence` 先按最小策略生成，完整审计/幂等口径在 025/026 加固）。
- 事件可靠投递（Transactional Outbox、缓存失效、对账接口）不在 024，统一在 026 落地。

### 7. 最小 UI（与 035 边界）
- 024 仅提供最小页面用于验证 CRUD：节点列表/创建/编辑、Assignment 创建与列表。
- 组织可视化、报告、深交互（拖拽/批量调整/权限申请入口等）统一在 035。

## 任务清单与验收标准
1. [ ] 模块骨架：创建 `modules/org`（DDD 目录）与 `module.go/links.go/permissions/constants.go`，保证可被 app 注册且不破坏 cleanarch 约束。验收：`go test ./...` 编译通过（仅作为烟囱验证；完整门禁在 readiness）。
2. [ ] Repository（sqlc + tenant 过滤）：实现 `org_nodes/org_node_slices/org_edges/positions/org_assignments` 的最小查询与写入 repo（Create/InsertUpdate/List/AsOf），所有查询强制 tenant 过滤。验收：repo 层测试覆盖基本 CRUD 与 tenant 隔离。
3. [ ] Service（主链写路径）：实现 Node/Assignment 的 Create 与 Update（Insert）主路径；实现 MoveNode（失效旧边+创建新边）；Update 的 `end_date` 计算与截断逻辑按 025 的算法落地（先跑通主路径，冻结窗口/审计在 025）。验收：服务层测试覆盖有效期基础路径、MoveNode 主路径与 DB 约束冲突报错稳定。
4. [ ] 自动 Position：实现 “Create Assignment -> 自动创建空壳 Position” 主路径与特性开关；明确矩阵/虚线写入默认拒绝；落地自动 Position 的 `code/title` 生成策略与并发去重策略。验收：测试覆盖有/无 position_id、开关关闭、并发冲突/重试、重复请求（幂等键在 025/026 固化）。
5. [ ] Controller/DTO：实现最小 REST/HTMX 控制器，强制 Session+tenant 校验，解析 `effective_date`（默认 now）并返回稳定错误结构；权限判定的 `pkg/authz` 接入与策略片段在 026。验收：controller 测试覆盖无 Session/无 tenant/参数非法/租户隔离。
6. [ ] 事件生成（应用内发布）：写入成功后生成并发布 `OrgChanged/OrgAssignmentChanged`（字段对齐 022；`transaction_time/initiator_id` 必填；`entity_version/sequence` 先按最小策略生成，审计与幂等加固在 025/026）。验收：测试验证事件字段齐全且发布时机在事务成功后。
7. [ ] 最小 templ 页面：提供节点/Assignment 的最小页面用于人工验证；执行 `templ generate && make css`（如有模板变更）。验收：生成后 `git status --short` 干净。
8. [ ] Readiness：执行 `make check lint` 与 `go test ./modules/org/...`（或影响路径），必要时补 `templ generate && make css`；将命令/耗时/结果记录到 `docs/dev-records/DEV-PLAN-024-READINESS.md`。

## 验证记录
- 将测试/生成/检查命令与结果写入 `docs/dev-records/DEV-PLAN-024-READINESS.md`，若有对账或临时文件需在文档中引用，确认 `git status --short` 干净。

## 风险与回滚/降级路径
- 业务风险：自动 Position 可能重复生成，需幂等键保护并允许回滚；若冲突，提供开关关闭自动创建并要求显式 position_id。
- 兼容风险：matrix/dotted 默认关闭，如未来打开需增加权限/验证与事件版本演进。
- 发布回滚：若 CRUD 写入导致约束冲突，可回滚至导入前快照或使用 023 rollback 清理 seed；迁移回滚遵循 021 的 org 迁移目录，避免影响 HRM。

## 交付物
- 主链 CRUD 代码与测试（domain/service/repo/controller/mapper）。
- 自动 Position 逻辑与事件生成（应用内发布）。
- 基础 templ 页面及生成产物（生成后工作区干净）。
- 文档与验证记录（`docs/dev-records/DEV-PLAN-024-READINESS.md`）。
