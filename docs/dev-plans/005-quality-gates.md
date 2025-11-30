# DEV-PLAN-005：代码质量门禁体系

**状态**: 规划中（2025-11-30 15:05）

## 背景
当前仓库以单人远程开发为主，虽有基本的开发者自律流程，但缺乏可以自动阻断低质量提交的门禁。新模块（如 IOTA SDK 各模块）持续扩张，涉及 Go、templ/Tailwind、SQL 迁移与多语言资源。如果仍然依赖手动检查，极易出现样式漏构建、翻译 key 缺失、Go 代码未格式化等问题。需要制定统一的质量门禁计划，并在 GitHub Actions 中逐步落地。

## 目标
- 所有 Pull Request 在合并前必须通过“静态分析 + 单元测试 + 资源校验”三个层面的门禁。
- 提供本地对齐脚本，使开发者在推送前可执行与 CI 相同的检查。
- 针对受限资源（templ、Tailwind、翻译、迁移）建立专属检查，避免仅依赖 Go 代码测试。
- 在仓库文档（CONTRIBUTING、README、dev-plan）中同步门禁策略，让协作者明确要求。

## 现状评估
- `.github/workflows/test.yml` 已经具备完整的工具安装、Go fmt/vet、templ fmt、CSS 构建、翻译校验与带 PostgreSQL/Redis 的 `go test` 流程，可以作为统一 workflow 的基础。
- Makefile 中已有 `test`、`css`、`check lint`/`check tr`、`db migrate`/`seed` 等细粒度目标，后续 `make verify-*` 可直接复用这些命令组合。
- 仍存在关键缺口：workflow 只在 push 时触发（PR 不受保护）、Go 版本固定为 1.23.2、缺少 `make verify-go` / `make verify-ui` 等本地入口，templ/Tailwind 校验未按路径条件执行且缺少 `git status` 检查，分支保护与文档同步尚未开展。

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
1. [ ] **CI 基线搭建** —— 以 `.github/workflows/test.yml` 为基础改名为 `quality-gates`，触发条件设置为 `push`（限 main/dev 等关键分支）+ `pull_request`。在 workflow 中统一使用 Go 1.24.10，并确保 templ、Tailwind、golangci-lint、pgformatter 等工具版本与 `docs/CONTRIBUTING.MD` 相同，避免本地/CI 不一致。  
2. [ ] **Go Lint/Test 门禁** —— 在 workflow 的 Go job 里串联以下步骤：`go fmt ./...` + `git diff --exit-code`、`go vet ./...`、`golangci-lint run ./...`、`go test -v ./...`（带覆盖率）。同时在 Makefile 新增 `make verify-go`：内部依次执行 `go fmt`, `go vet`, `golangci-lint run`, `go test -v ./...` 并在失败时返回非零；`make verify` 则聚合 `verify-go` 与 UI/i18n/db 检查。  
3. [ ] **前端模板与样式门禁** —— 在 workflow 添加条件步骤：当 `.templ`、`tailwind.config.js`、`modules/**/presentation/assets/**` 变更时，运行 `templ generate && make css`，随后执行 `git status --porcelain` 保证生成文件已提交。Makefile 中新增 `make verify-ui`，复用同一套命令以便开发者本地预检。  
4. [ ] **翻译与本地化门禁** —— 对 `modules/**/presentation/locales/*.json` 的 diff 才执行 `make check tr`（或轻量 JSON 校验脚本），并将其纳入 `make verify-i18n`（被 `make verify` 调用）。同时在 workflow 中根据路径过滤控制步骤，减少不必要运行时间。  
5. [ ] **数据库与缓存门禁** —— 继续使用 PostgreSQL 17 + Redis latest 服务容器，但将环境变量固定到 `DB_HOST=localhost` 等计划值，并在步骤中串联 `make db migrate up`, `make db migrate down` smoke（可选）, `make db seed`。输出 `migrate.log` 作为 artifact 以便排障。  
6. [ ] **分支保护策略** —— 在 GitHub `main` 分支启用保护：要求 `quality-gates` workflow 成功才能合并，禁止直接 push/force push，并开启最少一条 review。必要时在 `docs/dev-plans/005` 附带操作指引（截图/CLI 命令）。  
7. [ ] **文档与宣传** —— 更新 `docs/CONTRIBUTING.MD`、`README.MD`、`AGENTS.md`、`CLAUDE.md`，新增“质量门禁 & 本地验证”章节，列出 `make verify`, `make verify-go`, `make verify-ui`, `make verify-i18n` 的用途与触发条件。PR 模版中也提醒贡献者本地执行 `make verify`。

## 里程碑
- M1：质量门禁 workflow 雏形（Lint/Test）上线，并在 main 分支开启必需检查。
- M2：UI/翻译/迁移门禁接入，`make verify` 总入口完成。
- M3：文档同步 & 分支保护策略启用，形成稳定运作的 PR 审核流程。

## 交付物
- 更新后的 `.github/workflows/test.yml`（或改名后的单一质量门禁 workflow，涵盖所有门禁任务）。
- 新的 `make verify`, `make verify-go`, `make verify-ui` 等辅助命令。
- 更新后的文档：CONTRIBUTING、README、AGENTS/CLAUDE 门禁章节。
- GitHub 分支保护及 PR 检查配置说明。
