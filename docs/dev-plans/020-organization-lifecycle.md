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

## Workday 能力对齐
| Workday 关键点 | Workday 行为说明 | 本计划方案 | 差距/补充动作 |
| --- | --- | --- | --- |
| Supervisory / Company / Cost / Custom Hierarchies | 每个层级有独立版本与有效期，驱动 BP、财务和报表 | `HierarchyType` + 多棵树，提供 versioned OrgNode/OrgEdge | 需在 M2 完成“版本冻结 + 并行版本”能力，允许 Draft/Active 并存 |
| Business Process（BP）绑定 | 审批路由基于 Supervisory、Company 及 security group | Lifecycle 中接入 `pkg/workflow`，计划新增 BP 绑定表 | 需要在 M2/M3 引入 `org_bp_bindings` 与回调，支持 route preview |
| Security Domain / Group | Workday 通过 domain policy 授权到 org level，支持继承 | Org node 触发 `pkg/authz` 事件，新建 `OrgScope` ABAC 属性 | 需实现“组织节点 ↔ security group”映射与批量 policy 草稿出口 |
| Effective Dating & Retro Changes | 所有对象支持未来/过去生效，Retro 需影响历史审批/薪酬 | EffectiveWindow + retro correction API | 需在 M3 前定义 Retro API、冲突策略、audit log |
| Matrix / Shared Line | 员工可有主、辅组织用于审批/报表 | OrgEdge 支持 lateral link + assignment.primary 字段 | 需定义矩阵权限继承与 UI 提示 |
| Position Management | 职位必须挂载 Supervisory org，调动时影响 Budget/Comp | OrgAssignmentService 将 subject 扩展至 position/cost center | 需在 M3 与 HRM Position 实体打通，补充验证 |
| Impact Analysis & What-if Simulation | 变更前展示受影响员工、BP、security | Change Request Builder + Impact 面板 | 需列出默认指标（人数、薪酬总额、BP 列表）并持续扩展 |

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

**Workday 补充点**：
- Business Process（BP）绑定：每个 Change Request 记录 `bp_definition_id`，审批通过后写回 workflow，并可查询 BP route preview（展示每一级审批人）。
- Retroactive Correction：Active 后仍允许产生 `RetroChange`，以“撤销原记录 + 生成新记录”方式落表，并触发 HRM/财务补差事件。
- Mass Reorg：Change Request 支持角色“MassMove”，一次移动整棵子树或批量员工，默认生成 Impact 报告（人数、预算、BP 清单）。
- 并行版本：Scheduled 状态的数据形成“Parallel Version”，允许提前与 Active 版本对比；合并时进行冲突检测。
### 3. 时间约束策略
- **有效期重叠检测**：`OrgNode`、`OrgEdge`、`Assignment`（员工隶属）在同一实体/维度下不得出现重叠区间。算法：保存所有区间后运行线段树或 SQL 约束（`EXCLUDE USING gist` + `tsrange`）。
- **冻结窗口**：敏感层级（公司、成本中心）在财务结账期（例如月底 +3 天）禁止生效变更，仅允许未来日期。
- **自动补齐**：当创建新版本时，上一版本自动 `EndAt = StartAt - 1day`，确保无空洞。
- **历史追溯**：查询接口 `GET /org/nodes/{id}?effective_at=2025-04-01` 返回当时名称、父级、属性；若请求未来时间，需检查是否存在安排中的变更。
- **Retro 传播**：当 Retro API 修改过去记录时，会重新生成 `OrgChanged` 与 `OrgAssignmentChanged` 事件，并标记影响范围供薪酬/审批补记。
- **Mass Transfer 窗口**：针对同一员工在 Workday 的“Primary/Additional Supervisory Org”策略，实现“主组织唯一 + 辅组织多选但有权重”的约束。

### 4. 组织层级 & 权限
- 每个 Org Node 关联 `permission.Resource`（例如 `Org.Department.{id}`），用于 `pkg/authz` 推导访问控制。
- 支持 Workday 类“继承 + override”：默认继承父节点权限，可针对节点设置差异并生效于所有子节点（以 event 传播）。
- 组织变更触发：
  1. 生成 `OrgChanged` 领域事件，包含旧/新父节点、层级类型、时间范围。
  2. HRM 订阅后更新 `Employee.OrgAssignments` 并触发员工历史记录。
  3. Authz 订阅后调整 Casbin policy（或生成草稿 PR）。
- 引入 `OrgSecurityDomain` 与 `OrgSecurityGroup` 概念：节点可绑定一个或多个 security domain（对应 Workday Domain Security Policy），系统根据继承链生成“访问/审批/报表”三类策略，支持 override 与矩阵共享；安全继承计算由 `OrgSecurityService` 缓存并暴露 API。

## 技术方案
### Domain Layer
- `modules/org/domain/aggregates`:
  - `orgnode`：封装属性（名称、代码、负责人）、有效期、状态、行为（Rename, Merge, Split, ScheduleChange）。
  - `hierarchy`：管理层级类型、根节点、版本与约束。
  - `assignment`：员工/组织/职位的连接体，校验范围冲突与 primary/secondary。
  - `orgchange`：封装 Change Request、RetroChange、MassMove 等业务行为，内置 BP 绑定、冲突检测。
- 值对象：`EffectiveWindow`（start/end + 校验）、`HierarchyType`、`NodeType`、`SecurityDomain`, `SecurityGroup`, `BpRoute`.
- 领域服务：`OrgLifecycleService`（处理 Draft→Active/Retro 流程）、`OrgTimeValidator`（检测重叠/冻结窗口）、`OrgSecurityService`（计算继承/override）、`OrgBusinessProcessAdapter`（生成审批路由与 impact）。

### Infrastructure Layer
- 新建 schema `modules/org/infrastructure/persistence/schema/org-schema.sql`，核心表：
  - `org_nodes`（id, tenant_id, type, code, name, status, effective_start, effective_end, parent_hint, owner_user_id, created_at, updated_at）。
  - `org_edges`（id, hierarchy_id, parent_node_id, child_node_id, effective_start, effective_end, depth, path ltree）。
  - `org_assignments`（id, node_id, subject_type(enum: employee, position, cost_center), subject_id, effective_start, effective_end, primary bool, allocation_percent）。
  - `org_change_requests`（draft json、BP id、状态、审批轨迹、计划生效/终止时间、impact summary）。
  - `org_retro_changes`（源记录、矫正记录、差异说明、审批信息）。
  - `org_security_domains` / `org_security_groups`：映射节点与安全域/组、权限继承链。
  - `org_bp_bindings`：记录每个层级/节点关联的 BP 定义，用于 route preview。
  - `org_version_snapshots`：保存并行版本与 impact 结果，方便对比。
  - 附加索引：`gist (node_id, tstzrange(effective_start, effective_end))` 用于时间冲突约束。
- sqlc 包：`modules/org/infrastructure/sqlc/...` 负责 CRUD + 冲突检测查询。
- 需要 Atlas/Goose 迁移流程，沿用 HRM 指南。

### Service Layer
- `OrgHierarchyService`：增删改查层级、生成树状数据、缓存。
- `OrgLifecycleService`：协调 change request，与 workflow/approval 接口交互，可输出 BP route preview、支持并行版本合并。
- `OrgAssignmentService`：批量变更员工/职位/岗位/成本中心隶属，确保事务性并通知 HRM/财务。
- `OrgRetroService`：处理 RetroChange、回滚、审计。
- `OrgEffectiveDateService`：提供对外查询 API（给 HRM、财务、项目），封装时间点解析、缺省策略。
- `OrgSecurityService`：计算 security domain/group 继承，发布 `SecurityPolicyChanged` 事件。
- `OrgBusinessProcessAdapter`：封装与 `pkg/workflow` 或外部 BP 定义的集成，支持 route simulation。

### Presentation Layer & API
- Controller 前缀 `/org`：
  - `GET /org/hierarchies?type=Supervisory&effective_at=`：返回树概览。
  - `GET /org/nodes/{id}` / `PATCH /org/nodes/{id}`：支持 effective-dated 更新。
  - `POST /org/change-requests`：提交组织变更草稿（包含多个节点/关系/分配的批处理）。
  - `POST /org/change-requests/{id}/approve|reject|schedule|cancel`。
  - `POST /org/change-requests/{id}/simulate`：生成 What-if 报告（BP 路由、人数、薪酬影响、权限差异）。
  - `POST /org/retro-changes`：执行 retro correction。
  - `GET /org/assignments?subject=employee:{id}`：返回时间线（含 primary/secondary 与 allocation）。
  - `GET /org/security/nodes/{id}`：查看 security domain/group 继承情况。
- UI（templ）：
  - 可视化树 + 时间线控件（顶部选择日期，树自动切换）。
  - Change Request Builder：拖拽式编辑、模拟生效影响（Workday 的 “What-if”）。
  - Impact Analysis 面板：列出受影响员工、审批流、权限差异、预算/岗位数，并可导出。
  - Business Process Route Preview：在审批阶段展示实际审批链。
  - Security Inspector：类似 Workday 的 domain/group 视图，显示节点继承、override、缺口策略。

## 集成与依赖
- **HRM 员工**：新增 `employee.OrgAssignments` 视图模型，表单需选中所属节点，默认 `effective_start = hire_date`。
- **Authz/Casbin**：组织节点作为 `object` 维度之一，`pkg/authz` 增加 `OrgScope` 属性用于 ABAC。
- **Workflow**：若现有 `pkg/workflow` 未覆盖，将在本计划 M1 同步补齐最小审批引擎或复用外部服务。
- **Position/Compensation**：HRM Position/JobProfile/Comp 模块必须引用 Supervisory org，OrgAssignmentService 提供钩子确保职位移动时同步预算/薪酬计划。
- **Finance/Projects/Procurement**：Cost Center 与 Company 层级需要与 finance 模块共享，事件 `OrgCostCenterChanged` 触发总账/项目的成本归集更新。
- **缓存**：树结构在 Redis/内存缓存，Key 含层级类型 + effective date（按日）。变更事件触发缓存失效。
- **Reporting/Analytics**：提供 `org_reporting` 视图供 BI 工具使用，支持任意时间点快照，与 Workday Custom Reporting 对齐。

## 里程碑
1. **M1：域模型与 schema + Workday 映射**（2 周）
   - 定稿 `EffectiveWindow` 规则、OrgNode/Hierarchy/OrgChange 聚合、Atlas 迁移与 sqlc。
   - 输出 Workday 对齐表 v1，单元测试覆盖重叠检测、merge/split、版本冻结。
2. **M2：Lifecycle & BP/安全域绑定**（3 周）
   - 实现 change request 仓储、并行版本、BP route preview、security domain 映射。
   - API：`POST/GET /org/change-requests`，`POST /org/change-requests/{id}/simulate`，`PATCH /org/nodes`。
3. **M3：Assignments & Retro/HRM/Position 集成**（3 周）
   - 完成员工/职位/成本中心分配、Retro API，HRM/Position/Finance 调用与事件订阅。
   - 建立 `OrgChanged`、`OrgAssignmentChanged`、`SecurityPolicyChanged` 事件链。
4. **M4：Presentation & Workday 体验**（2 周）
   - 完成树形 UI、时间轴、Impact Simulation、Security Inspector、BP route 预览、多语言。
   - 发布 Workday 场景（mass move、future-dated、retro correction）演示脚本。
5. **M5：优化、验证与培训**（1-2 周）
   - 性能测试（1k 节点、10k 员工，查询延迟 < 200ms）、缓存策略、BI 导出、文档/培训、Workday parity checklist 签署。

## 验收标准
- 所有组织实体 CRUD 均可按任意 `effective_at` 查询/回滚，历史记录齐全，并支持 Retro API 生成矫正记录。
- 变更流程覆盖典型 Workday 场景：Future-dated move、Mass reorganization、Retro correction、Parallel version merge，均能输出 Impact 报告。
- HRM/Position/Finance 集成通过：员工/职位/成本中心分配更新、审批路由自动切换、财务成本归集无遗漏。
- Authz 可消费组织事件生成 security domain/group 策略草稿，并能在 Security Inspector 可视化继承链。
- Workflow/BP 路由与 Change Request 绑定，审批模拟结果与实际执行一致，支持 route export。
- 文档完整：模块 README、API 参考、操作手册、dev-record、Workday parity checklist。

## 风险与缓解
- **时间区间复杂度**：大量有效期校验可能拖慢写入 → 通过数据库 `EXCLUDE` 约束 + 应用层线段树缓存，并在事务内批量校验。
- **审批依赖**：Workflow 功能不全 → 若短期无法复用，提供轻量脚本审批或标记为前置依赖。
- **跨模块耦合**：HRM/财务需要同时改动 → 采用事件驱动 + 适配器，保持组织模块对外 API 稳定。
- **数据迁移**：需要从现有 Excel/静态结构导入 → 提供导入工具与校验报告，分租户试运行。
- **安全策略复杂度**：Workday 风格的 domain/group 继承易爆炸 → 通过缓存 + diff 预览 + policy 草稿流程，限制一次变更的影响面，并提供回滚脚本。
- **Retro 影响面**：Retro 更改可能触发薪酬/财务补差链路 → 在 Retro API 中强制生成 impact report，并要求相关模块确认后才能提交。
- **Workday 对齐失败**：若关键场景无法如期复刻，会影响产品定位 → 设立 parity checklist，每个里程碑复盘，并可临时降级成“最小可用”但在计划中保留后续补强任务。

## 后续动作
- 记录本计划在 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 的 PoC/联调日志。
- 与产品/HR BP 对齐 Workday 关键流程（Supervisory、Matrix、Effective Dating）。
- 建立 Workday parity checklist（含 BP、安全域、Retro、Position 管理）并在每个里程碑更新。
- 准备下一阶段（DEV-PLAN-021）聚焦“组织预算与人员编制”。
