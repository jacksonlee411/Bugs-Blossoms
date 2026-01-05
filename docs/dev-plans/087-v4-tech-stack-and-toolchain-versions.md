# DEV-PLAN-087：V4 技术栈与工具链版本冻结（Stack & Tooling Decisions）

**状态**: 草拟中（2026-01-05 07:36 UTC）

> 本文是 V4（全新实施，计划从 077 系列开始）的“技术栈 + 工具链”决策与版本基线文档：**明确我们用什么、用哪个版本、以什么为事实源（SSOT）**，避免本地/CI/部署版本漂移导致不可复现。

## 1. 背景与上下文

- 现有仓库已经形成一套可工作的技术栈与门禁体系（见 `AGENTS.md`/`Makefile`/`.github/workflows/quality-gates.yml`）。
- V4 选择“全新实施”而非改造/迁移旧功能：需要在最早期就把**版本与工具链口径**冻结下来，作为后续 077+ 系列计划的统一依赖。

## 2. 决策范围与原则

### 2.1 范围

- 运行时与基础设施：Go、PostgreSQL、Redis、容器基底镜像。
- UI 技术栈：Templ、HTMX、Alpine.js、Tailwind、核心前端依赖（以 vendored 静态资产为准）。
- 数据与迁移工具链：sqlc、Atlas、Goose、SQL 格式化门禁（pg_format）。
- 授权/路由/事件：Casbin、Routing Gates、Transactional Outbox（能力复用口径）。
- 质量门禁与测试：golangci-lint、go-cleanarch、Go test、Playwright E2E。
- 开发体验：Air、DevHub、Node/pnpm（用于 E2E；若采纳 `DEV-PLAN-086` 的 Astro 壳，也用于 UI build）。
- 部署形态：Docker 镜像与 compose 拓扑（含 superadmin）。

### 2.2 原则（SSOT 与可复现）

- **事实源（SSOT）**：
  - 命令/脚本：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - 本地服务编排：`devhub.yml`、`compose*.yml`
  - 示例环境变量：`.env.example`
  - 版本与依赖：`go.mod`、`e2e/pnpm-lock.yaml`
- **版本冻结粒度**：
  - 开发/构建工具优先固定到**精确版本**（例如 `v0.3.857`）。
  - 容器镜像至少固定到**主版本 tag**（例如 `postgres:17`）；生产环境建议进一步固定 digest（由部署侧落地）。

## 3. V4 版本基线（冻结清单）

> “版本”优先引用仓库内的可验证来源；若某项在仓库内仍是浮动（例如 `:latest`），在表中明确标注为“浮动”，并在第 7 节给出收敛计划。

### 3.1 运行时与基础设施

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| Go | `1.24.10` | `go.mod` + `.github/workflows/quality-gates.yml` |
| PostgreSQL | `17`（`postgres:17`） | `compose.dev.yml`/`compose.yml`/CI service |
| Redis | `latest`（`redis:latest`，浮动） | `compose.dev.yml`/CI service |
| Docker 基底（构建） | `golang:1.24.10-alpine` | `Dockerfile`/`Dockerfile.superadmin` |
| Docker 基底（运行） | `alpine:3.21` | `Dockerfile`/`Dockerfile.superadmin` |

### 3.1.1 PostgreSQL 扩展（V4 Kernel 依赖）

> 说明：`DEV-PLAN-077/079/080` 的 Kernel DDL 大量依赖下表扩展；因此它们属于 V4 的“运行时基线”，必须在 schema SSOT/迁移里显式创建，避免环境漂移。

| 扩展 | V4 基线 | 用途（摘要） | 来源/说明 |
| --- | --- | --- | --- |
| `pgcrypto` | enabled | `gen_random_uuid()` 默认值等 | `migrations/org/00001_org_baseline.sql`、`migrations/person/00001_person_baseline.sql`；以及 `DEV-PLAN-077/079/080` |
| `btree_gist` | enabled | `EXCLUDE USING gist` + `gist_uuid_ops`（no-overlap） | `migrations/org/00001_org_baseline.sql`；以及 `DEV-PLAN-077/079/080` |
| `ltree` | enabled（OrgUnit） | 路径（子树/祖先链）查询 | `migrations/org/00001_org_baseline.sql`；以及 `DEV-PLAN-077` |

### 3.2 UI 技术栈（Server-side Rendering）

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| Templ | `v0.3.857` | `go.mod` + CI 安装步骤 |
| HTMX | `2.0.2` | `modules/core/presentation/assets/js/lib/htmx.min.js` |
| Alpine.js | `3.14.1` | `modules/core/presentation/assets/js/lib/alpine.lib.min.js`（内容内含版本号） |
| Tailwind CLI | `v3.4.13` | CI 安装步骤（Tailwind 二进制下载） |
| ApexCharts | `v4.3.0` | `modules/core/presentation/assets/js/lib/apexcharts.min.js` |
| Flatpickr | `4.6.13` | `modules/core/presentation/assets/js/lib/flatpickr/flatpickr.esm.mjs`（esm.sh 标注） |
| SortableJS | `1.15.6` | `modules/core/presentation/assets/js/lib/sortable.min.js` 头部注释 |

> 说明：除上述核心库外，其余前端第三方库同样以 `modules/core/presentation/assets/js/lib/*` 的 vendored 文件为准；V4 若需要新增/升级前端库，必须在对应 dev-plan 中声明“文件来源、版本与验收方式”，避免静态资产漂移。

### 3.3 数据访问 / Schema / 迁移 / 生成

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| sqlc（CLI） | `v1.28.0` | `Makefile` 的 `sqlc-generate`（CI 会执行该目标） |
| sqlc（Go module） | `v1.30.0` | `go.mod`（工具依赖） |
| Atlas（CLI） | `v0.38.0` | `Makefile` 的 `ATLAS_VERSION`（源码构建安装） |
| Goose（CLI） | `v3.26.0` | `Makefile` 的 `GOOSE_VERSION`（`go install`） |
| pgx（PostgreSQL driver） | `v5.7.5` | `go.mod`（Kernel port/adapters 推荐使用 `pgx`；见 `DEV-PLAN-083/084`） |
| goimports（用于生成物整理） | `v0.26.0` | `Makefile` 的 `sqlc-generate` |
| SQL 格式化（pg_format） | OS 包（未 pin） | CI 安装 `pgformatter`（Ubuntu apt），本地用 `make check sqlfmt` 对齐 |

### 3.4 Authz / Routing / Outbox（能力复用）

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| GraphQL（gqlgen） | `v0.17.57` | `go.mod` |
| Casbin | `v2.88.0` | `go.mod` |
| Routing Gates | 仓库内门禁（无外部版本） | `docs/dev-plans/018-routing-strategy.md` + `make check routing` |
| Transactional Outbox | 仓库内实现（无外部版本） | `docs/dev-plans/017-transactional-outbox.md` + `pkg/outbox/**` |

### 3.5 质量门禁与测试

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| golangci-lint | `v2.7.2` | CI 安装步骤（Quality Gates） |
| go-cleanarch | `v1.2.1` | `go.mod`（`make check lint` 会运行） |
| E2E：Playwright | `@playwright/test@1.55.1` | `e2e/pnpm-lock.yaml` |
| E2E：pnpm | `10.24.0` | `e2e/package.json#packageManager` |
| E2E：Node.js | `20.x`（推荐） | `README.MD`/`.devcontainer/devcontainer.json`；Playwright 最低 `>=18` |

### 3.6 开发体验（可选但推荐）

| 组件 | V4 版本 | 来源/说明 |
| --- | --- | --- |
| Air | `v1.61.5` | `docs/CONTRIBUTING.MD` 与 `.devcontainer/Dockerfile` |
| DevHub CLI | `v0.0.2` | `.devcontainer/Dockerfile`（`devhub.yml` 为编排 SSOT） |
| Docker Engine/Compose | `27.x`（推荐） | `docs/CONTRIBUTING.MD`（实际以团队统一口径为准） |

## 4. 工具链使用口径（V4 统一）

### 4.1 本地命令入口

- 一切以 `Makefile` 为入口；不要绕过 Makefile 直接拼命令写在个人笔记里。
- 变更触发器矩阵与“改什么必须跑什么”：以 `AGENTS.md` 为准。

### 4.2 生成物与门禁

- `.templ` / Tailwind / sqlc 等生成物：**必须提交**，否则 CI 会失败。
- UI/路由/Authz/DB 等“治理型契约”：新增例外属于契约变更，必须先更新对应 dev-plan SSOT 再落代码。

### 4.3 RLS 与 DB Role（V4 运行态契约）

> 说明：v4 Kernel 默认启用 PostgreSQL RLS（fail-closed）；该契约会影响本地开发、CI 与部署口径，因此在工具链决策里显式写出。

- RLS 注入：事务内设置 `app.current_tenant`（`pkg/composables/rls.go`），policy 用 `current_setting('app.current_tenant')::uuid`（fail-closed），对齐 `DEV-PLAN-081`。
- 运行态开关：凡访问 v4 表，`RLS_ENFORCE` 必须为 `enforce`（否则视为配置错误），对齐 `.env.example` 与 `DEV-PLAN-081`。
- DB 账号：应用侧 `DB_USER` 必须为非 superuser，且不可带 `BYPASSRLS`（建议显式 `NOBYPASSRLS`）；superadmin 若需旁路能力，使用独立 role/连接池（见 `DEV-PLAN-088` 的边界）。

## 5. V4 开发环境指引（本地）

> 目的：新人按此文档能完成“启动 + smoke”，细节以 `docs/CONTRIBUTING.MD`/`devhub.yml`/`Makefile` 为准。

1. 安装并确认版本：Go `1.24.10`、Node `20.x`（E2E/工具）、Docker/Compose（推荐 27.x）。
2. 初始化环境变量：复制 `.env.example` 为 `.env`（必要时使用 `make dev-env` 生成 `.env.local`）。
3. 启动依赖服务：使用 `compose.dev.yml` 启动 Postgres/Redis（端口默认 `5438/6379`，以 `devhub.yml` 为准）。
4. 初始化数据库：执行迁移与 seed（入口见 `Makefile`；常用组合见 `AGENTS.md` TL;DR）。
5. 启动开发服务：
   - 方式 A（推荐）：使用 DevHub（`make devtools`）按 `devhub.yml` 一键编排；
   - 方式 B：分别启动 `templ generate --watch`、`make css watch` 与 `air -c .air.toml`（命令与端口以 SSOT 为准）。
6. v4 RLS（若命中 v4 表）：设置 `RLS_ENFORCE=enforce`，并确保 `DB_USER` 为非 superuser（否则 Postgres 会绕过 RLS）。
7. E2E（可选）：进入 `e2e/`，用 `pnpm` 安装依赖并运行 Playwright（要求本地 DB 与 Go server 已启动）。

## 6. V4 部署指引（Docker）

> 目的：明确部署形态与边界；具体运维细节以部署环境规范为准。

- 主应用镜像：`Dockerfile`（运行时基底 `alpine:3.21`）；入口会执行迁移/seed 并启动 server（现状）。
- Superadmin：独立镜像 `Dockerfile.superadmin`；部署与路由边界见 `docs/SUPERADMIN.md`。
- 生产 compose 参考：`compose.yml`（当前示例使用 `postgres:17`，并通过环境变量连接）。
- 版本 pin 建议（V4）：生产环境应把关键镜像（Postgres/Redis/应用镜像）进一步 pin 到 digest，避免“同 tag 不同内容”的不可复现。

## 7. 现状差异（Drift）与收敛计划（V4 必做）

> 说明：本节用于把“当前仓库存在的版本漂移”显式化，并给出 V4 的收敛动作；避免团队在 077+ 过程中继续背负漂移成本。

1. [ ] `golangci-lint`：CI 使用 `v2.7.2`，但本地文档/部分 Dockerfile 仍引用 `v1.64.8` —— 统一到 `v2.7.2` 并更新相关资产。
2. [ ] Tailwind CLI：CI 使用 `v3.4.13`，但 `scripts/install.sh`/`.devcontainer` 使用 `v3.4.15` —— 选择其一作为唯一版本并全量对齐。
3. [ ] sqlc：`go.mod` 为 `v1.30.0`，但 `make sqlc-generate` 固定 `v1.28.0` —— 统一版本并验证生成物无 diff。
4. [ ] goimports：`make sqlc-generate` 固定 `v0.26.0`，但 devcontainer 里为 `v0.31.0` —— 统一版本并对齐格式化输出。
5. [ ] Redis 镜像：当前为 `redis:latest`（浮动）—— 为 V4 生产/CI 口径增加 pin 策略（至少固定 major/minor，推荐 digest）。
6. [ ] DevContainer：当前基底为 Go `1.23`（与 `go.mod` 不一致）—— 视团队是否继续使用 DevContainer，决定升级或移除（参考 `DEV-PLAN-002`）。
7. [ ] Astro（AHA UI Shell，`DEV-PLAN-086`）：若确认为 v4 UI 方案，需要在新仓库 pin `Astro/Node/pnpm`（及其 lockfile），并明确静态资产构建与发布的 SSOT。
8. [ ] ORY Kratos（`DEV-PLAN-088`）：若确认为 v4 AuthN 方案，需要 pin `ory/kratos` 镜像版本（建议 digest）与配置格式，并在本地编排/CI 中加入可复现的启动口径。
9. [ ] 100% 覆盖率门禁（`DEV-PLAN-088`）：新仓库需明确“覆盖率统计口径/排除项/生成物处理/CI 入口”，避免实现期临时拼装导致口径漂移。

## 8. 验收标准（本计划完成定义）

- [ ] 本文中的“V4 版本基线”与仓库 SSOT（`go.mod`/`Makefile`/CI/workflows/compose）一致，且不再出现同一工具多版本并存而无说明。
- [ ] 新人按“第 5 节开发指引”可在本地完成：启动依赖服务 → 迁移+seed → 启动 server → 打开健康检查端点。
- [ ] 077+ 后续计划引用技术栈/工具链时，只引用本文或更细分的 SSOT 文档，不再在多个 dev-plan 里复制版本清单。
