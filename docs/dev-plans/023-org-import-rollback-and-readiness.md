# DEV-PLAN-023：Org 导入/回滚脚本与 Readiness

**状态**: 规划中（2025-01-15 14:00 UTC）

## 背景
- 对应 020 步骤 3，承接 021（schema 落地）与 022（占位表/事件契约），需提供导入/导出/回滚脚本雏形并完成 readiness（lint/test），为 024 CRUD 主链上线前做数据与执行面准备。

## 目标
- 提供基于 Go 的 CLI 工具 (`cmd/org-data`)，支持 CSV 数据的导入、导出与回滚，具备完整的 dry-run 校验能力。
- 定义明确的 CSV 数据模板与校验规则（唯一性、层级完整性、时间窗有效性）。
- `make check lint` 与 `go test ./modules/org/...`（或相关路径）通过，失败有回滚/清理方案。

## 范围与非目标
- 范围：为 org 主链提供数据工具；覆盖 021/022 已落地的表（org_nodes/edges/positions/assignments）。
- 非目标：不实现最终 UI、审批/流程、矩阵/继承逻辑执行，只提供数据导入与清理脚本；不交付生产级自动化灰度，仅最小可用路径。

## 依赖与里程碑
- 依赖：基于 DEV-PLAN-021 schema 与 DEV-PLAN-022 占位/契约，确保导入数据与现有约束一致；在 024 CRUD 主链前完成 readiness。
- 里程碑（按提交时间填充）：CLI 工具骨架 -> CSV 解析与校验逻辑 -> 数据库交互与回滚 -> Readiness 验证。

## 设计决策
### 1. 工具形态：Go CLI (`cmd/org-data`)
- **原因**：Shell 脚本处理 CSV 和复杂校验（如时间重叠、树环检测）能力不足且难以维护。使用 Go CLI 可复用 `modules/org/domain` 中的逻辑和 `pkg/db` 连接池。
- **命令结构**：
  - `org-data import --tenant <uuid> --file <path> [--dry-run] [--strict]`
  - `org-data export --tenant <uuid> --output <dir>`
  - `org-data rollback --tenant <uuid> --since <timestamp> [--dry-run]`
- **Shell 包装**：保留 `scripts/org/*.sh` 作为 CLI 的简单包装器，方便运维调用。

### 2. CSV 数据契约
- **格式**：UTF-8，逗号分隔，带 Header。
- **模板定义**：
  - `nodes.csv`: `code` (必填), `type` (OrgUnit), `name`, `i18n_names` (JSON), `effective_date` (YYYY-MM-DD), `end_date`, `parent_code` (用于构建 Edge), `display_order`
  - `positions.csv`: `code` (必填), `org_node_code` (必填), `title`, `effective_date`, `end_date`, `is_auto_created`
  - `assignments.csv`: `person_id` (必填), `position_code` (必填), `type` (primary), `effective_date`, `end_date`
- **引用解析**：导入时 CLI 需建立内存映射（Code -> UUID），将 CSV 中的 `parent_code` 转换为 DB 中的 `parent_node_id`。

### 3. 校验与执行逻辑
- **Phase 1: Parse & Static Validate**
  - 检查必填项、日期格式（`effective_date < end_date`）、JSON 格式。
  - 内存中构建树，检查是否存在**环路**（Cycle Detection）。
- **Phase 2: DB Dry-Run (Read-Only)**
  - 检查 `code` 在租户内是否冲突（若存在则标记为 Update 或 Skip）。
  - 检查 `parent_code` 是否存在（或在当前批次中创建）。
  - 检查时间片重叠（Overlap Check）：查询 DB 中该实体的现有时间片，模拟插入看是否触发 EXCLUDE 约束。
- **Phase 3: Apply (Transactional)**
  - 开启事务。
  - 按拓扑顺序写入：Nodes -> Edges -> Positions -> Assignments。
  - 失败则全量回滚。

### 4. 回滚策略 (Rollback)
- **机制**：由于 021 Schema 未引入 `batch_id`，回滚依赖 **时间窗口 + 租户**。
- **逻辑**：
  - `rollback --since "2025-01-15T12:00:00Z"`
  - 查询所有 `created_at >= since AND tenant_id = target` 的记录。
  - 逆序删除：Assignments -> Positions -> Edges -> Nodes。
  - **安全网**：必须先执行 `--dry-run` 列出将要删除的记录数，需用户二次确认（输入 `YES`）。

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
   - 验收：提供含环路/重叠数据的 CSV，运行 `import --dry-run` 能准确报错并输出错误行号。

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
  - `org-data import --dry-run` 的输出日志（包含校验失败的示例）。
  - `org-data rollback` 的执行日志。
  - `make check lint` 和 `go test` 的覆盖率报告。

## 风险与回滚/降级路径
- **数据污染风险**：CLI 必须默认开启 `--dry-run`，只有显式传入 `--apply` 才执行写入。
- **回滚误删风险**：`rollback` 命令必须强制要求 `--dry-run` 预览，并要求用户输入随机生成的验证码或固定字符串（如 "CONFIRM"）才能执行删除。

## 交付物
- `cmd/org-data` 源码及 `scripts/org/*.sh` 包装脚本。
- CSV 数据模板示例（`docs/samples/org/*.csv`）。
- Readiness 验证报告 (`docs/dev-records/DEV-PLAN-023-READINESS.md`)。
