# DEV-PLAN-026 Readiness

> 本记录用于对齐仓库门禁与 `docs/dev-plans/026-org-api-authz-and-events.md` 的 Readiness 清单。

## 门禁命令记录

### Go / Lint / Test
- `go fmt ./...`
- `go vet ./...`
- `make check lint`
- `make test`

### Authz
- `make authz-test authz-lint authz-pack`
- `git status --short`（必须为空）

### Org Atlas/Goose（命中 `migrations/org/**`）
- `make org plan`
- `make org lint`
- `make org migrate up`

## 结果

- `go fmt ./...`：PASS
- `go vet ./...`：PASS
- `make check lint`：PASS
- `make test`：PASS

- `make authz-test authz-lint authz-pack`：PASS

- `make org plan`：PASS（plan 输出包含 `org_outbox`）
- `make org lint`：PASS
- `make org migrate up`：PASS（迁移到 `20251218150000_org_outbox.sql`）
