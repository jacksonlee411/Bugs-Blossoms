# DEV-PLAN-096：V4 CI 质量门禁（Quality Gates：Lint/Tests/Routing/E2E）

**状态**: 草拟中（2026-01-05 09:25 UTC）

## 1. 背景与上下文 (Context)

`DEV-PLAN-077`～`DEV-PLAN-095`（当前仓库实际覆盖至 `DEV-PLAN-094`）明确了 V4 的 Greenfield 全新实施路线：不做存量兼容包袱，但必须尽早冻结“可验证的工程契约”，否则实现期会出现：
- 本地/CI/部署版本漂移（不可复现）
- 各模块各写一套脚本/门禁（长期 drift）
- 生成物漏提交、迁移闭环不完整、路由/授权口径不一致（PR 反复返工）

本计划的定位是：为 V4 新仓库建立一套**全面且可执行**的 CI 质量门禁（required checks），并把门禁与本地入口收敛到 `Makefile`/CI workflow 的单一事实源（SSOT）。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] **四大 required checks**：CI 至少包含并强制通过：
  - Code Quality & Formatting
  - Unit & Integration Tests
  - Routing Gates
  - E2E Tests
- [ ] **单一入口 + 可复现**：CI 只编排 `Makefile` 入口；工具链版本以 `DEV-PLAN-087` 的 SSOT 固定（`go.mod`/lockfile/CI workflow）。
- [ ] **生成物一致性门禁**：`.templ`/Tailwind/sqlc/Authz pack/Atlas hash 等生成物必须可复现且必须提交；CI 必须能阻断“漏提交/漂移”。
- [ ] **100% 覆盖率门禁**：对齐 `DEV-PLAN-088/091/092` 的要求，新仓库按 100% 覆盖率门禁执行（口径/范围/排除项必须固化为 SSOT）。
- [ ] **触发器可控（但 required checks 不跳过）**：通过 paths-filter 做“按需执行”，降低耗时；但 required checks 的 job 必须稳定产出可合并的结论（避免因 `skipped` 影响合并）。

### 2.2 非目标（明确不做）

- 不在本计划内引入第二套迁移系统或第二套测试框架（对齐 `DEV-PLAN-090`：Atlas+Goose 闭环为唯一方案）。
- 不在本计划内定义每个模块的完整测试用例清单（由各模块 dev-plan 负责）；本计划只定义门禁框架、入口、触发器与验收口径。
- 不在本计划内提供部署/发布流水线（CD）；仅聚焦 PR/merge 的 CI 质量门禁。

## 2.3 工具链与门禁（SSOT 引用）

> 本计划不复制“命令矩阵/脚本细节”，仅冻结门禁结构与入口；命令与触发器以 SSOT 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`
- 版本基线：`docs/dev-plans/087-v4-tech-stack-and-toolchain-versions.md`
- Astro UI（必选）：`docs/dev-plans/086-astro-aha-ui-shell-for-hrms-v4.md`
- 多语言门禁（仅 en/zh）：`docs/dev-plans/089-i18n-en-zh-only.md`
- Atlas+Goose 闭环：`docs/dev-plans/090-v4-atlas-goose-closed-loop-guide.md`
- sqlc 门禁：`docs/dev-plans/091-sqlc-guidelines-for-v4.md`
- Authz 门禁：`docs/dev-plans/092-v4-authz-casbin-toolchain.md`
- 路由门禁：`docs/dev-plans/094-v4-routing-strategy.md`

本计划实施时通常会命中：
- [ ] CI workflow / Makefile（本计划交付）
- [ ] Go 代码（lint/test/coverage 工具链对齐）
- [ ] DB schema / migrations（Atlas plan/lint + goose smoke）
- [ ] sqlc 生成物（生成一致性门禁）
- [ ] Authz policy 生成物（pack/diff/lint/test）
- [ ] UI（Astro/pnpm）与 E2E（Playwright）
- [X] 文档（本计划 → `make check doc`）

## 3. 总体方案：V4 CI 质量门禁框架 (Quality Gates)

### 3.1 Required Checks：四大门禁（稳定、可预测）

**选定**：CI workflow 以一个聚合工作流（`Quality Gates`）对外暴露四个稳定的 required checks（job name 必须冻结，避免后续改名导致保护规则失效）：
1. Code Quality & Formatting
2. Unit & Integration Tests
3. Routing Gates
4. E2E Tests

约束：
- required checks 的 job **不得通过 job-level `if:` 被跳过**；“按需执行”只能在 job 内部通过 paths-filter 控制步骤（确保 required checks 始终给出可合并结论）。

### 3.2 “本地 = CI”单一入口原则（Makefile 驱动）

**选定**：
- CI workflow 只负责：安装依赖、启动服务（Postgres/Redis）、调用 `Makefile` 目标、收集 artifact。
- 所有门禁逻辑必须封装为可本地复现的 `make ...` 入口（避免 CI/YAML 与本地两套口径）。

建议最小入口（名称以新仓库 SSOT 为准）：
- `make preflight`：等价于 CI 四大门禁的本地聚合入口。
- `make check lint` / `make check fmt`：质量与格式化门禁入口。
- `make test`（或等价）：单测/集成测试入口（含覆盖率门禁）。
- `make check routing`：路由门禁入口（对齐 `DEV-PLAN-094`）。
- `make e2e`（或等价）：E2E 门禁入口（对齐 `DEV-PLAN-086/044` 的可视化验收口径）。

### 3.3 Paths-Filter：触发器分层（只做“是否需要跑”，不做“是否需要合并”）

**选定**：CI 在每个 required check job 内先执行 paths-filter，得到布尔信号，用于控制：
- 是否需要安装某些工具（Atlas/sqlc/pg_format/pnpm）
- 是否需要执行某些昂贵步骤（schema plan/lint、E2E 全量）

约束（停止线）：
- 不允许“绕开 Makefile 入口直接在 CI 写 Atlas/sqlc/authz 命令串”，否则无法保证本地可复现与口径一致。

### 3.3.1 Required Checks 触发矩阵（Full Run vs No-op）

> 目标：把“哪些变更必须跑哪些门禁”提前固化，避免实现期为了省时临时跳过导致 drift。

**约束**：
- required checks 的 job 必须始终给出结论；未命中触发器时只能执行 no-op（例如打印 `no relevant changes` 并退出 0），不得返回 `skipped`。
- 触发器的路径细节以新仓库的 `AGENTS.md` 与 CI workflow filters 为 SSOT；本节只冻结“类别 → required check”的映射。

**变更类别（示例）**：
- `docs`：文档与规范（Doc Map / dev-plans / runbooks）。
- `go`：Go 源码与依赖（`*.go`/`go.mod`/`go.sum`/lint 配置）。
- `ui`：Astro（`DEV-PLAN-086`）与服务端 UI 资源（`.templ`/Tailwind/assets）。
- `i18n`：`en/zh` 翻译资源（`make check tr`）。
- `db`：schema/migrations/atlas/goose（对齐 `DEV-PLAN-090`）。
- `sqlc`：sqlc 配置/queries/schema export（对齐 `DEV-PLAN-091`）。
- `authz`：Casbin model/policy/scripts（对齐 `DEV-PLAN-092`）。
- `routing`：allowlist SSOT 与路由注册（对齐 `DEV-PLAN-094`）。
- `e2e`：Playwright 与 E2E 用例。

| Required Check | Full Run（命中任一类别） | No-op（未命中时） |
| --- | --- | --- |
| Code Quality & Formatting | `docs`（始终）+ `go`/`ui`/`i18n`/`db`/`sqlc`/`authz`/`routing` | 仅跑 docs gate（或等价最小检查），输出“no-op”并退出 0 |
| Unit & Integration Tests | `go`/`db`/`sqlc`/`authz`/`routing` | 打印“no-op（docs-only 等）”，不启动 DB，不跑 tests |
| Routing Gates | `routing`/`go` | 打印“no-op”，不启动 server，退出 0 |
| E2E Tests | `e2e`/`ui`/`routing`/`go`/`i18n` | 打印“no-op（docs-only）”，不安装 pnpm/playwright，退出 0 |

### 3.4 Gate 1：Code Quality & Formatting（含生成物一致性）

**覆盖范围（聚合门禁）**：
- Go：`gofmt`/`go vet`/`golangci-lint`/CleanArchGuard（对齐 `DEV-PLAN-082`）。
- UI：`.templ`/Tailwind 生成物 + Astro（`DEV-PLAN-086`）工程的格式化/构建基线（Node/pnpm 版本对齐 `DEV-PLAN-087`）。
- SQL：SQL 格式化门禁（pg_format，版本口径对齐 `DEV-PLAN-087`）。
- Docs：`make check doc`（新文档门禁）。
- i18n：`make check tr`（仅 en/zh，对齐 `DEV-PLAN-089`）。
- sqlc：命中触发器时强制 `make sqlc-generate` 且 `git status --porcelain` 为空（对齐 `DEV-PLAN-091`）。
- Authz：命中触发器时强制 policy pack/diff/lint/test（对齐 `DEV-PLAN-092`）。
- DB：命中触发器时强制 Atlas plan/lint（以及必要的 hash 校验）（对齐 `DEV-PLAN-090`）。

**统一结果判定（强制）**：
- [ ] CI 结束时必须断言 `git status --porcelain` 为空（生成物一致性门禁）。

#### 3.4.1 Gate-1 边界与停止线（防止“一个 job 塞所有东西”）

**边界（强制）**：
- Gate-1 只做静态检查与生成一致性（fmt/lint/generate/plan/lint/hash/diff），不启动长期运行进程（server/browser），不跑 E2E。
- 运行期/集成类验证必须进入 Gate-2/3/4（避免把“能跑起来”与“能合并”纠缠在一个大 job 里）。

**停止线（任何一条命中则拒绝合并该改动）**：
- 在 CI YAML 内新增一段“独立命令串”来替代 `Makefile` 入口（造成口径双轨）。
- 在 Gate-1 内引入跨模块/跨域的隐式依赖（例如为了让某模块 plan/lint 通过而要求另一个模块的迁移先 apply，且没有在对应 dev-plan 明确依赖顺序与回滚）。
- 通过“临时忽略/跳过某些文件”让生成物一致性检查变绿（生成物漂移必须在源头修复）。

### 3.5 Gate 2：Unit & Integration Tests（含 100% 覆盖率）

**选定**：
- 使用 CI service 启动 PostgreSQL 17 与 Redis（对齐 `DEV-PLAN-087`），按新仓库的 DB Kernel/迁移口径初始化测试库。
- 执行 Go tests（含 integration），并在 CI 中上传覆盖率与关键日志 artifact。

覆盖率门禁要求见 §6。

#### 3.5.1 DB 初始化职责边界（避免两套口径）

**选定**：
- DB 工具链闭环验证（Atlas plan/lint/hash + goose migrate smoke）属于 Gate-1 的“质量门禁”（对齐 `DEV-PLAN-090`）。
- Gate-2 只做“为测试准备数据库并跑 tests”，不得重新发明一套 schema/迁移入口；必须复用同一 `Makefile` 目标（例如 `make db test-reset` 或等价），其底层复用 `scripts/db/*` 的单一实现。

**停止线**：
- 在 Gate-2 的 CI YAML 里直接拼接 atlas/goose 命令（应由 `Makefile` 封装并可本地复现）。

### 3.6 Gate 3：Routing Gates（全局路由契约）

**选定**：路由治理与门禁以 `DEV-PLAN-094` 为 SSOT；CI required check 执行 `make check routing`（或等价）并阻断：
- allowlist SSOT 缺失/损坏/entrypoint 缺失
- 路由分类与 responder 契约不一致（JSON-only vs HTML-only 的返回漂移）

### 3.7 Gate 4：E2E Tests（Playwright，最小稳定集）

**选定**：
- E2E 采用 Playwright（对齐 `DEV-PLAN-087` 版本口径），通过 `Makefile` 入口完成：DB reset → 启动 server → 跑测试 → 产出报告 artifact。
- required check 以“最小稳定集（smoke）”为准入门禁；更大规模用例（长耗时/高波动）通过 nightly 或手动触发运行，避免把不稳定性引入 PR 合并主路径。

## 4. 实施步骤 (Checklist)

1. [ ] 新仓库建立 `Quality Gates` workflow：冻结四个 required checks 的 job 名称，并把 job-level 跳过作为停止线。
2. [ ] 新仓库 `Makefile` 固化门禁入口：`preflight`、`check lint`、`test`、`check routing`、`e2e`（名称以 SSOT 为准），并确保 CI 只调用这些入口。
3. [ ] Code Quality & Formatting：对齐 `DEV-PLAN-087` 的工具版本安装方式，并落地“生成物一致性门禁”（CI 断言 `git status --porcelain` 为空）。
4. [ ] Docs gate：对齐 `DEV-PLAN-000/AGENTS.md` 的新文档门禁；新增文档必须可发现（Doc Map）。
5. [ ] i18n gate：落地 `make check tr`（仅 en/zh），并在命中触发器时 fail-fast（对齐 `DEV-PLAN-089`）。
6. [ ] DB gate（Atlas+Goose）：按模块接入 `plan/lint/migrate smoke`，并把触发器/入口纳入 CI（对齐 `DEV-PLAN-090`）。
7. [ ] sqlc gate：确定 schema 输入 SSOT 与导出脚本，命中触发器时执行 `make sqlc-generate` 并检查生成物一致性（对齐 `DEV-PLAN-091`）。
8. [ ] Authz gate：落地 policy SSOT + pack 产物，并把 diff/lint/test 纳入 CI（对齐 `DEV-PLAN-092`）。
9. [ ] Routing gate：落地 `make check routing` 并加入 required checks（对齐 `DEV-PLAN-094`）。
10. [ ] Unit & Integration Tests：建立可复现的测试库初始化流程（含迁移/seed 的最小集），并在 CI 中稳定运行。
11. [ ] 覆盖率门禁：按 §6 固化口径/范围/排除项与证据记录方式，并接入 CI 阻断。
12. [ ] E2E smoke：固化 Playwright 入口与最小稳定集，上传报告 artifact，并明确 nightly/full-suite 的触发方式。
13. [ ] GitHub 保护规则：把四大 required checks 设置为合并前必须通过（repo settings 层面），并在文档中冻结其名称（避免后续改名）。

## 5. 失败路径与排障（Fail-Fast & Debuggability）

- [ ] 生成物漂移：CI 必须打印 `git status --porcelain` 与差异提示（必要时上传 diff artifact），以便开发者快速定位“漏跑 generate/漏提交”。
- [ ] DB 门禁失败：Atlas/goose/sqlc 的失败日志必须被保留为 artifact（至少包含 plan/lint 输出与迁移日志）。
- [ ] E2E 波动：必须默认输出 trace/screenshot/video（按新仓库 SSOT），并在失败时上传报告。

## 6. 测试与覆盖率（V4 新仓库 100% 门禁）

> 对齐 `DEV-PLAN-088/091/092`：100% 覆盖率门禁应作为“可测性设计”的约束，而不是末尾豁免。

- **覆盖率口径**：[ ] line coverage（如需 branch coverage 另开子计划，避免隐性加码）。
- **统计范围**：[ ] 仅统计手写 Go 代码；必须排除生成物（例如 `*_templ.go`、sqlc 生成文件、mock 生成文件等）。
- **SSOT 落点（强制）**：[ ] 在新仓库新增一个“可审计的覆盖率策略文件”，例如 `config/coverage/policy.yaml`（阈值=100、排除项/范围定义），并提供唯一执行入口（例如 `make check coverage` → `scripts/ci/coverage.sh`）。CI workflow 不得通过 env 临时覆盖阈值/排除项（避免口径漂移）。
- **排除规则约束（停止线）**：[ ] 不允许把核心业务代码移出统计范围以规避 100%；任何新增排除项必须在 PR 中说明理由，并在文档/策略文件中记录（可被 reviewer 审计）。
- **目标阈值**：[ ] 100%（对统计范围内的代码生效）。
- **证据记录**：[ ] CI 上传覆盖率报告 artifact；本地可通过 `make test`（或等价）复现同一口径；关键命令与结果应在执行期登记到对应 readiness 记录（如新仓库采用 `docs/dev-records/DEV-PLAN-096-READINESS.md`）。

## 7. 验收标准 (Acceptance Criteria)

- [ ] 新仓库存在一个聚合 workflow（`Quality Gates`），并对外暴露四个稳定的 required checks（名称冻结）。
- [ ] required checks 不会因路径不命中而变为 `skipped`；“按需执行”仅发生在 job 内步骤级别。
- [ ] `Makefile` 提供本地一键入口（`preflight` 或等价），可复现 CI 的四大门禁。
- [ ] 命中生成物触发器时，CI 能阻断“漏提交/漂移”（`git status --porcelain` 为空为硬约束）。
- [ ] Unit & Integration Tests 在 CI 稳定可复现，并满足 100% 覆盖率门禁（按 §6 的口径）。
- [ ] 覆盖率门禁的阈值/范围/排除项存在可审计的策略文件（例如 `config/coverage/policy.yaml`），且 CI workflow 不再隐式写入阈值/忽略规则。
- [ ] Routing Gates 能阻断 allowlist/分类/返回契约漂移（对齐 `DEV-PLAN-094`）。
- [ ] E2E smoke 在 CI 稳定可复现，失败时具备可用的报告/trace artifact。

## 8. 参考与链接 (Links)

- `docs/dev-plans/086-astro-aha-ui-shell-for-hrms-v4.md`
- `docs/dev-plans/087-v4-tech-stack-and-toolchain-versions.md`
- `docs/dev-plans/088-tenant-and-authn-v4.md`
- `docs/dev-plans/089-i18n-en-zh-only.md`
- `docs/dev-plans/090-v4-atlas-goose-closed-loop-guide.md`
- `docs/dev-plans/091-sqlc-guidelines-for-v4.md`
- `docs/dev-plans/092-v4-authz-casbin-toolchain.md`
- `docs/dev-plans/094-v4-routing-strategy.md`
- `.github/workflows/quality-gates.yml`
