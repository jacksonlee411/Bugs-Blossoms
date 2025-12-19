# DEV-PLAN-034 Readiness：Org 运维治理与压测（Step 14）

## 1. 目标
- 确认 DEV-PLAN-034 的交付物可运行：Org v1 指标、`/org/api/ops/health`、`org-load`（或等价入口）。
- 记录本地门禁命令、压测基线（最小可重复）、以及关键观测点采样。

## 2. 变更摘要（实现侧）
- 新增 `GET /org/api/ops/health`（Authz：`org.ops admin`），输出 outbox/deep-read/cache 等可执行排障线索。
- 新增 Org v1 Prometheus 指标：
  - `org_api_requests_total{endpoint,result}`
  - `org_api_latency_seconds{endpoint,result}`
  - `org_cache_requests_total{cache,result}`
  - `org_cache_invalidate_total{reason}`
  - `org_write_conflicts_total{kind}`
  - `org_deep_read_active_backend{backend}`
- 新增压测入口 `org-load`（Go CLI）以及 `make org-load-run/org-load-smoke`。

## 3. 本地门禁（与 AGENTS.md 对齐）
> 命令细节以 `Makefile` / CI 为准；此处记录执行结果。

- [X] `go fmt ./...`
- [X] `go vet ./...`
- [X] `make check lint`
- [X] `make test`
- [X] `make authz-test && make authz-lint`
- [X] `make check doc`

## 4. 手工验收步骤（建议）
### 4.1 启动服务并启用 Prometheus
- 确保启用 `PROMETHEUS_ENABLED=1`（或等价配置）并可访问 `PROMETHEUS_METRICS_PATH`（默认 `/debug/prometheus`）。
- 确保生产环境仍受 `OpsGuard` 保护（见 `pkg/middleware/ops_guard.go`）。

### 4.2 Org ops health
- 端点：`GET /org/api/ops/health`
- 预期：
  - 200/503 与 `status` 一致（`healthy|degraded|down`）
  - `checks` 至少包含 `database/outbox/deep_read/cache`
  - 403 forbidden payload 对齐 026（缺少权限时）

### 4.3 org-load（压测）
- 冒烟：`make org-load-smoke TENANT_ID=<uuid> SID=<sid> BASE_URL=http://localhost:3200`
- 运行：`make org-load-run TENANT_ID=<uuid> SID=<sid> PROFILE=org_read_1k`
- 产物：`tmp/org-load/org_load_report_<profile>_<timestamp>.json`

## 5. 观测采样（落盘）
> 将关键采样（截图/文本）补充到本节，便于回归对比（需在可运行环境中执行）。

- [ ] Prometheus 指标采样（至少包含 `org_api_requests_total`、`org_api_latency_seconds`、`outbox_pending`）
- [ ] `/org/api/ops/health` 返回示例（healthy 与 degraded 各 1 份）
- [ ] `org_load_report.v1` 摘要（p50/p95/p99、errors、阈值判定）
