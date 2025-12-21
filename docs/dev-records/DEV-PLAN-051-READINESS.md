# DEV-PLAN-051-READINESS：Position Management（051）Readiness 记录

**状态**: 阶段 E-Reporting（DEV-PLAN-057）已实现；059/059A（收口与上线）代码已合并（线上 rollout 演练待执行，2025-12-21）

## 1. 范围
- 本 readiness 覆盖 [DEV-PLAN-051](../dev-plans/051-position-management-implementation-blueprint.md) 的：
  - 阶段 C-Authz：子计划 [DEV-PLAN-054](../dev-plans/054-position-authz-policy-and-gates.md)
  - 阶段 C-UI：子计划 [DEV-PLAN-055](../dev-plans/055-position-ui-org-integration.md)
  - 阶段 E-Reporting：子计划 [DEV-PLAN-057](../dev-plans/057-position-reporting-and-operations.md)

## 2. 前置条件（未执行前 checklist）

### 2.1 053（Position/Assignment API）已具备鉴权接入点
- [X] 结论：满足
- 证据点：
  - 端点均统一调用 `ensureOrgAuthz`：`modules/org/presentation/controllers/org_api_controller.go`

### 2.2 Authz SSOT 工具链可用（pack / fixtures parity / lint）
- [X] 结论：满足
- 证据点：
  - 命令入口：`Makefile`
  - 策略碎片目录：`config/access/policies/`

### 2.3 029 deep-read 后端可用（无递归 CTE）
- [X] 结论：满足
- 证据点：
  - descendants 查询入口：`modules/org/infrastructure/persistence/org_033_export_repository.go`
  - 报表 scope 复用（subtree）：`modules/org/services/org_service_057.go`

## 3. 执行记录（命令 + 结果回填）

### 3.1 Authz 门禁
- [X] `make authz-pack` ——（2025-12-20）结果：通过
- [X] `make authz-test && make authz-lint` ——（2025-12-20）结果：通过

### 3.2 Go 质量门禁
- [X] `go fmt ./... && go vet ./...` ——（2025-12-20）结果：通过
- [X] `make check lint` ——（2025-12-20）结果：通过
- [X] `make test` ——（2025-12-20）结果：通过

### 3.3 UI 门禁（DEV-PLAN-055）
- [X] `make generate && make css` ——（2025-12-20）结果：通过
- [X] `make check tr` ——（2025-12-20）结果：通过
- [X] `make check routing` ——（2025-12-20）结果：通过
- [X] E2E 用例补齐：`e2e/tests/org/org-ui.spec.ts` ——（2025-12-20）结果：已新增（未在此记录中运行）

### 3.4 057（阶段 E-Reporting）门禁 + 冒烟 + 性能摘要
- [X] `go fmt ./... && go vet ./...` ——（2025-12-21）结果：通过
- [X] `make check lint` ——（2025-12-21）结果：通过
- [X] `make test` ——（2025-12-21）结果：通过
- [X] `make check doc` ——（2025-12-21）结果：通过
- [X] `go test ./modules/org/services -run TestOrg057Staffing -v` ——（2025-12-21）结果：通过（`staffing:summary` query budget：小/大数据集均为 4 queries）
- [X] `go test ./modules/org/presentation/controllers -run TestOrgAPIController_StaffingReports_RequirePositionReportsRead -v` ——（2025-12-21）结果：通过

### 3.5 059/059A（收口与上线）门禁 + 冒烟 + 追溯闭环
- [X] 实现合并：PR #107（059A）https://github.com/jacksonlee411/Bugs-Blossoms/pull/107 ——（2025-12-21）merge commit：`7ff68853e29fb59cf0fbc867a8a9e3201d7dc939`
- [X] 实现合并：PR #108（059 补齐）https://github.com/jacksonlee411/Bugs-Blossoms/pull/108 ——（2025-12-21）merge commit：`094b4156e13508fc49428365e4120443cd9b5164`
- [X] `go fmt ./... && go vet ./...` ——（2025-12-21）结果：通过
- [X] `make generate && make css` ——（2025-12-21）结果：通过
- [X] `make check lint` ——（2025-12-21）结果：通过
- [X] `make test` ——（2025-12-21）结果：通过
- [X] `make check doc` ——（2025-12-21）结果：通过
- [X] `go test ./modules/org/services -run '^TestOrg059A' -v -count=1` ——（2025-12-21）结果：通过
- [X] 可复跑冒烟脚本：`scripts/org/059A_smoke.sh` ——（2025-12-21）结果：已新增（脚本内含固定 request_id 与追溯 SQL）
- [X] Org DB 工具链门禁（本地回填，隔离 DB）：`make org plan DB_NAME=org_059a_gate ATLAS_DEV_DB_NAME=org_dev_059a_gate` ——（2025-12-21）结果：通过（plan 输出为 CREATE/INDEX/CONSTRAINT 为主，无 DROP）
- [X] Org DB 工具链门禁（本地回填，隔离 DB）：`make org lint DB_NAME=org_059a_gate ATLAS_DEV_DB_NAME=org_dev_059a_gate` ——（2025-12-21）结果：通过
- [X] Org DB 工具链门禁（本地回填，隔离 DB）：`make org migrate up DB_NAME=org_059a_gate GOOSE_TABLE=goose_db_version_org` ——（2025-12-21）结果：通过（version=20251221090000）
- [X] 迁移回滚演练（最小集）：`GOOSE_STEPS=1 make org migrate down DB_NAME=org_059a_gate GOOSE_TABLE=goose_db_version_org` + `make org migrate up ...` ——（2025-12-21）结果：通过
- [X] 追溯 SQL 结果摘要（来自 `scripts/org/059A_smoke.sh` 生成的测试 DB）——（2025-12-21）结果：
  - `shadow`：`reason_code=legacy, reason_code_mode=shadow, original_missing=true, filled=true`；对应 outbox 条目数 `1`
  - `enforce`：缺失输入的 request_id（pos/assignment）在 `org_audit_logs/org_outbox` 计数均为 `0`
  - `disabled`：`reason_code=''（空）, reason_code_mode=disabled, original_missing=true, filled=false`
- [X] 059 最小冒烟脚本（Position/Assignment/Reports/Vacancy）：`scripts/org/059_smoke.sh` ——（2025-12-21）结果：通过（脚本内含固定 request_id 与追溯 SQL）
