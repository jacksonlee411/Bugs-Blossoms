# DEV-PLAN-011B：多 worktree 本地 Docker 隔离（Postgres/Redis）

**状态**: 已完成（待合并，2025-12-14 08:04 UTC）

## 1. 背景与上下文 (Context)
- 当前机器上存在多个 `git worktree` 并行开发同一仓库（`bugs-blossoms`）。
- 当前 3 个 worktree（以本机实际路径为准）：
  - `Bugs-Blossoms`（分支：`feature/dev-plan-011a-impl`）
  - `Bugs-Blossoms-015b4`（分支：`main`）
  - `Bugs-Blossoms-020`（`detached HEAD`：`ae86336e`）
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
   - 重要提示：这一改动会让原本共享的 `sdk-data`/`sdk-redis` 不再被自动挂载，新启动的实例会使用新的 `<project>_sdk-data`/`<project>_sdk-redis`，表现为“数据库像被清空”。这是**隔离的必然结果**，不是数据真的丢失。
2. **端口策略**
   - 选定：`compose.dev.yml` 使用环境变量注入端口：
     - Postgres：`${PG_PORT:-5438}:5432`
     - Redis：`${REDIS_PORT:-6379}:6379`
3. **worktree 参数落地方式**
   - 选定：每个 worktree 创建一个不提交的 `.env.local`，在其中声明 `COMPOSE_PROJECT_NAME/PG_PORT/REDIS_PORT`（compose 隔离）以及 `DB_*`（应用/Makefile 用）。
   - 命令口径：
     - 运行 compose：`docker compose --env-file .env.local -f compose.dev.yml up -d db redis`
     - 运行 make（Go/脚本）：Makefile 通过 `-include .env.local` 自动加载（避免“配置了但 make 读不到”的误用）。
   - 可选增强：提供 `scripts/setup-worktree.sh` 或 `make dev-env` 自动生成 `.env.local`（按目录 hash 或扫描空闲端口），降低手工端口管理成本。
   - 变量一致性约定（减少误配）：
     - `DB_PORT` 与 `PG_PORT` **通常应保持一致**（同一 worktree 的应用连接到自己启动的 Postgres）。
     - `DB_PORT` 未显式设置时，默认等于 `PG_PORT`（由 Makefile 提供默认联动）。
     - 若你刻意让应用连接到“非本 worktree 的 DB”，必须同时在 `.env.local` 中显式设置 `DB_HOST/DB_PORT/DB_NAME`，避免误连接。

## 5. 命令口径 (CLI / Make Targets)
### 5.1 每个 worktree 的推荐 `.env.local`（不提交）
示例（worktree-1，按需替换端口/DB 名）：
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
docker compose --env-file .env.local -f compose.dev.yml up -d db redis
```

### 5.2 建议配置（与当前 3 个 worktree 对齐）
- `Bugs-Blossoms`：
  - `COMPOSE_PROJECT_NAME=bugs-blossoms-011a`
  - `PG_PORT=5438`、`REDIS_PORT=6379`
  - `DB_PORT=5438`、`DB_NAME=iota_erp_011a`
- `Bugs-Blossoms-015b4`：
  - `COMPOSE_PROJECT_NAME=bugs-blossoms-015b4`
  - `PG_PORT=5439`、`REDIS_PORT=6380`
  - `DB_PORT=5439`、`DB_NAME=iota_erp_015b4`
- `Bugs-Blossoms-020`：
  - `COMPOSE_PROJECT_NAME=bugs-blossoms-020`
  - `PG_PORT=5440`、`REDIS_PORT=6381`
  - `DB_PORT=5440`、`DB_NAME=iota_erp_020`

### 5.3 旧 volume 数据处理（可选）
当切换为 project-scoped volumes 后，旧的共享卷（例如 `sdk-data`）不会再自动挂载。

- 选项 A（推荐）：接受重建（dev 环境）
  - 直接按新配置启动后运行迁移/seed 即可。
- 选项 B：迁移旧卷数据到新卷（仅当你确实要保留本地数据）
  1. 找到旧卷与新卷名：
     - `docker volume ls | grep sdk-data`
     - `docker volume ls | grep <project>_sdk-data`
  2. 用临时容器复制数据（Postgres 必须处于停止状态）：
     - `docker run --rm -v sdk-data:/from -v <project>_sdk-data:/to alpine sh -c "cp -a /from/. /to/."`
  3. 启动新实例并确认数据。

## 6. 实施步骤 (Execution Plan)
1. [x] **compose.dev.yml 参数化与隔离**
   - [x] Postgres/Redis 端口改为环境变量注入（保留默认值以减少破坏性）。
   - [x] 移除 `volumes.*.name` 固定命名，恢复按 project 隔离。
   - [ ] （可选）补充 healthcheck（便于脚本等待 DB 就绪）。
2. [x] **文档口径更新**
   - [x] 在 README/CONTRIBUTING 增加“多 worktree 本地开发”章节：端口与 `.env.local` 约定、常见冲突排查（`docker ps` / `docker volume ls` / `docker compose ls`）。
3. [x] **Makefile 集成（必做）**
   - [x] Makefile 加入 `-include .env.local`，并明确导出 compose/db 相关变量，确保 `make db ...`/`docker compose ...` 行为与 worktree 配置一致。
4. [x] **端口管理辅助（可选但推荐）**
   - [x] 提供 `scripts/setup-worktree.sh` 与 `make dev-env`，自动生成/补齐 `.env.local`（避免手工分配端口冲突/记混）。
5. [x] **本地验证**
   - [x] 多个 project 并行启动 `db/redis` 不冲突（验证端口与 project-scoped volumes 生效）。
   - [x] `docker compose down -v` 仅清理当前 project 的 volumes，不影响其它 project。

## 7. 验收标准 (Acceptance Criteria)
- 三个 worktree 并行执行 `docker compose -f compose.dev.yml up -d db redis` 均成功，且 `docker ps` 显示为不同 project 容器。
- 任一 worktree 执行 `docker compose -f compose.dev.yml down -v` 仅清理自身的 volumes，不影响其它 worktree。
- `DB_PORT/DB_NAME` 与 compose 的端口/容器实例一致，迁移与测试不再出现“连接到错误实例/端口占用”类问题。

## 8. 回滚策略 (Rollback)
- 回滚 `compose.dev.yml` 的端口与 volume 修改（恢复固定端口与固定卷名）。
- 清理由新策略创建的 project-scoped volumes（按 `docker volume ls | grep <project>` 定位后删除）。

## 9. 合并提示 (PR Notes)
实施本方案的 PR 描述中必须显式提示：
- **注意：本地数据库将以 project-scoped volumes 重新初始化，看起来会“清空”。旧数据仍在旧卷（例如 `sdk-data`）中，未被删除。**
- 若需要保留旧数据，请按“5.3 旧 volume 数据处理”迁移；否则请重新运行迁移/seed。
