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

**Before pushing**, run the commands that match your changes so you know `quality-gates` will pass:

- Go code: `go fmt ./... && go vet ./... && make check lint`
- Tests: `make test` (or `go test -v ./...`)
- Templates/Tailwind assets: `make generate && make css` followed by `git status --short`
- Locales: `make check tr`
- Schema/migrations: `make db migrate up && make db seed`

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
