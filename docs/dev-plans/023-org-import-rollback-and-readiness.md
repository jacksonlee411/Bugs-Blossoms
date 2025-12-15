# DEV-PLAN-023：Org 导入/回滚脚本与 Readiness

**状态**: 规划中（2025-12-13 更新）

## 背景
- 对应 020 步骤 3，承接 021（schema 落地）与 022（占位表/事件契约），需提供导入/导出/回滚脚本雏形并完成 readiness（lint/test），为 024 CRUD 主链上线前做数据与执行面准备。

## 目标
- 提供基于 Go 的 CLI 工具 (`cmd/org-data`)，支持 CSV 数据的导入、导出与回滚，具备完整的 dry-run 校验能力。
- 定义明确的 CSV 数据模板与校验规则（唯一性、层级完整性、时间窗有效性）。
- `make check lint` 与 `go test ./cmd/org-data/... ./modules/org/...`（或相关路径）通过，失败有回滚/清理方案。

## 范围与非目标
- 范围：为 org 主链提供数据工具；覆盖 021/022 已落地的表（`org_nodes`/`org_node_slices`/`org_edges`/`positions`/`org_assignments`；可选扩展到 `org_roles/org_role_assignments/org_attribute_inheritance_rules/change_requests` 的导出）。
- 非目标：不实现最终 UI、审批/流程、矩阵/继承逻辑执行，只提供数据导入与清理脚本；不交付生产级自动化灰度，仅最小可用路径。

## 依赖与里程碑
- 依赖：基于 DEV-PLAN-021 schema 与 DEV-PLAN-022 占位/契约，确保导入数据与现有约束一致；在 024 CRUD 主链前完成 readiness。
- 里程碑（按提交时间填充）：CLI 工具骨架 -> CSV 解析与校验逻辑 -> 数据库交互与回滚 -> Readiness 验证。

## 设计决策
### 1. 工具形态：Go CLI (`cmd/org-data`)
- **原因**：Shell 脚本处理 CSV 和复杂校验（如时间重叠、树环检测）能力不足且难以维护。使用 Go CLI 可复用 `modules/org/domain` 中的逻辑和 `pkg/db` 连接池。
- **命令结构**：
  - `org-data import --tenant <uuid> --input <dir> [--apply] [--strict] [--mode seed|merge] [--backend db|api]`
  - `org-data export --tenant <uuid> --output <dir> [--as-of <date|rfc3339>]`
  - `org-data rollback --tenant <uuid> (--manifest <path> | --since <timestamp>) [--apply]`
- **Shell 包装**：保留 `scripts/org/*.sh` 作为 CLI 的简单包装器，方便运维调用。

### 2. CSV 数据契约
- **格式**：UTF-8，逗号分隔，带 Header。
- **时间字段**：`effective_date/end_date` 支持 `YYYY-MM-DD` 或 RFC3339；`YYYY-MM-DD` 解释为 `00:00:00Z`，统一按 UTC 写入；有效期语义为半开区间 `[effective_date, end_date)`，空 `end_date` 由工具自动补齐（同一实体按 `effective_date` 排序，取下一片段的 `effective_date` 或 `9999-12-31`）。
- **模板定义**：
  - `nodes.csv`: `code` (必填), `type` (OrgUnit), `name`, `i18n_names` (JSON), `status` (active/retired), `effective_date`, `end_date`, `parent_code` (为空表示 root), `display_order`, `manager_user_id` (可选), `manager_email` (可选，优先级低于 manager_user_id)
  - `positions.csv`: `code` (必填), `org_node_code` (必填), `title`, `effective_date`, `end_date`, `is_auto_created`
  - `assignments.csv`: `position_code` (必填), `assignment_type` (primary/matrix/dotted), `effective_date`, `end_date`, `pernr` (必填), `subject_id` (可选，若为空则按 pernr 生成稳定 UUID)
- **映射规则**：
  - `nodes.csv` 每行会落成：`org_nodes`（按 `code` 确保存在稳定 ID）+ `org_node_slices`（属性时间片）+ `org_edges`（父子关系时间片，root 行不写 edge；`parent_hint` 与 edge 在写入前做一致性校验）。
  - 导入时 CLI 需建立内存映射（Code -> UUID），将 CSV 中的 `parent_code/org_node_code` 转换为 DB 中的 `*_node_id`。
  - `assignments.csv` 不支持 024 的“隐式自动创建 Position”，Position 必须显式存在（批量导入口径更严格，避免隐式生成脏数据）。

### 3. 校验与执行逻辑
- **Phase 1: Parse & Static Validate**
  - 检查必填项、日期格式（`effective_date < end_date`）、JSON 格式。
  - 内存中构建树并做环路检测：默认按最小 `effective_date` 的快照检查；`--strict` 下对所有边界时间点（各行 `effective_date` 的去重集合）逐点做 as-of 检查，避免把“不同时间片的边”误判为同一时点成环。
  - 自动补齐 `end_date` 后，按实体维度检查“无重叠”；`--strict` 下额外检查“无空档”（对 `org_node_slices/org_edges/org_assignments` 默认按约束 1 执行）。
  - 时区一致性：对所有 `YYYY-MM-DD` 输入强制解释为 UTC 的 `00:00:00Z`，并在运行日志中打印规范化后的时间点（避免因运行环境时区差异导致跨天偏移）。
- **Phase 2: DB Dry-Run (Read-Only)**
  - seed 模式：若检测到租户已存在 org 主链数据则直接拒绝。
  - merge 模式：检查 `code` 在租户内是否冲突（若存在则标记为 Update 或 Skip）。
  - 检查 `parent_code` 是否存在（或在当前批次中创建）。
  - 检查时间片重叠（Overlap Check）：查询 DB 中该实体的现有时间片，模拟插入看是否触发 EXCLUDE 约束。
- **Phase 3: Apply (Transactional)**
  - 开启事务（db backend）或调用 API 批量入口（api backend，优先走 `POST /org/api/batch` 以复用 024/026 的校验、审计与事件发布路径）。
  - 按依赖顺序写入：`org_nodes` -> `org_node_slices` -> `org_edges` -> `positions` -> `org_assignments`（`org_edges` 的 `path/depth` 由触发器维护）。
  - 失败则全量回滚。
  - 事件与缓存一致性：
    - **db backend**：不直接触发应用内缓存失效；若在应用运行期间做 merge，需在执行后调用 026 的 `/org/api/snapshot` 或重启应用以完成对账/缓存重建（直到 outbox/relay 落地）。
    - **api backend**：由服务端在同一事务内写入 outbox 并发布 `OrgChanged/OrgAssignmentChanged`（对齐 022），触发下游（Authz/缓存/索引）更新。

### 4. 回滚策略 (Rollback)
- **机制**：M1 先采用 **时间窗口 + 租户** 的最小方案；为降低误删风险，导入默认只支持“空租户/空组织数据”的 seed 模式（若检测到 tenant 已存在 org 主链数据则拒绝，需显式开关才允许 merge）。
- **Manifest（推荐）**：每次 `import --apply` 生成 `import_manifest_<timestamp>.json`，记录本次导入写入/更新的主键集合、生成的 `subject_id(pernr->uuid)` 映射与执行摘要；`rollback --manifest` 以 manifest 为准做精确回滚，适用于 merge 场景。
- **逻辑**：
  - `rollback --since "2025-01-15T12:00:00Z"`
  - 查询所有 `created_at >= since AND tenant_id = target` 的记录。
  - 逆序删除：`org_assignments` -> `positions` -> `org_edges` -> `org_node_slices` -> `org_nodes`。
  - **安全网**：默认 dry-run（不加 `--apply`）列出将要删除的记录数；执行 `--apply` 前需用户二次确认（输入 `YES`）。

### 5. Readiness 检查集
- **Lint**: `golangci-lint` 覆盖 `cmd/org-data` 及 `modules/org`。
- **Test**:
  - `go test ./cmd/org-data/...`：测试 CSV 解析与 CLI 逻辑。
  - `go test ./modules/org/...`：测试领域校验逻辑。
- **DB Lint**: `atlas migrate lint` 确保 Schema 无破坏性变更。

## 任务清单与验收标准
1. [ ] **CLI 骨架与 CSV 解析**
   - 创建 `cmd/org-data/main.go` 及子命令结构。
   - 实现 CSV Parser，支持上述 `nodes.csv`, `positions.csv` 等模板，解析为 Go Struct。
   - 验收：`go run cmd/org-data/main.go import --help` 可用，能正确解析示例 CSV 并打印 JSON 结构。

2. [ ] **校验逻辑 (Dry-Run)**
   - 实现内存环路检测与时间片重叠预检。
   - 实现 `Code -> UUID` 的解析逻辑（Mock DB 或连接 Dev DB）。
   - 验收：提供含环路/重叠数据的 CSV，运行 `import`（不加 `--apply`）能准确报错并输出错误行号。

3. [ ] **数据库交互与回滚**
   - 集成 `pkg/db`，实现事务写入。
   - 实现 `rollback` 命令，按时间窗查询并生成删除计划。
   - 验收：在本地 DB 执行导入成功；执行回滚后数据被清理；`dry-run` 模式下不产生脏数据。

4. [ ] **Shell 包装与文档**
   - 编写 `scripts/org/import.sh` 等包装脚本。
   - 更新 README，提供 CSV 模板下载链接或示例内容。
   - 验收：`scripts/org/import.sh` 可直接调用，文档清晰。

5. [ ] **Readiness 验证**
   - 执行 `make check lint`。
   - 执行 `go test ./modules/org/... ./cmd/org-data/...`。
   - 记录执行结果到 `docs/dev-records/DEV-PLAN-023-READINESS.md`。

## 验证记录
- 在 `docs/dev-records/DEV-PLAN-023-READINESS.md` 中记录：
  - `org-data import`（不加 `--apply`）的输出日志（包含校验失败的示例）。
  - `org-data rollback` 的执行日志。
  - `make check lint` 和 `go test` 的覆盖率报告。

## 风险与回滚/降级路径
- **数据污染风险**：CLI 默认 dry-run（不加 `--apply`），只有显式传入 `--apply` 才执行写入；写入失败必须整事务回滚。
- **回滚误删风险**：merge 场景必须优先使用 `rollback --manifest`；`--since` 仅作为 seed 的兜底手段，且不保证恢复“被更新记录”的旧值（后续可通过 025 的审计/request_id 或引入 batch/run 追踪表增强可回放性）。

## 交付物
- `cmd/org-data` 源码及 `scripts/org/*.sh` 包装脚本。
- CSV 数据模板示例（`docs/samples/org/*.csv`）。
- Readiness 验证报告 (`docs/dev-records/DEV-PLAN-023-READINESS.md`)。
