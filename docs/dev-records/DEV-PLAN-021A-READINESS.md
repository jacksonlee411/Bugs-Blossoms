# DEV-PLAN-021A：Org Atlas+Goose 工具链与 CI 门禁 Readiness 记录

该记录用于 DEV-PLAN-021A（`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）实施过程中的可追溯验证：每次关键变更（atlas env / make 入口 / CI filter / lint 口径）都应在此记录执行命令与结果。

配套计划：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

## 环境信息（填写）
- 日期（UTC）：2025-12-17
- 分支 / PR：`feature/dev-plan-021-impl`
- Git Revision：`b42ee8f2`
- 数据库：
  - Postgres 版本：17.7（Docker 临时容器验证）
  - DB_NAME / ATLAS_DEV_DB_NAME：`iota_erp_org_atlas` / `org_dev`（端口 55432）

## 门禁与命令记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `ATLAS_DEV_DB_NAME=org_dev make org plan DB_PORT=55432 DB_NAME=iota_erp_org_atlas` | 输出 Org schema diff（不落盘） | 输出 CREATE TABLE/INDEX/CONSTRAINT | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `ATLAS_DEV_DB_NAME=org_dev make org lint DB_PORT=55432 DB_NAME=iota_erp_org_atlas` | `atlas migrate lint --env org_ci` 通过 | 无输出 | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `GOOSE_TABLE=goose_db_version_org make org migrate up DB_PORT=55432 DB_NAME=iota_erp_org_atlas` | goose smoke 通过，且使用独立版本表 | 迁移到 version 2 | 通过 |
| 2025-12-17 14:55 UTC | 本地 | `git status --porcelain` | 为空（生成物已提交） | 有变更（`migrations/org/atlas.sum` 等需提交） | 待提交 |
