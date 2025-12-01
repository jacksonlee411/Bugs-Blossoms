# DEV-PLAN-011：Atlas + goose 联动基线

**状态**: 规划中（2025-12-05 15:00）

## 背景
- DEV-PLAN-009 的第二项重点任务要求以 Atlas diff 生成 up/down SQL，再交由 goose/golang-migrate 执行，形成可重复的迁移工作流（`docs/dev-plans/009-r200-tooling-alignment.md:21-49`）。
- DEV-PLAN-010 已在 HRM 模块交付 sqlc 基线、`sqlc.yaml`、`scripts/db/export_hrm_schema.sh` 等资产（`docs/dev-plans/010-sqlc-baseline.md:5-40`），但迁移仍靠手写 SQL，schema 来源也未与 Atlas 串联。
- R200 工具链路线图需要在 sqlc 基线之上建立声明式 schema → 迁移生成 → goose 执行 → schema 导出 → sqlc generate 的闭环，让 HRM 模块成为 Atlas + goose 的首个 PoC，再向其他模块推广。
- 本计划限定在 HRM 范围（positions、employees 等表及其共享基础设施），通过 Atlas 让数据库 schema 与 sqlc/schema.sql 保持一致，并补齐 Makefile/CI/文档。

## 目标
- 产出 HRM 专用的 Atlas 声明式 schema（`schema.hcl`）与 `atlas.hcl` 配置，覆盖当前 HRM 表结构并内置 lint/plan 规则。
- 将 Atlas diff 生成的迁移脚本纳入 `migrations/`，由 goose 执行并与现有 `make db migrate up`、`scripts/db/export_hrm_schema.sh`、`sqlc generate` 串联。
- 在 Makefile 与 `quality-gates` workflow 中增加 `make db plan/apply`、Atlas lint、git 状态检查，确保 CI 能阻止缺失迁移/导出的 PR。
- 更新 README/CONTRIBUTING/AGENTS/dev-records，记录 Atlas→goose→sqlc 的操作顺序、回滚策略与常见故障，保障团队协作。

## 风险
- Atlas schema 与历史 goose 迁移存在漂移，若未及时回填声明式定义，diff 可能生成错误的 up/down。
- CLI 版本漂移会导致本地/CI 计划不一致，需要在 `tools.go` 固定 `ariga.io/atlas/cmd/atlas` 版本并同步安装方式。
- HRM 模块之外暂不接入 Atlas；若误改其他模块的迁移目录，易破坏冻结模块，需通过过滤器限制影响范围。
- 迁移执行顺序改变可能影响现有数据，需要在 PoC 阶段准备回滚脚本与验证用例。

## 实施步骤
1. **[ ] Schema 基线与目录规划**
   - 对齐 HRM 现有表（positions、employees、employee_meta、employee_positions、employee_contacts），以 `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql` 与 `migrations/changes-*.sql` 为依据，梳理字段/索引/约束，记录缺失的声明式信息。
   - 确定 Atlas schema 文件目录（例如 `modules/hrm/infrastructure/atlas/schema.hcl`）与命名规范，同时明确迁移输出目录（沿用 `migrations/changes-*.sql` 或新增 `migrations/hrm/` 子目录），并在本文档与 CONTRIBUTING 标注。
2. **[ ] Atlas 配置与工具链**
   - 在仓库根目录新增 `atlas.hcl`，声明 dev 数据库连接、迁移目录、lint 规则、格式化策略，并支持环境变量（DB_HOST/DB_PORT 等）以复用 `scripts/db/export_hrm_schema.sh` 的配置。
   - 在 `tools.go` 添加 `_ "ariga.io/atlas/cmd/atlas"`（固定版本），Makefile 新增 `atlas-install`、`db plan`、`db apply`、`db lint` 等目标，同时确保 `make db migrate up` 仍由 goose 执行。
3. **[ ] 迁移生成与 goose 串联**
   - 规范“修改 schema → 更新 schema.hcl → 运行 `atlas migrate diff --dir migrations --to file:///modules/hrm/.../schema.hcl`”的流程，产出的 up/down SQL 需符合 goose 命名约定。
   - 更新现有 `make db migrate up` / `scripts/db/export_hrm_schema.sh` / `make sqlc-generate` 顺序，形成官方指引：`atlas migrate diff → goose up → export_hrm_schema.sh → sqlc generate`，并在 dev-records 中记录 PoC 验证日志。
4. **[ ] CI Guardrail**
   - 在 `.github/workflows/quality-gates.yml` 增加 `hrm-atlas` 过滤器（命中 `atlas.hcl`、`modules/hrm/infrastructure/atlas/**`、`migrations/hrm/**`、`scripts/db/export_hrm_schema.sh` 等），触发 `atlas migrate lint`、`make db plan`、`git status --short` 检查。
   - 与 `hrm-sqlc` 过滤器联动：若两者同时命中，CI 需要顺序执行 `make db plan` → `make db migrate up`（dry-run）→ `make sqlc-generate`，确保 schema/sqlc 同步。
5. **[ ] 文档与回滚策略**
   - 更新 README/CONTRIBUTING/AGENTS 中的 HRM 部分，新增“Atlas + goose”章节：安装、常用命令、目录说明、如何在冲突时回滚、如何验证 goose 与 Atlas 的 state。
   - 在 `docs/dev-records/` 编写 DEV-PLAN-011 PoC 日志（命令输出、问题记录、性能对比），并追加到 `hrm-sql-inventory.md` 以展示每个表的 Atlas 状态。

## 里程碑
- M1：`schema.hcl` 与 `atlas.hcl` 初稿合并，工具版本固定，开发者可运行 `make db plan` 获得无差异结果。
- M2：首个 HRM 迁移通过 Atlas diff 生成并由 goose 执行，串联 `export_hrm_schema.sh` 与 `sqlc generate` 验证通过（含测试/验证日志）。
- M3：`quality-gates` 中的 `hrm-atlas` 检查上线，README/CONTRIBUTING/AGENTS 更新完毕，PoC 经验沉淀至 dev-records。

## 交付物
- `atlas.hcl`、`modules/hrm/infrastructure/atlas/schema.hcl`（及相关 include 文件）。
- 更新后的 `tools.go`、Makefile `db plan/apply` 目标、`scripts/db/export_hrm_schema.sh` 联动说明。
- 新增/更新的 goose 迁移文件（位于 `migrations/` 或 `migrations/hrm/`）。
- README/CONTRIBUTING/AGENTS 的 Atlas 流程章节，以及 `docs/dev-records` 中的 PoC/验证日志。
