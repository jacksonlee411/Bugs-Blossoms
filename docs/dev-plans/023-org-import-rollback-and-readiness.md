# DEV-PLAN-023：Org 导入/回滚工具与 Readiness

**状态**: 已评审（2025-12-17 11:44 UTC）— 按 `docs/dev-plans/001-technical-design-template.md` 补齐可编码契约

## 0. 进度速记
- ✅ 交付形态：Go CLI `cmd/org-data`（默认 dry-run）。
- ✅ MVP 目标：`--backend db` 的 **seed** 导入/导出/回滚 + manifest + readiness。
- ⏳ 后续增强：`--backend api`（复用 `/org/api/batch`）与 **merge** 导入，待 [DEV-PLAN-026](026-org-api-authz-and-events.md) 就绪后再补，不纳入本计划的 readiness 门槛。

## 1. 背景与上下文 (Context)
- 对应 [DEV-PLAN-020](020-organization-lifecycle.md) 步骤 3，承接：
  - [DEV-PLAN-021](021-org-schema-and-constraints.md)：Org 核心表/约束（导入写入目标）
  - [DEV-PLAN-022](022-org-placeholders-and-event-contracts.md)：事件契约（DB backend 不触发；API backend 未来复用）
- 目标是在 024 CRUD 主链上线前，提供“可重复执行、可校验、可回滚”的数据导入工具与 readiness 记录，降低数据污染与演练成本。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 提供 Go CLI `org-data`（`cmd/org-data`）：`import/export/rollback` 三个子命令。
- [ ] `import`：
  - [ ] 默认 dry-run（不落库），输出“影响摘要 + 校验错误（含行号/字段）”。
  - [ ] 显式 `--apply` 才允许写入；写入成功后生成 `import_manifest_*.json`（用于精确回滚）。
  - [ ] MVP 仅支持 `--backend db` + `--mode seed`（空租户种子导入）。
- [ ] `export`：从 DB 导出 CSV（支持 `--as-of` 导出快照；不带 `--as-of` 导出全量时间片）。
- [ ] `rollback`：支持 `--manifest` 精确回滚（推荐）；支持 `--since` 作为 seed 的兜底回滚（带强安全网）。
- [ ] Readiness：至少通过 `go fmt ./... && go vet ./... && make check lint && make test`，并将命令与输出记录到 `docs/dev-records/DEV-PLAN-023-READINESS.md`。

### 2.2 非目标（本计划明确不做）
- 不实现 Org UI、审批流、矩阵/继承逻辑执行（见 035/030 等）。
- 不实现生产级自动化灰度/调度；本工具仅提供最小可用的可重复执行路径。
- 不纳入本计划 M1 交付/Readiness：`--backend api` / `--mode merge`（待 026 的 `/org/api/batch` 就绪后，另行评审并更新本计划或拆分子计划）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  U[Dev/Ops] -->|CSV 目录| CLI[org-data CLI]
  CLI --> P[Parse & Normalize]
  P --> V[Validate]
  V -->|dry-run| OUT[Summary(JSON)+ExitCode]
  V -->|--apply| B{Backend}
  B -->|db (MVP)| DB[(Postgres)]
  B -->|api (post-026)| API[HTTP /org/api/batch]
  DB --> M[import_manifest_*.json]
  API --> M
```

### 3.2 关键设计决策
1. **Go CLI（选定）**：CSV 解析、时间片校验、环路检测等逻辑不适合 Shell；Go 可复用项目现有配置/DB/RLS 工具链。
2. **默认 dry-run（选定）**：任何写入必须显式 `--apply`；任何删除必须显式 `--apply` 且 `--yes` 二次确认（避免误操作）。
3. **MVP 限定 seed（选定）**：`--backend db` 只允许“空租户种子导入”，避免绕过 025/026 的审计/outbox/缓存导致线上不一致；需要 merge 时走未来的 API backend。
4. **RLS 注入复用现有 helper（选定）**：
   - 在事务内使用 `pkg/composables.WithTenantID` 注入 tenant context；
   - 在 `BEGIN` 后第一条 SQL 前调用 `pkg/composables.ApplyTenantRLS(ctx, tx)`（对齐 019A）。
5. **`subject_id` 映射 SSOT（选定）**：`org_assignments.subject_id` 的确定性映射以 [DEV-PLAN-026](026-org-api-authz-and-events.md) 的“Subject 标识与确定性映射（SSOT）”为唯一事实源；CLI 不允许自行实现漂移算法。

## 4. 数据契约 (Data Contracts)
> 本节定义 CSV/manifest 的合同；DB schema 合同以 [DEV-PLAN-021](021-org-schema-and-constraints.md) 为准。

### 4.1 输入目录结构（`import --input <dir>`）
- 必选：`nodes.csv`
- 可选：`positions.csv`、`assignments.csv`
- 约定：缺失的可选文件视为“空集”；存在但 Header 缺列/多列则报错（避免静默错位）。

### 4.2 CSV 通用约定
- 编码：UTF-8（允许 BOM；工具需剥离）。
- 分隔与引用：RFC4180（逗号分隔、双引号转义），Header 必须存在。
- Trim 规则：对所有 `code/pernr/email` 等标识字段执行 `strings.TrimSpace`；Trim 后为空视为缺失。
- 时间字段：
  - 输入：`YYYY-MM-DD` 或 RFC3339。
  - 规范化：`YYYY-MM-DD` 解释为 `00:00:00Z`；所有时间按 UTC 写入。
  - 语义：半开区间 `[effective_date, end_date)`；要求 `effective_date < end_date`。
- `end_date` 自动补齐：
  - 若行内提供 `end_date`：按输入写入。
  - 若行内未提供 `end_date`：按“同一实体的下一片段 `effective_date`，否则 `9999-12-31T00:00:00Z`”补齐。
- JSON 字段：必须为合法 JSON；写入 DB 时不做额外键校验（M1 保守），但必须是 object。

### 4.3 `nodes.csv`（映射到 `org_nodes` + `org_node_slices` + `org_edges`）
| 列 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `code` | string | 是 |  | 租户内唯一；用于建立 `code -> org_node_id` 映射 |
| `type` | string | 否 | `OrgUnit` | M1 仅允许 `OrgUnit`（其它值报错） |
| `name` | string | 是 |  | 默认 locale 名称 |
| `i18n_names` | json object | 否 | `{}` | 例如 `{"en":"Engineering","zh":"工程"}` |
| `status` | enum | 否 | `active` | `active/retired/rescinded` |
| `legal_entity_id` | uuid | 否 | null | 对齐 021（可选） |
| `company_code` | string | 否 | null | 对齐 021（可选） |
| `location_id` | uuid | 否 | null | 对齐 021（可选） |
| `display_order` | int | 否 | `0` | 同父排序 |
| `parent_code` | string | 否 | null | 为空表示 root（M1 必须且只能有一个 root code） |
| `manager_user_id` | bigint | 否 | null | 负责人 `users.id`（类型对齐 DB；导入时仅做存在性校验） |
| `manager_email` | string | 否 | null | 若无 `manager_user_id` 则可用 email 解析；`SELECT id FROM users WHERE tenant_id=$1 AND lower(email)=lower($2)`，找不到则报错 |
| `effective_date` | date/datetime | 是 |  |  |
| `end_date` | date/datetime | 否 | 自动补齐 |  |

**Root 规则（M1）**：
- 以 `parent_code` 为空识别 root。
- `nodes.csv` 中所有 `parent_code` 为空的行，其 `code` 必须相同（且全文件仅一个 root code）。
- root 不支持迁移：对 root code 的所有时间片，`parent_code` 必须始终为空（否则报错）。

### 4.4 `positions.csv`（映射到 `org_positions`）
| 列 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `code` | string | 是 |  | Position code |
| `org_node_code` | string | 是 |  | 引用 `nodes.csv.code` |
| `title` | string | 否 | null |  |
| `status` | enum | 否 | `active` | `active/retired/rescinded` |
| `is_auto_created` | bool | 否 | `false` |  |
| `effective_date` | date/datetime | 是 |  |  |
| `end_date` | date/datetime | 否 | 自动补齐 |  |

### 4.5 `assignments.csv`（映射到 `org_assignments`）
| 列 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `position_code` | string | 是 |  | 引用 `positions.csv.code` |
| `assignment_type` | enum | 否 | `primary` | `primary/matrix/dotted`（M1 仅允许写入 `primary`；其余返回校验错误） |
| `pernr` | string | 是 |  | 人员编号（trim 后不可空；允许前导零） |
| `subject_id` | uuid | 否 |  | 若提供则必须与 pernr 的映射一致；映射算法见 [DEV-PLAN-026](026-org-api-authz-and-events.md) |
| `effective_date` | date/datetime | 是 |  |  |
| `end_date` | date/datetime | 否 | 自动补齐 |  |

### 4.6 Import Manifest（`import --apply` 的产物）
- 文件名：`import_manifest_<timestamp>_<run_id>.json`
- 目的：记录本次写入产生的主键集合与关键映射，支持 `rollback --manifest` 精确回滚。
- 最小字段（v1）：
  - `version`（int，固定 1）
  - `run_id`（uuid）
  - `tenant_id`（uuid）
  - `mode`（`seed`）
  - `backend`（`db`）
  - `started_at`/`finished_at`（RFC3339）
  - `input`：`{dir, files:{nodes,positions,assignments}}`（文件名即可；hash 可后续追加）
  - `inserted`：按表列出插入的主键数组：`org_nodes/org_node_slices/org_edges/org_positions/org_assignments`
  - `subject_mappings`：`[{pernr, subject_id}]`（便于排障与对账）
  - `summary`：行数/插入数/跳过数（MVP 不做跳过）与耗时

## 5. 接口契约 (CLI Contracts)
### 5.1 二进制与子命令
- 二进制：`org-data`
- 子命令：
  - `org-data import --tenant <uuid> --input <dir> [--output <dir>] [--apply] [--strict] [--backend db] [--mode seed]`
  - `org-data export --tenant <uuid> --output <dir> [--as-of <date|rfc3339>]`
  - `org-data rollback --tenant <uuid> (--manifest <path> | --since <rfc3339>) [--apply] [--yes]`

### 5.2 退出码（MVP 约定）
- `0`：成功（dry-run 校验通过 / apply 成功 / export 成功 / rollback 成功）
- `2`：输入/校验错误（CSV/JSON/日期/环路/约束预检失败）
- `3`：使用错误（缺参数/冲突参数/不支持的组合，例如 `--backend db --mode merge`）
- `4`：DB 连接或事务错误
- `5`：DB 写入失败（违反 DB 约束/触发器拒绝等）
- `6`：安全网拒绝回滚（例如 `--since` 检测到历史数据、未提供 `--yes`）

### 5.3 输出约定
- 结构化摘要：打印到 stdout（JSON，一行），便于脚本采集。
- 日志：走项目 logger（stderr），必须包含 `run_id/tenant_id/mode/backend/apply` 字段。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 Parse & Normalize
1. 读取 CSV（剥离 BOM、解析 Header、逐行解析为 struct）。
2. 规范化：
   - trim 所有标识字段；
   - 解析 `effective_date/end_date` 并转为 UTC；
   - `end_date` 缺省按 4.2 规则补齐；
   - `manager_email` → `manager_user_id`（若提供 email 且缺 id，则查 `users` 表；查不到报错）。
3. 构建内存映射：
   - `nodeCode -> nodeID`（seed 模式下 nodeID 由工具生成 uuid 并写入 DB）
   - `positionCode -> positionID`
4. `subject_id`：
   - 若 CSV 未提供 `subject_id`：按 026 的 SSOT 算法生成；
   - 若提供：必须校验与算法一致，否则报错。

### 6.2 Static Validate（不访问 DB）
- 必填字段、枚举值、JSON shape（必须为 object）、`effective_date < end_date`。
- 同一实体时间片不重叠（node_slices / edges / positions / assignments 按各自键排序验证）。
- Root 规则（4.3）校验。
- 环路检测：
  - 默认：在最小 `effective_date` 的 as-of 快照上构建 parent 关系并做 DFS。
  - `--strict`：对输入中所有去重后的 `effective_date` 逐点做 as-of 检查（每点构建 parent 关系并 DFS），避免“不同时间片的边”误判或漏判。

### 6.3 DB Dry-Run（只读）
- `--mode seed --backend db`：
  - 空租户校验：`SELECT 1 FROM org_nodes WHERE tenant_id=$1 LIMIT 1` 若存在记录则拒绝（exit=2）。
  - 预检 manager_user_id（如提供）：`SELECT 1 FROM users WHERE tenant_id=$1 AND id=$2` 不存在则报错（避免写入后发现负责人无效）。

### 6.4 Apply（事务写入，`--apply`）
1. 开启事务。
2. 在 ctx 写入 tenant：`ctx = composables.WithTenantID(ctx, tenantID)`。
3. 注入 RLS：`composables.ApplyTenantRLS(ctx, tx)`（若 `RLS_ENFORCE=enforce` 且缺 tenant 则 fail-fast）。
4. 写入顺序（同一事务内）：
   1. `org_nodes`（root 的 `is_root=true`；其余 false）
   2. `org_node_slices`
   3. `org_edges`：必须按 parent-before-child 顺序写入（同一 `effective_date` 下按 depth 升序；root edge `parent_node_id=null`），以满足 021 的 path/depth 触发器对 parent_path 的 as-of 查询依赖
   4. `org_positions`
   5. `org_assignments`
5. 成功提交后写出 manifest（4.6）。

## 7. 安全与鉴权 (Security & Authz)
- DB backend 直接写库：仅允许 seed（空租户）以降低误用风险；merge 写入必须走未来的 API backend（以 025/026 的审计/outbox/缓存口径为准）。
- RLS 兼容：当 `RLS_ENFORCE=enforce` 时必须使用非 superuser 且无 `BYPASSRLS` 的 DB 账号（对齐 019/019A）；事务内必须注入 `app.current_tenant`（由 `composables.ApplyTenantRLS` 统一处理）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- [DEV-PLAN-021](021-org-schema-and-constraints.md)：表/约束/触发器（尤其 `org_edges.path/depth` 与 root edge 规则）。
- [DEV-PLAN-026](026-org-api-authz-and-events.md)：`subject_id` 映射 SSOT；以及未来 API backend 的 `/org/api/batch`。
- [DEV-PLAN-019A](019A-rls-tenant-isolation.md)：RLS 注入契约（通过 `pkg/composables` 落地）。

### 8.2 里程碑（按提交时间填充）
1. [ ] CLI 骨架（cobra）+ 命令/flag/退出码契约固化
2. [ ] CSV Parse/Normalize + Static Validate（含 strict 环路检测）
3. [ ] DB Dry-Run + seed Apply + manifest
4. [ ] export（全量/`--as-of`）
5. [ ] rollback（manifest / since 安全网）
6. [ ] Readiness 记录落盘

## 9. 测试与验收标准 (Acceptance Criteria)
- 单测（`go test ./cmd/org-data/...`）至少覆盖：
  - CSV 解析错误行号/字段定位
  - 环路/重叠/非法日期/非法枚举/非法 JSON
  - `subject_id` 映射一致性校验（对齐 026）
- 集成验收（本地 DB）至少覆盖：
  - seed 导入成功后，`org_edges` 的 root edge 与 child edge 均可写入（触发器 path/depth 生效）
  - `rollback --manifest --apply --yes` 能清理导入数据且无 FK 残留
- Readiness：将以下命令与输出记录到 `docs/dev-records/DEV-PLAN-023-READINESS.md`：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`

## 10. 运维与监控 (Ops & Monitoring)
- 工具默认 dry-run；任何写入/删除必须显式 `--apply`。
- `rollback --since` 仅用于 seed 兜底，且必须同时满足：
  - `--apply --yes`
  - `org_nodes.created_at < since` 不存在（否则拒绝，exit=6）
- 日志必须包含 `run_id/tenant_id`，并在成功时输出影响摘要（JSON）。

## 交付物
- `cmd/org-data` 源码。
- `scripts/org/*.sh`（可选包装器，用于运维一键调用）。
- CSV 模板示例：`docs/samples/org/*.csv`。
- Readiness 记录：`docs/dev-records/DEV-PLAN-023-READINESS.md`。
