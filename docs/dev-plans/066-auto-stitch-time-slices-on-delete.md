# DEV-PLAN-066：组织/职位/任职时间片删除自动缝补（Auto-Stitch），保障时间轴连贯

**状态**: 规划中（2025-12-28 00:00 UTC）

## 1. 背景与上下文 (Context)
- **SSOT（时间语义）**：Valid Time=DATE（日粒度闭区间），Audit/Tx Time=TIMESTAMPTZ（见 `docs/dev-plans/064-effective-date-day-granularity.md`）。
- **行业对标（R401）**：`docs/dev-records/R401-HRMS 时态数据与性能优化.md` 指出：仅依赖应用层维护时态连续性在高并发下容易产生竞态；Oracle DateTrack 通过 DB 侧触发器维护时间连续性（防断层）。本计划在“写逻辑仍由 Service 显式执行”的前提下，引入 DB 侧 commit-time 校验门禁（见 4.7），把 gap-free 从“约定”升级为“不可违背的不变量”。
- **现状问题**：
  - Postgres 的 `EXCLUDE USING gist (... daterange(effective_date, end_date + 1, '[)') WITH &&)` 只保证**不重叠**，不会阻止“中间删除导致的 gap（空洞）”。
  - 目前 Org 核心链路（组织架构、职位、任职等）写路径主要依赖“插入时截断/续接”或“ShiftBoundary”来维持自然拼接；但当用户执行“删除某条时间片/记录”时，如果仅做 `DELETE`，时间轴会出现空洞，从而破坏“相邻切片自然连贯”的心智与下游推导。

### 1.1 现状实现快照（Org 模块：可复用的“写入模板”）
> 目的：明确“现状为何如此”，并在本计划中声明复用点，避免实现阶段即兴发明新模式。

- **相邻边界移动（已实现）**：`ShiftBoundaryNode/ShiftBoundaryPosition`（事务内锁相邻切片，并同时更新相邻边界以保持拼接）。
- **“从某天起删除后续切片 + 截断旧段 + 插入替代段”（已实现）**：
  - Node：`RescindNode`（删除 `effective_date >= D` 的切片，对覆盖段做截断，再插入 `rescinded` 段）
  - Position：`RescindPosition`（同上，对 `org_position_slices`）
- **Assignment 现状**：`RescindAssignment` 通过更新当前记录的 `end_date` 实现“提前结束”，未提供“删除一条切片并缝补”的能力。

### 1.2 本计划的“删除”定义（避免语义漂移）
本计划中的“删除”专指：**撤销某次生效变更**——从同一条时间线上删除一段切片 `T`，并将 `T` 的有效期并入相邻切片，从而保证剩余切片仍然自然拼接。

本计划**不**覆盖“删除实体”（例如删除 org_node / org_position 本体）这类生命周期语义。

### 1.3 依赖与前置条件：仅在 DEV-PLAN-064 阶段 D 之后实施
为避免在 064 的 B1 双轨阶段引入偶然复杂度，DEV-PLAN-066 **仅在 DEV-PLAN-064 阶段 D 合并完成后**实施（参考 `docs/dev-plans/064-effective-date-day-granularity.md` 与 `docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`）。阶段 D 的收敛是本计划的硬前置条件：
- DB schema 中 Valid Time 的唯一权威表达为 `effective_date/end_date`（`date`，day 闭区间）。
- `effective_on/end_on` 已被彻底移除（schema/SQL/Go 运行时代码中不再出现；历史迁移文件除外）。
- legacy `timestamptz effective_date/end_date` 已被清理，不再存在同名混用风险（以阶段 D 的最终 schema 为准）。
- 本计划不提供任何双轨兼容逻辑；若环境仍处于阶段 C/B1，请先完成阶段 D 再推进 066。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] 为以下“时态（时间片）实体”的**删除操作**提供自动缝补（Auto-Stitch），确保删除后剩余切片满足“自然拼接”：
  - `org_node_slices`（组织节点属性切片）
  - `org_edges`（组织层级关系切片）
  - `org_position_slices`（职位属性切片）
  - `org_assignments`（任职切片）
- [ ] 删除中间切片后，自动更新相邻切片的结束/起始边界，使时间轴**无 gap、无 overlap**（overlap 由 DB EXCLUDE 兜底，gap 由本计划补齐）。
- [ ] 增加数据库侧“只校验不修复”的 gap-free 门禁：同一时间线 key 下提交时必须满足自然拼接，否则事务回滚（用于防止绕过 Service 写路径直接落库 gap）。
- [ ] 删除行为具备与现有“Correct/ShiftBoundary”一致的：
  - 事务与行级锁（避免并发写入撕裂）
  - freeze 窗口校验（遵循 org_settings）
  - 审计日志（`org_audit_logs`）与事件发布（`*.v1`）
- [ ] 增加集成测试覆盖“删除中间切片自动缝补”的关键场景。

### 2.2 非目标
- 不引入“在 DB 侧自动缝补/隐式改写数据”的触发器（缝补逻辑仍由 Service 显式执行；DB 侧仅做约束校验门禁）。
- 不把系统扩展为双时态（Bi-temporal）；仅处理 Valid Time（day）维度上的自然拼接。
- 不新增事件版本（topic 保持 `*.v1`，与 DEV-PLAN-064 的约束一致）。
- 不在本计划中解决“同日多次生效（EFFSEQ）”问题（仍遵循 DEV-PLAN-064 的前提限制）。
- 不引入任何“过渡双轨/双写”字段或逻辑：本计划按最终 `date effective_date/end_date` 的唯一契约实现。

## 2.3 工具链与门禁（SSOT 引用）
- 本计划涉及 Go 代码、Org 模块写路径与（可能的）DB 行为验证；触发器与必跑入口以 `AGENTS.md` 与 `Makefile` 为准。
- 若触及 `migrations/org/**` 或 `modules/org/infrastructure/persistence/schema/**`，按 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md` 执行对应门禁。

## 3. 关键约束与术语 (Invariants & Definitions)
### 3.1 自然拼接（Natural Stitch）
Valid Time 的业务语义为 day 闭区间 `[effective_date, end_date]`（均为 `date`）。相邻切片自然拼接要求：
- `prev.end_date + 1 day == next.effective_date`

### 3.2 自动缝补（Auto-Stitch）的语义
删除切片 `T` 时，为避免产生“中间空洞”，本计划 v1 固化为单一、稳定且可解释的策略：**合并到上一片（merge-into-prev）**。

规则：
- 若存在上一片 `P`：删除 `T` 后，把 `P` 的结束边界延长到 `T` 的结束边界（从而让 `P` 覆盖原本由 `T` 覆盖的时间窗口）。
- 若不存在上一片（删除的是最早切片）：仅删除 `T`，时间线从下一片开始（不视为“中间 gap”）。
- v1 不提供 merge-into-next（把下一片起点前移）的能力；若未来确需支持，再另起决策（避免实现期出现隐式 fallback）。

### 3.3 “无 gap”不等于“全程有效/在职”
本计划要求的 gap-free 是**时间轴连续性不变量**：任意一天都应被某条切片覆盖；这不代表业务状态永远为“有效/在职”。对齐 SAP HCM 的做法：通过“状态切片”表达正常的暂停/无效期间，而不是把这段时间留空。

典型示例（同构于组织/职位/任职）：
- 组织停用→启用：停用期间是“无效”切片，不是 gap。
- 职位停用→启用：停用期间是“inactive”切片，不是 gap（`org_position_slices.lifecycle_status` 已具备）。
- 离职→再入职：离职生效日起到再入职前一天，是“无效/非在职”切片，不是 gap。
- 待岗/暂离→返岗：暂离期间同样用“无效/非在职”切片表达。

## 4. 设计方案（Service-First，事务内缝补）
### 4.1 作用范围与时间线 key
为保证“找得到相邻切片”，每类表必须明确其“时间线 key”（同 key 下时间片应自然拼接）：
- `org_node_slices`：`(tenant_id, org_node_id)`
- `org_edges`：`(tenant_id, hierarchy_type, child_node_id)`
- `org_position_slices`：`(tenant_id, position_id)`
- `org_assignments`（primary 时间线）：`(tenant_id, subject_type, subject_id, assignment_type)` 且 `assignment_type='primary'`（仅对 primary 启用 gap-free 门禁；其他类型不强制）

并发写入需要额外强调：仅靠 “锁定 T 与 P” 容易在“缺失相邻片 / 新写入路径忘记同样锁序 / 幻读”时漏掉互斥。为将“同一条时间线的写操作”串行化，本计划在事务内按时间线 key 获取 `pg_advisory_xact_lock`（v1 固化 lock key 计算方式；对齐既有 `hashtext` 用法）：
- `pg_advisory_xact_lock(hashtext(lock_key_text))`（碰撞只会带来额外串行化，不影响正确性）
- `timeline_key_text` 的构成按 4.1 的 key 顺序拼接（例如 `org_edges`：`hierarchy_type:child_node_id`；`org_assignments`：`subject_type:subject_id:assignment_type`）
- `lock_key_text = format('%s:%s:%s', table_name, tenant_id, timeline_key_text)`

### 4.2 写入权威表达（避免两套写法）
本计划实现只允许读写 `effective_date/end_date`（`date`）。任何层面出现“同义字段/第二套边界表达”都视为违反唯一性原则，直接打回。

### 4.3 核心算法：DeleteSliceAndStitch（确定顺序，避免 EXCLUDE 瞬时冲突）
输入（v1）：`target_effective_date`（UTC day boundary），`reason_code/reason_note` 等审计元信息。

1) 开启事务；获取“时间线级”互斥（`pg_advisory_xact_lock`）；锁定目标切片 `T`（`SELECT ... FOR UPDATE`）。
2) `affected_at = T.effective_date`，并据此执行 freezeCheck。
3) 锁定上一片 `P`（同一时间线 key）：
   - 优先按“自然拼接边界相等”定位：`P.end_date + 1 day == T.effective_date`。
   - 若 `P` 不存在：视为删除最早切片（允许，直接进入步骤 5）。
4) **先删除后更新**（制造短暂 gap，不触发 overlap；再填补 gap）：
   - `DELETE` 目标切片 `T`。
   - 若 `P` 存在：更新 `P.end_date = T.end_date`。
5) 写入审计日志：记录 `T` 的被删除事实、以及 `P` 的窗口变更（若存在），`Operation="DeleteSliceAndStitch"`。
6) 发布事件（`*.v1`）：视为 `*.corrected` 类变更，`effective_date = affected_at`，`end_date = end_of_time`（窗口粒度沿用现有事件契约）。
7) 提交事务；按现有策略触发缓存失效。

### 4.4 每类实体的“可执行规格”（时间线 key / 锁定方式 / 写入点）
> 目的：让实现具有确定性；避免“另一个人按同一份 Spec 会写出不同结构”。

#### 4.4.1 `org_node_slices`
- 时间线 key：`(tenant_id, org_node_id)`
- 目标定位：`effective_date == target_effective_date`（锁 `FOR UPDATE`）
- 上一片定位：`end_date == target_effective_date - 1 day`（锁 `FOR UPDATE`）
- 缝补更新：`UPDATE ... SET end_date = T.end_date`
- 删除：`DELETE ... WHERE tenant_id=? AND id=?`

#### 4.4.2 `org_position_slices`
- 时间线 key：`(tenant_id, position_id)`
- 目标定位：`effective_date == target_effective_date`（锁 `FOR UPDATE`）
- 上一片定位：`end_date == target_effective_date - 1 day`（锁 `FOR UPDATE`）
- 缝补更新：`UPDATE ... SET end_date = T.end_date`
- 删除：`DELETE ... WHERE tenant_id=? AND id=?`

#### 4.4.3 `org_edges`
- 时间线 key：`(tenant_id, hierarchy_type, child_node_id)`
- 目标/上一片定位：需要补齐 Repository 锁定能力（以 `effective_date/end_date` 边界相等为准，`FOR UPDATE`）
- 缝补更新：可复用现有 `TruncateEdge`（本质是 `UPDATE ... SET end_date=?`，同时适用于“截断/延长”）
- 删除：可复用现有 `DeleteEdgeByID`（按 `id` 删除）
- 副作用：边关系变化可能影响 closure/build/snapshot；实现阶段需明确复用既有“写入后失效/重建触发”路径，并用集成测试验证读路径一致性。对齐 R401 的建议：避免在命令写事务内同步触发大规模级联重算，优先采用 Outbox 驱动的异步刷新（最终一致性）。

#### 4.4.4 `org_assignments`
- 时间线 key（primary）：`(tenant_id, subject_type, subject_id, assignment_type)` 且 `assignment_type='primary'`
- 字段约束（v1）：在 `org_assignments` 增加“在职状态”列（例如 `employment_status`：`active|inactive`），用于填充离职/停用/暂离等期间，避免以“无记录”表示正常状态。
  - 示例：离职生效日 `2025-12-28` ⇒ 写入 `[2025-12-28, 9999-12-31]` 的 `employment_status='inactive'` 切片（而不是留下空洞）。
- 目标定位（v1）：通过 `assignment_id` 锁定 `T`（`FOR UPDATE`），并以 `T.effective_date` 作为边界。
- 上一片定位：需要补齐 Repository 锁定能力：按时间线 key 查找 `end_date == T.effective_date - 1 day` 的 `P`（`FOR UPDATE`）。
- 缝补更新：`UPDATE ... SET end_date = T.end_date`
- 删除：需要补齐 `DELETE FROM org_assignments WHERE tenant_id=? AND id=?`
- 语义说明：该操作等价于“从 T 起恢复为上一片 P 的属性”，需要在审计与事件中明确这一点。

### 4.5 边界场景决策（v1 固化）
- 删除中间切片：若存在 `P`，必须缝补（延长 `P` 覆盖 `T`）。
- 删除最后切片（`T.end_date=end_of_time`）：若存在 `P`，则 `P` 延长到 `end_of_time`；否则时间线为空。
- 删除最早切片（无 `P`）：允许直接删除；时间线从下一片开始（不视为“中间 gap”）。
- 删除唯一切片：允许；结果为时间线为空。

### 4.6 失败路径与约束冲突
Auto-Stitch 可能因“延长相邻切片窗口”触发其他约束失败（例如 sibling-name-unique 的时间窗口扩大后发生冲突）：
- 约束失败时整体回滚，并返回与现有写路径一致的业务错误映射（`409/422`，由 `mapPgError`/service error 策略决定）。
- v1 不做任何隐式 fallback（例如改用 merge-into-next），避免行为不可解释。

另外，若 DB 侧 gap-free 校验门禁（见 4.7）失败，也会在事务提交阶段回滚；v1 固化将其映射为 `409 ORG_TIME_GAP`（见 4.8），避免客户端误以为是随机 DB 故障。

### 4.7 数据库约束：commit 时校验 gap-free（不做隐式修复）
> 目的：将“gap-free”从“约定/习惯”升级为“不可违背的不变量”，同时保持缝补逻辑显式在 Service 层，避免 DB 隐式改写数据。

对齐 `docs/dev-records/R401-HRMS 时态数据与性能优化.md` 的行业对标结论：时态连续性属于“数据质量硬约束”，更适合落在 Postgres Constraints/Triggers 作为最后一道门禁，而不是依赖应用层约定。

建议为各时间片表增加 **DEFERRABLE CONSTRAINT TRIGGER**（仅校验，不修复），在事务提交时检查同一时间线 key 下是否满足自然拼接：
- 触发时机：`AFTER INSERT OR UPDATE OR DELETE`，`DEFERRABLE INITIALLY DEFERRED`（提交时统一校验，避免中间态干扰）。
- 覆盖范围（v1）：对 `org_assignments` **仅对 `assignment_type='primary'` 的时间线**启用 gap-free 校验；其他表按各自时间线 key 全量启用。
- 校验范围（v1 固化）：以“受影响的时间线 key”为粒度，**校验该 key 下所有切片的相邻边界**（O(n)；n 通常较小且仅对变更 key 生效）。若未来遇到性能瓶颈，再评估引入 transition table 去重或邻域校验（不在 v1 范围内）。
- 校验规则：按 `effective_date` 升序，要求任意相邻两片满足 `prev.end_date + 1 day == next.effective_date`；首片不要求有 `prev`。
- 实现建议：以 `daterange(effective_date, end_date + 1, '[)')` 作为唯一的 Period 表达（与 no-overlap 的 EXCLUDE 约束同源），并尽量使用 range 运算符以复用 GiST 索引能力。

这样即便有人绕过 Service 直接执行 `DELETE` 或不按 066 算法更新边界，最终也无法提交产生 gap 的状态；同时 DB 不会“偷偷帮你缝补”，避免隐式副作用与审计漂移。

### 4.8 错误码与排障（v1 固化）
- DB 门禁失败：constraint trigger 函数使用 `ERRCODE = 'integrity_constraint_violation'`（`SQLSTATE 23000`）并设置 `CONSTRAINT = 'org_*_gap_free'`；错误信息必须包含 `tenant_id` 与时间线 key，便于定位。
- Service 映射：`modules/org/services/pg_errors.go` 增加分支，把 `Code==23000 && ConstraintName ~ '_gap_free$'` 映射为 `409 ORG_TIME_GAP`（message：`time slices must be gap-free`）。
- 排障 SQL（示例：按任意时间片表的时间线 key 定位断层）：
  - `SELECT effective_date, end_date, lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date FROM ... WHERE ... ORDER BY effective_date;`
  - 任意一行满足 `prev_end_date IS NOT NULL AND prev_end_date + 1 <> effective_date` 即为 gap。

## 5. 接口契约（API / UI）
### 5.1 Service 层接口（建议）
- `DeleteNodeSliceAndStitch(tenantID, nodeID, targetEffectiveDate, reasonCode, reasonNote)`
- `DeleteEdgeSliceAndStitch(tenantID, hierarchyType, childNodeID, targetEffectiveDate, reasonCode, reasonNote)`
- `DeletePositionSliceAndStitch(tenantID, positionID, targetEffectiveDate, reasonCode, reasonNote)`
- `DeleteAssignmentAndStitch(tenantID, assignmentID, reasonCode, reasonNote)`

入参约束（v1）：
- `targetEffectiveDate` 必须来自 `YYYY-MM-DD` 的统一解析/归一化路径（UTC 00:00），并遵循 DEV-PLAN-064 的“Valid Time=date（日）”契约。

### 5.2 Controller/路由（按需）
若暴露为 API：
- 采用与现有 `:rescind` / `:correct` 风格一致的命名（例如 `:delete-slice`），并接受 `effective_date=YYYY-MM-DD`。
- 鉴权与审计策略与同实体的 `Correct/ShiftBoundary` 对齐（本计划不新增 policy，沿用既有对象/动作语义）。

## 6. 测试与验收标准 (Acceptance Criteria)
- [ ] 删除中间切片后，剩余切片满足自然拼接：
  - 对任意相邻切片 `prev.end_date + 1 day == next.effective_date`
- [ ] 覆盖关键场景的集成测试：
  - 3 段切片 A/B/C，删除 B 后 A 与 C 连续
  - 删除最后一段（`T.end_date=end_of_time`）后，上一段延长至 `end_of_time`
  - 删除第一段（无 `P`）时不做缝补，但不产生“中间 gap”
  - 约束冲突时回滚并返回可解释错误
- [ ] DB 侧 gap-free 校验门禁生效：绕过 Service 的写入（例如仅 `DELETE T`）无法提交产生 gap 的状态。
- [ ] 审计日志与事件发布行为符合现有约定（topic 仍为 `*.v1`）。

## 7. 里程碑与实施步骤 (Milestones)
1. [ ] 明确每类实体的“时间线 key”与删除入口（UI/API/内部调用点）。
2. [ ] 补齐 Repository 能力缺口（以 4.4 为清单），避免引入新 package（仅在现有 repo 内补函数）。
3. [ ] 写路径并发门禁：在相关 Service 写路径中引入时间线级 `pg_advisory_xact_lock`（对齐 4.1/4.3），确保同一时间线写入串行化。
4. [ ] 实现 Service：`Delete*SliceAndStitch`（复用 freeze、audit、outbox、cache invalidation 模板；算法顺序遵循 4.3）。
5. [ ] 数据库门禁：为时间片表增加 4.7 的 gap-free 校验（DEFERRABLE CONSTRAINT TRIGGER，仅校验不修复）；必要时在启用前做一次数据审计/修复，避免把历史脏数据直接“锁死”。
6. [ ] 新增集成测试覆盖关键场景（对齐既有 `ShiftBoundary*` 测试风格，并显式验证 `end_date==next.effective_date` 与 day 口径等价性）。
7. [ ] Readiness：按 `AGENTS.md` 的触发器执行并记录（必要时创建 `docs/dev-records/DEV-PLAN-066-READINESS.md`）。
