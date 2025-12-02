# DEV-PLAN-011：Atlas + goose 联动基线

**状态**: ✅ 已完成（2025-12-02 20:00）

## 背景
- DEV-PLAN-009 的第二项重点任务要求以 Atlas diff 生成 up/down SQL，再交由 goose/golang-migrate 执行，形成可重复的迁移工作流（`docs/dev-plans/009-r200-tooling-alignment.md:21-49`）。
- DEV-PLAN-010 已在 HRM 模块交付 sqlc 基线、`sqlc.yaml`、`scripts/db/export_hrm_schema.sh` 等资产（`docs/dev-plans/010-sqlc-baseline.md:5-40`），但迁移仍靠手写 SQL，schema 来源也未与 Atlas 串联。
- R200 工具链路线图需要在 sqlc 基线之上建立声明式 schema → 迁移生成 → goose 执行 → schema 导出 → sqlc generate 的闭环，让 HRM 模块成为 Atlas + goose 的首个 PoC，再向其他模块推广。
- 本计划限定在 HRM 范围（positions、employees 等表及其共享基础设施），通过 Atlas 让数据库 schema 与 sqlc/schema.sql 保持一致，并补齐 Makefile/CI/文档。

## 目标
- 产出 HRM 专用的 Atlas 声明式 schema（`modules/hrm/infrastructure/atlas/schema.hcl`）与根目录 `atlas.hcl` 配置，覆盖当前 HRM 表结构并内置 lint/plan 规则。
- 将 Atlas diff 生成的迁移脚本输出到 `migrations/hrm/changes_*.sql`，由 goose 执行并与现有 `make db migrate up`、`scripts/db/export_hrm_schema.sh`、`sqlc generate` 串联。
- 在 Makefile 与 `quality-gates` workflow 中增加 `make db plan`、Atlas lint、git 状态检查，确保 CI 能阻止缺失迁移/导出的 PR。
- 更新 README/CONTRIBUTING/AGENTS/dev-records，记录 Atlas→goose→sqlc 的操作顺序、回滚策略与常见故障，保障团队协作。

## 风险
- Atlas schema 与历史 goose 迁移存在漂移，若未及时回填声明式定义，diff 可能生成错误的 up/down。
- 需要先用 `atlas migrate import --dir "file://migrations/hrm"` 将现有 goose 历史导入 Atlas state，否则第一次 diff 会尝试重建所有表。
- CLI 版本漂移会导致本地/CI 计划不一致，需要在 `tools.go` 固定 `ariga.io/atlas/cmd/atlas` 版本并同步安装方式。
- HRM 模块之外暂不接入 Atlas；若误改其他模块的迁移目录，易破坏冻结模块，需通过过滤器限制影响范围。
- 迁移执行顺序改变可能影响现有数据，需要在 PoC 阶段准备回滚脚本与验证用例。

## 实施步骤
1. **[x] Schema 基线与目录规划**
   - 对齐 HRM 现有表（positions、employees、employee_meta、employee_positions、employee_contacts），以 `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql` 与 `migrations/changes-*.sql` 为依据，梳理字段/索引/约束，记录缺失的声明式信息。
   - 在 `modules/hrm/infrastructure/atlas/` 新建 `schema.hcl`（主入口）与若干 include（按聚合拆分），并将迁移输出目录固定为 `migrations/hrm/`，使用 goose 风格命名 `changes_<unix>.{up,down}.sql`；相关规范同步到 CONTRIBUTING。
   - M1 阶段将现有 HRM 相关迁移（以 employees/positions 等表为内容的 `migrations/changes-*.sql`）整体搬迁至 `migrations/hrm/`，同步更新 goose checksum 与版本表（例如 `migrations/hrm/goose_db_version.sql`），非 HRM 迁移保留在原目录；搬迁清单在 `DEV-PLAN-011-HRM-ATLAS-POC.md` 记录，避免遗漏。
   - 使用 `atlas migrate import --dir "file://migrations/hrm"` 将现有 goose 历史迁移导入 Atlas 版本存档 (`migrations/hrm/atlas.sum` / `atlaslock.hcl`)，确保 diff 以当前状态为基准。
2. **[x] Atlas 配置与工具链**
  - 在仓库根目录新增 `atlas.hcl`，声明 dev/test/ci 三个环境，复用 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD` 环境变量，指定 `migrations/hrm` 为唯一受控目录，并启用内建 lint（`destructive`, `dependent`, `index-name` 等）。
  - 在 `tools.go` 添加 `_ "ariga.io/atlas/cmd/atlas"`（锁定为 `v0.38.0`，该版本兼容 Postgres 17），Makefile 新增 `atlas-install`、`db plan`（`atlas migrate diff --env dev --dry-run` 输出计划文件但不写入磁盘）、`db lint` 目标，同时保留/强化 `make db migrate up`（goose 执行），形成“Atlas 生成 SQL → goose 负责 apply”的唯一流水线。
  - 文档中记录 CLI 安装顺序：`make atlas-install`（git clone ariga/atlas 并 checkout `v0.38.0` 后 go build）→ `atlas version` 校验，并在 `make help` 输出新增命令的作用与依赖。
3. **[x] 迁移生成与 goose 串联**
  - 规范“修改 schema → 更新 schema.hcl → 运行 `atlas migrate diff --env dev --dir file://migrations/hrm --to file://modules/hrm/infrastructure/atlas/schema.hcl`”的流程，产出的 up/down SQL 需符合 goose 命名约定，并自动调用 `goose status` 验证迁移链连续性。
  - 更新现有 `make db migrate up` / `scripts/db/export_hrm_schema.sh` / `make sqlc-generate` 顺序，形成唯一官方指引：`atlas migrate diff`（生成 `migrations/hrm/changes_<unix>.{up,down}.sql`）→ `make db migrate up HRM_MIGRATIONS=1`（goose 通过 `-dir migrations/hrm` 执行新文件）→ `scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1` → `make sqlc-generate`，并在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md` 记录验证日志及 `atlas schema inspect` 对比结果。
  - 为回滚场景准备 `atlas migrate hash`（核对 state）+ `goose down -1` 示例，确保开发者能撤销最新 HRM 迁移，并在 README 中标注“生产/测试库均先运行 plan（dry-run）再由 goose apply”。
4. **[x] CI Guardrail**
  - 在 `.github/workflows/quality-gates.yml` 增加 `hrm-atlas` 过滤器（命中 `atlas.hcl`、`modules/hrm/infrastructure/atlas/**`、`migrations/hrm/**`、`scripts/db/export_hrm_schema.sh` 等），触发 `atlas migrate lint --git-base origin/main --env ci`、`make db plan HRM_MIGRATIONS=1`（连接 CI Postgres 服务）、`git status --short` 检查，并缓存 `migrations/hrm/atlas.sum` 以加速重复执行；workflow 将新增 `services.postgres`（`image: postgres:17`, env `POSTGRES_DB=iota_erp`, `POSTGRES_PASSWORD=postgres`, ports `5432:5432`），确保命令有数据库可连。
  - 与 `hrm-sqlc` 过滤器联动：若两者同时命中，CI 依次执行 `make db plan HRM_MIGRATIONS=1` → `make db migrate up HRM_MIGRATIONS=1`（goose 连接 CI Postgres 并执行空迁移验证）→ `scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1 DB_HOST=localhost DB_PORT=5432` → `make sqlc-generate`，确保 schema 与生成代码保持一致，并在 job 末尾强制 `git diff --exit-code modules/hrm/infrastructure/sqlc/schema.sql`.
5. **[ ] 文档与回滚策略**
- 更新 README/CONTRIBUTING/AGENTS 中的 HRM 部分，新增“Atlas + goose”章节，模版包括：命令速查、目录结构截图、`atlas migrate diff` → `make db migrate up HRM_MIGRATIONS=1` 的流程、常见错误（hash 不一致、state 漂移）及处理方式。
  - 在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md` 记录 PoC：按“时间 → 命令 → 预期 → 实际 → 结果/回滚”模板撰写，并附 `atlas schema inspect`、`goose status` 的关键输出；`docs/dev-records/hrm-sql-inventory.md` 新增“Atlas Schema File”、“Latest HRM Migration”两列，例：`employee -> modules/.../employee.hcl -> changes_1759667232.up.sql`。

## 里程碑
- M1：`schema.hcl`、`atlas.hcl`、`migrations/hrm/atlas.sum` 初稿合并，`tools.go`/Makefile 锁定 Atlas 版本，`make db plan` 返回“No Changes”，并由至少两名开发者在各自环境验证。
- M2：首个 HRM 迁移通过 `atlas migrate diff` 生成（包含 up/down），经 goose 执行并通过 HRM service 集成测试，随后运行 `scripts/db/export_hrm_schema.sh` + `make sqlc-generate` 无 diff，PoC 日志入库。
- M3：`hrm-atlas` 过滤器在 `quality-gates` 上线且保持 5 次连续绿灯，README/CONTRIBUTING/AGENTS/`hrm-sql-inventory.md` 更新完毕，Atlas → goose → sqlc 操作文档完成评审。

## 交付物
- `atlas.hcl`、`modules/hrm/infrastructure/atlas/schema.hcl`（及相关 include 文件）。
- 更新后的 `tools.go`、Makefile `db plan`/`db lint` 目标、`scripts/db/export_hrm_schema.sh` 联动说明。
- 新增/更新的 goose 迁移文件（位于 `migrations/hrm/`）。
- README/CONTRIBUTING/AGENTS 的 Atlas 流程章节、`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`、更新后的 `docs/dev-records/hrm-sql-inventory.md`。
