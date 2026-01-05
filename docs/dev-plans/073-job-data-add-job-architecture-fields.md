# DEV-PLAN-073：任职记录页增加字段（职类/职种/职位模板/职级）

**状态**: 规划中（2025-12-31 10:48 UTC）

## 1. 背景与上下文 (Context)
### 1.1 需求来源
HR 在查看“任职记录/任职经历”时，需要同时看到岗位在职位体系（Job Catalog / Job Architecture）上的分类信息，以减少跨页面跳转与误判成本。

### 1.2 当前现状
当前 `/org/assignments` 时间线表格主要展示人员号、生效区间、事件类型、组织、职位等信息；缺少职位体系维度：
- 职类（Job Family Group）
- 职种（Job Family）
- 职位模板（Job Profile）
- 职级（Job Level）

### 1.3 范围澄清（避免语义漂移）
- 目标页面：`GET /org/assignments` 的时间线表格（`orgui.AssignmentsTimeline`）。
- 同时影响并纳入验收：Person 详情页“任职经历”区块通过 HTMX 复用 `/org/assignments?...&include_summary=1` 渲染同一时间线表格，因此新增列必须在两处一致生效。

## 2. 目标与非目标
### 2.1 目标（Goals）
- [ ] 在 `/org/assignments` 时间线表格中新增 4 列：职类、职种、职位模板、职级。
- [ ] 每条任职记录按其 `effective_date` 对齐展示对应“职位切片（org_position_slices）”上的职位体系信息。
- [ ] 职种/职类使用“职位模板默认分配（org_job_profile_job_families）”的 `is_primary=TRUE` 作为展示来源（单值，避免多值 UI 歧义）。
- [ ] 缺失值降级：`job_level_code` 为空时显示空值（或 `—`），但不影响页面渲染。
- [ ] 不引入新的数据库表/迁移；仅使用现有 DEV-PLAN-072 结构与索引。
- [ ] 与 DEV-PLAN-074（取消百分比分配）兼容：本计划不依赖 allocation_percent，只依赖 primary 关系。

### 2.2 非目标（Non-Goals）
- 不在任职记录页提供这 4 个字段的编辑能力（编辑仍在职位/职位模板相关页面完成）。
- 不调整任职写入口与数据模型（参见 DEV-PLAN-062/061A1 等相关计划）。
- 不新增/变更对外 API（本计划仅涉及 UI 展示与内部查询字段扩展）。

## 2.1 工具链与门禁（SSOT 引用）
本计划命中触发器（细节以 `AGENTS.md` / `Makefile` / CI 为准）：
- [ ] Go 代码（lint + test）
- [ ] `.templ` / Tailwind（表格列与页面渲染变更）
- [ ] 多语言 JSON（新增列名的 i18n key）

SSOT：
- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 数据流（读路径）
`GET /org/assignments`（UI Controller）→ `OrgService.GetAssignments` → Repo 查询返回 `AssignmentViewRow` → Mapper 转为 `OrgAssignmentsTimeline` → templ 表格渲染。

### 3.2 关键决策
- 不做 UI 端 N+1：在 Repo 层通过联表一次性取回新增字段。
- 多职种场景只展示 primary：使用 `org_job_profile_job_families.is_primary=TRUE`，保证单值输出（UI 无歧义）。
- 不新增迁移：仅扩展查询与 UI 展示，不改变 schema。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 读取涉及的表（现有）
- `org_assignments`：任职时间片
- `org_position_slices`：职位切片（含 `job_profile_id`、`job_level_code`）
- `org_job_profiles`：职位模板（code/name）
- `org_job_profile_job_families`：模板→职种关系（取 `is_primary=TRUE`）
- `org_job_families`、`org_job_family_groups`：职种/职类
- `org_job_levels`：职级（通过 `job_level_code` 查 code/name）

### 4.2 不变量与失败路径
- 不变量：任职 `effective_date` 必须命中一个 position slice（现状假设，本计划不改变）。
- 不变量：对单个 `job_profile_id` 存在且仅存在一个 primary job family（本计划仅依赖 primary，不依赖百分比分配；兼容 DEV-PLAN-074）。
- 失败路径/降级：
  - `job_level_code` 为空或找不到 `org_job_levels`：职级列展示空值（或 `—`），页面不 500。
  - `job_profile_id` 存在但取不到 primary job family（历史/脏数据）：职类/职种列为空，页面不 500。

## 5. 接口契约 (UI + HTMX)
### 5.1 页面入口
- `GET /org/assignments?effective_date=YYYY-MM-DD&pernr=...`

### 5.2 HTMX 复用入口（Person 详情页）
- `GET /org/assignments?effective_date=YYYY-MM-DD&pernr=...&include_summary=1`
  - 期望：返回 HTML 片段用于 `#org-assignments-timeline` 的 `innerHTML` 替换，且不依赖页面 layout。

### 5.3 表格列契约（新增）
在 `orgui.AssignmentsTimeline` 的表头新增 4 列：
- `JobFamilyGroup`（职类）
- `JobFamily`（职种）
- `JobProfile`（职位模板）
- `JobLevel`（职级）

## 6. 核心逻辑与算法 (Business Logic)
### 6.1 字段来源（按单条 assignment）
令 `d = assignment.effective_date`，命中 `org_position_slices`（`position_id` + `[effective_date,end_date]` 包含 `d`）：
- 职位模板：`ps.job_profile_id` → `org_job_profiles`
- 职种/职类：`org_job_profile_job_families` 取 `is_primary=TRUE` 的单行 → `org_job_families` → `org_job_family_groups`
- 职级：`ps.job_level_code` → `org_job_levels (tenant_id, code)`

### 6.2 Label 规范
统一展示为 `code — name`；若 `name` 为空，则仅展示 `code`；两者都为空则展示空值（或 `—`）。

## 7. 安全与鉴权 (Security & Authz)
- 复用现有鉴权：`org.assignments` 的 `read`/`assign` 能力控制不变。
- 不新增新的 authz object/action。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- 依赖 DEV-PLAN-072 的 Job Catalog 结构可用（profile/family/group/level）。
- 与 DEV-PLAN-074 的关系：本计划只依赖 primary 关系，074 取消百分比不会破坏本计划。

### 8.2 里程碑
1. [ ] 扩展 Repo 查询：`ListAssignmentsTimeline` + `ListAssignmentsAsOf` 增加联表字段，确保一条 assignment 一条 row。
2. [ ] 扩展 service/viewmodel/mapper：新增字段并拼装 label。
3. [ ] 更新 `.templ` 与 i18n：新增表头 key 与渲染列。
4. [ ] E2E：覆盖 `/org/assignments` 全页与 Person 详情页复用入口。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] `/org/assignments`：表头出现 4 个新列且可正常加载/刷新。
- [ ] Person 详情页：任职经历区块加载的时间线也出现 4 个新列（同一组件复用一致生效）。
- [ ] 至少一条任职行能展示期望的职类/职种/模板/职级 label；缺失值降级不 500。
- [ ] E2E：断言表头存在，且不渲染 UUID 到新增列（避免“看起来像 id”）。

### 9.1 验收样例（最小可重复）
样例数据（建议固定 code，避免歧义）：
- Job Family Group：`G1`（name：管理类）
- Job Family：`F01`（name：全面管理，归属 `G1`）
- Job Profile：`P001`（name：总经理，primary family=`F01`）
- Job Level：`L1`（name：一级）

样例操作（任意方式保证数据存在即可：Job Catalog UI 创建 / seed / import）：
1) 选择一个职位并确保其 position slice：`job_profile_id=P001` 且 `job_level_code=L1`
2) 创建任职记录生效日命中该 position slice

期望展示（同一条任职行）：
- 职类：`G1 — 管理类`
- 职种：`F01 — 全面管理`
- 职位模板：`P001 — 总经理`
- 职级：`L1 — 一级`

## 10. 运维与监控 (Ops & Monitoring)
不新增监控/开关；本计划为 UI 展示增强，不引入额外运维复杂度。
