# DEV-PLAN-011B：多 worktree 本地 Docker 隔离（Postgres/Redis）

**状态**: 规划中（2025-12-14 08:04 UTC）

## 1. 背景与上下文 (Context)
- 当前机器上存在多个 `git worktree` 并行开发同一仓库（`bugs-blossoms`）。
- 本地开发依赖 `compose.dev.yml` 启动 Postgres/Redis，但 Docker 资源默认不随 worktree 隔离，容易发生以下问题：
  - **端口冲突**：不同 worktree 的 redis 都尝试绑定 `6379`，导致 `docker compose up redis` 失败（`port is already allocated`）。
  - **数据互相污染**：`compose.dev.yml` 把 volume 固定命名为 `sdk-data` / `sdk-redis`，导致不同 compose project 仍共享同一份数据卷（即使 worktree 目录不同）。
  - **清理互相误伤**：任一 worktree 执行 `docker compose down -v` 可能影响其它 worktree 的数据，或因为“volume still in use”导致清理失败。
- 该漂移会直接影响 011A/011B 相关的本地验证与 CI 对齐目标（尤其是需要稳定可复现的 DB 环境）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **每个 worktree 独立一套本地 Postgres/Redis**：容器/网络/volume 互不干扰。
- [ ] **并行可运行**：三套 worktree 同时 `docker compose up -d db redis` 不冲突。
- [ ] **一键清理可控**：在某个 worktree 里 `docker compose down -v` 不影响其它 worktree。
- [ ] **口径统一**：Makefile/文档统一说明 `DB_PORT`/`DB_NAME`/`COMPOSE_PROJECT_NAME` 的约定，减少“记忆式操作”。

### 2.2 非目标（本计划不做）
- 不引入 Kubernetes / Tilt / DevSpace 等新编排系统。
- 不改变 CI 的数据库拓扑（CI 仍由 workflow 的 Postgres service 提供）。
- 不把全仓库所有 compose 文件都重构为一套复杂参数系统（以最小必要改动为准）。

## 3. 现状复核与问题定位 (Findings)
### 3.1 compose.dev.yml 的关键问题
- `db` 映射固定端口：`5438:5432`（可用但无法多实例并行）。
- `redis` 映射固定端口：`6379:6379`（多实例必冲突）。
- volumes 显式固定名字：
  - `sdk-data: { name: sdk-data }`
  - `sdk-redis: { name: sdk-redis }`
  这会绕过 compose 默认的“按 project 前缀隔离”，导致不同 worktree 共享同一份卷。

### 3.2 已观测到的故障形态
- 当某个 worktree 的 redis 已占用 `6379`，其它 worktree 启动 redis 会失败：`Bind for 0.0.0.0:6379 failed: port is already allocated`。
- 即使不同 worktree 使用不同 `COMPOSE_PROJECT_NAME`，由于 volume 被强制命名，仍会共享 `sdk-data`/`sdk-redis`。

## 4. 方案与关键决策 (Design & Decisions)
### 4.1 方案选型
#### 方案 A（选定）：每个 worktree 独立一套 compose project + 端口分配
- 每个 worktree 都启动自己的 Postgres/Redis（强隔离）。
- 通过 `COMPOSE_PROJECT_NAME` 让容器/网络/卷命名隔离。
- 通过 `PG_PORT` / `REDIS_PORT`（或等价变量）让宿主机端口隔离。
- 对应用侧，通过 `DB_PORT`/`DB_NAME` 约定指向对应 worktree 的 DB。

#### 方案 B（不选）：多个 worktree 共用一套 DB（多 DB_NAME 或 schema 隔离）
- 优点：少启动两套服务，省资源。
- 缺点：清理/迁移/缓存容易互相影响，联调与测试稳定性差，不适合并行开发。

### 4.2 关键决策
1. **Volume 命名策略**
   - 选定：移除 `compose.dev.yml` 中对 volume 的显式 `name:` 固定命名，恢复 compose 默认的 project-scoped 命名（`<project>_<volume>`）。
2. **端口策略**
   - 选定：`compose.dev.yml` 使用环境变量注入端口：
     - Postgres：`${PG_PORT:-5438}:5432`
     - Redis：`${REDIS_PORT:-6379}:6379`
3. **worktree 参数落地方式**
   - 选定：每个 worktree 创建一个不提交的 `.env.local`（或同名文件），在其中声明 `COMPOSE_PROJECT_NAME/PG_PORT/REDIS_PORT/DB_PORT/DB_NAME`。
   - 可选增强：提供一个脚本/Make target 自动生成建议端口与 project name，降低人工维护成本。

## 5. 命令口径 (CLI / Make Targets)
### 5.1 每个 worktree 的推荐 `.env.local`（不提交）
示例（worktree-1）：
```bash
COMPOSE_PROJECT_NAME=bugs-blossoms-w1
PG_PORT=5438
REDIS_PORT=6379

DB_HOST=localhost
DB_PORT=5438
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=iota_erp_w1
```

启动：
```bash
set -a; source .env.local; set +a
docker compose -f compose.dev.yml up -d db redis
```

### 5.2 端口分配建议（3 worktree）
- w1：`PG_PORT=5438`、`REDIS_PORT=6379`
- w2：`PG_PORT=5439`、`REDIS_PORT=6380`
- w3：`PG_PORT=5440`、`REDIS_PORT=6381`

## 6. 实施步骤 (Execution Plan)
1. [ ] **compose.dev.yml 参数化与隔离**
   - [ ] Postgres/Redis 端口改为环境变量注入（保留默认值以减少破坏性）。
   - [ ] 移除 `volumes.*.name` 固定命名，恢复按 project 隔离。
   - [ ] （可选）补充 healthcheck（便于脚本等待 DB 就绪）。
2. [ ] **文档口径更新**
   - [ ] 在 README/CONTRIBUTING 增加“多 worktree 本地开发”章节：端口与 `.env.local` 约定、常见冲突排查（`docker ps` / `docker volume ls` / `docker compose ls`）。
3. [ ] **Makefile 辅助（可选但推荐）**
   - [ ] 提供 `make dev-env` 或脚本生成器，输出当前 worktree 的建议配置（project name + 端口）。
4. [ ] **本地验证**
   - [ ] 三个 worktree 同时启动 `db/redis` 不冲突。
   - [ ] 每个 worktree 能独立执行迁移、跑关键测试，不污染其它 worktree。

## 7. 验收标准 (Acceptance Criteria)
- 三个 worktree 并行执行 `docker compose -f compose.dev.yml up -d db redis` 均成功，且 `docker ps` 显示为不同 project 容器。
- 任一 worktree 执行 `docker compose -f compose.dev.yml down -v` 仅清理自身的 volumes，不影响其它 worktree。
- `DB_PORT/DB_NAME` 与 compose 的端口/容器实例一致，迁移与测试不再出现“连接到错误实例/端口占用”类问题。

## 8. 回滚策略 (Rollback)
- 回滚 `compose.dev.yml` 的端口与 volume 修改（恢复固定端口与固定卷名）。
- 清理由新策略创建的 project-scoped volumes（按 `docker volume ls | grep <project>` 定位后删除）。

