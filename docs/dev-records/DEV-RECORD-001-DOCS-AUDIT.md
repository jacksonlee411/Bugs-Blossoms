# DEV-RECORD-001：文档一致性审计与收敛方案（Docs Audit & Consolidation）

更新时间：2025-12-14  
范围：仅针对文档信息一致性与维护成本；不涉及代码/配置改动。

## 1. 背景与目标

近期仓库引入/强化了多项基础设施与工具链（CI Quality Gates、DevHub、Authz/Casbin、HRM sqlc + Atlas/Goose、Superadmin 独立部署等），导致多处文档存在重复描述与局部漂移。该方案用于：

- 明确各文档的“职责边界”（谁是单一信息源，谁只做链接/摘要）。
- 修复会误导上手的配置漂移（尤其是端口/启动方式/命令差异）。
- 收敛重复内容，降低后续维护成本与漂移概率。

## 2. 审计对象（含路径，避免同名歧义）

本次审计覆盖以下文件（按用户需求全部带路径）：

- `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`
- `/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`
- `/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md`
- `/home/shangmeilin/Bugs-Blossoms-020/README.MD`
- `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`
- `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md`

用于核对“真实实施进展/配置”的关键依据（同样列出路径）：

- `/home/shangmeilin/Bugs-Blossoms-020/devhub.yml`（本地开发服务编排与端口）
- `/home/shangmeilin/Bugs-Blossoms-020/Makefile`（本地命令入口与流程）
- `/home/shangmeilin/Bugs-Blossoms-020/pkg/configuration/environment.go`（默认端口/Origin 等运行时配置）
- `/home/shangmeilin/Bugs-Blossoms-020/internal/server/default.go`（CORS/本地 origin 白名单）
- `/home/shangmeilin/Bugs-Blossoms-020/cmd/server/main.go`（主服务启动与 GraphQL controller 注册）
- `/home/shangmeilin/Bugs-Blossoms-020/cmd/superadmin/main.go`（Superadmin 独立服务启动）
- `/home/shangmeilin/Bugs-Blossoms-020/.env.example`（示例环境变量与默认 DB 端口）
- `/home/shangmeilin/Bugs-Blossoms-020/compose.dev.yml`（本地 Postgres 暴露端口）

## 3. 现状结论（基于当前仓库真实实现）

### 3.1 技术栈引入与落地情况（与文档的一致性）

已落地（代码/配置可验证）：

- Go 1.24.x 工具链与 CI Quality Gates：`/home/shangmeilin/Bugs-Blossoms-020/README.MD`、`/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`、`/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`、`/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`、`/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md`均有描述，且与 `/home/shangmeilin/Bugs-Blossoms-020/Makefile` 对应。
- 前端栈（HTMX + Templ + Tailwind + Alpine）：与仓库结构、依赖与构建命令一致（`/home/shangmeilin/Bugs-Blossoms-020/go.mod`、`/home/shangmeilin/Bugs-Blossoms-020/Makefile`）。
- GraphQL（gqlgen）：存在实际生成代码与 controller（`/home/shangmeilin/Bugs-Blossoms-020/modules/core/interfaces/graph/generated.go`、`/home/shangmeilin/Bugs-Blossoms-020/cmd/server/main.go`），因此 README/CONTRIBUTING/CLAUDE 中的 GraphQL 描述是“基本属实”。
- PostgreSQL 17：本地 compose 与 DevHub 配置对齐（`/home/shangmeilin/Bugs-Blossoms-020/compose.dev.yml`、`/home/shangmeilin/Bugs-Blossoms-020/devhub.yml`）。
- HRM sqlc + Atlas/Goose 工作流：README/CONTRIBUTING/AGENTS 中有多处说明，且 Makefile 目标存在（`/home/shangmeilin/Bugs-Blossoms-020/Makefile` 中的 `sqlc-generate`、`db plan`、`db lint`、`HRM_MIGRATIONS=1` 分支）。
- Authz/Casbin（policy pack、.rev、bot、debug API、403 契约）：README/CONTRIBUTING/AGENTS 中描述较完整且与 Makefile 目标一致（`/home/shangmeilin/Bugs-Blossoms-020/Makefile` 的 `authz-pack`、`authz-test`、`authz-lint`）。
- Superadmin 独立部署：存在独立入口与运行目标（`/home/shangmeilin/Bugs-Blossoms-020/cmd/superadmin/main.go`、`/home/shangmeilin/Bugs-Blossoms-020/Makefile` 的 `superadmin` 目标、`/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md`）。

### 3.2 明确存在的“文档漂移/冲突点”

1) 端口信息不一致，影响上手：
- `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD` 仍写 Web app 为 `http://localhost:8080`，但仓库默认服务端口为 `3200`（`/home/shangmeilin/Bugs-Blossoms-020/pkg/configuration/environment.go`，`/home/shangmeilin/Bugs-Blossoms-020/devhub.yml`）。

2) Superadmin 的本地运行方式与端口示例混乱：
- `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md` 多处示例使用 `3000`，但仓库 `make superadmin` 默认走 `4000`（`/home/shangmeilin/Bugs-Blossoms-020/Makefile`）。
- `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md` 多处使用 `go build` 方式示例，而 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`、`/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`、`/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md` 明确建议“不要跑 go build（用 go vet）”。这里建议调整为：文档可以保留“构建产物/容器化需要 go build”的场景，但应明确区分“本地开发检查”与“产物构建”。

3) 信息重复严重，导致后续维护成本高：
- `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`、`/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`、`/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md` 三份文件中存在大量重叠规则/命令/DDD 结构说明，且文本粒度不同，易漂移。
- `/home/shangmeilin/Bugs-Blossoms-020/README.MD` 同时承担了“对外介绍 + 贡献者指南 + runbook/运维细节”，与 `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`、`/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md` 重叠。

4) “冻结模块政策”仅在一处出现，容易引发对外认知冲突：
- `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md` 声明 `modules/billing`、`modules/crm`、`modules/finance` 冻结；但仓库的对外文档（`/home/shangmeilin/Bugs-Blossoms-020/README.MD`、`/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`）仍包含相关模块描述/路径示例。需要明确该政策的“适用范围”（内用/阶段性/分支特定），否则会造成贡献者误判。

## 4. 各文档的职责（Purpose）与调整建议

### 4.1 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`

定位（建议保留为“助手/自动化的硬约束”）：
- 质量门禁与必须执行的本地命令（与 CI 对齐）
- 架构约束（DDD 分层、cleanarchguard、禁止读 *_templ.go 等）
- 高风险操作红线（禁止 git restore/reset/clean 等）
- 特殊工作流（Authz、HRM sqlc、Atlas/Goose）

需要调整（建议）：
- 去重：同一段“Quality Gates/必跑命令”出现多次，建议合并为一处，并明确“命令以 `/home/shangmeilin/Bugs-Blossoms-020/Makefile` 为准”。
- 解释范围：冻结模块政策建议迁移为“仓库级政策”或“当前阶段/分支政策”，并在对外文档引用或明确“不对外承诺”。

### 4.2 `/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`

定位（建议）：
- 仅保留 Claude/多代理编排相关内容（agent 选择矩阵、工作模式、与本仓库的最短映射）

需要调整（建议）：
- 删除/下沉重复内容：把通用命令、质量门禁、DDD 结构说明等改为引用 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md` 或新增的单一开发文档（见第 5 节）。
- 对齐“唯一事实来源”：端口、启动方式、Make targets 应引用 `/home/shangmeilin/Bugs-Blossoms-020/devhub.yml` 与 `/home/shangmeilin/Bugs-Blossoms-020/Makefile`。

### 4.3 `/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md`

定位（建议）：
- 仅保留 Gemini 使用约束/工具提示（MCP/DevHub 工具说明、注意事项），其余与 `AGENTS.md` 去重。

需要调整（建议）：
- 命令一致性：模板/样式变更不应只写 `make css`，建议对齐 CI 触发链路（`make generate && make css` 或按变更范围说明）。

### 4.4 `/home/shangmeilin/Bugs-Blossoms-020/README.MD`

定位（建议）：
- 对外“门面”：项目简介、核心特性、技术栈、最短 Quick Start、文档入口链接集合

需要调整（建议）：
- 下沉长流程：把 Authz/HRM/Atlas/Goose/备份迁移等 runbook 级细节迁移到 `/home/shangmeilin/Bugs-Blossoms-020/docs/`（保留摘要与链接）。

### 4.5 `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`

定位（建议）：
- 贡献者“上手 + CI 对齐”单一入口（包含：本地启动、端口、依赖、常用命令矩阵、常见问题）

需要立刻修复（必须）：
- 端口漂移：将 `http://localhost:8080` 更正为与默认配置一致的 `http://localhost:3200`（以 `/home/shangmeilin/Bugs-Blossoms-020/devhub.yml`、`/home/shangmeilin/Bugs-Blossoms-020/pkg/configuration/environment.go` 为准）。

### 4.6 `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md`

定位（建议）：
- Superadmin 的架构/部署/鉴权/端点与运维说明，作为 README 的外链细文档

需要立刻修复（必须）：
- 本地开发入口与端口：增加“本地开发建议使用 `/home/shangmeilin/Bugs-Blossoms-020/Makefile` 的 `make superadmin dev`”并明确默认端口（当前为 4000）；
- 区分“本地开发检查 vs 构建产物”：保留 `go build` 场景时，明确其用途是“构建二进制/容器产物”，不要与“本地改动后该跑什么检查”混淆。

## 5. 收敛方案（可执行建议）

### 5.1 设定“单一信息源”（Single Source of Truth）

建议采用以下职责划分：

- 开发/贡献者流程单一信息源：`/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`
- 助手/代理硬约束单一信息源：`/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`
- 运行时真实配置单一信息源：`/home/shangmeilin/Bugs-Blossoms-020/Makefile` 与 `/home/shangmeilin/Bugs-Blossoms-020/devhub.yml`
- 对外入口：`/home/shangmeilin/Bugs-Blossoms-020/README.MD`（只做摘要 + 链接）
- Superadmin 专题：`/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md`

### 5.2 AI 文档合并/撤销策略（择一落地）

选项 A（推荐，风险最低）：保留三份文件但大幅去重  
- `/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md` 与 `/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md` 只保留“工具差异化内容”，通用规范用链接引用 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`。

选项 B（更激进）：只保留一份 AI 规则入口  
- 将通用规则收敛到 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md`，`/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md` 与 `/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md` 其中一份撤销或缩减为“指向 AGENTS 的短文件”。

### 5.3 README 收敛策略

- `/home/shangmeilin/Bugs-Blossoms-020/README.MD` 保留：
  - 简短 Quick Start（推荐基于 `make devtools` 或 `devhub`）
  - 关键链接：`/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD`、`/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md`、Authz/HRM runbook 等
- 将“长篇流程”（Authz、HRM、Atlas/Goose、备份迁移）下沉到 `docs/`，README 只留摘要与跳转。

## 6. 分阶段落地步骤（建议）

阶段 1（必须，优先级 P0：修复误导信息）
- 修复 `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD` 的 Web app 端口（8080 → 3200）。
- 修复 `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md` 的本地运行方式与默认端口说明（明确 `make superadmin dev` 与默认 4000；区分开发/构建）。

阶段 2（P1：降低重复）
- 对 `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md` 去重并明确“以 Makefile/devhub.yml 为准”。
- 将 `/home/shangmeilin/Bugs-Blossoms-020/CLAUDE.md`、`/home/shangmeilin/Bugs-Blossoms-020/GEMINI.md` 的通用规则改为引用 AGENTS/CONTRIBUTING（保留差异化内容）。

阶段 3（P2：README 信息架构优化）
- 将 `/home/shangmeilin/Bugs-Blossoms-020/README.MD` 的 runbook 级内容迁移到 `docs/`，README 改为摘要 + 链接集合。

## 7. 验收标准（Definition of Done）

- 新贡献者只按 `/home/shangmeilin/Bugs-Blossoms-020/docs/CONTRIBUTING.MD` 可以完成：启动本地服务、访问正确端口、运行与 CI 对齐的检查命令。
- `/home/shangmeilin/Bugs-Blossoms-020/docs/SUPERADMIN.md` 明确区分：本地开发入口（Makefile）与构建/部署入口（Dockerfile/二进制）。
- `/home/shangmeilin/Bugs-Blossoms-020/AGENTS.md` 不再存在重复段落，且对冻结政策的适用范围有清晰说明。
- `/home/shangmeilin/Bugs-Blossoms-020/README.MD` 不再承载大量 runbook 细节，改为稳定入口与索引。
