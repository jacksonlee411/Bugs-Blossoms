# DEV-PLAN-095：文档创建与过程治理规范（Docs Governance Guide）

**状态**: 草拟中（2026-01-05 10:38 UTC）

> 目标：为后续全新实施（Greenfield，077+）提供一套“文档如何创建、放哪里、如何维护与收敛”的统一口径，减少文档熵增与漂移。  
> 本文不替代任何事实源（SSOT）；它做的是**规则汇总 + 入口索引 + 检查清单**，并明确“写什么/不写什么”。

## 1. 背景与问题（Context）

- 本仓库已建立较强的工具链与门禁（CI Quality Gates、本地 Makefile 入口、DevHub 编排、DB/Authz/Routing 等工作流）。随着能力增长，文档容易出现：
  - 同一事实（端口/命令/版本）多处复制，后续修改导致漂移；
  - 新文档散落、不可发现（孤儿文档），维护责任不清；
  - “活体指南/归档快照/执行记录”混杂，读者无法判断可信度与适用期。
- V4 采用 Greenfield 全新实施，需要在最早期固定“文档治理口径”，让后续 dev-plan/runbook/guides 都能一致收敛。

## 2. 目标与非目标（Goals & Non-Goals）

### 2.1 核心目标

- [ ] 统一文档分类、落点与命名规则，避免新增文档无处安放。
- [ ] 明确事实源（SSOT）与引用规则：文档尽量“薄封装 + 链接”，避免复制端口/命令/流程细节。
- [ ] 固化文档生命周期：草拟 → 评审 → 合并 → 更新状态 → 归档/退场。
- [ ] 提供可执行的检查清单（作者/Reviewer），并对齐文档门禁 `make check doc`。

### 2.2 非目标（本计划不做）

- 不替代 `AGENTS.md` 的仓库级合约与触发器矩阵；不在本文重复其全部细节。
- 不新增或改动门禁脚本；本文仅总结现有规范并给出用法与流程。

## 3. 工具链与门禁（SSOT 引用）

> 入口与触发器以 SSOT 为准，避免本文复制导致 drift。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`
- 文档规范（dev-plan 格式）：`docs/dev-plans/000-docs-format.md`
- 详细设计模板（复杂计划建议使用）：`docs/dev-plans/001-technical-design-template.md`
- 文档一致性审计与收敛（原则来源）：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`

## 4. 事实源（SSOT）与写作约束（防漂移）

### 4.1 单一事实源（谁说了算）

- **规则与入口索引**：`AGENTS.md`（主干 SSOT，含 Doc Map）。
- **命令与流程真实实现**：`Makefile` / `scripts/**`（文档优先引用 make target，而不是复制其内部细节）。
- **本地服务编排与端口**：`devhub.yml`、`compose*.yml`、`.env.example`（文档避免写死端口，优先指向这些文件）。
- **CI 真实门禁**：`.github/workflows/quality-gates.yml`（文档不应与 CI 口径冲突）。

### 4.2 防漂移写作约束（写什么 / 不写什么）

> 原则来源：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`（“文档防漂移规则”）。

- 端口/地址**尽量不写死**：除非是对外固定示例（如 `default.localhost`），本地端口优先引用 `devhub.yml` 或用环境变量名表达（`PORT`/`ORIGIN`/`DB_PORT`）。
- 命令优先写 **Makefile 入口**：写 `make check lint`，而不是复制其内部 `golangci-lint ...` 细节。
- 计划文档（dev-plan）默认不复制“触发器矩阵/命令清单”：只声明命中哪些触发器，并链接到 SSOT。
- **唯一例外：执行记录**（readiness/dev-record）允许写死命令与输出，但必须带时间戳与环境要素，且以可复现为目标。

### 4.3 文档治理不变量（必须始终成立）

- **SSOT 单一**：端口/命令/版本/编排等“可验证事实”必须有唯一事实源（例如 `Makefile`、`devhub.yml`、`.env.example`、CI workflow）；文档默认只引用，不复制。
- **可发现性**：仓库级新增文档必须在 `AGENTS.md` 的 Doc Map 中可被发现；避免“孤儿文档”。
- **类型边界清晰**：dev-plan 是契约（约束未来实现），dev-record 是证据（记录已执行），runbook 是可复现操作；不要混用导致读者误判可信度/适用期。
- **活体 vs 归档明确**：过期或仅供历史参考的内容必须迁移到 `docs/Archived/` 并标注 `[Archived]`；归档不得作为活体 SSOT 被引用。
- **新增文档可被门禁校验**：新增/移动文档必须通过 `make check doc`（命名、落点、入口链接与资源归口一致）。

## 5. 文档类型与落点（Routing & Ownership）

> 目标：用“目录边界”表达维护责任与文档类型，降低漂移成本（对齐 `AGENTS.md` 的 New Doc Gate）。

| 文档类型 | 目录/路径 | 写什么 | 不写什么 | 命名约定 |
| --- | --- | --- | --- | --- |
| 主干规则与入口索引 | `AGENTS.md` | 规则、触发器矩阵、红线、Doc Map | 业务实现细节/长 runbook | 固定文件 |
| 对外入口 | `README.MD` | 摘要 + 入口链接 | 过长流程与细则 | 固定文件 |
| 贡献者上手 | `docs/CONTRIBUTING.MD` | 上手步骤 + 与 CI 对齐矩阵（入口） | 深度 runbook | 固定文件 |
| 活体架构 | `docs/ARCHITECTURE.md` | 长期维护的架构约定 | 临时执行记录 | 固定文件 |
| 概念/约定/参考 | `docs/guides/**` | 相对稳定的指南/约定 | 强时效 runbook | `kebab-case.md` |
| 操作/排障/流程 | `docs/runbooks/**` | 可复现操作步骤、排障手册 | 计划决策正文 | `kebab-case.md` |
| 计划/规格（Contract） | `docs/dev-plans/**` | 目标/非目标/契约/验收/依赖/步骤 | 复制易漂移事实（端口/命令/版本）或脚本实现细节（除非作为执行记录）；把临时 workaround 当契约 | 见 `docs/dev-plans/000-docs-format.md` |
| 记录/Readiness | `docs/dev-records/**` | 时间戳 + 命令 + 结果 + 链接 | 重新定义计划契约 | 以现有模式为准（如 `DEV-PLAN-XXX-READINESS.md`） |
| 仓库级文档资源 | `docs/assets/**` | 截图、图表等 | 代码/配置 | 目录与文件名全小写 `kebab-case` |
| 归档快照（非活体） | `docs/Archived/**` | 历史快照，标题/头部标注 `[Archived]` | 作为活体 SSOT 引用 | `kebab-case.md`（建议） |
| 模块级文档（豁免） | `modules/{module}/README.md`、`modules/{module}/docs/**` | 模块内部实现与说明 | 仓库级规则/流程 | 目录与文件名建议 `kebab-case` |

## 6. 文档创建与更新流程（Process）

### 6.1 选择文档类型（先分流，再写）

1. **要约束未来实现/协作边界** → `docs/dev-plans/`（Contract First，见 `AGENTS.md` 与 `docs/dev-plans/000-docs-format.md`）。
2. **要记录“已经发生/已经验证”的事实**（readiness/执行命令）→ `docs/dev-records/`。
3. **要指导“怎么做/怎么排障”**（可复现步骤）→ `docs/runbooks/`。
4. **要沉淀长期约定/概念/参考** → `docs/guides/` 或 `docs/ARCHITECTURE.md`。
5. **只与某个模块强绑定** → `modules/{module}/README.md` 或 `modules/{module}/docs/**`（模块级豁免）。

### 6.2 创建新文档（Author 流程）

- [ ] 选择目录与文件名（遵循第 5 节）。
- [ ] 写明适用范围与状态（dev-plan 必须包含状态行；见 `docs/dev-plans/000-docs-format.md`）。
- [ ] 明确 SSOT 引用：端口/命令/门禁优先写“引用链接”，而不是写死细节。
- [ ] 更新可发现性（Discovery）：
  - [ ] 仓库级新增文档必须加入 `AGENTS.md` 的 Doc Map；
  - [ ] `docs/guides/` 与 `docs/assets/` 建议同步更新各自 `index.md`（作为目录入口）。
- [ ] 运行文档门禁：`make check doc`。

### 6.3 Contract First（计划驱动实现）

- 必须新建/更新 dev-plan 的场景（摘要；完整口径以 `AGENTS.md` 为准）：
  - DB 迁移/Schema、权限与鉴权（Authz/AuthN）、API/路由/交互契约、工具链/版本基线等。
  - 任何会改变“外部可见行为/可复现环境”的变更，都应先有对应的契约文档。
- 例外：仅修复拼写/格式、或不改变外部行为的极小重构，可不强制新增计划文档。
- 实现若偏离计划：先更新 dev-plan，再改代码（避免“对话驱动/Vibe Coding”）。

### 6.4 合并后的维护（Maintainer 流程）

- [ ] 计划完成时更新状态（`草拟中/进行中/已完成`），并补齐验收证据或 readiness 记录入口。
- [ ] 发现文档过期但仍被引用：优先修正文档或把其迁移到 `docs/Archived/` 并明确退场（同时更新入口链接）。

## 7. 变更触发器与同步义务（摘要）

> 完整说明与背景见：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`。

| 事实源变更（路径） | 你必须检查/同步 | 推荐策略 |
| --- | --- | --- |
| `devhub.yml` / `compose*.yml` | `AGENTS.md`、`docs/CONTRIBUTING.MD`、相关 runbook | 文档不写死端口，引用 SSOT |
| `Makefile` / `scripts/**` | `AGENTS.md`、`docs/CONTRIBUTING.MD`、相关 runbook | 文档用 make target 作为入口 |
| `.env.example` / `pkg/configuration/environment.go` | `docs/CONTRIBUTING.MD`、相关 runbook | 用 env key 表达（PORT/ORIGIN/DB_PORT） |
| `.github/workflows/**` | `AGENTS.md`、`docs/CONTRIBUTING.MD` | 以 CI 为准，减少文档复制 |

## 8. Checklist（作者/Reviewer 可直接使用）

### 8.1 新增文档（Author）

- [ ] 目录选择正确（计划/记录/指南/runbook/归档/模块级）。
- [ ] 文件命名符合约定；根目录未新增 `.md`（白名单除外）。
- [ ] 若涉及命令/端口/版本，优先引用 `Makefile`/`devhub.yml`/`.env.example` 等 SSOT。
- [ ] 已更新 `AGENTS.md` Doc Map（仓库级文档）。
- [ ] `make check doc` 通过。

### 8.2 评审文档（Reviewer）

- [ ] 可发现：从 `AGENTS.md` Doc Map 能找到；无“孤儿文档”。
- [ ] 不漂移：未复制易变事实（端口/命令/版本）或已明确其 SSOT。
- [ ] 边界清晰：适用范围/非目标明确；不会误导为“活体 SSOT”。
- [ ] 可复现：runbook/记录类文档步骤完整（必要时含时间戳/环境）。

## 9. 验收标准（本计划完成定义）

- [ ] 新增本指引文档并纳入 Doc Map。
- [ ] 文档治理规则与 `AGENTS.md` / `DEV-RECORD-001` / `DEV-PLAN-000` 不冲突。
- [ ] `make check doc` 通过。

## 10. 常见违规与修复（最小处置）

- `make check doc` 失败：优先检查是否新增文档未进入 `AGENTS.md` Doc Map、命名/落点不符合约定、或资源未归口到 `docs/assets/` / `modules/{module}/docs/`。
- 出现孤儿文档：把入口补到 `AGENTS.md` Doc Map（仓库级），并视情况补充 `docs/guides/index.md` / `docs/assets/index.md` 的目录入口链接。
- 文档复制了易漂移事实（端口/命令/版本）：改为引用 `devhub.yml`/`Makefile`/`.env.example`/CI workflow；仅在执行记录中写死命令并带时间戳。
- 活体/归档混用：把过期内容迁移到 `docs/Archived/` 并标注 `[Archived]`；同时修正入口链接，避免归档被当作 SSOT。
- 文档类型放错目录：按第 5 节迁移到正确目录（guides/runbooks/dev-plans/dev-records），并同步更新入口索引（Doc Map / index）。
