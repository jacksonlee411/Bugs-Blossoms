# DEV-PLAN-006：本地 PostgreSQL 统一到 17

**状态**: 规划中（2025-11-30 17:40）

## 背景
CI 已经在 `.github/workflows/test.yml` 中使用 PostgreSQL 17，但 `compose.yml`、`compose.dev.yml`、`compose.testing.yml` 仍固定到 `postgres:15.1`。这导致以下问题：
- 本地回归时无法提前暴露 PostgreSQL 17 的行为差异（数据类型、planner、新关键字等）。
- CI 排查困难：若 Bug 只在 17 上出现，开发者难以复现。
- 镜像、备份脚本、文档对版本描述不一致，降低新人上手效率。

## 目标
- 所有 Docker Compose（dev/test/部署示例）统一拉取 `postgres:17`。
- 提供迁移指引（如何备份旧 volume、重新 seed）。
- 更新文档、脚本与工具提示，明确默认数据库版本。
- 在本地 `make db migrate up` / `make db seed` 验证 17 可用，确保与 CI 对齐。

## 风险与兼容性
- 需要验证 rubenv/sql-migrate、pgx、pgformatter 等依赖在 17 下无兼容性问题。
- 现有数据卷（`sdk-data` 等）需手动备份或重新初始化，避免 15 → 17 直接启动导致 catalog 版本不匹配。
- 生产/其他环境若尚未升级，需在文档中强调“仅本地/CI 默认 17”，并给出回退方案。

## 实施步骤
1. [ ] **兼容性确认**  
   - 复查 `go.mod` 中与 PostgreSQL 交互的库（pgx、sql-migrate、lens postgres datasource）是否声明支持 17。  
   - 核对 `.github/workflows/test.yml` 的 PostgreSQL 17 job，确认其 `make db migrate up`、`make db seed`、`go test ./...` 已在 PG17 上执行且通过；若未来质量门禁新增校验，直接沿用该 job 的环境配置，无需再新增重复脚本。

2. [ ] **更新 Compose 镜像**  
   - 将 `compose.yml`、`compose.dev.yml`、`compose.testing.yml` 的 `image: postgres:15.1` 改为 `postgres:17`，并在需要时同步 `command`/`env`。  
   - 对 `compose.dev.yml` 中的端口/volume 保持不变，但在变更说明里提示开发者需要 `docker volume rm sdk-data` 或 `make db clean` 以重新初始化。

3. [ ] **提供迁移脚本与备份指引**  
   - 在 `docs/README` 或 `docs/dev-plans/002-devcontainer-to-native.md` 中新增一节，描述如何使用 `pg_dump`/`docker cp` 导出旧数据并导入 17。  
   - 更新 `Makefile`/脚本（若需要）提供 `make db backup`（可选）或在 `make db reset` 说明中强调版本切换步骤。

4. [ ] **同步配置与文档**  
   - 更新 `README.MD`、`AGENTS.md`、`CLAUDE.md`、`devhub.yml` 等提及数据库版本的位置。  
   - 全量审计仓库中所有带版本标签的 Postgres 镜像引用（`postgres:X`、`postgresql:X` 等），包含 `.env.example`、`docs/SUPERADMIN.md`、Docker/Compose 示例等，统一替换为 `postgres:17` 或在文档中明确说明例外场景；无须修改连接字符串中 `postgresql://` 这类协议前缀。  
   - 在新版本说明中明确：CI、本地默认 17，若需要旧版本需手动修改 compose。

5. [ ] **验证与回归**  
   - 启动 `docker compose -f compose.dev.yml up db redis`，执行 `make db migrate up`、`make db seed`，确认日志正常。  
   - 运行 `go test ./pkg/application`（覆盖迁移管理逻辑）以及一次全量 `go test ./...`，或等效的 `make test coverage`，确保数据库交互未回归。  
   - 记录在 `docs/dev-plans/006` 或变更日志中，包含验证命令与遇到的问题。

## 里程碑
- M1：Compose 文件与脚本完成版本替换，CI 验证通过。
- M2：文档、示例、入门指引更新完成，提供迁移/备份指南。
- M3：开发者确认在本地完成升级，反馈收集完毕（如在 AGENTS 里记录 FAQ）。

## 交付物
- 更新后的 `compose*.yml`（全部指向 `postgres:17`）。
- 迁移/备份指引文档（README、DEV-PLAN-006 或其他教程章节）。
- 可选的脚本/Make 目标（若添加 `make db backup` 等辅助命令）。
- 升级验证记录：`make db migrate up`、`make db seed`、`go test ./...` 的执行结果说明。
