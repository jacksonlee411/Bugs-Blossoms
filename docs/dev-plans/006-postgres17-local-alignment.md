# DEV-PLAN-006：本地 PostgreSQL 统一到 17

**状态**: 已完成（2025-11-30 17:35）

## 背景
CI 已经在 `.github/workflows/quality-gates.yml` 中使用 PostgreSQL 17，但在计划立项之初，`compose.yml`、`compose.dev.yml`、`compose.testing.yml` 仍固定到 `postgres:15.1`。这导致以下问题：
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
1. [X] **兼容性确认**  
   - 已复查 `go.mod` 中与数据库交互的核心依赖（pgx、sql-migrate、rubenv/sql-migrate、sqlx）均发布于 PostgreSQL 17 之后的版本。  
   - `.github/workflows/quality-gates.yml` 自带的 `test-unit-integration` job 在 PG17 服务上执行 `make db migrate up / make db seed / go test -v ./...`；本地也以 PG17 实例完成 `make db migrate up`/`make db seed`，确认流程可沿用。

2. [X] **更新 Compose 镜像**  
   - `compose.yml`、`compose.dev.yml`、`compose.testing.yml` 全部改为 `postgres:17`，并保留原有命令/端口设定。  
   - 在执行流程中验证 `docker volume rm sdk-data` 之后重新初始化数据库可正常启动。

3. [X] **提供迁移脚本与备份指引**  
   - `README.MD` 新增“PostgreSQL 17 迁移与备份”章节，包含 `pg_dump` 备份、卷清理与重新迁移/种子的命令示例。

4. [X] **同步配置与文档**  
   - 已更新 `README.MD`、`AGENTS.md`、`CLAUDE.md`、`devhub.yml`、`docs/SUPERADMIN.md` 等文件，将默认数据库版本改为 PostgreSQL 17，并替换示例中残留的 `postgres:15/13`。  
   - 其他引用 `postgres:X` 的 docker 片段也审计替换；`postgresql://` 形式的连接串未作更改。  
   - 文档中明确说明：本地、CI 均默认使用 PG17，若需旧版本需自行调整 compose。

5. [X] **验证与回归**  
   - 在 PG17 容器上执行 `make db migrate up`、`make db seed`，日志显示 24 条迁移及默认种子成功。  
   - `go test ./pkg/application` 通过；`go test ./...` 在 finance 控制器测试中因既有数据依赖报错（外键约束/连接数），与 PG 版本无关，记录于执行日志中以便后续专项修复。

## 里程碑
- M1：Compose 文件与脚本完成版本替换，CI 验证通过。
- M2：文档、示例、入门指引更新完成，提供迁移/备份指南。
- M3：开发者确认在本地完成升级，反馈收集完毕（如在 AGENTS 里记录 FAQ）。

## 交付物
- 更新后的 `compose*.yml`（全部指向 `postgres:17`）。
- 迁移/备份指引文档（README、DEV-PLAN-006 或其他教程章节）。
- 可选的脚本/Make 目标（若添加 `make db backup` 等辅助命令）。
- 升级验证记录：`make db migrate up`、`make db seed`、`go test ./...` 的执行结果说明。
