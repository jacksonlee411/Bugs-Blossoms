# DEV-PLAN-028 Readiness

> 目标：记录 DEV-PLAN-028（Org 属性继承解析与角色读侧占位）落地后的本地门禁执行结果，作为 CI 对齐与回归依据。

## 变更摘要

- Feature flags：
  - `ORG_INHERITANCE_ENABLED`（默认 `false`）
  - `ORG_ROLE_READ_ENABLED`（默认 `false`）
- API：
  - `GET /org/api/hierarchies?include=resolved_attributes`
  - `GET /org/api/nodes/{id}:resolved-attributes`
  - `GET /org/api/roles`
  - `GET /org/api/role-assignments`
- Authz：
  - 新增 object：`org.roles`、`org.role_assignments`

## 门禁执行记录（2025-12-18 23:31 UTC）

### Go / Lint / Test
- `go fmt ./...`
- `go vet ./...`
- `make check lint`
- `make test`

### Authz
- `make authz-pack`
- `make authz-test`
- `make authz-lint`

### Docs
- `make check doc`

## 备注
- Query Budget：新增 `TestOrg028QueryBudget`（若本地 Postgres 不可用，该类测试会跳过；以 CI 为准）。

