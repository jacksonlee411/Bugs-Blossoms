# [Archived] DEV-PLAN-078：Org v4 全量替换（基于 077，无并行/无兼容/防漂移）

**状态**: 已归档（2026-01-05）— 本项目改为 Greenfield 全新实施（新系列从 `DEV-PLAN-077` 开始），本计划不再适用。

> 归档说明：本文仅保留为历史参考，不再作为实施依据。

> 本计划的定位：以一次 cutover 的方式，把当前 `modules/org` 中的 **OrgUnit + Position + Job Catalog**（以及它们的旧支撑表）整体替换为 v4（事件 SoT + 同步投射）的实现，并在同一变更内删除旧 schema 与旧实现。  
> 强约束：**不并行**（不保留双实现/双写/双读）、**不向后兼容**（允许破坏现有 API/UI/数据形态）、**杜绝漂移**（单一事实源、单一写入口、强门禁）。
>
> SSOT：OrgUnit v4=`DEV-PLAN-077`；Position v4=`DEV-PLAN-079`；Job Catalog v4=`DEV-PLAN-080`；多租户隔离（RLS）=`DEV-PLAN-081`。

## 1. 背景与上下文 (Context)
- `DEV-PLAN-077` 定义了 OrgUnit v4 的 **权威契约**：SoT=`org_events`，同步投射到 `org_unit_versions`，DB Kernel + Go Facade，强一致读与可重放重建。
- `DEV-PLAN-079` 定义了 Position v4 的 **权威契约**：SoT=事件表，同步投射到 versions 读模型（并评估并采用“去掉 `org_` 前缀”的表命名策略）。
- `DEV-PLAN-080` 定义了 Job Catalog v4 的 **权威契约**：SoT=事件表，同步投射到 versions 读模型（并评估并采用“去掉 `org_` 前缀”的表命名策略）。
- 本仓库现有 `modules/org` 已累积大量历史包袱与一致性修补（例如围绕 path/长名称一致性的多轮计划与排障），继续增量演进易发生“补丁式叠加 + 权威表达分裂”。
- 因本计划明确“不兼容/不并行”，最佳策略是一次性 cutover：将 OrgUnit/Position/Job Catalog 整体切换到 v4（077/079/080），并在同一变更内删除旧实现与旧 schema（或至少删除其可达路径与依赖），避免长期漂移。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **全量替换**：OrgUnit/Position/Job Catalog 的读写路径、API/UI、持久化层与 schema 全部以 `DEV-PLAN-077/079/080` 为准实现（各自为唯一权威来源）。
- [ ] **无并行**：主干上不可同时存在“可达的”旧实现与新实现（禁止双写/双读/Feature Flag/Shadow Run）。
- [ ] **无向后兼容**：不承诺保留现有 endpoint、payload、UI 行为与旧数据语义；若需保留，应另开计划（本计划明确拒绝）。
- [ ] **强制不保留数据**：清库/重建/仅 seed（选项 A，见 5），以“干净彻底”为唯一目的。
- [ ] **彻底删除旧支撑表**：不保留/不替代旧的 outbox/audit/settings/change-requests/roles 等支撑能力（清单见 4.2.1），相关能力随 cutover 下线。
- [ ] **防漂移**：落地各子域的 One Door Policy（写入口唯一）与错误契约；禁止绕过 Kernel 的写路径。
- [ ] **严格门禁**：命中工具链触发器时按 SSOT 执行（见 2.3），并在验收标准中提供“不可漂移”的机器校验点。

### 2.2 非目标（明确不做）
- 不提供迁移期并行方案（不做双写/回填/灰度/回滚到旧实现）。
- 不保证旧 API/旧 UI 继续可用（外部调用方需自行适配）。
- 不尝试重建历史事件链路（除非作为数据保留策略的显式决策；见 5.1）。
- 不保留旧的 outbox/audit/settings 等支撑能力，也不提供等价替代（彻底方案的预期行为）。
- 不在本计划内引入新的监控/开关切换（对齐仓库原则与 `AGENTS.md`）。

## 2.3 工具链与门禁（SSOT 引用）
> 目的：避免在 dev-plan 里复制工具链细节导致 drift；本文只声明命中项与 SSOT 链接。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码（`AGENTS.md`）
  - [ ] DB 迁移 / Schema（Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [ ] E2E（若修改 org UI/API 流程与断言）
  - [X] 文档（本计划）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org Atlas+Goose 工具链与门禁：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
  - OrgUnit v4 目标架构（唯一权威）：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`
  - Position v4 目标架构（唯一权威）：`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`
  - Job Catalog v4 目标架构（唯一权威）：`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`
  - 多租户隔离（RLS）：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`（对齐 `docs/dev-plans/019-multi-tenant-toolchain.md` / `docs/dev-plans/019A-rls-tenant-isolation.md`）
  - 时间语义（Valid Time=DATE）：`docs/dev-plans/064-effective-date-day-granularity.md`

## 3. 关键原则（本计划的“防漂移合约”）
### 3.1 SSOT（单一事实源）
- **架构/算法/契约 SSOT（按子域）**：
  - OrgUnit：`DEV-PLAN-077`
  - Position：`DEV-PLAN-079`
  - Job Catalog：`DEV-PLAN-080`
  `DEV-PLAN-078` 不复写各 SSOT 的 schema/函数/算法细节，只定义“如何替换与如何验收”。
- **Schema SSOT（实施时）**：仍沿用 Org 工具链的单一事实源文件：`modules/org/infrastructure/persistence/schema/org-schema.sql`（对齐 021A），其内容以 077/079/080 的 schema 为准组合落盘。

### 3.2 One Door Policy（写入口唯一）
- 写入必须经由各子域唯一入口：`submit_org_event(...)` / `submit_position_event(...)` / `submit_assignment_event(...)` / `submit_job_family_group_event(...)` / `submit_job_family_event(...)` / `submit_job_level_event(...)` / `submit_job_profile_event(...)`（或各 SSOT 定义的等价唯一入口）。
- 通过 DB 权限/Schema 组织限制，确保应用角色不能直写任一事件表/versions 表，也不能直调 `apply_*_logic`（它们属于 Kernel 内部实现细节，而非公共 API）。

### 3.3 “无并行”的落地口径
- **禁止 Feature Flag/路由分流**：不允许在主干保留旧/new 两套实现再“按开关选路”。
- 允许在同一个 PR 中出现“旧表 + 新表”短暂并存（迁移原子性所需），但合并后不得存在旧实现的可达路径；旧表需在同一 cutover 里被 drop/改名隔离并彻底断开依赖。
- 所有 PR 必须满足：主干任一提交点都能解释“当前线上只会跑哪一套 org 实现”（否则拒绝合并）。

### 3.4 v4“双表时序 + 全量重放”模式合约（强制）
> 目的：把 077-080 的共同模式固化为“可验收合约”，避免实现期各子域各写一套时序算法导致复杂度爆炸。

**双表分离（SSOT + Projection）**：
- 每个业务领域采用 **事件表（`*_events`，Write Model/SSOT）** + **versions 表（`*_versions`，Read Model/Projection）**。
- `*_events` append-only（不可变）；`*_versions` 为衍生数据，可丢弃重建。

**同事务全量重放（选定，保持逻辑简单）**：
- `submit_*_event` 的职责固定为：插入事件（幂等）→ **全量重放**（删除并重建对应 versions）→ 校验不变量 → 同一事务提交。
- “重放范围”必须在各子域 SSOT 中明确（例如 OrgUnit=整棵树；Position=单 position 聚合；Job Catalog=单实体或单聚合）。

**gapless + no-overlap（纳入合同）**：
- versions 必须同时满足：
  - `EXCLUDE USING gist` 保证 no-overlap；
  - **gapless**：相邻版本区间必须严丝合缝（上一段 `upper(validity)` 等于下一段 `lower(validity)`），最后一段 `upper(validity)='infinity'`。
- 业务上“停用/失效”必须建模为 status 切片（例如 `status='disabled'`），而不是制造空洞。

**同日事件唯一（按实体，按表）**：
- 对任一 events 表，约束为：同一 `tenant_id + <entity_id> + effective_date` **最多一条事件**（不引入 `effseq`）。
- 同日发生的不同业务意图，必须建模为不同业务领域/不同 events 表（例如晋升 vs 调薪；Position vs Assignment）。

**合同完备性一致（不得降级）**：
- 077/079/080 必须同构提供最小合同：schema（events+versions）/重放算法/并发互斥（锁粒度与 key）/错误契约（SQLSTATE+constraint+stable code）/索引建议/运维重放/验收标准。

## 4. 替换策略（Big-bang Cutover）
### 4.1 交付策略（避免主干阶段性漂移）
- 推荐：**单个 cutover PR**（或严格串联的少量 PR），保证合并时同时完成：
  - v4 schema + Kernel + Go Facade 落地；
  - 路由/调用链切换；
  - 旧实现删除；
  - 旧 schema 的 drop/隔离迁移；
  - 关键验收与门禁通过。
- 如必须拆分 PR：除“纯重构/纯测试/纯文档”外，任何引入新实现的 PR 都不得让主干出现双实现可达路径。

### 4.2 Schema 变更策略（不兼容 + 不并行）
- 以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为唯一 schema SSOT，将其替换为 v4 schema（077/079/080：OrgUnit/Position/Job Catalog 的新增表/函数/索引/约束）。
- **命名策略（选定）**：OrgUnit v4 保持 `org_` 前缀（对齐 077）；Position/Assignment v4 去掉 `org_` 前缀并采用 `position_*`/`assignment_*`；Job Catalog v4 去掉 `org_` 前缀并采用 `job_*`（不使用共享 `job_catalog_events` 事件表）。
- 通过 Org Atlas+Goose 工具链生成并提交 `migrations/org/**`，确保 plan/lint/goose smoke 可重复执行（SSOT：021A）。
- **破坏性变更是预期行为**：drop 旧表/旧列/旧函数属于计划内动作；需要在迁移脚本中显式标注并通过 atlas lint 的允许机制（口径以现有仓库实践为准）。

> 红线提醒：实施阶段一旦出现 `CREATE TABLE`（新增表），按 `AGENTS.md` 必须先获得用户手工确认。本计划先定义决策与步骤，不直接执行新增表。

#### 4.2.1 需要新增/删除/改造的数据库表（方案清单）
> 说明：本清单用于“替换落盘范围”与“验收点”对齐；具体字段/索引/函数的权威定义以 `DEV-PLAN-077/079/080` 为准，避免在本文复制导致 drift。

**新增（最终保留，v4 SSOT）**：
- `org_trees`（OrgUnit；每租户单树锚点，root 唯一性事实源；077）
- `org_events`（OrgUnit；SoT，append-only；077）
- `org_unit_versions`（OrgUnit；读模型，versions + ltree + daterange + no-overlap；077）
- `positions`（Position；稳定实体；079）
- `position_events`（Position；SoT，append-only；079）
- `position_versions`（Position；读模型，versions + daterange + no-overlap；079）
- `assignments`（Assignment；稳定实体；079）
- `assignment_events`（Assignment；SoT，append-only；079）
- `assignment_versions`（Assignment；读模型，versions + daterange + no-overlap；079）
- `job_family_groups`（Job Catalog；identity；080）
- `job_families`（Job Catalog；identity；080）
- `job_levels`（Job Catalog；identity；080）
- `job_profiles`（Job Catalog；identity；080）
- `job_family_group_events`（Job Catalog；SoT，append-only；080）
- `job_family_events`（Job Catalog；SoT，append-only；080）
- `job_level_events`（Job Catalog；SoT，append-only；080）
- `job_profile_events`（Job Catalog；SoT，append-only；080）
- `job_family_group_versions`（Job Catalog；versions（daterange + no-overlap）；080）
- `job_family_versions`（Job Catalog；versions（daterange + no-overlap）；080）
- `job_level_versions`（Job Catalog；versions（daterange + no-overlap）；080）
- `job_profile_versions`（Job Catalog；versions（daterange + no-overlap）；080）
- `job_profile_version_job_families`（Job Catalog；ProfileVersion↔Families 多值关系；080）

**删除（旧实现彻底移除，不保留数据）**：
- `org_nodes`
- `org_node_slices`
- `org_edges`
- `org_hierarchy_closure_builds`
- `org_hierarchy_closure`
- `org_hierarchy_snapshot_builds`
- `org_hierarchy_snapshots`
- `org_reporting_nodes`
- `org_positions`
- `org_position_slices`
- `org_job_family_groups`
- `org_job_families`
- `org_job_levels`
- `org_job_profiles`
- `org_job_family_group_slices`
- `org_job_family_slices`
- `org_job_level_slices`
- `org_job_profile_slices`
- `org_job_profile_slice_job_families`
- `org_position_slice_job_families`
- `org_assignments`
- `org_attribute_inheritance_rules`
- `org_roles`
- `org_role_assignments`
- `org_change_requests`
- `org_settings`
- `org_audit_logs`
- `org_outbox`
- `org_security_group_mappings`
- `org_links`
- `org_personnel_events`

**改造（结构调整/保留表名）**：
- 无（本计划选择“清库/重建/仅 seed”，不保留旧表名与旧结构；全部以 v4 新表替换）。

### 4.3 Go 层替换策略（不保留旧契约）
- 以 077/079/080 的边界为准：Go 仅作为 Command Facade；写路径只调用各子域 `submit_*_event`；读路径通过各 SSOT 定义的查询函数/视图对齐（例如 OrgUnit 的 `get_org_snapshot` 等）。
- 旧的 repository/service/controller（以及其测试/E2E 断言）在 cutover PR 中删除或彻底改写，避免“旧逻辑仍在某些角落被调用”。同理：所有旧支撑表相关逻辑（outbox/audit/settings/change-requests 等）必须同步删除。

## 5. 数据处理策略（必须先定：保留还是丢弃）
> 本计划“不兼容”不等于“可以不做数据决策”。为避免实现期临时补丁，本节将数据策略固化为唯一选择（强制），并作为验收与门禁的一部分。

### 5.1 强制决策（已确认）：选项 A（不保留）
- [X] **不保留（清库/重建/仅 seed）**：以“干净彻底”为唯一目的；数据不可逆。
- [X] 明确禁止：不做旧数据导入、不做快照迁移、不做历史事件重建。

### 5.2 执行口径（清库/重建/仅 seed）
目标：在一次 cutover 变更中，把“旧 Org 模块的一切数据与实现”从系统中移除，使系统进入“只有 v4（077/079/080）”这一种可运行状态。
- 数据库层面：
  - 迁移中 drop 旧表/旧函数/旧索引（不保留历史），并创建 v4 schema（077/079/080）。
  - 在干净库上执行 seed（具体入口与数据集以 `Makefile`/现有 seed 体系为准；本计划不复制命令细节）。
- 应用层面：
  - 删除旧实现（读写/控制器/服务/仓储/模板/测试断言），仅保留 v4（077/079/080）对外入口。

## 6. 实施步骤（建议顺序）
> 采用 045 的 Research→Plan→Implement 思路，但本计划以“替换交付”为主线列步骤。

1. [ ] 现状盘点（Research，必须）
   - [ ] 列出当前 `modules/org` 的对外契约入口（路由/API/页面）与调用链。
   - [ ] 列出当前 OrgUnit/Position/Job Catalog schema 关键表与其 SSOT 文件位置（以 021A 定义为准）。
   - [ ] 列出 authz 依赖点（若存在），确认是否需要同步更新（不兼容允许改，但必须显式）。
2. [X] 数据决策确认：强制不保留（5.1）
3. [ ] v4 schema/Kernal 落地（按 077/079/080/081；遵守“新增表需确认”红线）
4. [ ] Go Facade + 路由/UI 全量切换（按 077/079/080；删除旧实现）
5. [ ] 清库/重建/seed（按 5.2 的执行口径）
6. [ ] 删除遗留（旧表/旧函数/旧代码/旧测试断言）并通过全部门禁

## 7. 验收标准（Acceptance Criteria）
### 7.1 “无并行/防漂移”机器校验（必须）
- [ ] 主干中不存在可达的旧写路径：所有写请求只能走 `submit_*_event`（`submit_org_event/submit_position_event/submit_assignment_event/submit_job_family_group_event/submit_job_family_event/submit_job_level_event/submit_job_profile_event` 或等价唯一入口）。
- [ ] 主干中不存在对旧表/旧函数的引用（以 ripgrep/编译失败作为硬门禁；具体关键字在实施阶段由“现状盘点”产出）。
- [ ] DB 权限或 schema 组织保证：应用角色无法绕过入口直写表/直调内部函数（落地口径以实施时环境为准）。
- [ ] 不保留数据被强制执行：不包含任何“旧数据导入/迁移脚本/回填逻辑”；仅存在 seed 数据集与 v4 schema。
- [ ] 数据库中旧表均不存在（drop 完成），且仅存在 4.2.1“新增（最终保留）”的 v4 表作为持久化基座。

### 7.2 工具链门禁（必须）
- [ ] Org Atlas+Goose：`make org plan && make org lint && make org migrate up`（SSOT：021A）。
- [ ] Go 质量门禁：按 `AGENTS.md` 触发器矩阵执行并通过。

### 7.3 行为验收（必须）
- [ ] 在 seed 初始化后，OrgUnit 的 Create/Move/Rename/Disable as-of 查询满足 077 的不变量与错误契约。
- [ ] 在 seed 初始化后，Position/Assignment 的写入与 as-of 查询满足 079 的不变量与错误契约。
- [ ] 在 seed 初始化后，Job Catalog 的写入与 as-of 查询满足 080 的不变量与错误契约。
- [ ] RLS（对齐 081）：v4 tenant-scoped 表默认启用 RLS（ENABLE+FORCE+policy，fail-closed），且运行态 `RLS_ENFORCE=enforce` 下跨租户读写被拒绝/隔离。
- [ ] 并发写互斥与 fail-fast 行为符合各 SSOT（busy 错误稳定可映射）。
- [ ] 每次写入均触发“同事务全量重放”，且重放后 versions 同时满足 no-overlap + gapless（3.4）。

## 8. 风险与缓解
- **破坏性迁移风险**：drop 旧表不可逆。
  - 缓解：在执行 cutover 前明确备份/快照策略与验证脚本（本计划不保留数据，但仍需要“可恢复的事故处理手段”；具体形式在实施阶段补齐并写入 readiness）。
- **替换 PR 过大**：
  - 缓解：允许少量“纯重构/纯测试”前置 PR，但禁止主干出现双实现可达路径。
- **RLS fail-closed 误配置风险**：v4 默认启用 RLS；若漏注入 `app.current_tenant` 或将 `RLS_ENFORCE=disabled`，会导致读写 fail-closed（表现为报错/无数据）。
  - 缓解：按 `DEV-PLAN-081` 强制 `RLS_ENFORCE=enforce` + 非 superuser 且 `NOBYPASSRLS` 的 DB role；所有 v4 DB 访问统一走 `InTenantTx`/`ApplyTenantRLS`。
- **数据丢失的组织风险**：
  - 缓解：在执行前获得明确确认（本计划已确认选项 A），并将“不可逆”写入变更说明与验收记录。

## 9. 既有 DEV-PLAN 的处置清单（SSOT 收敛，杜绝漂移）
> 目的：避免旧 dev-plan 仍被误当作“当前实施依据”。本节定义哪些文档需要在 cutover PR 中被**修改标注**或**撤销其实施地位**。

### 9.1 仍保持为 SSOT/依赖输入（保留）
- `docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`：OrgUnit v4 唯一权威契约（Kernel/Fascade/Schema/算法/错误契约）。
- `docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`：Position v4 唯一权威契约（Kernel/Fascade/Schema/算法/错误契约）。
- `docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`：Job Catalog v4 唯一权威契约（Kernel/Fascade/Schema/算法/错误契约）。
- `docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`：v4 tenant-scoped 表启用 RLS 的权威契约（fail-closed、注入、错误/回滚口径）。
- `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`：Org Atlas+Goose 工具链与门禁口径（实施必需）。
- `docs/dev-plans/064-effective-date-day-granularity.md`：Valid Time=DATE 语义约束（实施必需）。
- `docs/dev-plans/045-simple-not-easy-review-guide.md`：实施/评审方法论（过程约束）。

### 9.2 需要“撤销实施地位/标注已被取代”的计划（在 cutover PR 中执行）
> 执行规则（历史记录）：在这些文档顶部状态行追加备注 `— 已被 DEV-PLAN-078 取代（无并行/无兼容）`，并在正文首段加一句“仅作历史记录，不再作为实施依据”。

- `docs/dev-plans/035-org-ui.md`
- `docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
- `docs/dev-plans/036-org-sample-tree-data.md`（如 seed 数据集在 078 下重建，则该计划需明确不再适用）
- `docs/dev-plans/037-org-ui-ux-audit.md`
- `docs/dev-plans/037A-org-ui-verification-and-optimization.md`
- `docs/dev-plans/065-org-node-details-long-name.md`
- `docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`
- `docs/dev-plans/069-org-long-name-parent-display-mismatch-investigation.md`
- `docs/dev-plans/069A-org-long-name-generate-from-org-edges-path.md`
- `docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`
- `docs/dev-plans/069C-org-tree-long-name-inconsistency-investigation.md`
- `docs/dev-plans/070-org-ui-correct-and-delete-records.md`
- `docs/dev-plans/053-position-core-schema-service-api.md`
- `docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`
- `docs/dev-plans/075-job-catalog-effective-dated-attributes.md`
- `docs/dev-plans/075A-job-catalog-identity-legacy-columns-retirement.md`

### 9.3 明确保留为“历史研究输入”（无需撤销，但禁止当作 SSOT）
- `docs/dev-plans/076-org-v4-transactional-event-sourcing-gap-analysis.md`：作为 v3→v4 的差异研究记录保留；实施以 077/078 为准。
