# DEV-PLAN-041：彻底移除 warehouse（仓库）模块（Hard Delete）

**状态**: 规划中（2025-12-16 14:17 UTC）

## 1. 背景与上下文 (Context)

- DEV-PLAN-040 已将 `modules/finance` / `modules/billing` / `modules/crm` / `modules/projects` 从主干 Hard Delete，目标是收敛仓库“有效代码面”，降低路由/测试/维护成本。
- 本计划在 DEV-PLAN-040 的模块收敛策略基础上，继续彻底移除 `modules/warehouse`（仓库）模块，确保主干只保留当前明确需要维护与演进的模块。
- 当前 `modules/warehouse` 提供：
  - UI：`/warehouse/*`（Products/Positions/Orders/Units/Inventory）
  - GraphQL：`/query/warehouse`
  - 权限集：`modules/warehouse/permissions/*`（并被 `pkg/defaults/schema.go` 纳入默认权限 schema）
  - 模块 schema：`modules/warehouse/infrastructure/persistence/schema/warehouse-schema.sql`（通过 `app.Migrations().RegisterSchema` 注册到 schema collector）

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] 仓库内不再存在 `modules/warehouse` 目录及其源码/模板/资源文件。
- [ ] 不再注册/暴露 warehouse 的服务、controller、路由入口、导航入口、locale、GraphQL schema（`/query/warehouse`）。
- [ ] 默认权限 schema 不再包含 warehouse 权限（`pkg/defaults/schema.go` 清理 imports 与 permission set），并确保 seed 口径收敛。
- [ ] CleanArchGuard 配置不再引用 warehouse 包路径（`.gocleanarch.yml`）。
- [ ] 文档/示例不再引用已删除的 warehouse 路径与文件（至少覆盖：路由策略文档、JS Runtime 示例、TestKit 说明）。
- [ ] 通过 CI 同口径门禁（按触发器补齐）：`make generate && make css && make check tr && go fmt ./... && go vet ./... && make check lint && make test && make check doc`。

### 2.2 非目标（Out of Scope）

- 不在本计划内删除生产数据库中的历史表/数据；本计划只移除代码与入口。
  - 若需要清理 warehouse 相关表（`warehouse_*`、`inventory_*`），将另立计划，明确数据归档与回滚策略。
- 不在本计划内提供替代的“库存/商品/订单/单位/盘点”实现或数据迁移工具。

## 3. 影响面梳理 (Surface Area)

### 3.1 路由/入口

- UI 前缀：`/warehouse/*`
  - 典型入口：`/warehouse/products`、`/warehouse/positions`、`/warehouse/orders`、`/warehouse/units`、`/warehouse/inventory`
  - 典型查询：`/warehouse/positions/search`、`/warehouse/inventory/positions/search`
- GraphQL：`/query/warehouse`

### 3.2 DB 表（遗留保留，不在本计划内删除）

来自 `modules/warehouse/infrastructure/persistence/schema/warehouse-schema.sql`：

- `warehouse_units`
- `warehouse_positions`
- `warehouse_position_images`
- `warehouse_products`
- `warehouse_orders`
- `warehouse_order_items`
- `inventory_checks`
- `inventory_check_results`

> 说明：历史 `migrations/**` 会继续保留，因此“新库初始化”时仍可能创建上述表；本计划仅移除代码与入口，不做 schema 重基线/清理。

### 3.3 代码与配置引用点（必须清理）

- 模块加载与导航：
  - `modules/load.go`：`warehouse.NewModule()`、`warehouse.NavItems`
  - 默认权限 schema：
    - `pkg/defaults/schema.go`：`modules/warehouse/permissions` imports + permission set
    - 相关测试（例如 core 角色/权限测试）可能引用 warehouse permissions
  - TestKit（测试端点/seed 场景）：
    - `modules/testkit/domain/schemas/populate_schema.go`：`WarehouseSpec` / `data.warehouse`
    - `modules/testkit/services/test_data_service.go`：`warehouse` 场景描述与样例数据
    - `modules/testkit/services/populate_service.go`：warehouse populate 分支与 `createWarehouseData` 占位实现
  - 路由示例/演示页：
    - `modules/core/presentation/templates/pages/showcase/components/combobox.templ` 目前引用 `"/warehouse/positions/search"`（删除模块后应同步修正示例）
  - CleanArchGuard：
    - `.gocleanarch.yml` 当前含 warehouse 包路径
- 文档引用（删除模块后会出现“文件路径不存在/示例过期”）：
  - `docs/dev-plans/018-routing-strategy.md`、`docs/dev-plans/018A-routing-strategy-review.md`
  - `docs/js-runtime/js-runtime-integration-spec.md`、`docs/js-runtime/tasks/phase-02-domain-entities.md`
  - `docs/SUPERADMIN.md`（模块列表描述）
  - `modules/testkit/README.md`（warehouse 场景说明）
  - `e2e/README.md`、`e2e/tests/README.md`（未来模块清单描述）
  - 其他命中文档以 `rg -n "modules/warehouse|/warehouse" docs -S` 为准

## 4. 架构与关键决策 (Architecture & Decisions)

- [ ] ADR-041-01（删除策略）：Hard Delete（目录删除 + 清理所有引用：Go import/模块注册/导航/权限 schema/cleanarch/doc）。
- [ ] ADR-041-02（路由处置）：不保留 tombstone handler；移除模块注册后 `/warehouse/*` 与 `/query/warehouse` 最终行为为全局 NotFound（HTTP 404）。
- [ ] ADR-041-03（DB 处置）：不删表、不生成 drop migrations；不运行 `command migrate collect` 作为本计划的一部分。

## 5. 实施步骤 (Milestones)

> 约束：每个里程碑完成后都必须保持 `go test ./...` 可运行，且本地门禁命令可通过。

### 5.0 Readiness（执行前置确认）

- [ ] 确认不存在对外依赖：`/warehouse/*` UI 与 `/query/warehouse` GraphQL 已完成下线/替代（或确认可接受 404）。
- [ ] 确认仓库内无其他模块依赖 warehouse 包（执行 `rg -n "modules/warehouse|/warehouse" -S .` 并清理非文档引用）。
- [ ] 确认路由 SSOT 未登记 `/warehouse`：`config/routing/allowlist.yaml` 不应包含 `/warehouse`（若存在则一并移除，保持与实际路由一致）。

1. [ ] M1：清理加载入口与权限 schema（确保可编译）
   - [ ] `modules/load.go`：移除 warehouse module 注册与 NavLinks concat。
   - [ ] `pkg/defaults/schema.go`：移除 warehouse permission set（以及任何依赖 warehouse permissions 的测试/seed 逻辑）。
   - [ ] `.gocleanarch.yml`：移除 warehouse 包路径条目。
2. [ ] M2：清理路由示例与测试依赖
   - [ ] 修正 `modules/core/presentation/templates/pages/showcase/components/combobox.templ` 中的 warehouse endpoint 示例（替换为仍存在的搜索/示例入口）。
      - [ ] `.templ` 变更后运行 `make generate && make css`，并确保 `git status --short` 为空（生成物需提交的部分必须提交）。
    - [ ] 搜索并清理仓库内所有 `modules/warehouse` imports（`rg -n "modules/warehouse" -S .`）。
    - [ ] TestKit 清理：移除 `warehouse` 场景与 populate schema 中的 warehouse 字段（避免“已删除模块仍暴露测试入口/文案”）。
3. [ ] M3：Hard Delete 模块目录 + 依赖收敛
   - [ ] 删除 `modules/warehouse`。
   - [ ] `go mod tidy`，确认仅为依赖收敛，不引入无关变更。
4. [ ] M4：文档口径收敛（避免引用已删除路径）
   - [ ] 更新/替换 docs 中的 warehouse 示例路径与文件引用（见 3.3）。
   - [X] 更新 `AGENTS.md` Doc Map：新增本计划链接（本文件）。
5. [ ] M5：门禁验证
   - [ ] `make generate && make css`
   - [ ] `make check tr`（因为会删除 `modules/warehouse/presentation/locales/**/*.json`）
   - [ ] `go fmt ./... && go vet ./... && make check lint && make test`
   - [ ] `make check doc`

## 6. 验收标准 (Acceptance Criteria)

- [ ] `modules/warehouse` 目录不存在。
- [ ] `rg -n "github.com/iota-uz/iota-sdk/modules/warehouse" -S .` 无匹配（允许 `docs/Archived/**` 作为历史材料存在，但需避免把已删除路径作为“现行规范示例”）。
- [ ] `modules/load.go` 不再加载 warehouse；`pkg/defaults/schema.go` 不再引用 warehouse permissions。
- [ ] `.gocleanarch.yml` 不再包含 warehouse 包路径。
- [ ] `/warehouse/*` 与 `/query/warehouse` 在主服务上均不可达（最终行为为 404/NotFound）。
- [ ] `go fmt ./... && go vet ./... && make check lint && make test` 通过。
- [ ] `make check tr && make check doc` 通过。

## 7. 运维与回滚 (Ops & Rollback)

- 运维影响：移除 `/warehouse/*` 与 `/query/warehouse` 后，任何调用方会收到 404；上线前需确认不存在依赖或已完成替代/下线。
- 回滚策略：Hard Delete 不提供细粒度回滚；需要回滚时使用 Git revert 或回退到删除前 tag。
