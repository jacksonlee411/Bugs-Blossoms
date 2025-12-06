# DEV-PLAN-020：组织机构模块（对标 Workday）

**状态**: 草拟中（2025-01-15 11:30）  
**评审结论**：M1 收敛为“单一 Organization Unit 树（原 Supervisory）+ 有效期校验 + 去重/无重叠 + 基础审计/查询性能”，暂不落地 workflow/BP 绑定、Authz 策略生成、并行版本、What-if/Impact UI 等高阶能力，统一挪到后续阶段或 backlog。

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
- **命名约定**：Workday “Supervisory Organization” 在本项目统一称为 “Organization Unit”，字段/标签使用 “Org Unit”，`HierarchyType` 固定使用 `OrgUnit`；日期字段命名与 Workday 对齐：`effective_date`（开始），`end_date`/`inactive_date`（结束，半开区间）。
- **人员标识**：采用 `person_id` 作为自然人主键（不可变）；工号字段沿用 SAP 术语 `PERNR`，中文“工号”，同一租户下同一 person 不变。

## 目标
- **Phase 0/1（M1）**：仅交付单一 Organization Unit 树的 OrgNode/OrgEdge/Assignment CRUD，强制有效期/去重名/无重叠、租户隔离查询性能（1k 节点 200ms 内）、基础审计与冻结窗口。无审批流、无 BP 绑定、无并行版本、无策略生成，先提供稳定 SOR 与事件出口。
- **Phase 2（后续可选）**：在 M1 稳定后，再扩展多层级（Company/Cost/Custom）与矩阵占位，同时保持主 Organization Unit 约束；并发/锁、并行版本骨架可酌情进入。
- **Phase 3+（后续可选）**：Lifecycle/BP 绑定、Retro、更复杂冲突检测、Impact/What-if UI、安全域继承等高级特性，根据资源与需求再行立项。
- 组织/职位/岗位边界：本计划仅覆盖组织层级与“人/职位/成本中心隶属”关系，不引入编制/空岗管理，Position 编制控制在后续 DEV-PLAN-021 处理。
- 每阶段可独立上线，确保 HRM/Authz/Finance 等依赖至少获得稳定的组织主数据引用与事件，不因未完成高级特性而阻塞。

## 范围
- **M1 主数据最小集**：单一 Organization Unit 树，必备属性（code、name、parent、effective_date、end_date、tenant），强制去重名/无重叠/租户隔离性能；父子校验、基础审计、冻结窗口为硬约束。
- **时间维度**：节点/关系/分配的有效期字段与校验；只提供 `effective_date` 查询与基础 CRUD，不做 retro/并行版本。
- **树一致性**：每租户仅一棵 Organization Unit 树，唯一根节点；禁止环、禁止双亲、禁止孤儿，`OrgEdge` 为父子真相，`OrgNode.parent_hint` 由边反查并在写入时强校验一致。
- **主数据治理**：编码规则（唯一/长度/前缀）、命名规范、必填属性/字典校验、发布模式（API + 事件 + 批量导入）、冲突处理与审核责任写入文档；Org 为组织层级 SOR，Position/编制留在后续 DEV-PLAN-021，Cost Center/Finance 仅消费事件/视图（冻结期不改 schema）。
- **后续可选（非 M1 交付）**：多层级/矩阵占位、workflow/BP 绑定、Authz 策略生成、并行版本、What-if/Impact UI 与安全域继承，待 M1 稳定后再立项。
- **非目标**：不实现薪酬预算、绩效考核，不做编制/空岗管理，不调整 finance 模块 schema，仅通过事件/视图消费。

## Workday 能力对齐
| Workday 关键点 | Workday 行为说明 | 本计划方案 | 差距/补充动作 |
| --- | --- | --- | --- |
| Organization Unit（原 Supervisory）/ Company / Cost / Custom Hierarchies | 每个层级有独立版本与有效期，驱动 BP、财务和报表 | **M1：仅单一 Organization Unit 树，无版本化**；M2+ 才开放多层级/占位 | 需在 M2 引入多树与版本冻结，允许 Draft/Active 并存 |
| Business Process（BP）绑定 | 审批路由基于 Supervisory、Company 及 security group | **M1 不做**，仅保留事件出口 | M3+ 才接入 `pkg/workflow` 与 `org_bp_bindings`、route preview |
| Security Domain / Group | Workday 通过 domain policy 授权到 org level，支持继承 | **M1 不做策略生成**，仅事件；Authz 继承放入后续 | M3+ 实现“组织节点 ↔ security group”映射与 policy 草稿 |
| Effective Dating & Retro Changes | 所有对象支持未来/过去生效，Retro 需影响历史审批/薪酬 | M1 支持 EffectiveWindow + 重叠/冻结校验，**无 retro** | Retro API/冲突策略/审计放入 M3+ |
| Matrix / Shared Line | 员工可有主、辅组织用于审批/报表 | **M1 不含矩阵**，仅 primary | M2+ 才开放 lateral link/secondary 并定义权限提示 |
| Position Management | 职位必须挂载 Supervisory org，调动时影响 Budget/Comp | M1 仅覆盖 employee assignment；position/cost center 延后 | M3 与 HRM Position 实体打通后再补验证 |
| Impact Analysis & What-if Simulation | 变更前展示受影响员工、BP、security | **M1 不含 Impact/What-if** | M4 才引入 Impact 面板与指标口径 |

## 关键业务蓝图
### 1. 组织结构与类型
- **M1**：仅交付单一 Organization Unit 树，字段集中在 code/name/parent/effective_date/end_date/tenant/审计。
- **M2+**：Company/Cost/Custom 层级与矩阵/侧向链接作为占位扩展，确保不破坏 Supervisory 约束。
- 层级由 `Hierarchy`/`OrgNode`/`OrgEdge` 表达，先保证基础查询性能，再考虑版本策略。
- 根节点仅允许一条，创建/变更需要管理员操作，根不可被设为子节点或删除，只能通过新根+迁移策略在后续里程碑处理。

### 2. 生命周期管理
- **M1**：直接 CRUD（含审计），无审批流、无并行版本；更新需通过有效期/重名/无重叠校验与冻结窗口检查。
- **后续（M3+）**：Draft/Review/Scheduled/Active/Retired、MassMove、并行版本、promotion/回滚、批次幂等、脏数据隔离等流程能力放入 backlog，待工作流能力成熟后再立项。

### 3. 时间约束策略（对齐 SAP/Workday）
- 默认模型：`OrgNode`、`OrgEdge`、`Assignment` 采用“约束 1”口径 —— 任意时点恰好一条记录，禁止重叠且无空档。写入时自动截断上一段，半开区间 `[effective_date, end_date)`，缺省 `end_date=9999-12-31`。
- 冻结窗口：对 Organization Unit 树应用配置化冻结窗口（默认月末+3 天，可按租户覆盖），冻结期仅允许未来日期变更。
- 历史/未来查询：接口统一接受 `effective_date` 作为查询点。Retro 重播与补偿不在 M1 范围，列为后续增强。
- SAP 约束对照：约束 1（无空档、无重叠）为本方案默认；A/2/3/B/T 仅作为参考，不在 Org 模块使用。

### 4. 组织层级 & 权限
- **M1**：仅发布 `OrgChanged`/`OrgAssignmentChanged` 事件供 Authz/HRM 订阅，不做策略生成/继承计算。
- **后续（M3+）**：再评估 OrgSecurityDomain/Group 映射、继承计算与 policy 草稿出口，配合 `make authz-pack/test`。

## 技术方案
### Domain Layer
- `modules/org/domain/aggregates`:
  - `orgnode`：封装名称、代码、parent_hint、有效期、状态（Active/Retired），提供基础行为（Create/Update/Rename/Move），parent_hint 必须由 OrgEdge 反查校验。
  - `hierarchy`：仅管理单一 Organization Unit 树及约束（无版本化），维护唯一根、无环、无孤儿、不允许双亲。
  - `assignment`：连接员工与组织节点，校验 primary 唯一、有效期无重叠。
- 值对象：`EffectiveWindow`（effective_date/end_date + 校验）、`HierarchyType`、`NodeType`。
- 领域服务（M1）：`OrgTimeValidator`（重叠/冻结窗口校验）、`OrgAuditTrail`（审计事件生成）。
- 领域服务（后续）：`OrgLifecycleService`、`OrgSecurityService`、`OrgBusinessProcessAdapter` 等待后续里程碑再补。

### Infrastructure Layer
- 新建 schema `modules/org/infrastructure/persistence/schema/org-schema.sql`，核心表：
  - `org_nodes`（tenant_id, id, type, code, name, status, effective_date, end_date, parent_hint, owner_user_id, created_at, updated_at）。
  - `org_edges`（tenant_id, id, hierarchy_id, parent_node_id, child_node_id, effective_date, end_date, depth, path ltree）。
  - `org_assignments`（tenant_id, id, node_id, subject_type=person, subject_id=person_id, pernr, effective_date, end_date, primary bool）。
  - 其他表（change_requests、retro/security/bp/version 等）不在 M1 创建，待后续里程碑再设计。
  - 附加索引：`gist (tenant_id, node_id, tstzrange(effective_date, end_date))` 用于时间冲突约束。
- 约束（M1 落地）：`org_nodes` 的 code 需在 tenant 内唯一；name 在同一父节点+时间窗口内唯一；`org_edges` 需防环/双亲（ltree path + 唯一 child per hierarchy）；`org_assignments` 对同一 subject 在重叠时间内仅允许一个 primary（部分唯一约束）。
- 多租户隔离：所有主键/唯一约束均以 `(tenant_id, <id/unique fields>)` 复合，外键与 sqlc 查询强制带 tenant 过滤；路径/缓存 key 同样纳入 tenant，避免跨租户穿透。
- Postgres 依赖：迁移启用 `ltree` 与 `btree_gist` 扩展；有效期字段统一 `tstzrange` 且使用 `[start, end)` 半开区间，写入/校验一律 UTC，迁移包含时区/边界说明。
- sqlc 包：`modules/org/infrastructure/sqlc/...` 负责 CRUD + 冲突检测查询。
- 需要 Atlas/Goose 迁移流程，沿用 HRM 指南。

### Service Layer
- **M1**：`OrgHierarchyService`（增删改查树 + 缓存失效）、`OrgAssignmentService`（员工分配 CRUD + 重叠校验）、`OrgEffectiveDateService`（effective_date 查询封装）。
- **后续**：`OrgLifecycleService`（change request）、`OrgRetroService`、`OrgSecurityService`、`OrgBusinessProcessAdapter` 在 M3+ 再加入。

### Presentation Layer & API
- Controller 前缀 `/org`：
  - `GET /org/hierarchies?type=OrgUnit&effective_date=`：返回树概览。
  - `POST /org/nodes` / `PATCH /org/nodes/{id}`：节点 CRUD（含有效期、重名、父子校验）；删除改为 `PATCH` 设置 `end_date` 或状态 `Retired`，防止破坏历史。
  - `POST /org/assignments` / `PATCH /org/assignments/{id}` / `GET /org/assignments?subject=person:{id}`：人员分配时间线。
  - 不提供 change-requests/retro/security/BP/Impact 相关接口于 M1。
- UI（templ）：
  - M1：树形视图 + 基础表单（节点、分配），日期选择器切换有效期。
  - 拖拽式 Change Request Builder / Impact / Security Inspector / route preview 留作 M4+ backlog。

## 集成与依赖
- **SOR 边界**：Org 模块为组织层级 SOR；HRM/Position 为人/岗位 SOR，Position 编制/空岗在 DEV-PLAN-021；Finance 为 Cost Center/Company 财务口径 SOR（冻结期间不改 schema）。
- **HRM 员工**：提供 `OrgAssignments` 视图和分配 API，表单默认 `effective_date = hire_date`，按 SOR 边界执行回写/订阅；主体标识使用 `person_id`（不可变）+ `pernr`（工号，租户内唯一且不变）。
- **Authz/Casbin**：M1 仅发布事件；`OrgScope` ABAC 属性与 policy pack/test 放入后续里程碑。
- **Workflow**：M1 不引入审批引擎，后续若需要再复用 `pkg/workflow`。
- **Position/Compensation**：M1 仅面向员工，职位/成本中心钩子留待 DEV-PLAN-021/M3+。
- **Finance/Projects/Procurement**：冻结期仅消费事件与只读视图，不改 finance 相关 schema；解冻后若有需求在 dev-plan 记录。
- **缓存**：树结构在 Redis/内存缓存，Key 含层级类型 + effective date（按日）。变更事件触发缓存失效。
- **Reporting/Analytics**：提供 `org_reporting` 视图供 BI 工具使用，支持任意时间点快照，与 Workday Custom Reporting 对齐。

## 里程碑
1. **M1（Phase 0/1）：最小可用主链**
   - 单一 Organization Unit 树 + OrgNode/OrgEdge/Assignment CRUD，`EffectiveWindow` 规则、无重叠/重名、租户隔离查询；Atlas 迁移/sqlc、ltree/btree_gist 扩展与基础审计。
   - 性能基线（1k 节点 <200ms 查询）、冻结窗口规则、API/事件规范；测试覆盖重叠检测、父子校验、冻结窗口。
2. **M2（Phase 2，可选）：多层级与矩阵占位**
   - 开放 Company/Cost/Custom 多树只读/占位能力，保持主 Organization Unit 约束；可选 Matrix/Lateral link 占位。
   - 评估是否需要并行版本骨架与 Authz 事件属性扩展，仍不生成策略。
3. **M3+（后续 backlog）**：
   - Lifecycle/BP 绑定、并行版本、Retro、更复杂冲突检测。
   - Authz policy 生成、Security Domain/Group 继承、Impact/What-if UI、route preview、批量 MassMove 脚本。

## 验收标准
- Phase 0/1：单树 CRUD + 有效期/去重名/无重叠 + 租户隔离查询性能达标（1k 节点 <200ms），审计和冻结窗口生效；主数据治理（编码/命名/必填/发布模式/SOR 边界）落地。
- Phase 2：多层级/矩阵占位可查询，不影响主链；若上线，需保持无跨租户穿透、无额外策略耦合。
- Phase 3+：仅在立项后评估，需明确依赖（workflow/authz），并通过 `make authz-pack/test` 等质量门禁再交付。
- HRM/Position/Finance 集成：M1 仅事件/视图消费，不改 finance 冻结模块；后续若回写需在 dev-plan 记录。
- 文档完整：模块 README、API 参考、口径说明、操作手册、dev-record、Workday parity checklist。

## 风险与缓解
- **时间区间复杂度**：有效期重叠校验可能拖慢写入 → 结合 `EXCLUDE` 约束 + 应用侧批量校验，提前压测 1k 节点场景。
- **数据迁移**：需要从现有 Excel/静态结构导入 → 提供导入校验报告，按租户灰度导入并可回滚。
- **性能/缓存一致性**：树缓存与事件失效可能不一致 → 定义幂等失效策略，提供全量重建脚本。
- **范围回弹**：高级特性（workflow/Authz/Impact）被提前挤入 M1 → 通过文档声明与里程碑约束，严格 CR 控制。
- **跨模块耦合**：若 HRM/Finance 期待同步 schema 变更 → 明确 SOR 边界，仅发布事件/视图，避免修改冻结模块。

## 术语映射表（Workday/主流 HR ↔ SAP HCM ↔ 本项目）
| Workday 术语 | 主流 HR 习惯 | SAP HCM 字段名 | 本项目字段名 | 说明 |
| --- | --- | --- | --- | --- |
| Supervisory Organization | 部门/主管线 | ORGEH（Org Unit），关系 A/B002 | 术语：Organization Unit（字段/标签：Org Unit）；`hierarchy_type=OrgUnit`，`node_type=OrgUnit` | M1 仅单树。 |
| Company Organization | 法人/公司 | BUKRS（Company Code） | `hierarchy_type=Company`（M2+ 预留） | M1 不落地，后续占位。 |
| Cost Center | 成本中心 | KOSTL（Cost Center） | `hierarchy_type=CostCenter`（M2+ 预留） | finance SOR，仅消费事件/视图。 |
| Custom Organization | 自定义报表分组 | 自定义评估路径/对象类型（例如 O/Z*） | `hierarchy_type=Custom`（M2+ 预留） | 用于报表分组，占位。 |
| Org ID/Code | 组织编码 | ORGEH / OBJID | `org_nodes.code` | 租户内唯一。 |
| Org Name | 组织名称 | STEXT（短名） | `org_nodes.name` | 父节点+时间窗口内唯一。 |
| Parent Org | 父子关系 | 关系 A/B002（O→O 上级） | `org_edges.parent_node_id` / `child_node_id` | `org_nodes.parent_hint` 仅缓存，建议改名或隐藏。 |
| Manager/Supervisor | 组织负责人 | Chief Position 标记（S-CHIEF，关系 A/B012） | `owner_user_id`（建议改 `manager_user_id`） | 对齐 Manager 语义。 |
| Effective Date / End Date（Inactive Date） | 生效/失效日期 | BEGDA / ENDDA | `effective_date` / `end_date` (`tstzrange` 半开) | 默认失效为开区间 `9999-12-31`。 |
| As-of Date | 查询时点（Key Date） | Key Date（Stichtag） | `effective_date` 参数 | 未传则默认 `time.Now()`。 |
| Primary Supervisory Org | 主属组织 | PA0001-ORGEH（主组织） | `org_assignments.primary` | 仅支持主属，辅属/矩阵待 M2+。 |
| Worker（本项目术语 Person，工号 PERNR） | 员工/雇员 | PERNR | `subject_type=person` + `subject_id=person_id`（工号 `pernr` 不变） | 职位信息单独用 `position_id` 承载，不改主体标识。 |
| Position | 职位 | PLANS（Position） | （未落地）`position_id` 占位 | 计划在 DEV-PLAN-021/M3+。 |
| Effective Status | 状态 | OBJSTAT（对象状态）/ STAT2（雇佣状态） | `status=Active/Retired` | 如需停用态可扩展 `Inactive`。 |
| Org Level | 组织层级 | OTYPE+层级自定义（如 O 等级自定义字段） | （未落地）`org_level` 占位 | 便于报表/BP 路由。 |
| Org Roles (HR Partner 等) | HR 伙伴/业务负责人 | 关系 1001 A/B003（责任人）等 | （未落地）`hr_partner_user_id` 等 | 可在后续里程碑补充。 |

## 后续动作
- 记录本计划在 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 的 PoC/联调日志。
- 与产品/HR BP 对齐 Workday 关键流程（Supervisory、Matrix、Effective Dating）。
- 建立 Workday parity checklist（含 BP、安全域、Retro、Position 管理）并在每个里程碑更新。
- 准备下一阶段（DEV-PLAN-021）聚焦“组织预算与人员编制”。
