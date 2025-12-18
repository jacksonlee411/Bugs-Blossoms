# DEV-PLAN-024 Readiness

本记录用于复现 `docs/dev-plans/024-org-crud-mainline.md` 的最小可用交付（Org 模块注册 + `/org/api/*` 最小 CRUD 子集）与门禁验证路径。

## 1. 本次交付范围（M1）

- 模块注册：`modules/org/module.go`、`modules/org/links.go`，并接入 `modules/load.go`。
- 路由 allowlist：`config/routing/allowlist.yaml` 增加 `/org`（ui）与 `/org/api`（internal_api）。
- 最小 API（JSON-only）：
  - `GET /org/api/hierarchies`
  - `POST /org/api/nodes`
  - `PATCH /org/api/nodes/{id}`
  - `POST /org/api/nodes/{id}:move`
  - `GET /org/api/assignments`
  - `POST /org/api/assignments`
  - `PATCH /org/api/assignments/{id}`
- 自动空壳 Position 开关：
  - `ENABLE_ORG_AUTO_POSITIONS`（默认 `true`）
  - `ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES`（默认 `false`）

## 2. 本地验证命令

- Go 门禁：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
- 路由门禁：
  - `make check routing`
- 目标包快速校验：
  - `go test ./modules/org/...`
- 全量测试（需要本地 PostgreSQL / compose 环境）：
  - `make test`

