# DEV-PLAN-061 Readiness

本记录用于验证 `docs/dev-plans/061-org-position-person-bridge-and-minimal-personnel-events.md` 的“Person 模块落地 + Org(Position/Assignment) 打通 + 最小人事事件”在本地/CI 工具链下可执行、可回归、可收口。

## 1. 本地验证命令（SSOT）

> 说明：命令细节以 `Makefile` 为准；本节仅列出与 061 变更触发器对齐的入口命令。

- Go 门禁：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`
- `.templ` / Tailwind 生成物（如涉及 UI/templ/CSS 变更）：
  - `make generate`
  - `make css`
  - `git status --short` 必须为空（生成物需提交）
- 多语言 JSON（如涉及 `modules/**/presentation/locales/**/*.json`）：
  - `make check tr`
- Authz（如涉及 `config/access/**` / `pkg/authz/**` 等）：
  - `make authz-test`
  - `make authz-lint`
- Routing gates（如涉及路由 allowlist / exposure）：
  - `make check routing`
- 迁移/Schema（061 引入 `migrations/person/**` 与 `migrations/org/**`）：
  - `PERSON_MIGRATIONS=1 make db migrate up`
  - `make db plan && make db lint`
  - `make org plan && make org lint && make org migrate up`

## 2. 实际跑通记录

### 2025-12-23（本地，无 Docker）

- DB：本机 PostgreSQL（`localhost:5432`）
- 说明：仓库 `.env.local` 可能配置 `DB_PORT=5440`（worktree docker 隔离口径）；本机无该端口服务时需覆写端口：
  - `make test DB_PORT=5432`
- 已验证：
  - `go fmt ./...`：PASS
  - `go vet ./...`：PASS
  - `make check lint`：PASS
  - `make generate`：PASS
  - `make css`：PASS
  - `make check tr`：PASS
  - `make authz-test`：PASS
  - `make authz-lint`：PASS
  - `make check routing`：PASS
  - `make check doc`：PASS
  - `make db plan DB_PORT=5432 DB_NAME=iota_erp_061`：PASS（diff 可输出）
  - `make db lint DB_PORT=5432 DB_NAME=iota_erp_061`：PASS
  - `PERSON_MIGRATIONS=1 make db migrate up DB_PORT=5432 DB_NAME=iota_erp_061`：PASS
  - `make org plan DB_PORT=5432 DB_NAME=iota_erp_org_061`：PASS（diff 可输出）
  - `make org lint DB_PORT=5432 DB_NAME=iota_erp_org_061`：PASS
  - `make org migrate up DB_PORT=5432 DB_NAME=iota_erp_org_061`：PASS
  - `make test DB_PORT=5432`：PASS
