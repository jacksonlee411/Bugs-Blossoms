# DEV-PLAN-011A：HRM Atlas + Goose 基线缺口复核与补齐方案

**状态**: 已完成（2025-12-14 07:48 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**：
  - `docs/dev-plans/011-atlas-goose-baseline.md`（宣称 HRM 已具备 Atlas+Goose 闭环）
  - `.github/workflows/quality-gates.yml`（实际已存在 `hrm-atlas` 过滤器与相关 job 步骤）
- **当前痛点**：`DEV-PLAN-011` 文档标记为“已完成”，且 `README.MD` / `docs/CONTRIBUTING.MD` / `AGENTS.md` 已引用 “HRM Atlas + Goose 流水线”。
- 但当前仓库实际缺少 `atlas.hcl`、`modules/hrm/infrastructure/atlas/**`、`migrations/hrm/**`、`scripts/db/run_goose.sh` 等关键资产，导致：
  - `make db plan` / `make db lint` 在启用 `hrm-atlas` 过滤器时无法稳定运行；
  - 文档与代码/CI 行为存在漂移；
  - 一旦 PR 触发 `hrm-atlas`（例如改动 `scripts/db/export_hrm_schema.sh`），CI 可能在“工具链缺失”层面直接失败。
- 本 011A 的定位是：把“缺口调查结果 + 可执行补齐方案”固化为可实施的计划文档，并定义清晰验收口径，避免继续扩大 drift。
- **业务价值**：恢复 `hrm-atlas` 质量门禁的可信度（CI 报红仅代表真实 drift/规则失败，而非“缺文件/缺工具”），为后续 HRM 迁移与 sqlc 生成提供稳定基线。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [X] 让 HRM 的 Atlas/Goose 工具链在本仓库**真实可用**：本地与 CI 均可执行 `make atlas-install`、`make db plan`、`make db lint`，并有明确的失败诊断路径。
- [X] 补齐 HRM 迁移执行面：实现 `scripts/db/run_goose.sh`，使 `HRM_MIGRATIONS=1 make db migrate up|down|redo|status` 可用。
- [X] 建立 HRM Atlas “受控目录”与最小闭环资产：`atlas.hcl`、`modules/hrm/infrastructure/atlas/**`、`migrations/hrm/**`、`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`。
- [X] 修正文档与实现的**单一事实来源**：README/CONTRIBUTING/AGENTS 不再描述不存在的路径或命令；`quality-gates` 的 `hrm-atlas` 过滤器命中后不会因缺文件而失败。

### 2.2 非目标（本计划不做）
- 不将全仓库迁移系统从 `rubenv/sql-migrate` 迁移到 goose（仅补齐 HRM 专用链路）。
- 不改动冻结模块：`modules/billing`、`modules/crm`、`modules/finance`。
- 不在本计划内新增 HRM 业务表/字段（仅对齐已有 `hrm-schema.sql` 的结构基线与工具链）。

## 3. 现状复核与缺口清单 (Gap Analysis)
### 3.1 仓库实际状态（已验证）
- [X] （实施前）`Makefile` 已包含 `atlas-install` 与 `db plan/db lint` 入口，但 `db plan` 曾依赖不存在的 `modules/hrm/infrastructure/atlas/schema.hcl`（需在 011A 中修正为实际存在的 schema source/配置）。
- [X] `.github/workflows/quality-gates.yml` 已定义 `hrm-atlas` 过滤器，并在命中时运行 `make atlas-install`、`make db plan`、`make db lint`（`.github/workflows/quality-gates.yml:65`-`.github/workflows/quality-gates.yml:71`、`.github/workflows/quality-gates.yml:113`-`.github/workflows/quality-gates.yml:115`、`.github/workflows/quality-gates.yml:226`-`.github/workflows/quality-gates.yml:245`）。
- [X] HRM Goose 执行面仅在 `Makefile` 有入口（`./scripts/db/run_goose.sh`），但仓库未提供脚本，也未声明/锁定 goose CLI 的安装来源与版本（会造成 CI/开发者环境版本漂移）。
- [X] 仓库缺失以下关键文件/目录（导致工具链不可用）：
  - [X] `atlas.hcl`（CI/Makefile 期望存在）
  - [X] `modules/hrm/infrastructure/atlas/`（CI/文档期望存在）
  - [X] `migrations/hrm/`（CI/文档期望存在）
  - [X] `scripts/db/run_goose.sh`（Makefile/CI/文档期望存在）
  - [X] `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`（CI/文档期望存在）
- [X] `tools.go` 未锁定 `ariga.io/atlas/cmd/atlas`（当前仅通过 `make atlas-install` 从源码构建锁版本），与 011 文档描述存在差异（`tools.go`）。
- [X] `tools.go` 未锁定 `github.com/pressly/goose/v3/cmd/goose`，导致 `run_goose.sh` 依赖宿主环境版本，存在不一致风险。

### 3.2 风险与影响
- 触发 `hrm-atlas` 过滤器后，CI 可能在“缺文件/错误入口”阶段失败，无法体现真实质量门禁（迁移漂移、lint 失败等）。
- 文档宣称 “HRM 已迁入 migrations/hrm 并由 goose 执行”，但现状并非如此，会误导后续 DEV-PLAN（例如 `DEV-PLAN-017/020/021`）对工具链能力的依赖判断。
- Atlas `schema diff` / `migrate lint` 需要 `--dev-url`（临时/隔离 dev 数据库）来验证 SQL 正确性：若复用同一库或 dev 库不可用，会引入噪音甚至导致 lint/plan 不稳定，从而削弱 CI/CD 的可信度。
- goose baseline 迁移在“已有表但无 goose 版本表”的环境可能直接失败（表已存在），需要明确既有环境的接入策略，否则上线/迁移会出现不可预期的阻塞。

## 4. 架构与关键决策 (Architecture & Decisions)
### 4.1 目标架构图（HRM 专用链路）
```mermaid
flowchart LR
  HRMSchema[modules/hrm/.../schema/hrm-schema.sql] --> AtlasPlan[atlas schema diff (dry-run)]
  HRMDB[(PostgreSQL 17)] --> AtlasPlan
  AtlasPlan --> PlanSQL[SQL plan (stdout)]

  HRMMigs[migrations/hrm/*.sql] --> Goose[goose up/down/redo/status]
  Goose --> HRMDB
  HRMMigs --> AtlasLint[atlas migrate lint]
  AtlasLint --> HRMMigs

  HRMDB --> Export[scripts/db/export_hrm_schema.sh]
  Export --> SQLCSchema[modules/hrm/infrastructure/sqlc/schema.sql]
  SQLCSchema --> SQLCGen[make sqlc-generate]
```

### 4.2 关键决策（需要在实施前锁定）
1. **迁移目录格式（Atlas migration.format）**
   - 选项 A：`format = goose`（单文件包含 `-- +goose Up/Down`）。
   - 选项 B：`format = golang-migrate`（`*.up.sql/*.down.sql` 双文件）。
   - 选定：A（与 goose 原生格式一致，避免双文件拆分与自研封装）。
2. **Schema Source（`atlas.hcl` 的 `src` 取值）**
   - 选项 A：`src = "file://modules/hrm/infrastructure/atlas/schema.hcl"`（与 011 文档一致，但需要解决 HRM 外键引用到 core 表时的建模边界）。
   - 选项 B：`src = "file://modules/hrm/infrastructure/persistence/schema/hrm-schema.sql"`（直接复用现有 SQL，避免为外键引用补齐 core 表的 HCL 建模；但需更新文档/过滤器/库存表中的 “Atlas Schema File” 列）。
   - 选定：B（以最小闭环优先，先止血恢复 CI；为解决 HRM 外键依赖，在 `atlas.hcl` 的 `src` 里额外引入最小 Core stub：`modules/hrm/infrastructure/atlas/core_deps.sql`；后续如需 HCL 再单独开 DEV-PLAN）。
3. **工具链版本与安装策略（Atlas/Goose）**
   - 目标：CI 与开发者本地使用同一版本的 atlas/goose，避免“工具链漂移”掩盖真实质量门禁。
   - 选定：
     - Atlas：继续使用 `make atlas-install` 从上游源码构建，并由 `Makefile` 的 `ATLAS_VERSION` 锁定版本。
     - Goose：通过 `tools.go` 锁定依赖，并提供 `make goose-install` 以 `go install ...@<version>` 安装指定版本（不依赖宿主机 apt/brew）。
   - 版本基线（需在实施时写死并保持 CI/本地一致）：
     - Atlas：`ATLAS_VERSION`（当前 `Makefile` 设为 `v0.38.0`）
     - Goose：建议 `v3.26.0`（按需更新，但必须同步更新 `tools.go` 与文档）

### 4.3 最小配置与约束 (Config & Constraints)
#### 4.3.1 Atlas 配置（`atlas.hcl` 最小可用骨架）
> 目标：`make db lint` 可直接运行 `atlas migrate lint --env ci ...`，无需额外手工参数。

```hcl
variable "db_url" {
  type    = string
  default = getenv("DB_URL")
}

variable "atlas_dev_url" {
  type    = string
  default = getenv("ATLAS_DEV_URL")
}

env "ci" {
  url = var.db_url
  dev = var.atlas_dev_url
  # 注意：若 HRM 依赖 Core 表（如外键），需在此处同时加载 Core Schema 以通过 Dev DB 校验
  src = [
    "file://modules/hrm/infrastructure/atlas/core_deps.sql",
    "file://modules/hrm/infrastructure/persistence/schema/hrm-schema.sql"
  ]
  migration {
    dir    = "file://migrations/hrm"
    format = "goose"
  }
}

env "dev" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = [
    "file://modules/hrm/infrastructure/atlas/core_deps.sql",
    "file://modules/hrm/infrastructure/persistence/schema/hrm-schema.sql"
  ]
  migration {
    dir    = "file://migrations/hrm"
    format = "goose"
  }
}
```

#### 4.3.2 Dev DB 约束（Atlas `--dev-url`）
- 必须使用**隔离的 dev 数据库**（不能与 `DB_NAME` 相同），否则 schema diff/lint 可能受既有对象影响而不稳定。
- 默认约定：`ATLAS_DEV_DB_NAME=hrm_dev`（本地与 CI 保持一致，避免“默认用 DB_NAME”导致不隔离）。
- 约定环境变量（实现时建议由 Makefile/CI 从 `DB_*` 自动拼接，减少口径漂移）：
  - `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME`
  - `ATLAS_DEV_DB_NAME`（默认 `hrm_dev`）
  - （可选）`DB_URL` / `ATLAS_DEV_URL`（当需要覆盖拼接规则时使用）

#### 4.3.3 Goose 版本表约束（baseline bootstrap）
- 默认使用 goose 的版本表 `goose_db_version`（除非在脚本中显式通过 `-table` 覆盖）。
- baseline 版本号以迁移文件前缀决定（例如 `00001_hrm_baseline.sql` 的版本号为 `1`）。
 - 为保证 `atlas migrate lint` 可在“干净数据库”上执行，HRM baseline 迁移需要为外键依赖的被引用表提供最小 Core stub（例如在 `00001_hrm_baseline.sql` 的 Up 段落用 `CREATE TABLE IF NOT EXISTS ...` 预置 `tenants/uploads/currencies`）。

## 5. 接口契约与命令口径 (CLI / Make Targets)
### 5.1 必须支持的命令（对齐文档与 CI）
- `make atlas-install`
- `make goose-install`（或等价的可重复安装入口，用于锁定 goose CLI 版本）
- `make db plan`：输出 dry-run 的 SQL 计划（不落盘、不写迁移文件）
- `make db lint`：运行 `atlas migrate lint`（至少覆盖 destructive/dependent 等规则）
- `HRM_MIGRATIONS=1 make db migrate up|down|redo|status`：通过 `scripts/db/run_goose.sh` 执行 HRM 迁移
- `scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1`：导出 HRM schema（要求 HRM 表已存在）
- `scripts/db/export_hrm_schema.sh` 在需要执行迁移时，必须走 HRM goose 链路（等价于内部执行 `HRM_MIGRATIONS=1 make db migrate up`），避免误用旧迁移入口造成“导出前未迁移/迁移口径漂移”。
- `make sqlc-generate`：生成 HRM sqlc 产物并保证 `git status --short` 干净

### 5.1.1 `make goose-install` 口径（选定）
- 实现建议（示例）：
  - `GOWORK=off go install github.com/pressly/goose/v3/cmd/goose@v3.26.0`
- 验证口径：
  - `goose -version` 输出与文档一致（CI 与本地一致）。

### 5.2 `scripts/db/run_goose.sh` 行为约束（拟定）
- 输入：子命令 `up|down|redo|status`（从 `make db migrate <cmd>` 传入）。
- DSN：使用 `postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable`。
- 迁移目录：固定 `migrations/hrm`。
- Down/Redo：尊重 `GOOSE_STEPS`（例如 `GOOSE_STEPS=1` 时仅回滚/重做 1 步）。
- 失败处理：任何失败返回非 0；打印关键环境与 goose 输出；禁止吞错。
- 依赖检查：若 `goose` 不存在或版本不符合要求，必须给出明确提示（建议打印 `goose -version` 与建议的安装命令）。
- 既有环境接入（baseline bootstrap）：支持一种“仅写入版本表、不执行 baseline SQL”的模式，用于目标库已有人为建表但尚未接入 goose 版本表的场景（不使用不存在的 `--fake` 参数）。
  - 建议形态：通过环境变量显式开启（例如 `GOOSE_BOOTSTRAP_BASELINE=1`），并要求显式指定/推导 baseline 版本号（避免误写版本）。
  - 行为约束（必须可重复执行/幂等）：
    - 若 `goose_db_version` 不存在：创建该表（仅针对版本表，允许使用 `DO $$ ... $$;` 方式实现幂等）。
    - 若 baseline 版本记录已存在且 `is_applied=true`：直接成功退出（不重复插入）。
    - 否则：插入 baseline 版本记录（`version_id=<baseline>`，`is_applied=true`），并打印“已 bootstrap”的明确日志。
  - Postgres 版本表结构参考：`github.com/pressly/goose/v3/internal/dialect/dialectquery/postgres.go`（`goose_db_version` 包含 `version_id bigint`、`is_applied boolean`、`tstamp` 等字段）。

#### 5.2.1 Baseline bootstrap（Postgres）参考 SQL（用于实现脚本）
> 说明：仅用于 goose 版本表的幂等初始化与 baseline 标记，不用于业务 schema 的“容错创建”。

```sql
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_tables
    WHERE schemaname = current_schema()
      AND tablename = 'goose_db_version'
  ) THEN
    CREATE TABLE goose_db_version (
      id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
      version_id bigint NOT NULL,
      is_applied boolean NOT NULL,
      tstamp timestamp NOT NULL DEFAULT now()
    );
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM goose_db_version
    WHERE version_id = 1 AND is_applied = true
  ) THEN
    INSERT INTO goose_db_version (version_id, is_applied)
    VALUES (1, true);
  END IF;
END $$;
```

### 5.3 Atlas Dev DB 约定（`--dev-url` / `env.dev`）
- 目标：Atlas 的 plan/lint 必须在一个“隔离的 dev 数据库”上运行，避免与业务库/测试库互相污染。
- 本地建议：在同一 Postgres 实例上创建独立 dev 库（例如 `ATLAS_DEV_DB_NAME=hrm_dev`），由 `make db plan`/CI 步骤负责确保其存在。
- CI 建议（无 DinD）：复用 workflow 的 Postgres service，但在运行 Atlas 前创建独立 dev 库（例如 `hrm_dev`）；若权限受限，则在计划中明确替代方案与约束（例如使用专用角色或预置数据库）。

#### 5.3.1 Dev DB 创建口径（CI/本地通用）
- 约定（推荐）：
  - `DB_URL` 指向目标库（例如 `.../iota_erp`）
  - `ATLAS_DEV_URL` 指向隔离 dev 库（例如 `.../hrm_dev`）
- 示例（使用 maintenance DB 创建 dev 库）：
  - 幂等创建（推荐，避免重复创建导致 job 失败）：
    - `psql "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='${ATLAS_DEV_DB_NAME:-hrm_dev}'" | grep -q 1 || psql "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/postgres?sslmode=disable" -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${ATLAS_DEV_DB_NAME:-hrm_dev};"`
  - 若需要幂等：在脚本/CI 中先查 `pg_database` 再创建，避免重复创建导致 job 失败。

### 5.4 Baseline 迁移兼容性（既有环境）
- 问题：若目标环境已通过 `hrm-schema.sql` 手工建表，直接执行 baseline 迁移很可能因“表已存在”失败。
- 建议策略（优先）：提供可审核、可重复的 bootstrap 流程：在确认 schema 已符合 baseline 的前提下，仅初始化 goose 版本表并标记 baseline 版本已应用，然后进入后续增量迁移。
- 不建议策略：在 baseline SQL 中大量使用 `IF NOT EXISTS` 规避失败（容易掩盖漂移与约束差异）；如确需使用，必须在计划中列明适用范围与风险。

## 6. 实施步骤 (Execution Plan)
1. [X] **补齐缺失文件与目录骨架**
   - [X] 新增 `atlas.hcl`（至少包含 `dev/test/ci` env、`migration.dir` 指向 `migrations/hrm`、`src` 指向 HRM schema 源）。
   - [X] 新增 `modules/hrm/infrastructure/atlas/`（放置最小 Core stub：`core_deps.sql`，以及 README 说明）。
   - [X] 新增 `migrations/hrm/`（至少包含 baseline + `atlas.sum`）。
     - [X] baseline：`00001_hrm_baseline.sql`（goose 格式，版本号=1；内容与 `hrm-schema.sql` 对齐；包含最小 Core stub 以支持干净库校验）
     - [X] smoke：`00002_hrm_migration_smoke.sql`（用于验证 `down/redo` 链路，创建并删除 `__hrm_migration_smoke` 表）
   - [X] 新增 `scripts/db/run_goose.sh`（按 5.2 约束实现）。
   - [X] 在 `tools.go` 锁定 goose CLI 依赖，并补齐 Make 安装入口（`make goose-install`）。
   - [X] 新增 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`（按“时间-命令-预期-实际-结论/回滚”模板）。

2. [X] **修正 Makefile 与文档声明的漂移**
   - [X] `make db plan`：输出 dry-run SQL plan（不落盘），并保证 CI 可运行。
   - [X] 明确并落实 Atlas dev-db 策略：`make db plan/db lint` 默认使用独立 `ATLAS_DEV_DB_NAME`，并确保该库存在（本地/CI 均可重复执行）。
   - [X] `make db lint`：无需手工参数即可运行（通过 `atlas.hcl` + `DB_URL/ATLAS_DEV_URL`）。
   - [X] 修正 `scripts/db/export_hrm_schema.sh`：迁移阶段走 HRM goose（设置 `HRM_MIGRATIONS=1`），并兼容 Postgres 17 的 `pg_dump` 版本差异。
   - [X] 修正 `README.MD` / `docs/CONTRIBUTING.MD` / `AGENTS.md` 中关于文件路径与命令的漂移。
   - [X] 同步更新 `docs/dev-records/hrm-sql-inventory.md` 的 “Atlas Schema File / Latest HRM Migration” 列，使其真实存在且可点击。

3. [X] **完善 CI 的 `hrm-atlas` 质量门禁闭环**
   - [X] `lint-and-format` job 在 `hrm-atlas=true` 时可连接 Postgres 并成功运行 `make db plan`、`make db lint`。
   - [X] CI 侧使用独立 target/dev DB（`DB_NAME=iota_erp_hrm_atlas`、`ATLAS_DEV_DB_NAME=hrm_dev`），避免污染默认库。
   - [X] CI 在 `hrm-atlas=true` 时安装 goose（`make goose-install`），并运行 `HRM_MIGRATIONS=1 make db migrate up` 做 smoke。
   - [X] CI 在 `hrm-sqlc=true` 时先执行 `scripts/db/export_hrm_schema.sh` 再运行 `make sqlc-generate`，并在末尾用 `git status --porcelain` 确保无遗留 diff。

4. [X] **Readiness 记录与回归验证**
   - [X] 本地验证已跑通并写入记录模板（`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`）：
     - [X] `make atlas-install`
     - [X] `make goose-install`
     - [X] `make db plan`
     - [X] `make db lint`
     - [X] `HRM_MIGRATIONS=1 make db migrate up` / `... down|redo|status`（含 smoke）
     - [X] `scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1 && make sqlc-generate`
     - [X] `make check lint`（避免 cleanarch/golangci 漂移）

## 7. 测试与验收标准 (Acceptance Criteria)
- CI：任意触发 `hrm-atlas` 的 PR，`lint-and-format` job 不因缺文件/缺脚本失败；`make db plan`、`make db lint` 可重复通过。
- 本地：在干净 PG17 数据库上可按文档执行完整闭环：生成/应用 HRM 迁移 → 导出 schema → sqlc generate → `git status --short` 干净。
- 文档：README/CONTRIBUTING/AGENTS/Inventory 中引用的所有路径都真实存在且命令可执行。
- 工具链：atlas/goose 版本来源清晰且可重复安装；CI/开发者本地不依赖宿主机随机版本。
- 既有环境：存在可审核的 baseline bootstrap 指引，不会因“表已存在”导致 HRM goose 接入卡死。

## 8. 回滚与降级策略 (Rollback)
- 如补齐方案引入 CI 不稳定：优先把 `hrm-atlas` 过滤器临时收敛到“仅在存在 atlas/goose 资产后再启用”的最小集合（以避免误触发）。
- 如 HRM 迁移链路阻塞其它开发：允许短期继续使用 `modules/hrm/infrastructure/persistence/schema/hrm-schema.sql` 的手工执行（仅限 e2e/本地验证），但必须在 011A 完成后移除临时路径并补齐记录。

## 9. 里程碑与交付物 (Milestones & Deliverables)
- 里程碑 M1（可跑通）：`atlas.hcl` + `scripts/db/run_goose.sh` + `migrations/hrm` 基线产物 + `make db plan/lint` 可执行。
- 里程碑 M2（闭环）：CI 在 `hrm-atlas` 命中时通过；`DEV-PLAN-011-HRM-ATLAS-POC.md` 有可复现日志；文档/Inventory 无漂移。

## 10. 后续动作 (Next)
- 完成 011A 后，允许 `DEV-PLAN-017`（outbox）与 `DEV-PLAN-021`（org atlas/goose）复用同一套“Atlas+Goose”约定，避免每个模块重复踩坑。
