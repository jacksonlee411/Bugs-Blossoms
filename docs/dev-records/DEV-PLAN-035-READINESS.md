# DEV-PLAN-035-READINESS：Org UI（M1）Readiness 记录

**状态**: 草拟中（2025-12-18 11:29 UTC）— 本文件先提供“未执行前 checklist + 证据点”；执行后在对应项回填时间戳/结果/链接

## 1. 范围
- 本 readiness 覆盖 [DEV-PLAN-035](../dev-plans/035-org-ui.md) 的实施前置条件核对与关键命令记录。
- IA/侧栏集成的决策与约束以 [DEV-PLAN-035A](../dev-plans/035A-org-ui-ia-and-sidebar-integration.md) 为准。

## 2. 前置条件（未执行前 checklist）
> 每项需满足“证据点可定位 + 结论明确”。如结论为不满足，请在同一项下追加“阻塞原因/修复计划/负责人/预计完成时间”。

### 2.1 026（API/Authz/错误码）已稳定可供 UI 复用
- [ ] 结论：满足/不满足
- 证据点（代码/配置）：
  - API 端点已具备树/节点/分配主链入口：`modules/org/presentation/controllers/org_api_controller.go`
  - Org Authz object 常量包含 `org.hierarchies/org.nodes/org.edges/org.assignments`：`modules/org/presentation/controllers/authz_helpers.go`
  - 策略至少覆盖 superadmin（含 edges）：`config/access/policies/org/org.csv` 与 `config/access/policy.csv`
- 约束（并行开发时必须遵守）：
  - 任何性能/灰度改造不得破坏 UI 依赖的 API 契约（尤其 `GET /org/api/hierarchies`、`GET /org/api/assignments` 的 query/返回结构）。

### 2.2 014D/015（Unauthorized/申请入口/403 契约）可复用
- [ ] 结论：满足/不满足
- 证据点（代码/组件）：
  - 统一 403 输出（JSON/HTMX/Full page）：`modules/core/presentation/templates/layouts/authz_forbidden_responder.go`
  - Unauthorized 组件（含申请入口 `/core/api/authz/requests`）：`components/authorization/unauthorized.templ`
  - 模板 capability 判断：`pkg/types/pagecontext.go`（`CanAuthz`）

### 2.3 templ + Tailwind 工具链可用
- [ ] 结论：满足/不满足
- 证据点（命令入口）：
  - `make generate`：`Makefile`（`go generate ./... && templ generate`）
  - `make css`：`Makefile`（`tailwindcss -c tailwind.config.js ...`）
- 环境要求（如不满足需补齐）：
  - 本机可执行 `templ`（版本可打印）
  - 本机可执行 `tailwindcss`

### 2.4 E2E 基础设施可用（为新增 org 套件做准备）
- [ ] 结论：满足/不满足
- 证据点：
  - e2e 工程存在（Playwright）：`e2e/package.json`
  - 当前 `e2e/tests/` 目录结构（确认新增 `e2e/tests/org/` 的位置与约定）：`e2e/tests/`

## 3. 执行记录（命令 + 结果回填）
> 执行后将 `[ ]` 改为 `[X]`，并补充时间戳、结果、必要链接（例如 PR、截图放 `docs/assets/` 并在此引用）。

### 3.1 生成物与 UI 资源
- [X] `make generate && make css` —— （2025-12-18 12:48 UTC）结果：通过（已执行 `make generate`、`make css`）
- [ ] `git status --short` —— （2025-12-18 12:48 UTC）结果：非空（本次变更未提交，包含新增 UI/模板/locale/e2e 等文件）

### 3.2 Go 质量门禁（若命中 Go 代码）
- [X] `go fmt ./... && go vet ./...` —— （2025-12-18 12:48 UTC）结果：通过（已执行 `make fix fmt`、`go vet ./...`）
- [X] `make check lint && make test` —— （2025-12-18 12:48 UTC）结果：`make check lint` 通过；`make test` 失败（`pkg/crud` 测试依赖本地 `127.0.0.1:5432`，当前环境连接被拒绝）

### 3.3 路由治理（如新增 UI 路由命中）
- [X] `make check routing` —— （2025-12-18 12:48 UTC）结果：通过

### 3.4 翻译门禁（如新增/调整 locale keys）
- [X] `make check tr` —— （2025-12-18 12:48 UTC）结果：通过

### 3.5 Authz 门禁（如调整策略/聚合）
- [ ] `make authz-test && make authz-lint` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败
- [ ] 如修改 `config/access/policies/**`：`make authz-pack` —— （YYYY-MM-DD HH:MM UTC）结果：通过/失败

### 3.6 E2E（新增 Org 套件后）
- [X] `make e2e test` —— （2025-12-18 12:48 UTC）结果：失败（提示需先启动 e2e server：`make e2e dev`）
- [ ] 或 `cd e2e && pnpm test tests/org` —— （YYYY-MM-DD HH:MM UTC）结果：待执行（需先启动 e2e server 或补齐 CI/本地运行方式）
