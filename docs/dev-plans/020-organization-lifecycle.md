# DEV-PLAN-020：组织机构模块（对标 Workday）

**状态**: 草拟中（2025-01-15 11:30）

## 背景
- 现有 HRM 仅具备员工实体与基础表单，缺乏组织维度，导致薪酬、审批流与权限无法按照部门/成本中心划分。
- Workday 以“有效期管理 + 多层级组织 + 动态权限”著称，本计划以其核心能力为标杆，设计适配仓库 DDD 架构的组织模块。
- 模块需服务 HR、财务、采购、项目等多个领域，并支持“历史追溯 + 未来排程”的时间约束场景。

## 设计原则
1. **DDD 模块化**：新增 `modules/org`，严格遵守 AGENTS 规定的 domain/infrastructure/services/presentation 分层；禁止其他模块直接访问其内部实现，统一通过 service 接口或事件集成。
2. **有效期优先**：所有组织单元、层级关系、分配记录均采用“生效时间 / 失效时间”双字段，默认强制 `StartAt <= EndAt` 并避免重叠。
3. **多层级模型**：同时支持 Workday 的 Supervisory、Company、Cost Center、Custom Reporting 四类层级，通过 `HierarchyType` 与 `NodeType` 组合实现。
4. **生命周期驱动**：组织单元的创建、重命名、合并、撤销等动作均以“请求→审批→生效”三阶段执行，提供草稿与仿真视图。
5. **时间线 API**：所有读写接口必须显式接受 `effective_at`，未提供时默认 `time.Now()`，以保障历史查询与未来排程。
6. **可扩展事件流**：关键变更（新建部门、层级调整、员工调动、权限继承）通过 `pkg/eventbus` 发布，供 HRM/财务/审批模块订阅。

## 目标
1. 交付 Workday 级别的组织架构生命周期管理：草稿、审批、定时生效、撤销/回滚。
2. 建立完整的组织层级体系，支持至少四种层级以及跨层级的“虚拟/矩阵”关系。
3. 引入时序约束与冲突检测（同一员工同一时间仅能挂载单一 Supervisory 节点等）。
4. 提供 API/UI 以可视化组织树、变更时间线，并允许按时间点执行 Impact Analysis。
5. 为 HRM 员工模型、权限系统、成本核算提供统一的组织引用与缓存。

## 范围
- **组织单元（Org Unit）**：公司、事业部、部门、项目团队、自定义群组。
- **层级关系**：多棵树 + 侧向链接（矩阵汇报、共享服务线）。
- **时间维度**：组织、层级、分配、权限继承的有效期字段与校验。
- **流程引擎对接**：与现有审批/工作流复用同一 `pkg/workflow`（若缺失则提供最小审批服务）。
- **权限钩子**：组织层级变化触发 `pkg/authz` policy 生成建议。
- **非目标**：不实现薪酬预算、绩效考核，只预留事件。

## 关键业务蓝图
### 1. 组织结构与类型
- **Supervisory**：核心管理线，驱动员工直属关系、审批流。
- **Company / Legal Entity**：财务、税务主体；限制跨公司调动需要额外审批。
- **Cost Center**：费用归集；允许一个部门挂多个成本中心，但需时间切片。
- **Custom Reporting**：自定义标签型层级，用于报表和访问控制。
- 每个层级由 `Hierarchy` 实体管理（包含类型、根节点、版本策略），节点实体 `OrgNode` 存储基础属性与有效期；节点之间通过 `OrgEdge` 维护父子关系（同样有效期化）。

### 2. 生命周期管理
| 阶段 | 描述 | 关键约束 |
| --- | --- | --- |
| Draft | 业务负责人创建或编辑组织变更方案（可批量导入） | 校验重名/冲突，但不写主表 |
| Review | HR BP / 财务审批，支持多级签核与附件 | 检查预算/人数配额、并发冲突 |
| Scheduled | 审批通过后等待生效，可随时回滚 | 锁定版本，生成事件 |
| Active | 数据写入主表生效，发布事件、刷新缓存 | 与员工/权限挂接 |
| Retired | 被撤销或合并的单元，保留历史 | 禁止再次引用 |

### 3. 时间约束策略
- **有效期重叠检测**：`OrgNode`、`OrgEdge`、`Assignment`（员工隶属）在同一实体/维度下不得出现重叠区间。算法：保存所有区间后运行线段树或 SQL 约束（`EXCLUDE USING gist` + `tsrange`）。
- **冻结窗口**：敏感层级（公司、成本中心）在财务结账期（例如月底 +3 天）禁止生效变更，仅允许未来日期。
- **自动补齐**：当创建新版本时，上一版本自动 `EndAt = StartAt - 1day`，确保无空洞。
- **历史追溯**：查询接口 `GET /org/nodes/{id}?effective_at=2025-04-01` 返回当时名称、父级、属性；若请求未来时间，需检查是否存在安排中的变更。

### 4. 组织层级 & 权限
- 每个 Org Node 关联 `permission.Resource`（例如 `Org.Department.{id}`），用于 `pkg/authz` 推导访问控制。
- 支持 Workday 类“继承 + override”：默认继承父节点权限，可针对节点设置差异并生效于所有子节点（以 event 传播）。
- 组织变更触发：
  1. 生成 `OrgChanged` 领域事件，包含旧/新父节点、层级类型、时间范围。
  2. HRM 订阅后更新 `Employee.OrgAssignments` 并触发员工历史记录。
  3. Authz 订阅后调整 Casbin policy（或生成草稿 PR）。

## 技术方案
### Domain Layer
- `modules/org/domain/aggregates`:
  - `orgnode`：封装属性（名称、代码、负责人）、有效期、状态、行为（Rename, Merge, Split, ScheduleChange）。
  - `hierarchy`：管理层级类型、根节点、版本与约束。
  - `assignment`：员工/组织/职位的连接体，校验范围冲突。
- 值对象：`EffectiveWindow`（start/end + 校验）、`HierarchyType`、`NodeType`。
- 领域服务：`OrgLifecycleService`（处理 Draft→Active 流程）、`OrgTimeValidator`（检测重叠/冻结窗口）。

### Infrastructure Layer
- 新建 schema `modules/org/infrastructure/persistence/schema/org-schema.sql`，核心表：
  - `org_nodes`（id, tenant_id, type, code, name, status, effective_start, effective_end, parent_hint, owner_user_id, created_at, updated_at）。
  - `org_edges`（id, hierarchy_id, parent_node_id, child_node_id, effective_start, effective_end, depth, path ltree）。
  - `org_assignments`（id, node_id, subject_type(enum: employee, position, cost_center), subject_id, effective_start, effective_end, primary bool）。
  - `org_change_requests`（draft json、状态、审批轨迹、计划生效时间）。
  - 附加索引：`gist (node_id, tstzrange(effective_start, effective_end))` 用于时间冲突约束。
- sqlc 包：`modules/org/infrastructure/sqlc/...` 负责 CRUD + 冲突检测查询。
- 需要 Atlas/Goose 迁移流程，沿用 HRM 指南。

### Service Layer
- `OrgHierarchyService`：增删改查层级、生成树状数据、缓存。
- `OrgLifecycleService`：协调 change request，与 workflow/approval 接口交互。
- `OrgAssignmentService`：批量变更员工/职位隶属，确保事务性并通知 HRM。
- `OrgEffectiveDateService`：提供对外查询 API（给 HRM、财务），封装时间点解析、缺省策略。

### Presentation Layer & API
- Controller 前缀 `/org`：
  - `GET /org/hierarchies?type=Supervisory&effective_at=`：返回树概览。
  - `GET /org/nodes/{id}` / `PATCH /org/nodes/{id}`：支持 effective-dated 更新。
  - `POST /org/change-requests`：提交组织变更草稿（包含多个节点/关系/分配的批处理）。
  - `POST /org/change-requests/{id}/approve|reject|schedule|cancel`。
  - `GET /org/assignments?subject=employee:{id}`：返回时间线。
- UI（templ）：
  - 可视化树 + 时间线控件（顶部选择日期，树自动切换）。
  - Change Request Builder：拖拽式编辑、模拟生效影响（Workday 的 “What-if”）。
  - Impact Analysis 面板：列出受影响员工、审批流、权限差异。

## 集成与依赖
- **HRM 员工**：新增 `employee.OrgAssignments` 视图模型，表单需选中所属节点，默认 `effective_start = hire_date`。
- **Authz/Casbin**：组织节点作为 `object` 维度之一，`pkg/authz` 增加 `OrgScope` 属性用于 ABAC。
- **Workflow**：若现有 `pkg/workflow` 未覆盖，将在本计划 M1 同步补齐最小审批引擎或复用外部服务。
- **缓存**：树结构在 Redis/内存缓存，Key 含层级类型 + effective date（按日）。变更事件触发缓存失效。

## 里程碑
1. **M1：域模型与 schema**（2 周）
   - 定稿 `EffectiveWindow` 规则、OrgNode/Hierarchy 聚合、Atlas 迁移与 sqlc。
   - 单元测试覆盖核心校验（重叠检测、merge/split 行为）。
2. **M2：Lifecycle & Change Request**（3 周）
   - 实现 change request 仓储、服务、审批对接与事件。
   - API：`POST/GET /org/change-requests`，`PATCH /org/nodes`.
3. **M3：Assignments & HRM 集成**（2 周）
   - 完成员工/职位/成本中心分配服务 + HRM 调用（含 e2e 数据）。
   - 建立 `OrgChanged` 事件链与权限钩子。
4. **M4：Presentation & Impact Analysis**（2 周）
   - 完成树形 UI、时间轴、模板及多语言；加入 Impact Simulation。
5. **M5：优化与验收**（1 周）
   - 性能测试（1k 节点、10k 员工，查询延迟 < 200ms）、缓存策略、文档/培训。

## 验收标准
- 所有组织实体 CRUD 均可按任意 `effective_at` 查询/回滚，历史记录齐全。
- 变更流程能够覆盖“新增部门 + 未来生效 + 员工批量调动”场景，并能取消/重排。
- 与 HRM 员工列表联调通过：指派部门、查看历史、限制无效时间段。
- Authz 可消费组织事件并生成最少一条策略草稿。
- 文档：模块 README、API 参考、操作手册、dev-record 更新、Workday 对标总结。

## 风险与缓解
- **时间区间复杂度**：大量有效期校验可能拖慢写入 → 通过数据库 `EXCLUDE` 约束 + 应用层线段树缓存，并在事务内批量校验。
- **审批依赖**：Workflow 功能不全 → 若短期无法复用，提供轻量脚本审批或标记为前置依赖。
- **跨模块耦合**：HRM/财务需要同时改动 → 采用事件驱动 + 适配器，保持组织模块对外 API 稳定。
- **数据迁移**：需要从现有 Excel/静态结构导入 → 提供导入工具与校验报告，分租户试运行。

## 后续动作
- 记录本计划在 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 的 PoC/联调日志。
- 与产品/HR BP 对齐 Workday 关键流程（Supervisory、Matrix、Effective Dating）。
- 准备下一阶段（DEV-PLAN-021）聚焦“组织预算与人员编制”。
