# DEV-PLAN-030 Readiness

> 目标：记录 DEV-PLAN-030（Org 变更请求与预检）落地后的本地门禁执行结果，作为 CI 对齐与回归依据。

## 变更摘要
- change-requests API：`/org/api/change-requests*`（draft/submit/cancel + list/get）
- preflight API：`POST /org/api/preflight`
- feature flags：
  - `ORG_CHANGE_REQUESTS_ENABLED`（默认 `false`）
  - `ORG_PREFLIGHT_ENABLED`（默认 `false`）

## 门禁执行记录

### Go / Lint / Test
- `go fmt ./...`
- `go vet ./...`
- `make check lint`
- `make test`

### Authz
- `make authz-pack`
- `make authz-test`
- `make authz-lint`

### 路由门禁
- CI：`.github/workflows/quality-gates.yml` 的 `Routing Gates` job

## 备注
- 若本地 Postgres 不可用，部分集成测试可能会失败或跳过；以 CI 环境为准。

