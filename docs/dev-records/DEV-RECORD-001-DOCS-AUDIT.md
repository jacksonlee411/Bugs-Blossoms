# DEV-RECORD-001：文档一致性审计与收敛方案（Docs Audit & Consolidation）

**状态**: 草拟中（2025-12-14 06:36 UTC）

范围：仅针对文档信息一致性与维护成本；不涉及代码/配置改动。

## 1. 背景与目标

近期仓库引入/强化了多项基础设施与工具链（CI Quality Gates、DevHub、Authz/Casbin、HRM sqlc + Atlas/Goose、Superadmin 独立部署等），导致多处文档存在重复描述与局部漂移。该方案用于：

- 明确各文档的“职责边界”（谁是单一信息源，谁只做链接/摘要）。
- 修复会误导上手的配置漂移（尤其是端口/启动方式/命令差异）。
- 收敛重复内容，降低后续维护成本与漂移概率。

## 2. 审计对象（含仓库相对路径，避免同名歧义）

本次审计覆盖以下文件（按用户需求全部带仓库相对路径，以仓库根目录为基准）：

- `AGENTS.md`
- `CLAUDE.md`
- `GEMINI.md`
- `README.MD`
- `docs/CONTRIBUTING.MD`
- `docs/SUPERADMIN.md`
- `docs/dev-records/R200r-Go语言ERP系统最佳实践.md`
- `docs/dev-plans/000-docs-format.md`

用于核对“真实实施进展/配置”的关键依据（同样列出仓库相对路径）：

- `devhub.yml`（本地开发服务编排与端口）
- `Makefile`（本地命令入口与流程）
- `pkg/configuration/environment.go`（默认端口/Origin 等运行时配置）
- `internal/server/default.go`（CORS/本地 origin 白名单）
- `cmd/server/main.go`（主服务启动与 GraphQL controller 注册）
- `cmd/superadmin/main.go`（Superadmin 独立服务启动）
- `.env.example`（示例环境变量与默认 DB 端口）
- `compose.dev.yml`（本地 Postgres 暴露端口）

## 3. 现状结论（基于当前仓库真实实现）

### 3.1 技术栈引入与落地情况（与文档的一致性）

已落地（代码/配置可验证）：

- Go 1.24.x 工具链与 CI Quality Gates：`README.MD`、`docs/CONTRIBUTING.MD`、`AGENTS.md`、`CLAUDE.md`、`GEMINI.md`均有描述，且与 `Makefile` 对应。
- 前端栈（HTMX + Templ + Tailwind + Alpine）：与仓库结构、依赖与构建命令一致（`go.mod`、`Makefile`）。
- GraphQL（gqlgen）：存在实际生成代码与 controller（`modules/core/interfaces/graph/generated.go`、`cmd/server/main.go`），因此 README/CONTRIBUTING/CLAUDE 中的 GraphQL 描述是“基本属实”。
- PostgreSQL 17：本地 compose 与 DevHub 配置对齐（`compose.dev.yml`、`devhub.yml`）。
- HRM sqlc + Atlas/Goose 工作流：README/CONTRIBUTING/AGENTS 中有多处说明，且 Makefile 目标存在（`Makefile` 中的 `sqlc-generate`、`db plan`、`db lint`、`HRM_MIGRATIONS=1` 分支）。
- Authz/Casbin（policy pack、.rev、bot、debug API、403 契约）：README/CONTRIBUTING/AGENTS 中描述较完整且与 Makefile 目标一致（`Makefile` 的 `authz-pack`、`authz-test`、`authz-lint`）。
- Superadmin 独立部署：存在独立入口与运行目标（`cmd/superadmin/main.go`、`Makefile` 的 `superadmin` 目标、`docs/SUPERADMIN.md`）。

### 3.2 明确存在的“文档漂移/冲突点”

1) 端口信息不一致，影响上手：
- `docs/CONTRIBUTING.MD` 仍写 Web app 为 `http://localhost:8080`，但仓库默认服务端口为 `3200`（`pkg/configuration/environment.go`，`devhub.yml`）。

2) Superadmin 的本地运行方式与端口示例混乱：
- `docs/SUPERADMIN.md` 多处示例使用 `3000`，但仓库 `make superadmin` 默认走 `4000`（`Makefile`）。
- `docs/SUPERADMIN.md` 多处使用 `go build` 方式示例，而 `AGENTS.md`、`CLAUDE.md`、`GEMINI.md` 明确建议“不要跑 go build（用 go vet）”。这里建议调整为：文档可以保留“构建产物/容器化需要 go build”的场景，但应明确区分“本地开发检查”与“产物构建”。

3) 信息重复严重，导致后续维护成本高：
- `AGENTS.md`、`CLAUDE.md`、`GEMINI.md` 三份文件中存在大量重叠规则/命令/DDD 结构说明，且文本粒度不同，易漂移。
- `README.MD` 同时承担了“对外介绍 + 贡献者指南 + runbook/运维细节”，与 `docs/CONTRIBUTING.MD`、`AGENTS.md` 重叠。

4) “冻结模块政策”仅在一处出现，容易引发对外认知冲突：
- `AGENTS.md` 声明 `modules/billing`、`modules/crm`、`modules/finance` 冻结；但仓库的对外文档（`README.MD`、`docs/CONTRIBUTING.MD`）仍包含相关模块描述/路径示例。需要明确该政策的“适用范围”（内用/阶段性/分支特定），否则会造成贡献者误判。

5) 最佳实践与规范文档的定位模糊：
- `docs/dev-records/R200r-Go语言ERP系统最佳实践.md` 虽位于 `dev-records` 目录，但包含“更新记录”且被多个 Plan 引用为理论基线，实际上扮演了“Living Architecture Guide”的角色。其内容（工具链、分层架构）与 `AGENTS.md` 高度重叠，若不明确两者关系（理论 vs 执法），极易产生漂移。
- `docs/dev-plans/000-docs-format.md` 定义了文档规范，但在 `docs/CONTRIBUTING.MD` 中未被引用，导致新贡献者难以发现该标准。

## 4. 各文档的职责（Purpose）与调整建议

### 4.1 `AGENTS.md`

定位（建议保留为“助手/自动化的硬约束”）：
- 质量门禁与必须执行的本地命令（与 CI 对齐）
- 架构约束（DDD 分层、cleanarchguard、禁止读 *_templ.go 等）
- 高风险操作红线（禁止 git restore/reset/clean 等）
- 特殊工作流（Authz、HRM sqlc、Atlas/Goose）

需要调整（建议）：
- 去重：同一段“Quality Gates/必跑命令”出现多次，建议合并为一处，并明确“命令以 `Makefile` 为准”。
- 解释范围：冻结模块政策建议迁移为“仓库级政策”或“当前阶段/分支政策”，并在对外文档引用或明确“不对外承诺”。

### 4.2 `CLAUDE.md`

定位（建议）：
- 仅保留 Claude/多代理编排相关内容（agent 选择矩阵、工作模式、与本仓库的最短映射）

需要调整（建议）：
- 删除/下沉重复内容：把通用命令、质量门禁、DDD 结构说明等改为引用 `AGENTS.md` 或新增的单一开发文档（见第 5 节）。
- 对齐“唯一事实来源”：端口、启动方式、Make targets 应引用 `devhub.yml` 与 `Makefile`。

### 4.3 `GEMINI.md`

定位（建议）：
- 仅保留 Gemini 使用约束/工具提示（MCP/DevHub 工具说明、注意事项），其余与 `AGENTS.md` 去重。

需要调整（建议）：
- 命令一致性：模板/样式变更不应只写 `make css`，建议对齐 CI 触发链路（`make generate && make css` 或按变更范围说明）。

### 4.4 `README.MD`

定位（建议）：
- 对外“门面”：项目简介、核心特性、技术栈、最短 Quick Start、文档入口链接集合

需要调整（建议）：
- 下沉长流程：把 Authz/HRM/Atlas/Goose/备份迁移等 runbook 级细节迁移到 `docs/`（保留摘要与链接）。

### 4.5 `docs/CONTRIBUTING.MD`

定位（建议）：
- 贡献者“上手 + CI 对齐”单一入口（包含：本地启动、端口、依赖、常用命令矩阵、常见问题）

需要立刻修复（必须）：
- 端口漂移：将 `http://localhost:8080` 更正为与默认配置一致的 `http://localhost:3200`（以 `devhub.yml`、`pkg/configuration/environment.go` 为准）。

### 4.6 `docs/SUPERADMIN.md`

定位（建议）：
- Superadmin 的架构/部署/鉴权/端点与运维说明，作为 README 的外链细文档

需要立刻修复（必须）：
- 本地开发入口与端口：增加“本地开发建议使用 `Makefile` 的 `make superadmin dev`”并明确默认端口（当前为 4000）；
- 区分“本地开发检查 vs 构建产物”：保留 `go build` 场景时，明确其用途是“构建二进制/容器产物”，不要与“本地改动后该跑什么检查”混淆。

### 4.7 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md`

定位（建议）：
- 理论基线与深度参考（Reference）：解释“为什么要这样做”，作为 `AGENTS.md` 的理论支撑。

需要调整（建议）：
- 明确状态：在文件头部声明它是“Living Reference”还是“Historical Record”。若为 Living，需定期与 `AGENTS.md` 同步；若为 Record，应停止更新并新建 `docs/ARCHITECTURE.md`。

### 4.8 `docs/dev-plans/000-docs-format.md`

需要调整（建议）：
- 增加可见性：在 `docs/CONTRIBUTING.MD` 中增加链接，指引编写 Plan/Record 的规范。

## 5. 收敛方案（可执行建议）

### 5.1 设定“单一信息源”（Single Source of Truth）

建议采用以下职责划分：

- 开发/贡献者流程单一信息源：`docs/CONTRIBUTING.MD`（面向外部贡献者，但不复制规则；必要时引用 AGENTS）
- **主干 SSOT（首要阅读入口）**：`AGENTS.md`（所有通用规则、工作流、入口索引与文档地图都在这里；其他文档尽量“薄封装 + 链接”）
- 运行时真实配置单一信息源：`Makefile` 与 `devhub.yml`（文档尽量引用，不复制细节）
- 对外入口：`README.MD`（只做摘要 + 链接）
- Superadmin 专题：`docs/SUPERADMIN.md`
- 架构活体指南（建议新增）：`docs/ARCHITECTURE.md`（承载可持续维护的架构约定；被 AGENTS/CONTRIBUTING 引用）
- 架构/规范历史快照（归档）：`docs/Archived/`（用于保留决策快照类长文，不作为活体 SSOT）

### 5.2 AI 文档合并/撤销策略（择一落地）

建议采用 **“AGENTS.md 为核心，其他为薄封装”** 的策略：
- **保留** `AGENTS.md`：作为 AI 交互的单一事实来源（SSOT），包含所有通用规则、架构约束与命令。
- **收敛** `CLAUDE.md` 与 `GEMINI.md`：删除其中关于 DDD、测试命令、质量门禁的重复描述，仅保留针对特定 LLM 的 Prompt 优化或工具定义（如 MCP 配置）。若无特定内容，可考虑**取消**这两个文件，直接在 AGENTS.md 头部声明适用性。

### 5.3 统一落点（避免“迁移后又长回去”）

为避免重复内容从一个地方迁移到另一个地方后继续膨胀，建议固定信息架构与目标落点如下（本阶段只确定落点与边界，不立刻执行迁移）：

- 对外入口（稳定、低频更新）：`README.MD`
  - 只包含：项目简介、核心特性、最短 Quick Start、关键链接（CONTRIBUTING / SUPERADMIN / runbooks 等）。
  - 不包含：长篇命令矩阵、CI 细节、Authz/HRM/Atlas/Goose 的完整操作手册。
- 贡献者入口（高频更新的“工作指南”）：`docs/CONTRIBUTING.MD`
  - 只包含：环境准备、开发启动方式、CI 对齐命令矩阵、变更触发器（改了什么该跑什么）、常见问题。
  - 不包含：过长的专题 runbook（迁移到 `docs/runbooks/`）。
- 专题手册（按主题拆分，避免 CONTRIBUTING 变成“大杂烩”）：
  - Runbook（可执行流程、应急/排障）：建议统一放在 `docs/runbooks/`（例如 Authz、HRM、Atlas/Goose、备份/迁移等）。
  - Guide（相对稳定的概念/约定）：建议统一放在 `docs/guides/`（例如代码结构、模块约定、文档写作规范入口等）。
  - 静态资源（截图/图表）：建议统一放在 `docs/assets/`（按主题分子目录，避免散落在 `docs/` 根目录）。
- AI/代理规则（“执法”文档）：`AGENTS.md`
  - 作为“必须遵循”的最小集合；其余文档引用它，而不复制它。

> 说明：上述目录 `docs/runbooks/`、`docs/guides/` 如当前不存在，可在执行阶段创建；本方案阶段仅把“应该搬去哪”定死，防止后续迁移随意落在 CONTRIBUTING/README 里导致重复回潮。

### 5.4 建议合并、虚化或归档的文档清单（决策表）

| 文档 | 建议操作 | 理由 | 目标位置 |
| :--- | :--- | :--- | :--- |
| `README.MD` (技术细节部分) | **瘦身/迁移** | 首页包含大量 Authz/HRM/Atlas 实施细节，维护成本高且易漂移 | `docs/CONTRIBUTING.MD` 或 `docs/guides/` |
| `CLAUDE.md` / `GEMINI.md` | **合并/虚化** | 规则与 `AGENTS.md` 高度重复 | `AGENTS.md` |
| `docs/dev-records/R200r...` | **拆分/归档** | 混杂了“历史记录”与“活体架构指南” | 活体部分 -> `docs/ARCHITECTURE.md`<br>历史部分 -> 保持归档 |
| `docs/dev-plans/000-docs-format.md` | **引用** | 孤立存在，未被贡献指南引用 | 在 `docs/CONTRIBUTING.MD` 增加链接 |

### 5.5 README 收敛策略（具体到“保留/移出”）

- `README.MD` 保留：
  - 简短 Quick Start（推荐基于 `make devtools` 或 `devhub`）
  - 关键链接：`docs/CONTRIBUTING.MD`、`docs/SUPERADMIN.md`、Authz/HRM runbook 等
- 将“长篇流程”（Authz、HRM、Atlas/Goose、备份迁移）下沉到 `docs/`，README 只留摘要与跳转。

### 5.6 迁移清单（可执行、可逐条勾选）

该清单只定义“要删什么重复、要加什么引用、要迁移到哪里”，用于把收敛从口号变成可执行步骤。执行时建议按“先修漂移、再去重、再搬迁”的顺序，减少来回返工。

#### 5.6.1 AI/规则文档去重（以 `AGENTS.md` 为 SSOT）

| 来源文件（保留什么） | 需要删除/迁移的重复内容 | 迁移/引用目标 |
| :--- | :--- | :--- |
| `AGENTS.md`（保留：所有硬规则与流程；并升级为主干入口） | 去重内部重复段落（Quality Gates/必跑命令等重复出现）；补齐“入口索引/文档地图/变更触发器/维护政策”，避免规则散落在其他文件 | 仍在 `AGENTS.md` 内合并与补齐；其他文档仅引用（不复制） |
| `CLAUDE.md`（保留：agent 编排/选择矩阵/Claude 特有提示） | 删除：通用命令、质量门禁、DDD 结构、Authz/HRM 工作流的重复段落 | 用链接指向 `AGENTS.md`（规则）与 `docs/CONTRIBUTING.MD`（上手） |
| `GEMINI.md`（保留：Gemini/MCP/DevHub 工具提示） | 删除：通用命令、DDD/架构、质量门禁重复；修正模板/样式命令描述的简化 | 用链接指向 `AGENTS.md` 与 `devhub.yml` |

#### 5.6.2 对外/贡献者文档去重（README 变索引，CONTRIBUTING 变入口）

| 来源文件（保留什么） | 需要移出的重复内容 | 迁移/落点（统一） |
| :--- | :--- | :--- |
| `README.MD`（保留：简介/特性/最短 Quick Start/链接） | 移出：Authz 详细流程、HRM sqlc/Atlas/Goose 全流程、备份/迁移步骤、过长的 CI 说明 | 迁移到 `docs/runbooks/`（流程类）或 `docs/guides/`（约定类）；README 只保留摘要 + 链接 |
| `docs/CONTRIBUTING.MD`（保留：上手+CI 对齐矩阵） | 避免吸收 runbook 长文；只保留“变更触发器矩阵 + 链接” | 作为入口，链接到 `docs/runbooks/` 与 `docs/SUPERADMIN.md` |

#### 5.6.3 “理论基线”文档治理（避免 R200r 成为新漂移源）

| 文件 | 决策点 | 建议执行 |
| :--- | :--- | :--- |
| `docs/dev-records/R200r-Go语言ERP系统最佳实践.md` | （已决策）该文件定位为“历史记录/决策快照”，不再作为活体参考 | 执行阶段直接拆分：将该文件头部标记为 `[Archived]`，并迁移到 `docs/Archived/`；同时在 `docs/ARCHITECTURE.md`（新建）中沉淀需要长期维护的“活体”架构约定，并由 AGENTS 引用 |

### 5.7 变更触发器表（防漂移机制：改了什么必须同步哪里）

该表用于建立“同步义务”，降低端口/命令等关键事实漂移的概率。原则：**凡是事实源变更（Makefile/devhub.yml/env），必须触发文档同步**；文档只做引用/入口，尽量避免复制具体数字与命令细节。

| 事实源变更（以路径为准） | 可能影响的事实 | 必须检查/同步的文档（以路径为准） | 推荐策略（减少复制） |
| :--- | :--- | :--- | :--- |
| `devhub.yml` | 端口、启动命令、服务名 | `docs/CONTRIBUTING.MD`、`README.MD`、`AGENTS.md` | 文档尽量描述“用 devhub 管理”，端口用“见 devhub.yml”而非写死 |
| `Makefile` | make target 名称/行为、质量门禁命令、superadmin/e2e 入口 | `docs/CONTRIBUTING.MD`、`AGENTS.md`、`docs/SUPERADMIN.md` | 文档优先引用 make 目标；避免复制完整脚本细节 |
| `.env.example` | 默认 DB 端口/第三方配置示例 | `docs/CONTRIBUTING.MD`、`docs/SUPERADMIN.md` | 文档引用“按 .env.example 配置”，避免写死 DB_PORT |
| `pkg/configuration/environment.go` | 默认端口/Origin/Env 变量名 | `docs/CONTRIBUTING.MD`、`docs/SUPERADMIN.md` | 文档用环境变量名（PORT/ORIGIN），少写具体数值 |
| `.github/workflows/quality-gates.yml` 或 lint/test 规则变化 | 本地必跑命令集合 | `AGENTS.md`、`docs/CONTRIBUTING.MD` | 在 AGENTS/CONTRIBUTING 里维持“变更触发器矩阵”，不要多点复制 |

### 5.8 文档防漂移规则（写作约束，减少“事实复制”）

建议在 `docs/dev-plans/000-docs-format.md` 或 `docs/guides/` 中明确以下规则，并在 CONTRIBUTING 中链接它（本方案仅提出规则，不立即修改其他文件）：

1. **端口/地址不要写死**：除非是对外固定端口（例如示例域名），本地端口优先引用 `devhub.yml` 或以环境变量 `PORT/ORIGIN` 表达。
2. **命令优先引用 Makefile target**：文档写 `make check lint`，而不是复制其内部执行细节；减少改 Makefile 后需要同步多处文案。
3. **runbook 与入口分离**：CONTRIBUTING 只放入口与矩阵，runbook 放到 `docs/runbooks/`，README 只索引。
4. **“活体”与“归档”必须标注**：归档类长文统一迁移到 `docs/Archived/` 并在标题/文件头部标注 `[Archived]`；需要长期维护的活体指南统一放到 `docs/guides/` 或 `docs/ARCHITECTURE.md` 并标注“维护责任与更新频率”。
5. **新建文档门禁（New Doc Gate）**（防止文档熵增，约束增量）：
   - **放置门禁（Placement）**：禁止在仓库根目录新增 `.md`；根目录仅允许白名单文件：`README.MD`、`AGENTS.md`、`CLAUDE.md`、`GEMINI.md`（如后续撤销 Claude/Gemini，则同步收缩白名单）。
   - **分类存放（Routing）**：新增仓库级文档默认落入 `docs/` 分区：
     - 流程/操作手册/排障/应急：`docs/runbooks/`
     - 概念/约定/架构指南（活体）：`docs/guides/` 或 `docs/ARCHITECTURE.md`
     - 计划/记录：`docs/dev-plans/` 或 `docs/dev-records/`（必须遵循 `docs/dev-plans/000-docs-format.md`）
     - 归档快照（非活体）：`docs/Archived/`（标题/文件头标注 `[Archived]`）
     - 图片/图表/静态资源：`docs/assets/`（避免散落在 `docs/` 根目录或其他目录）
   - **模块级文档豁免（Co-location Exception）**：允许在 `modules/{module}/` 下就近存放与该模块强关联的文档：
     - 允许新增 `modules/{module}/README.md`
     - 允许新增 `modules/{module}/docs/**`（用于模块内的说明、实现细节与图片等）
     - 该豁免仅用于“模块内部细节”，不用于仓库级 runbook/规范/流程（仓库级文档仍归类到 `docs/`）
   - **命名门禁（Naming）**：对“新增文件”生效（不追溯历史文件）：
     - `docs/runbooks/`、`docs/guides/`、`docs/Archived/`：必须使用 `kebab-case.md`，禁止空格/驼峰。
     - `docs/assets/`：目录与文件名建议使用 `kebab-case`（全小写，禁止空格/驼峰），并按主题分子目录（例如 `docs/assets/authz/`、`docs/assets/hrm/`）。
     - `docs/dev-plans/`、`docs/dev-records/`：遵循 `docs/dev-plans/000-docs-format.md` 的编号/命名约定（不强制 kebab-case，避免与现有编号体系冲突）。
   - **可发现性门禁（Discovery）**：任意新增文档必须在 `AGENTS.md` 的 Doc Map/专题入口新增链接，避免产生“孤儿文档”。
   - **门禁落地（Enforcement）**：执行阶段建议新增 `make check docs`（仅在 `.md` / `docs/assets/**` / `modules/**/README.md` / `modules/**/docs/**` 变更时触发），自动校验：
     - 根目录新增 `.md` 是否违反白名单
     - `docs/` 下新增文件是否符合“目录归类 + 命名规则”
     - 新增文档是否被 `AGENTS.md` Doc Map/专题入口链接（仓库级文档）
     - 图片是否落入 `docs/assets/`（仓库级文档）或 `modules/{module}/docs/`（模块级文档）
     并接入 CI 作为质量门禁的一部分。

### 5.9 `AGENTS.md` 主干化（SSOT Trunk）方案

目标：在不改变 `AGENTS.md` 现有“规则/约束”作用的基础上，将其提升为“主要阅读入口”，做到**读 AGENTS 即可通过链接掌握其他关联文件**，并把重复内容从其他文档中收敛回 AGENTS（或收敛为对 Makefile/devhub.yml 的引用）。

#### 5.9.1 AGENTS.md 推荐目录骨架（以“链接替代复制”为核心）

建议将 AGENTS 的内容按以下结构重排（保留现有内容，但组织方式变为“入口 + 可执行规则 + 索引”）：

1. **TL;DR（30 秒）**：最常见变更 → 必跑命令（只写 `make ...` 目标，不展开脚本细节）。
2. **事实源索引（不要复制细节）**：明确并链接到  
   - `Makefile`  
   - `devhub.yml`  
   - `.env.example`  
   - `.github/workflows/quality-gates.yml`  
   - `.golangci.yml`、`.gocleanarch.yml`
3. **变更触发器矩阵（核心）**：改了 X（Go/templ/css/locale/authz/sqlc/atlas/migrations/e2e/superadmin）→ 必跑 Y（make 目标 + 必要的 `go test` 范围）。
4. **硬规则（仓库级合约）**：质量门禁、危险操作红线、生成文件读取禁令、冻结模块政策（需补“适用范围/解除条件/对外披露策略”）。
5. **专题入口（只放摘要 + 链接）**：Authz/HRM/Atlas+Goose/Superadmin/文档规范等。
6. **文档地图（Doc Map）**：列出每个相关文档的“目的/何时阅读/与 AGENTS 关系”，全部使用路径（避免同名歧义）。
7. **维护政策（防回潮）**：新增规则：任何“通用规则/工作流/必跑命令”必须先落到 AGENTS；其他文档只能引用；端口与命令细节默认不在文档写死（引用 Makefile/devhub.yml）。

#### 5.9.2 其他文档“薄封装”化的收敛准则（避免再次复制 AGENTS）

- `CLAUDE.md`：只保留 Claude 专用编排/工具差异；通用规则全部链接到 AGENTS。
- `GEMINI.md`：只保留 Gemini/MCP/DevHub 工具差异；通用规则全部链接到 AGENTS。
- `docs/CONTRIBUTING.MD`：只保留贡献者上手与 CI 对齐矩阵；涉及规则时引用 AGENTS。
- `README.MD`：只做对外索引与链接（包含 AGENTS/CONTRIBUTING/SUPERADMIN/runbooks）。

## 6. 实施步骤（Milestones）

1. [ ] 阶段 0（P0）：SSOT 主干先行（避免回潮）
   - [ ] 重排并主干化 `AGENTS.md`：补齐“事实源索引/触发器矩阵/文档地图/维护政策”，并完成内部去重与冻结政策范围说明。

2. [ ] 阶段 1（P0）：修复误导信息（降低上手成本）
   - [ ] 修复 `docs/CONTRIBUTING.MD` 的 Web app 端口（8080 → 3200）。
   - [ ] 修复 `docs/SUPERADMIN.md` 的本地运行方式与默认端口说明（明确 `make superadmin dev` 与默认 4000；区分开发/构建）。

3. [ ] 阶段 2（P1）：降低重复（减少维护面）
   - [ ] 对 `AGENTS.md` 去重并明确“以 `Makefile`/`devhub.yml` 为准”。
   - [ ] 将 `CLAUDE.md`、`GEMINI.md` 的通用规则改为引用 `AGENTS.md`/`docs/CONTRIBUTING.MD`（保留差异化内容）。
   - [ ] R200r 归档拆分：
     - [X] 创建 `docs/Archived/` 与 `docs/Archived/README.md`（已完成）
     - [ ] 将 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md` 标记为 `[Archived]` 并迁移到 `docs/Archived/`
     - [ ] 新建 `docs/ARCHITECTURE.md` 并沉淀可长期维护的活体架构约定（由 `AGENTS.md` 引用）
   - [ ] 引入“新建文档门禁”：
     - [ ] 创建 `docs/assets/`（仓库级文档静态资源归口）
     - [ ] 新增 `make check docs` 并接入 CI（按 `.md`、`docs/assets/**`、`modules/**/README.md`、`modules/**/docs/**` 变更触发），校验路径/命名/`AGENTS.md` 链接与模块级豁免

4. [ ] 阶段 3（P2）：README 信息架构优化（README 变索引）
   - [ ] 将 `README.MD` 的 runbook 级内容迁移到 `docs/`，README 改为摘要 + 链接集合。

## 7. 验收标准（Definition of Done）

- 新贡献者只按 `docs/CONTRIBUTING.MD` 可以完成：启动本地服务、访问正确端口、运行与 CI 对齐的检查命令。
- `docs/SUPERADMIN.md` 明确区分：本地开发入口（Makefile）与构建/部署入口（Dockerfile/二进制）。
- `AGENTS.md` 不再存在重复段落，且对冻结政策的适用范围有清晰说明。
- `README.MD` 不再承载大量 runbook 细节，改为稳定入口与索引。
