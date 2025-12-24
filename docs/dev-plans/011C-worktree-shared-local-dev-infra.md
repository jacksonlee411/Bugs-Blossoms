# DEV-PLAN-011C：多 worktree 共享本地开发基础设施（Postgres/Redis）

**状态**: 已完成（2025-12-24 09:28 UTC — 默认方案）

> 本计划为默认方案：从“每个 worktree 独立一套 DB/Redis（端口隔离）”，调整为“所有 worktree 共用一套 DB/Redis（固定端口）”。
>
> 需要“每个 worktree 独立一套 DB/Redis 并行运行”时，使用可选高级模式 `DEV-PLAN-011B`。

## 1. 背景与上下文 (Context)
- 本机存在多个 `git worktree` 并行维护同一仓库，但**并不需要**每个 worktree 同时启动一整套 Postgres/Redis。
- `DEV-PLAN-011B` 的隔离策略虽然能并行跑多套 infra，但带来额外的端口/配置管理成本（`.env.local`、端口分配、数据卷迁移等）。
- 目标调整：让所有 worktree **无脑共享**一套本地 Postgres/Redis（同一套容器、同一套数据卷、同一组端口），降低使用门槛与资源占用。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [X] **共享基础设施**：所有 worktree 共用同一套 Postgres/Redis 容器与数据卷。
- [X] **固定端口**：Postgres 固定 `5438`，Redis 固定 `6379`（避免每个 worktree 单独分配端口）。
- [X] **任意 worktree 可操作**：从任意 worktree 执行 `docker compose -f compose.dev.yml up -d db redis` 都指向同一套 infra（不会尝试起第二套）。
- [X] **口径统一**：README/CONTRIBUTING/脚本保持同一套指引。

### 2.2 非目标（本计划不做）
- 不支持“多个 worktree 同时各自拥有一套 DB/Redis”并行运行（那是 011B 的目标）。
- 不提供 worktree 级数据隔离；迁移/seed/数据变更对所有 worktree 生效。

### 2.3 工具链与门禁（SSOT 引用）
> 触发器与命令入口以仓库 SSOT 为准，避免本文复制导致 drift。

- **本计划命中触发器**：
  - [X] 新增/调整文档（`make check doc`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`

## 3. 方案与关键决策 (Design & Decisions)
### 3.1 固定 Compose Project 名称（关键）
为保证“从不同目录运行 compose 仍指向同一套容器”，必须固定 project 名称，否则 `docker compose` 会按目录名推导 project，导致：
- A worktree 已占用 `5438/6379` 后，B worktree 再 `up` 会因端口占用失败；
- 或者误以为“共享”，但实际上起了另一套 project（只是端口冲突阻止了它）。

选定实现：
- 在 `compose.dev.yml` 顶层声明固定 `name`（例如 `iota-sdk-dev`），作为共享 infra 的 SSOT。

### 3.2 固定端口
- `compose.dev.yml` 对外端口保持 `5438/6379`。
- 仍允许通过环境变量覆盖（为了应急），但文档口径为“默认不改”。

### 3.3 `.env.local` 辅助脚本
保留 `make dev-env`（`scripts/setup-worktree.sh`）作为“补齐本地环境变量”的工具，但默认行为改为：
- 不再探测空闲端口、不再生成 worktree-scoped `COMPOSE_PROJECT_NAME`；
- 默认写入固定端口与默认数据库名（`DB_NAME=iota_erp`），确保开箱可用。

## 4. 使用方式 (How-To)
### 4.1 一次性准备
1. 初始化 `.env`：
   - `cp .env.example .env`
2. （可选）补齐 `.env.local`（不提交）：
   - `make dev-env`

### 4.2 启动共享基础设施
在任意 worktree 执行：
```bash
docker compose -f compose.dev.yml up -d db redis
```

### 4.3 初始化/更新数据库
```bash
make db migrate up && make db seed
```

### 4.4 注意事项
- **不要在任意 worktree 随意执行** `docker compose -f compose.dev.yml down -v`：这会把共享数据卷一起清掉，相当于重置所有 worktree 的本地数据。
- 如果确实需要“每个 worktree 独立 DB/Redis 并行运行”，请回到 `DEV-PLAN-011B` 的隔离方案（或基于其思路另开计划）。

## 5. 实施步骤 (Execution Plan)
1. [X] 固定 `compose.dev.yml` 的 project 名称，使其在所有 worktree 一致。
2. [X] 调整 `scripts/setup-worktree.sh` 默认值为共享 infra（固定端口/默认 DB_NAME）。
3. [X] 更新 `README.MD` 与 `docs/CONTRIBUTING.MD` 的 worktree 指引，改为引用本计划。
4. [X] 更新 `AGENTS.md` Doc Map，确保新文档可发现。

## 6. 验收标准 (Acceptance Criteria)
- 在两个不同 worktree 目录中分别执行：
  - `docker compose -f compose.dev.yml up -d db redis`
  两次都成功，且 `docker ps` 显示同一套容器（不会出现第二套 project 的容器创建尝试）。
- `make db migrate up && make db seed` 能在共享 Postgres 上正常执行。
- README/CONTRIBUTING 的入口链接指向 011C。

## 7. 回滚策略 (Rollback)
- 移除 `compose.dev.yml` 的固定 project 名称，并恢复/启用 011B 的隔离口径（worktree-scoped `COMPOSE_PROJECT_NAME/PG_PORT/REDIS_PORT`）。
- 如已创建新的共享 volumes/containers，按需 `docker compose down -v` 清理（注意这是破坏性操作）。

## 8. 验证记录（命令与结果）
> 记录最终合并前的本地门禁执行情况（按需补充）。

- [X] `make check doc`（2025-12-24 09:28 UTC）：通过
- [X] `bash -n scripts/setup-worktree.sh`（2025-12-24 09:28 UTC）：通过
- [X] `go fmt ./... && go vet ./... && make check lint && make test`（2025-12-24 09:28 UTC）：通过
