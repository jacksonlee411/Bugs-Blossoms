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

## 6. 实际跑通记录（2025-12-19）

- 时间：2025-12-19
- 环境：
  - OS：WSL2 Linux（`uname -a`：`Linux DESKTOP-S9U9E9K 5.15.167.4-microsoft-standard-WSL2 ...`）
  - CPU：AMD Ryzen AI 9 365（20 vCPU，10 cores，`lscpu`）
  - 内存：15Gi（`free -h`）
  - Postgres：17.7（docker `postgres:17`，`db_version=17.7 (Debian 17.7-3.pgdg13+1)`）
  - DB：`DB_HOST=localhost` `DB_PORT=5438` `DB_NAME=iota_erp`
  - 关键 env：`ORG_READ_STRATEGY=path`（默认）、`ORG_CACHE_ENABLED=false`（默认）、`RLS_ENFORCE=disabled`（默认）
- 依赖准备：
  - 启动 DB：`make db local`
  - 跑迁移：`make db migrate up`
- 数据集（固定 1k profile）：
  - tenant：`20a8c36b-3348-434c-abe0-22408e9ba5df`
  - effective_date：`2025-01-01T00:00:00Z`
  - scale/profile/seed：`1k` / `balanced` / `42`
  - dry-run：`TENANT_ID=20a8c36b-3348-434c-abe0-22408e9ba5df SCALE=1k SEED=42 PROFILE=balanced make org-perf-dataset`
  - apply：`TENANT_ID=20a8c36b-3348-434c-abe0-22408e9ba5df SCALE=1k SEED=42 PROFILE=balanced APPLY=1 make org-perf-dataset`
- 基准结果（DB backend；`ITERATIONS=200` / `WARMUP=50` / `CONCURRENCY=1`；git_rev=`eec640b5a6f792a767751400dcbb4ea8fa4e50f4`）：
  - run1：P50=6.731ms P95=7.729ms P99=8.795ms（`tmp/org-perf/report-run1.json`）
  - run2：P50=6.881ms P95=8.017ms P99=8.803ms（`tmp/org-perf/report-run2.json`）
  - run3：P50=6.641ms P95=7.643ms P99=9.086ms（`tmp/org-perf/report-run3.json`）
  - 结论：
    - 性能预算：3 次 P99 均远低于 200ms（通过）。
    - 可重复性：以 3 次 P99 中位数（8.803ms）为基准，偏差 -0.09% / 0% / +3.21%（通过）。
- Query Budget（防 N+1）：
  - 命令：`go test ./modules/org/services -run '^TestOrgTreeQueryBudget$' -count=1`
  - 结果：small(10 nodes)=1 query；large(1k nodes)=1 query；budget=1（通过）。
- 灰度/回滚演练（dry-run + apply）：
  - 数据层：已演练 dataset `dry-run` 与 `apply`（基准租户独立、可重复导入）。
  - 开关层：回滚可通过 `ORG_ROLLOUT_MODE=disabled` 或将租户从 `ORG_ROLLOUT_TENANTS` 移除；降级可通过 `ORG_READ_STRATEGY=recursive`，排除缓存因素可 `ORG_CACHE_ENABLED=false`。
