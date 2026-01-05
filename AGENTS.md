# 请总是用中文回复。

# AGENTS.md（主干 SSOT）

本文件是仓库内“如何开发/如何验证/如何组织文档与规则”的**主干入口**。优先阅读本文件，并通过链接跳转到其他专题文档；避免在多个文档里复制同一套规则，减少漂移。

## 0. TL;DR（最常见变更要跑什么）

- Go 代码：`go fmt ./... && go vet ./... && make check lint && make test`
- `.templ`/Tailwind 相关：`make generate && make css`，然后 `git status --short` 必须为空
- 多语言 JSON：`make check tr`
- 发 PR 前一键对齐 CI（推荐）：`make preflight`
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
| Authz（`config/access/**` / `pkg/authz/**` / `scripts/authz/**` 等） | `make authz-test && make authz-lint` | 另见 `docs/runbooks/authz-policy-apply-api.md` |
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
- 新增数据库表（新建迁移中的 `CREATE TABLE` 或 schema 中新增表）前，必须获得用户手工确认。

### 3.3 契约文档优先（Contract First）

- 新增或调整功能（尤其是 API/数据库/鉴权/交互契约变化）前，必须在 `docs/dev-plans/` 新建或更新相应计划文档（遵循 `docs/dev-plans/000-docs-format.md`，可基于 `docs/dev-plans/001-technical-design-template.md`）。
- 代码变更应是对文档契约的履行：文档是“意图”，代码是“实现”；若实现过程中发生范围/契约变化，应先更新计划文档再改代码。
- 例外：仅修复拼写/格式、或不改变外部行为的极小重构，可不强制新增计划文档；但一旦涉及迁移、权限、接口、数据契约，必须按本条执行。

### 3.4 AI 驱动开发：简单而非容易（Simple > Easy）

使用 AI 辅助时，优先追求“简单（Simple）”而不是“容易（Easy）”：先写清边界、不变量、失败路径与验收标准（建议以 dev-plan/Spec 固化），再实现；拒绝补丁式堆叠分支、复制粘贴与相似文件增殖；任何新抽象必须可在 5 分钟内解释清楚、具备可替换性，并能对应到明确的业务约束（评审清单见 `docs/dev-plans/045-simple-not-easy-review-guide.md`）。

### 3.5 时间语义（Valid Time vs Audit/Tx Time）

- 将“业务生效日期/有效期（Valid Time）”从 `timestamptz`（秒/微秒级）收敛为 **day（date）粒度**，对齐 SAP HCM（`BEGDA/ENDDA`）与 PeopleSoft（`EFFDT/EFFSEQ`）的 HR 习惯；同时明确 **时间戳（秒/微秒级）仅用于操作/审计时间（Audit/Tx Time）**（如 `created_at/updated_at/transaction_time`）。
- 方案与迁移路径：`docs/dev-plans/064-effective-date-day-granularity.md`。

### 3.6 运维与监控（早期阶段）

关于运维与监控，不需要引入开关切换。本项目仍处于初期，未发布上线，避免过度运维和监控。

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
- 管理员直接维护生效（015C）：见 `docs/runbooks/authz-policy-apply-api.md`。

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
- Authz Policy Apply API（015C）：`docs/runbooks/authz-policy-apply-api.md`
- 简化授权管理（DEV-PLAN-015C）：`docs/dev-plans/015C-authz-direct-effective-policy-management.md`
- Transactional Outbox（relay/cleaner/排障）：`docs/runbooks/transactional-outbox.md`
- Person sqlc：`docs/runbooks/person-sqlc.md`
- Person Atlas+Goose：`docs/runbooks/person-atlas-goose.md`
- PostgreSQL 17 迁移：`docs/runbooks/postgres17-migration.md`
- DEV-PLAN-061 示例数据集（seed_061）：`docs/runbooks/dev-plan-061-seed-dataset.md`
- 文档规范：`docs/dev-plans/000-docs-format.md`
- R200 工具链落地现状与复用指引（DEV-PLAN-009A）：`docs/dev-plans/009A-r200-tooling-playbook.md`
- 多 worktree 共享本地开发基础设施（DEV-PLAN-011C）：`docs/dev-plans/011C-worktree-shared-local-dev-infra.md`
- Core 用户权限页 IA 优化：`docs/dev-plans/016A-core-users-permissions-ia.md`
- Core 用户权限页签 UI/交互优化：`docs/dev-plans/016B-core-users-permissions-ui.md`
- Authz（Casbin）模块“简单而非容易”评审发现与整改计划（DEV-PLAN-016C）：`docs/dev-plans/016C-authz-simple-not-easy-audit.md`
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
- Org 模块功能目录（DEV-PLAN-020L）：`docs/dev-plans/020L-org-feature-catalog.md`
- Org 测试缺口补齐方案（DEV-PLAN-020T1）：`docs/dev-plans/020T1-org-test-gap-closure-plan.md`
- 制造示例组织树数据集（DEV-PLAN-036）：`docs/dev-plans/036-org-sample-tree-data.md`
- Org UI 页面交互问题调查与改进（DEV-PLAN-037）：`docs/dev-plans/037-org-ui-ux-audit.md`
- Org UI 可视化验收与交互验证（DEV-PLAN-037A）：`docs/dev-plans/037A-org-ui-verification-and-optimization.md`
- Org Atlas+Goose 工具链与门禁（DEV-PLAN-021A）：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- Org UI（DEV-PLAN-035）：`docs/dev-plans/035-org-ui.md`
- Org UI IA 与侧栏集成（DEV-PLAN-035A）：`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
- 移除 finance/billing/crm/projects 模块（DEV-PLAN-040）：`docs/dev-plans/040-remove-finance-billing-crm.md`
- 移除 warehouse（仓库）模块（DEV-PLAN-041）：`docs/dev-plans/041-remove-warehouse-module.md`
- 移除 ru/uz 多语言（DEV-PLAN-042）：`docs/dev-plans/042-remove-ru-uz-locales.md`
- HTMX 操作异常统一反馈（DEV-PLAN-043）：`docs/dev-plans/043-ui-action-error-feedback.md`
- UI 可视化验收与交互验证工具链（DEV-PLAN-044）：`docs/dev-plans/044-frontend-ui-verification-playbook.md`
- AI 驱动开发的“简单而非容易”方案评审指引（DEV-PLAN-045）：`docs/dev-plans/045-simple-not-easy-review-guide.md`
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
- 任职（人员）生效日期 + 操作类型（DEV-PLAN-061A1）：`docs/dev-plans/061A1-person-assignment-effective-date-and-action-type.md`
- 任职记录（Job Data）入口收敛：唯一写入口（DEV-PLAN-062）：`docs/dev-plans/062-job-data-entry-consolidation.md`
- 任职时间线部门/职位名称按时间切片渲染（DEV-PLAN-063）：`docs/dev-plans/063-assignment-timeline-org-labels-by-effective-slice.md`
- 任职经历列表新增“组织长名称”列（DEV-PLAN-063A）：`docs/dev-plans/063A-assignments-timeline-org-long-name-column.md`
- 生效日期（日粒度）统一：Valid Time=DATE，Audit Time=TIMESTAMPTZ（DEV-PLAN-064）：`docs/dev-plans/064-effective-date-day-granularity.md`
- Valid Time 字段唯一性评审（DEV-PLAN-064A）：`docs/dev-plans/064A-effective-on-end-on-dual-track-assessment.md`
- 组织架构详情页显示“组织长名称”（DEV-PLAN-065）：`docs/dev-plans/065-org-node-details-long-name.md`
- 组织/职位/任职时间片删除自动缝补（DEV-PLAN-066）：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`
- 职位分类字段 UI 贯通 + Job Catalog 维护入口（二级目录）（DEV-PLAN-067）：`docs/dev-plans/067-position-classification-ui-and-job-catalog-nav.md`
- 组织长名称投影（SSOT + 批量解析）（DEV-PLAN-068）：`docs/dev-plans/068-org-node-long-name-projection.md`
- Org 结构索引（`org_edges.path`）一致性修复（DEV-PLAN-069）：`docs/dev-plans/069-org-long-name-parent-display-mismatch-investigation.md`
- DEV-PLAN-069 Readiness：`docs/dev-records/DEV-PLAN-069-READINESS.md`
- 基于 `org_edges.path` 生成组织长路径名称（DEV-PLAN-069A）：`docs/dev-plans/069A-org-long-name-generate-from-org-edges-path.md`
- DEV-PLAN-069A Readiness：`docs/dev-records/DEV-PLAN-069A-READINESS.md`
- 调查：组织架构树与组织长名称不一致（DEV-PLAN-069C）：`docs/dev-plans/069C-org-tree-long-name-inconsistency-investigation.md`
- DEV-PLAN-069C Readiness：`docs/dev-records/DEV-PLAN-069C-READINESS.md`
- 在 066 删除/边界变更场景保持 `org_edges.path` 一致性（DEV-PLAN-069B）：`docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`
- DEV-PLAN-069B Readiness：`docs/dev-records/DEV-PLAN-069B-READINESS.md`
- 组织架构页增加“修改记录 / 删除记录”（DEV-PLAN-070）：`docs/dev-plans/070-org-ui-correct-and-delete-records.md`
- Docker 中 PostgreSQL CPU 偏高调查与建议（DEV-PLAN-071）：`docs/dev-plans/071-postgres-docker-high-cpu-investigation.md`
- 对标 Workday 的职位体系（Job Architecture）（DEV-PLAN-072）：`docs/dev-plans/072-job-architecture-workday-alignment.md`
- Job Catalog 二级菜单中文名修正为“职位分类” + 职种创建补齐（DEV-PLAN-072A）：`docs/dev-plans/072A-job-catalog-zh-nav-rename-and-job-family-create.md`
- DEV-PLAN-073 任职记录页增加字段（职类/职种/职位模板/职级）：`docs/dev-plans/073-job-data-add-job-architecture-fields.md`
- DEV-PLAN-074 取消职位模板职种“百分比分配”（保留多职种配置）：`docs/dev-plans/074-remove-job-profile-allocation-percent.md`
- 职位分类（Job Catalog）主数据属性 Effective Dating：切片化 + 同步展示 + 复用抽象（DEV-PLAN-075）：`docs/dev-plans/075-job-catalog-effective-dated-attributes.md`
- 职位分类（Job Catalog）identity legacy 列退场评估与清理（DEV-PLAN-075A）：`docs/dev-plans/075A-job-catalog-identity-legacy-columns-retirement.md`
- Org 模块现状研究与 v4（事务性事件溯源 + 同步投射）差异评估（DEV-PLAN-076）：`docs/dev-plans/076-org-v4-transactional-event-sourcing-gap-analysis.md`
- Org v4（事务性事件溯源 + 同步投射）完整方案（Greenfield）（DEV-PLAN-077）：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`
- [Archived] Org v4 全量替换（历史参考，已不适用）（DEV-PLAN-078）：`docs/Archived/078-org-v4-full-replacement-no-compat.md`
- Position v4（事务性事件溯源 + 同步投射）方案（去掉 org_ 前缀）（DEV-PLAN-079）：`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`
- Job Catalog v4（事务性事件溯源 + 同步投射）方案（去掉 org_ 前缀）（DEV-PLAN-080）：`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`
- 启用 PostgreSQL RLS 强租户隔离（Org/Position/Job Catalog v4）（DEV-PLAN-081）：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`
- DDD 分层框架方案（对齐 CleanArchGuard + v4 DB Kernel）（DEV-PLAN-082）：`docs/dev-plans/082-ddd-layering-framework.md`
- Greenfield HR 模块骨架与契约（OrgUnit/JobCatalog/Staffing/Person）（DEV-PLAN-083）：`docs/dev-plans/083-greenfield-hr-modules-skeleton.md`
- 任职记录（Job Data / Assignments）v4 全新实现（Staffing，事件 SoT + 同步投射）（DEV-PLAN-084）：`docs/dev-plans/084-greenfield-assignment-job-data-v4.md`
- Person 最小身份锚点（Pernr 1-8 位数字字符串，前导 0 同值）以支撑 Staffing 落地（DEV-PLAN-085）：`docs/dev-plans/085-person-minimal-identity-for-staffing.md`
- 引入 Astro（AHA Stack）到 HTMX + Alpine 的 HRMS v4 UI 方案（077-084）（DEV-PLAN-086）：`docs/dev-plans/086-astro-aha-ui-shell-for-hrms-v4.md`
- V4 SuperAdmin 控制面认证与会话（与租户登录链路解耦）（DEV-PLAN-088A）：`docs/dev-plans/088A-superadmin-authn-v4.md`
- 文档收敛实施方案：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`
- 归档区说明：`docs/Archived/index.md`
