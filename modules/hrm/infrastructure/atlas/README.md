# HRM Atlas / Goose

本仓库对 HRM 使用 Atlas 做质量门禁（plan/lint），使用 goose 执行迁移（`migrations/hrm/*.sql`，goose 格式）。

- Schema source：`atlas.hcl` 的 `src` 直接引用 SQL：
  - `core_deps.sql`：HRM 外键依赖的最小 Core stub（用于 Atlas plan/lint 的自包含校验）。
  - `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql`：HRM 目标结构。
- 迁移目录：`migrations/hrm/`（文件名以数字前缀作为版本号，例如 `00001_*.sql`）。
- 注意：`migrations/hrm/00001_hrm_baseline.sql` 在 Up 段落会为 `tenants/uploads/currencies` 做最小化 `IF NOT EXISTS` stub，以保证 HRM 迁移在“干净数据库”上可执行（CI/PoC）。
