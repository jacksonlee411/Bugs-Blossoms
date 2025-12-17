# HRM Atlas + Goose 流水线

本手册描述 HRM schema 通过 Atlas 做质量门禁（plan/lint）、由 goose 执行迁移的推荐工作流。

## 基线产物

- Schema source：`atlas.hcl` 的 `src`（当前为 `modules/hrm/infrastructure/atlas/core_deps.sql` + `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql`）
- Atlas 配置：`atlas.hcl`（`dev`/`test`/`ci` 环境复用 `DB_URL`/`ATLAS_DEV_URL`，由 `make db plan/lint` 负责组装）
- 迁移目录：`migrations/hrm/`（goose 格式，数字前缀版本号，例如 `00001_hrm_baseline.sql`）
- 工作日志：`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`

## 推荐流程

1. 安装 Atlas CLI（一次性）：
   ```bash
   make atlas-install
   ```
2. 安装 goose CLI（一次性）：
   ```bash
   make goose-install
   ```
3. 更新 HRM schema（目标结构）：编辑 `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql`（必要时同步更新 `modules/hrm/infrastructure/atlas/core_deps.sql` 的最小 stub）。
4. Dry-run 计划（Atlas）：
   ```bash
   make db plan
   ```
5. 编写/更新 goose 迁移（HRM 专用目录）：在 `migrations/hrm/` 新增迁移文件（goose 单文件 Up/Down 格式，版本号递增）。
6. 应用迁移（goose）：
   ```bash
   make db migrate up HRM_MIGRATIONS=1
   ```
   - 回滚最近一次：`GOOSE_STEPS=1 make db migrate down HRM_MIGRATIONS=1`
   - redo：`GOOSE_STEPS=1 make db migrate redo HRM_MIGRATIONS=1`
   - 状态：`make db migrate status HRM_MIGRATIONS=1`
7. 导出 schema & 重建 sqlc（推荐）：
   ```bash
   scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1
   make sqlc-generate
   ```
8. 提交前检查（Atlas lint）：
   ```bash
   make db lint
   ```

> 所有 Atlas/Goose 操作请在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md` 补充记录，方便审计与回溯。
