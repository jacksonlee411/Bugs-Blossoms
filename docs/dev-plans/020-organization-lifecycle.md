# DEV-PLAN-020：组织机构模块（对标 Workday）

**状态**: 已完成（2025-12-07 16:20）  
**评审结论**：M1 收敛为“单一 Organization Unit 树（原 Supervisory）+ 有效期校验 + 去重/无重叠 + 基础审计/查询性能”，暂不落地 workflow/BP 绑定、Authz 策略生成、并行版本、What-if/Impact UI 等高阶能力，统一挪到后续阶段或 backlog；M1 即交付最小权限集（Org.Read/Org.Write/Org.Assign/Org.Admin）及基础策略片段。

## 背景
- 现有 HRM 仅具备员工实体与基础表单，缺乏组织维度，导致薪酬、审批流与权限无法按照部门/成本中心划分。
- Workday 以“有效期管理 + 多层级组织 + 动态权限”著称，本计划以其核心能力为标杆，设计适配仓库 DDD 架构的组织模块。
- 模块需服务 HR、财务、采购、项目等多个领域，并支持“历史追溯 + 未来排程”的时间约束场景。

## 设计原则
1. **DDD 模块化**：新增 `modules/org`，严格遵守 AGENTS 规定的 domain/infrastructure/services/presentation 分层；禁止其他模块直接访问其内部实现，统一通过 service 接口或事件集成。
2. **有效期优先**：所有组织单元、层级关系、分配记录均采用“生效时间 / 失效时间”双字段，默认强制 `effective_date <= end_date` 并避免重叠。
3. **多层级模型**：同时支持 Workday 的 Supervisory、Company、Cost Center、Custom Reporting 四类层级，通过 `HierarchyType` 与 `NodeType` 组合实现。
4. **生命周期驱动（后续）**：组织单元的创建、重命名、合并、撤销等动作理想路径是“请求→审批→生效”，但 M1 仅直接 CRUD；审批/草稿/仿真视图待后续里程碑再启用。
5. **时间线 API**：所有读写接口必须显式接受 `effective_date`（对齐 Workday 的 `Effective Date` 查询点），未提供时默认 `time.Now()`，以保障历史查询与未来排程。
6. **可扩展事件流**：关键变更（新建部门、层级调整、员工调动、权限继承）通过 `pkg/eventbus` 发布，供 HRM/财务/审批模块订阅。
- **安全与最小权限**：M1 定义 `Org.Read`/`Org.Write`/`Org.Assign`/`Org.Admin`，接口默认要求 Session+租户校验与对应权限，配套策略片段纳入 `make authz-pack/test`。
- **命名约定**：Workday “Supervisory Organization” 在本项目统一称为 “Organization Unit”，字段/标签使用 “Org Unit”，`HierarchyType` 固定使用 `OrgUnit`；日期字段命名与 Workday 对齐：`effective_date`（开始），`end_date`/`inactive_date`（结束，半开区间）。
- **人员标识**：采用 `person_id` 作为自然人主键（不可变）；工号字段沿用 SAP 术语 `PERNR`，中文“工号”，同一租户下同一 person 不变。

- **必需语境**：本模块文档、接口、评审交流均默认中文。

## 目标
- **Phase 0/1（M1）**：交付单一 Organization Unit 树的 OrgNode/OrgEdge/Assignment CRUD，强制有效期/去重名/无重叠、租户隔离查询性能（1k 节点 200ms 内）、基础审计与冻结窗口。无审批流、无 BP 绑定、无并行版本、无策略生成，先提供稳定 SOR 与事件出口；Assignment 以 Position 为锚点（Person → Position → Org），支持一对一自动创建空壳 Position。
- **Phase 2（后续可选）**：在 M1 稳定后，再扩展多层级（Company/Cost/Custom）与矩阵占位，同时保持主 Organization Unit 约束；并发/锁、并行版本骨架可酌情进入。
- **Phase 3+（后续可选）**：Lifecycle/BP 绑定、Retro、更复杂冲突检测、Impact/What-if UI、安全域继承等高级特性，根据资源与需求再行立项。
- 组织/职位/岗位边界：覆盖组织层级与“Person → Position → Org” 隶属关系，不引入编制/空岗管理，编制控制在后续 DEV-PLAN-021 处理；空岗/多岗可在 Position 侧演进。
- 每阶段可独立上线，确保 HRM/Authz/Finance 等依赖至少获得稳定的组织主数据引用与事件，不因未完成高级特性而阻塞。

## 范围
- **M1 主数据最小集**：单一 Organization Unit 树，必备属性（code、name+i18n_names、parent、legal_entity_id/company_code、location_id、display_order、effective_date、end_date、tenant），强制去重名/无重叠/租户隔离性能；父子校验、基础审计、冻结窗口为硬约束；预留 change_requests/继承规则/角色表结构但可不启用业务。
- **时间维度**：节点/关系/分配的有效期字段与校验；只提供 `effective_date` 查询与基础 CRUD，不做 retro/并行版本。
- **树一致性**：每租户仅一棵 Organization Unit 树，唯一根节点；禁止环、禁止双亲、禁止孤儿，`OrgEdge` 为父子真相，`OrgNode.parent_hint` 由边反查并在写入时强校验一致。
- **主数据治理**：编码规则（唯一/长度/前缀）、命名规范、必填属性/字典校验、发布模式（API + 事件 + 批量导入）、冲突处理与审核责任写入文档；Org 为组织层级 SOR，Position 为人员隶属锚点（可自动生成空壳），编制留在 DEV-PLAN-021，Cost Center/Finance 仅消费事件/视图（冻结期不改 schema）。
- **跨 SOR 协议**：`person_id/pernr` 写层不建 FK，通过 HRM 只读视图或缓存做软校验并周期性对账；`position_id` 在 M1 必填（可自动生成），`assignment_type` 默认 primary（matrix/dotted 为占位），`org_level` 等字段可空占位。
- **后续可选（非 M1 交付）**：多层级/矩阵占位、workflow/BP 绑定、Authz 策略生成、并行版本、What-if/Impact UI 与安全域继承，待 M1 稳定后再立项。
- **非目标**：不实现薪酬预算、绩效考核，不做编制/空岗管理，不调整 finance 模块 schema，仅通过事件/视图消费。

## Workday 能力对齐
| Workday 关键点 | Workday 行为说明 | 本计划方案 | 差距/补充动作 |
| --- | --- | --- | --- |
| Organization Unit（原 Supervisory）/ Company / Cost / Custom Hierarchies | 每个层级有独立版本与有效期，驱动 BP、财务和报表 | **M1：仅单一 Organization Unit 树，无版本化**；M2+ 才开放多层级/占位 | 需在 M2 引入多树与版本冻结，允许 Draft/Active 并存 |
| Business Process（BP）绑定 | 审批路由基于 Supervisory、Company 及 security group | **M1 不做**，仅保留事件出口 | M3+ 才接入 `pkg/workflow` 与 `org_bp_bindings`、route preview |
| Security Domain / Group | Workday 通过 domain policy 授权到 org level，支持继承 | **M1 不做策略生成**，仅事件；Authz 继承放入后续 | M3+ 实现“组织节点 ↔ security group”映射与 policy 草稿 |
| Effective Dating & Retro Changes | 所有对象支持未来/过去生效，Retro 需影响历史审批/薪酬 | M1 支持 EffectiveWindow + 重叠/冻结校验，**无 retro** | Retro API/冲突策略/审计放入 M3+ |
| Matrix / Shared Line | 员工可有主、辅组织用于审批/报表 | **M1 仅主属，assignment_type 占位（matrix/dotted）不启用** | M2+ 才开放 lateral link/secondary 并定义权限提示 |
| Position Management | 职位必须挂载 Supervisory org，调动时影响 Budget/Comp | **M1：Assignment 以 Position 为锚点（Person → Position → Org），可自动生成一对一空壳 Position；无编制/空岗** | M2/021 再扩展编制、空岗、多岗与成本中心耦合 |
| Impact Analysis & What-if Simulation | 变更前展示受影响员工、BP、security | **M1 不含 Impact/What-if** | M4 才引入 Impact 面板与指标口径 |

## 关键业务蓝图
### 1. 组织结构与类型
- **M1**：仅交付单一 Organization Unit 树，字段集中在 code/name+i18n_names/parent/legal_entity_id/company_code/location_id/display_order/effective_date/end_date/tenant/审计。
- **M2+**：Company/Cost/Custom 层级与矩阵/侧向链接作为占位扩展，确保不破坏 Supervisory 约束。
- 层级由 `Hierarchy`/`OrgNode`/`OrgEdge` 表达，先保证基础查询性能，再考虑版本策略。
- 根节点仅允许一条，创建/变更需要管理员操作，根不可被设为子节点或删除，只能通过新根+迁移策略在后续里程碑处理。

### 2. 生命周期管理
- **M1**：直接 CRUD（含审计），无审批流、无并行版本；更新需通过有效期/重名/无重叠校验与冻结窗口检查；接口区分 Correct（更正历史，原位修改需更高权限与审计标记）与 Update（新增时间片截断旧片段）；支持 Rescind 状态用于撤销误创建（软删除+审计）；预留 change request 占位（草稿/提交），后续接入审批流。
- **后续（M3+）**：Draft/Review/Scheduled/Active/Retired、MassMove、并行版本、promotion/回滚、批次幂等、脏数据隔离等流程能力放入 backlog，待工作流能力成熟后再立项。

### 3. 时间约束策略（对齐 SAP/Workday）
- 默认模型：`OrgNode`、`OrgEdge`、`Assignment` 采用“约束 1”口径 —— 任意时点恰好一条记录，禁止重叠且无空档。写入时自动截断上一段，半开区间 `[effective_date, end_date)`，缺省 `end_date=9999-12-31`。
- 冻结窗口：对 Organization Unit 树应用配置化冻结窗口（默认月末+3 天，可按租户覆盖），冻结期仅允许未来日期变更。
- 历史/未来查询：接口统一接受 `effective_date` 作为查询点。Retro 重播与补偿不在 M1 范围，列为后续增强。
- SAP 约束对照：约束 1（无空档、无重叠）为本方案默认；A/2/3/B/T 仅作为参考，不在 Org 模块使用。
- Correction vs Update：API 设计需显式区分“更正历史”（Correct，如 `POST /nodes/{id}:correct`，原地修改当前切片，需更高权限与审计标记）与“新增时间片”（Update，如 `PATCH /nodes/{id}`，截断旧片段再写新段），避免误用。

### 4. 组织层级 & 权限
- **M1**：仅发布 `OrgChanged`/`OrgAssignmentChanged` 事件供 Authz/HRM 订阅，不做策略生成/继承计算。
- **后续（M3+）**：再评估 OrgSecurityDomain/Group 映射、继承计算与 policy 草稿出口，配合 `make authz-pack/test`。

## 技术方案
### Domain Layer
- `modules/org/domain/aggregates`:
  - `orgnode`：封装代码、i18n_names、parent_hint、legal_entity_id/company_code、location_id、display_order、有效期、状态（Active/Retired/Rescinded），提供基础行为（Create/Update/Rename/Move/Rescind），parent_hint 必须由 OrgEdge 反查校验；属性支持“显式值 + 继承解析”模式（解析值用于读，显式值用于写）。
  - 属性继承规则通过 `org_attribute_inheritance_rules` 配置（属性维度/层级类型/是否可覆盖/继承断点），解析结果可缓存。
  - `hierarchy`：仅管理单一 Organization Unit 树及约束（无版本化），维护唯一根、无环、无孤儿、不允许双亲。
  - `position`：最小 Position 实体，绑定 OrgNode 与时间窗；可由 Assignment 自动隐式创建，后续编制/空岗可在本实体扩展。
  - `assignment`：连接员工与 Position，Position 绑定 OrgNode；校验 primary 唯一、有效期无重叠。
  - `orgrole`（占位）：角色字典与角色分配（如 Manager/HRBP/Finance Controller），带有效期。
- 值对象：`EffectiveWindow`（effective_date/end_date + 校验）、`HierarchyType`、`NodeType`、`DisplayOrder`、`I18nName`、`AssignmentType`。
- 领域服务（M1）：`OrgTimeValidator`（重叠/冻结窗口校验，含 Correct/Update 分支）、`OrgAuditTrail`（审计事件生成，含 transaction_time/version）、`OrgInheritanceResolver`（属性继承解析只读视图与缓存，支持继承断点/覆盖）、`OrgChangeRequestDraft`（占位，处理草稿校验与提交预检）。
- 领域服务（后续）：`OrgLifecycleService`、`OrgSecurityService`、`OrgBusinessProcessAdapter` 等待后续里程碑再补。

### Infrastructure Layer
- 新建 schema `modules/org/infrastructure/persistence/schema/org-schema.sql`，核心表：
  - `org_nodes`（tenant_id, id, type, code, name, i18n_names jsonb, status, legal_entity_id/company_code, location_id, display_order int, effective_date, end_date, parent_hint, manager_user_id, created_at, updated_at）。
  - `org_edges`（tenant_id, id, hierarchy_id, parent_node_id, child_node_id, effective_date, end_date, depth, path ltree）。
  - `positions`（tenant_id, id, org_node_id, code, title, status, effective_date, end_date, is_auto_created bool, created_at, updated_at），M1 可自动为 Assignment 创建一对一空壳，通过 `is_auto_created` 标记以供后续治理。
  - `org_assignments`（tenant_id, id, position_id, subject_type=person, subject_id=person_id, pernr, assignment_type=primary|matrix|dotted, effective_date, end_date, primary bool）。
  - 占位/可选表：`org_attribute_inheritance_rules`（属性继承策略配置）、`org_roles`（角色字典）、`org_role_assignments`（角色分配，带有效期）、`change_requests`（草稿/提交/审批/生效占位，M1 可仅存草稿+审计字段）、`org_matrix_links`（矩阵/虚线组织关联）、`org_security_group_mappings`（组织节点与安全组关联）、`org_links`（组织与项目/成本中心/预算科目等多对多关联，带有效期）。
  - 其他表（retro/security/bp/version 等）不在 M1 创建，待后续里程碑再设计。
  - 附加索引：`gist (tenant_id, node_id, tstzrange(effective_date, end_date))` 用于时间冲突约束，`(tenant_id, parent_node_id, display_order)` 便于排序。
- 约束（M1 落地）：`org_nodes` 的 code 需在 tenant 内唯一；name/i18n_names 在同一父节点+时间窗口内唯一（按默认 locale）；`org_edges` 防环/双亲（ltree path + 唯一 child per hierarchy）；`positions` 需归属 OrgNode；`org_assignments` 对同一 subject 在重叠时间内仅允许一个 primary（部分唯一约束），position_id 必填，assignment_type 默认 primary。
- 多租户隔离：所有主键/唯一约束均以 `(tenant_id, <id/unique fields>)` 复合，外键与 sqlc 查询强制带 tenant 过滤；路径/缓存 key 同样纳入 tenant，避免跨租户穿透。
- Postgres 依赖：迁移启用 `ltree` 与 `btree_gist` 扩展；有效期字段统一 `tstzrange` 且使用 `[start, end)` 半开区间，写入/校验一律 UTC，迁移包含时区/边界说明；`EXCLUDE USING gist (tenant_id WITH =, node_id WITH =, tstzrange(effective_date, end_date) WITH &&)` 防重叠，重名用 `(tenant_id,parent_node_id,name,effective_date,end_date)` 唯一。
- sqlc 包：`modules/org/infrastructure/sqlc/...` 负责 CRUD + 冲突检测查询，包含 Position、继承解析只读视图、变更请求占位。
- 性能与冲突验证：itf/bench 覆盖 1k 节点查询 <200ms，重叠/重名写入被拒绝；在 CI 以基准或集成测试执行。
- 需要 Atlas/Goose 迁移流程，沿用 HRM 指南。
- 深层级读性能（M2 预留）：写侧保持 `OrgEdge` + ltree，禁止同步级联更新；读侧引入时态闭包表 `org_hierarchy_closure`（ancestor_id, descendant_id, depth, validity tstzrange, tenant_id，GiST/EXCLUDE）仅供查询；为常用时间片构建 `org_hierarchy_snapshots`/物化视图按 as_of_date/tenant 索引，Outbox 驱动 Job 刷新，热点查询禁止递归 CTE，优先走闭包表/快照；Manager/Leader 解析可在视图中冗余当前生效负责人以减递归。**M1 阶段应将所有层级遍历查询封装在 Repository 层，为 M2 无缝切换到闭包表实现做准备。**
- 审计/事务时间：主表保持单时态（effective_date/end_date + EXCLUDE），事务时间写入审计表或 Outbox 事件（recorded_at/operator/旧值/新值、transaction_time、version），供时光机/回放使用，不在主表叠加 tx_range；Rescind 操作以审计标记 + 软删除策略区分于 Retired。

### Service Layer
- **M1**：`OrgHierarchyService`（增删改查树 + 缓存失效）、`OrgPositionService`（Position 与 Org 绑定，支持自动创建一对一空壳 Position）、`OrgAssignmentService`（基于 Position 的员工分配 CRUD + 重叠校验）、`OrgEffectiveDateService`（effective_date 查询封装）。
- **后续**：`OrgLifecycleService`（change request 审批流）、`OrgRetroService`、`OrgSecurityService`、`OrgBusinessProcessAdapter` 在 M3+ 再加入。

### Presentation Layer & API
- Controller 前缀 `/org`：
  - `GET /org/hierarchies?type=OrgUnit&effective_date=`：返回树概览，支持 display_order 排序，属性提供显式值与继承解析值。
  - `POST /org/nodes` / `PATCH /org/nodes/{id}`：节点 CRUD（含有效期、重名、父子校验）；支持字段 code/name/i18n_names/legal_entity_id/company_code/location_id/display_order；删除改为 `PATCH` 设置 `end_date` 或状态 `Retired`，误创建支持 Rescind；预留 change request 草稿/提交接口（无审批流，仅存档）。
  - `POST /org/positions`（可选）/ `POST /org/assignments`：Assignment 以 Position 为锚点；若未显式传 position_id，默认创建一对一 Position 再绑定；支持 `assignment_type`（primary/matrix/dotted）；`PATCH /org/assignments/{id}` / `GET /org/assignments?subject=person:{id}`：人员分配时间线。
  - `POST /org/role-assignments`（占位）/ `GET /org/role-assignments?role=HRBP`：组织角色分配查询。
  - API 设计区分“修正”与“更新”：`PATCH /org/nodes/{id}` 用于常规更新（创建新时间片），`POST /org/nodes/{id}:correct` 用于修正历史数据（原地修改），后者需要更高权限（如 `Org.Admin`）。
  - 不提供 retro/security/BP/Impact 相关接口于 M1。
- UI（templ）：
  - M1：树形视图 + 基础表单（节点、Position、分配），日期选择器切换有效期，支持多语言名称编辑、display_order 调整。
  - 拖拽式 Change Request Builder / Impact / Security Inspector / route preview 留作 M4+ backlog。
- 权限要求：`/org/**` 需 Session+租户校验与 `Org.Read`/`Org.Write`/`Org.Assign`/`Org.Admin`，无策略生成前可用特性开关仅对管理员开放。

## 集成与依赖
- **SOR 边界**：Org 模块为组织层级 SOR；Position 为人员隶属锚点（可自动生成空壳），编制/空岗在 DEV-PLAN-021；HRM 为人 SOR；Finance 为 Cost Center/Company 财务口径 SOR（冻结期间不改 schema）。
- **HRM 员工**：提供 `OrgAssignments` 视图和分配 API，表单默认 `effective_date = hire_date`，按 SOR 边界执行回写/订阅；主体标识使用 `person_id`（不可变）+ `pernr`（工号，租户内唯一且不变），position_id 必填（可由系统自动生成）。
- **Authz/Casbin**：M1 仅发布事件；事件 payload 预留 `tenant_id/org_id/hierarchy_type/node_type/person_id/pernr/position_id/effective_date/end_date/assignment_type` 和 `version/timestamp`，供后续 `OrgScope`/ABAC 计算。policy pack/test 放入后续里程碑。
- **Workflow**：M1 不引入审批引擎，事件中预留变更上下文字段（如 `change_type`, `initiator_id`）便于后续 route preview/绑定；change request 占位表可提前写入草稿。
- **Position/Compensation**：M1 面向人员，Assignment 必须落在 Position 上，若未提供 position_id 则自动生成空壳 Position；编制/岗位/成本中心钩子留待 DEV-PLAN-021/M3+。
- **Finance/Projects/Procurement**：冻结期仅消费事件与只读视图，不改 finance 相关 schema；解冻后若有需求在 dev-plan 记录。
- **缓存**：树结构在 Redis/内存缓存，Key 含层级类型 + effective date（按日）。变更事件触发缓存失效。
- **Reporting/Analytics**：提供 `org_reporting` 视图供 BI 工具使用，支持任意时间点快照，与 Workday Custom Reporting 对齐；后续补组织图导出、路径查询、人员路径查询接口。
- **事件契约**：OrgChanged/OrgAssignmentChanged 附带 tenant_id/node_id/effective_window/version/assignment_type/幂等键，向后兼容扩展字段；预留继承解析后的属性、变更请求上下文字段。
- **跨模块校验**：person/pernr 通过 HRM 只读视图或缓存软校验并周期性对账；position_id 必填且归属 OrgNode，HRM Position SOR 成熟后再启用更强校验。

## 上线与迁移
- 租户初始化：导入脚本（CSV/JSON）创建唯一根节点、批量导入节点/边、补齐员工 primary assignment；导入前执行重叠/重名校验，导入后输出对账报告并记入 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md`。
- 灰度与回滚：按租户/环境开关只读接口，写接口仅对导入账号开放；导入前对 org 相关表快照（pg_dump），提供清理脚本与缓存重建。
- 性能基准：itf/bench 生成 1k 节点树验证查询耗时、重叠写入拒绝；挂入 CI 执行。

## 里程碑（按现阶段资源和风险重新规划）
1. **阶段 0：基线与占位**
   - 建表与约束：OrgNode/Edge/Position/Assignment（含 assignment_type 占位）、继承规则表、角色表、change_requests 草稿占位，事件契约补充 assignment_type/继承属性。
   - Readiness：`make check lint`、相关路径 `go test`，导入/回滚脚本雏形。
2. **阶段 1（M1 可交付）：最小可用主链**
   - Person→Position→Org 单树 CRUD，有效期/重名/无重叠/防环、事务时间审计、Rescind/Correct/Update 区分，Position 自动创建路径，1k 节点 <200ms 查询基线。
   - API：节点/Position/Assignment CRUD，assignment_type 默认 primary；事件发布；导入/灰度最小脚本。
3. **阶段 2：继承与矩阵读侧、角色占位**
   - 落地属性继承解析与缓存（规则表生效），角色分配与查询占位；矩阵/虚线 assignment_type 可读不可写（或特性开关）。
   - 深层读性能：启用时态闭包表、as_of 快照/物化视图雏形，禁用递归 CTE 热点查询。
4. **阶段 3：变更请求 & 数据质量**
   - change_requests 流程雏形（草稿/提交，审批占位）、Pre-flight 影响预检接口；数据质量规则与报告（编码正则、必填/叶子需岗位等），批量修复脚本雏形。
   - org_matrix_links 占位表可写（特性开关），角色/继承规则管理接口。
5. **阶段 4：权限映射与业务关联**
   - org_security_group_mappings 读写占位（仅预览，不生成策略），权限预览接口；org_links 关联项目/成本中心/预算科目可写占位。
   - 事件补充安全组/关联对象上下文，Authz 订阅方对接。
6. **阶段 5：可视化与路径/报告**
   - 组织图导出（JSON/SVG/PNG）、节点路径/人员路径查询；Reporting 视图拍平路径和继承属性；矩阵/虚线查询可用。
   - 性能基线扩展至 10k+ 节点时间片查询与快照刷新。
7. **阶段 6：运维与治理 / 可选 SOM 对齐**
   - 监控与指标（查询/写入耗时、冲突计数、缓存命中、事件延迟、快照刷新）、健康检查；压测脚本常驻、导入/回滚自动化。
   - 视情况评估 SOM/标准对象框架对齐与并行版本全面化。

## 实施步骤规划
本节将计划中的里程碑分解为更具体、可执行的步骤，以指导开发团队的落地执行。

### **阶段 0：基线与占位 (地基搭建)**
此阶段的目标是完成所有数据库表结构、约束和基本脚本的准备工作，为后续功能开发打下坚实基础。

*   **步骤 1: 创建数据库 Schema 与约束**
    *   **任务**: 根据计划，创建 `org_nodes`, `org_edges`, `positions`, `org_assignments` 等核心表。
    *   **关键**: 必须在数据库层面严格实现约束，例如使用 `EXCLUDE USING gist` 防止时间段重叠，使用 `ltree` 防止层级关系成环，以及确保 `code` 和 `name` 在指定范围内的唯一性。
    *   **产出**: 可通过 Atlas/Goose 执行的数据库迁移脚本，迁移生成后必须跑 `make db lint`（含 `atlas migrate lint`）与一次 `make db migrate up HRM_MIGRATIONS=1`/`make db migrate down HRM_MIGRATIONS=1` 验证可回滚，并记录命令与结果。

*   **步骤 2: 建立占位表与事件契约**
    *   **任务**: 创建 `org_attribute_inheritance_rules`, `org_roles`, `change_requests` 等占位表，即使 M1 阶段不实现其业务逻辑，也要确保表结构预留到位。
    *   **任务**: 定义 `OrgChanged` 和 `OrgAssignmentChanged` 事件的详细数据结构（契约），包含 `assignment_type`、继承属性等未来扩展字段。
    *   **验证**: 若涉及 sqlc/atlas 生成，执行 `make sqlc-generate`/`make generate` 后确保 `git status --short` 干净。

*   **步骤 3: 准备基础脚本与验证**
    *   **任务**: 编写初步的数据导入/导出脚本和回滚脚本的雏形。
    *   **任务**: 确保项目通过 `make check lint`、`go test ./modules/org/...`（或相关路径），必要时补充 bench/seed 脚本的空壳以便后续填充；验证失败时必须提供回滚/清理脚本。

### **阶段 1 (M1 里程碑): 最小可用主数据链**
此阶段的目标是交付一个功能完备但最小化的组织主数据核心，让下游模块（如 HRM、Authz）可以开始消费数据。

*   **步骤 4: 实现核心 CRUD 功能**
    *   **任务**: 开发“人员(Person) → 职位(Position) → 组织(Org)”这一主数据链的增删改查（CRUD）功能。
    *   **关键**: 实现“当分配人员时若未提供职位，系统自动创建一对一的空壳职位”的逻辑；所有读写必须强制 Session+租户隔离（无租户/无 Session 直接拒绝）。

*   **步骤 5: 落地时间约束与审计**
    *   **任务**: 在所有写操作中强制执行有效期校验（无重叠、无空档、防循环），并实现基于事务时间的审计日志。
    *   **任务**: 在 API 和服务层明确区分 `Correct` (修正历史) 和 `Update` (创建新时间片) 两种操作。
    *   **任务**: 实现 `Rescind` (撤销) 状态，用于软删除误创建的数据；冻结窗口（默认月末+3 天，可按租户覆盖）违反时拒绝写入并记录审计。

*   **步骤 6: 开发 API 与发布事件**
    *   **任务**: 提供节点、职位、分配的 RESTful API。
    *   **任务**: 在数据变更成功后，通过事件总线发布 `OrgChanged` 和 `OrgAssignmentChanged` 事件；所有入口统一使用 `pkg/authz` 判定 `Org.Read/Org.Write/Org.Assign/Org.Admin`，提交 `config/access/policies/org/**` 片段并运行 `make authz-test authz-lint authz-pack` 作为准入。

*   **步骤 7: 性能与上线准备**
    *   **任务**: 完成性能基准测试，确保“1000个节点，查询时间小于200毫秒”的指标达成（基准脚本需固定数据集、PG17 环境、命令行参数，纳入 repo/CI 可重复执行）。
    *   **任务**: 完善数据导入和灰度发布脚本，准备上线；若性能不达标，提供特性开关/降级查询与回滚剧本。

### **阶段 2：继承、矩阵与角色占位**
此阶段在 M1 稳定后启动，重点是增强读取和查询能力。

*   **步骤 8: 实现属性继承与角色查询**
    *   **任务**: 基于 `org_attribute_inheritance_rules` 表，实现组织属性的继承解析逻辑和缓存。
    *   **任务**: 提供角色分配和查询的占位接口。
    *   **任务**: 允许读取（但不可写入）矩阵/虚线汇报关系。

*   **步骤 9: 优化深层级读取性能**
    *   **任务**: 根据计划，引入时态闭包表或物化视图，优化涉及全树或深层级的查询，禁止在热点查询中使用递归；上线前需提供闭包表迁移/回填脚本、幂等刷新任务与 feature flag/回滚策略，确保读路径可平滑切换。

### **阶段 3：变更请求与数据质量**
此阶段开始引入流程管理的雏形。

*   **步骤 10: 开发变更请求流程**
    *   **任务**: 实现 `change_requests` 表的草稿和提交功能，审批部分可暂时占位（workflow 模块未启用时仅存草稿+审计，不触发路由）。
    *   **任务**: 提供一个预检（Pre-flight）API，用于在变更前分析可能的影响，并要求权限校验/审计；相关接口需在 `go test ./modules/org/...` 中覆盖无权限/有权限/租户隔离路径。

*   **步骤 11: 强化数据质量**
    *   **任务**: 开发数据质量规则和报告（如编码格式、必填项检查等）。
    *   **任务**: 编写批量修复数据的脚本，并提供 dry-run/回滚验证，提交前需通过 lint/test。

### **阶段 4 及以后：高级功能迭代**
这些阶段属于远期规划，将在核心功能完全稳定后，根据业务优先级逐一实现。

*   **步骤 12: 权限映射与业务关联**
    *   **任务**: 实现组织节点与安全组的映射、权限预览接口、组织与其他业务对象（如项目、成本中心）的关联；涉及 Finance 冻结目录的变更必须先在 dev-plan 解除冻结声明并更新 AGENTS，否则不得触碰 finance 侧 schema/代码。

*   **步骤 13: 可视化与高级报告**
    *   **任务**: 实现组织图导出、节点路径查询、人员路径查询等高级报表功能。

*   **步骤 14: 运维、治理与可选的 SOM 对齐**
    *   **任务**: 建立完善的监控指标、健康检查、自动化压测和运维脚本。

## 验收标准
- Phase 0/1：单树 CRUD + 有效期/去重名/无重叠 + 租户隔离查询性能达标（1k 节点 <200ms），审计和冻结窗口生效；主数据治理（编码/命名/必填/发布模式/SOR 边界）落地。
- Phase 2：多层级/矩阵占位可查询，不影响主链；若上线，需保持无跨租户穿透、无额外策略耦合。
- Phase 3+：仅在立项后评估，需明确依赖（workflow/authz），并通过 `make authz-pack/test` 等质量门禁再交付。
- HRM/Position/Finance 集成：M1 仅事件/视图消费，不改 finance 冻结模块；后续若回写需在 dev-plan 记录。
- 文档完整：模块 README、API 参考、口径说明、操作手册、dev-record、Workday parity checklist。
- 权限/策略：M1 提供权限常量与基础策略片段，`make authz-pack/test` 通过；无策略生成前，接口需经权限校验或特性开关保护。

## 风险与缓解
- **时间区间复杂度**：有效期重叠校验可能拖慢写入 → 结合 `EXCLUDE` 约束 + 应用侧批量校验，提前压测 1k 节点场景。
- **数据迁移**：需要从现有 Excel/静态结构导入 → 提供导入校验报告，按租户灰度导入并可回滚。
- **性能/缓存一致性**：树缓存与事件失效可能不一致 → 定义幂等失效策略，提供全量重建脚本。
- **范围回弹**：高级特性（workflow/Authz/Impact）被提前挤入 M1 → 通过文档声明与里程碑约束，严格 CR 控制。
- **跨模块耦合**：若 HRM/Finance 期待同步 schema 变更 → 明确 SOR 边界，仅发布事件/视图，避免修改冻结模块。

## 未来扩展点（M2+ 清单）
- 变更请求审批流：基于 `change_requests` 启用 Draft/Submit/Approve/Schedule/Activate，全链路审计与回滚。
- 变更影响预检：提供 Pre-flight API，输出受影响员工/岗位/权限/事件列表。
- 组织图与路径：导出组织图（PNG/SVG/JSON）、节点间路径查询、人员所在路径查询。
- 矩阵/虚线：完善 `assignment_type` + `org_matrix_links`，路由/权限识别矩阵关系。
- 权限映射：`org_security_group_mappings` + 继承标记，提供只读权限预览。
- 业务对象关联：`org_links` 关联项目/成本中心/预算科目等（带有效期）。
- 数据质量：规则配置（编码正则、必填约束、叶子节点需岗位等）、质量报告、批量修复工具。
- 地理/国际化：地理层级、时区/货币/语言属性强化，location 维度筛选。
- 监控与运维：关键操作审计、查询/写入耗时、冲突计数、缓存命中、事件延迟的指标与健康检查接口。

## 术语映射表（Workday/主流 HR ↔ SAP HCM ↔ 本项目）
| Workday 术语 | 主流 HR 习惯 | SAP HCM 字段名 | 本项目字段名 | 说明 |
| --- | --- | --- | --- | --- |
| Supervisory Organization | 部门/主管线 | ORGEH（Org Unit），关系 A/B002 | 术语：Organization Unit（字段/标签：Org Unit）；`hierarchy_type=OrgUnit`，`node_type=OrgUnit` | M1 仅单树。 |
| Company Organization | 法人/公司 | BUKRS（Company Code） | `hierarchy_type=Company`（M2+ 预留）；M1 在 OrgNode 上有 `legal_entity_id/company_code` 属性 | M1 先用属性挂载发薪主体，M2+ 才建独立层级。 |
| Cost Center | 成本中心 | KOSTL（Cost Center） | `hierarchy_type=CostCenter`（M2+ 预留） | finance SOR，仅消费事件/视图。 |
| Custom Organization | 自定义报表分组 | 自定义评估路径/对象类型（例如 O/Z*） | `hierarchy_type=Custom`（M2+ 预留） | 用于报表分组，占位。 |
| Org ID/Code | 组织编码 | ORGEH / OBJID | `org_nodes.code` | 租户内唯一。 |
| Org Name | 组织名称 | STEXT（短名） | `org_nodes.name` + `i18n_names` | 父节点+时间窗口内唯一，支持多语言。 |
| Parent Org | 父子关系 | 关系 A/B002（O→O 上级） | `org_edges.parent_node_id` / `child_node_id` | `org_nodes.parent_hint` 仅缓存，建议改名或隐藏。 |
| Manager/Supervisor | 组织负责人 | Chief Position 标记（S-CHIEF，关系 A/B012） | `manager_user_id` | M1 简化实现；长远应通过 `org_role_assignments` 表实现带有效期的角色分配。 |
| Effective Date / End Date（Inactive Date） | 生效/失效日期 | BEGDA / ENDDA | `effective_date` / `end_date` (`tstzrange` 半开) | 默认失效为开区间 `9999-12-31`。 |
| As-of Date | 查询时点（Key Date） | Key Date（Stichtag） | `effective_date` 参数 | 未传则默认 `time.Now()`。 |
| Primary Supervisory Org | 主属组织 | PA0001-ORGEH（主组织） | `org_assignments.primary` + `assignment_type=primary` | M1 支持主属；矩阵/虚线用 assignment_type=matrix/dotted 占位。 |
| Worker（本项目术语 Person，工号 PERNR） | 员工/雇员 | PERNR | `subject_type=person` + `subject_id=person_id`（工号 `pernr` 不变） | 职位信息单独用 `position_id` 承载，不改主体标识。 |
| Position | 职位 | PLANS（Position） | `positions.id`，Assignment 必须 Person → Position → Org | M1 支持自动生成一对一空壳 Position；编制/空岗在 DEV-PLAN-021/M2+ 扩展。 |
| Effective Status | 状态 | OBJSTAT（对象状态）/ STAT2（雇佣状态） | `status=Active/Retired` | 如需停用态可扩展 `Inactive`。 |
| Org Level | 组织层级 | OTYPE+层级自定义（如 O 等级自定义字段） | （未落地）`org_level` 占位 | 便于报表/BP 路由。 |
| Org Roles (HR Partner 等) | HR 伙伴/业务负责人 | 关系 1001 A/B003（责任人）等 | （未落地）`hr_partner_user_id` 等 | 可在后续里程碑补充。 |

## 后续动作
- 记录本计划在 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 的 PoC/联调日志。
- 与产品/HR BP 对齐 Workday 关键流程（Supervisory、Matrix、Effective Dating）。
- 建立 Workday parity checklist（含 BP、安全域、Retro、Position 管理）并在每个里程碑更新。
- 准备下一阶段（DEV-PLAN-021）聚焦“组织预算与人员编制”。
