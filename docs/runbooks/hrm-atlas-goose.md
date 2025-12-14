# HRM Atlas + Goose 流水线

本手册描述 HRM schema 通过 Atlas 生成迁移、由 goose 执行迁移的推荐工作流。

## 基线产物

- 目标 schema：`modules/hrm/infrastructure/atlas/schema.hcl`
- Atlas 配置：`atlas.hcl`（`dev`/`test`/`ci` 环境复用 `DB_*` 环境变量）
- 迁移目录：`migrations/hrm/changes_<unix>.{up,down}.sql`
- 工作日志：`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`

## 推荐流程

1. 安装 Atlas CLI（一次性）：
   ```bash
   make atlas-install
   ```
2. 更新 schema 定义：编辑 `modules/hrm/infrastructure/atlas/schema.hcl`（或补充 include）。
3. Dry-run 计划：
   ```bash
   make db plan
   ```
4. 生成迁移：
   ```bash
   atlas migrate diff --env dev \
     --dir file://migrations/hrm \
     --to file://modules/hrm/infrastructure/atlas/schema.hcl
   ```
5. 应用迁移（goose）：
   ```bash
   make db migrate up HRM_MIGRATIONS=1
   ```
   - 回滚最近一次：`GOOSE_STEPS=1 make db migrate down HRM_MIGRATIONS=1`
   - redo：`GOOSE_STEPS=1 make db migrate redo HRM_MIGRATIONS=1`
   - 状态：`make db migrate status HRM_MIGRATIONS=1`
6. 导出 schema & 重建 sqlc（推荐）：
   ```bash
   scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1
   make sqlc-generate
   ```
7. 提交前检查：
   ```bash
   make db lint
   ```

> 所有 Atlas/Goose 操作请在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md` 补充记录，方便审计与回溯。

