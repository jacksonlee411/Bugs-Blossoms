# DEV-PLAN-031：Org 数据质量与修复（Step 11）

**状态**: 已完成（2025-12-19）— 已落地 `org-data quality` 与 Readiness 记录（PR #87）
**对齐更新**：
- 2025-12-27：对齐 DEV-PLAN-064：Valid Time / as-of 一律按天（`YYYY-MM-DD`）语义；示例不再使用 RFC3339 timestamp 表达生效日。

## 0. 进度速记
- 本计划聚焦“数据质量规则/报告 + 可回滚的批量修复工具”，为 Org 进入长期运行提供最小治理闭环。
- 修复工具默认 **dry-run**，任何写入必须显式 `--apply` + `--yes` 二次确认，并生成 manifest 以支持回滚。
- 已落地：`org-data quality check/plan/apply/rollback`、质量规则 v1（`ORG_Q_001`~`ORG_Q_009`）、以及 `docs/dev-records/DEV-PLAN-031-READINESS.md`。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` **步骤 11：强化数据质量**。
- **依赖链路**：
  - `docs/dev-plans/021-org-schema-and-constraints.md`：DB 约束（no-overlap、FK、ltree path/depth 触发器）提供“硬底座”，但无法覆盖业务口径（编码格式、叶子需岗位等）。
  - `docs/dev-plans/024-org-crud-mainline.md`、`docs/dev-plans/025-org-time-and-audit.md`：主链写语义与 Correct/Rescind；修复必须复用这些稳定接口与错误码。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：`/org/api/batch`、`/org/api/snapshot` 与 `subject_id` 映射 SSOT；修复执行与回滚必须对齐。
  - `docs/dev-plans/030-org-change-requests-and-preflight.md`：变更草稿/预检能力；本计划将质量修复与 preflight/变更记录对齐，确保“可解释、可审计”。
- **当前痛点**：
  - 仅依赖 DB 约束无法发现“软规则”问题（编码/口径/叶子岗位等），数据漂移难以提前暴露。
  - 缺少统一的质量报告格式，无法跨环境对比、无法作为 Readiness/回归基线。
  - 线上数据修复缺少“默认不写 + 可回滚”的安全网，容易形成二次污染。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [X] **质量规则（v1）**：覆盖以下最小口径并给出稳定 rule_id（见 §6.2）：
    - 编码格式（regex）检查（node/position）。
    - “必填项/结构完整性”检查：node 必须有 slice/edge；root 规则；孤儿/多 root/parent=null 异常等。
    - 叶子节点需岗位（as-of 视图下 leaf 必须存在 active position）。
    - assignment 的 `subject_id` 映射一致性（SSOT：026），以及可安全自动修复的 whitespace/映射漂移问题。
  - [X] **质量报告（SSOT）**：定义稳定 JSON 输出格式 `org_quality_report.v1`（见 §4.1），用于：
    - CI/Readiness 留档（`docs/dev-records/DEV-PLAN-031-READINESS.md`）
    - 运维对账（跨环境 diff）
    - 修复工具生成 fix plan 的输入
  - [X] **批量修复工具（v1）**：
    - [X] 支持从报告生成 **fix plan**（仅包含“可自动修复且可回滚”的问题）。
    - [X] 支持 `dry-run`（默认）与 `--apply` 执行；执行入口复用 026 的 `POST /org/api/batch`（避免绕过审计/outbox/冻结窗口）。
    - [X] 每次执行必须生成 **fix manifest**（见 §4.3），并提供 `rollback` 子命令按 manifest 逆向回滚。
  - [X] **门禁与可复现**：本计划落地后至少满足本仓库门禁，并在 Readiness 记录中可重复执行（见 §9）。

- **非目标 (Out of Scope)**：
  - 不引入新的 Org 表结构（如需持久化质量结果到 DB，必须另起计划并走 Org Atlas+Goose 工具链）。
  - 不扩展 Org 主链写语义与错误码（以 024/025 为 SSOT；本计划只复用）。
  - 不解决闭包表/深读优化（归属 029），也不建设长期监控压测体系（归属 034）。
  - 不自动修复“需要业务判断”的问题（例如 code 变更、结构重组、缺岗补齐的人岗匹配等）：只报告 + 给出建议操作。

### 2.1 工具链与门禁（SSOT 引用）
> **目的**：避免在 dev-plan 内复制门禁命令导致 drift；这里只声明“本计划命中哪些触发器”，命令细节以 SSOT 为准。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（质量检查/修复 CLI、规则实现、测试）
  - [X] 文档 / Readiness（新增 `DEV-PLAN-031-READINESS` 与示例报告/说明）
  - [ ] DB 迁移 / Schema（v1 不新增表；若后续新增需按 021A 走 Org Atlas+Goose）
  - [ ] Authz（不新增 policy；但修复执行会调用 `org.batch admin`，需确保策略已存在且 026 口径一致）
  - [ ] `.templ` / Tailwind、多语言 JSON、路由治理（本计划不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`（触发器矩阵与本地必跑）
  - `Makefile`（命令入口）
  - `.github/workflows/quality-gates.yml`（CI 门禁定义）
  - `docs/dev-plans/009A-r200-tooling-playbook.md`（工具链复用索引）
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`（如未来触发 Org 迁移/Schema）

### 2.2 与其他子计划的边界（必须保持清晰）
- 021：负责 schema/硬约束；031 只定义“软规则检查与可回滚修复”，不修改 021 的约束口径。
- 024/025：负责写语义与稳定错误码；031 的 fix 必须复用其 Correct/Rescind（不发明新写路径）。
- 026：负责 batch/snapshot/subject_id SSOT；031 的执行面必须复用 `/org/api/batch`，规则 ORG_Q_008 必须复用 026 的映射算法。
- 030：负责 change-requests/preflight；031 允许把 fix 与 change request 绑定，但不改变 030 的状态机语义（仍为“记录/预检，不执行”）。
- 023：定义 `org-data` 导入/回滚工具与 manifest 思路；031 复用同一二进制并延续“默认 dry-run + 可回滚”约束。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  Op[Dev/Ops] --> CLI[org-data quality]
  CLI -->|check| RPT[quality_report.v1.json]
  RPT -->|plan| PLAN[fix_plan.v1.json]
  CLI -->|apply (default dry-run)| API[/org/api/batch/]
  CLI -->|optional preflight| PF[/org/api/preflight/]
  CLI -->|optional record| CR[/org/api/change-requests/]
  API -->|results| MAN[fix_manifest.v1.json]
  MAN -->|rollback| CLI
```

### 3.2 关键设计决策（ADR 摘要）
1. **执行面复用 `/org/api/batch`（选定）**
   - 修复写入必须走 API，以复用 025 冻结窗口、审计与 026 outbox/缓存失效口径；避免 DB 直写绕过治理。
2. **“只自动修复可回滚的问题”（选定）**
   - v1 仅自动修复“assignment `pernr/subject_id` 规范化”这类可逆更正；结构/编码等需要业务判断的问题只报告不自动改。
3. **报告/计划/manifest 全部版本化（选定）**
   - `schema_version` 固定为 `1`；任何字段演进必须 bump 版本并保持后向兼容解析策略。
4. **默认 dry-run（选定）**
   - `check` 永不写库；`apply` 默认 `dry_run=true`（走 batch 的 dry-run）；显式 `--apply --yes` 才允许真正写入。
5. **回滚以 manifest 为唯一事实源（选定）**
   - apply 前读取“变更前快照”，写入 manifest；rollback 仅依赖 manifest 生成逆向 batch，避免“依赖当前库状态猜测回滚”。

## 4. 数据契约 (Data Contracts)
> 本节定义质量报告、修复计划与回滚 manifest 的 SSOT；API/batch 形状以 026 为准。

### 4.1 `org_quality_report.v1`（质量报告）
- 文件名建议：`org_quality_report_<tenant>_<asof>_<run_id>.json`
- 目的：作为“发现问题”的唯一输出；同时可作为 fix plan 的输入。

**Schema（v1）**
```json
{
  "schema_version": 1,
  "run_id": "uuid",
  "tenant_id": "uuid",
  "as_of": "2025-03-01",
  "generated_at": "2025-03-01T12:00:00Z",
  "ruleset": { "name": "org-quality", "version": "v1" },
  "summary": {
    "errors": 0,
    "warnings": 0,
    "issues_total": 0
  },
  "issues": [
    {
      "issue_id": "uuid",
      "rule_id": "ORG_Q_008_ASSIGNMENT_SUBJECT_MAPPING",
      "severity": "error",
      "entity": { "type": "org_assignment", "id": "uuid" },
      "effective_window": { "effective_date": "2025-01-01", "end_date": "9999-12-31" },
      "message": "subject_id mismatch with SSOT mapping",
      "details": { "pernr": " 000123 ", "expected_subject_id": "uuid", "actual_subject_id": "uuid" },
      "autofix": {
        "supported": true,
        "fix_kind": "assignment.correct",
        "risk": "low"
      }
    }
  ]
}
```

约定：
- `severity`：`error|warning`（v1 不引入 info）。
- `issues[]` 默认全量输出；如超过上限（例如 10k），必须截断并在 `summary` 中标记 `truncated=true`（字段可在 v1 预留为可选）。

### 4.2 `org_quality_fix_plan.v1`（修复计划）
- 文件名建议：`org_quality_fix_plan_<tenant>_<asof>_<run_id>.json`
- 目的：把“可自动修复问题”转为 026 batch 的 commands（dry-run/execute 共用）。

**Schema（v1）**
```json
{
  "schema_version": 1,
  "run_id": "uuid",
  "tenant_id": "uuid",
  "as_of": "2025-03-01",
  "source_report_run_id": "uuid",
  "created_at": "2025-03-01T12:01:00Z",
  "batch_request": {
    "dry_run": true,
    "effective_date": "2025-03-01",
    "commands": [
      {
        "type": "assignment.correct",
        "payload": { "id": "uuid", "pernr": "000123", "subject_id": "uuid", "position_id": "uuid" }
      }
    ]
  },
  "maps": {
    "issue_to_command_indexes": { "issue_uuid": [0] }
  }
}
```

约定：
- `batch_request` 必须严格对齐 026 的 batch request（含 type/payload 形状）。
- v1 fix plan 只允许产生 `assignment.correct`（见 §6.3）。

### 4.3 `org_quality_fix_manifest.v1`（执行与回滚清单）
- 文件名建议：`org_quality_fix_manifest_<tenant>_<asof>_<run_id>.json`
- 目的：记录一次 apply 的“输入 + 变更前快照 + 执行结果”，支撑精确 rollback。

**Schema（v1）**
```json
{
  "schema_version": 1,
  "run_id": "uuid",
  "tenant_id": "uuid",
  "as_of": "2025-03-01",
  "applied_at": "2025-03-01T12:02:00Z",
  "source_fix_plan_run_id": "uuid",
  "change_request_id": "uuid|null",
  "batch_request": { "...": "same as 026 batch request, dry_run=false" },
  "before": {
    "assignments": [
      { "id": "uuid", "pernr": " 000123 ", "subject_id": "uuid", "position_id": "uuid" }
    ]
  },
  "results": {
    "ok": true,
    "events_enqueued": 1,
    "batch_results": [
      { "index": 0, "type": "assignment.correct", "ok": true, "result": { "assignment_id": "uuid" } }
    ]
  }
}
```

约定：
- `before.*` 只记录本次会被更正的字段最小集（避免写入不必要的 PII）。
- rollback 必须只依赖 manifest 生成逆向 batch（见 §6.5）。

## 5. 接口契约 (CLI Contracts)
> CLI 以 023 的 `org-data` 为基础扩展（避免引入新的二进制与配置漂移）。

### 5.1 子命令
- `org-data quality check --tenant <uuid> [--as-of <YYYY-MM-DD|rfc3339>] [--backend db|api] [--output <dir>] [--format json]`
  - 默认：`--as-of todayUTC`（兼容期允许 RFC3339 但会归一化为 UTC date），`--backend db`，输出 `org_quality_report.v1.json`（见 §4.1）。
- `org-data quality plan --report <path> --output <path> [--max-commands 100]`
  - 只为可 autofix 的 issue 生成 fix plan（见 §4.2）；超出上限必须失败（exit=2）。
- `org-data quality apply --fix-plan <path> [--dry-run] [--apply] [--yes] [--change-request-id <uuid>]`
  - 默认 `--dry-run=true`（调用 batch dry-run）；显式 `--apply --yes` 才落库并生成 manifest（见 §4.3）。
- `org-data quality rollback --manifest <path> [--dry-run] [--apply] [--yes]`
  - 读取 manifest，生成逆向 batch；默认 dry-run。

### 5.2 退出码（v1 约定）
- `0`：成功（check/plan/apply/rollback 的 dry-run 或 apply 成功）
- `2`：输入/校验错误（报告/计划 schema 不合法、规则校验失败、参数冲突等）
- `3`：使用错误（缺参数、不支持组合，例如 `apply` 未提供 `--yes`）
- `4`：DB/API 连接错误
- `5`：执行失败（batch 或 rollback 失败；返回的稳定错误码应在 stderr/JSON 摘要中可见）

### 5.3 输出约定
- stdout：打印“一行 JSON 摘要”（包含 `run_id/tenant_id/as_of/paths`），便于脚本采集。
- stderr：打印结构化日志（必须包含 `run_id/tenant_id/subcommand/dry_run/apply`）。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 `quality check`：检查管线（v1）
1. 解析 `tenant_id` 与 `as_of`（UTC）。
2. 读取数据（backend 选择）：
   - `db`：直接查询 Org 表（必须注入 tenant RLS 对齐 019A/023）。
   - `api`：调用 026 的 `GET /org/api/snapshot?effective_date=...&include=...`（更贴近线上真实读口径）。
3. 按 rule 顺序执行检查，生成 `issues[]`：
   - 每条 issue 必须包含 `rule_id/severity/entity/id/message`；
   - 若支持 autofix，必须生成 `autofix.supported=true` 并填充最小 `details`（供 plan/fix 使用）。
4. 汇总 `summary` 并写出 `org_quality_report.v1`。

### 6.2 质量规则（v1 SSOT）
> 规则 id 必须稳定；新增/调整规则必须更新本节并在 Readiness 中留档对比。

#### ORG_Q_001_NODE_CODE_FORMAT（warning，manual）
- **定义**：`org_nodes.code` 必须匹配 regex：`^[A-Z0-9][A-Z0-9_-]{0,63}$`。
- **检查**：扫描 `org_nodes`（全量，不按 as-of）。
- **自动修复**：不支持（code 作为稳定标识，修改需要业务确认）。

#### ORG_Q_002_POSITION_CODE_FORMAT（warning，manual）
- **定义**：`org_positions.code` 必须匹配：
  - 自动岗：`^AUTO-[0-9A-F]{16}$`（对齐 024 的生成逻辑）
  - 或通用：`^[A-Z0-9][A-Z0-9_-]{0,63}$`
- **检查**：扫描 `org_positions`（全量）。
- **自动修复**：不支持。

#### ORG_Q_003_ROOT_INVARIANTS（error，manual）
- **定义**（as-of）：
  - 存在且仅存在一个 `org_nodes.is_root=true` 的节点；
  - root 在 as-of 必须存在 node slice（name/status 等）；
  - root 在 as-of 必须存在 edge slice，且 `parent_node_id is null`。
- **自动修复**：不支持（涉及结构语义）。

#### ORG_Q_004_NODE_MISSING_SLICE_ASOF（error，manual）
- **定义**（as-of）：任一 `org_nodes` 在 as-of 不存在覆盖该时点的 `org_node_slices`。
- **自动修复**：不支持（缺失属性需人工补齐；可通过 024/025 写入）。

#### ORG_Q_005_NODE_MISSING_EDGE_ASOF（error，manual）
- **定义**（as-of）：非 root 节点在 as-of 不存在覆盖该时点的 `org_edges`（孤儿）。
- **自动修复**：不支持（需要结构修复，可能涉及 move/correct-move）。

#### ORG_Q_006_EDGE_PARENT_NULL_FOR_NON_ROOT（error，manual）
- **定义**（as-of）：存在 edge slice 满足 `parent_node_id is null` 且 child 不是 root。
- **自动修复**：不支持。

#### ORG_Q_007_LEAF_REQUIRES_POSITION_ASOF（warning，manual）
- **定义**（as-of）：对满足以下条件的节点，必须存在至少 1 条 active 的 position：
  - node 在 as-of `status=active`
  - node 在 as-of 为 leaf（无子节点）
  - `exists org_positions where org_node_id=node_id and status=active and position_window contains as_of`
- **自动修复（v1）**：不支持（缺岗补齐需要业务判断；可在后续阶段引入“占位岗位创建”专用接口/脚本再启用）。

#### ORG_Q_008_ASSIGNMENT_SUBJECT_MAPPING（error，autofix）
- **定义**：对每条 `org_assignments`：
  - `pernr_trim = strings.TrimSpace(pernr)`
  - `expected_subject_id = person_uuid`（SSOT：026 §7.3）
    - resolve：`SELECT person_uuid FROM persons WHERE tenant_id=$1 AND pernr=$2`
  - 要求：
    - `pernr == pernr_trim`
    - `subject_type == 'person'`
    - `subject_id == expected_subject_id`
- **自动修复**：
  - 生成 `assignment.correct`：
    - `pernr = pernr_trim`
    - `subject_id = expected_subject_id`
    - `position_id` 复用当前行的 `position_id`
  - **限制**：
    - 仅当该 assignment slice 的 `assignment_type='primary'`（M1）且能读取到 `position_id` 时才生成；否则降级为 manual issue。
    - 若无法从 `persons` 表 resolve `pernr -> person_uuid`（人员不存在或未具备 Person SOR），则降级为 manual issue（不做 autofix，避免写入错误的 `subject_id`）。

#### ORG_Q_009_POSITION_OVER_CAPACITY（error，manual）
- **定义**（as-of）：对每个 position：
  - `capacity_fte` 以 `org_position_slices` 的 as-of 切片为准；
  - `occupied_fte = sum(org_assignments.allocated_fte)`（as-of 且 `assignment_type='primary'`）；
  - 若 `occupied_fte > capacity_fte`，则判定为超编（over capacity）。
- **自动修复（v1）**：不支持（需要业务确认与写链路治理；修复可能涉及任职截断/容量调整/原因码审计）。

### 6.3 `quality plan`：生成 fix plan（v1）
1. 读取并校验 `org_quality_report.v1`。
2. 过滤 `issues`：`autofix.supported=true` 且 `fix_kind=assignment.correct`。
3. 将每条 issue 转换为 026 batch `commands[]`（不超过 `--max-commands`）。
4. 输出 `org_quality_fix_plan.v1`。

### 6.4 `quality apply`：执行修复（v1）
1. 读取并校验 fix plan。
2. （可选）若传入 `--change-request-id`：
   - 调用 030 的 `GET /org/api/change-requests/{id}`，校验其 `payload` 与 fix plan 的 `batch_request` 等价（忽略 `dry_run` 字段差异）。
   - 调用 030 的 `POST /org/api/preflight` 输出影响摘要（写入日志与 manifest）。
3. 生成 manifest 的 `before`：
   - 针对每条 `assignment.correct`，读取当前 `org_assignments` 行的 `pernr/subject_id/position_id`（db backend 或 api backend，二选一但必须可复现）。
4. 调用 026 `POST /org/api/batch`：
   - 默认：`dry_run=true`
   - `--apply --yes`：`dry_run=false`，写入后落地 `org_quality_fix_manifest.v1`。

### 6.5 `quality rollback`：按 manifest 回滚（v1）
1. 读取并校验 manifest。
2. 根据 `before.assignments[]` 生成逆向 batch commands（`assignment.correct` 把字段恢复为 before）。
3. 调用 026 `POST /org/api/batch` 执行（默认 dry-run；显式 `--apply --yes` 才写入）。

## 7. 安全与鉴权 (Security & Authz)
- **Authz（复用 026/030）**：
  - 读取 snapshot（api backend）需要 `org.snapshot admin`。
  - 预检与 batch 执行需要 `org.preflight admin` / `org.batch admin`。
  - 如需要读取/绑定 change request：`org.change_requests admin`。
  - 403 payload 必须对齐 026 的 `ForbiddenPayload`（CLI 需透传并打印关键字段）。
- **租户隔离**：
  - db backend 必须注入 tenant context 并调用 `composables.ApplyTenantRLS`（对齐 019A/023），禁止跨租户扫描。
- **最小化 PII**：
  - 报告与 manifest 默认只输出 pernr（已存在于 Org assignment 的可读标识）；不得输出员工姓名/email 等额外信息。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - `docs/dev-plans/024-org-crud-mainline.md`、`docs/dev-plans/025-org-time-and-audit.md`：Correct/Rescind 口径与稳定错误码（修复必须复用）。
  - `docs/dev-plans/026-org-api-authz-and-events.md`：batch/snapshot 与 subject_id 映射 SSOT。
  - `docs/dev-plans/030-org-change-requests-and-preflight.md`：preflight 与变更记录（可选但推荐纳入执行链路）。
- **里程碑**：
  1. [X] 落地 `org-data quality check` 与 v1 规则集（含报告 JSON）。
  2. [X] 落地 `plan/apply/rollback`（仅 assignment.correct autofix）。
  3. [X] 补齐单元测试与最小集成测试（含 dry-run 与回滚校验）。
  4. [X] 新增 `docs/dev-records/DEV-PLAN-031-READINESS.md`，记录命令/环境/输出摘要。

## 9. 测试与验收标准 (Acceptance Criteria)
- **规则正确性**：
  - 能稳定识别 v1 规则集对应的坏数据样例；issue 的 `rule_id/entity/id` 精确可定位。
  - `ORG_Q_008_ASSIGNMENT_SUBJECT_MAPPING` 的 autofix 在 dry-run 与 apply 下行为一致（除写入副作用），并可通过 rollback 恢复。
- **工程门禁**：
  - 触发 Go 代码时按 `AGENTS.md` 执行 Go/Lint/Test 门禁并通过。
  - 文档变更通过 `make check doc`。
- **可复现**：
  - Readiness 记录包含：`check`、`plan`、`apply(dry-run)`、`apply(--apply)`、`rollback(dry-run)`、`rollback(--apply)` 的命令与输出摘要（敏感信息打码）。

## 10. 运维与降级/回滚 (Ops & Rollback)
- **开关建议（契约）**：
  - `ORG_DATA_QUALITY_ENABLED=true|false`（默认 `false`）：关闭时禁止 `quality apply/rollback`，仅允许 `check`。
  - `ORG_DATA_FIXES_MAX_COMMANDS=100`（默认 `100`）：限制单次 batch 规模（对齐 026 的 commands 上限）。
- **回滚**：
  - 首选：`org-data quality rollback --manifest ... --apply --yes`（精确恢复字段）。
  - 若 batch 执行失败：必须保留 manifest 与 server 返回的 `meta.command_index/type`，便于定位与重试。

## 11. 交付物 (Deliverables)
- `org-data quality` 子命令（check/plan/apply/rollback）与对应测试。
- `org_quality_report.v1` / `fix_plan.v1` / `fix_manifest.v1` 的 SSOT 文档（本计划 §4）。
- `docs/dev-records/DEV-PLAN-031-READINESS.md`（执行记录 + 输出示例）。
