# DEV-PLAN-020L：Org 模块功能目录（用于测试覆盖映射）

**状态**: 草拟中（2025-12-24）— 汇总 DEV-PLAN-020~036 的功能点清单，供测试用例与回归清单对照

## 1. 背景

DEV-PLAN-020~036 将 Org 模块拆分为：Schema/迁移工具链、主链 CRUD、时间治理（Correct/Rescind/ShiftBoundary）、Authz/Outbox/Snapshot/Batch、性能与灰度、继承与权限映射、深读优化、可视化与报表、运维监控、UI 与示例数据集。

测试需要一份“功能 → 用途 → 入口 → 推荐验证层级”的目录，用于：
- 制定覆盖矩阵（unit/integration/http/e2e/perf/ops）
- 做回归清单（功能点不遗漏）
- 快速定位 SSOT（某功能的契约细节以对应 dev-plan 为准）

## 2. 目标与非目标

### 2.1 目标
- 汇总 DEV-PLAN-020~036 范围内的 Org 功能列表（含 API/CLI/UI/运维入口）。
- 为每个功能点给出“用途（为何存在）”与“推荐验证方式”（便于后续补齐测试）。

### 2.2 非目标
- 本文不取代各 dev-plan 的“字段/错误码/算法/门禁”的 SSOT；仅做索引与清单，避免复制导致漂移。
- 本文不覆盖 036 之后的扩展计划（例如 053/056/057/058/059/061 等）。若在 020~036 中被引用，仅作为“后续扩展/外部依赖”标注，不纳入本目录的测试门槛。

## 3. 020~036 计划索引（主题速览）

- 020：组织模块总体蓝图与里程碑（对标 Workday）：`docs/dev-plans/020-organization-lifecycle.md`
- 021：Org 核心表与 DB 约束（时态、ltree、no-overlap、root 规则）：`docs/dev-plans/021-org-schema-and-constraints.md`
- 021A：Org Atlas+Goose 工具链与 CI 门禁：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- 022：占位表（继承/角色/变更请求）与事件契约（topics/change_type/payload）：`docs/dev-plans/022-org-placeholders-and-event-contracts.md`
- 023：导入/导出/回滚工具 `org-data` 与 readiness：`docs/dev-plans/023-org-import-rollback-and-readiness.md`
- 024：主链 CRUD（nodes/edges/assignments，含 auto-position）：`docs/dev-plans/024-org-crud-mainline.md`
- 025：时间约束、冻结窗口与审计（Correct/Rescind/ShiftBoundary/Correct-Move）：`docs/dev-plans/025-org-time-and-audit.md`
- 026：API 鉴权与事件发布（outbox、snapshot、batch、403 合同）：`docs/dev-plans/026-org-api-authz-and-events.md`
- 027：性能基准与灰度发布（query budget、rollout flags、读策略/缓存）：`docs/dev-plans/027-org-performance-and-rollout.md`
- 028：属性继承解析与角色读侧占位：`docs/dev-plans/028-org-inheritance-and-role-read.md`
- 029：闭包表与深读优化（snapshot/closure build、后端切换、回滚/清理）：`docs/dev-plans/029-org-closure-and-deep-read-optimization.md`
- 030：变更请求与预检（draft/submit/cancel + preflight）：`docs/dev-plans/030-org-change-requests-and-preflight.md`
- 031：数据质量与修复闭环（quality check/plan/apply/rollback + 规则集）：`docs/dev-plans/031-org-data-quality-and-fixes.md`
- 032：权限映射与业务关联（security group mappings、links、permission preview）：`docs/dev-plans/032-org-permission-mapping-and-associations.md`
- 033：可视化与高级报告（export、node path、person path、reporting view）：`docs/dev-plans/033-org-visualization-and-reporting.md`
- 034：运维治理与压测（指标、ops health、load report）：`docs/dev-plans/034-org-ops-monitoring-and-load.md`
- 035：Org UI（templ + HTMX）：`docs/dev-plans/035-org-ui.md`
- 035A：Org UI IA 与侧栏集成契约：`docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`
- 036：制造示例组织树数据集（200+ 节点、最深 17 级）：`docs/dev-plans/036-org-sample-tree-data.md`

> Readiness 记录位于 `docs/dev-records/DEV-PLAN-0XX-READINESS.md`（例如 `docs/dev-records/DEV-PLAN-027-READINESS.md`），用于保存手工演练步骤与证据；当自动化测试缺失时，优先以 readiness 作为回归清单来源。

## 4. 功能目录（按能力域）

### 4.1 基础模型与迁移工具链（020/021/021A）

- **多租户隔离与 RLS fail-closed**
  - 用途：防止跨租户数据泄漏；测试与本地/CI 环境保持一致。
  - 入口：DB（所有 `org_*` 表均含 `tenant_id`）、事务内注入（详见 019A/023/024/026）。
  - 推荐验证：集成测试覆盖“无 tenant/跨 tenant 访问被拒绝”、以及“RLS enforce 下必须注入 tenant”。
- **OrgUnit 单树（唯一 root + root edge）**
  - 用途：保证每租户组织结构唯一且可作为 path/depth 计算锚点。
  - 入口：DB 约束（root 唯一、root edge `parent_node_id is null`），API（024 CreateNode）。
  - 推荐验证：DB 约束 + 集成测试（创建 root、重复 root 拒绝、root edge 存在）。
- **时态模型基线（Valid Time）**
  - 用途：支持 as-of 查询、未来排程、历史更正；避免“覆盖式更新”导致审计不可追溯。
  - 入口：DB（`effective_date/end_date` + EXCLUDE no-overlap）、Service（Insert/Correct/Rescind/ShiftBoundary）。
  - 推荐验证：集成测试覆盖 no-overlap 与按天闭区间 `[effective_date,end_date]` 语义（SSOT：DEV-PLAN-064）。
- **层级路径（ltree path/depth）与成环拒绝**
  - 用途：在线查询可避免递归；写入时 DB 兜底拒绝成环。
  - 入口：DB trigger（021），写入顺序要求 parent-before-child（023 seed、024 move 子树重切片）。
  - 推荐验证：迁移 smoke + 集成测试（非法成环拒绝、move 后子树 path/depth 重算正确）。
- **Org Atlas+Goose 工具链（独立门禁）**
  - 用途：确保 Org 迁移可 lint、可 plan、可上/下行、可在 CI 复现。
  - 入口：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`（SSOT）与 `Makefile`。
  - 推荐验证：门禁冒烟（plan/lint/migrate up/down）+ readiness 记录（按 021A 要求）。

### 4.2 数据导入/导出/回滚（023/036）

- **`org-data` CLI：import/export/rollback（默认 dry-run）**
  - 用途：在 024 主链上线前提供“可重复执行、可校验、可回滚”的数据注入路径；降低数据污染风险。
  - 入口：`cmd/org-data`（详见 `docs/dev-plans/023-org-import-rollback-and-readiness.md`）。
  - 推荐验证：
    - 单测：CSV 解析/行号定位、静态校验（环路/重叠/非法日期/非法枚举/非法 JSON）
    - 集成：seed apply 后结构可读、manifest rollback 无残留
- **seed 安全网（空租户限定）**
  - 用途：避免绕过 025/026 的审计/outbox 口径在非空租户“merge 写入”。
  - 入口：`org-data import --mode seed --backend db`（023）。
  - 推荐验证：在非空租户执行应被拒绝（稳定退出码/错误摘要）。
- **`subject_id` 映射一致性（SSOT=026）**
  - 用途：避免多端派生算法漂移；`pernr` 与 `subject_id(person_uuid)` 必须一致。
  - 入口：`org-data`（023）、写 API（024/025/026）、质量规则（031 ORG_Q_008）。
  - 推荐验证：单测覆盖 pernr trim/person 不存在/subject_id 不匹配；集成测试覆盖写入后可被 snapshot/quality 检查验证。
- **036 制造样例数据集**
  - 用途：UI 联调、演示、排障与可重复造数输入；与导入/回滚/对账链路绑定。
  - 入口：`docs/samples/org/036-manufacturing/` + `org-data import/rollback`（036/023）。
  - 推荐验证：dry-run/apply/manifest rollback 演练 + 对账摘要符合 036 DoD（节点数、最大深度、root 名称）。

### 4.3 主链 CRUD（024）

- **组织树读取（as-of）**
  - 用途：为 UI/下游提供稳定的层级读模型；作为性能基准与后续深读切换的基线入口。
  - 入口：`GET /org/api/hierarchies`（024）。
  - 推荐验证：集成测试覆盖排序、depth、错误码（无 session/tenant/非法参数）。
- **节点创建（root/child）**
  - 用途：建立组织结构主数据；root 规则与 root edge 约束为全链路基础。
  - 入口：`POST /org/api/nodes`（024）。
  - 推荐验证：root/child 创建成功、父节点不存在拒绝、code 冲突拒绝、manager 解析失败拒绝。
- **节点更新（Insert 时间片语义）**
  - 用途：允许未来排程，不覆盖历史；与 025 Correct 明确分界。
  - 入口：`PATCH /org/api/nodes/{id}`（024）。
  - 推荐验证：`effective_date` 等于当前片段起点时返回 `ORG_USE_CORRECT`；时间片截断与新片段写入正确。
- **变更上级（MoveNode：Insert edge + 子树重切片）**
  - 用途：结构变更需保证子树 path/depth 正确；为后续深读/报表/权限继承提供一致输入。
  - 入口：`POST /org/api/nodes/{id}:move`（024）。
  - 推荐验证：移动 root 拒绝；移动后子树 path/depth 在 as-of 时刻一致；`ORG_USE_CORRECT_MOVE` 分支被正确引导到 025 correct-move。
- **人员分配读取（时间线/as-of）**
  - 用途：提供 subject 的任职时间线与 as-of 视图；供 UI、报表 person-path、质量规则使用。
  - 入口：`GET /org/api/assignments`（024）。
  - 推荐验证：subject 格式、as-of 过滤、返回包含 `assignment_type`（028 约束：允许出现 matrix/dotted）。
- **人员分配写入（含自动空壳 Position）**
  - 用途：以 Position 为锚点承载任职；M1 允许“只给 org_node_id 也可创建主分配”，系统自动补齐 Position。
  - 入口：`POST /org/api/assignments`、`PATCH /org/api/assignments/{id}`（024）。
  - 推荐验证：
    - `assignment_type` 写入限制（M1 仅 primary；否则 `ORG_ASSIGNMENT_TYPE_DISABLED`）
    - primary 唯一冲突（`ORG_PRIMARY_CONFLICT`）
    - auto-position 的确定性与并发去重（同 tenant/node/subject 产生同一 position）

### 4.4 时间治理、冻结窗口与审计（025）

- **冻结窗口（org_settings：disabled/shadow/enforce + grace_days）**
  - 用途：控制“是否允许修改历史月份”；shadow 用于观测与灰度，不允许静默绕过。
  - 入口：`org_settings`（025）+ 所有写路径（024/025/026 batch）。
  - 推荐验证：cutoff 计算口径与 `ORG_FROZEN_WINDOW` 拒绝行为（enforce）。
- **审计日志（org_audit_logs）**
  - 用途：事务时间可追溯、原因码治理、与事件（outbox）/预检/质量修复闭环对齐。
  - 入口：写路径（025 约束：仅成功提交后产出）、后续由 outbox/工具消费。
  - 推荐验证：失败回滚不写审计；成功写入包含 `request_id/initiator_id/transaction_time/change_type`。
- **Correct（原位更正）**
  - 用途：修正历史片段的非时间字段；与 Insert 明确区分以避免误用。
  - 入口：`POST /org/api/nodes/{id}:correct`、`POST /org/api/assignments/{id}:correct`（025）。
  - 推荐验证：定位覆盖 as-of 的片段、拒绝修改时间字段、subject_id 一致性校验（026 SSOT）。
- **Rescind（撤销）**
  - 用途：撤销误创建/误更新；M1 保守策略避免破坏结构与下游依赖。
  - 入口：`POST /org/api/nodes/{id}:rescind`、`POST /org/api/assignments/{id}:rescind`（025）。
  - 推荐验证：root 不可 rescind、叶子节点/无依赖约束、`ORG_INVALID_RESCIND_DATE`。
- **ShiftBoundary（边界移动）**
  - 用途：调整相邻片段交界线（用于修复有效期边界错误）。
  - 入口：`POST /org/api/nodes/{id}:shift-boundary`（025）。
  - 推荐验证：倒错/吞没错误码（`ORG_SHIFTBOUNDARY_INVERTED/ORG_SHIFTBOUNDARY_SWALLOW`），双片段更新顺序避免触发 EXCLUDE。
- **Correct-Move（结构原位更正）**
  - 用途：当 move 的 `effective_date` 命中 edge slice 起点时，使用 correct-move 原位修正结构。
  - 入口：`POST /org/api/nodes/{id}:correct-move`（025）。
  - 推荐验证：命中条件校验（`ORG_USE_MOVE`）与子树重切片一致性。

### 4.5 Authz / Outbox / Snapshot / Batch（022/026）

- **事件契约（integration events v1）**
  - 用途：对外发布“组织主数据变更”的稳定契约；下游按 `event_id` 去重、按 `effective_window` 回放。
  - 入口：topics `org.changed.v1`、`org.assignment.changed.v1`、`org.personnel_event.changed.v1`（022）。
  - 推荐验证：payload 字段 snake_case、change_type 枚举、new_values/old_values 口径与 as-of 一致。
- **Transactional Outbox（org_outbox）**
  - 用途：保证业务写入与事件 enqueue 原子一致；relay at-least-once，消费者必须幂等。
  - 入口：DB 表 `public.org_outbox`（026，对齐 017）。
  - 推荐验证：写路径成功时 outbox enqueue；dry-run（batch）不 enqueue；失败回滚不 enqueue。
- **统一 403 Forbidden 合同（ForbiddenPayload）**
  - 用途：权限缺失时可引导申请权限与 debug；UI/HTMX/JSON 输出一致。
  - 入口：`/org/api/**`（026）与 UI（035）。
  - 推荐验证：403 body 字段（object/action/domain/subject/missing_policies/request_id）稳定。
- **Snapshot（as-of 全量纠偏）**
  - 用途：下游纠偏/对账；为质量检查（031）提供 API backend；避免事件 v1 承载完整时间线。
  - 入口：`GET /org/api/snapshot`（026）。
  - 推荐验证：include 裁剪、分页 cursor、排序稳定性、错误码稳定性。
- **Batch（单事务、多指令、支持 dry-run）**
  - 用途：统一写入口；为 change request/预检/质量修复提供“同一校验管线”的执行面。
  - 入口：`POST /org/api/batch`（026）。
  - 推荐验证：原子性（全成全败）、dry-run 无副作用、command 类型与单条接口 SSOT 对齐、限流错误码（too large/too many moves）。

### 4.6 性能、灰度与可观测（027/034）

- **Query Budget 守卫（防 N+1）**
  - 用途：保证树/路径/深读查询 SQL roundtrip 为常数级；作为 CI 硬门槛。
  - 入口：go test（027），以及后续在各关键读路径增加预算测试。
  - 推荐验证：小/大数据集 query count 相同（或差异 ≤ 1）的断言。
- **org-perf 基准与固定数据集**
  - 用途：可重复度量 P50/P95/P99；不达标时进入降级决策树并留档。
  - 入口：`cmd/org-perf` 与 `Makefile`（027）。
  - 推荐验证：DB 基准 P99 < 200ms（027 口径），输出 JSON 报告字段完整。
- **灰度开关（rollout flags）**
  - 用途：按租户启用/下线 Org；读策略与缓存可一键回滚。
  - 入口：环境变量（027：`ORG_ROLLOUT_MODE/ORG_ROLLOUT_TENANTS/ORG_READ_STRATEGY/ORG_CACHE_ENABLED`）。
  - 推荐验证：disabled/allowlist 行为、回滚路径可演练。
- **Prometheus 指标（Org v1）**
  - 用途：在线观测请求量/延迟、缓存命中、写冲突、深读 build 状态。
  - 入口：指标名/label 契约（034）。
  - 推荐验证：不使用高基数 label；关键端点有埋点。
- **Org Ops Health（内部 API）**
  - 用途：提供 outbox 堆积/深读新鲜度/缓存状态等健康信息，区别于全局 `/health`。
  - 入口：`GET /org/api/ops/health`（034）。
  - 推荐验证：200/503 与 status 一致；403 复用 026 forbidden payload；checks 字段稳定。
- **压测报告（org_load_report.v1）**
  - 用途：可对比不同后端/缓存/数据集下的阈值达标情况。
  - 入口：`cmd/org-load`（034）。
  - 推荐验证：报告 schema_version 与 profile/threshold 输出稳定。

### 4.7 继承、角色读侧与权限映射（028/032）

- **属性继承解析（resolved_attributes）**
  - 用途：以规则表驱动属性继承（legal_entity/company/location/manager）；用于权限/报表/排障。
  - 入口：`GET /org/api/hierarchies?include=resolved_attributes`（028）、`org_attribute_inheritance_rules`（022）。
  - 推荐验证：O(N) 解析、来源可解释、未知属性处理策略（忽略 vs 400）。
- **继承解析调试端点**
  - 用途：快速定位“某属性解析值来自哪个祖先节点”，便于灰度/Readiness 排障。
  - 入口：`GET /org/api/nodes/{id}:resolved-attributes`（028）。
  - 推荐验证：404/400/403 行为与返回结构稳定。
- **角色读侧占位（roles / role-assignments）**
  - 用途：为后续组织角色治理提供最小只读接口；支持祖先继承汇总。
  - 入口：`GET /org/api/roles`、`GET /org/api/role-assignments`（028）。
  - 推荐验证：include_inherited 返回 source_org_node_id；filter 行为稳定。
- **安全组映射（security_group_mappings）**
  - 用途：将组织节点映射到“安全组 key”，并支持对子树继承；供 permission preview 与导出/报表复用。
  - 入口：`GET/POST /org/api/security-group-mappings`、`POST ...:rescind`（032）。
  - 推荐验证：as-of 视图、applies_to_subtree 语义、no-overlap、撤销边界校验。
- **业务关联（links）**
  - 用途：将组织节点与外部业务对象（project/cost_center/budget_item/custom）做时态关联。
  - 入口：`GET/POST /org/api/links`、`POST ...:rescind`（032）。
  - 推荐验证：object_type 白名单、metadata JSON、as-of 与撤销语义。
- **权限预览（permission preview）**
  - 用途：给管理员解释“某节点在 as-of 时刻生效的安全组/关联”，并指出来源节点。
  - 入口：`GET /org/api/permission-preview`（032，feature flag 控制）。
  - 推荐验证：继承计算、来源标注、超限 warnings、authz/租户隔离守卫。

### 4.8 深读优化、可视化与报表（029/033）

- **深读后端切换（edges/closure/snapshot）**
  - 用途：在不引入递归 CTE 的前提下提供祖先/子树查询；为路径/报表提供可扩展读模型。
  - 入口：`ORG_DEEP_READ_ENABLED/ORG_DEEP_READ_BACKEND`（029）。
  - 推荐验证：同输入下三种 backend 结果集合一致（差异视为 bug）。
- **build/activate/prune 运维入口**
  - 用途：可重复构建 snapshot/closure build，并可回滚到上一 build；控制数据膨胀。
  - 入口：`Makefile` 中的 `org-closure-*`/`org-snapshot-*`（029，SSOT）。
  - 推荐验证：幂等、失败不影响 active、回滚可演练。
- **组织图导出（JSON）**
  - 用途：稳定对外对接与可视化输入；支持分页与深度/规模限制。
  - 入口：`GET /org/api/hierarchies:export`（033）。
  - 推荐验证：`max_depth/limit/cursor`、错误码（too deep/too large）、include（nodes/edges/可选 security_groups/links）。
- **节点路径查询（root→node）**
  - 用途：在线查询节点祖先链；用于 UI 面包屑、报表解释、排障。
  - 入口：`GET /org/api/nodes/{id}:path`（033）。
  - 推荐验证：深读后端切换一致；SQL 次数常数级。
- **人员路径查询（person→primary assignment→node path）**
  - 用途：从人员定位组织路径（对标 HR 常见“汇报路径/组织归属”查询）。
  - 入口：`GET /org/api/reports/person-path`（033）。
  - 推荐验证：primary assignment 缺失时稳定错误码；不泄露 PII。
- **BI 读模型（org_reporting_nodes + org_reporting view）**
  - 用途：对 BI 暴露 active build 的稳定视图，避免下游理解 build pointer。
  - 入口：DB 视图 `org_reporting`（033）与 reporting build（033/029）。
  - 推荐验证：active build 切换后视图一致；build 幂等可重跑。

### 4.9 变更请求与预检（030）

- **变更请求（change requests：草稿/提交/取消）**
  - 用途：承载“待执行的 batch payload”，为审批流与治理扩展留接口；M2 最小状态机。
  - 入口：`/org/api/change-requests*`（030；表结构 SSOT=022）。
  - 推荐验证：request_id 幂等、非 draft 不可修改 payload（immutable）、列表分页 cursor。
- **预检（preflight：校验 + 影响分析）**
  - 用途：在不执行写入的前提下输出影响摘要；与 batch 共享校验管线，避免双合同。
  - 入口：`POST /org/api/preflight`（030）。
  - 推荐验证：复用稳定错误码并携带 command_index；影响面超限返回 `ORG_PREFLIGHT_TOO_LARGE`。

### 4.10 数据质量与修复闭环（031）

- **质量检查/修复/回滚（org-data quality）**
  - 用途：以“报告 → 修复计划 → 执行 → manifest 回滚”形成治理闭环；避免手工修复不可追溯。
  - 入口：`org-data quality check/plan/apply/rollback`（031；复用 023 二进制）。
  - 推荐验证：dry-run 与 apply 行为一致（除副作用）；rollback 能恢复 before 状态。
- **规则集 v1（ORG_Q_001~ORG_Q_009）**
  - 用途：固化可重复的质量门槛；其中 ORG_Q_008 支持 autofix（assignment.correct）。
  - 入口：031（规则 SSOT）。
  - 推荐验证：规则 id 稳定、输出 `org_quality_report.v1` schema 稳定、autofix 通过 026 batch 执行并可回滚。

### 4.11 UI 与信息架构（035/035A）

- **Org UI（树 + 节点面板 + 分配时间线）**
  - 用途：M1 可用的组织维护与分配查看/写入入口；与 `/org/api/*` 分工明确。
  - 入口：`/org/nodes`、`/org/assignments`（035）。
  - 推荐验证：内容协商（JSON > HTMX > full page）、422 表单错误展示、写入成功 OOB 刷新树/面板。
- **HTMX partial 路由与交互流**
  - 用途：减少全页刷新、保持选中态与滚动位置稳定；提供一致的 403 体验。
  - 入口：`/org/hierarchies`、`/org/nodes/{id}`、`/org/nodes`(POST/PATCH/move)、`/org/assignments`(GET/POST/PATCH)（035）。
  - 推荐验证：403 retarget/reshwap 行为、Hx-Push-Url 固化 effective_date、错误码映射为可读提示。
- **侧栏集成（Sidebar）**
  - 用途：与全站 Authenticated Layout 一致；按 capability 过滤导航入口，减少“点进去才 403”。
  - 入口：`modules/org/links.go` + `middleware.NavItems()`（035A）。
  - 推荐验证：NavItems 注入存在；入口按 `AuthzObject/AuthzAction` 可见性过滤（不替代后端鉴权）。

## 5. 入口总表（用于测试用例对照）

### 5.1 内部 API（JSON-only，`/org/api/*`）

- 树与节点（024/025/028/033）：
  - `GET /org/api/hierarchies`（028 扩展 `include=resolved_attributes`）
  - `POST /org/api/nodes`
  - `PATCH /org/api/nodes/{id}`
  - `POST /org/api/nodes/{id}:move`
  - `POST /org/api/nodes/{id}:correct`
  - `POST /org/api/nodes/{id}:rescind`
  - `POST /org/api/nodes/{id}:shift-boundary`
  - `POST /org/api/nodes/{id}:correct-move`
  - `GET /org/api/nodes/{id}:resolved-attributes`
  - `GET /org/api/nodes/{id}:path`
  - `GET /org/api/hierarchies:export`
- 分配（024/025/033）：
  - `GET /org/api/assignments`
  - `POST /org/api/assignments`
  - `PATCH /org/api/assignments/{id}`
  - `POST /org/api/assignments/{id}:correct`
  - `POST /org/api/assignments/{id}:rescind`
  - `GET /org/api/reports/person-path`
- outbox/snapshot/batch（026）：
  - `GET /org/api/snapshot`
  - `POST /org/api/batch`
- 角色/权限映射（028/032）：
  - `GET /org/api/roles`
  - `GET /org/api/role-assignments`
  - `GET/POST /org/api/security-group-mappings`
  - `POST /org/api/security-group-mappings/{id}:rescind`
  - `GET/POST /org/api/links`
  - `POST /org/api/links/{id}:rescind`
  - `GET /org/api/permission-preview`
- 变更与治理（030）：
  - `POST/PATCH/GET /org/api/change-requests*`
  - `POST /org/api/change-requests/{id}:submit`
  - `POST /org/api/change-requests/{id}:cancel`
  - `POST /org/api/preflight`
- 运维（034）：
  - `GET /org/api/ops/health`

### 5.2 UI 路由（HTML/HTMX，`/org/*`）

> 细节以 035/035A 为准；本节只列出“测试可见入口”。

- Full page：`GET /org/nodes`、（可选）`GET /org/assignments`
- HTMX partial：`GET /org/hierarchies`、`GET /org/nodes/{id}`、`POST/PATCH /org/nodes`、`POST /org/nodes/{id}:move`、`GET/POST/PATCH /org/assignments*`

### 5.3 CLI

- `org-data`（023/031）：`import/export/rollback/quality ...`
- `org-perf`（027）：`dataset apply`、`bench tree`
- `org-deep-read`（029）：build/activate/prune（以 `Makefile` 入口为 SSOT）
- `org-reporting`（033）：reporting build（以 `Makefile` 入口为 SSOT）
- `org-load`（034）：`run/smoke`（输出 `org_load_report.v1`）

## 6. 现有实现/测试落点映射（截至 `main@7af69390`）

> 说明：本节用于把本文的“功能入口”映射到当前仓库的实现与测试落点，便于后续补齐覆盖矩阵与回归清单。
> - 本节只列“索引”（文件/测试/Make 入口），不复制各 DEV-PLAN 的详细合同（以避免漂移）。
> - `*_integration_test.go` / query budget 测试多数依赖 Postgres；本地 DB 不可达时会 `t.Skip`，CI 环境会要求可连通。

### 6.1 DEV-PLAN → 代码/测试/命令

- **DEV-PLAN-020（蓝图/里程碑）**
  - 实现：无直接代码落点；以 021~036 的子计划为准。
  - 已有测试：无（本计划为总纲）。
- **DEV-PLAN-021/021A（Schema/迁移/工具链）**
  - 实现：`modules/org/infrastructure/persistence/schema/org-schema.sql`、`migrations/org/`、`atlas.hcl`、`Makefile`（`make org plan|lint|migrate ...`）
  - 已有测试（间接验证：测试内执行迁移并依赖约束/触发器）：
    - `modules/org/services/org_tree_query_budget_test.go`（baseline + 查询预算）
    - `modules/org/services/org_028_query_budget_test.go`（baseline + placeholders + 查询预算）
    - `modules/org/services/org_029_deep_read_consistency_test.go`（baseline + deep-read migrations + 一致性/预算）
    - `modules/org/services/org_032_permission_preview_integration_test.go`（baseline + security_group_mappings/links migrations）
    - `modules/org/services/org_033_visualization_and_reporting_integration_test.go`（baseline + deep-read/reporting migrations）
    - `modules/org/services/change_request_repository_integration_test.go`（baseline + placeholders migrations）
  - 快速命令：
    - `make org plan && make org lint && make org migrate up`
- **DEV-PLAN-022（占位表 + 事件契约 v1）**
  - 实现：
    - 占位表迁移：`migrations/org/20251218005114_org_placeholders_and_event_contracts.sql`
    - 事件结构（v1）：`modules/org/domain/events/`（consumer 通过 outbox/relay 获取）
  - 已有测试（偏“表/读侧一致性”，事件 payload contract 目前以 outbox envelope 断言为主）：
    - `modules/org/services/org_028_query_budget_test.go`（rules/roles/role_assignments）
    - `modules/org/services/change_request_repository_integration_test.go`（change_requests 表 tenant 隔离）
    - `modules/org/presentation/controllers/org_api_crud_integration_test.go`（outbox payload 可反序列化为 v1、topic 路由与字段名稳定）
  - 快速命令（需 Postgres）：`go test ./modules/org/services -run '^TestOrg028QueryBudget$|^TestChangeRequestRepository_TenantIsolation$' -count=1`
- **DEV-PLAN-023（org-data 导入/导出/回滚）**
  - 实现：`cmd/org-data/`
  - 已有测试：
    - `cmd/org-data/import_cmd_test.go`（`TestNormalizeNodes_*`、`TestNormalizeAssignments_*`）
    - `cmd/org-data/seed_import_rollback_integration_test.go`（seed apply + manifest rollback）
  - 快速命令：`go test ./cmd/org-data/... -count=1`
- **DEV-PLAN-024（主链 CRUD：nodes/edges/assignments）**
  - 实现：
    - Service：`modules/org/services/org_service.go`
    - API：`modules/org/presentation/controllers/org_api_controller.go`
    - UI：`modules/org/presentation/controllers/org_ui_controller.go`
  - 已有测试：
    - 单测：`modules/org/services/org_service_test.go`（auto-position 生成/格式、`ORG_INVALID_BODY`、`ORG_ASSIGNMENT_TYPE_DISABLED` 等）
    - 集成：`modules/org/services/org_assignment_subject_mismatch_integration_test.go`（`ORG_SUBJECT_MISMATCH`）
    - API controller 集成：`modules/org/presentation/controllers/org_api_crud_integration_test.go`（nodes create/update/move、assignments create、outbox 事件 envelope）
    - E2E：`e2e/tests/org/org-ui.spec.ts`（创建/编辑/移动节点；创建/编辑分配）
  - 快速命令：
    - `go test ./modules/org/services -run '^TestAutoPosition|^TestCreateAssignment|^TestCreateNode' -count=1`
    - `go test ./modules/org/services -run '^TestOrgAssignment_SubjectMismatch_ReturnsServiceError$' -count=1`（需 Postgres）
    - `make e2e ci`（Playwright）
  - 缺口：CRUD 在 controller 层仍缺少更完整的契约覆盖（positions 全链路、错误码映射边界、更多字段组合/回归用例）；但 nodes/assignments 的最小主路径已补齐 controller 集成测试。
- **DEV-PLAN-025（时间治理/冻结窗口/审计：Correct/Rescind/ShiftBoundary/Correct-Move）**
  - 实现：
    - Service：`modules/org/services/org_service_025.go`、`modules/org/services/freeze.go`
    - 迁移：`migrations/org/20251218130000_org_settings_and_audit.sql`
  - 已有测试：
    - 单测：`modules/org/services/freeze_test.go`（cutoff 计算与 enforce 拒绝）
    - 集成：`modules/org/services/org_025_time_and_audit_integration_test.go`（Correct/Rescind/ShiftBoundary/CorrectMove + `org_audit_logs` 断言）
  - 快速命令：`go test ./modules/org/services -run '^TestComputeFreezeCutoffUTC_|^TestFreezeCheck_|^TestOrg025' -count=1`
  - 缺口：仍缺少 025 写路径在 API controller 层的直接契约测试（HTTP status/JSON 错误映射）；当前覆盖到 service 集成层。
- **DEV-PLAN-026（Authz/Outbox/Snapshot/Batch）**
  - 实现：
    - 403 契约：`modules/org/presentation/controllers/authz_helpers.go`
    - Snapshot：`modules/org/services/snapshot.go`、`modules/org/infrastructure/persistence/org_snapshot_repository.go`、`modules/org/presentation/controllers/org_api_controller.go`
    - Batch：`modules/org/presentation/controllers/org_api_controller.go`（写入口；命令语义复用 024/025/032）
    - Outbox：`modules/org/services/org_service.go`（事务内 enqueue）、DB：`public.org_outbox`
  - 已有测试：
    - `modules/org/presentation/controllers/authz_helpers_test.go`（Forbidden JSON contract + object/action 映射守卫）
    - `modules/org/services/org_assignment_subject_mismatch_integration_test.go`（subject 映射不一致时拒绝）
    - `modules/org/presentation/controllers/org_api_snapshot_batch_integration_test.go`（`GET /org/api/snapshot` 分页/cursor、`POST /org/api/batch` dry-run/command_index meta）
  - 快速命令：
    - `go test ./modules/org/presentation/controllers -count=1`
    - `go test ./modules/org/services -run '^TestOrgAssignment_SubjectMismatch_ReturnsServiceError$' -count=1`（需 Postgres）
  - 缺口：仍缺少对更多 batch command 组合与边界（例如 move 限制/限流、permission mapping 相关命令）在 controller 层的契约测试。
- **DEV-PLAN-027（性能基准/灰度/回滚）**
  - 实现：
    - Query budget：`modules/org/services/org_tree_query_budget_test.go`
    - 基准 CLI：`cmd/org-perf/`；Make 入口：`make org-perf-dataset`、`make org-perf-bench`
    - 灰度开关：配置项读取在 `pkg/configuration/*` 与 Org service/controller 内（以 027/Makefile 为准）
  - 已有测试：
    - `modules/org/services/org_tree_query_budget_test.go`（树读 roundtrip 次数恒定）
  - 快速命令：
    - `go test ./modules/org/services -run '^TestOrgTreeQueryBudget$' -count=1`（需 Postgres）
    - `make org-perf-dataset && make org-perf-bench`
- **DEV-PLAN-028（继承解析 + 角色读侧占位）**
  - 实现：`modules/org/services/org_inheritance_resolver.go`、`modules/org/infrastructure/persistence/org_028_repository.go`
  - 已有测试：
    - 单测：`modules/org/services/org_inheritance_resolver_test.go`
    - 集成/预算：`modules/org/services/org_028_query_budget_test.go`（resolved hierarchy + role-assignments query budget）
  - 快速命令：
    - `go test ./modules/org/services -run '^TestResolveOrgAttributes_' -count=1`
    - `go test ./modules/org/services -run '^TestOrg028QueryBudget$' -count=1`（需 Postgres）
- **DEV-PLAN-029（闭包表/快照表/深读后端切换）**
  - 实现：
    - Repo/build：`modules/org/infrastructure/persistence/org_deep_read_repository.go`、`modules/org/infrastructure/persistence/org_deep_read_build_repository.go`
    - Make 入口：`make org-snapshot-build`、`make org-closure-build`、`make org-closure-activate`、`make org-closure-prune`
  - 已有测试：
    - `modules/org/services/org_029_deep_read_consistency_test.go`（edges/closure/snapshot 一致性 + query budget）
  - 快速命令（需 Postgres）：`go test ./modules/org/services -run '^TestOrg029DeepReadConsistency$|^TestOrg029DeepReadQueryBudget$' -count=1`
- **DEV-PLAN-030（Change Requests / Preflight）**
  - 实现：
    - Repo：`modules/org/infrastructure/persistence/org_change_request_repository.go`
    - API：`modules/org/presentation/controllers/org_api_controller.go`（`/org/api/change-requests*`、`/org/api/preflight`）
  - 已有测试：
    - `modules/org/services/change_request_repository_integration_test.go`（repo tenant 隔离）
    - `modules/org/presentation/controllers/org_api_change_requests_preflight_integration_test.go`（preflight：幂等/immutable/too-large、command_index meta；change-request：draft/immutable 规则）
  - 快速命令（需 Postgres）：`go test ./modules/org/services -run '^TestChangeRequestRepository_TenantIsolation$' -count=1`
  - 缺口：仍缺少 change-request 生命周期（submit/cancel）与权限边界的覆盖（API/E2E）。
- **DEV-PLAN-031（数据质量与修复闭环）**
  - 实现：`cmd/org-data/`（quality 子命令），以及 026 的 `/org/api/batch` 执行面（SSOT）
  - 已有测试：
    - `cmd/org-data/quality_plan_cmd_test.go`（从 report 生成 fix plan）
    - `cmd/org-data/quality_rollback_cmd_test.go`（从 manifest 生成回滚 batch request；对齐 030 change-request payload 校验）
    - `cmd/org-data/quality_apply_rollback_integration_test.go`（apply 生成 manifest + rollback 通过 batch 回放）
  - 快速命令：`go test ./cmd/org-data/... -count=1`
- **DEV-PLAN-032（安全组映射/links/permission preview）**
  - 实现：`modules/org/services/org_service_032.go`、`modules/org/infrastructure/persistence/org_032_repository.go`、`modules/org/presentation/controllers/org_api_controller.go`
  - 已有测试：
    - `modules/org/services/org_032_permission_preview_integration_test.go`（preview 继承/来源 + links 截断 warning + no-overlap）
  - 快速命令（需 Postgres）：`go test ./modules/org/services -run '^TestOrg032' -count=1`
- **DEV-PLAN-033（导出/路径/人员路径/BI reporting view）**
  - 实现：
    - Export/path：`modules/org/infrastructure/persistence/org_033_export_repository.go`、`modules/org/services/org_033_visualization.go`、`modules/org/presentation/controllers/org_api_controller.go`
    - reporting build：`modules/org/infrastructure/persistence/org_reporting_build_repository.go`；Make 入口：`make org-reporting-build`
  - 已有测试：
    - `modules/org/services/org_033_visualization_and_reporting_integration_test.go`（node path/export/person-path/reporting build）
  - 快速命令（需 Postgres）：`go test ./modules/org/services -run '^TestOrg033' -count=1`
- **DEV-PLAN-034（Ops/Monitoring/Load）**
  - 实现：
    - 指标：`modules/org/presentation/controllers/org_metrics.go`
    - Ops health：`modules/org/presentation/controllers/org_ops_health.go`（`GET /org/api/ops/health`）
    - load 工具：`cmd/org-load/`；Make 入口：`make org-load-smoke`、`make org-load-run`
  - 已有测试：
    - `modules/org/presentation/controllers/org_ops_health_integration_test.go`（ops health 响应形状/授权）
    - `modules/org/presentation/controllers/org_metrics_test.go`（metrics label 红线：endpoint label 稳定，避免高基数）
  - 快速命令：`make org-load-smoke`（以及按 034 契约运行 `make org-load-run`）
  - 缺口：org-load 报告（`org_load_report.v1`）schema/阈值与 ops health 细分阈值逻辑仍缺少自动化测试。
- **DEV-PLAN-035/035A（UI/IA/Sidebar 集成）**
  - 实现：
    - UI controller：`modules/org/presentation/controllers/org_ui_controller.go`
    - templates/locales：`modules/org/presentation/templates/`、`modules/org/presentation/locales/`
    - Sidebar：`modules/org/links.go`（由 `middleware.NavItems()` 注入并被 layout 消费，详见 035A）
  - 已有测试：
    - `e2e/tests/org/org-ui.spec.ts`（DEV-PLAN-035：管理员主路径 + 无权限访问 403）
  - 快速命令：`make e2e ci`
- **DEV-PLAN-036（示例数据集）**
  - 实现：`docs/samples/org/036-manufacturing/`（CSV）
  - 已有测试：暂无（以 036 Readiness 演练与 023 工具链为主）。

### 6.2 建议优先补齐的覆盖缺口（按风险排序）

1. 025 写路径在 API controller 层的直接契约测试（correct/rescind/shift-boundary/correct-move：HTTP status/JSON 错误映射）。
2. 030 change-request 生命周期（submit/cancel）与权限边界的自动化覆盖（API/E2E）。
3. 034 org-load 报告（`org_load_report.v1`）schema/threshold 与 ops health 细分阈值逻辑测试。
4. 022/026 事件 payload（outbox → integration events v1）仍缺少 old/new values 口径测试（当前主要覆盖 envelope/字段名/topic）。

## 7. 回归清单（可执行入口）

> 说明：以下是按 020~036 功能覆盖门槛挑选的“最小回归集合”；更广泛的全仓验证仍以 `Makefile`/CI 为准。

- 迁移/门禁（021A）：`make org plan && make org lint && make org migrate up`
- Service/Repo（024/025/028/029/032/033）：`go test ./modules/org/services -count=1`
- API 鉴权/合同（026/034）：`go test ./modules/org/presentation/controllers -count=1`
- CLI（023/031）：`go test ./cmd/org-data/... -count=1`
- UI 回归（035）：`make e2e ci`
- 性能/运维（027/034）：`make org-perf-dataset && make org-perf-bench`、`make org-load-smoke`

## 8. 备注（后续扩展提示）

020~036 文档中有少量对 053/056/057 等“后续扩展能力”的引用（例如 positions/job-catalog/reports）。这些不属于本目录的覆盖门槛；如需纳入测试清单，请以对应 dev-plan（053/056/057/…）为 SSOT 另起目录或扩展本文件版本。
