# 请总是用中文回复。

# AGENTS.md（主干 SSOT）

本文件是仓库内“如何开发/如何验证/如何组织文档与规则”的**主干入口**。优先阅读本文件，并通过链接跳转到其他专题文档；避免在多个文档里复制同一套规则，减少漂移。

## 0. TL;DR（最常见变更要跑什么）

- Go 代码：`go fmt ./... && go vet ./... && make check lint && make test`
- `.templ`/Tailwind 相关：`make generate && make css`，然后 `git status --short` 必须为空
- 多语言 JSON：`make check tr`
- 迁移/Schema：`make db migrate up && make db seed`（可选 `make db migrate down`）
- Authz：`make authz-test && make authz-lint`（以及相关包的 `go test`）
- 文档新增/整理：`make check doc`

> 说明：命令细节以 `Makefile` 为准；本文件只维护“入口与触发器”，尽量不复制脚本内部实现。

## 1. 事实源（不要复制细节，统一引用）

- 本地开发服务编排：`devhub.yml`
- 本地命令入口：`Makefile`
- 示例环境变量：`.env.example`
- CI 门禁：`.github/workflows/quality-gates.yml`
- Lint/架构约束：`.golangci.yml`、`.gocleanarch.yml`

## 2. 变更触发器矩阵（与 CI 对齐）

| 你改了什么 | 本地必跑 | 备注 |
| --- | --- | --- |
| 任意 Go 代码 | `go fmt ./... && go vet ./... && make check lint && make test` | 不要仅跑 `gofmt`/`go test`，它们覆盖不到 CI lint |
| `.templ` / Tailwind / presentation assets | `make generate && make css` + `git status --short` | 生成物必须提交，否则 CI 会失败 |
| `modules/**/presentation/locales/**/*.json` | `make check tr` | |
| `migrations/**` 或 `modules/**/infrastructure/persistence/schema/**` | `make db migrate up && make db seed` | CI 会跑 PG17 + migrate smoke |
| Authz（`config/access/**` / `pkg/authz/**` / `scripts/authz/**` 等） | `make authz-test && make authz-lint` | 另见 `docs/runbooks/AUTHZ-BOT.md` |
| Person sqlc（`sqlc.yaml` / `modules/person/infrastructure/sqlc/**` 等） | `scripts/db/export_person_schema.sh` + `make sqlc-generate` + `git status --short` | |
| Person Atlas/Goose（`modules/person/infrastructure/atlas/**` / `migrations/person/**` 等） | `make db plan && make db lint` | 另见 `docs/runbooks/person-atlas-goose.md` |
| Org Atlas/Goose（`modules/org/infrastructure/atlas/**` / `modules/org/infrastructure/persistence/schema/**` / `migrations/org/**` 等） | `make org plan && make org lint && make org migrate up` | 另见 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md` |
| 新增/调整文档 | `make check doc` | 门禁见“文档收敛与门禁” |

## 3. 开发与编码规则（仓库级合约）

### 3.1 基本编码风格

- DO NOT COMMENT EXCESSIVELY：用清晰、可读的代码表达意图，不要堆注释。
- 错误处理使用 `pkg/serrors`（遵循项目标准错误类型）。
- UI 交互使用 `pkg/htmx`，优先复用 `components/` 组件。
- NEVER read `*_templ.go`（templ 生成文件不可读且无意义）。
- 不要手动对齐缩进：用 `go fmt`/`templ fmt`/已有工具完成格式化。

### 3.2 工具使用红线

- DO NOT USE `sed` 做文件内容修改。
- 未经用户明确批准，禁止通过 `git checkout --` / `git restore` / `git reset` / `git clean` 丢弃或回退未提交改动。

### 3.3 契约文档优先（Contract First）

- 新增或调整功能（尤其是 API/数据库/鉴权/交互契约变化）前，必须在 `docs/dev-plans/` 新建或更新相应计划文档（遵循 `docs/dev-plans/000-docs-format.md`，可基于 `docs/dev-plans/001-technical-design-template.md`）。
- 代码变更应是对文档契约的履行：文档是“意图”，代码是“实现”；若实现过程中发生范围/契约变化，应先更新计划文档再改代码。
- 例外：仅修复拼写/格式、或不改变外部行为的极小重构，可不强制新增计划文档；但一旦涉及迁移、权限、接口、数据契约，必须按本条执行。

## 4. 架构与目录约束（DDD + CleanArchGuard）

每个模块遵循 DDD 分层，依赖约束由 `.gocleanarch.yml` 定义，`make check lint` 会同时执行 golangci-lint 与 cleanarchguard。

```
modules/{module}/
├── domain/
├── infrastructure/
├── services/
└── presentation/
```

更完整的“活体架构说明”以 `docs/ARCHITECTURE.md` 为准（由本文件引用，不在多处复制）。

## 5. 新增实体（Repository/Service/Controller）最短路径

1. Domain：`modules/{module}/domain/aggregates/{entity_name}/`（接口、实现、事件、Repository 接口）
2. Infrastructure：`modules/{module}/infrastructure/persistence/`（model、repo 实现、mappers）
3. Services：`modules/{module}/services/`（构造器 `NewEntityService(repo, eventPublisher)`）
4. Presentation：controller/DTO/viewmodel/mapper + `.templ` 页面
5. Locales：`modules/{module}/presentation/locales/**`
6. 注册：`modules/{module}/links.go` + `modules/{module}/module.go`
7. 验证：按“变更触发器矩阵”跑命令

## 6. Authz（Casbin）工作流（摘要）

- 政策碎片：修改 `config/access/policies/**` 后运行 `make authz-pack`（会生成 `config/access/policy.csv` 与 `config/access/policy.csv.rev`，不要手改聚合文件）。
- 测试与校验：Authz 相关改动必须跑 `make authz-test && make authz-lint`。
- Bot：见 `docs/runbooks/AUTHZ-BOT.md`。

## 7. Person/Org（sqlc / Atlas+Goose）工作流（摘要）

- sqlc：影响 `sqlc.yaml` / `modules/person/infrastructure/sqlc/**` / `modules/person/infrastructure/persistence/**/*.sql` 时：先 `scripts/db/export_person_schema.sh`，再 `make sqlc-generate`，最后 `git status --short` 必须为空。
- Atlas/Goose：Person schema 由 `atlas.hcl` 的 `src`（SQL 文件组合）为权威；`make db plan` 做 dry-run，`make db lint` 跑 atlas lint；执行 Person 迁移用 `PERSON_MIGRATIONS=1 make db migrate up`。
- Atlas/Goose（Org）：命中 `modules/org/infrastructure/**` 或 `migrations/org/**` 时：按 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md` 的口径执行 `make org plan && make org lint && make org migrate up`；并确保 Org 使用独立 goose 版本表（例如 `GOOSE_TABLE=goose_db_version_org`）。

## 8. 文档收敛与门禁（New Doc Gate）

目标：防止文档熵增；新增文档必须可发现、可归类、可维护。

- 仓库根目录禁止新增 `.md`（白名单：`README.MD`、`AGENTS.md`、`CLAUDE.md`、`GEMINI.md`）。
- 仓库级文档分类：
  - 操作/排障：`docs/runbooks/`
  - 概念/架构/参考：`docs/guides/` 或 `docs/ARCHITECTURE.md`
  - 计划/记录：`docs/dev-plans/`、`docs/dev-records/`（遵循 `docs/dev-plans/000-docs-format.md`）
  - 静态资源（截图/图表）：`docs/assets/`
  - 归档快照：`docs/Archived/`（标题/头部标注 `[Archived]`，不作为活体 SSOT）
- 模块级豁免（就近存放实现细节）：
  - 允许 `modules/{module}/README.md`
  - 允许 `modules/{module}/docs/**`（含模块内图片）
- 命名（新增文件）：
  - `docs/runbooks/`、`docs/guides/`、`docs/Archived/`：`kebab-case.md`
  - `docs/assets/`：目录与文件名建议全小写 `kebab-case`（图片也同理）
- 可发现性：新增仓库级文档必须在本文件的“文档地图（Doc Map）”中新增链接。
- 门禁：`make check doc`（执行阶段由 CI 触发，仅在文档/资源变更时运行）。

## 9. 模块冻结政策（已移除）

- 历史冻结模块 `modules/billing`、`modules/crm`、`modules/finance` 已在 DEV-PLAN-040 中被 Hard Delete；仓库不再保留针对冻结模块的门禁豁免/排除口径。
- 如将来需要引入“冻结快照”类模块，必须先通过 `docs/dev-plans/` 明确范围、门禁与回滚策略，避免默认破窗。

## 10. 文档地图（Doc Map）

- 对外入口：`README.MD`（摘要 + 链接索引）
- 贡献者指南：`docs/CONTRIBUTING.MD`（上手与 CI 对齐矩阵）
- Superadmin：`docs/SUPERADMIN.md`（独立部署与本地开发入口）
- 活体架构：`docs/ARCHITECTURE.md`
- Guides 入口：`docs/guides/index.md`
- 静态资源约定：`docs/assets/index.md`
- Authz Policy Draft API：`docs/runbooks/authz-policy-draft-api.md`
- Authz Bot：`docs/runbooks/AUTHZ-BOT.md`
- Transactional Outbox（relay/cleaner/排障）：`docs/runbooks/transactional-outbox.md`
- Person sqlc：`docs/runbooks/person-sqlc.md`
- Person Atlas+Goose：`docs/runbooks/person-atlas-goose.md`
- PostgreSQL 17 迁移：`docs/runbooks/postgres17-migration.md`
- DEV-PLAN-061 示例数据集（seed_061）：`docs/runbooks/dev-plan-061-seed-dataset.md`
- 文档规范：`docs/dev-plans/000-docs-format.md`
- R200 工具链落地现状与复用指引（DEV-PLAN-009A）：`docs/dev-plans/009A-r200-tooling-playbook.md`
- Core 用户权限页 IA 优化：`docs/dev-plans/016A-core-users-permissions-ia.md`
- Core 用户权限页签 UI/交互优化：`docs/dev-plans/016B-core-users-permissions-ui.md`
- Transactional Outbox 工具链（DEV-PLAN-017）：`docs/dev-plans/017-transactional-outbox.md`
- DEV-PLAN-017 Readiness：`docs/dev-records/DEV-PLAN-017-READINESS.md`
- DEV-PLAN-029 Readiness：`docs/dev-records/DEV-PLAN-029-READINESS.md`
- DEV-PLAN-030 Readiness：`docs/dev-records/DEV-PLAN-030-READINESS.md`
- DEV-PLAN-031 Readiness：`docs/dev-records/DEV-PLAN-031-READINESS.md`
- DEV-PLAN-033 Readiness：`docs/dev-records/DEV-PLAN-033-READINESS.md`
- DEV-PLAN-034 Readiness：`docs/dev-records/DEV-PLAN-034-READINESS.md`
- DEV-PLAN-035 Readiness：`docs/dev-records/DEV-PLAN-035-READINESS.md`
- 全局路由策略统一（DEV-PLAN-018）：`docs/dev-plans/018-routing-strategy.md`
- DEV-PLAN-018A 路由策略评审输入：`docs/dev-plans/018A-routing-strategy-review.md`
- DEV-PLAN-018B 路由策略门禁启动方案：`docs/dev-plans/018B-routing-strategy-gates.md`
- 多租户工具链选型与落地（DEV-PLAN-019）：`docs/dev-plans/019-multi-tenant-toolchain.md`
- RLS 强租户隔离（DEV-PLAN-019A）：`docs/dev-plans/019A-rls-tenant-isolation.md`
- ORY Kratos 接入与会话桥接（DEV-PLAN-019B）：`docs/dev-plans/019B-ory-kratos-session-bridge.md`
- Jackson 企业 SSO（DEV-PLAN-019C）：`docs/dev-plans/019C-jackson-enterprise-sso.md`
- 多租户管理页面可视化管理（DEV-PLAN-019D）：`docs/dev-plans/019D-multi-tenant-management-ui.md`
- 制造示例组织树数据集（DEV-PLAN-036）：`docs/dev-plans/036-org-sample-tree-data.md`
- Org Atlas+Goose 工具链与门禁（DEV-PLAN-021A）：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- Org UI（DEV-PLAN-035）：`docs/dev-plans/035-org-ui.md`
- Org UI IA 与侧栏集成（DEV-PLAN-035A）：`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
- 移除 finance/billing/crm/projects 模块（DEV-PLAN-040）：`docs/dev-plans/040-remove-finance-billing-crm.md`
- 移除 warehouse（仓库）模块（DEV-PLAN-041）：`docs/dev-plans/041-remove-warehouse-module.md`
- 移除 ru/uz 多语言（DEV-PLAN-042）：`docs/dev-plans/042-remove-ru-uz-locales.md`
- 职位管理业务需求（DEV-PLAN-050）：`docs/dev-plans/050-position-management-business-requirements.md`
- 职位管理实施蓝图（DEV-PLAN-051）：`docs/dev-plans/051-position-management-implementation-blueprint.md`
- DEV-PLAN-051 Readiness：`docs/dev-records/DEV-PLAN-051-READINESS.md`
- 职位管理子计划：契约冻结（DEV-PLAN-052）：`docs/dev-plans/052-position-contract-freeze-and-decisions.md`
- 职位管理子计划：Position Core（DEV-PLAN-053）：`docs/dev-plans/053-position-core-schema-service-api.md`
- 职位管理子计划：Position 合同字段贯通（DEV-PLAN-053A）：`docs/dev-plans/053A-position-contract-fields-pass-through.md`
- 职位管理子计划：Authz（DEV-PLAN-054）：`docs/dev-plans/054-position-authz-policy-and-gates.md`
- 职位管理子计划：UI（DEV-PLAN-055）：`docs/dev-plans/055-position-ui-org-integration.md`
- 职位管理子计划：主数据与限制（DEV-PLAN-056）：`docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`
- 职位管理子计划：报表与运营（DEV-PLAN-057）：`docs/dev-plans/057-position-reporting-and-operations.md`
- 职位管理子计划：任职管理增强（DEV-PLAN-058）：`docs/dev-plans/058-assignment-management-enhancements.md`
- 职位管理子计划：收口与上线（DEV-PLAN-059）：`docs/dev-plans/059-position-rollout-readiness-and-observability.md`
- 职位管理子计划：收口补齐（DEV-PLAN-059A）：`docs/dev-plans/059A-position-rollout-reason-code-mode-and-readiness-smoke.md`
- PeopleSoft Core HR 功能菜单参考（DEV-PLAN-060）：`docs/dev-plans/060-peoplesoft-corehr-menu-reference.md`
- Org-Position-Person 打通与最小人事事件（DEV-PLAN-061）：`docs/dev-plans/061-org-position-person-bridge-and-minimal-personnel-events.md`
- 人员详情页对齐 HR 操作习惯（DEV-PLAN-061A）：`docs/dev-plans/061A-person-detail-hr-ux-improvements.md`
- 文档收敛实施方案：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`
- 归档区说明：`docs/Archived/index.md`
