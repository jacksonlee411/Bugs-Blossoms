# Person（人员）sqlc 工作流

当你修改 Person（人员）表结构或 SQL（含 `sqlc.yaml`、`modules/person/infrastructure/sqlc/**`、`modules/person/infrastructure/persistence/**/*.sql` 等）时，按以下顺序执行：

1. 维护迁移或 schema（以 `modules/person/infrastructure/persistence/schema/person-schema.sql` 为准）。
2. 运行 `scripts/db/export_person_schema.sh` 导出最新 schema（可通过 `SKIP_MIGRATE=1` 跳过自动迁移）。
3. 执行 `make sqlc-generate`（或 `make generate`，它会自动调用该目标）以重建 `modules/person/infrastructure/sqlc/**`。
4. `git status --short` 确认没有遗漏的生成文件，再提交变更。

CI 的 `person-sqlc` 过滤器会在涉及 Person SQL / schema / sqlc.yaml 变更时强制执行上述流程，并拒绝包含未提交生成结果的 PR。
