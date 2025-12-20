# DEV-PLAN-058：任职管理增强（对齐 050 §10，051 阶段 F）

**状态**: 草拟中（2025-12-20 05:12 UTC）

## 0. 进度速记
- 本计划对齐 050 §10 的“任职管理后续能力清单”，作为 051 阶段 F 的独立里程碑：在不阻塞 053（v1）上线的前提下，分增量交付“任职类型/计划任职/多段任职/历史更正与审计增强”。
- 已有基线能力（当前代码）：
  - Org API 已提供 `/org/api/assignments`（list/create/update/correct/rescind）与 batch command（`assignment.*`）。
  - Correct/Rescind 已复用 025 的冻结窗口与审计落盘；但扩展任职类型目前被禁用（`EnableOrgExtendedAssignmentTypes` 尚未启用）。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 业务需求：`docs/dev-plans/050-position-management-business-requirements.md`（§10）。
  - 实施蓝图：`docs/dev-plans/051-position-management-implementation-blueprint.md`（阶段 F：任职管理增强）。
- **依赖链路（必须对齐）**：
  - `docs/dev-plans/052-position-contract-freeze-and-decisions.md`：状态/口径冻结、System/Managed 策略。
  - `docs/dev-plans/053-position-core-schema-service-api.md`：Assignment v1 的写入口与稳定错误码（本计划不改变 v1 合同，只做增量扩展）。
  - `docs/dev-plans/025-org-time-and-audit.md`：冻结窗口、审计、Correct/Rescind/ShiftBoundary 口径（Assignment 必须复用）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：Authz/403 payload、outbox、batch、subject_id 映射（Assignment 以此为 SSOT）。
  - `docs/dev-plans/054-position-authz-policy-and-gates.md`：Assignment 的 object/action 与测试门禁（新增能力需补齐）。
  - `docs/dev-plans/059-position-rollout-readiness-and-observability.md`：灰度/回滚/可观测收口（本计划需提供可回退路径）。
- **当前痛点**：
  - 仅支持 `primary` 任职：无法表达“兼任/代理”等业务场景，也无法支持后续 vacancy/time-to-fill 的更精细口径。
  - “计划任职/未来调岗”操作需要多步手工组合（先截断当前任职，再创建未来任职），容易造成时间窗冲突或数据断档。
  - 多段任职事件缺少明确的数据契约与操作语义（如何表示、如何更正、如何审计对账），后续报表口径会漂移。
- **业务价值**：
  - 以最小增量补齐 050 §10 的任职能力：支持历史/当前/未来任职、同一员工多段任职、任职类型，并确保“可审计、可回滚、可复现”。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **任职类型可用**：支持至少“主任职/兼任/代理”（050 §10），并明确默认规则、互斥约束与对占编/统计的影响。
- [ ] **计划任职可用**：允许创建未来生效的任职；提供“调岗/转任职”的原子命令，避免人工拼接导致冲突。
- [ ] **多段任职事件可追溯**：同一员工在同一职位可出现多段任职；列表/时间线可解释；vacancy/time-to-fill 可复用。
- [ ] **历史更正与审计增强**：支持更正历史（Correct）与撤销（Rescind）并保留可追溯审计；冻结窗口策略一致且可灰度（disabled/shadow/enforce）。
- [ ] **门禁与可回滚**：触发的本地门禁可通过；扩展能力具备 feature flag 灰度与快速回退路径（对齐 059）。

### 2.2 非目标（Out of Scope）
- 不实现招聘全链路与招聘事件（本计划的 vacancy/time-to-fill 仍以 Position/Assignment 的时间线推导为准）。
- 不在本计划内引入跨域强耦合（例如直接依赖 HRM domain types）；跨域通过 subject_id 映射与只读字段对齐。

### 2.3 工具链与门禁（SSOT 引用）
> 本节只声明触发器与 SSOT 引用，避免复制命令矩阵导致 drift。

- **触发器清单（本计划预计命中）**：
  - [X] Go 代码（Org service/repo/controller、测试）
  - [X] 路由治理（新增/调整 `/org/api/assignments*` 或 `/org/api/batch` command）
  - [X] Authz（Assignment 扩展能力的 object/action 与测试）
  - [ ] DB 迁移 / Schema（如需新增列/约束/索引；需按 Org Atlas+Goose 工具链执行）
  - [X] 文档（本计划更新；readiness 记录以 059 为准）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Org 时间/审计：`docs/dev-plans/025-org-time-and-audit.md`
  - Org API/Authz/outbox：`docs/dev-plans/026-org-api-authz-and-events.md`
  - Authz 工作流：`docs/runbooks/AUTHZ-BOT.md`
  - Org 迁移工具链：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 任职以“时间片 + 审计/outbox”作为 SSOT（选定）
- 任职仍使用 Org 的 valid-time 时间片模型（`effective_date/end_date`），并复用 025 的冻结窗口与审计落盘；禁止绕过 service 直接写表（否则无法保证审计/outbox/冻结窗口一致）。

### 3.2 任职类型落地策略（选定：复用现有 `assignment_type` 枚举）
- DB 已允许 `assignment_type in ('primary','matrix','dotted')`；本计划在业务语义上做映射：
  - `primary`：主任职（同一 subject 同窗仅一个 primary，现有排他约束兜底）
  - `matrix`：兼任（允许与 primary 重叠；是否占编由 v1 规则冻结）
  - `dotted`：代理/临时任职（允许与 primary 重叠；默认需显式 end_date 或通过 rescind 截断）
- 若业务最终要求不同命名（例如 `acting`），以“新增值 + 兼容旧值”的方式演进，避免破坏存量数据与审计链路。

### 3.3 计划任职优先通过“原子转任职命令”交付（选定）
- 为避免“先截断再创建”的人工拼装，本计划新增 batch command `assignment.transfer`（或等价命令），在同一事务内：
  1) 截断当前任职到 `transfer_effective_date`
  2) 创建目标职位的新任职（effective_date=transfer_effective_date）
  3) 写审计 + outbox（对齐 025/026）

## 4. 数据模型与约束 (Data Model & Constraints)
> 以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为 schema SSOT；本节冻结“本计划需要的最小字段与约束”。

### 4.1 `org_assignments`（扩展字段，v2）
> 说明：若 053 已引入同名字段/约束，以 053 的最终契约为准；058 只追加必要的增量，不重复定义。

- `fte numeric not null default 1.0`：任职占用的 FTE（支持 0.5/1.0 等），用于占编/填充状态与报表（对齐 050 §7.3）。
- `end_reason text null`：结束/撤销原因（写审计 meta 仍为主；此字段仅用于常用查询/导出，避免频繁扫 audit logs）。

**约束（v2）**
- `check (fte >= 0)`；`fte=0` 仅用于“计划但不占编”或特殊策略，是否允许需冻结（默认不允许）。
- primary 互斥：继续由现有 EXCLUDE 约束兜底（同 subject 同窗仅一个 primary）。
- Position 维度的“同窗仅一条 assignment”约束在支持“一岗多人/部分填充”后应移除或改造（由 053 先行；若仍存在将阻塞本计划能力）。

### 4.2 迁移策略（Up/Down，最小可回退）
- **Up**：
  1. 为 `org_assignments` 新增 `fte/end_reason`（如未存在）与必要索引。
  2. 如需新增 `assignment.transfer`：仅涉及 service/controller + 现有表，尽量不引入新表。
- **Down（本地/非生产）**：
  - 移除新增列；生产环境回滚优先通过 feature flag 禁用新能力（对齐 059）。

## 5. 接口契约 (API Contracts)
> 约定：内部 API 前缀 `/org/api`；JSON-only；Authz/403 payload 对齐 026；时间参数支持 `YYYY-MM-DD` 或 RFC3339（统一 UTC）。

### 5.1 现有 Assignment API（v1，作为基线）
- `GET /org/api/assignments?subject=person:{pernr}[&effective_date=...]`：查询任职（as-of 或时间线）
- `POST /org/api/assignments`：创建任职（支持未来 effective_date）
- `PATCH /org/api/assignments/{id}`：Update（新增版本）
- `POST /org/api/assignments/{id}:correct`：Correct（原位更正）
- `POST /org/api/assignments/{id}:rescind`：Rescind（截断 end_date）

### 5.2 变更：开放扩展任职类型（v2）
- `POST /org/api/assignments` 的 `assignment_type` 允许值扩展为 `primary|matrix|dotted`（受 feature flag 控制）。
- `GET /org/api/assignments` 返回中保留 `assignment_type/is_primary` 字段（作为对账口径）。

**错误码（最小集）**
- `422 ORG_ASSIGNMENT_TYPE_DISABLED`：当 feature flag 未开启且请求非 primary。
- `409 ORG_ASSIGNMENT_OVERLAP`：违反 primary 互斥或（如仍存在）position 同窗互斥约束。

### 5.3 新增：`POST /org/api/batch` command `assignment.transfer`（v2）
> 通过 batch 交付“原子转任职”，减少 UI/API 编排复杂度，并复用 030/031 的 dry-run 安全网。

**Payload（示意）**
```json
{
  "type": "assignment.transfer",
  "payload": {
    "pernr": "000123",
    "effective_date": "2025-03-01",
    "from_assignment_id": "uuid",
    "to_position_id": "uuid",
    "to_org_node_id": "uuid|null",
    "to_assignment_type": "primary",
    "fte": "1.0",
    "reason": "transfer"
  }
}
```

**Result（示意）**
```json
{ "from_assignment_id": "uuid", "to_assignment_id": "uuid" }
```

### 5.4 计划任职（planned assignments，v2 口径）
- 任职 `effective_date > nowUTC` 即视为“计划任职”；该任职：
  - 不影响 `as-of now` 的占编与填充状态；
  - 会在到达生效日后自动纳入 as-of 统计（无需额外迁移步骤）。
- 报表/看板若需要展示“已计划填补”的空缺，必须明确：
  - “vacant but planned”的判定口径（例如：当前 vacant 且存在未来 primary assignment）；
  - 该口径以 057 的 SSOT 为准，本计划只提供必要查询支撑。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 任职类型校验（v2）
输入：`assignment_type`
1. 若 `assignment_type=primary`：允许（维持 v1 行为）。
2. 若 `assignment_type!=primary`：
   - 需要启用 feature flag（例如 `EnableOrgExtendedAssignmentTypes=true`）；
   - 写入后需明确是否纳入占编（默认：纳入，占用由 `fte` 决定；如业务确认不纳入，则以白名单类型控制并记录到 052/059）。

### 6.2 原子转任职（assignment.transfer）
输入：`tenant_id, pernr, transfer_effective_date, from_assignment_id, to_position_id|to_org_node_id, fte, reason`
1. 开启事务并应用租户 RLS（对齐 026）。
2. 计算 subject_id（SSOT：026 的 NormalizedSubjectID）。
3. 冻结窗口校验：`affected_at = transfer_effective_date`（对齐 025）。
4. 锁定 from assignment（FOR UPDATE），并校验其在 `transfer_effective_date` 之前有效。
5. 截断 from assignment：`end_date = transfer_effective_date`（不得产生负窗/重叠）。
6. 创建 to assignment（effective_date=transfer_effective_date，end_date=9999-12-31）：
   - Position 存在性（as-of）校验；
   - DB 约束兜底：primary 互斥、（如仍存在）position 互斥；
7. 写审计（old/new values + reason）并 enqueue outbox events（对齐 025/026）。

### 6.3 多段任职事件（segments）表示与约束（v2）
- 表示方式：每段任职对应 `org_assignments` 一行时间片 `[effective_date,end_date)`；同一员工在同一职位出现多次任职即为多段时间片（050 §10）。
- 约束策略（最小集）：
  - primary：同一 subject 同窗最多一个（DB EXCLUDE 兜底）。
  - 非 primary：允许与 primary 重叠；是否允许“同类型多条重叠”需冻结（默认不允许：建议新增 EXCLUDE 约束或 service 校验）。
  - 同一 subject 在同一 position 的多段任职：允许多段但禁止重叠（以校验/约束兜底）。

## 7. 安全与鉴权 (Security & Authz)
> 以 054 为 SSOT；本节只冻结 endpoint/command → object/action 的最小映射。

- `assignment.create/update`：object=`org.assignments` action=`assign`
- `assignment.correct/rescind/transfer`：object=`org.assignments` action=`admin`（历史更正/强治理能力默认高权限）
- `assignment.list`：object=`org.assignments` action=`read`

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- 053：Assignment v1 合同稳定（写入口/错误码/事件）。
- 025/026：冻结窗口、审计、outbox、subject_id 映射。
- 054：Authz 能力与门禁测试。

### 8.2 里程碑（建议拆分为独立增量）
1. [ ] F1：任职类型（primary/matrix/dotted）启用 + 最小测试集 + Authz 对齐
2. [ ] F2：计划任职/原子转任职（`assignment.transfer`）+ dry-run/可回滚
3. [ ] F3：多段任职事件回归（同一员工同一职位多段任职）+ 对账查询/导出入口
4. [ ] F4：历史更正审计增强（reason/回放）+ readiness 记录（对齐 059）

## 9. 测试与验收标准 (Acceptance Criteria)
- **正确性**：
  - 主任职互斥可复现：同一 subject 同窗只能存在一个 primary。
  - 转任职原子性：同一 request 内完成“截断旧任职 + 创建新任职”，且审计/outbox 一致。
  - 多段任职可追溯：同一员工在同一职位多段任职可被查询并可用于 vacancy/time-to-fill 推导。
- **门禁**：
  - 触发的门禁按 `AGENTS.md` 执行；涉及 Authz 必跑 `make authz-test && make authz-lint`；如触发 schema 变更按 021A 执行。

## 10. 运维与监控 (Ops & Monitoring)
- **Feature Flag（灰度）**：
  - `EnableOrgExtendedAssignmentTypes`：控制非 primary 类型写入（默认关闭）。
  - `EnableOrgAssignmentTransfer`（建议新增）：控制 `assignment.transfer`（默认 shadow，逐租户灰度）。
- **结构化日志**：对 `assignment.transfer`、非 primary 写入、冻结窗口拒绝路径输出 `tenant_id, pernr, subject_id, assignment_type, effective_date, error_code`。
- **回滚策略**：优先通过关闭 feature flag 禁用新能力；必要时通过 batch + rescind 回退错误的计划任职（对齐 059 的收口策略）。
