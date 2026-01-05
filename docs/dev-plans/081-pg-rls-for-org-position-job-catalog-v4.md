# DEV-PLAN-081：启用 PostgreSQL RLS 强租户隔离（Org/Position/Job Catalog v4）

**状态**: 草拟中（2026-01-05 02:45 UTC）

> 本计划聚焦：在 `DEV-PLAN-077/079/080` 的 v4 Kernel 表上 **默认启用 PostgreSQL RLS**，把“租户隔离”从应用层 `WHERE tenant_id=...` 的约定，升级为 DB 层可验证的强制约束；并与 `DEV-PLAN-019/019A` 的 RLS 注入与 DB 角色契约对齐，确保 Greenfield v4 默认强隔离，不依赖实现期临时补丁。

## 1. 背景与上下文 (Context)
- `DEV-PLAN-077/079/080` 将大量不变量与核心计算下沉到 PostgreSQL（advisory lock / GiST exclusion / ltree / jsonb 校验 / 同事务 replay），DB 已被选定为“Kernel/最终裁判”。
- v4 表普遍以 `(tenant_id, <id>)` 复合唯一/外键对齐多租户，但隔离仍主要依赖应用层显式追加 `WHERE tenant_id = ...`，存在“漏写/误 join”导致跨租户泄露的系统性风险。
- `DEV-PLAN-019/019A` 已确立 RLS PoC 的系统级契约：**事务内**注入 `app.current_tenant`（`set_config(..., true)`/`SET LOCAL`）+ policy 使用 `current_setting('app.current_tenant')::uuid` **fail-closed**；并已落地 `pkg/composables/rls.go:ApplyTenantRLS`。
- 需要一个面向 v4 的“启用清单 + 迁移落盘 + Go 调用约束 + 验收/回滚”的方案，作为 v4 系列（077/079/080）的默认启用策略与后续扩展模板。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 对 `DEV-PLAN-077/079/080` 中所有 **tenant-scoped v4 表**启用 RLS（`ENABLE` + `FORCE` + policy），且 policy 为 **fail-closed**。
- [ ] Go 层所有访问 v4 表的路径在事务内调用 `composables.ApplyTenantRLS`；读路径同样要求显式事务（遵循 019 的 “No Tx, No RLS” 契约）。
- [ ] 为 v4 Kernel 的写入口（`submit_*_event`）增加“tenant 一致性断言”（`p_tenant_id` 必须等于 `app.current_tenant`），降低“串租户/误传参”导致的隐性数据污染。
- [ ] 提供可验证、可渐进推广（按表/按模块）的实施步骤、验收标准与回滚路径。

### 2.2 非目标（明确不做）
- 不在本计划内对系统队列表（如 outbox/relay/claim 相关表）启用 RLS；边界与理由以 `DEV-PLAN-019A` 为准。
- 不在本计划内实现 superadmin 跨租户旁路查询/报表；如需旁路必须使用专用 DB role/连接池 + 审计，另起计划。
- 不在本计划内移除应用层 `WHERE tenant_id=...`（RLS 作为兜底 + 最终裁判；迁移期保持双保险）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划仅声明命中项与 SSOT 链接，不复制命令清单。

- **触发器（实施阶段将命中）**：
  - [ ] DB 迁移 / Schema（Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`；以及 `AGENTS.md` 的触发器矩阵）
  - [ ] Go 代码（`AGENTS.md`）
- **SSOT 链接**：
  - 多租户工具链与 RLS 契约：`docs/dev-plans/019-multi-tenant-toolchain.md`、`docs/dev-plans/019A-rls-tenant-isolation.md`
  - Org/Position/Job Catalog v4：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`
  - Greenfield HR 模块骨架（schema SSOT 目录约定）：`docs/dev-plans/083-greenfield-hr-modules-skeleton.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.0 主流程（可解释性 / Mermaid）
```mermaid
flowchart TD
  Req[HTTP Request / Job] --> Ctx[ctx: tenant_id]
  Ctx --> Tx[BEGIN Tx]
  Tx --> Set[ApplyTenantRLS\nset_config(app.current_tenant, tenant_id, true)]
  Set --> Q[SQL / submit_*_event]
  Q --> PG[(PostgreSQL)]
  PG --> RLS[RLS policy tenant_isolation]
  RLS --> OK[Same-tenant rows]
  RLS --> Deny[No rows / ERROR]
```

### 3.1 为什么 v4 优先启用 RLS（对 077-080 的分析）
- v4 的写入/重放/校验大量依赖 DB 内函数与 SQL 片段，SQL 面积增大；仅靠应用侧 `WHERE tenant_id` 的“代码审查习惯”难以形成系统性兜底。
- v4 数据模型已一致收敛到 `tenant_id`：事件表/versions/identity/关系表均显式携带并参与约束；RLS 可以用一条统一 policy 覆盖大部分表，落地成本可控。
- 本系列按 Greenfield 口径实施，因此可以从 v4 表一开始就默认启用 RLS：无需对旧表做渐进兼容。

### 3.2 RLS 注入策略（沿用 019/019A）
- **选定**：RLS policy 仅信任事务内注入的 `app.current_tenant`（`SELECT set_config('app.current_tenant', $1, true)` 或 `SET LOCAL app.current_tenant = ...`）。
- **选定（fail-closed）**：policy 使用 `current_setting('app.current_tenant')::uuid`（不使用 `current_setting(..., true)`），缺失上下文时直接报错暴露注入遗漏。
- **约束**：应用 DB role 不能是 superuser 且不能带 `BYPASSRLS`（否则 RLS 不生效/不可验证）；对齐 `DEV-PLAN-019/019A`。

### 3.3 v4 Kernel 写入口的 tenant 一致性断言（新增决策）
> 目的：避免“事务注入的 tenant ≠ 函数参数 tenant”时产生隐性污染；同时保持 v4 函数签名不变（继续显式传 `p_tenant_id`，与 077/079/080 对齐）。

- **选定**：在 DB 中提供稳定 helper：
  - `current_tenant_id()`：返回 `current_setting('app.current_tenant')::uuid`（fail-closed）。
  - `assert_current_tenant(p_tenant_id uuid)`：若 `p_tenant_id <> current_tenant_id()` 则 `RAISE EXCEPTION`（稳定错误码与错误信息，便于 Go 映射到 `pkg/serrors`）。
- **落点**：所有 `submit_*_event`（以及维护入口 replay/rebuild，如需）在函数开头调用 `assert_current_tenant(p_tenant_id)`。

## 4. 数据模型与 RLS 落盘清单 (Data Model & RLS Inventory)
### 4.1 统一 DDL 模板（示意）
```sql
-- helper（建议放在 v4 schema 的通用段落，便于复用）
CREATE OR REPLACE FUNCTION current_tenant_id()
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
  SELECT current_setting('app.current_tenant')::uuid;
$$;

CREATE OR REPLACE FUNCTION assert_current_tenant(p_tenant_id uuid)
RETURNS void
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
  v_current uuid;
BEGIN
  v_current := current_tenant_id(); -- fail-closed
  IF p_tenant_id <> v_current THEN
    RAISE EXCEPTION USING
      MESSAGE = 'RLS_TENANT_MISMATCH',
      DETAIL = format('p_tenant_id=%s current_tenant=%s', p_tenant_id, v_current);
  END IF;
END;
$$;

-- 表级 policy（每张表同构）
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON <table>;
CREATE POLICY tenant_isolation ON <table>
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
```

### 4.2 OrgUnit v4（DEV-PLAN-077）
- `org_trees`
- `org_events`
- `org_unit_versions`

### 4.3 Position/Assignment v4（DEV-PLAN-079）
- `positions` / `position_events` / `position_versions`
- `assignments` / `assignment_events` / `assignment_versions`

### 4.4 Job Catalog v4（DEV-PLAN-080）
- Identity：`job_family_groups` / `job_families` / `job_levels` / `job_profiles`
- Events：`job_family_group_events` / `job_family_events` / `job_level_events` / `job_profile_events`
- Versions：`job_family_group_versions` / `job_family_versions` / `job_level_versions` / `job_profile_versions`
- Relation：`job_profile_version_job_families`

## 5. 接口契约 (API Contracts)
### 5.1 Go 事务契约（No Tx, No RLS）
- 当 `RLS_ENFORCE=enforce` 时：
  - 所有访问启用 RLS 的 v4 表的路径必须在显式事务中执行；
  - 在该事务内第一时间调用 `composables.ApplyTenantRLS(ctx, tx)`，再执行任何 SQL（含调用 DB Kernel 函数）。
- 当 `RLS_ENFORCE=disabled` 时：
  - 允许旧模块继续使用非事务读（仅限未启用 RLS 的表）；
  - 但对 v4（默认启用 RLS）不提供“无事务访问”的兼容口径（避免引入“偶发无数据/偶发报错”的漂移）。

### 5.2 DB Kernel 契约
- 所有 v4 `submit_*_event` 必须在 `app.current_tenant` 已注入的事务内执行，且 `p_tenant_id` 必须与之相等（由 `assert_current_tenant` 强制）。

### 5.3 开关与组合（避免脚枪）
> 目标：把“RLS_ENFORCE=disabled 但 DB 已启用 RLS”这类隐性配置错误显式化，避免实现期靠试错收敛。

| DB：v4 表启用 RLS | 应用：`RLS_ENFORCE` | 结果（典型表现） | 允许 |
| --- | --- | --- | --- |
| enabled | enforce | ✅ 正常（强隔离生效） | ✅ |
| enabled | disabled | ❌ fail-closed（报错/无数据） | ❌ |
| disabled | enforce | ✅ 正常（仅多一次 `set_config`） | ✅ |
| disabled | disabled | ✅ 旧模式（仅应用层过滤） | ✅ |

**本计划的选定落点**：v4 表将默认启用 RLS（DB=enabled），因此凡运行时会访问 v4 表，则 `RLS_ENFORCE` 必须为 `enforce`（否则视为配置错误）。

### 5.4 错误契约（DB → Go → serrors）
> 对齐 `DEV-PLAN-077` 的口径：Go 侧优先用 `SQLSTATE`（必要时结合 constraint name）与稳定 `MESSAGE` 做映射，避免依赖自然语言字符串匹配。

- **缺失 tenant 上下文（fail-closed）**：
  - 触发点：`current_setting('app.current_tenant')`（或 `current_tenant_id()`）。
  - 识别建议：用 `SQLSTATE` 识别并收敛为稳定错误码（例如 `RLS_TENANT_CONTEXT_MISSING`），并记录 `rls_setting_key=app.current_tenant`。
- **tenant 参数与上下文不一致**：
  - 触发点：`assert_current_tenant(p_tenant_id)`。
  - 识别建议：解析 DB exception 的 `MESSAGE='RLS_TENANT_MISMATCH'`，动态信息放在 `DETAIL`。
- **跨租户写入（RLS policy 拒绝）**：
  - 触发点：`WITH CHECK` 失败（或 `USING` 导致 UPDATE/DELETE 无效）。
  - 识别建议：按 `SQLSTATE` 收敛为 `RLS_VIOLATION`（并在日志中输出 `tenant_id/request_id/sqlstate`）。

## 6. 实施步骤（建议顺序）
1. [ ] 在 v4 schema SSOT 中引入 `current_tenant_id()` 与 `assert_current_tenant(...)`，并将写入口函数接入断言。
2. [ ] 按 4.2-4.4 清单为 v4 表添加 RLS DDL（`ENABLE/FORCE` + `tenant_isolation` policy），并确保最终 SSOT 落在对应模块的 schema SSOT 目录：`modules/orgunit/infrastructure/persistence/schema/`、`modules/jobcatalog/infrastructure/persistence/schema/`、`modules/staffing/infrastructure/persistence/schema/`（对齐 `DEV-PLAN-083`）。
3. [ ] Go 层：在 v4 写路径/读路径统一走事务入口，并在事务内调用 `composables.ApplyTenantRLS`（对齐 019A 的注入契约）。
4. [ ] 补齐测试（至少一条 DB/集成测试）：验证缺失 `app.current_tenant` 时 fail-closed；跨租户读写被拒绝；同租户正常。
5. [ ] 本地验证与门禁对齐（命中项按 `AGENTS.md` 触发器矩阵执行），确保 `RLS_ENFORCE=enforce` + 非 superuser 且 `NOBYPASSRLS` 账号下稳定通过。

## 7. 风险与缓解
- **性能**：RLS 会引入额外谓词与 plan 变化；缓解：policy 只做 `tenant_id = current_tenant_id()` 的等值过滤，且 v4 关键索引均以 `tenant_id` 为前导；实现期以 `EXPLAIN (ANALYZE, BUFFERS)` 复核热点查询。
- **开发体验**：遗漏事务/注入会表现为“报错或无数据”；缓解：在 repo/service 层提供明确的 `InTenantTx`/`RequireTx` 约束与可读错误（对齐 019）。
- **系统组件**：outbox relay/后台 job 可能需要跨租户扫描；缓解：按 019A 边界，系统表不启用 RLS；若未来必须启用，使用专用 role/连接池，禁止“缺 tenant 放行”的 policy。

## 8. 测试与验收标准 (Acceptance Criteria)
- [ ] **fail-closed**：未设置 `app.current_tenant` 时，访问启用 RLS 的 v4 表必须报错（不可“默认放行”）。
- [ ] **隔离**：在 tenant=A 的上下文下，读取/写入 tenant=B 的行必须被拒绝（无数据或报错）。
- [ ] **一致性断言**：`p_tenant_id` 与 `app.current_tenant` 不一致时，v4 写入口必须以稳定错误码失败。
- [ ] **回归**：在 `RLS_ENFORCE=enforce` 模式下，v4 关键路径（写入 + 快照读取 + replay 维护入口）稳定可用。

### 8.1 最小验证点（避免“跑一跑就算”）
- [ ] **行为**：用同一套 SQL 分别验证（1）未注入 tenant 时 fail-closed；（2）tenant=A 下不可见 tenant=B；（3）跨租户 insert/update 被拒绝。
- [ ] **计划**：对各子域最常用的 as-of 快照查询做一次 `EXPLAIN (ANALYZE, BUFFERS)`，确认仍使用以 `tenant_id` 为前导的索引（RLS 作为兜底，不替代显式 `tenant_id` 过滤）。
- [ ] **可观测性**：RLS 相关失败在应用日志中至少包含：`tenant_id`, `request_id`, `rls_enforce`, `sqlstate`, `error`（对齐 019A 的建议字段）。

## 9. 回滚路径（最小可行）
- DB：对 v4 表执行 `ALTER TABLE <table> DISABLE ROW LEVEL SECURITY;`（必要时同时 `DROP POLICY tenant_isolation ON <table>;`）。
- 应用：仅当 DB 侧已回滚（v4 表 RLS=disabled）时，才允许设置 `RLS_ENFORCE=disabled`；否则 v4 访问将 fail-closed（属于配置错误，而非“正常降级”）。
