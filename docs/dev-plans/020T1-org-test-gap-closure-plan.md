# DEV-PLAN-020T1：Org 测试缺口补齐方案（回归清单化）

**状态**: 草拟中（2025-12-23 23:50 UTC）— 针对 `docs/dev-plans/020L-org-feature-catalog.md` 的覆盖缺口，制定 TDD 测试补齐与回归清单落地步骤

## 1. 背景

`docs/dev-plans/020L-org-feature-catalog.md` 已将 DEV-PLAN-020~036 的 Org 功能点整理为“功能目录 + 覆盖映射 + 可执行回归清单”，并在 **6.2** 列出了当前自动化覆盖缺口。

本文（020T1）的职责是把这些缺口收敛为**可执行的测试补齐方案**：
- 逐项给出“应补什么测试、放在哪一层、验证哪些契约、如何判定完成”；
- 形成“测试驱动开发（TDD）”的实施节奏：先写失败的契约测试，再补实现/修复，再回填 020L 的覆盖映射，最终沉淀为回归清单。

> SSOT 规则：  
> - 功能目录与覆盖现状以 020L 为准（本文不重复列出全量功能）。  
> - 具体契约（字段/错误码/算法/阈值）以对应 dev-plan 为准（022/024/025/026/030/034）。  

## 2. 目标与非目标

### 2.1 目标（DoD）
- [ ] 将 020L:6.2 列出的缺口全部转换为自动化测试（或明确切分到后续 dev-plan 的测试方案 / readiness 证据，并在 020L 标注原因与入口）。
- [ ] 为每个缺口补齐“最小但稳定”的回归断言：优先断言 **HTTP status + 错误码/JSON 形状 + 关键不变式**，避免对易变字段做脆弱断言。
- [ ] 每补齐一项测试：在 `docs/dev-plans/020L-org-feature-catalog.md` 回填“已有测试/缺口”映射，并把回归入口命令保持可执行。
- [ ] 通过仓库门禁（以 `AGENTS.md` / `Makefile` / `.github/workflows/quality-gates.yml` 为准）。

### 2.2 非目标（Out of Scope）
- 不重做 020~036 的功能设计；不引入新 API/新错误码/新 schema（除非为修复测试暴露的既有缺陷且不改变已冻结契约）。
- 不把 053/056/057 等后续计划纳入本轮测试门槛；若缺口涉及“后续扩展”功能，仅在 020L 以注记形式记录，不在本计划强制补齐。

## 3. 测试策略（面向 TDD）

### 3.1 分层策略（优先级从高到低）
1. **API controller 契约集成测试（httptest + Postgres）**  
   目标：锁定 HTTP 行为（status/error code/meta）、鉴权 403 合同、以及写入副作用（outbox/audit/DB 不变式）。  
   适用：025 写路径、030 生命周期、026 batch 边界、以及 024 主链 nodes/assignments（含 auto-position 的最小副作用回归）。
2. **Service/Repository 集成测试（Postgres）**  
   目标：验证 DB 约束/触发器、不变式与复杂写语义（no-overlap、冻结窗口、审计落盘等）。  
   适用：022/025/029/032/033 等需要真实 DB 行为的能力域。
3. **纯单元测试（无 DB）**  
   目标：稳定验证纯逻辑（解析、阈值判定、schema 结构、label 红线）。  
   适用：034 `org-load` 报告 schema/阈值、metrics label、部分 request 解析器。
4. **E2E（Playwright）**  
   目标：锁定关键 UI 主路径；不追求覆盖全部边界（边界由 controller/服务层测试承担）。  
   适用：035 UI 回归与权限 403 页面。

### 3.2 TDD 执行节奏（每个缺口的固定套路）
- [ ] 先写测试：引用对应 dev-plan 的契约（022/024/025/026/030/034），让测试在当前实现下失败。  
- [ ] 再补最小实现/修复：优先修根因；避免为“让测试通过”引入临时逻辑。  
- [ ] 最后回填文档：更新 020L 的覆盖映射；如测试需要额外环境约束（例如需要 Postgres），明确写入 020L 的回归入口说明。

## 4. 缺口 → 测试补齐矩阵（020L:6.2）

> 说明：此处只列“缺口与测试落点”；契约细节请直接跳转到对应 dev-plan 章节。

| 缺口 | 契约 SSOT | 推荐层级 | 主要落点（建议） | 关键断言（最小集） |
| --- | --- | --- | --- | --- |
| 025 写路径 controller 契约缺失（nodes/assignments 的 correct/rescind/shift-boundary/correct-move） | 025 + 026 | controller 集成 | `modules/org/presentation/controllers/*_integration_test.go` | status + 错误码/JSON 形状；冻结窗口拒绝；`request_id` 关联；审计落盘；outbox 事件 topic/new_values 必填字段 |
| 030 change-request 生命周期（submit/cancel）与权限边界不足 | 030 + 026 | controller 集成 +（可选）E2E | `modules/org/presentation/controllers/*_integration_test.go` / `e2e/tests/org/*` | 状态机：draft→submitted/cancelled；immutable；跨租户拒绝；不写主表/不写 outbox |
| 034 `org_load_report.v1` schema/threshold 与 ops health 阈值逻辑缺失 | 034 | unit + controller 集成 | `cmd/org-load/*_test.go` + `modules/org/presentation/controllers/org_ops_health_*_test.go` | 报告 JSON 字段稳定；阈值判定可测；ops health 在 outbox backlog / deep-read build 过期时 degraded |
| 022/026 事件 payload old/new values 口径缺失 | 022 + 025/026 | service/controller 集成 | 复用 outbox 断言 helper（见 020L 已有测试） | `new_values` v1 必填字段；effective_window 口径一致；`old_values` 若已提供则非空且口径一致 |
| 024 controller 契约覆盖不足（nodes/assignments 更多字段组合、错误码映射边界、auto-position 副作用回归） | 024 + 025 + 026 | controller 集成 | `modules/org/presentation/controllers/org_api_*_integration_test.go` | nodes/assignments 的 CRUD 主路径 + 典型边界（invalid body、conflict、use_correct 等） |
| 026 batch command 组合/边界覆盖不足（move 限制/限流、permission mapping 命令等） | 026 | controller 集成 | `modules/org/presentation/controllers/org_api_snapshot_batch_integration_test.go`（扩展） | 原子性（单条失败全失败）；dry-run 无副作用；命令组合顺序；command_index meta；permission mapping 命令产出事件 |

## 5. 实施步骤（按风险排序）

### 5.1 025：写路径 controller 契约测试（优先）

SSOT：
- 025：`docs/dev-plans/025-org-time-and-audit.md`
- 026：`docs/dev-plans/026-org-api-authz-and-events.md`
- 022：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`

计划：
1. [ ] nodes：`POST /org/api/nodes/{id}:correct|:rescind|:shift-boundary|:correct-move`
   - [ ] happy path：返回 200；`effective_window` 口径正确；`org_audit_logs` 产生记录。
   - [ ] frozen window：返回 409 `ORG_FROZEN_WINDOW`（或 025 SSOT 定义的冻结错误码）；不产生写入副作用。
   - [ ] not found at date：返回 422 `ORG_NOT_FOUND_AT_DATE`。
   - [ ] shift-boundary：覆盖 `ORG_SHIFTBOUNDARY_INVERTED` / `ORG_SHIFTBOUNDARY_SWALLOW`。
2. [ ] assignments：按 025 的同名契约补齐最小覆盖（至少 1 条 happy path + 1 条冻结/subject 映射边界）。
3. [ ] outbox 事件：对写路径事件补齐 `new_values` 的 v1 必填字段断言（详见 5.4）。

实现提示（复用基建，避免重复造轮子）：
- controller 集成测试建议复用现有 helper（例如 `setupOrgTestDB`、`newOrgAPIRequestWithBody`、`withOrgRolloutEnabled`），并沿用当前测试的迁移清单约定（在单测内显式 apply 需要的 Org migrations）。

### 5.2 030：change-request submit/cancel 与权限边界

SSOT：
- 030：`docs/dev-plans/030-org-change-requests-and-preflight.md`
- 022：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`（payload 形状）
- 026：403 payload、authz 映射与 batch 合同

计划：
1. [ ] 状态机：draft → submit → (immutable)；draft/submitted → cancel → cancelled
   - [ ] submit：非 draft 返回 409 `ORG_CHANGE_REQUEST_NOT_DRAFT` 或 `ORG_CHANGE_REQUEST_IMMUTABLE`（以 030 SSOT 为准）。
   - [ ] cancel：幂等（重复 cancel 不应 500；返回码以 030 SSOT 为准）。
2. [ ] 权限边界：
   - [ ] 无权限：403 forbidden payload 对齐 026（已有 403 合同单测可复用断言）。
   - [ ] 跨租户：读取/提交/取消必须失败（优先断言 404/403/400 的稳定口径；以实现为准，但需“不会泄漏存在性”）。
3. [ ] 无副作用：
   - [ ] submit/cancel 不得写主链表（nodes/edges/assignments/positions）。
   - [ ] submit/cancel 不得写 outbox（022/017 链路）。

实现提示（复用基建，减少重复 setup）：
- 优先在现有 controller 集成测试文件上扩展（例如 `org_api_change_requests_preflight_integration_test.go`），复用其 DB 初始化、request 构造与 rollout/authz 开关 helper，避免在多处复制同一套测试脚手架。

### 5.3 034：org-load 报告 schema/阈值 与 ops health 阈值逻辑

SSOT：
- 034：`docs/dev-plans/034-org-ops-monitoring-and-load.md`

计划：
1. [ ] `cmd/org-load` 单测（无网络/无真实服务）：
   - [ ] `schema_version=1` 且 JSON 字段名冻结（`schema_version/run_id/started_at/.../thresholds`）。
   - [ ] 阈值判定：构造 stats 结果，断言 `p99_ms` 的 OK/Limit 逻辑稳定（避免未来重构破坏阈值语义）。
2. [ ] `/org/api/ops/health` 阈值集成测试（需 Postgres）：
   - [ ] outbox backlog：插入 `org_outbox` 未发布记录，触发 pending/oldest_available_age → degraded。
   - [ ] deep-read freshness：开启 rollout + deep-read backend=closure/snapshot，并构造“无 active build/active build age>24h/status!=ready” → degraded。
   - [ ] 断言稳定性：避免断言具体耗时字符串/age 字符串的精确值；优先断言 `status` 与关键字段存在/类型正确。

### 5.4 022/026：事件 old/new values 口径测试

SSOT：
- 022：事件 v1 字段与 `new_values` 必填结构
- 025：审计 `old_values/new_values` 语义

计划（以最小稳定断言为目标）：
1. [ ] `new_values` 必须包含 022 v1 对应 `entity_type` 的必填字段（只断言字段存在与类型，不断言不稳定的派生字段）。
2. [ ] `effective_window` 与主表 slice 的 `effective_date/end_date` 一致。
3. [ ] `old_values`：按 022 v1 约定其为“可选但推荐”，测试应先盘点当前实现哪些 change_type 会带 `old_values`，对已提供的场景断言其为非空 JSON object；如需将 `old_values` 升级为硬合同，应先更新 022（发布 v2 或提升 v1 约束）再补强测试。

### 5.5 024：controller 契约扩展（nodes/assignments + 错误码边界）

SSOT：
- 024：主链 CRUD 合同（nodes/assignments）
- 025：use_correct / use_correct_move 等分流错误码

计划：
1. [ ] nodes/assignments：补齐“更多字段组合”的回归用例（i18n_names/company_code/location/manager 解析等），避免只覆盖最小字段导致回归漏检。
2. [ ] auto-position：补齐“assignment 写入触发 position 自动创建”的稳定断言（如果 020L 已覆盖仅做回归巩固，不再扩张断言面）。

范围说明（避免漂移）：
- `/org/api/positions*`（含 correct/rescind/shift-boundary）在 026 的 Authz 映射中归属 053/056；不属于 020~036 的门槛。本计划仅覆盖 024 的 auto-position 副作用与其对 outbox/audit 的口径一致性。

### 5.6 026：batch command 组合与边界

SSOT：
- 026：`/org/api/batch` 合同、command_index meta、dry-run 语义、permission mapping 命令

计划：
1. [ ] 原子性：构造 `commands=[valid, invalid]`，断言整体失败且无副作用（主表/outbox/audit 都不变）。
2. [ ] 组合路径：同一 batch 内“先建 node 再建 assignment（触发 auto-position）”等组合（按 026 SSOT 的允许范围）。
3. [ ] permission mapping 命令：`security_group_mapping.*`、`link.*` 的最小 happy path + rescind path（并断言事件产出）。
4. [ ] move 限制/限流：覆盖至少一条“影响面过大/不允许 move root/跨层级非法”的稳定错误码（以 026/030 的 SSOT 为准）。

实现提示：
- 建议优先扩展 `modules/org/presentation/controllers/org_api_snapshot_batch_integration_test.go`，复用其 helper 与迁移清单；新增场景尽量使用 “dry-run + 无副作用断言” 避免污染测试 DB。

## 6. 完成与回填（文档/门禁）

当 5.x 全部 `[X]` 后：
- [ ] 将本文状态更新为 `已完成`（按 `docs/dev-plans/000-docs-format.md` 的状态规则）。
- [ ] 更新 `docs/dev-plans/020L-org-feature-catalog.md` 的缺口列表与回归入口（使其成为可直接执行的回归清单）。
- [ ] 若发现 020L 的缺口描述与 020~036 scope 不一致（例如 `/org/api/positions*` 实际归属 053/056），需在 020L 标注“后续计划”并链接到对应测试方案，避免范围漂移。
- [ ] 运行并记录与变更命中的门禁（SSOT：`AGENTS.md` / `Makefile`）。
