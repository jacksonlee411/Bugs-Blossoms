# DEV-PLAN-093：V4 多工作区并行开发指引（3 worktree 模式 + 共享本地 infra）

**状态**: 草拟中（2026-01-05 09:50 UTC）

> 适用范围：**全新实施的 V4 新代码仓库（Greenfield）**，但本指引可复用于现仓库的多 worktree 工作方式。  
> 目标：在一台开发机上维持 **3 个并行 worktree**（常见：main/feature-a/feature-b 或 main/feature/review），并与 `DEV-PLAN-087` 的本地开发/部署口径对齐（Docker/compose/devhub/端口与版本基线）。

## 1. 背景与上下文 (Context)

- V4 采用 Greenfield 全新实施（从 077 系列开始），研发阶段常见需要同时打开多个分支：一边开发、一边 review/复现、另一边保持 main 同步。
- 多 worktree 的核心收益是：**不需要频繁切分支**，每个分支拥有独立工作目录与生成物，降低上下文切换与“误改错分支”的风险。
- 本地基础设施（Postgres/Redis）不应为每个 worktree 各起一套：资源浪费且端口/配置管理复杂。默认采用 **共享 infra**（参考 `DEV-PLAN-011C`）。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] 给出“3 worktree 并行开发”的推荐拓扑、目录命名与日常工作流。
- [ ] 共享一套本地 Postgres/Redis（固定端口），并说明如何避免端口冲突与破坏性操作。
- [ ] 当确需同时运行多个 server 时，给出最小的端口/环境变量分配口径（避免 cookie/redirect/Origin 漂移）。
- [ ] 明确与 `DEV-PLAN-087` 的对齐点：本地启动与 Docker 部署的边界、风险与建议做法。

### 2.2 非目标（本计划不做）

- 不覆盖“每个 worktree 独立一套 DB/Redis 并行运行”的完整方案（见 `DEV-PLAN-011B`）。
- 不在本计划内新增脚本或修改 compose/Makefile；本指引只给出用法与约束（实施由后续 PR 承接）。

### 2.3 工具链与门禁（SSOT 引用）

> 触发器与命令入口以 SSOT 为准，避免本文复制导致 drift。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- 本地服务编排：`devhub.yml`、`compose.dev.yml`、`compose.yml`
- 示例环境变量：`.env.example`
- 共享 infra 默认方案：`docs/dev-plans/011C-worktree-shared-local-dev-infra.md`
- V4 技术栈/部署口径：`docs/dev-plans/087-v4-tech-stack-and-toolchain-versions.md`

## 3. 推荐拓扑：3 worktree 角色分工

> 一句话：**main 只做同步与基线验证；两个工作 worktree 用于开发与 review/复现。**

- `wt-main`（基线/同步）：
  - 永远保持干净（无本地改动），只做 `git pull --ff-only`、跑门禁、验证合并后的 main。
  - 推荐由它“管理共享 infra”（启动/停止 Postgres/Redis），避免在多个目录里同时操作 docker。
- `wt-dev-a`（开发 A）：当前主要开发分支（feature/bugfix）。
- `wt-dev-b`（开发 B / review）：用于并行任务、代码 review、复现线上/CI 问题，避免打断 A 的上下文。

## 4. 创建 worktree（一次性）

> 说明：以下为示例命令；实际目录命名以团队约定为准。关键点是：所有 worktree 共享同一 `.git` 存储（省空间，且便于并行）。

1. 以 main 为基线创建两个并行目录：
   - `git worktree add ../repo-wt-dev-a -b feature/<topic-a> origin/main`
   - `git worktree add ../repo-wt-dev-b -b feature/<topic-b> origin/main`
2. 查看当前 worktree 列表：
   - `git worktree list`
3. 完成任务后的清理（建议在合并后执行）：
   - `git worktree remove ../repo-wt-dev-a`
   - `git branch -d feature/<topic-a>`

## 5. 共享本地基础设施（Postgres/Redis）

> 默认采用 `DEV-PLAN-011C`：所有 worktree 共享一套 Postgres/Redis，端口固定为 `5438/6379`。

### 5.1 为什么能“从任意目录启动同一套 infra”

关键前提：`compose.dev.yml` 必须固定 `name`（compose project name）。否则 `docker compose` 会按目录推导 project，导致不同 worktree 误起多套或端口冲突（详见 `DEV-PLAN-011C`）。

### 5.2 启动/停止（推荐由 wt-main 执行）

- 启动：
  - `docker compose -f compose.dev.yml up -d db redis`
- 停止（非破坏）：
  - `docker compose -f compose.dev.yml down`

**禁止**：在任意 worktree 随意执行 `docker compose -f compose.dev.yml down -v`（会清空共享数据卷，相当于重置所有 worktree 的本地数据）。

## 6. 每个 worktree 的本地环境变量（避免冲突）

### 6.1 `.env` / `.env.local` 约定

- 每个 worktree 都需要自己的 `.env`（从 `.env.example` 复制），避免“改错目录导致另一分支配置被污染”。
- `.env.local` 仅用于本地覆盖（不提交）；推荐用 `make dev-env` 生成基础模板，再手工微调。

### 6.2 只运行一个 server（默认推荐）

最简单的并行方式是“多 worktree 并行写代码，但同一时刻只跑一个 server”。此时各 worktree 可以共享默认：
- `PORT=3200`
- `DOMAIN=default.localhost`
- `DB_PORT=5438`、`DB_NAME=iota_erp`（共享 DB）

### 6.3 同时运行多个 server（可选）

当确需同时跑 2~3 个 server（例如对比两条分支的行为），需要为每个 worktree 分配不同端口，并同步调整 `ORIGIN`/OAuth redirect 等依赖端口的配置：

| worktree | `PORT` | `ORIGIN`（示例） |
| --- | --- | --- |
| `wt-main`（可选运行） | `3200` | `http://default.localhost:3200` |
| `wt-dev-a` | `3201` | `http://default.localhost:3201` |
| `wt-dev-b` | `3202` | `http://default.localhost:3202` |

> 注意：如果启用 OAuth（如 Google），必须同步更新对应的 redirect URL（示例见 `.env.example` 的 `GOOGLE_REDIRECT_URL`）；否则回调会打到错误端口。

### 6.4 与 v4 RLS 的对齐（只在命中 v4 表时）

`DEV-PLAN-081/087` 已明确：访问启用 RLS 的 v4 表时，`RLS_ENFORCE` 必须为 `enforce`，且 `DB_USER` 必须为非 superuser（否则 Postgres 会绕过 RLS）。  
多 worktree 并行时，务必在**每个 worktree 的 `.env/.env.local`**保持一致，避免“一个 worktree enforce、另一个 disabled”导致行为分叉。

## 7. 数据库迁移与 seed：协作规则（避免互相踩踏）

共享 DB 的代价是：任一 worktree 的迁移/seed 都会影响其他 worktree。为减少互相踩踏，建议遵循：

- 单写者原则（推荐）：约定由 `wt-main`（或指定的一个 worktree）执行迁移/seed；其他 worktree 只在需要时执行 `make db migrate up` 同步到最新。
- 避免破坏性回滚：不要在共享 DB 上频繁 `migrate down`/重置；需要验证“回滚链路”时，使用独立 DB（同容器不同 `DB_NAME`）或采用 011B 的隔离方案。
- 变更数据库 schema/迁移时：对齐 `AGENTS.md` 触发器矩阵与 `DEV-PLAN-087` 的“可复现”要求；并在 PR 前按门禁组合验证。

## 8. 与 `DEV-PLAN-087` 的部署口径对齐（本地 → Docker）

### 8.1 本地开发对齐点

- Postgres 主版本对齐：本地 compose 使用 `postgres:17`（与 `DEV-PLAN-087` 一致）。
- 本地编排 SSOT：`devhub.yml` / `compose.dev.yml`；不要在不同 worktree 各自维护“私人 compose”。

### 8.2 Docker 部署对齐点（避免在本机多套 stack 互撞）

`DEV-PLAN-087` 的部署口径包含：`Dockerfile`/`Dockerfile.superadmin` 与 `compose.yml`。在一台开发机上同时跑多套“应用+数据库”的 compose stack，容易导致：
- 端口冲突（应用端口、Postgres/Redis 端口）；
- 多实例同时执行迁移/seed（若镜像入口包含自动迁移），造成不可预期的竞态。

建议：
- 本地并行开发优先使用“共享 infra + 多 worktree +（必要时）多端口 server”；
- 需要做“接近生产的 Docker 演练”时，单独选择一套端口/DB（或单独一台环境）进行验证，不要与共享 dev infra 混跑。

## 9. 验收标准（本计划完成定义）

- [ ] 在两个不同 worktree 目录中分别执行 `docker compose -f compose.dev.yml up -d db redis`，均成功且不会创建第二套容器（对齐 `DEV-PLAN-011C`）。
- [ ] 3 worktree 模式下，能在不切分支的前提下完成：开发/复现/基线验证。
- [ ] 如需并行运行多 server，按 §6.3 分配端口后可同时启动且互不抢占端口。
- [ ] 指引中涉及的本地/部署口径不与 `DEV-PLAN-087` 冲突。

