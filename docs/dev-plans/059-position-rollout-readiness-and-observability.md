# DEV-PLAN-059：Position 交付收口（Readiness / 回滚 / 可观测性）（对齐 051 收口）

**状态**: 实施中（最小冒烟/追溯闭环已落地并合并；兼容性回归与更全面可观测仍待收口，2025-12-21）

## 0. 进度速记
- 本计划负责把 052-058 的交付收口成“可上线、可回滚、可观测、可复现”的整体：readiness 记录 + 灰度/回滚开关 + 最小冒烟/演示闭环 + 关键指标与审计追溯。
- 收口原则：**先口径冻结、再灰度启用、最后 enforce**；任何强校验/强治理能力必须具备 `disabled/shadow/enforce` 的回退路径（对齐 025 的冻结窗口模式）。
- 实现已合并：
  - PR #108（DEV-PLAN-059 补齐）：https://github.com/jacksonlee411/Bugs-Blossoms/pull/108（merged 2025-12-21，merge commit `094b4156e13508fc49428365e4120443cd9b5164`）
  - PR #107（DEV-PLAN-059A）：https://github.com/jacksonlee411/Bugs-Blossoms/pull/107（merged 2025-12-21，merge commit `7ff68853e29fb59cf0fbc867a8a9e3201d7dc939`）
  - PR #110（059 可观测补齐：治理拒绝结构化日志）：https://github.com/jacksonlee411/Bugs-Blossoms/pull/110（merged 2025-12-21，merge commit `36154960`）

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 业务需求：`docs/dev-plans/050-position-management-business-requirements.md`（effective dating、Correct/Rescind、冻结窗口、FTE、限制、统计与空缺分析）。
  - 实施蓝图：`docs/dev-plans/051-position-management-implementation-blueprint.md`（052-059 拆分与收口门槛；Readiness/回滚/可观测）。
- **依赖链路（必须对齐）**：
  - Contract/口径冻结：`docs/dev-plans/052-position-contract-freeze-and-decisions.md`
  - Position Core：`docs/dev-plans/053-position-core-schema-service-api.md`
  - Authz：`docs/dev-plans/054-position-authz-policy-and-gates.md`
  - UI：`docs/dev-plans/055-position-ui-org-integration.md`
  - 主数据/限制：`docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`
  - 报表/运营：`docs/dev-plans/057-position-reporting-and-operations.md`
  - 任职增强：`docs/dev-plans/058-assignment-management-enhancements.md`
  - 时间/冻结窗口/审计：`docs/dev-plans/025-org-time-and-audit.md`
  - 事件契约/outbox：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`、`docs/dev-plans/026-org-api-authz-and-events.md`、`docs/dev-plans/017-transactional-outbox.md`
  - 组织范围/性能/运维：`docs/dev-plans/029-org-closure-and-deep-read-optimization.md`、`docs/dev-plans/027-org-performance-and-rollout.md`、`docs/dev-plans/034-org-ops-monitoring-and-load.md`
- **当前痛点**：
  - 变更项分散在多个子计划：如果缺少统一收口清单与复现记录，reviewer 无法判断“是否可上线/可回滚/可定位问题”。
  - 强校验（冻结窗口/冲突拒绝/超编/限制）若直接 enforce，容易误伤兼容链路（System Position/auto position）并造成线上写阻断。
  - 可观测不足会导致“拒绝原因不可定位、审计串不起来、outbox 堆积无法发现”。
- **业务价值**：
  - 形成一套可复跑的 readiness 记录 + 冒烟闭环脚本，让 Position 交付满足“可验证、可演示、可回退、可运维”的工程化上线门槛。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [X] **readiness 可复现**：关键门禁命令、结果与时间戳有记录，reviewer 可复跑（对齐 CI 门禁口径）。
- [X] **灰度与回滚可执行**：关键写入口与强校验具备灰度/开关回滚/校验降级等最小回退路径，并明确不可逆点（优先“开关回滚”，避免依赖 destructive down）。
- [ ] **兼容性回归**：System/Managed 策略不破坏存量链路；口径映射在 UI/API/统计中一致（对齐 052 冻结）。
- [ ] **可观测可排障**：Correct/Rescind/冻结窗口拒绝/超编阻断/冲突拒绝等关键路径可通过结构化日志 + 审计 + outbox 追溯定位。
- [X] **最小冒烟闭环可演示**：在单租户环境完成“创建/变更/更正/撤销/查询/统计/空缺”最小链路，并把请求/结果写入 readiness 记录。

### 2.2 非目标（Out of Scope）
- 不在本计划内新增大范围业务能力；本计划只做“收口与上线门槛”，不引入与 050 无关的新功能。
- 不在本计划内建设长期 BI/告警体系；只交付最小可观测与排障入口（与 034 对齐）。

## 2.3 工具链与门禁（SSOT 引用）
> 目的：避免在 dev-plan 中复制工具链细节导致 drift；本节只声明“本计划命中哪些触发器/工具链”并引用 SSOT。

- **触发器清单（本计划预计命中）**：
  - [X] Go 代码（Position/Assignment/Reports/ops 相关实现与测试）
  - [X] DB 迁移 / Schema（Org Atlas+Goose；若子计划引入 schema 变更）
  - [X] Authz（Position/Assignment/Reports 的 object/action、策略碎片与测试门禁）
  - [X] Outbox（Position/Assignment 事件必须通过 outbox 落盘投递）
  - [X] 路由治理（新增/调整 API 时需对齐 routing 策略）
  - [ ] `.templ` / Tailwind（仅当 055/Position UI 触发）
  - [ ] 多语言 JSON（仅当新增/修改 locales）
  - [X] 文档（本计划与 readiness 记录）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org 工具链：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
  - Authz 流程：`docs/runbooks/AUTHZ-BOT.md`
  - Outbox 与排障：`docs/runbooks/transactional-outbox.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 收口分层：模块开关 / 能力开关 / 校验模式（选定）
- **模块级灰度（已存在，复用 027）**：按租户 allowlist 启用 Org（含 Position）：
  - `ORG_ROLLOUT_MODE` + `ORG_ROLLOUT_TENANTS`（一键下线与逐租户灰度）。
- **能力级灰度（建议）**：Position/Assignment/Reports 的高风险能力（Correct/历史改动/限制强校验/扩展任职类型/计划任职与调岗：复用 `assignment.update` Insert slice）必须可单独开关。
- **校验模式三态（选定）**：所有强校验统一采用 `disabled/shadow/enforce`：
  - `disabled`：不校验（仅用于紧急止血/回滚）
  - `shadow`：不阻断，但必须写审计/日志（用于观察与评估误伤）
  - `enforce`：阻断并返回稳定错误码

### 3.2 Readiness 记录作为 SSOT（选定）
- 本系列收口 readiness 记录统一落盘：`docs/dev-records/DEV-PLAN-051-READINESS.md`。
- 记录要求：
  - 必须包含：环境要素（Go/PG 版本、DB、tenant）、开关配置、关键门禁命令与结果、最小冒烟步骤与响应摘要。
  - 禁止“只写结论不写命令”；reviewer 必须能按记录复跑得到一致结论。
- 文档可发现性：readiness 记录必须在 `AGENTS.md` 的 Doc Map 中有入口链接（避免“文件存在但不可发现”）。

### 3.3 可观测链路：request_id → audit → outbox（选定）
```mermaid
flowchart LR
  UI[UI/Client] --> API[Org API]
  API --> SVC[Org Service]
  SVC --> DB[(Postgres)]
  SVC --> AUD[org_audit_logs]
  SVC --> OB[org_outbox]
  API --> LOG[structured logs]
  OB --> RELAY[outbox relay]
```
- `request_id` 必须贯穿：API 响应、日志、审计（`org_audit_logs.request_id`）、事件（outbox payload/headers）。
- 任何“拒绝/阻断”必须可定位到：错误码 + 关键字段（tenant、entity、effective_date、change_type、mode）。

**追溯口径（v1，冻结）**
- **成功写入**：必须产生 `org_audit_logs` 与 `org_outbox`，可按下述 SQL 追溯。
- **outbox relay（是否要求 publish，v1，冻结）**：
  - readiness 最低要求：验证 `org_outbox` **enqueue** 可追溯（存在记录即可）。
  - 若环境启用了 relay（例如 `OUTBOX_RELAY_ENABLED=true` 且 `OUTBOX_RELAY_TABLES` 包含 `public.org_outbox`）：readiness 需要额外记录至少一条事件被 publish（`published_at IS NOT NULL`）的证据；否则在 readiness 中明确标注“未启用 relay，仅验证 enqueue”。
- **拒绝/阻断（400/403/409/422 等）**：默认**不落审计/不写 outbox**；以结构化日志 + API 返回的 `meta.request_id` 为唯一追溯入口（最低字段见 §7）。
- **日志字段命名（现状对齐）**：
  - 语义上的 `request_id` 指同一值：HTTP header `X-Request-Id` / API error `meta.request_id` / OTel attribute `http.request_id`。
  - 结构化日志字段名现状为 `request-id`（中间件）；本计划在文档中统一用“`request_id`（值）”描述，不强制日志 key 必须叫 `request_id`，但必须能通过该值检索。

**最小追溯 SQL（示例）**
```sql
-- 审计：按 request_id 串起“成功写入”的上下文（拒绝默认不落审计）
SELECT
  transaction_time, change_type, entity_type, entity_id, effective_date, end_date, meta
FROM org_audit_logs
WHERE tenant_id = $1 AND request_id = $2
ORDER BY transaction_time ASC;

-- Outbox：按 request_id 找到对应事件（payload 形状以 026/022 为 SSOT）
SELECT
  created_at, topic, event_id, published_at, attempts, last_error
FROM org_outbox
WHERE tenant_id = $1 AND payload->>'request_id' = $2
ORDER BY created_at ASC;
```

## 4. 配置模型与约束 (Data Model & Constraints)
> 本节冻结“上线/灰度/回滚”依赖的最小配置合同，避免在收口阶段临时发明开关导致 drift。

### 4.1 开关与模式清单（v1，SSOT 对齐）
| 层级 | 开关/字段 | 位置 | 允许值 | 默认 | 说明（SSOT） |
| --- | --- | --- | --- | --- | --- |
| 模块级灰度 | `ORG_ROLLOUT_MODE` | env | `disabled|enabled` | `disabled` | 一键下线与 allowlist 灰度（027） |
| 模块级灰度 | `ORG_ROLLOUT_TENANTS` | env | `<uuid,uuid,...>` | 空 | `enabled` 时生效；空表示不启用任何租户（027） |
| 能力开关 | `ENABLE_ORG_AUTO_POSITIONS` | env | `true|false` | `true` | auto position 链路兼容（053/058） |
| 能力开关 | `ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES` | env | `true|false` | `false` | 任职类型写入（`matrix/dotted`）灰度（058） |
| 校验模式 | `org_settings.freeze_mode` | DB（tenant） | `disabled|shadow|enforce` | `enforce` | 冻结窗口模式（025） |
| 校验模式 | `org_settings.freeze_grace_days` | DB（tenant） | `0..31` | `3` | 冻结宽限（025） |
| 校验模式 | `org_settings.position_catalog_validation_mode` | DB（tenant） | `disabled|shadow|enforce` | `shadow` | Job Catalog/Profile 校验灰度（056） |
| 校验模式 | `org_settings.position_restrictions_validation_mode` | DB（tenant） | `disabled|shadow|enforce` | `shadow` | Position Restrictions 校验灰度（056） |
| 校验模式 | `org_settings.reason_code_mode` | DB（tenant） | `disabled|shadow|enforce` | `shadow` | reason_code 兼容期→加严收口（合同与实施 SSOT：DEV-PLAN-059A） |

### 4.2 优先级与回退原则（选定）
- **优先级**：
  1) 模块级灰度（`ORG_ROLLOUT_MODE/ORG_ROLLOUT_TENANTS`）优先于一切能力/校验开关：未启用租户直接返回 404 `ORG_ROLLOUT_DISABLED`（现状实现）。
  2) 能力开关（env）用于全局 kill-switch；校验模式（tenant `org_settings`）用于逐租户灰度与回退。
- **System/Managed 兼容**（对齐 052）：
  - System/auto-created 链路优先保证“不中断”；强校验默认只对 Managed 生效，System 至多进入 `shadow` 并写审计/日志。
- **`disabled/shadow/enforce` 语义统一**：
  - `disabled`：跳过该校验（仅紧急止血）
  - `shadow`：允许写入，但必须在审计 `meta` 与结构化日志中标记命中（用于评估误伤）
  - `enforce`：阻断写入并返回稳定错误码（错误码口径以 052/053/056/058 为 SSOT）
- **reason_code 收口（对齐 052）**：
  - `shadow`：若缺失 `reason_code`，后端可填充 `legacy` 以保持审计链路不断，并在审计/日志标记“缺失被填充”。
  - `enforce`：缺失 `reason_code` 返回 400 `ORG_INVALID_BODY`（不再隐式填充）。

## 5. Readiness 记录与最小冒烟（Contract）
### 5.1 Readiness 记录模板（v1）
`docs/dev-records/DEV-PLAN-051-READINESS.md` 至少包含：
1. 本次交付范围（涉及哪些子计划：053/054/055/056/057/058）
2. 开关与配置（env + 租户级 settings；含 `disabled/shadow/enforce` 的模式）
3. 本地门禁命令（按 `AGENTS.md` 触发器矩阵；记录命令/结果/时间戳）
4. 最小冒烟闭环（见 §5.2）：请求/响应摘要 + 关联 request_id
5. 回滚演练：至少一次“开关回滚”或“能力/校验降级回退”演练记录

### 5.2 最小冒烟闭环（v1，执行时填写）
> 目标：覆盖 050 的关键治理链路，并对齐 051 的“可演示/可回滚/可观测”门槛；具体 API 以 053/057/058 的最终合同为准。

- Position：
  - 创建（Managed）→ Update（新版本）→ Correct（原位更正）→ Rescind（撤销/截断）
- Assignment：
  - 占用/释放（`allocated_fte`）→ 调岗/计划任职（复用 `PATCH /org/api/assignments/{id}` 的 Insert slice；对齐 058）→（可选）非 primary 写入（`matrix/dotted`，受 `ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES` 控制）
- 查询：
  - as-of 查询（Position/Assignment）→ 时间线查询（Position/Assignment）
- 报表（若 057 落地）：
  - 编制汇总（FTE）→ vacancies 列表 → time-to-fill（基础）
- 可观测：
  - 对任一 Correct/Rescind：能从 response/request_id 找到对应审计与 outbox 事件（或 shadow 记录）。
  - 对任一拒绝/阻断：能从 response/request_id 在结构化日志中定位原因与上下文（拒绝默认不落审计/不写 outbox）。

## 6. 回滚与灰度（Rollout & Rollback）
### 6.1 回滚优先级（选定）
1. **开关回滚（优先）**：从 `ORG_ROLLOUT_TENANTS` 移除租户或切 `ORG_ROLLOUT_MODE=disabled`。
2. **能力开关回滚**：关闭高风险能力（例如 `ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES=false`），优先让写入口恢复到 v1 安全口径（对齐 058/052）。
3. **校验降级**：把高风险校验从 `enforce` 切回 `shadow`/`disabled`（保留审计/日志；对齐 §4.2）。
4. **数据回滚（最后手段）**：仅在有 manifest/明确范围时执行（对齐 023/031 的“默认 dry-run + 可回滚”口径）。

### 6.2 不可逆点清单（必须明确）
- schema 破坏性变更（drop/rename/类型缩窄）在生产环境不可依赖 down 回滚；必须以兼容期/双写/开关回滚替代。
- audit/outbox 属于可追溯链路：历史审计不删除；回滚只能通过新事件/新更正补偿。

## 7. 可观测性（Observability）
- **结构化日志（最低字段）**：
  - `request_id`, `tenant_id`, `initiator_id`, `change_type`, `entity_type`, `entity_id`, `effective_date`, `mode`, `error_code`
- **拒绝/阻断日志事件名（最低集，v1）**：
  - `org.reason_code.rejected`（059A：reason_code_mode=enforce 拒绝）
  - `org.frozen_window.rejected`（冻结窗口 enforce 拒绝，error_code=`ORG_FROZEN_WINDOW`）
  - `org.position_catalog.rejected`（Job Catalog/Profile 校验 enforce 拒绝）
  - `org.position_restrictions.rejected`（Position Restrictions 校验 enforce 拒绝）
- **关键指标（建议纳入 034 Prometheus；先复用现有指标，避免新增高基数 label）**：
  - API：`org_api_requests_total{endpoint,result}`、`org_api_latency_seconds{endpoint,result}`（现状已有）
  - 写冲突：`org_write_conflicts_total{kind}`（现状已有）
  - Outbox：`outbox_pending{table=\"public.org_outbox\"}`、`outbox_dispatch_total{table,topic,result}`、`outbox_dead_total{table,topic}`（017/034）
- **审计追溯**：
  - 必须能按 `request_id` 查询 `org_audit_logs`，并从 `meta` 看见 `freeze_mode/freeze_violation`、限制冲突等关键上下文（对齐 025/056/057/058）。
  - 拒绝/阻断默认不落审计：必须至少在日志中包含 `request_id`（值）与 `error_code`，并能关联到 tenant 与操作类型（对齐 §3.3 的“追溯口径”）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：`DEV-PLAN-053/054/055/056/057/058` 的核心交付完成（或明确 scope 缩减）；并对齐 025/026 的冻结窗口、审计与 outbox。
- **里程碑**：
  1. [X] readiness 记录落盘：补齐 `docs/dev-records/DEV-PLAN-051-READINESS.md` 并覆盖模板项（已回填 051 readiness §3.5，2025-12-21）
  2. [ ] 灰度/回滚开关到位：至少包含模块开关 + 关键校验模式切换
  3. [ ] 可观测收口：日志字段/指标/审计链路可追溯
  4. [X] 最小冒烟跑通并记录（含 request_id 追溯）（`scripts/org/059_smoke.sh` + 051 readiness §3.5，2025-12-21）

## 9. 测试与验收标准 (Acceptance Criteria)
- readiness 记录可复现：关键门禁与冒烟步骤可按记录复跑。
- 强校验具备回退路径：从 `enforce → shadow/disabled` 可在不回滚代码/不改库的情况下止血。
- 关键拒绝可定位：拒绝/阻断能通过日志（按 `request_id` 值）定位原因；成功写入能通过日志+审计+outbox（按 `request_id`）追溯上下文与事件。

## 10. 交付物
- `docs/dev-records/DEV-PLAN-051-READINESS.md`（收口记录，含门禁/冒烟/回滚演练）。
- `AGENTS.md` Doc Map：确保 readiness 记录入口链接可发现（文档可发现性）。
- 回滚/灰度/兼容策略清单（含不可逆点说明与回退优先级）。
- 可观测性清单与落地要求（日志字段/指标/outbox 排障入口）。
- 059 最小冒烟入口：`scripts/org/059_smoke.sh` + `modules/org/services/org_059_rollout_smoke_integration_test.go`。
