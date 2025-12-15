# DEV-PLAN-019A：PostgreSQL RLS 强租户隔离（PoC）

**状态**: 规划中（2025-12-15 13:47 UTC）

> 本文是 `DEV-PLAN-019` 的子计划，聚焦 **数据隔离（RLS）** 的“代码级详细设计”。身份域与 SSO 见 `DEV-PLAN-019B/019C`。

## 1. 背景与上下文 (Context)
- 当前系统主要依赖应用层在 SQL 中显式追加 `WHERE tenant_id = ...` 达成“逻辑隔离”，存在漏写导致跨租户数据泄露的系统性风险。
- PostgreSQL 17 支持 Row-Level Security（RLS），可将“租户隔离”下沉到数据库层，形成强制约束与兜底安全网。
- 本计划目标是交付一个 **可验证、可回滚、可渐进推广** 的 RLS PoC，作为后续“按模块/按表”推广的模板。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 在 `hrm.employees`（PoC 表）启用 RLS，并保证跨租户读写在 DB 层被拒绝。
- [ ] 定义并实现“租户上下文注入”标准：在事务内设置 `app.current_tenant`，RLS policy 仅信任该值。
- [ ] 明确并落地“连接池下不串租户”的约束：**只使用事务本地变量（`SET LOCAL` / `set_config(..., true)`）**。
- [ ] 提供 Feature Flag 与回滚路径（DB 与应用两侧），保证 PoC 可控。

### 2.2 非目标
- 不在本计划内一次性为所有表启用 RLS（仅 PoC 表 + 可复用模板）。
- 不在本计划内实现“跨租户/全局查询”（如 superadmin 视角的跨租户报表）；该能力需要独立设计（可能通过独立连接角色/视图/专用服务实现）。
- 不在本计划内移除应用层 `tenant_id` 过滤条件（先保留，RLS 作为兜底）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图（Mermaid）
```mermaid
flowchart TD
  Req[HTTP Request] --> MW[Auth Middleware\ninject tenant_id into ctx]
  MW --> Svc[Service/Controller]
  Svc --> Tx[BEGIN Tx\nset_config(app.current_tenant, tenant_id, true)]
  Tx --> Q[SQL Query (may forget tenant_id filter)]
  Q --> PG[(PostgreSQL)]
  PG --> RLS[RLS Policy\nUSING/WITH CHECK]
  RLS --> OK[Rows in same tenant]
  RLS --> Deny[No rows / ERROR]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：租户上下文只通过事务本地设置传递**
  - 选定：`SELECT set_config('app.current_tenant', $1, true)`（或 `SET LOCAL app.current_tenant = ...`）。
  - 原因：连接池复用时，session 级别的 `SET` 可能跨请求泄露；事务本地可天然避免串租户。
- **决策 2：RLS policy 使用 `current_setting('app.current_tenant')::uuid`（fail-closed）**
  - 选定：不使用 `current_setting(..., true)`，缺失上下文时直接报错，尽早暴露注入遗漏。
- **决策 3：应用 DB 连接账号不得为 superuser / BYPASSRLS**
  - 说明：PostgreSQL 的 superuser 或带 `BYPASSRLS` 的角色会绕过 RLS；PoC 必须使用普通应用角色验证隔离有效性。
- **决策 4：系统级组件（Outbox Relay / 后台 Job）不直接依赖“放宽 RLS”**
  - 说明：Outbox Relay/后台 Job 往往需要跨租户扫描队列（当前 outbox relay 的 claim 查询即是全表扫描 + SKIP LOCKED）。
  - 策略：
    - PoC 阶段：**不对系统级队列表（如 `<module>_outbox`）启用 RLS**（系统表作为单独安全域处理）。
    - 若未来确需对系统表启用 RLS：使用**专用连接池/专用 DB role**（可 BYPASSRLS 或通过更细粒度 policy 允许 system role），并补齐审计与最小权限；禁止使用 “`current_setting(...) IS NULL` 则放行” 这类会把注入遗漏变成跨租户可读的策略。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 PoC 表（现状）
`migrations/hrm/00001_hrm_baseline.sql` 定义 `employees`：
- `tenant_id uuid NOT NULL`
- `UNIQUE (tenant_id, email)` / `UNIQUE (tenant_id, phone)`

### 4.2 RLS DDL（PoC）
> 注意：实际 schema 是否带 namespace（`public`/`hrm`）以现有 migrations 为准；PoC 以 `employees` 表为例。

```sql
ALTER TABLE employees ENABLE ROW LEVEL SECURITY;
ALTER TABLE employees FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON employees;
CREATE POLICY tenant_isolation ON employees
  USING (tenant_id = current_setting('app.current_tenant')::uuid)
  WITH CHECK (tenant_id = current_setting('app.current_tenant')::uuid);
```

### 4.3 数据库角色（必须项）
PoC 必须使用非 superuser 账号连接数据库（示例）：
```sql
-- 由管理员或迁移阶段执行（需要具备创建角色权限）
CREATE ROLE iota_app LOGIN PASSWORD '***' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
ALTER ROLE iota_app NOBYPASSRLS;

-- 最小权限示意：按实际 schema/表补齐
GRANT CONNECT ON DATABASE iota_erp TO iota_app;
GRANT USAGE ON SCHEMA public TO iota_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE employees TO iota_app;
```
应用侧需配置 `DB_USER=iota_app`（以及对应密码），确保 RLS 真正生效。

## 5. 接口契约 (API Contracts)
### 5.1 Feature Flag（应用侧）
定义环境变量（PoC）：
- `RLS_ENFORCE=disabled|enforce`（默认 `disabled`）
  - `disabled`：不强制注入 `app.current_tenant`（仅走现有 `WHERE tenant_id` 模式）。
  - `enforce`：在开启事务时注入 `app.current_tenant`；启用 RLS 的表将被强隔离。

### 5.2 租户上下文来源（契约）
所有访问 RLS 表的路径必须满足：
- `composables.UseTenantID(ctx)` 可用（来源于 session/user 或 domain 解析）。
- 在 `BEGIN` 后第一条语句完成 `set_config('app.current_tenant', tenantID, true)`。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 注入点设计
PoC 要求覆盖两类事务入口：
1. `pkg/composables/db_composables.go:54` 的 `InTx`（服务层常用）
2. `pkg/middleware/db_middleware.go:11` 的 `WithTransaction`（虽标注 deprecated，但仍存在调用点）

推荐实现一个统一 helper（示意接口）：
```go
// pkg/routing/rls or pkg/repo/rls（以仓库约定为准）
func ApplyTenantRLS(ctx context.Context, tx pgx.Tx) error
```

实现策略（伪代码）：
```go
func ApplyTenantRLS(ctx context.Context, tx pgx.Tx) error {
  if os.Getenv("RLS_ENFORCE") != "enforce" {
    return nil
  }
  tenantID, err := composables.UseTenantID(ctx)
  if err != nil {
    return err // fail-closed
  }
  _, err = tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", tenantID.String())
  return err
}
```

### 6.2 非事务访问防护（fail-fast）
当 `RLS_ENFORCE=enforce` 且访问了启用 RLS 的表时：
- 必须在显式事务中完成注入；否则 Postgres 会 fail-closed（报错/无数据），容易在开发期表现为“数据凭空消失/偶发报错”。
- 为避免“隐式失败”，建议在访问 RLS 表的 repo/service 层提供明确的 guard：
  - `RequireTx(ctx)`：若 ctx 中不存在 `pgx.Tx`，直接返回可读错误（提示使用 `composables.InTx` 包裹）。
  - `InTenantTx(ctx, fn)`：对外提供统一入口，内部完成 `BEGIN + ApplyTenantRLS + fn + COMMIT/ROLLBACK`。

### 6.3 失败模式与错误处理
- 缺失 tenant：注入失败，返回应用错误（建议使用 `pkg/serrors` 包装为可观测错误）。
- DB 返回 `unrecognized configuration parameter`：表示未创建 GUC（Postgres 允许自定义 `app.*`，通常不会发生）；需记录为配置错误。
- DB 返回 `invalid input syntax for type uuid`：表示注入值不合法，属于严重 bug（应触发告警）。

## 7. 安全与鉴权 (Security & Authz)
- RLS 只解决“数据隔离”，不替代 Casbin（业务授权仍必须在应用层完成）。
- superadmin/跨租户操作必须使用明确的旁路机制（例如专用连接账号 + 审计），不得通过普通租户链路隐式绕过。
- **系统组件与 RLS**：
  - Outbox Relay / 后台 Job 的“跨租户扫描”属于系统能力，不应通过放宽 RLS policy 来绕过隔离。
  - PoC 阶段明确：仅对业务表（如 `employees`）启用 RLS；系统队列表不启用。

## 8. 依赖与里程碑 (Dependencies & Milestones)
1. [ ] 选择 PoC 表：`employees`（已定）。
2. [ ] 引入 `RLS_ENFORCE` 开关与统一注入 helper，并在事务入口处调用。
3. [ ] 本地/CI 通过非 superuser 账号验证 RLS 生效。
4. [ ] 在 migrations 中为 `employees` 启用 RLS + policy（或在 PoC 环境先手工执行并记录）。
5. [ ] 扩展到第二张表（可选），验证模板可复制。

## 9. 测试与验收标准 (Acceptance Criteria)
- **行为**：
  - [ ] tenant=A 的上下文下，读取 tenant=B 的行必须失败（无数据或报错）。
  - [ ] 未设置 `app.current_tenant` 时，对启用 RLS 的表进行查询必须 fail-closed（按 policy 选择“报错”口径）。
  - [ ] 启用 RLS 的业务表不影响系统组件（例如 outbox relay 仍可跨租户扫描其队列表）。
- **工程**：
  - [ ] `go fmt ./... && go vet ./... && make check lint` 通过。
  - [ ] 若涉及 migrations：按 AGENTS 触发器执行 `make db migrate up && make db seed` 并记录结果。

## 10. 运维与监控 (Ops & Monitoring)
### 10.1 日志字段（建议）
- `tenant_id`, `request_id`, `rls_enforce`, `sqlstate`, `error`

### 10.2 回滚路径（PoC）
- DB 侧：
  - `ALTER TABLE employees DISABLE ROW LEVEL SECURITY;`
  - 或 `DROP POLICY tenant_isolation ON employees;`
- 应用侧：
  - 将 `RLS_ENFORCE=disabled`（注入停用，回退到现有应用层过滤模式）
