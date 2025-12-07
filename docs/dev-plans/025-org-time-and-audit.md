# DEV-PLAN-025：Org 时间约束与审计

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 5，需落地有效期校验、防重叠/防空档/防环，并区分 Correct/Update、支持 Rescind，执行冻结窗口审计。

## 目标
- 写入前有效期/层级校验完整，冻结窗口（默认月末+3 天，可租户覆盖）生效。
- Correct/Update/Rescind 行为审计可见。
- 审计/事件含 transaction_time/version/initiator 等字段，便于回放与对账。

## 实施步骤
1. [ ] 在 service/repo 层实现有效期冲突、无空档、防环校验；违反冻结窗口时拒绝并记录审计。
2. [ ] 实现 Correct（原地修正）与 Update（新时间片）的分支，并标记审计字段。
3. [ ] 实现 Rescind 软撤销路径（含审计）。
4. [ ] 单测覆盖：正常、重叠、冻结窗口、Rescind、无租户拒绝等，并验证审计/Outbox 包含 transaction_time/version/initiator。

## 交付物
- 时间/审计能力代码与测试。
- 冻结窗口策略说明。
