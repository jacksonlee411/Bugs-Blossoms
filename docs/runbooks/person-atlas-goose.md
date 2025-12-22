# Person（人员）Atlas + Goose 流水线

本手册描述 Person（人员）schema 通过 Atlas 做质量门禁（plan/lint）、由 goose 执行迁移的推荐工作流。

## 基线产物

- Schema source：`atlas.hcl` 的 `src`（当前为 `modules/person/infrastructure/atlas/core_deps.sql` + `modules/person/infrastructure/persistence/schema/person-schema.sql`）
- Atlas 配置：`atlas.hcl`（`dev`/`test`/`ci` 环境复用 `DB_URL`/`ATLAS_DEV_URL`，由 `make db plan/lint` 负责组装）
- 迁移目录：`migrations/person/`（goose 格式，数字前缀版本号，例如 `00001_person_baseline.sql`）
- 工作日志：`docs/dev-records/DEV-PLAN-061-READINESS.md`

## 推荐流程

1. 安装 Atlas CLI（一次性）：
   ```bash
   make atlas-install
   ```
2. 安装 goose CLI（一次性）：
   ```bash
   make goose-install
   ```
3. 更新 Person schema（目标结构）：编辑 `modules/person/infrastructure/persistence/schema/person-schema.sql`（必要时同步更新 `modules/person/infrastructure/atlas/core_deps.sql` 的最小 stub）。
4. Dry-run 计划（Atlas）：
   ```bash
   make db plan
   ```
5. 编写/更新 goose 迁移（Person 专用目录）：在 `migrations/person/` 新增迁移文件（goose 单文件 Up/Down 格式，版本号递增）。
6. 应用迁移（goose）：
   ```bash
   PERSON_MIGRATIONS=1 make db migrate up
   ```
   - 回滚最近一次：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate down`
   - redo：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate redo`
   - 状态：`PERSON_MIGRATIONS=1 make db migrate status`
7. 导出 schema & 重建 sqlc（推荐）：
   ```bash
   scripts/db/export_person_schema.sh SKIP_MIGRATE=1
   make sqlc-generate
   ```
8. 提交前检查（Atlas lint）：
   ```bash
   make db lint
   ```

> 所有 Atlas/Goose 操作请在对应 Dev-Plan 的 readiness 记录补充命令与结果，方便审计与回溯。
