# DEV-PLAN-075A：Job Catalog identity legacy 列退场评估与清理

**状态**: 规划中（2026-01-04 04:22 UTC）

> DEV-PLAN-075 已完成“Job Catalog 主数据切片化（slices SSOT）”与 legacy 表退场（`org_job_profile_job_families`）。  
> 但各 identity 表仍保留 legacy 列（如 `name/is_active/description/display_order/external_refs`），数据库层面仍存在“双 SSOT 形态”，需要单独评估并决定是否物理删除（drop columns）及其安全落地路径。

## 1. 背景与上下文 (Context)
- **现状**：Job Catalog 的展示/校验/联表展示已改为按 `*_slices` as-of 解析（SSOT= slices），identity 表的 legacy 字段理论上不应再被读取。
- **风险**：
  - **隐性依赖**：历史脚本、导入/导出、报表 SQL、调试查询可能仍读取 legacy 列。
  - **维护成本**：长期保留 legacy 列会让未来维护者误以为 identity 为 SSOT，导致“修一处漏一处”的漂移。
  - **约束/索引漂移**：若 legacy 列仍用于索引（如 `tenant_id,is_active,code`），会让“active”语义在 slices 与 identity 间分叉。
- **关联计划**：
  - DEV-PLAN-075：`docs/dev-plans/075-job-catalog-effective-dated-attributes.md`
  - DEV-PLAN-075 Readiness：`docs/dev-records/DEV-PLAN-075-READINESS.md`

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **清点与评估**：列出所有待退场的 identity legacy 列清单，并给出“可删除/需保留/需延期”的结论与证据（代码搜索、SQL 搜索、迁移/脚本/测试覆盖）。
- [ ] **确定最终 Schema**：定义 Job Catalog identity 表的“最小稳定形态”（保留 `id/tenant_id/code/created_at/updated_at` 等必要字段；其余业务字段以 slices 为准）。
- [ ] **安全落地迁移**：落地 Org goose migration（drop columns + drop/调整相关索引/约束），并确保 `make org lint` 通过（含 `atlas.sum` 同步）。
- [ ] **门禁通过**：按 SSOT 执行并通过 `AGENTS.md` 触发器矩阵中命中的门禁；CI 全绿。
- [ ] **文档收口**：更新 DEV-PLAN-075/075A 与 readiness 记录，保证“为什么删、删了什么、如何验证”可追溯。

### 2.2 非目标
- 不新增新的 Job Catalog 维度/表（仅清理 legacy 列；不改变 slices 表结构）。
- 不引入额外运维/监控开关（对齐仓库原则：早期阶段避免过度运维与监控）。
- 不在本计划内重构 Job Catalog 的 API/UI（除非被迫移除对 legacy 列的引用）。

## 2.1 工具链与门禁（SSOT 引用）
> 目的：避免在本文复制命令细节导致 drift；只声明本计划命中的触发器，并引用 SSOT。

- **触发器清单**：
  - [ ] Go 代码（SSOT：`AGENTS.md`）
  - [ ] Org DB 迁移 / Schema（SSOT：`AGENTS.md` + `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [ ] 文档（`make check doc`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 关键决策：identity 表的定位
- **定位（选定）**：identity 表只承担“稳定 identity + 代码索引 + 外键被引用”的角色；所有可变业务属性以 slices 为 SSOT。
- **不变量**：
  - 任何展示/校验必须以 slices as-of 解析（不得回读 identity legacy 字段）。
  - identity 表不得再承载“active/inactive”的业务语义（否则会与 slices 分叉）。

### 3.2 迁移策略选项
- **选项 A（更彻底，目标）**：删除 identity legacy 列，并同步移除依赖这些列的索引/约束；所有读写仅使用 slices。
  - 优点：从根上消除“双 SSOT 形态”，减少长期维护成本。
  - 风险：若存在外部/脚本/报表依赖，drop 会直接破坏。
- **选项 B（更保守）**：保留 legacy 列但标记为 deprecated（文档 + 代码禁止读取），并在后续窗口再 drop。
  - 优点：风险低。
  - 缺点：双 SSOT 继续存在，容易 drift。

本计划的交付是：完成依赖面评估并给出“是否可执行选项 A”的结论；若评估通过，则直接落地选项 A。

## 4. 数据模型与约束 (Data Model & Constraints)
> 下列“候选待退场字段”以 DEV-PLAN-075 的 slices 设计为前提；最终是否 drop 以评估结论为准。

### 4.1 候选：Job Catalog identity 表 legacy 列
- `org_job_family_groups`：`name`, `is_active`
- `org_job_families`：`name`, `is_active`
- `org_job_levels`：`name`, `display_order`, `is_active`
- `org_job_profiles`：`name`, `description`, `is_active`, `external_refs`

### 4.2 关联索引/约束（需同步处理）
- 典型示例：`(tenant_id, is_active, code)` 这类“把 active 语义绑定到 identity”的索引需要删除或改为 `(tenant_id, code)`。

## 5. 评估与实施步骤 (Steps)
### 5.1 依赖面评估（必须先做）
1. [ ] **Repo 全量搜索（代码/SQL/脚本/测试）**：
   - `rg -n \"org_job_(family_groups|families|levels|profiles)\\.name\" --glob '!docs/**'`
   - `rg -n \"org_job_(family_groups|families|levels|profiles)\\.is_active\" --glob '!docs/**'`
   - `rg -n \"org_job_levels\\.display_order\" --glob '!docs/**'`
   - `rg -n \"org_job_profiles\\.description\" --glob '!docs/**'`
   - `rg -n \"org_job_profiles\\.external_refs\" --glob '!docs/**'`
2. [ ] **确认无运行时依赖**：如果存在外部报表/手写 SQL（仓库外），需由负责人确认迁移影响范围与替代查询（否则选项 B 延期）。
3. [ ] **结论落盘**：在本文 6.1 中记录“可删除/需保留/延期”的清单与证据摘要。

### 5.2 实施（仅在评估通过时执行）
1. [ ] 更新 `modules/org/infrastructure/persistence/schema/org-schema.sql`：删除对应列与索引定义，保证 schema SSOT 单一。
2. [ ] 新增 `migrations/org/` goose migration：drop 相关索引 + drop columns（必要时按 Atlas lint 提示添加 `atlas:nolint` 或 pre-check）。
3. [ ] 更新 `migrations/org/atlas.sum`：`atlas migrate hash --dir file://migrations/org --dir-format goose`。
4. [ ] 若存在代码引用，先改为 slices 读路径，再执行 drop migration。

### 5.3 验证与收口
1. [ ] 门禁：
   - `make check doc`
   - Go 门禁（如命中）：`go fmt ./... && go vet ./... && make check lint && make test`
   - Org 门禁：`make org plan && make org lint && make org migrate up`
2. [ ] Readiness：创建并填写 `docs/dev-records/DEV-PLAN-075A-READINESS.md`（记录关键命令/结果与时间戳）。
3. [ ] 更新 DEV-PLAN-075：明确 legacy 列退场已由 075A 承接（避免范围不清）。

## 6. 输出与验收 (Deliverables & Acceptance)
### 6.1 评估结论（本文必须填写）
- [ ] 结论表：每张表/每列 —— `删除 / 保留 / 延期` + 证据链接（`rg` 命中点、相关 PR、或外部依赖确认记录）。

### 6.2 若执行删除（选项 A）
- [ ] DB 层不再存在上述 legacy 列与相关索引/约束。
- [ ] CI 全绿（含 Org Atlas lint）。
- [ ] `rg` 在仓库内不再出现对这些列的运行时代码引用（允许 docs 提及）。

