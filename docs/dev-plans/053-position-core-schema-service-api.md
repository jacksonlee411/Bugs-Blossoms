# DEV-PLAN-053：Position Core（Schema + Service + API 最小闭环）（对齐 051 阶段 B）

**状态**: 草拟中（2025-12-20 01:46 UTC）

## 1. 背景
- 目标是在 Org BC 内补齐 Position/Assignment 的核心闭环：可有效期化、可审计、可治理（Update/Correct/Rescind/冻结窗口）、可占编（FTE）、可查询（as-of/时间线/过滤）。
- 关键约束：必须兼容既有 Org 主链的 auto position 写链路（System Position），避免“一刀切”约束破坏存量流程。

## 2. 工具链与门禁（SSOT 引用）
- Org Atlas+Goose：见 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`。
- 时间/审计/冻结窗口口径：见 `docs/dev-plans/025-org-time-and-audit.md`。
- 路由与 JSON-only：见 `docs/dev-plans/018-routing-strategy.md`。
- 事件与 Outbox：见 `docs/dev-plans/022-org-placeholders-and-event-contracts.md`、`docs/dev-plans/026-org-api-authz-and-events.md`、`docs/dev-plans/017-transactional-outbox.md`。
- 具体命令以 `AGENTS.md` 触发器矩阵与 `Makefile` 为准。

## 3. 目标（完成标准）
- [ ] Schema：`org_positions/org_assignments` 支持核心字段、FTE 容量/占用与“一岗多人/部分填充”，并兼容 System/Managed。
- [ ] Service：实现 v1 的核心治理命令（Update/Correct/Rescind/ShiftBoundary/转移/汇报链）与强校验（不变量、前置检查、可观测拒绝原因）。
- [ ] API：提供最小可用写入口与查询入口（as-of、时间线、列表过滤、组织范围含下级），错误码口径稳定。
- [ ] 事件：Position/Assignment 变更事件满足 022 v1 契约，并通过 026 的 outbox 机制落盘投递。

## 4. 依赖与并行
- 前置依赖：`DEV-PLAN-052`（契约与口径冻结）。
- 可并行协作：UI（DEV-PLAN-055）可在 API 合同冻结后先做 IA/原型；Authz（DEV-PLAN-054）可并行定义 object/action。

## 5. 实施步骤
1. [ ] Schema 设计（Org 轨道）
   - 扩展 `org_positions/org_assignments`：补齐 v1 字段、容量（FTE 为主，人数可选）、汇报关系、reason code 审计落点、必要索引/约束。
   - 支持“一岗多人/部分填充”：同一 Position 在同一时间窗允许多条 Assignment（不同 subject），以容量规则阻止超编（替代“position 唯一占用”的硬约束）。
   - System/Managed 策略：新增字段/约束避免“一刀切 NOT NULL”破坏 System Position；对 Managed 强制更严格校验。
2. [ ] 迁移与约束落地（Org Atlas+Goose）
   - 输出 atlas plan 并通过 lint；在 PG17 冒烟迁移（包含回滚策略与不可逆点说明）。
   - 确保与既有 valid-time/no-overlap 规则不冲突（必要时调整唯一/排他约束口径以支持“一岗多人/部分填充”）。
3. [ ] 服务层能力落地（不展开代码细节）
   - 有效期治理：Update/Correct/Rescind/ShiftBoundary + 冻结窗口策略复用 025 的口径。
   - 组织转移、汇报关系更新：以有效期版本表达并保留历史；reports-to 防环校验可靠。
   - 占编：FTE 占用计算、部分填充与超编阻断；拒绝原因可观测且可审计追溯。
   - 创建/变更强校验：组织有效期、分类启用/自洽、（如接入）Job Profile→Catalog 冲突拒绝（以 DEV-PLAN-056 的映射作为 SSOT）。
   - 撤销前置检查：对齐 050 §7.6（下属职位、任职记录治理策略、错误提示口径）。
4. [ ] API 合同落地（Org API）
   - 写入口：Position 创建/更新/纠错/撤销/转移；Assignment 占用/释放（FTE 为主）。
   - 查入口：as-of 获取、时间线、列表过滤（生命周期/填充状态/类型/分类/关键词/组织范围含下级）、分页。
   - 统一错误码与 JSON-only 行为（对齐 018/026）。
5. [ ] 事件生成与投递
   - Position/Assignment 的变更事件满足 022 v1 契约；通过 026 的 outbox 统一落盘与投递，避免多处各发各的事件造成漂移。
6. [ ] 兼容性与灰度策略
   - System Position：维持既有 auto position 入口可写可查；Managed Position 逐步开启强校验。
   - 在不破坏既有写链路的前提下，引入 feature flag/禁写策略/只读模式（与 DEV-PLAN-059 收口计划对齐）。
7. [ ] 验收与门禁记录（执行时填写）
   - 按 `AGENTS.md` 触发器矩阵跑对应门禁，并将命令/结果/时间戳登记到收口计划指定的 readiness 记录。

## 6. 交付物
- Org schema 迁移：支持 Position/Assignment v1 核心语义与容量/占编（FTE）。
- Service + API：核心治理与查询闭环，可演示可审计。
- 事件：对齐 022/026 的 v1 事件产出与 outbox 投递链路。

