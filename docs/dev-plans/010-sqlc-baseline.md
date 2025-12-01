# DEV-PLAN-010：sqlc 数据访问生成基线

**状态**: ✅ 已完成（2025-12-02 10:00）

## 背景
- `docs/dev-records/R200r-Go语言ERP系统最佳实践.md:207-320` 建议 ERP 优先采用 SQL-first 的 sqlc，在编译期生成类型安全的数据访问层，避免 ORMapper 运行期开销。
- 仓库当前以手写 `sqlx` 查询为主，SQL 分散在 `modules/*/infrastructure` 与 `pkg` 目录，缺乏统一的声明位置与生成脚本，难以复用/审查。
- DEV-PLAN-009 将 sqlc 作为 R200 工具链引入路线的第一阶段，需要明确 PoC、配置、CI 集成与文档动作，让后续 Atlas/Asynq/Casbin 等方案有可复用的数据访问基线。
- 根据最新决策，本计划的实施范围 **限定在 `modules/hrm` 模块及其所需的共享基础设施（例如 pkg/db、scripts、工具链）**，其他业务模块不在本期改造内。

## 目标
- 梳理 HRM 模块需要纳入 sqlc 的查询/命令，建立 `modules/hrm/infrastructure/sqlc/**` 目录结构与命名规范。
- 在 `tools.go` 固定 sqlc CLI 版本，并在 Makefile/CI（quality-gates）加入 `sqlc generate`，保证 HRM SQL 变更与生成代码同步。
- 交付 HRM 模块的 PoC（如员工信息、考勤、薪酬子域），形成生成的 Go 包并接入现有 repository/service，其他模块保持冻结。
- 在 README/CONTRIBUTING/AGENTS dev-plan 体系中记录生成流程、代码审查要点与常见故障排查。

## 风险
- SQL 散落范围大，迁移到 sqlc 可能需要分阶段抽取，需避免一次性改动阻塞业务。
- sqlc 生成需要稳定的 schema，若数据库迁移频繁，可能造成生成代码与数据库不一致。
- 需要统一命名/目录规范，否则不同模块的 SQL/Go 包可能冲突。
- CI/本地环境若 sqlc 版本不一致，生成代码易出现 diff，必须通过 tools.go + go install 模式锁定版本。

## 实施步骤
1. [x] **需求盘点与目录规划**
   - 抽样分析 `modules/hrm/infrastructure` 中的查询，按照 HRM 业务能力（如 `employee`, `attendance`, `payroll` 等聚合）整理 SQL 分类，输出《HRM SQL 清单》（包含 SQL 文件路径、用途、所属聚合、负责人）并进入 repo（例如 `docs/dev-records/hrm-sql-inventory.md`），同时在 CI 中增加检查：若 `modules/hrm` SQL 有改动但清单未更新，则阻断 PR。
   - 确定 HRM SQL 存放规则：例如 `modules/hrm/infrastructure/sqlc/{aggregate}/queries_read.sql`、`queries_write.sql`，生成代码放在 `modules/hrm/infrastructure/sqlc/{aggregate}` 包下，命名遵循 `{aggregate}_queries.sql.go`；将该规范补充到本文档与 `CONTRIBUTING.MD` 的 HRM 章节。
   - 与 DBA/HRM 领域负责人确认 PoC 范围（至少覆盖一个 CRUD 场景 + 一个事务性写操作 + 一个报表类查询），列出验收 checklist，并标记需要的共享依赖（如 `pkg/db/connector.go`、`pkg/serrors`）以确认可引用；首个聚合固定为 `employee`，聚焦 `modules/hrm/infrastructure/repositories/employee_repository.go` 的替换。
2. [x] **配置与生成 PoC**
   - 编写仓库根目录 `sqlc.yaml`：细化 `version: "2"`、`sql/` 与 `overrides` 区块，声明 `engine: postgresql`、`sql_package: pgx/v5`、`emit_pointers_for_null_types: true`、`emit_json_tags: true`、`emit_prepared_queries: true`、`emit_empty_slices: true` 等参数，并通过 `paths` 仅包含 HRM SQL 目录；工具版本固定为 `github.com/sqlc-dev/sqlc/cmd/sqlc@v1.28.0`。
   - 新增 HRM PoC SQL（以 `modules/hrm/infrastructure/sqlc/employee/queries.sql`、`modules/hrm/infrastructure/sqlc/employee/commands.sql` 为首批文件），运行 `sqlc generate`，得到 `modules/hrm/infrastructure/sqlc/employee/queries.sql.go` 等文件，并把生成包引用进 `employee_repository.go`；旧手写实现暂留以对比性能/行为，并记录差异。
   - 制定 HRM schema 来源：短期通过脚本 `scripts/db/export_hrm_schema.sh`（封装 `make db migrate up` + `pg_dump --schema-only --table=employees --table=employee_positions ...`）导出到 `modules/hrm/infrastructure/sqlc/schema.sql`；若未来迁移到独立 schema，再调整导出命令；在 README/CONTRIBUTING 中记录“更新 HRM 迁移 → 运行 export_hrm_schema.sh → sqlc generate”的操作顺序，并要求 schema 文件随 PR 评审。
   - 对 PoC 结果进行端到端验证：编写临时测试（或 re-run 现有 HRM service test）对比 sqlc 生成代码与旧查询的行为一致性，并记录验证日志在 `docs/dev-records/`。
3. [x] **工具链与 CI 集成**
   - 在 `tools.go` 添加 `_ "github.com/sqlc-dev/sqlc/cmd/sqlc"`（版本 `v1.28.0`），并在 `Makefile` 增加 `sqlc-generate` 目标：`sqlc generate -f sqlc.yaml && gofmt -w modules/hrm/infrastructure/sqlc && goimports -w modules/hrm/infrastructure/sqlc`；`make generate` 总是调用 `sqlc-generate`，即便当前改动与 HRM 无关，也要求开发者显式执行一次。
   - 更新 `.github/workflows/quality-gates.yml`，新增 `hrm-sqlc` 过滤器（匹配 `sqlc.yaml`、`modules/hrm/infrastructure/sqlc/**`、`modules/hrm/infrastructure/persistence/**/*.sql`、`scripts/db/export_hrm_schema.sh`）。仅当该过滤器命中时执行 `make sqlc-generate` 并检查 `git status`，未命中时跳过以避免冻结模块被影响。
   - 在本地/CI 文档中明确：开发者每次修改 HRM SQL/迁移都须运行 `make sqlc-generate`，CI 会通过 `hrm-sqlc` 过滤器阻止未生成的 PR；若未安装 sqlc，可运行 `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.28.0`。
4. [x] **知识沉淀与推广**
   - 在 `README.MD` / `CONTRIBUTING.MD` / `AGENTS.md` 新增 “HRM sqlc 使用指南”，覆盖：目录命名、`make sqlc-generate` 用法、schema 导出脚本、常见错误（缺少 schema、枚举未定义、NULL 处理）以及如何在 IDE 中跳转到生成代码。
   - 将 HRM PoC 样例记录到 `docs/dev-records/DEV-PLAN-010-HRM-POC.md`，包括：运行命令、性能/类型安全对比、遇到的问题与回滚策略；在 dev 例会或 wiki 中同步该经验。
   - 制定 HRM 内部 roll-out 清单（聚合/功能维度，标记优先级与 owner），在 dev-plan 中维护表格/清单，并列出解禁其他模块前需满足的条件（例如：HRM 阶段成功、工具链稳定、文档完备）。

## 里程碑
- M1：HRM sqlc 目录规范、《HRM SQL 清单》、`sqlc.yaml` 初稿评审通过（含 schema 导出脚本），并在 `tools.go` 锁定 sqlc 版本。
- M2：HRM 至少一个聚合（如 employee/payroll）使用 sqlc 生成代码并通过服务层测试，旧 SQL 在清单中标记为“待迁移/已迁移”，PoC 验证记录完成。
- M3：`quality-gates` 中的 `hrm-sqlc` 过滤器生效，`make sqlc-generate` 集成完成，README/CONTRIBUTING/AGENTS 指南更新并发布。

## 交付物
- `sqlc.yaml` 配置文件、HRM SQL 目录与 sample SQL/Go 生成结果。
- 更新后的 `tools.go`、`Makefile`、`quality-gates` workflow 片段与 HRM PoC 代码。
- README/CONTRIBUTING/AGENTS 中的 HRM sqlc 指南，以及 HRM PoC 在 dev-records 中的记录。
