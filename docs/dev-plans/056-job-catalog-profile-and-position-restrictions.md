# DEV-PLAN-056：Job Catalog / Job Profile 与 Position Restrictions（对齐 051 阶段 D）

**状态**: 草拟中（2025-12-20 01:46 UTC）

## 1. 背景
- 050 定义了与 Workday 类似的“岗位分类与主数据”与“职位限制”能力：Job Catalog（四级分类）、Job Profile（岗位定义）、以及写入时强制校验的限制规则。
- 仓库现状存在 HRM `positions` 的历史包袱；需要避免与 Org Position 形成双 SSOT，并把“主数据/规则”以可并行轨道交付（先可用，后加严）。

## 2. 工具链与门禁（SSOT 引用）
- 组织侧 schema 工具链：见 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`。
- Authz/Outbox/事件契约：见 `docs/dev-plans/026-org-api-authz-and-events.md`、`docs/dev-plans/022-org-placeholders-and-event-contracts.md`。
- 门禁触发与本地必跑以 `AGENTS.md` 为准。

## 3. 目标（完成标准）
- [ ] Job Catalog 与 Job Profile 的数据模型与维护入口可用（含启停规则与唯一性约束）。
- [ ] Job Profile 与 Job Role/Job Level 的绑定/允许集合模型落地，并作为冲突校验 SSOT（对齐 050 §3.1）。
- [ ] Position Restrictions 可配置，且在 Position/Assignment 写入口强校验；拒绝原因可观测、可审计追溯。
- [ ] 强治理能力补齐：冻结窗口默认策略、reason code 治理（枚举/租户自定义）、改历史（Correct）高权限边界明确。

## 4. 依赖与并行
- 依赖：`DEV-PLAN-052`（合同与口径冻结）；`DEV-PLAN-053`（Position Core 写入口与占编口径）。
- 可并行：主数据维护 UI 可先做原型；强校验启用可灰度（与 DEV-PLAN-059 收口策略对齐）。

## 5. 实施步骤
1. [ ] 数据模型设计与 SSOT 冻结
   - Job Catalog：四级分类（含 code/name/启停/层级自洽），并明确是否允许跨层级引用与默认展示策略。
   - Job Profile：岗位定义字段边界与版本策略；必须绑定 Job Role（可选 Job Level 集合），并定义“Profile→Catalog 允许集合/冲突拒绝”规则。
2. [ ] HRM legacy `positions` 处置收口
   - 按 052 的决策执行：冻结为 legacy 字典，或制定迁移/重命名为 Job Profile 的数据迁移路线（避免继续同名漂移）。
3. [ ] Schema 与迁移（若落在 Org BC）
   - 通过 Atlas+Goose 落地表结构、枚举与索引；明确不可逆点与回滚策略。
4. [ ] 维护入口（API + UI）
   - Job Catalog/Profile：最小 CRUD、启停、查询；审计与有效期策略对齐项目口径。
   - 与 Authz 对齐：管理权限与只读权限区分明确（对齐 DEV-PLAN-054）。
5. [ ] Position Restrictions 落地与强校验
   - 定义限制维度与校验时机（Position 写入时/Assignment 写入时）；拒绝原因可观测且可追溯。
   - 与 Position Core 结合：容量/占用与限制共同决定“是否允许占用/是否允许变更”。
6. [ ] 强治理与灰度
   - 冻结窗口默认策略与 reason code 治理策略落地；明确改历史（Correct）高权限边界与审计口径。
   - 灰度启用强校验：优先对 Managed Position 启用，避免破坏 System Position 兼容链路。
7. [ ] 验收与门禁记录（执行时填写）
   - 按 `AGENTS.md` 触发器矩阵跑对应门禁，并将命令/结果/时间戳登记到收口计划指定的 readiness 记录。

## 6. 交付物
- Job Catalog / Job Profile 主数据（含维护入口与启停治理）。
- Position Restrictions 的配置与写入强校验（含可观测拒绝原因与审计追溯）。
- 强治理策略（冻结窗口、reason code、改历史高权限边界）与灰度启用方案。

