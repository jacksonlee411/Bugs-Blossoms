# CLAUDE.md - IOTA SDK Guide

## Overview
DO NOT COMMENT EXECESSIVELY. Instead, write clear and concise code that is self-explanatory.

## 项目现状
- 项目仍处于早期开发阶段，尚未投产，所有特性以快速验证为主。
- 当前仅由单一开发者负责全栈交付，可在保证质量前提下省略非必要的审批与跨组沟通流程，直接按照本指南自行决策。

## Module Architecture

Each module follows a strict **Domain-Driven Design (DDD)** pattern with clear layer separation:

```
modules/{module}/
├── domain/                     # Pure business logic
│   ├── aggregates/{entity}/    # Complex business entities
│   │   ├── {entity}.go         # Entity interface
│   │   ├── {entity}_impl.go    # Entity implementation
│   │   ├── {entity}_events.go  # Domain events
│   │   └── {entity}_repository.go # Repository interface
│   ├── entities/{entity}/      # Simpler domain entities
│   └── value_objects/          # Immutable domain concepts
├── infrastructure/             # External concerns
│   └── persistence/
│       ├── models/models.go    # Database models
│       ├── {entity}_repository.go # Repository implementations
│       ├── {module}_mappers.go # Domain-to-DB mapping
│       ├── schema/{module}-schema.sql # SQL schema
│       └── setup_test.go       # Test utilities
├── services/                   # Business logic orchestration
│   ├── {entity}_service.go     # Service implementation
│   ├── {entity}_service_test.go # Service tests
│   └── setup_test.go           # Test setup
├── presentation/               # UI and API layer
│   ├── controllers/
│   │   ├── {entity}_controller.go # HTTP handlers
│   │   ├── {entity}_controller_test.go # Controller tests
│   │   ├── dtos/{entity}_dto.go # Data transfer objects
│   │   └── setup_test.go       # Test utilities
│   ├── templates/
│   │   ├── pages/{entity}/     # Entity-specific pages
│   │   │   ├── list.templ      # List view
│   │   │   ├── edit.templ      # Edit form
│   │   │   └── new.templ       # Create form
│   │   └── components/         # Reusable UI components
│   ├── viewmodels/             # Presentation models
│   ├── mappers/mappers.go      # Domain-to-presentation mapping
│   └── locales/                # Internationalization
│       ├── en.json             # English translations
│       ├── ru.json             # Russian translations
│       └── uz.json             # Uzbek translations
├── module.go                   # Module registration
├── links.go                    # Navigation items
└── permissions/constants.go    # RBAC permissions
```

## Creating New Entities (Repositories, Services, Controllers)

### 1. Domain Layer
- Create domain entity in `modules/{module}/domain/aggregates/{entity_name}/`
- Define repository interface with CRUD operations and domain events
- Follow existing patterns (see `payment_category` or `expense_category`)

### 2. Infrastructure Layer
- Add database model to `modules/{module}/infrastructure/persistence/models/models.go`
- Create repository implementation in `modules/{module}/infrastructure/persistence/{entity_name}_repository.go`
- Add domain-to-database mappers in `modules/{module}/infrastructure/persistence/{module}_mappers.go`

### 3. Service Layer
- Create service in `modules/{module}/services/{entity_name}_service.go`
- Include event publishing and business logic methods
- Follow constructor pattern: `NewEntityService(repo, eventPublisher)`

### 4. Presentation Layer
- Create DTOs in `modules/{module}/presentation/controllers/dtos/{entity_name}_dto.go`
- Create controller in `modules/{module}/presentation/controllers/{entity_name}_controller.go`
- Create viewmodel in `modules/{module}/presentation/viewmodels/{entity_name}_viewmodel.go`
- Add mapper in `modules/{module}/presentation/mappers/mappers.go`

### 5. Templates (if needed)
- Create templ files in `modules/{module}/presentation/templates/pages/{entity_name}/`
- Common templates: `list.templ`, `edit.templ`, `new.templ`
- Run `templ generate` after creating/modifying .templ files

### 6. Localization
- Add translations to all locale files in `modules/{module}/presentation/locales/`
- Include NavigationLinks, Meta (titles), List, and Single sections

### 7. Registration
- Add navigation item to `modules/{module}/links.go`
- Register service and controller in `modules/{module}/module.go`:
  - Add service to `app.RegisterServices()` call
  - Add controller to `app.RegisterControllers()` call  
  - Add quick links to `app.QuickLinks().Add()` call

### 8. Verification
- Run `go vet ./...` to verify compilation
- Run `templ generate && make css` if templates were modified

### Casbin / Authorization
- Update modular policy fragments under `config/access/policies/**`, then run `make authz-pack` to regenerate `config/access/policy.csv`.
- `make authz-pack` 同时会生成/刷新 `config/access/policy.csv.rev`，`pkg/authz/version.Provider` 依赖该文件提供 base revision；不要手动编辑。
- Core 模块暴露 `/core/api/authz/**` API：`GET /policies`、`GET /requests`、`POST /requests` 及 `POST /requests/{id}/approve|reject|cancel|trigger-bot|revert`，调用前确保用户拥有 `Authz.*` 权限；若收到 `AUTHZ_INVALID_REQUEST`，请检查请求体的 `base_revision` 是否落后 `config/access/policy.csv.rev`。
  - 示例：`curl -b sid=<sid> -X POST /core/api/authz/requests -d '{"object":"core.users","action":"read","diff":[...]}' -H 'Content-Type: application/json'`.
- `GET /core/api/authz/debug` 仅对 `Authz.Debug` 权限开放，必需 `subject/object/action` 查询参数，可选 `domain` 与 `attr.<key>=<value>` 形式的 ABAC 属性；接口自带 `20 req/min/IP` 限流，响应包含 `allowed/mode/latency_ms/request/attributes/trace.matched_policy`，并在日志与 `authz_debug_requests_total|latency_seconds` 指标中记录 request id 与 tenant。
- Run `make authz-test` (compiles `pkg/authz` plus helper packages) before committing any authz-related Go changes.
- Run `make authz-lint` to execute policy packing and the deterministic parity fixtures (`scripts/authz/verify --fixtures ...`). CI hooks onto the same targets.
- Use `go run ./scripts/authz/export -dsn <dsn> -out <path> -dry-run` for audited exports (requires `ALLOWED_ENV=production_export`).
- Use `go run ./scripts/authz/verify --sample 0.2` for on-demand parity checks against a live database (set `AUTHZ_MODE`/`AUTHZ_FLAG_CONFIG` as needed).

## Tool use
- DO NOT USE `sed` for file manipulation

## HRM sqlc 指南
- HRM SQL 与 schema 必须通过 `scripts/db/export_hrm_schema.sh` 更新（可设置 `SKIP_MIGRATE=1` 仅导出 schema）。
- 任意影响 `sqlc.yaml`、`modules/hrm/infrastructure/sqlc/**`、`modules/hrm/infrastructure/persistence/**/*.sql` 或 `docs/dev-records/hrm-sql-inventory.md` 的改动都要运行 `make sqlc-generate`。`make generate` 会自动调用该目标。
- sqlc 生成的内容全部位于 `modules/hrm/infrastructure/sqlc/**`，生成后必须 `git status --short` 确认无遗留 diff。CI 的 `hrm-sqlc` 过滤器也会执行同样检查。
- 变更 HRM SQL 时记得同步维护《HRM SQL Inventory》，方便评审追踪迁移进度。

## HRM Atlas + Goose
- HRM schema 的权威定义位于 `modules/hrm/infrastructure/atlas/schema.hcl`，配套配置在仓库根目录 `atlas.hcl`（`dev/test/ci` 环境复用 `DB_*` 变量）。
- 生成迁移：`atlas migrate diff --env dev --dir file://migrations/hrm --to file://modules/hrm/infrastructure/atlas/schema.hcl`，Dry-run 可用 `make db plan`。
- 执行迁移：`make db migrate up HRM_MIGRATIONS=1`（当 `HRM_MIGRATIONS=1` 时会调用 `scripts/db/run_goose.sh`，使用 goose 操作 `migrations/hrm/changes_<unix>.{up,down}.sql`）。
  - 回滚最新步骤：`GOOSE_STEPS=1 make db migrate down HRM_MIGRATIONS=1`
  - redo：`GOOSE_STEPS=1 make db migrate redo HRM_MIGRATIONS=1`
  - 查看链路：`make db migrate status HRM_MIGRATIONS=1`
- `make db lint` 会运行 `atlas migrate lint --env ci --git-base origin/main`，CI 通过新建的 `hrm-atlas` 过滤器强制该检查。
- Atlas/Goose 的操作日志请登记在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`。

## Build/Lint/Test Commands
- After changes to css or .templ files: `templ generate && make css`
- After changes to Go code: `go vet ./...` (Do NOT run `go build` as it is not needed)
- Run all tests: `make test` or `go test -v ./...` 
- Run single test: `go test -v ./path/to/package -run TestName`
- Run specific subtest: `go test -v ./path/to/package -run TestName/SubtestName`
- Check translation files: `make check tr`
- Apply migrations: `make db migrate up`
- Default database runtime: PostgreSQL 17 (local compose + CI)

### Quality Gates
- `.github/workflows/quality-gates.yml` runs on every push to `main`/`dev` and on all PRs. It enforces Go fmt/vet, `make check lint`, unit/integration tests, templ/Tailwind regeneration (with `git status`), locale checks, and PostgreSQL 17 migration smoke tests with `migrate.log` artifacts.
- Before pushing, run the commands tied to your change scope:
  - Go code: `go fmt ./... && go vet ./... && make check lint && make test`
  - `.templ`/Tailwind assets: `make generate && make css` then ensure `git status --short` is clean
  - Locale JSON: `make check tr`
  - Migrations/schema SQL: `make db migrate up && make db seed` (optional `make db migrate down`)

## Architecture Guard
- `.gocleanarch.yml` 定义了 domain/services/presentation/infrastructure 与 pkg/cmd/shared 层的依赖约束。
- `make check lint` 会先执行 golangci-lint，再运行 `go run ./cmd/cleanarchguard -config .gocleanarch.yml`。
- `quality-gates` 的 lint job 在 Go 文件或 `.gocleanarch.yml` 变更时自动触发，同样复用该命令。

## Code Style Guidelines
- Use `go fmt` for formatting. Do not indent code manually.
- Use Go v1.24.10 and follow standard Go idioms
- File organization: group related functionality in modules/ or pkg/ directories
- Naming: use camelCase for variables, PascalCase for exported functions/types
- Testing: table-driven tests with descriptive names (TestFunctionName_Scenario), use the `require` and `assert` packages from `github.com/stretchr/testify`
- Error handling: use pkg/serrors for standard error types
- Type safety: use strong typing and avoid interface{} where possible
- Follow existing patterns for database operations with jmoiron/sqlx
- For UI components, follow the existing templ/htmx patterns
- NEVER read *_templ.go files, they contain no useful information since they are generated by templ generate (make generate) command from .templ files

## UI Implementation Guidelines

### HTMX Best Practices
- Use `htmx.IsHxRequest(r)` to check if a request is from HTMX
- Use `htmx.SetTrigger(w, "eventName", payload)` for setting HTMX response triggers

## 模块冻结政策（Billing / CRM / Finance）

- `modules/billing`, `modules/crm`, `modules/finance` 已进入长期冻结状态，暂停一切新特性、重构与 Bug 修复；除非产品委员会重新解冻，否则禁止修改这些目录下的任何代码、SQL、模板与资源文件。
- 质量门禁（`quality-gates` workflow）、本地测试脚本与 `sql` 格式化步骤均已排除上述模块，它们不会参与 `go test`、`go vet`、`golangci-lint`、`pg_format` 等自动化检查。遇到故障也无需修复，保持当前快照即可。
- 若业务需求确实需要调整，请先更新 dev-plan 并经负责人批准，随后在 AGENTS.md 中撤销冻结声明，再恢复质量门禁与测试范围。
