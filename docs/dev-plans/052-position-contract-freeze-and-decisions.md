# DEV-PLAN-052：职位管理（Position）契约冻结与关键决策（对齐 051 阶段 A）

**状态**: 草拟中（2025-12-20 01:46 UTC）

## 1. 背景
- [DEV-PLAN-050](050-position-management-business-requirements.md) 已给出对标 Workday 的业务需求；[DEV-PLAN-051](051-position-management-implementation-blueprint.md) 将其拆成阶段并给出并行路线图。
- 本子计划负责“Contract First”的**关键决策冻结**：统一 SSOT、口径、状态映射、v1 字段边界、事件与 API 合同，避免后续实现阶段发生漂移与返工。

## 2. 工具链与门禁（SSOT 引用）
- 统一以 `AGENTS.md`、`Makefile`、`.github/workflows/quality-gates.yml` 以及 [DEV-PLAN-051](051-position-management-implementation-blueprint.md) §3 为准（本计划不复制命令矩阵）。

## 3. 目标（完成标准）
- [ ] 冻结“Position 与 Org 的关系与 SSOT”：Position/Assignment 以 Org BC（`modules/org`）为主干；HRM 仅消费只读映射，避免双 SSOT。
- [ ] 冻结核心口径：生命周期状态、填充状态（EMPTY/PARTIALLY_FILLED/FILLED）、VACANT 语义、容量/占用（FTE 为主）与 as-of 计算口径。
- [ ] 冻结 v1 字段清单与扩展策略（结构化列 vs `profile jsonb`），并给出字段可变性矩阵与 reason code 约束。
- [ ] 冻结“最小稳定 API 面”与 Position/Assignment 事件契约（先改契约文档，再改实现）。

## 4. 依赖与并行
- 依赖：050/051；Org 时间与审计/冻结窗口口径以 [DEV-PLAN-025](025-org-time-and-audit.md) 为准；事件契约 SSOT 以 [DEV-PLAN-022](022-org-placeholders-and-event-contracts.md) 为准；API/Authz/Outbox 门禁以 [DEV-PLAN-026](026-org-api-authz-and-events.md) 为准。
- 本计划完成后可并行启动：`DEV-PLAN-053/054/055/056`。

## 5. 实施步骤
1. [ ] 决策冻结：SSOT 与边界
   - 明确 Position/Assignment 在业务上从属 OrgUnit、在模型上为 Org BC 内独立聚合根（Staffing 子域）。
   - 明确 HRM 旧 `positions` 的定位与处置：仅保留为 legacy 字典（冻结不扩展）或迁移/重命名为 Job Profile（并在 DEV-PLAN-056 收口），避免双 SSOT。
   - 明确并固化 **System Position（auto-created）/Managed Position（业务管理）** 的兼容策略与约束边界。
2. [ ] 口径冻结：状态、占编与停用策略
   - 生命周期状态与现有 `org_positions.status` 的映射策略（含“未来生效=PLANNED”等口径）。
   - 填充状态派生口径：按 as-of + Assignment FTE 占用与 Position 容量计算。
   - 停用/撤销策略：对齐 050 §7.6/§7.6.1（停用在任策略 A/B）与撤销前置检查口径。
3. [ ] v1 字段清单与字段可变性矩阵冻结
   - 冻结 v1 必填/可选字段（对齐 050 §4.1/§4.2）。
   - 输出字段可变性矩阵（对齐 050 §7.8）：哪些字段允许 Update/Correct，哪些在“在任/满编”等条件下必须阻断或走特定流程。
   - reason code：明确是否租户可配置、是否对关键操作必填、审计落点与可追溯口径。
4. [ ] API 合同冻结（不展开 handler 细节）
   - Position：创建/更新/纠错/撤销/转移/查询（as-of、时间线、列表过滤、包含下级组织口径）。
   - Assignment：占用/释放（FTE 为主），以及 v1 对“多段任职事件/计划任职”的边界声明。
   - 统计：组织维度容量/占用/可用/填充率（FTE 口径为主；人数口径可后续并行）。
5. [ ] 事件契约冻结（Contract First）
   - 在 022 中补齐/确认 Position/Assignment 的事件字段（v1），并对枚举/口径给出 SSOT 定义。
   - 若需新增字段/变更枚举，先更新契约文档（022/026）并评审通过，再进入实现阶段（053+）。
6. [ ] 评审与门禁准备
   - 与实现负责人完成评审：确认“可并行拆分点”“不可逆决策点”“兼容期策略”。
   - 明确后续计划的验收口径与最小测试用例集（以便 053/054/055/056 并行推进时对齐）。

## 6. 交付物
- 冻结决策清单（SSOT、System/Managed、HRM legacy positions 处置、兼容期原则）。
- 口径与映射（状态/占编/撤销/停用/统计口径）与字段可变性矩阵。
- v1 字段清单与 API/事件契约变更清单（指向 022/026 等 SSOT 文档）。

