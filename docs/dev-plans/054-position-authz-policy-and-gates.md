# DEV-PLAN-054：Position 权限（Authz）与策略门禁（对齐 051 阶段 C-Authz）

**状态**: 草拟中（2025-12-20 04:40 UTC）

## 0. 评审结论（已采纳）
- v1 固化 `read/write/assign/admin`（不引入新 action），并对齐 Org API（026）现有口径。
- v1 不提供“按用户绕过冻结窗口”的能力：冻结窗口为租户级策略（对齐 025），任何角色均受其约束；如需 break-glass 另起计划评审。
- 统计/报表、主数据（Catalog/Profile/Restrictions）与治理（freeze/reason codes）采用独立 object，避免能力耦合与越权扩散。
- 角色采用最小集 + 显式授权（不做 role-to-role 继承），保持策略碎片可读、可审计、易回归。
- 测试分两层：Casbin 决策用 fixtures 锁定；端点映射与 403 契约用 controller 测试锁定（对齐既有 HRM/Authz 测试风格）。

## 1. 背景与上下文 (Context)
- **需求来源**：[DEV-PLAN-050](050-position-management-business-requirements.md) §9（业务能力边界清单）；[DEV-PLAN-051](051-position-management-implementation-blueprint.md) 阶段 C-Authz。
- **对齐目标**：复用 Org 已落地的 Casbin 口径与 403 契约（见 [DEV-PLAN-026](026-org-api-authz-and-events.md)），把 Position/Assignment 的“可见性 + 可操作性 + 强治理能力（Correct/Rescind/ShiftBoundary/冻结窗口）”固化为可审计、可复现的策略碎片与门禁。
- **关键约束**：避免 UI 权限/按钮显隐与 API 鉴权口径漂移；避免为 Position 单独发明一套 action 语义（保持与 Org API 一致的 `read/write/assign/admin`）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **能力覆盖**：object/action 覆盖 050 §9 的能力清单，并明确哪些能力属于 `admin`（强治理）边界。
- [ ] **API 鉴权落地**：053 的 Position/Assignment v1 API 入口逐一接入 `ensureOrgAuthz`（对齐 026），鉴权失败统一返回 `modules/core/authzutil.ForbiddenPayload`（含 missing_policies / 申请入口 / debug_url）。
- [ ] **策略碎片可复现**：仅修改 `config/access/policies/**` 策略碎片；聚合产物由 `make authz-pack` 生成（禁止手改 `config/access/policy.csv*`）。
- [ ] **门禁可通过**：本计划落地后可通过 `make authz-test && make authz-lint`（以及命中 Go 代码时的仓库 Go 门禁）。
- [ ] **最小越权测试集**：至少覆盖“无权限读不到/写不了”“不能 Correct/Rescind/ShiftBoundary”“不能读统计（如拆分为独立 object）”三类越权路径，并能定位到缺失的 object/action。

### 2.2 非目标（Out of Scope）
- 不在本计划内引入“按 OrgNode 范围”的 ABAC/行级权限模型（如需做，需单独立项并评审 Casbin matcher/attrs 或服务层 org-scope 方案）。
- 不在本计划内落地 Job Catalog / Job Profile / Position Restrictions 的功能实现（见 [DEV-PLAN-056](056-job-catalog-profile-and-position-restrictions.md)）；本计划只定义其 Authz 命名与边界，避免后续漂移。
- 不在本计划内改造 Authz 工具链（pack/bot/draft API），仅按 SSOT 流程使用。

## 2.3 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器/工具链”，避免复制命令细节导致 drift；具体命令以 `AGENTS.md`/`Makefile`/`.github/workflows/quality-gates.yml` 为准。

- **触发器清单（勾选本计划命中的项）**：
  - [ ] Go 代码（Position/Assignment API/controller/service 接入鉴权与测试）
  - [ ] Authz（策略碎片 + pack + 门禁）
  - [ ] 路由治理（若新增 `/org/api/positions*` 等路由需对齐 018）
  - [ ] 文档（本计划文档变更；实现阶段如新增 runbook/record 需对齐 doc gate）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Authz Bot：`docs/runbooks/AUTHZ-BOT.md`
  - Authz Draft API：`docs/runbooks/authz-policy-draft-api.md`
  - Org API 鉴权/403 契约：`docs/dev-plans/026-org-api-authz-and-events.md`
  - 路由治理（如命中）：`docs/dev-plans/018-routing-strategy.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  UI[UI/脚本/集成] --> API[/org/api/positions & assignments/]
  API --> Authz[ensureOrgAuthz<br/>ForbiddenPayload]
  Authz --> Casbin[pkg/authz + policy.csv]
  Authz --> Draft[/core/api/authz/requests<br/>/core/api/authz/debug/]
  API --> S[Org Staffing Services (053)]
  S --> DB[(org_positions/org_assignments)]
  S --> Audit[(org_audit_logs)]
```

### 3.2 关键设计决策（必须冻结）
1. **action 语义（选定：复用 Org）**
   - `read`：读/列表/as-of/时间线（含历史/未来）与只读导出。
   - `write`：创建/更新（Update）/生命周期状态变更/组织转移/reports-to 等“业务变更”。
   - `assign`：Assignment 写入（占用/释放、计划任职等 v1 范围内的写操作）。
   - `admin`：强治理能力：Correct/Rescind/ShiftBoundary 等高风险治理入口（v1 不提供“绕过冻结窗口”的按用户特权；冻结窗口为租户级策略，对所有角色生效）。
2. **object 命名（选定：`org.<resource>`）**
   - 使用 `authz.ObjectName("org", "<resource>")`（与 026 一致），并将资源名保持为小写 + snake_case。
3. **domain 策略（选定：tenant 为主，policy dom 用 `*`）**
   - 请求 domain：`authz.DomainFromTenant(tenantID)`（由 `authzutil.DomainFromContext` 推导）。
   - 策略 domain：对 tenant 内通用角色使用 `*`，避免每租户重复写策略；仅“全局角色/调试能力”使用 `global`。
4. **“可分配但不能分配”的权限悖论（选定：对齐 026）**
   - Assignment 的写入（包括触发 auto/system position 的场景）仅要求 `org.assignments assign`，不额外要求 `org.positions write`；系统创建的 Position 通过 `is_auto_created`/System 标记区分并默认只读。
5. **角色层级实现（选定：显式授权，不做继承）**
   - 概念上 `viewer ⊂ editor ⊂ admin`；落地时用显式 `p` 条目展开（避免 role-to-role 的 `g/g2` 继承带来的隐式授权与排障复杂度）。

## 4. 权限模型与契约 (Security & Authz)
### 4.1 Casbin 请求形状（v1 约定）
- **Subject**：`tenant:{tenant}:user:{uuid}`（由 `modules/core/authzutil.SubjectForUser` 生成）。
- **Domain**：`{tenant_uuid}`（默认）；仅非租户场景才为 `global`。
- **Object**：`org.positions` / `org.assignments` / ...（见下表）。
- **Action**：`read/write/assign/admin`（统一走 `authz.NormalizeAction`）。

### 4.2 object/action 覆盖矩阵（对齐 050 §9）
> 目的：把“业务能力清单”落到可执行的 object/action；后续实现必须与 053 的 API 入口逐项对齐并补齐测试。

| 050 业务能力 | Object | Action | 说明 |
| --- | --- | --- | --- |
| 读取职位信息（含 as-of / 列表 / 详情） | `org.positions` | `read` | 包含历史/未来只读视图 |
| 创建/更新职位（Update） | `org.positions` | `write` | 常规业务变更（新时间片） |
| 生命周期操作（激活/停用/转移等） | `org.positions` | `write` | 具体操作是否拆端点以 053 合同为准 |
| 读取历史/未来生效信息（时间线） | `org.positions` | `read` | UI 的时间线展示不得绕过鉴权 |
| 修改历史版本（Correct/ShiftBoundary） | `org.positions` | `admin` | 强治理：默认严格授权 |
| 撤销（Rescind） | `org.positions` | `admin` | 强治理：需可追溯审计 |
| 读取任职/占用信息 | `org.assignments` | `read` | |
| 任职写入（占用/释放/计划任职） | `org.assignments` | `assign` | 与 026 的 `org.assignments assign` 语义对齐 |
| 修改历史任职（Correct/ShiftBoundary/Rescind） | `org.assignments` | `admin` | 强治理：默认严格授权 |
| 读取编制统计与分析报表 | `org.position_reports` | `read` | v1 选定独立 object，避免“能读明细=默认能读统计” |
| 读取 Job Catalog（用于表单/过滤/展示） | `org.job_catalog` | `read` | 若主数据落为独立端点/读模型，读权限需明确 |
| 维护 Job Catalog（启停/层级治理） | `org.job_catalog` | `admin` | 由 056 落地具体端点与策略碎片 |
| 读取 Job Profile（用于表单/过滤/展示） | `org.job_profiles` | `read` | |
| 维护 Job Profile（绑定/允许集合治理） | `org.job_profiles` | `admin` | 由 056 落地 |
| 读取 Position Restrictions（用于解释拒绝原因/展示摘要） | `org.position_restrictions` | `read` | |
| 维护 Position Restrictions | `org.position_restrictions` | `admin` | 由 056 落地 |
| 读取冻结窗口/Reason Codes（治理配置只读） | `org.governance` | `read` | v1 仅预留 object；如无端点可先不落策略 |
| 维护冻结窗口策略与 reason code | `org.governance` | `admin` | 若后续提供维护入口，统一归到治理 object（避免散落） |

### 4.3 Endpoint → object/action 映射（以 053 合同为准，落地时逐项勾选）
- [ ] `GET /org/api/positions` → `org.positions read`
- [ ] `GET /org/api/positions/{id}` → `org.positions read`
- [ ] `GET /org/api/positions/{id}/timeline`（或等价设计）→ `org.positions read`
- [ ] `POST /org/api/positions` → `org.positions write`
- [ ] `PATCH /org/api/positions/{id}` → `org.positions write`
- [ ] `POST /org/api/positions/{id}:transfer`（或等价）→ `org.positions write`
- [ ] `POST /org/api/positions/{id}:correct|:rescind|:shift-boundary` → `org.positions admin`
- [ ] `GET /org/api/assignments` → `org.assignments read`
- [ ] `POST/PATCH /org/api/assignments*`（占用/释放/计划任职等 v1）→ `org.assignments assign`
- [ ] `POST /org/api/assignments/{id}:correct|:rescind|:shift-boundary` → `org.assignments admin`
- [ ] `GET /org/api/position-reports*`（或等价）→ `org.position_reports read`

## 5. 策略碎片与角色设计（Policy Design）
### 5.1 策略文件组织（建议）
- [ ] 在 `config/access/policies/org/` 下新增 `staffing.csv`，用于集中管理 Position/Assignment/Reporting/Governance 相关策略；避免把所有条目挤在 `org.csv` 里。
- [ ] 保留 `org.csv` 作为 Org 主干能力（nodes/edges/roles/...）的策略碎片；staffing 相关能力独立演进。

### 5.2 v1 角色（建议最小集，按最小权限拆分）
> 角色名仅约定字符串；实际授予方式可通过 Authz Draft API / Bot / 预置 seed 完成。

- [ ] `role:org.staffing.viewer`（读）：`org.positions read`、`org.assignments read`、`org.job_catalog read`、`org.job_profiles read`、`org.position_restrictions read`
- [ ] `role:org.staffing.editor`（业务写）：`org.positions read/write`、`org.assignments read/assign`、`org.job_catalog read`、`org.job_profiles read`、`org.position_restrictions read`
- [ ] `role:org.staffing.admin`（强治理）：`org.positions read/write/admin`、`org.assignments read/assign/admin`、`org.job_catalog read`、`org.job_profiles read`、`org.position_restrictions read`
- [ ] `role:org.staffing.reports`（统计读）：`org.position_reports read`（建议与 viewer 分离；需要 drill-down 时叠加授予 viewer）
- [ ] `role:org.staffing.masterdata.admin`（主数据治理，056 用）：`org.job_catalog read/admin`、`org.job_profiles read/admin`、`org.position_restrictions read/admin`
- [ ] `role:org.staffing.governance.admin`（治理配置，056 用）：`org.governance read/admin`

### 5.3 最小策略草案（落地时生成/评审）
- [ ] 为上述角色补齐 `p, role:..., <object>, <action>, *, allow` 条目（domain 选 `*`；除 `role:core.superadmin` 外不使用 `*` action）。
- [ ] 保持 `role:core.superadmin` 的兜底能力不变（`*`/`*`/`*`），确保调试与回归不被阻断。
- [ ] v1 不引入 role-to-role 继承（不新增 `g/g2` 角色链），用显式 `p` 条目展开 viewer/editor/admin 关系，降低排障成本。

## 6. 测试与验收标准 (Acceptance Criteria)
### 6.1 自动化测试（最小集）
- [ ] **Authz fixtures（策略决策锁定）**：在 `config/access/fixtures/testdata.yaml` 增补 viewer/editor/admin/reports 等角色的 allow/deny 用例，确保 `make authz-lint` 能回归新 object/action。
- [ ] **Controller 级鉴权测试（端点映射 + 403 契约）**：无用户/租户时返回 403，且 `ForbiddenPayload.object/action/missing_policies` 与端点映射一致（对齐既有 HRM/Authz helper 测试风格）。
- [ ] **Enforce 模式下的拒绝回归**：对需要“真 403”的用例（如 `write`≠`admin`），测试中将 Authz mode 强制为 `enforce`（建议用临时 `AUTHZ_FLAG_CONFIG` + `authz.Reset()`，避免受仓库默认 `shadow` 影响）。
- [ ] **强治理测试**：具备 `write` 但无 `admin` 时，Correct/Rescind/ShiftBoundary 必须 403（并可申请）。
- [ ] **统计隔离测试**：具备 `org.positions read` 但无 `org.position_reports read` 时，统计端点 403（v1 已选定独立 reports object）。
- [ ] **assign vs admin**：具备 `assign` 但无 `admin` 时，允许占用/释放但不允许更正历史。

### 6.2 门禁（执行时填写）
- [ ] `make authz-test && make authz-lint` 通过，并记录时间戳与结果到 059 指定的 readiness 记录中。

## 7. 运维与排障 (Ops & Monitoring)
- **排障入口**：403 必须返回 `ForbiddenPayload`（含 `request_id`、`missing_policies`、`request_url`、`debug_url`、`base_revision`），保证“看见拒绝 → 能自助申请/调试”闭环。
- **shadow → enforce 迁移**：仓库默认可能处于 `shadow`；在准备切到 `enforce` 前，先通过 missing policies 与日志定位缺口，再补齐策略碎片与 fixtures，避免直接切换导致写入口大面积 403。
- **回滚策略**：策略变更只通过 `config/access/policies/**` 回滚（不要手改聚合文件）；必要时 revert 对应 PR 并重新 `make authz-pack` 生成聚合产物。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - [DEV-PLAN-052](052-position-contract-freeze-and-decisions.md)：口径与强治理边界冻结（哪些操作算 admin）。
  - [DEV-PLAN-053](053-position-core-schema-service-api.md)：API 入口与错误码/403 契约落地（Authz 接入点）。
  -（可选）[DEV-PLAN-056](056-job-catalog-profile-and-position-restrictions.md)：主数据与治理入口的 object 扩展。
- **里程碑**：
  1. [ ] object/action 与端点映射冻结（§4.2/§4.3）。
  2. [ ] 策略碎片落地 + pack 产物可复现（§5）。
  3. [ ] API 接入 ensureOrgAuthz + 403 契约回归（§2.1）。
  4. [ ] 最小越权测试集通过（§6.1）。
  5. [ ] readiness 记录补齐（对齐 059）。

## 9. 实施步骤
1. [ ] 冻结 object/action 与端点映射（对齐 050 §9、026 口径与 053 API）
2. [ ] 落地策略碎片（新增 `config/access/policies/org/staffing.csv` 或等价文件）
3. [ ] 执行 `make authz-pack` 并确保生成物提交（禁止手改聚合文件）
4. [ ] 在 Position/Assignment API controller 中统一接入 `ensureOrgAuthz`，并对齐 403 ForbiddenPayload 契约
5. [ ] 补齐最小越权测试用例集（覆盖 read/write/assign/admin 的边界）
6. [ ] 执行门禁并把命令/结果/时间戳记录到 059 指定的 readiness 文档

## 10. 交付物
- Position/Assignment/Reporting/Governance 的 object/action 定义与策略碎片（含最小角色集合）。
- 053 的 API 鉴权接入（403 契约一致），以及最小越权测试集。
- 可复现生成的策略聚合产物（`policy.csv`/`.rev`）与门禁记录（由 059 收口）。
