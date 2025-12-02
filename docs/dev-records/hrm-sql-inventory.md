# HRM SQL Inventory

> 依据 DEV-PLAN-010 要求，用于跟踪 HRM 模块现有 SQL 查询/命令，并标记迁移状态与负责人。

## 摘要
- 聚焦 `modules/hrm/infrastructure/persistence` 目录现有仓储。
- 聚合范围：`employee`, `position`（后续可追加 attendance/payroll 等）。
- 状态列：`legacy` 表示仍由手写 `sqlx/pgx` 承载，`sqlc` 表示已由 sqlc 生成。

## 明细

| 聚合 | 文件 | Atlas Schema File | Latest HRM Migration | 语句标识 | 类型 | 用途/备注 | 状态 | Owner |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| employee | modules/hrm/infrastructure/sqlc/employee/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employees`） | migrations/hrm/1744627696_multitenant.up.sql | `ListEmployeesPaginated` | SELECT | 员工分页列表 + meta join | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employees`） | migrations/hrm/1744627696_multitenant.up.sql | `ListEmployeesByTenant` | SELECT | 全量员工列表 | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employees`） | migrations/hrm/1744627696_multitenant.up.sql | `GetEmployeeByID` | SELECT | 员工详情（含 meta） | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employees`） | migrations/hrm/1744627696_multitenant.up.sql | `CountEmployees` | SELECT | 统计租户员工数量 | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employee_meta`） | migrations/hrm/1740741698_changes.up.sql | `CreateEmployee` / `CreateEmployeeMeta` | INSERT | 创建员工主表 + meta | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employee_meta`） | migrations/hrm/1744627696_multitenant.up.sql | `UpdateEmployee` / `UpdateEmployeeMeta` | UPDATE | 更新主表 + meta | sqlc | HRM Platform |
| employee | modules/hrm/infrastructure/sqlc/employee/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`employee_meta`） | migrations/hrm/1740741698_changes.up.sql | `DeleteEmployee` / `DeleteEmployeeMeta` | DELETE | 删除主表 + meta | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1744627696_multitenant.up.sql | `ListPositionsPaginated` | SELECT | 职位分页列表 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1744627696_multitenant.up.sql | `ListPositionsByTenant` | SELECT | 职位全量列表 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1744627696_multitenant.up.sql | `GetPositionByID` | SELECT | 职位详情 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/queries.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1744627696_multitenant.up.sql | `CountPositions` | SELECT | 统计租户职位数量 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1740741698_changes.up.sql | `CreatePosition` | INSERT | 创建职位 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1744627696_multitenant.up.sql | `UpdatePosition` | UPDATE | 更新职位信息 | sqlc | HRM Platform |
| position | modules/hrm/infrastructure/sqlc/position/commands.sql | modules/hrm/infrastructure/atlas/schema.hcl（`positions`） | migrations/hrm/1740741698_changes.up.sql | `DeletePosition` | DELETE | 删除职位 | sqlc | HRM Platform |

## PoC 范围确认
- 首批聚合：`employee`（全 CRUD）+ `position`（辅助 CRUD）。
- 验收：至少覆盖一个带 meta join 的读、一个事务性写（employee create/update）、一个列表/报表查询（employee + filter）。
- 依赖：`pkg/composables`、`pkg/repo`、`pkg/serrors`、`github.com/jackc/pgx/v5`，允许在 sqlc 生成包中引用。

后续变更 HRM SQL 时需同步更新该文件，CI 将通过 `hrm-sqlc` 过滤器提醒缺失（见 DEV-PLAN-010）。
