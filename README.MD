# IOTA SDK

![Dashboard](images/showcase.jpg)

## 🔗 Links

- [Discord Community](https://discord.gg/zKeTEZAQqF): Join our community for support and Q&A
- [Contribution Guide](docs/CONTRIBUTING.MD): Learn how to contribute to the project
- [Setup Demo](https://github.com/iota-uz/sdk-demo): Learn how to setup IOTA-SDK
- [Demo stand](https://demo.iotaerp.com): Explore the SDK UI
- [Docker hub](https://hub.docker.com/r/iotauz/sdk) - Official Docker images
- [Roadmap (Coming soon...)](docs/): Explore our upcoming features and milestones
- [Figma Designs (Coming soon...)](https://www.figma.com/): Access and contribute to our open-source designs

## 🛠 Built With

- [Go](https://go.dev): A statically typed, compiled programming language designed for high performance.
- [htmx](https://htmx.org): Enables modern interactivity with minimal JavaScript.
- [Alpine.js](https://alpinejs.dev): A lightweight JavaScript framework for reactive components.
- [Templ](https://templ.sh): A powerful templating system for building server-rendered web applications.

## ⚙️ Prerequisites

- Go 1.24.10+
- Node.js 20+ (for tooling like Playwright/Tailwind)
- PostgreSQL 17 (local `docker compose` files run `postgres:17` to mirror CI)

## ✅ Quality Gates

- `.github/workflows/quality-gates.yml` runs on every push to `main`/`dev` and on all pull requests. It pins Go 1.24.10, installs templ/Tailwind/golangci-lint/pgformatter, and uploads `coverage.out`, `coverage.html`, and `migrate.log` artifacts.
- Costly checks are scoped to file changes: `.templ`/Tailwind assets trigger `make generate` + `make css` + `git status`, locale JSON triggers `make check tr`, and migrations/schema SQL trigger PostgreSQL 17 + Redis smoke migrations (`make db migrate up/down` + `make db seed`).
- Unit/integration tests always run with PostgreSQL 17/Redis containers, ensuring parity between local development and CI.
- Clean architecture 008 守护：`.gocleanarch.yml` 约束了 modules/*/{domain,services,presentation,infrastructure} 之间的依赖，`make check lint` 和 `quality-gates` 的 lint job 都会运行 go-cleanarch（仅在 Go 代码或该配置文件变更时触发）。

**Before pushing**, run the commands that match your changes so you know `quality-gates` will pass:

- Go code: `go fmt ./... && go vet ./... && make check lint`
- Clean architecture: `.gocleanarch.yml` 描述了 domain/services/presentation/infrastructure 的依赖图谱，`make check lint` 会自动运行 go-cleanarch（本地与 CI 均复用）。
- Tests: `make test` (or `go test -v ./...`)
- Templates/Tailwind assets: `make generate && make css` followed by `git status --short`
- Locales: `make check tr`
- Schema/migrations: `make db migrate up && make db seed`
- Finance 模块的集成测试暂时停用，如需修改 `modules/finance/**` 请在本地运行相关 `go test` 并附上结果。

## 🔐 Authz Policy Draft API

- 修改 `config/access/policies/**` 后运行 `make authz-pack`，它会同时重建聚合策略 `config/access/policy.csv` 以及 `config/access/policy.csv.rev`（供 `pkg/authz/version` 读取 base revision）。
- Core 模块注册了 `PolicyDraftService` 与 `/core/api/authz/**` 控制器，可用来创建/审批/调试策略草稿；所有端点都需要对应的 `Authz.*` 权限。
- 常用端点：
  - `GET /core/api/authz/policies` —— 返回当前聚合策略列表（仅 `Authz.Debug` 可见）。
  - `POST /core/api/authz/requests` —— 创建策略草稿；`POST /requests/{id}/approve|reject|cancel|trigger-bot|revert` 负责生命周期操作。
- `GET /core/api/authz/debug?subject=...&object=...&action=...` —— 调试接口，仅 `Authz.Debug` 可见，支持 `attr.<key>=<value>` 注入 ABAC 属性，并带有 `20 req/min/IP` 速率限制；响应包含 `allowed`、`mode`、`latency_ms`、`trace.matched_policy` 以及回显的 request/attributes。

示例：

```bash
REVISION=$(jq -r .revision config/access/policy.csv.rev)
curl -X POST http://localhost:3200/core/api/authz/requests \
  -H "Content-Type: application/json" \
  -b sid=<session_cookie> \
  -d '{
    "object": "core.users",
    "action": "read",
    "reason": "Grant reporting access",
    "base_revision": "'"$REVISION"'",
    "diff": [{"op":"add","path":"/p/-","value":["role:reporting","core.users","read","global","allow"]}]
  }'
```

Authz.Debug 示例（携带 request id 方便审计）：

```bash
curl "http://localhost:3200/core/api/authz/debug?subject=role:core.superadmin&domain=global&object=core.users&action=list&attr.ip=10.0.0.1" \
  -H "X-Request-Id: authz-debug-sample" \
  -b sid=<session_cookie>
# => {
#   "allowed": true,
#   "mode": "enforce",
#   "latency_ms": 3,
#   "request": {...},
#   "attributes": {"ip":"10.0.0.1"},
#   "trace": {"matched_policy":["role:core.superadmin","*","*","*","allow"]}
# }
```

### Authz Bot & PR 流程

- Bot 可通过 `scripts/authz/bot.sh run [--once]` 启动；需要以下环境变量：
  - `AUTHZ_BOT_GITHUB_TOKEN`：具备 `repo` 权限的 PAT，用于创建/更新 PR。
  - 可选 `AUTHZ_BOT_GIT_TOKEN`：若本地 remote 仍为 SSH，可临时注入 HTTPS 令牌。
  - `AUTHZ_BOT_GIT_REMOTE`/`AUTHZ_BOT_GIT_BRANCH_PREFIX`/`AUTHZ_BOT_GIT_AUTHOR`（如需覆盖默认值）。
- 流程：审批通过的草稿 → 运行 bot → bot 应用 diff、执行 `make authz-pack && make authz-test`、推送分支并调用 GitHub API 开 PR → `PolicyDraftService` 回写 `applied_policy_revision/snapshot/pr_link`，请求状态变为 `merged`。
- 如需强制释放锁，可执行 `scripts/authz/bot.sh force-release <request-id>`，它会清空 `policy_change_requests.bot_lock/bot_locked_at`，便于重试。
- 回滚：调用 `/core/api/authz/requests/{id}/revert` 生成新草稿，或手动 revert bot 创建的 PR，再由 bot 重跑一次。
- 更多手动/应急步骤、缓存策略与回滚说明请参见 `docs/runbooks/AUTHZ-BOT.md`。

### PostgreSQL 17 迁移与备份

1. **备份旧数据**（如果之前运行的是 `postgres:15`）：  
   ```bash
   docker compose -f compose.dev.yml down db
   docker run --rm --network host -v sdk-data:/var/lib/postgresql/data postgres:15 \
     pg_dump -h localhost -p 5438 -U postgres -F c -b -v -f /tmp/pg15-backup.dump iota_erp
   ```
   将 `/tmp/pg15-backup.dump` 复制到安全位置。

2. **清理旧卷并切换到 17**：  
   ```bash
   docker volume rm sdk-data
   docker compose -f compose.dev.yml up -d db redis
   ```

3. **重新迁移与种子**：  
   ```bash
   DB_HOST=localhost DB_PORT=5438 DB_NAME=iota_erp DB_USER=postgres DB_PASSWORD=postgres make db migrate up
   DB_HOST=localhost DB_PORT=5438 DB_NAME=iota_erp DB_USER=postgres DB_PASSWORD=postgres make db seed
   ```

若需要从备份恢复，可使用 `pg_restore` 将 `pg15-backup.dump` 导入新的 `postgres:17` 实例。

---  

## A Modular and Configurable ERP Solution

**IOTA SDK** is an open-source, modular, and feature-complete **ERP system** designed to adapt to multiple economic
sectors, including **finance**, **manufacturing**, and **warehouse management**. Inspired by leading platforms like
Odoo, IOTA SDK provides an intuitive framework to accelerate enterprise operations and unlock efficiency with powerful
tools and integrations.

---  

## 🚀 Key Features

- **Configurable Architecture**: Fully modular components to customize workflows.
- **Industry-Specific Modules**: Finance, manufacturing, warehouse management, and more.
- **GraphQL API**: Simplifies querying and data management.
- **Open Source**: Fully extensible and transparent.

---  

## 📅 Roadmap

We have an ambitious roadmap to enhance IOTA SDK's capabilities and provide a robust framework for ERP needs across
various industries. Here's what's in the pipeline:

### Core Modules

- **Finance & Accounting**: Develop comprehensive tools for managing financial operations, including general ledger,
  accounts payable/receivable, and payroll.

### Work In Progress

- **CRM**: Building a customer relationship management module to streamline client interactions and sales processes.
- **BI‑Chat**: Introducing a business intelligence chatbot for real-time data insights and reporting.

### Upcoming Features

- **HRM**: Integrate a human resources management module for employee onboarding, payroll, and benefits.
- **Password Vault**: Securely store and manage sensitive credentials within the platform.
- **DSL**: Integrate V8/Python for scripting and customizations.
- **Logging**: Implement a logging system for tracking user actions and system events.
- **Invoice and Document Generation**: Leverage [GOBL](https://github.com/invopop/gobl) for generating and managing
  invoices and other essential business documents.
- **Documentation Website**: Launch a comprehensive docs website to streamline onboarding and provide detailed guidance
  for developers and users.
- **Open Source Designs and Documentation**: Share Figma designs and other internal documentation to help the community
  understand the vision and contribute more effectively.

---  

## 🛠️ Developer Tools

### DevHub CLI

DevHub is a powerful Terminal User Interface (TUI) for managing development services. It provides centralized control over all your project's services from a single interface.

**Installation:**
```bash
go install github.com/iota-uz/devhub/cmd/devhub@latest
```

Learn more in the [DevHub documentation](https://github.com/iota-uz/devhub).

---

## 🧩 HRM sqlc 工作流

`modules/hrm` 的 SQL 现已统一使用 sqlc 生成。修改 HRM 表结构或 SQL 时请遵循以下顺序：

1. 维护迁移或 schema，并同步更新 `docs/dev-records/hrm-sql-inventory.md`（新增/调整查询都要登记）。
2. 运行 `scripts/db/export_hrm_schema.sh` 导出最新 schema（可通过 `SKIP_MIGRATE=1` 跳过自动迁移）。
3. 执行 `make sqlc-generate`（或 `make generate`，它会自动调用该目标）以重建 `modules/hrm/infrastructure/sqlc/**`。
4. `git status --short` 确认没有遗漏的生成文件，再提交 PR。

CI 中新增的 `hrm-sqlc` 过滤器会在涉及 HRM SQL / schema / sqlc.yaml 变更时强制执行上述流程，并拒绝包含未提交生成结果的 PR。

---

## 🧭 HRM Atlas + Goose 流水线

HRM 聚合已经迁入 `migrations/hrm/`，并由 Atlas 生成 SQL、goose 负责执行。基线产物：

- 目标 schema：`modules/hrm/infrastructure/atlas/schema.hcl`
- Atlas 配置：根目录 `atlas.hcl`（`dev`/`test`/`ci` 环境复用 `DB_*` 环境变量）
- 迁移目录：`migrations/hrm/changes_<unix>.{up,down}.sql`
- 工作日志：`docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md`

推荐流程：

1. **安装 Atlas CLI（一次性）**
   ```bash
   make atlas-install   # 从 GitHub 构建 atlas v0.38.0 并安装到 $(go env GOPATH)/bin
   ```
2. **更新 schema 定义**：编辑 `modules/hrm/infrastructure/atlas/schema.hcl`（或补充 include），确保与 HRM 期望结构一致。
3. **Dry-run 计划**：
   ```bash
   make db plan
   # 等价于 atlas migrate diff --env dev --dir file://migrations/hrm --to file://modules/hrm/infrastructure/atlas/schema.hcl --dry-run
   ```
   该命令会连接本地 Postgres（使用现有 `DB_*` 环境变量）并打印 SQL 计划，不写入磁盘。
4. **生成迁移**：
   ```bash
   atlas migrate diff --env dev \
     --dir file://migrations/hrm \
     --to file://modules/hrm/infrastructure/atlas/schema.hcl
   ```
   输出文件位于 `migrations/hrm/changes_<unix>.{up,down}.sql`，并自动更新 `migrations/hrm/atlas.sum`。
5. **应用迁移（goose）**：
   ```bash
   make db migrate up HRM_MIGRATIONS=1
   ```
   当 `HRM_MIGRATIONS=1` 时，`Makefile` 会调用 `scripts/db/run_goose.sh`，使用 `postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}` 作为 DSN。  
   - 回滚最近一次：`GOOSE_STEPS=1 make db migrate down HRM_MIGRATIONS=1`  
   - redo：`GOOSE_STEPS=1 make db migrate redo HRM_MIGRATIONS=1`  
   - 状态：`make db migrate status HRM_MIGRATIONS=1`
6. **导出 schema & 重建 sqlc**：
   ```bash
   scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1
   make sqlc-generate
   ```
7. **提交前检查**：`make db lint` 会运行 `atlas migrate lint --env ci --git-base origin/main`，确保缺失迁移 / hash 漂移会被阻止。

> 所有 Atlas/Goose 操作请在 `docs/dev-records/DEV-PLAN-011-HRM-ATLAS-POC.md` 补充记录，方便后续审计和回溯。

---

## 🔐 Super Admin

IOTA SDK includes a separate Super Admin server for managing tenants and platform-wide operations. This deployment is isolated from the main application and requires super admin authentication for all routes.

**Key Features:**
- Separate deployment context (uses same environment variables)
- Only core + superadmin modules loaded (lightweight)
- Global authentication middleware enforcing super admin access
- Tenant management and platform analytics

**Quick Start:**
```bash
# Build Super Admin server
docker build -f Dockerfile.superadmin -t iota-sdk-superadmin .

# Run Super Admin server (uses same env vars as main app)
docker run -p 3000:3000 \
  -e DATABASE_URL=postgres://... \
  -e PORT=3000 \
  iota-sdk-superadmin
```

For detailed documentation, deployment guides, and authentication flow, see [Super Admin Documentation](docs/SUPERADMIN.md).

---

## 📄 Licence

Apache 2.0 

Updated at Jan 6, 2025

## 🌟 Join Us

<img width="250" style="display: block" src="https://cdn.prod.website-files.com/660549b5ac888804e6582845/6606afddd43da2d659209149_logo.svg">  

We welcome contributions to improve and expand IOTA SDK! Check out our [contributing guide](docs/CONTRIBUTING.MD) to get
started.
