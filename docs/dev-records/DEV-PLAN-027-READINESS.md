# DEV-PLAN-027 Readiness

本记录用于复现 `docs/dev-plans/027-org-performance-and-rollout.md` 的最小可用交付（性能基准 + Query Budget 守卫 + 灰度开关口径）与门禁验证路径。

## 1. 本次交付范围（M1）

- 性能基准入口：
  - `make org-perf-dataset`：生成固定 profile 数据集（默认 dry-run，`APPLY=1` 才落库）
  - `make org-perf-bench`：运行树读取基准并输出 `./tmp/org-perf/report.json`
  - Go CLI：`cmd/org-perf`（`dataset apply` / `bench tree`）
- 灰度开关（按租户 allowlist）：
  - `ORG_ROLLOUT_MODE=disabled|enabled`
  - `ORG_ROLLOUT_TENANTS=<uuid,uuid,...>`
  - `ORG_READ_STRATEGY=path|recursive`
  - `ORG_CACHE_ENABLED=true|false`
- 查询计数守卫（CI）：
  - `TestOrgTreeQueryBudget`：对“树读取场景”执行 query budget 检查，捕获 N+1 回退。

## 2. 本地验证命令

- Go 门禁：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`
- 文档门禁：
  - `make check doc`

## 3. 基准数据集与基准执行（示例）

> 说明：基准租户必须是**空租户**（`org_nodes` 等表无数据），且 `tenants` 表中存在该租户（建议用基准专用租户）。

1. 数据集 dry-run（不落库）：
   - `TENANT_ID=<tenant_uuid> SCALE=1k SEED=42 PROFILE=balanced make org-perf-dataset`
2. 数据集 apply（落库）：
   - `TENANT_ID=<tenant_uuid> SCALE=1k SEED=42 PROFILE=balanced APPLY=1 make org-perf-dataset`
3. DB 基准（输出 JSON 报告）：
   - `TENANT_ID=<tenant_uuid> SCALE=1k SEED=42 PROFILE=balanced BACKEND=db make org-perf-bench`
   - 输出：`./tmp/org-perf/report.json`

## 4. Query Budget 守卫（CI/本地）

- `SCALE=1k PROFILE=balanced SEED=42 TENANT_ID=<tenant_uuid> go test ./modules/org/... -run '^TestOrgTreeQueryBudget$' -count=1`

## 5. 灰度/回滚开关（环境变量）

- 一键下线（推荐默认）：
  - `ORG_ROLLOUT_MODE=disabled`
- 开启灰度（仅 allowlist 租户可用）：
  - `ORG_ROLLOUT_MODE=enabled`
  - `ORG_ROLLOUT_TENANTS=<tenant_uuid>`
- 降级/回滚：
  - `ORG_READ_STRATEGY=recursive`（正确性优先的回退路径）
  - `ORG_CACHE_ENABLED=false`（排除缓存一致性因素）

## 6. 实际跑通记录（待填写）

- 时间：
- 环境（CPU/内存/DB 版本、关键 env）：
- 数据集（tenant/scale/seed/profile）：
- 基准结果（P50/P95/P99，重复 3 次波动）：
- Query Budget（小/大规模查询次数）：
- 灰度/回滚演练结论（dry-run + apply）：

