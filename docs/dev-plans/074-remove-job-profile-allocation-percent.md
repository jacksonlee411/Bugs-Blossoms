# DEV-PLAN-074：取消职位模板职种“百分比分配”（保留多职种配置）

**状态**: 规划中（2025-12-31 10:48 UTC）

## 1. 背景与上下文 (Context)
### 1.1 现状（DEV-PLAN-072）
DEV-PLAN-072 引入 Job Architecture 后，职位模板（Job Profile）与职种（Job Family）之间存在“默认分配”关系：`org_job_profile_job_families.allocation_percent`，并由数据库约束强制：
- allocation 之和必须为 100
- 必须且只能有一个 primary

同时，职位切片（Position Slice）侧也存在 `org_position_slice_job_families.allocation_percent` 与同类 sum=100 校验，用于表达“岗位职种分摊”。

### 1.2 需求
取消“职位模板/职位切片”的百分比分配输入与约束，但仍保留一个职位模板可配置多个职种，并保留主职种（primary）用于 UI 展示与默认选择。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] Job Profile 可配置多个 Job Family，但不再维护 `allocation_percent`。
- [ ] Job Profile 与 Position Slice 均保留 `is_primary`，并保证“恰好一个 primary”（避免 UI 歧义）。
- [ ] 移除所有 “sum=100” 相关的 DB 触发器/函数/校验与 UI 输入项。
- [ ] 兼容存量数据：迁移不丢失“多职种集合”，primary 以现有 `is_primary` 为准。

### 2.2 非目标
- 不引入新的 Job Catalog 维度（不新增表/不新增概念）。
- 不在本计划中重做报表/成本分摊算法；若未来仍需要比例，必须单独 dev-plan 定义业务含义与口径。
- 不改变 Valid Time 口径（参见 DEV-PLAN-064）。

## 2.3 工具链与门禁（SSOT 引用）
本计划命中触发器（细节以 `AGENTS.md` / `Makefile` 为准）：
- [ ] Go 代码（lint + test）
- [ ] `.templ` / Tailwind（移除百分比输入、调整交互）
- [ ] 多语言 JSON（文案变化）
- [ ] Org DB 迁移（`migrations/org/**`，按 Org Atlas+Goose 门禁执行）

SSOT：
- 触发器矩阵：`AGENTS.md`
- 命令入口：`Makefile`
- Org Atlas+Goose 工具链：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 关键决策：范围选择（Simple > Easy）
为避免“Profile 没有比例但 Slice 仍要求 sum=100”导致默认复制/校验出现偶然复杂度，本计划选择：
- **方案 A（确定）：同时移除 Job Profile 与 Position Slice 的 `allocation_percent` 与 sum=100 校验，仅保留集合 + primary。**

### 3.2 不变量（删除比例后系统必须始终成立）
- 不变量：对每个 `job_profile_id`，其 job families 集合必须在事务提交时满足 `primary_count == 1`（允许中间过程，最终必须成立）。
- 不变量：对每个 `position_slice_id`，其 job families 集合必须在事务提交时满足 `primary_count == 1`。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 目标 schema（列级别口径）
#### 4.1.1 `org_job_profile_job_families`
- 保留：`tenant_id`, `job_profile_id`, `job_family_id`, `is_primary`, `created_at`, `updated_at`
- 删除：`allocation_percent`
- 约束：
  - PK：`(tenant_id, job_profile_id, job_family_id)`（保持）
  - Unique：`(tenant_id, job_profile_id) WHERE is_primary=TRUE`（保持，确保最多一个 primary）
  - Deferrable trigger：提交时校验 `primary_count==1`（替代原 sum=100+primary=1 校验）

#### 4.1.2 `org_position_slice_job_families`
- 保留：`tenant_id`, `position_slice_id`, `job_family_id`, `is_primary`, `created_at`, `updated_at`
- 删除：`allocation_percent`
- 约束：
  - PK：`(tenant_id, position_slice_id, job_family_id)`（保持）
  - Unique：`(tenant_id, position_slice_id) WHERE is_primary=TRUE`（保持）
  - Deferrable trigger：提交时校验 `primary_count==1`

### 4.2 Schema SSOT
除 goose migration 外，必须同步更新 `modules/org/infrastructure/persistence/schema/org-schema.sql`（否则 `make org plan/lint` 会 drift）。

## 5. 迁移策略（Org Goose）
> 移除列属于破坏性变更；本计划默认“迁移 Up 可重复执行、Down 尽力恢复结构但不可恢复历史比例数据”。

### 5.1 Up（精确步骤）
- [ ] 删除旧 trigger/函数（sum=100 + primary=1）：
  - `org_job_profile_job_families_validate` / `org_job_profile_job_families_validate_trigger`
  - `org_position_slice_job_families_validate` / `org_position_slice_job_families_validate_trigger`
- [ ] `ALTER TABLE ... DROP COLUMN allocation_percent`（两张表）
- [ ] 新增新的 deferrable constraint trigger/函数：
  - 仅校验 `primary_count == 1`（不再校验 sum）
  - 触发时机：`AFTER INSERT OR UPDATE OR DELETE ... DEFERRABLE INITIALLY DEFERRED`

### 5.2 Down（边界声明）
- [ ] Down 仅“尽力恢复结构”：
  - 重新添加 `allocation_percent`（可 NULL；不恢复历史值）
  - 不强制恢复 sum=100 校验（否则可能与无比例时期产生的数据冲突）

## 6. 接口契约 (API + UI)
### 6.1 UI（Job Catalog / Profiles）
- 移除“比例（%）”输入与显示列，仅保留：
  - 多职种选择（多选）
  - primary 选择（单选）
- 保存时的校验失败路径：
  - 422：缺少 primary 或 primary 不唯一（应映射为表单错误而非 500）

### 6.2 JSON API（如存在内部调用）
当前 repo 内 Job Catalog JSON API/DTO 含 `allocation_percent`（用于创建/更新/读取）。
- 本计划将其视为 **breaking change（仓库内同步升级）**，且**不提供兼容/版本化策略**：
  - Request/Response 中直接移除 `allocation_percent`
  - 在同一个变更集中更新所有仓库内调用方与测试（含 e2e/集成测试）
  - 说明：若存在仓库外消费者，将被直接破坏；本计划不处理向后兼容

## 7. 核心逻辑 (Business Logic)
### 7.1 服务层校验
将校验从 “sum=100 且 primary=1” 收敛为：
- 至少 1 条 job family
- primary_count == 1

### 7.2 默认复制逻辑（Profile → Position Slice）
`CopyJobProfileJobFamiliesToPositionSlice` 改为复制集合 + primary 标记，不再复制比例字段。

## 8. 安全与鉴权 (Security & Authz)
不引入新的权限点；复用现有 `org.job_catalog`（或对应 object）授权逻辑。

## 9. 测试与验收标准 (Acceptance Criteria)
### 9.1 验收标准
- [ ] Job Catalog（Profiles）创建/编辑：可配置多个职种并保存；无需填写百分比。
- [ ] 对每个 profile/position slice，提交时保证恰好一个 primary；缺 primary/多 primary 返回 422（或 DB 约束错误映射为表单错误）。
- [ ] 不再出现与 `allocation_percent` / “sum=100”相关的数据库错误与 UI 文案。
- [ ] 通过门禁（见 `AGENTS.md` / `Makefile` SSOT），并确保 `migrations/org/atlas.sum` 同步更新。

### 9.2 验收样例（最小可重复）
样例数据（固定 code）：
- Group：`G1`
- Families：`F01`, `F02`
- Profile：`P001`（关联 `F01`/`F02`，primary=`F01`）

期望：
- 可保存且无需输入百分比。
- 再次编辑切换 primary（`F02`）可保存。
- DB 中仅存在一个 `is_primary=TRUE`。

## 10. 实施步骤 (Milestones)
1. [ ] 设计并落地 Org migration（含 atlas.sum 更新与 `make org lint`）。
2. [ ] 更新 schema SSOT（`modules/org/.../org-schema.sql`）。
3. [ ] 更新 Repo/Service：删除 allocation 字段与校验/复制逻辑。
4. [ ] 更新 `.templ` + i18n：移除百分比输入/显示与相关 key。
5. [ ] 更新测试：集成测试 + E2E 覆盖“多职种 + primary”保存。
6. [ ] Readiness：按触发器矩阵执行并记录关键结果（如需要 dev-record 则补齐）。

## 11. Simple > Easy Review（DEV-PLAN-045）评审结论
> 结论：**准备就绪**。本计划明确接受 breaking change，且不引入兼容/版本化层；以“仓库内一次性同步升级”为交付边界（若存在仓库外消费者将被直接破坏）。
