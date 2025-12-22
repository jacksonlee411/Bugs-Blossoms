# IOTA SDK

![Dashboard](images/showcase.jpg)

## 🔗 Links

- [Discord Community](https://discord.gg/zKeTEZAQqF): Join our community for support and Q&A
- [Contribution Guide](docs/CONTRIBUTING.MD): Learn how to contribute to the project
- [Developer Rules (SSOT)](AGENTS.md): Local checks, workflows, and doc gates
- [Multi-worktree local Docker isolation](docs/dev-plans/011B-worktree-local-dev-docker-isolation.md): Avoid port/volume conflicts
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

- `quality-gates` workflow enforces formatting, linting, tests, template regeneration, locale checks, and PostgreSQL 17 migration smoke tests.
- Run the matching local commands before pushing: see [Contribution Guide](docs/CONTRIBUTING.MD) and `AGENTS.md`.

## 🔐 Authz

- 策略草稿 API：`docs/runbooks/authz-policy-draft-api.md`
- Bot 流程与应急：`docs/runbooks/AUTHZ-BOT.md`

## 🧩 Person（人员）

- sqlc 工作流：`docs/runbooks/person-sqlc.md`
- Atlas + Goose：`docs/runbooks/person-atlas-goose.md`

## 🐘 PostgreSQL 17（本地）

- 迁移与备份：`docs/runbooks/postgres17-migration.md`

---  

## A Modular and Configurable ERP Solution

**IOTA SDK** is an open-source, modular, and feature-complete **ERP system** designed to adapt to multiple operational
domains (e.g. core hr, internal tooling, and platform operations). Inspired by leading platforms like
Odoo, IOTA SDK provides an intuitive framework to accelerate enterprise operations and unlock efficiency with powerful
tools and integrations.

---  

## 🚀 Key Features

- **Configurable Architecture**: Fully modular components to customize workflows.
- **Modular Business Apps**: Person/Org, BI‑Chat, logging, and more.
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

- **Core HR**: Expand People/Org capabilities for onboarding, payroll, and benefits.
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

## 🧩 Person（人员）sqlc 工作流

`modules/person` 的 SQL 现已统一使用 sqlc 生成。修改 Person 表结构或 SQL 时请遵循以下顺序：

1. 维护迁移或 schema（以 `modules/person/infrastructure/persistence/schema/person-schema.sql` 为准）。
2. 运行 `scripts/db/export_person_schema.sh` 导出最新 schema（可通过 `SKIP_MIGRATE=1` 跳过自动迁移）。
3. 执行 `make sqlc-generate`（或 `make generate`，它会自动调用该目标）以重建 `modules/person/infrastructure/sqlc/**`。
4. `git status --short` 确认没有遗漏的生成文件，再提交 PR。

CI 中的 `person-sqlc` 过滤器会在涉及 Person SQL / schema / sqlc.yaml 变更时强制执行上述流程，并拒绝包含未提交生成结果的 PR。

---

## 🧭 Person（人员）Atlas + Goose 流水线

Person 使用 Atlas 做 plan/lint（质量门禁），使用 goose 执行迁移（`migrations/person/*.sql`，goose 格式）。基线产物：

- Atlas 配置：根目录 `atlas.hcl`（从环境变量 `DB_URL` / `ATLAS_DEV_URL` 读取连接串）
- Schema source：
  - `modules/person/infrastructure/atlas/core_deps.sql`（Person 外键依赖的最小 Core stub）
  - `modules/person/infrastructure/persistence/schema/person-schema.sql`（Person 目标结构）
- 迁移目录：`migrations/person/*.sql`（goose 格式，文件名前缀为数字版本号）
- 工作日志：`docs/dev-records/DEV-PLAN-061-READINESS.md`

推荐流程：

1. **安装 Atlas CLI（一次性）**
   ```bash
   make atlas-install   # 从 GitHub 构建 atlas v0.38.0 并安装到 $(go env GOPATH)/bin
   ```
2. **安装 goose CLI（一次性）**
   ```bash
   make goose-install   # 安装 goose v3.26.0 到 $(go env GOPATH)/bin
   ```
3. **更新 schema 定义**：更新 Person schema SQL（`modules/person/infrastructure/persistence/schema/person-schema.sql`），如涉及外键依赖同步更新 `modules/person/infrastructure/atlas/core_deps.sql`。
3. **Dry-run 计划**：
   ```bash
   # 建议使用独立的 target/dev DB，避免与业务库/测试库互相污染
   DB_NAME=iota_erp_person_atlas ATLAS_DEV_DB_NAME=person_dev make db plan
   ```
   该命令会连接 Postgres 并打印 SQL 计划（stdout），不写入磁盘。
4. **应用迁移（goose）**：
   ```bash
   DB_NAME=iota_erp_person_atlas PERSON_MIGRATIONS=1 make db migrate up
   ```
   当 `PERSON_MIGRATIONS=1` 时，`Makefile` 会调用 `scripts/db/run_goose.sh`，使用 `postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}` 作为 DSN。  
   - 回滚最近一次：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate down`  
   - redo：`GOOSE_STEPS=1 PERSON_MIGRATIONS=1 make db migrate redo`  
   - 状态：`PERSON_MIGRATIONS=1 make db migrate status`
5. **导出 schema & 重建 sqlc**：
   ```bash
   scripts/db/export_person_schema.sh SKIP_MIGRATE=1
   make sqlc-generate
   ```
6. **提交前检查**：
   ```bash
   DB_NAME=iota_erp_person_atlas ATLAS_DEV_DB_NAME=person_dev make db lint
   ```
   `make db lint` 会运行 `atlas migrate lint --env ci --git-base origin/main`，确保缺失迁移 / hash 漂移会被阻止。

> 所有 Atlas/Goose 操作请在对应 Dev-Plan 的 readiness 记录补充命令与结果，方便后续审计和回溯。

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
