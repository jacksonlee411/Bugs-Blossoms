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
- **MVP（Phase 0/1，对齐主流 org 基线）**：单一 Supervisory 树 + OrgNode/OrgEdge/Assignment CRUD，强制有效期/去重名/无重叠、租户隔离查询性能（1k 节点 200ms 内）、审计/冻结窗口和主数据接口（ID/Code 规范、引用口径）。
- **Phase 2**：扩展多层级（Company/Cost/Custom），仍保持“先主组织后矩阵”，矩阵/侧向链接仅提供数据模型占位与只读视图，不影响主链约束。
- **Phase 3**：Lifecycle & BP 绑定、并行版本、Retro、更完整冲突检测（MassMove/版本合并）。
- **Phase 4**：Impact Analysis/UI（树 + 时间轴 + What-if），口径锁定：员工数/FTE、岗位数、成本额、审批链；数据源/滞后性在文档中声明。
- 组织/职位/岗位边界：本计划仅覆盖组织层级与“人/职位/成本中心隶属”关系，不引入编制/空岗管理，Position 编制控制在后续 DEV-PLAN-021 处理。
- 持续 Workday 对齐，但每阶段可独立上线，确保 HRM/Authz/Finance 等依赖至少获得稳定的组织主数据引用与事件。

## 范围
- **主数据最小集（Phase 0/1）**：单一 Supervisory 树，必备属性（code、name、parent、effective window、tenant），强制去重名/无重叠/租户隔离性能；父子校验、审计、冻结窗口为硬约束。
- **扩展层级**：多棵树（Company/Cost/Custom）在后续阶段逐步放量，默认不影响主 Supervisory 约束；矩阵/侧向链接仅在 Phase 2+ 开启且首版只读占位。
- **时间维度**：组织、层级、分配、权限继承的有效期字段与校验。
- **流程引擎对接**：与现有审批/工作流复用同一 `pkg/workflow`（若缺失则提供最小审批服务）；下游审批编排属于后置集成，不阻塞主数据上线。
- **权限钩子**：组织层级变化触发 `pkg/authz` policy 生成建议，按阶段纳入 CI（见里程碑）。
- **主数据治理**：编码规则（唯一/长度/前缀）、命名规范、必填属性/字典校验、发布模式（API + 事件 + 批量导入）、冲突处理与审核责任写入文档；Org 为组织层级 SOR，Position/编制留在后续 DEV-PLAN-021，Cost Center/Finance 仅消费事件/视图（冻结期不改 schema）。
- **非目标**：不实现薪酬预算、绩效考核，不做编制/空岗管理，只预留事件。

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
- 并发与锁：同一节点+时间窗口的 Draft/Review/Scheduled 需序列化，使用 `(tenant_id, node_id, tstzrange)` 应用锁/DB 锁防止双重提交；变更执行时生成幂等 token，重复执行跳过已处理批次。
- 生效批次与补偿：Scheduled→Active 支持按批次（节点/子树分批）幂等执行，记录批次游标，可重试失败批次；提供“跳过已执行步骤/重放事件”机制，避免半成品。
- 预验证与脏数据隔离：大批量导入先跑离线校验报告（必填/编码/命名/字典/重名/时间重叠），不通过不入 Draft；Draft 仅存隔离空间，不污染主表，审核通过后才写并发检查。
- Retro/结账耦合：Retro 请求需检查 payroll/审批结账周期，强制生成补差/重放计划（事件重播、下游对账）；若下游已消费，标记漂移并要求对账确认后执行。
- 版本推广：并行版本支持环境/审批模板版本绑定（dev→staging→prod），promotion/回滚有审计记录；审批模板版本化，变更绑定模板版本，回滚可快速切换至上一个稳定版本。
- 对象生命周期：Org 对象需覆盖创建（Draft→Active）、变更（重命名/移动/属性更新，需有效期与无重叠校验）、停用/注销（Retired/Disabled，禁止新分配，历史保留）、重新启用（新有效期段，需与旧段不重叠）、合并/拆分（Retired 原节点并生成新节点/边）、软删除（仅限误创建的草稿/未发布记录，Active 数据禁止硬删，需通过 Retro/撤销变更）、永久删除（仅治理脚本在隔离环境执行，生产路径禁用）。
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
  - `org_nodes`（tenant_id, id, type, code, name, status, effective_start, effective_end, parent_hint, owner_user_id, created_at, updated_at）。
  - `org_edges`（tenant_id, id, hierarchy_id, parent_node_id, child_node_id, effective_start, effective_end, depth, path ltree）。
  - `org_assignments`（tenant_id, id, node_id, subject_type(enum: employee, position, cost_center), subject_id, effective_start, effective_end, primary bool, allocation_percent）。
  - `org_change_requests`（tenant_id, id, draft json、BP id、状态、审批轨迹、计划生效/终止时间、impact summary）。
  - `org_retro_changes`（tenant_id, id, 源记录、矫正记录、差异说明、审批信息）。
  - `org_security_domains` / `org_security_groups`：映射节点与安全域/组、权限继承链。
  - `org_bp_bindings`：记录每个层级/节点关联的 BP 定义，用于 route preview。
  - `org_version_snapshots`：保存并行版本与 impact 结果，方便对比。
  - 附加索引：`gist (tenant_id, node_id, tstzrange(effective_start, effective_end))` 用于时间冲突约束。
- 多租户隔离：所有主键/唯一约束均以 `(tenant_id, <id/unique fields>)` 复合，外键与 sqlc 查询强制带 tenant 过滤；路径/缓存 key 同样纳入 tenant，避免跨租户穿透。
- Postgres 依赖：迁移启用 `ltree` 与 `btree_gist` 扩展；有效期字段统一 `tstzrange` 且使用 `[start, end)` 半开区间，写入/校验一律 UTC，迁移包含时区/边界说明。
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
- **SOR 边界**：Org 模块为组织层级 SOR；HRM/Position 为人/岗位 SOR，Position 编制/空岗在 DEV-PLAN-021；Finance 为 Cost Center/Company 财务口径 SOR（冻结期间不改 schema）。
- **HRM 员工**：新增 `employee.OrgAssignments` 视图模型，表单需选中所属节点，默认 `effective_start = hire_date`；与 HRM 的回写/订阅规则按 SOR 边界文档执行。
- **Authz/Casbin**：组织节点作为 `object` 维度之一，`pkg/authz` 增加 `OrgScope` 属性用于 ABAC；授权策略生成/pack/test 随阶段推进。
- **Workflow**：若现有 `pkg/workflow` 未覆盖，将在本计划 M1 同步补齐最小审批引擎或复用外部服务；审批编排属后置集成包，不阻塞主数据上线。
- **Position/Compensation**：HRM Position/JobProfile/Comp 模块必须引用 Supervisory org，OrgAssignmentService 提供钩子确保职位移动时同步预算/薪酬计划。
- **Finance/Projects/Procurement**：Cost Center 与 Company 层级事件 `OrgCostCenterChanged` 触发总账/项目的成本归集；冻结期仅发布事件与只读视图，解除冻结后再落地消费/表结构改动并在 dev-plan 记录。
- **缓存**：树结构在 Redis/内存缓存，Key 含层级类型 + effective date（按日）。变更事件触发缓存失效。
- **Reporting/Analytics**：提供 `org_reporting` 视图供 BI 工具使用，支持任意时间点快照，与 Workday Custom Reporting 对齐。

## 里程碑
1. **M1（Phase 0/1）：MVP 主链**（2 周）
   - 单一 Supervisory 树 + OrgNode/OrgEdge/Assignment CRUD，`EffectiveWindow` 规则、无重叠/重名、租户隔离查询；Atlas 迁移/sqlc、ltree/btree_gist 扩展。
   - 输出 Workday 对齐表 v1，单元测试覆盖重叠检测、merge/split、冻结窗口；主数据接口规范（ID/Code/口径）。
2. **M2（Phase 2）：多层级与矩阵占位**（3 周）
   - 开放 Company/Cost/Custom 多树读写，矩阵/侧向链接仅只读占位，不影响主 Supervisory 约束；并行版本骨架。
   - API：`GET /org/hierarchies`、`PATCH /org/nodes` 基于多层级；`org_bp_bindings` 数据模型占位。
   - 补充 `config/access/policies/**` 片段并执行 `make authz-pack`、`make authz-test`，生成/校验 `policy.csv/.rev`，确保 OrgScope 权限链路纳入 CI。
3. **M3（Phase 3）：Lifecycle & BP/安全域绑定**（3 周）
   - 实现 change request 仓储、并行版本合并、BP route preview、security domain 映射；`POST/GET /org/change-requests`，`/simulate`，`/approve|reject|schedule|cancel`。
   - Retro API 定义与冲突策略，事件链 `OrgChanged`、`OrgAssignmentChanged`、`SecurityPolicyChanged`；审批/下游消费作为独立集成包，可与主数据解耦发布。
   - 并发/锁/幂等：同节点+时间窗口的请求序列化（应用锁/DB 锁），执行批次记录幂等 token，可重试失败批次；Draft/Review 预验证通过后才进入 Scheduled。
   - 版本推广：并行版本与审批模板版本绑定，支持 dev→staging→prod promotion/回滚审计。
4. **M4（Phase 4）：Assignments & Impact UI**（2 周）
   - 完成员工/职位/成本中心分配批量接口、Impact Simulation、树+时间轴 UI、Security Inspector、BP route 预览、多语言；发布口径说明（员工数、FTE、岗位数、成本额、审批链）。
   - 发布 Workday 场景（mass move、future-dated、retro correction）演示脚本。
5. **M5：优化、验证与培训**（1-2 周）
   - 性能测试（1k 节点、10k 员工，查询延迟 < 200ms）、缓存策略、BI 导出、文档/培训、Workday parity checklist 签署。
   - Retro/补偿演练：结账前/后 Retro 演练、事件重播和下游对账脚本；生效批次重试/跳过流程演练。

## 验收标准
- Phase 0/1：单树 CRUD + 有效期/去重名/无重叠 + 租户隔离查询性能达标（1k 节点 <200ms），审计和冻结窗口生效；主数据治理（编码/命名/必填/发布模式/SOR 边界）落地。
- Phase 2：多层级读写稳定，矩阵/侧向链接仅只读占位，不影响主链；并行版本骨架可保存对比。
- Phase 3：Lifecycle/Retro/BP/安全域链路闭环，可生成/合并并行版本并通过 `make authz-pack`/`make authz-test`；审批/下游消费可独立发版。
- Phase 4：Impact/UI 输出的口径明确（员工数、FTE、岗位数、成本额、审批链），与数据源/滞后性说明一致，典型 Workday 场景（Future-dated、Mass reorg、Retro、Parallel merge）可生成报告。
- HRM/Position/Finance 集成：员工/职位/成本中心分配更新与事件链打通；Finance 端在冻结期仅消费事件/视图，无侵入改动，解冻后按 SOR 规则回写。
- 文档完整：模块 README、API 参考、口径说明、操作手册、dev-record、Workday parity checklist。

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
