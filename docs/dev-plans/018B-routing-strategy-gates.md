# DEV-PLAN-018B：路由策略门禁启动方案（Routing Quality Gates）

**状态**: 已完成（2025-12-16 11:05 UTC）

## 1. 背景与上下文 (Context)

DEV-PLAN-018/018A 已完成“全局路由策略统一”的 SSOT 与最小落地（M1），并引入了：
- allowlist SSOT：`config/routing/allowlist.yaml`
- `/api` 版本化约束（route-lint）：`internal/routelint/routelint_test.go`
- API 命名空间的全局错误返回契约（404/405 JSON-only）：`modules/core/presentation/controllers/errors_controller.go`
- dev-only 与 ops 路由的暴露基线：`pkg/middleware/ops_guard.go`、`modules/core/module.go` 等

上述内容属于“治理型契约”：一旦后续新增/改动路由而缺少自动化门禁，最容易出现“例外未登记”“/api 回退为非版本化”“API 误返回 HTML”“生产暴露 dev-only/ops 端点”等口径漂移。为保证 018 的契约可持续，需要将关键约束固化为可执行门禁，并在 CI/分支保护中作为 required checks。

关联文档：
- 路由策略 SSOT：`docs/dev-plans/018-routing-strategy.md`
- 评审输入与对齐：`docs/dev-plans/018A-routing-strategy-review.md`
- 仓库质量门禁总纲：`docs/dev-plans/005-quality-gates.md`、`.github/workflows/quality-gates.yml`

## 2. 目标与非目标 (Goals & Non-Goals)

**核心目标**：
- [X] 将 018 的关键路由契约转换为“可自动阻断的门禁”（CI/本地均可执行）。
- [X] 新增/改动路由时，出现以下行为必须被门禁阻断：
  - [X] 新增非版本化 `/api/*`（除非在 allowlist 登记；迁移窗口/owner 由附录 B + PR 描述承载）。
  - [X] allowlist 无法加载/格式非法/entrypoint 缺失，导致错误处理与分类逻辑静默降级。
  - [X] `/{module}/api/*` 与 `/api/v1/*` 在 404/405/500 等全局错误路径返回 HTML（必须 JSON-only）。
  - [X] 生产环境暴露 `/_dev/*`、`/playground`、`/__test__/*` 或缺少 ops 保护基线（至少一种：网关 allowlist / BasicAuth / OpsGuard）。
- [X] 提供开发者低摩擦入口（`make check routing`），让路由相关改动可以“就地自检”。

**约束澄清（可自动化 vs 流程约束）**：
- 本计划的“门禁”以 **CI 可执行、可阻断** 为定义；无法从代码侧可靠判断的部署约束（例如“网关侧已做 CIDR allowlist”）不纳入自动门禁，而纳入 review checklist（见第 5 节）。
- “说明迁移窗口/owner/截止时间”目前 **无法写入 allowlist SSOT**（`config/routing/allowlist.yaml` 仅含 `prefix/class`），因此：
  - 自动门禁仅校验“例外必须登记在 allowlist”；
  - 迁移窗口说明作为 **流程要求**：在 `docs/dev-plans/018-routing-strategy.md`（附录 B）与 PR 描述中同步登记（见第 5 节第 1、5 项）。

**非目标（Out of Scope）**：
- 不在本计划内一次性迁移/重写所有存量路由；迁移节奏仍以 018 的 M2+ 为准。
- 不在本计划内引入新的路由框架或全量 OpenAPI 生成门禁（如需引入，另开 DEV-PLAN）。

## 3. 拟启动/补强的门禁清单 (Gates)

### 3.1 Gate-A：Route Lint（禁止新增非版本化 `/api/*`）

**契约来源**：DEV-PLAN-018 的 `/api/v1/*` 强版本化约束。  
**门禁实现**：`internal/routelint/routelint_test.go`（已存在）。  
**阻断规则**：出现 `/api/*` 且不是 `/api/v1/*` 且不匹配 allowlist 时，测试失败并列出 offending routes。  
**注意**：该测试通过构建 server/superadmin router 进行收集，依赖 DB 连接可用（与现有 CI test job 一致）。  
**本地运行提示**：
- 该门禁属于“路由收集型集成测试”，会构建完整 router；本地执行前应确保数据库可连接（命令入口以 `Makefile` 为准）。

### 3.2 Gate-B：Allowlist 健康检查（防止静默降级）

**问题**：错误处理与分类逻辑在 allowlist 加载失败时会降级为 `rules=nil`，可能导致 API/HTML 口径漂移不易被发现。  
**门禁目标**：保证 `config/routing/allowlist.yaml` 在默认路径下可被加载，且包含 `server`、`superadmin` entrypoint，规则合法（prefix、class、version）。  
**门禁形式**（建议）：
- [X] `go test`：新增 `pkg/routing` 下的轻量测试，显式断言 `routing.LoadAllowlist("", "server")`/`("superadmin")` 不报错。
- [X] 必做增强：断言关键前缀与 route_class 映射稳定（避免误删/误分类导致行为回退）。最小集合建议包含：
  - `server`：`/api/v1 -> public_api`、`/health -> ops`、`/debug/prometheus -> ops`、`/_dev -> dev_only`、`/playground -> dev_only`、`/__test__ -> test`。
  - `superadmin`：至少断言 `entrypoints.superadmin` 存在且非空（后续如引入 ops/dev/test 前缀，同步补齐断言）。
**备注**：Gate-B 的目标是阻断“allowlist 失效导致 classifier 静默退化”的真实风险（ops/dev/test 的分类依赖 allowlist，而 `/api/v1` 有兜底规则）。

### 3.3 Gate-C：API 全局错误返回契约（404/405/500 JSON-only）

**契约来源**：DEV-PLAN-018 5.5（内部/对外 API 不得返回 HTML）。  
**门禁目标**：以“全局 handler + panic recovery”作为单一事实源，做稳定断言（避免依赖某个业务路由是否存在）。
- [X] 404：`GET /api/v1/__nonexistent__` 与 `GET /core/api/__nonexistent__` 返回 JSON（至少包含 `code/message` 与 `meta.path`）。
- [X] 405：构造一个最小 mux router（仅注册 `GET /api/v1/ping`），对 `POST /api/v1/ping` 断言为 JSON（至少包含 `code/message` 与 `meta.method/meta.path`）。
- [X] 500：用会 panic 的 handler + `middleware.WithLogger` 断言 `/api/v1/*`（或 `/{module}/api/*`）在未写 header 前发生 panic 时返回 JSON（至少包含 `code/message/meta.request_id/meta.path`）。
- [X] UI 404 保持非 JSON-only：对任意 UI 路径（如 `/__nonexistent_ui__`，且不显式设置 `Accept: application/json`）断言 `Content-Type` 不为 `application/json`（允许 HTML/纯文本；目标是避免 UI 被误切为 JSON-only）。

### 3.4 Gate-D：环境暴露基线（dev-only/test/ops）

**契约来源**：DEV-PLAN-018 安全验收条目。  
**门禁目标**：
- [X] 生产配置（或等价 env）下，`/_dev/*` 与 `/playground` 默认不可用（404）。
- [X] 生产配置（或等价 env）下，`/__test__/*` 默认不可用（404）。
- [X] 生产配置下，`/health` 与 `/debug/prometheus` 至少具备一层保护基线（OpsGuard/网关/BasicAuth），避免“默认公网可达”。

> 注：如果现有实现无法在纯单测中稳定判定（例如依赖外部网关），Gate-D 至少应对“应用侧是否注册/是否被 OpsGuard 包裹”做可测试的最小断言。
>
> “生产配置（等价 env）”建议在测试中显式设置：`GO_APP_ENV=production`、`ENABLE_DEV_ENDPOINTS=false`、`ENABLE_GRAPHQL_PLAYGROUND=false`、`ENABLE_TEST_ENDPOINTS=false`（其余按默认值）。
>
> 建议落地为两段最小可测断言：
> - **D-ops（纯单测）**：`GO_APP_ENV=production` 且 `OPS_GUARD_ENABLED=true` 时，对 `/health`/`/debug/prometheus` 的未授权请求应返回 404（表示被 guard 遮蔽）；提供 token/BasicAuth 时应放行（可选增强）。
> - **D-register（集成测试）**：以“构建 router 并收集 routes”的方式，在 `GO_APP_ENV=production` 且默认关闭 dev/playground/test endpoints 时，断言 router 中不存在 `/_dev`、`/playground`、`/__test__` 前缀路由。

## 4. CI 集成策略 (CI & Required Checks)

### 4.1 CI 入口
以现有 `Quality Gates` workflow 为单一事实源（见 `.github/workflows/quality-gates.yml`）：
- Gate-A/B/C/D 均应落在 `go test` 覆盖范围内（推荐归入 `test-unit-integration` job）。
- 若担心定位困难，可在 CI 中追加“Routing Gates”专用 step/job（只跑路由相关测试包），但仍保持 `Quality Gates` 为 required check。

### 4.2 本地入口（建议）
- [X] 新增 `make check routing`：聚合运行 Gate-A/B/C/D 对应的测试包（以及必要的 env 提示），让开发者在改路由/allowlist 前后能快速自检。

## 5. 实施步骤 (Checklist)

1. [X] 明确门禁边界与 owner —— 由 Core/Infra 维护路由 allowlist 与 route-lint；各模块新增路由须遵守 018 的命名空间契约。对“allowlist 例外”（尤其非版本化 `/api/*`）必须在 `docs/dev-plans/018-routing-strategy.md`（附录 B）登记 owner/迁移窗口（流程要求）。
2. [X] 落地 Gate-B（allowlist 健康检查）单测：`pkg/routing/allowlist_health_test.go`。
3. [X] 落地 Gate-C（API 错误契约）单测：`internal/routinggates/api_error_contract_test.go`。
4. [X] 落地 Gate-D 的最小可测断言：`internal/routinggates/exposure_baseline_test.go`。
5. [X] 新增 `make check routing` 并登记到 018：`Makefile`、`docs/dev-plans/018-routing-strategy.md`。
6. [X] Readiness 记录（时间戳 + 命令 + 结果）：
   - [X] `make check routing`（2025-12-16 11:05 UTC）
   - [X] `go fmt ./... && go vet ./... && make check lint && make test`（2025-12-16 11:05 UTC）
   - [X] `make check doc`（2025-12-16 11:05 UTC）
7. [X] 将本文档状态更新为 `已完成`（2025-12-16 11:05 UTC）。

## 6. 验收标准 (Acceptance Criteria)

- [X] 任何 PR 若引入非版本化 `/api/*`（且未登记 allowlist）会被 CI 阻断，并输出可定位的 offending route 列表。
- [X] allowlist 文件损坏/entrypoint 缺失会被 CI 阻断（不允许静默降级）。
- [X] `/{module}/api/*` 与 `/api/v1/*` 的 404/405（及至少一个 500 场景）在 CI 中被断言为 JSON-only。
- [X] 生产默认不暴露 `/_dev/*`、`/playground`、`/__test__/*`（可测断言通过）。
- [X] （流程要求）新增/变更 allowlist 例外（尤其非版本化 `/api/*`）时，`docs/dev-plans/018-routing-strategy.md`（附录 B）与 PR 描述包含 owner/迁移窗口；否则 reviewer 有权拒绝合并。
