# DEV-PLAN-005：代码质量门禁体系

**状态**: 规划中（2025-11-30 15:05）

## 背景
当前仓库以单人远程开发为主，虽有基本的开发者自律流程，但缺乏可以自动阻断低质量提交的门禁。新模块（如 IOTA SDK 各模块）持续扩张，涉及 Go、templ/Tailwind、SQL 迁移与多语言资源。如果仍然依赖手动检查，极易出现样式漏构建、翻译 key 缺失、Go 代码未格式化等问题。需要制定统一的质量门禁计划，并在 GitHub Actions 中逐步落地。

## 目标
- 所有 Pull Request 在合并前必须通过“静态分析 + 单元测试 + 资源校验”三个层面的门禁。
- 提供本地对齐脚本，使开发者在推送前可执行与 CI 相同的检查。
- 针对受限资源（templ、Tailwind、翻译、迁移）建立专属检查，避免仅依赖 Go 代码测试。
- 在仓库文档（CONTRIBUTING、README、dev-plan）中同步门禁策略，让协作者明确要求。

## 门禁矩阵
| 阶段 | 内容 | 命令/工具 | 触发场景 |
| --- | --- | --- | --- |
| Lint | `go fmt ./...` + `go vet ./...` | GitHub Actions + pre-push | 所有 Go 文件改动 |
| Test | `go test -v ./...` 或 `make test` | GitHub Actions | 任意 Go 代码改动 |
| Templates & CSS | `templ generate && make css` 后确保 git clean | GitHub Actions 条件步骤 | `.templ`、`tailwind.config.js`、`modules/**/presentation/assets` 变更 |
| 翻译校验 | `make check tr` | GitHub Actions 条件步骤 | `modules/**/presentation/locales/*.json` 变更 |
| 数据库迁移 | `make db migrate up`（连接临时 PostgreSQL） | GitHub Actions 服务容器 | `migrations/`、`modules/**/schema` 变更 |
| 质量概览 | `go tool cover` 或 sonar-like 报告（后续扩展） | 可选 | 大型功能分支 |

## 实施步骤
1. [ ] **CI 基线搭建** —— 直接扩展现有 `.github/workflows/test.yml`（必要时改名但保持一个 workflow），触发条件仍为 push main + 所有 PR。统一安装 Go 1.24.10、templ、tailwind、golangci-lint、pgformatter 等依赖，并预置门禁矩阵对应 job/step，避免维护两套 CI。  
2. [ ] **Go Lint/Test 门禁** —— 在 workflow 中串联 `go fmt`（配合 `git diff --exit-code`）、`go vet`、`go test -v`，并保持 `golangci-lint run ./...` 等现有静态分析步骤；同时在 Makefile 中提供 `make verify-go` 并更新 CONTRIBUTING。  
3. [ ] **前端模板与样式门禁** —— 通过 paths 条件检测 `.templ`、`tailwind.config.js`、`modules/**/presentation/assets` 变更，执行 `templ generate && make css` 与 `git status --porcelain`，并提供 `make verify-ui`。  
4. [ ] **翻译与本地化门禁** —— 针对 `modules/**/presentation/locales/*.json` 执行 `make check tr` 或新增 JSON 校验脚本，确保 key 完整性与排序，并纳入 `make verify-ui` 或独立 `make verify-i18n`。  
5. [ ] **数据库与缓存门禁** —— 在 workflow 中启用 PostgreSQL 17 与 Redis latest 服务容器（保持 `DB_HOST=localhost`、`DB_PORT=5432`、`DB_NAME=iota_erp`、`DB_USER=postgres`、`DB_PASSWORD=postgres`、`REDIS_URL=localhost:6379`），运行 `make db migrate up`（必要时附 `down` smoke）以及 `make db seed` 验证，并保存 `migrate.log` 以便排查。  
6. [ ] **分支保护策略** —— 在 GitHub 设置里要求 `main` 分支通过 `quality-gates` workflow 才可合并，禁止直接推送与 force push，必要时要求至少一条 review。  
7. [ ] **文档与宣传** —— 更新 `docs/CONTRIBUTING.MD`、`README.MD`、`AGENTS.md`、`CLAUDE.md` 等门禁说明，引导开发者使用 `make verify-*`。

## 里程碑
- M1：质量门禁 workflow 雏形（Lint/Test）上线，并在 main 分支开启必需检查。
- M2：UI/翻译/迁移门禁接入，`make verify` 总入口完成。
- M3：文档同步 & 分支保护策略启用，形成稳定运作的 PR 审核流程。

## 交付物
- 更新后的 `.github/workflows/test.yml`（或改名后的单一质量门禁 workflow，涵盖所有门禁任务）。
- 新的 `make verify`, `make verify-go`, `make verify-ui` 等辅助命令。
- 更新后的文档：CONTRIBUTING、README、AGENTS/CLAUDE 门禁章节。
- GitHub 分支保护及 PR 检查配置说明。
