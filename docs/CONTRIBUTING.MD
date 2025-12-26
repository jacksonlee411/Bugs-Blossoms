# Contributing Guide

> 仓库级开发规则、变更触发器与文档门禁以 `AGENTS.md` 为主干 SSOT；本文件聚焦贡献者上手与 CI 对齐操作。

## ⚙️ Prerequisites

Ensure these tools are installed before you begin:

- [Go v1.24.10](https://golang.org/doc/install)
- [Air v1.61.5](https://github.com/air-verse/air#Installation) - Hot-reloading
- [Docker v27.2.0](https://docs.docker.com/get-docker/) - Containerization
- [Templ v0.3.857](https://templ.guide/) - Templating
- [TailwindCSS v3.4.13](https://tailwindcss.com/docs/installation) - Styling
- [golangci-lint 1.64.8](https://golangci-lint.run/welcome/install/) - Linting
- [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) - Tunnel (Optional)

### Windows Setup

1. **Go**: Download from [golang.org](https://golang.org/doc/install), install, and verify with `go version`

2. **Air**:
   ```cmd
   go install github.com/air-verse/air@v1.61.5
   ```
   Add `%USERPROFILE%\go\bin` to your PATH

3. **Docker Desktop**: Download from [docker.com](https://docs.docker.com/desktop/install/windows-install/), enable WSL 2 during installation

4. **Templ**:
   ```cmd
   go install github.com/a-h/templ/cmd/templ@v0.3.857
   ```

5. **TailwindCSS**: Using npm (requires Node.js):
   ```cmd
   npm install -g tailwindcss
   ```
   Or with standalone executable:
   ```cmd
   curl.exe -sLO https://github.com/tailwindlabs/tailwindcss/releases/v3.4.13/download/tailwindcss-windows-x64.exe
   rename tailwindcss-windows-x64.exe tailwindcss.exe
   ```

6. **golangci-lint**:
   ```cmd
   curl.exe -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.ps1 | powershell -Command -
   ```

7. **cloudflared**
   ```cmd
   winget install --id Cloudflare.cloudflared
   ```

## 🛠️ Development Setup

1. **Clone the repository**:
   ```bash
   git clone https://github.com/iota-uz/iota-sdk.git
   cd iota-sdk
   ```

2. **Create env file**:
   ```bash
   cp .env.example .env
   ```
   Windows: `copy .env.example .env`

3. **Install dependencies**:
   ```bash
   make deps
   ```
   Windows: If `make` is unavailable, install via [GnuWin32](http://gnuwin32.sourceforge.net/packages/make.htm) or use [Git Bash](https://gitforwindows.org/)

### 多 Worktree 本地开发（可选，但推荐）

如果你在同一台机器上用 `git worktree` 同时维护多个工作目录，当前推荐做法是：**所有 worktree 共用一套本地 Postgres/Redis（固定端口）**，减少端口/配置管理成本与资源占用。

推荐做法（详见 `docs/dev-plans/011C-worktree-shared-local-dev-infra.md`）：

1. 在任意 worktree 启动共享 DB/Redis：
   ```bash
   docker compose -f compose.dev.yml up -d db redis
   ```
2. 初始化数据库（通常只需要做一次）：
   ```bash
   make db migrate up && make db seed
   ```

> 高级模式：如果你需要“每个 worktree 独立一套 DB/Redis 并行运行（端口/数据隔离）”，请参考 `docs/dev-plans/011B-worktree-local-dev-docker-isolation.md`。

> 注意：不要在任意 worktree 随意执行 `docker compose -f compose.dev.yml down -v`，它会清理共享数据卷。

4. **Run PostgreSQL**:
   ```bash
   make db local
   ```
   Ensure Docker is running before executing this command

5. **Apply migrations**:
   ```bash
   make db migrate up && make db seed
   ```

6. **Run TailwindCSS in watch mode** (new terminal):
   ```bash
   make css watch
   ```

7. **Start development server**:
   ```bash
   air
   ```

8. **Access the application**:
   - Web app: [http://localhost:3200](http://localhost:3200)
   - Login credentials:
     - Email: `test@gmail.com`
     - Password: `TestPass123!`
   - GraphQL: [http://localhost:3200/query](http://localhost:3200/query)

### 常见问题：登录页 404（租户域名不匹配）

登录页路由本身存在，但应用会先根据请求的 `Host` 头做租户解析；如果找不到对应 `tenants.domain`，会直接返回 `404`（包括 `/login`）。

排查/修复步骤：

1. **确认已迁移+seed**（否则 `tenants` 表不存在也会导致 `404`）：
   ```bash
   make db migrate up && make db seed
   ```

2. **确认访问域名与 `DOMAIN` 一致**：默认租户域名会随 `DOMAIN`（见 `.env` / `.env.example`）写入/更新。
   - 方案 A（最简单）：把 `.env` 里的 `DOMAIN` 改成 `localhost`，然后再次执行 `make db seed`，最后访问 `http://localhost:3200/login`
   - 方案 B：保留 `DOMAIN=default.localhost`，在本机添加 hosts 记录 `127.0.0.1 default.localhost`，然后访问 `http://default.localhost:3200/login`

### 常见问题：登录后页面无样式/样式未应用（CSS 404）

如果登录后页面排版“像没加载 CSS”，通常是 `/assets/css/main*.css` 返回了 `404`。原因一般是 **Tailwind 构建产物未生成**（`modules/core/presentation/assets/css/main.min.css`），但页面仍会引用它。

排查/修复步骤：

1. **确认静态资源是否 404**：
   ```bash
   curl -I http://localhost:3200/assets/css/main.min.css
   ```

2. **生成 CSS（一次即可）**：
   ```bash
   make css
   ```

3. **如果你在开发中需要自动重建**（推荐新开一个终端）：
   ```bash
   make css watch
   ```

### Development Tools Manager

For a more convenient development experience, use the DevHub TUI tool to manage all development services from a single interface:

```bash
make devtools
```

The DevHub allows you to:
- Start/stop the development server
- Toggle template file watching (`templ generate --watch`)
- Toggle CSS file watching (TailwindCSS watch mode)
- Manage local PostgreSQL database
- Start/stop Cloudflare tunnel

Navigate with arrow keys, toggle services with Space/Enter, and quit with 'q'.

## ✅ Quality Gates & Local Checks

All pushes to `main`/`dev` and every pull request run `.github/workflows/quality-gates.yml`. The workflow enforces Go formatting/linting/testing on Go 1.24.10, regenerates templ/Tailwind artifacts only when presentation files change, checks locale JSON diffs, and runs PostgreSQL 17 + Redis-backed migrations with a `migrate.log` artifact.

| When you change… | CI step | Run locally before pushing |
| --- | --- | --- |
| Any Go code | `go fmt ./...`, `go vet ./...`, `make check lint`, `go test -v ./...` | `go fmt ./... && go vet ./... && make check lint && make test` |
| `.templ`, `tailwind.config.js`, or `modules/**/presentation/assets/**` | `make generate`, `make css`, `git status --short` | `make generate && make css` then `git status --short` |
| `modules/**/presentation/locales/**/*.json` | `make check tr` | `make check tr` |
| `migrations/**` or `modules/**/infrastructure/persistence/schema/**` | PostgreSQL/Redis services + `make db migrate up/down` + `make db seed` (logs stored in `migrate.log`) | `make db migrate up && make db seed` (optionally `make db migrate down` to smoke test) |
| `sqlc.yaml`, `modules/person/infrastructure/sqlc/**`, `modules/person/infrastructure/persistence/**/*.sql`, `scripts/db/export_person_schema.sh` | `make sqlc-generate` + `git status --short` via `person-sqlc` filter | `scripts/db/export_person_schema.sh` + `make sqlc-generate` + `git status --short` |

> **说明**：历史冻结模块（billing/finance/crm/projects）已在 DEV-PLAN-040 中移除；门禁不再对其保留排除/豁免口径。

Running the matching commands locally ensures your branch passes the `quality-gates` check before you open a PR.

### Person sqlc 变更

- Person 任何 SQL/迁移调整都必须先运行 `scripts/db/export_person_schema.sh` 更新 `modules/person/infrastructure/sqlc/schema.sql`，随后执行 `make sqlc-generate`（`make generate` 会自动调用），以便重建 `modules/person/infrastructure/sqlc/**`。
- 生成步骤也会在 CI 中通过 `person-sqlc` 过滤器自动执行；若还有未提交的生成文件，工作流会失败。

### Person Atlas + Goose

- Atlas 配置位于仓库根目录 `atlas.hcl`，schema source 使用 SQL：
  - `modules/person/infrastructure/atlas/core_deps.sql`（Person 外键依赖的最小 Core stub）
  - `modules/person/infrastructure/persistence/schema/person-schema.sql`（Person 目标结构）
- 迁移目录为 `migrations/person/*.sql`（goose 格式，文件名前缀为数字版本号）。
- Dry-run 计划：建议使用独立的 target/dev DB，执行 `DB_NAME=iota_erp_person_atlas ATLAS_DEV_DB_NAME=person_dev make db plan`（输出 SQL plan，不写入磁盘）。
- 使用 `PERSON_MIGRATIONS=1 make db migrate up` 触发 `scripts/db/run_goose.sh`，Goose 会对 `migrations/person/*.sql` 执行 apply/rollback。  
  - 回滚：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate down`  
  - redo：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate redo`  
  - 状态：`PERSON_MIGRATIONS=1 make db migrate status`
- 迁移执行完毕后必须运行 `scripts/db/export_person_schema.sh SKIP_MIGRATE=1 && make sqlc-generate`，保持 schema/sqlc 与数据库一致。
- `make db lint` 会执行 `atlas migrate lint --env ci --git-base origin/main`（需要 `ATLAS_DEV_DB_NAME` 指向干净的隔离库），CI 通过 `person-atlas` 过滤器强制该步骤；任何缺失的迁移或 `atlas.sum` 漂移都会导致 PR 失败。
- 所有 Atlas / Goose 演练请写入对应 Dev-Plan 的 readiness 记录，确保有可追踪的“命令 → 结果”记录。

## 🔐 Casbin 授权与策略维护（015C）

015C 起已移除“策略草稿（requests）/审批/bot”链路；管理员通过 UI 直接维护生效策略。当前口径见：

- `docs/runbooks/authz-policy-apply-api.md`

提交 PR 前的最小门槛：

1. **策略文件改动**：修改 `config/access/policies/**` 后运行 `make authz-pack` 生成聚合版 `config/access/policy.csv` 与 `config/access/policy.csv.rev`；随后执行 `make authz-test && make authz-lint`。
2. **403 契约示例**：未经授权时 HX/REST 返回统一 JSON：`{\"error\":\"forbidden\",\"object\":\"logging.logs\",\"action\":\"view\",\"subject\":\"user:<uuid>\",\"domain\":\"logging\",\"missing_policies\":[...],\"debug_url\":\"/core/api/authz/debug?subject=...&object=...&action=...\",\"base_revision\":\"<sha256>\",\"request_id\":\"<rid>\"}`。

完成上述步骤后再提交 PR，可显著降低策略回归风险，也方便合作者复现你的改动。

## 🧪 Running Tests

### E2E Testing

The project includes Playwright end-to-end tests that run against a separate database to isolate test data from development data.

#### Setup:
1. **Create and prepare e2e database**:
   ```bash
   make e2e setup
   ```

2. **Start e2e server** (in a separate terminal):
   ```bash
   make e2e server
   ```
   This starts the server on port 3201 connected to the e2e database (`iota_erp_e2e`).

3. **Run tests**:
   ```bash
   # All tests
   make e2e test

   # Interactive mode
   make e2e run

   # Or directly with npm
   cd e2e/
   pnpm run test:headed
   ```

#### Available E2E Commands:
- `make e2e setup` - Create e2e database, run migrations, and seed
- `make e2e reset` - Drop and recreate e2e database with fresh data
- `make e2e server` - Start server on port 3201 (connected to e2e database)
- `make e2e test` - Run all e2e tests
- `make e2e run` - Open Playwright UI mode
- `make e2e clean` - Drop e2e database

#### Important Notes:
- E2E tests use a separate database (`iota_erp_e2e`) from development (`iota_erp`)
- E2E server runs on port 3201, development server runs on port 3200
- Always run `make e2e server` before running tests to ensure server connects to correct database
- Configuration files: `/e2e/.env.e2e`, `/e2e/playwright.config.ts`

## 📚 Documentation

Generate code documentation:

```bash
# For entire project
make docs

# With specific options
go run cmd/command/main.go doc -dir [directory] -out [output file] [-recursive] [-exclude "dir1,dir2"]
```

Options:
- `-dir`: Target directory (default: current directory)
- `-out`: Output file path (default: DOCUMENTATION.md)
- `-recursive`: Process subdirectories
- `-exclude`: Skip specified directories (comma-separated)

## Explicitly Name Database Constraints

When defining or altering table schemas in `.sql` files that are processed by the `schema/collector` (both in `db/migrations/` and embedded module schemas):

## IMPORTANT NOTE:
Migration tool only supports UNIQUE constraints (other constraints are not yet supported). Keep this in mind when mutating schemas.
Changes in configuration such as altering varchar(255) to varchar(500) or changing datetime from null to now() are not handled.
TODO list of limitations.

**```All constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK) MUST be explicitly named using the CONSTRAINT <constraint_name> syntax.```**

### Reasoning:

The `schema/collector` tool automatically generates `Up` and `Down` migration scripts by comparing schema states. To create correct `DROP CONSTRAINT` commands (especially critical for `Down` migrations and for modifying existing constraints), the tool relies on predictable constraint names. Database auto-generated names are inconsistent and difficult for the tool to determine reliably, leading to potential migration failures. Explicit naming ensures that schema comparisons and generated migrations are accurate and robust.

### Recommended Naming Convention:

Please use the following convention for consistency:

`<table>_<column(s)>_<type_suffix>`

**Suffixes:**

* `_pkey` for Primary Keys
* `_key` for Unique Constraints (please be consistent within the project)
* `_fk` for Foreign Keys
* `_check` for Check Constraints

**Note:** For multi-column constraints, include relevant column names separated by underscores if feasible, or provide a meaningful description.

This documentation provides context for LLMs working on the IOTA-SDK project.

## ❓ Known Issues and Troubleshooting

### Linting Issues

Do not run
```shell
golangci-lint run --fix
```
It will break the code.

When facing an error like this:
```
WARN [runner] Can't run linter goanalysis_metalinter: buildssa: failed to load package : could not load export data: no
export data for "github.com/iota-uz/iota-sdk/modules/core/domain/entities/expense_category"
```
Try running:
```shell
go mod tidy
```

### Windows Setup Issues

1. **Make commands fail**:
   - Install via [GnuWin32](http://gnuwin32.sourceforge.net/packages/make.htm)
   - Add installation directory to PATH
   - Or use Git Bash which includes Make

2. **Docker issues**:
   - Ensure WSL 2 is properly configured
   - Run `wsl --update` as administrator
   - Restart Docker Desktop

3. **Air hot-reloading problems**:
   - Verify Air is in your PATH
   - Check for `.air.toml` configuration
   - Try `air init` to create new configuration

4. **PostgreSQL connection issues**:
   - Ensure Docker is running
   - Check container status: `docker ps`
   - Verify database credentials in `.env`

### Start cloudflare tunnel

To intercept incoming traffic from third-party systems, such as payment gateways, you can use a Cloudflare Tunnel.

Try running:
```bash
make tunnel
```

## 🤖 Using AI

### Claude Code Setup

1. **Install Claude Code**:
   ```bash
   npm install -g @anthropic-ai/claude-code
   ```
   
   See [Claude Code docs](https://docs.anthropic.com/en/docs/claude-code/overview) for more details.

2. **Add MCP servers**:
   ```bash
   # Context7 (documentation and API references)
   claude mcp add -s user --transport http context7 https://mcp.context7.com/mcp

   # Puppeteer (browser automation)
   claude mcp add -s user puppeteer -e 'PUPPETEER_LAUNCH_OPTIONS={"headless":false,"args":["--remote-debugging-port=9222","--remote-debugging-address=127.0.0.1","--user-data-dir=/tmp/mcp-profile","--no-sandbox","--disable-setuid-sandbox"]}' -- npx -y puppeteer-mcp-server@0.7.2 --stdio

   # Go documentation (install first)
   go install github.com/mrjoshuak/godoc-mcp@latest
   claude mcp add -s user godoc-mcp $(go env GOPATH)/bin/godoc-mcp
   
   # Code Indexer (semantic code search)
   # Requires OPENAI_API_KEY and MILVUS_TOKEN environment variables
   # Contact team lead for MILVUS_TOKEN if you don't have it
   claude mcp add code-indexer -e OPENAI_API_KEY=$OPENAI_API_KEY -e MILVUS_ADDRESS=https://in03-2c743230eb1daa2.serverless.gcp-us-west1.cloud.zilliz.com -e MILVUS_TOKEN=$MILVUS_TOKEN -- npx @code-indexer/mcp@latest
   ```

3. **Usage**:
   ```bash
   # List configured MCP servers
   claude mcp list
   
   # Start Claude Code (MCP auto-enabled)
   claude
   ```

### Code Indexing

The repository uses automated code indexing for semantic search. When you push changes, the GitHub Actions workflow automatically indexes the codebase using the `@code-indexer/core` package with OpenAI embeddings.

## 🤝 Communication Guidelines

Contributors should close conversations when complete. Reviewers may reopen if needed.

For additional help, see our [FAQ](./FAQ.md) or open a GitHub issue.
