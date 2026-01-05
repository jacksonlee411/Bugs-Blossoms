# DEV-PLAN-091：V4 sqlc 工具链使用指引与规范（SQL-first + DB Kernel + RLS）

**状态**: 草拟中（2026-01-05 08:48 UTC）

> 适用范围：**全新实现的 V4 新代码仓库（Greenfield）**，并将按 `DEV-PLAN-077/079/080/081` 的 DB Kernel 范式实现。  
> 上游依赖：`DEV-PLAN-087`（工具链版本冻结）、`DEV-PLAN-082`（DDD 分层框架）、`DEV-PLAN-083`（模块骨架）、`DEV-PLAN-081`（RLS 注入契约）。  
> 目标：把 sqlc 从“生成工具”收敛为一套**可审查、可验证、可复用**的工程规范，避免 077+ 体系在实现期因 SQL 漂移而退化为“容易（Easy）”的补丁堆叠。

## 1. 结论先行（必须遵守的最小规则）

1. **SQL-first**：只要是持久化读写，优先用 SQL + sqlc（拒绝 ORM 逃逸口）。
2. **One Door**（对齐 077-080）：写入必须走 DB Kernel 的 `submit_*_event(...)`（或等价唯一入口）；应用层禁止直写 events/versions/identity 表。
3. **No Tx, No RLS**（对齐 081）：凡访问 v4 tenant-scoped 表，必须显式事务，并在事务内注入 `app.current_tenant`（fail-closed）。
4. **生成物必须提交**：sqlc 生成的 Go 代码属于门禁产物，必须随 PR 提交；CI 只接受“生成物与源码一致”。
5. **模块边界优先**：sqlc 查询/包按模块拆分，禁止跨模块 import 对方的 `infrastructure/**` 生成包；跨模块读写通过 DB Kernel 函数/视图或在本模块内封装 adapter。

## 2. 背景：为何 V4 仍要用 sqlc（结合 077+）

`DEV-PLAN-077/079/080` 选定了 **DB Kernel（Postgres）作为不变量与投射的最终裁判**：advisory lock、GiST exclusion、ltree、jsonb 校验、同事务 replay 等都让 SQL 成为“领域合同”的主要载体。此时：
- 如果继续用手写分散 SQL，reviewer 很难判断“权威表达是否唯一”“是否绕过 One Door”“是否遗漏 RLS 注入”。
- 如果引入 ORM，SQL 会被隐藏在运行期拼装里，反而削弱 077+ 追求的可解释性与可验证性。

因此 V4 的 sqlc 目标不是“少写 Go”，而是：
- 让 SQL 成为显式资产（可审查/可测试/可版本化），
- 让生成的 Go 调用形状稳定一致（编译期类型安全），
- 让“门禁与生成物”成为强约束，阻止实现期漂移。

## 3. 范围与非目标

### 3.1 范围（In Scope）
- V4 业务域模块：`orgunit` / `jobcatalog` / `staffing` / `person`（对齐 `DEV-PLAN-083`）。
- 平台模块：如 `iam`（若存在；对齐 `DEV-PLAN-088/088A` 的思路），同样遵循本文规范。
- sqlc 配置（`sqlc.yaml`）、schema 输入、查询文件组织、生成目录、门禁触发器与验收标准。

### 3.2 非目标（Out of Scope）
- 不在本文决定 schema 的具体字段；schema SSOT 仍在各模块 `infrastructure/persistence/schema/**`（对齐 `DEV-PLAN-082` 的形态 B）。
- 不在本文决定 Atlas/Goose 迁移细节；仅规定 sqlc 如何依赖 schema SSOT（引用 `DEV-PLAN-087` 的工具链口径）。

## 4. 工具链版本与入口（SSOT）

- sqlc 版本：以 `DEV-PLAN-087` 冻结清单为准，且 **Makefile 为唯一命令入口**。
- 生成入口（约束）：必须提供 `make sqlc-generate`，并在 CI 中对命中路径启用“生成一致性检查”（见 §9）。
- 生成后必须 `git status --short` 为空（无遗漏生成物）。

## 5. 目录结构与归属（对齐 082/083）

### 5.1 约定的目录（每个模块一致）
在 V4 新仓库中建议采用统一形状（示意）：
```
modules/<module>/
  infrastructure/
    persistence/
      schema/                 # DB Kernel SSOT（DDL/函数/约束/视图）
    sqlc/
      queries/                # sqlc 查询（仅 SQL）
      schema.sql              # sqlc 编译用 schema（由脚本导出；见 §6）
      gen/                    # sqlc 生成物（Go 包；必须提交）
```

说明：
- `schema/` 是 **权威契约**（Kernel），进入 DB 迁移/plan/lint 的 SSOT。
- `schema.sql` 是 **sqlc 编译输入**：允许由脚本导出（为了确定性与跨模块依赖可解析），但其来源必须可追溯到 `schema/` + 迁移工具链。
- `gen/` 仅存生成物；禁止手改；禁止放业务逻辑。

### 5.2 CleanArchGuard 约束
- sqlc 生成包必须位于 `modules/<module>/infrastructure/**`，不得被 `domain/` 或 `services/` 直接依赖。
- `services/` 只依赖本模块的端口接口（Repository/Kernel Port），不依赖具体 sqlc 生成包。

## 6. schema 输入策略（解决“sqlc 看见的 schema 与真实 DB 不一致”）

V4 需要同时满足：
- 077+ 的 schema/函数/约束是合同（SSOT），
- sqlc 需要“可解析的 schema”做静态分析（含跨模块 FK/类型）。

### 6.0 ADR：选定“全量可解析 schema.sql”作为 sqlc 输入（V4）
**结论（选定）**：V4 采用 **全量可解析 schema** 作为 sqlc 输入，并纳入门禁；不采用“每模块自维护最小 schema 子集”的方式。

原因：
- 077/079/080 的 v4 方案存在跨模块引用与组合（例如 staffing ↔ jobcatalog 的 identity，orgunit 的 as-of 校验），若 schema 输入按模块裁剪，容易引入“缺表/缺类型/缺函数”的隐式依赖清单，最终靠试错补齐（违背 045 的确定性要求）。
- 全量 schema 输入可把“依赖边界”从“sqlc 能否解析”解耦出来：边界由模块分层/ports/one-door 决定，而不是由 schema 文件裁剪决定。

回滚/演化：
- 若未来必须拆分（例如 schema 体量过大导致 sqlc 性能问题），必须另开子计划（例如 091A）并给出：依赖清单生成方式、回滚策略、以及 CI 门禁如何保持确定性。

### 6.1 导出策略（建议落地为脚本）
- 提供脚本（示意名）：`scripts/db/export_v4_schema.sh`
  - 在干净数据库中应用 schema SSOT（Atlas/Goose，入口以 Makefile 为准）。
  - 导出 schema-only 到 **全局** `internal/sqlc/schema.sql`（单一事实源），再由 `sqlc.yaml` 引用。
- 导出文件必须可复现：同一份 schema SSOT，导出结果应稳定（避免排序漂移）。

### 6.2 选择“每模块 schema.sql”还是“全局 schema.sql”
本计划已在 6.0 选定：**全局 schema.sql**。

## 7. 查询组织规范（SQL 文件即合同）

### 7.1 文件分层（强制）
在 `modules/<module>/infrastructure/sqlc/queries/` 下，按语义分文件（示意）：
- `kernel.sql`：仅包含对 DB Kernel 入口的调用（`submit_*_event` / `replay_*` / `get_*_snapshot` 等函数/视图调用）。
- `queries.sql`：读模型查询（版本表、快照函数、列表/搜索）。
- （可选）`ops.sql`：运维/管理查询（只能在 superadmin 或内部 job 边界使用；必须明确旁路策略与审计，见 §8.3 / §12）。

### 7.2 命名规范（强制）
- sqlc `-- name:` 必须采用 `VerbObject`：`Get*` / `List*` / `Search*` / `Submit*Event` / `Replay*`。
- 同一语义的查询只能有一个权威名字；禁止 `Query1/Query2`。
- 返回类型选择：
  - `:one` 仅当逻辑上保证 1 行（如 `no-overlap` 的 as-of 版本点查）。
  - `:many` 必须配合稳定排序与分页策略（不要“默认顺序”）。
  - `:exec` 仅用于不返回行的语句；Kernel 入口应设计为返回稳定结果（例如 `event_row_id`），避免 sqlc 无法表达。

### 7.3 复杂 SQL 的“升格”规则（避免 query 文件膨胀）
当某条查询满足任一条件，必须从 sqlc query 文件“升格”为 schema SSOT 中的 STABLE SQL 函数/视图：
- 被多个控制器/服务重复使用；
- SQL 本身包含复杂 join/聚合/递归/ltree 操作；
- 需要与 RLS/有效期不变量强绑定（避免不同查询写出不同口径）。

sqlc 侧只负责调用该函数/视图（提高一致性、降低漂移）。

## 8. 多租户与 RLS（与 sqlc 的组合口径）

### 8.1 No Tx, No RLS（必须）
对所有 v4 tenant-scoped 表（077/079/080 定义的 events/versions/identity/关系表）：
- **必须**在事务内执行 sqlc 查询；
- **必须**在事务开始后首先注入 RLS tenant：`set_config('app.current_tenant', ...)`（语义对齐 `DEV-PLAN-081`）。

### 8.2 双保险策略（推荐）
为降低实现期“漏注入/漏过滤”风险，V4 推荐同时满足：
- DB 层：RLS policy fail-closed；
- SQL 层：查询仍显式包含 `tenant_id = $1`（尤其在 join/子查询中），作为可读性与审查锚点。

例外必须显式声明（并写测试）：
- 仅按高熵 token 精确查询的表（例如 session 表），且表本身不启用 RLS；
- superadmin bypass（必须走专用连接池/role，并写审计；不允许放宽 RLS policy）。

### 8.3 `ops.sql` 边界（防止“运维查询”污染业务面）
- `ops.sql` 只能被以下两类调用方使用：
  1) superadmin server（控制面），且使用 bypass 连接池/role；
  2) 内部 job（运维/重建），且具备显式开关与审计记录。
- tenant app（业务面）禁止引用/执行 `ops.sql` 的任何查询。
- 任何 `ops.sql` 涉及跨租户读写，必须把 `target_tenant_id` 作为显式参数（不得靠缺省/“全表扫”）。

## 9. sqlc 配置规范（`sqlc.yaml`）

### 9.1 组织策略
- V4 建议使用 **单一 `sqlc.yaml`**（根目录），在 `sql:` 下按模块输出多个 `gen.go` 包。
- 每个包只包含本模块的 queries（路径隔离），但 schema 输入可共享（见 §6）。

### 9.2 必选配置（推荐基线）
以 `DEV-PLAN-087` 为版本事实源，但建议启用以下稳定性选项：
- `sql_package: pgx/v5`
- `emit_prepared_queries: true`（性能与一致性；若造成 DX 问题，必须以 ADR 形式说明）
- `emit_json_tags: true`
- `emit_pointers_for_null_types: true`
- `emit_empty_slices: true`

类型覆写（按需，但必须集中在 `sqlc.yaml`）：
- `uuid` → `github.com/google/uuid.UUID`
- `jsonb` → `encoding/json.RawMessage`
- `ltree` → `string`（或 `pkg/ltree.Path`，但必须提供清晰边界与测试）
- `daterange` → `pgtype.Range[...]`（或自定义 `pkg/daterange`，但必须有清晰的 `[start,end)` 语义单测）

## 10. 测试与覆盖率（对齐 V4 100% 门禁）

若新仓库启用“100% 覆盖率门禁”，必须明确覆盖率统计范围：
- **推荐**：覆盖率门禁排除生成物（sqlc/templ/mock 等），100% 仅对手写代码生效；排除规则与审计方式必须在新仓库 SSOT 中固定（对齐 `DEV-PLAN-000` 的“覆盖率口径/范围/证据记录”要求）。
- 生成物仍需被编译与集成测试覆盖其关键调用路径（例如 repository 层对 sqlc 的调用）。

最小测试集（必须具备）：
- Kernel 入口调用的集成测试：确保 One Door 未被绕过、错误码映射稳定。
- RLS fail-closed 测试：缺 tenant 注入必须失败；跨租户不可见（对齐 `DEV-PLAN-081`）。
- 关键 as-of 查询测试：`validity @> date` 的点查必须稳定返回 1 行（no-overlap 合同）。

## 11. 门禁与工作流（CI/本地）

### 11.1 触发器（建议 CI 过滤器）
当改动命中任一项时必须运行 sqlc 生成并确保工作区干净：
- `sqlc.yaml`
- `modules/**/infrastructure/sqlc/**`
- `modules/**/infrastructure/persistence/schema/**`（或 schema SSOT 等效路径）
- `scripts/db/export_*schema*.sh`（若存在）

### 11.2 开发者本地流程（必须）
1. 修改 schema SSOT（Atlas/Goose 入口按 Makefile）。
2. 运行 schema 导出脚本（见 §6）。
3. 运行 `make sqlc-generate`（或 `make generate` 若其包含 sqlc）。
4. `git status --short` 必须为空。

## 12. 停止线（命中即打回，按 DEV-PLAN-045）
- 绕过 One Door：应用层出现对 v4 events/versions/identity 表的直接 DML（除运维 replay 且有明确边界）。
- 绕过 RLS：访问 tenant-scoped 表但没有事务与 tenant 注入；或为了“方便”把 policy 改成可缺省 tenant。
- 手改生成物：对 `modules/**/infrastructure/sqlc/**/gen/**` 的手工编辑。
- 生成物未提交：PR 命中触发器但 `git status` 非空或生成物与源码不一致。
- 跨模块依赖生成包：`modules/A` 直接 import `modules/B/infrastructure/sqlc/gen`。
- `ops.sql` 逃逸：tenant app 直接调用 `ops.sql`，或 ops 查询未使用 bypass 连接池/role，或缺少审计记录。

## 13. 验收标准（本计划完成定义）
- [ ] schema 输入已选定为 `internal/sqlc/schema.sql`，并由脚本从 schema SSOT 可复现导出（同一 SSOT 导出无 diff）。
- [ ] `sqlc.yaml` 以多包方式按模块输出生成物，但引用同一份全局 schema 输入（不允许按模块“随手裁剪 schema”）。
- [ ] 命中触发器时，CI 必须执行 `make sqlc-generate` 并强制 `git status --short` 为空（生成物一致性门禁）。
- [ ] 所有 v4 tenant-scoped 查询在事务内执行且注入 RLS（缺 tenant 注入时 fail-closed 有测试覆盖）。
- [ ] `ops.sql` 仅能在 superadmin/job 边界使用，且所有跨租户写操作具备审计记录；tenant app 无法调用 ops 查询（通过静态依赖与测试双重保证）。
