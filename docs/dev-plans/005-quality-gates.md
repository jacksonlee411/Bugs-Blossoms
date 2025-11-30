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
1. [ ] **CI 基线调整** —— 以现有 `.github/workflows/test.yml` 为基础改名为 `quality-gates`，触发条件扩展为 `push`（限定 main/dev 等关键分支）+ `pull_request`，统一切换到 Go 1.24.10，并让 templ/Tailwind/golangci-lint/pgformatter 版本与 `docs/CONTRIBUTING.MD` 中记录的版本保持一致，避免本地与 CI 发散。  
2. [ ] **条件化资源检查** —— 沿用当前 lint/test job 中的 `go fmt`、`go vet`、`make check lint`、`go test -v` 等步骤，但为成本较高的任务增加 `paths` 条件：`.templ`、`tailwind.config.js`、`modules/**/presentation/assets/**` 触发 `go generate ./... && make generate` + `make css` + `git status --porcelain`，`modules/**/presentation/locales/*.json` 触发 `make check tr`，`migrations/**` 或 `modules/**/schema/**` 触发 `make db migrate up/down` 与 `make db seed`。借助 `if: steps.changed-files.outputs.any_changed == 'true'` 等模式削减重复执行。  
3. [ ] **数据库/缓存日志可观测性** —— 保留现有 PostgreSQL 17 + Redis 服务容器，固定 `DB_HOST=localhost` 等变量，但补充 `tee migrate.log` 或 artifact 上传，便于排查迁移失败；同时在 job 结尾上传 `coverage.out`、`migrate.log` 等核心产物。  
4. [ ] **分支保护策略** —— 在 GitHub `main` 分支启用保护：要求 `quality-gates` workflow 通过才能合并，禁止直接 push/force push，并开启至少一条 review。整理操作说明（CLI/API/截图）附在本计划，方便后续执行。  
5. [ ] **文档同步** —— 在 `README.MD`、`docs/CONTRIBUTING.MD`、`AGENTS.md`、`CLAUDE.md` 更新“质量门禁 & 本地校验”章节，强调 `make check lint`、`make test`、`make css`、`make check tr`、`make db migrate` 等现有命令即可复现 CI 行为，提醒贡献者在提交前手动运行与自己改动相关的命令。

## 里程碑
- M1：质量门禁 workflow 雏形（Lint/Test）上线，并在 main 分支开启必需检查。
- M2：UI/翻译/迁移门禁接入，`make verify` 总入口完成。
- M3：文档同步 & 分支保护策略启用，形成稳定运作的 PR 审核流程。

## 交付物
- 更新后的 `.github/workflows/test.yml`（或改名后的单一质量门禁 workflow，涵盖所有门禁任务）。
- 新的 `make verify`, `make verify-go`, `make verify-ui` 等辅助命令。
- 更新后的文档：CONTRIBUTING、README、AGENTS/CLAUDE 门禁章节。
- GitHub 分支保护及 PR 检查配置说明。
