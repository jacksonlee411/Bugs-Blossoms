# DEV-PLAN-010 HRM sqlc PoC 记录

**日期**: 2025-12-02

## 动作概览
1. 在 `modules/hrm/infrastructure/sqlc/` 建立 `employee`、`position` 聚合的 SQL（含 `queries.sql`/`commands.sql`）。
2. 通过 `scripts/db/export_hrm_schema.sh` 导出 HRM 相关表的 schema，并固定到 `modules/hrm/infrastructure/sqlc/schema.sql`。
3. 执行 `make sqlc-generate`（内部运行 `sqlc generate -f sqlc.yaml && gofmt/goimports`），生成 `employee_sqlc` / `position_sqlc` Go 包。
4. 重写 `employee_repository.go`、`position_repository.go`，改为调用生成代码，旧的手写 SQL 常量全部删除。
5. 运行 `go test ./modules/hrm/...` 验证仓储→服务层最小行为保持一致。

## 性能与类型安全对比
- **类型安全**：sqlc 根据 SQL 精确生成 `struct` 与参数类型，避免手写 `Scan` 时漏字段或类型不符的问题；IDE 可直接跳转至生成函数。
- **性能**：生成的查询仍是原始 SQL，`pgx/v5` Prepared Statement + `emit_prepared_queries` 保持与旧实现同级的执行计划，无额外 ORM 开销。
- **维护成本**：目录规范 + `hrm-sql-inventory.md` 让查询位置集中，审查 SQL 变更无需在仓储中搜索字符串。

## 遇到的问题 & 解决
| 问题 | 说明 | 解决方式 |
| --- | --- | --- |
| `schema.sql` 未同步造成字段缺失 | 早期忘记导出 `employee_contacts`，`sqlc generate` 报错表不存在 | 强制执行 `scripts/db/export_hrm_schema.sh`，并在 README/CONTRIBUTING 中写明流程 |
| goimports 缺失 | CI 环境未必预装 goimports | `make sqlc-generate` 改用 `go run golang.org/x/tools/cmd/goimports@v0.26.0`，无需全局安装 |

## 回滚策略
- 若 sqlc 生成代码出现错误，可切回 `git` 之前的 commit（旧仓储实现仍在历史中）。
- 任何 SQL 变动都在 `modules/hrm/infrastructure/sqlc/*.sql` 中，可用 `git diff` 直接比较。
- `hrm-sql-inventory.md` 作为索引，若需回退，可按文件表格恢复对应 SQL/Go。

---
若需要扩展至其他聚合，重复上述流程并在 `docs/dev-records/hrm-sql-inventory.md` 添加条目，CI 的 `hrm-sqlc` 过滤器会强制执行 `make sqlc-generate`。
