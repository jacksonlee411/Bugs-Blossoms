# DEV-PLAN-022：Org 占位表与事件契约

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 2，需提前创建占位表（继承规则、角色、change_requests 草稿等）并定义 `OrgChanged` / `OrgAssignmentChanged` 事件契约。

## 目标
- 占位表结构就绪且不破坏 M1 范围。
- 事件 payload 定义覆盖 assignment_type、继承属性、change_type/initiator/version/timestamp 及幂等键等扩展字段。
- 生成命令（sqlc/atlas）执行后工作区干净。

## 实施步骤
1. [ ] 在 schema 中新增 `org_attribute_inheritance_rules`、`org_roles`、`change_requests` 等占位表并生成迁移。
2. [ ] 定义事件契约（文档或类型定义），含 assignment_type、继承属性、change_type/initiator、version/timestamp、幂等键等预留字段，保持与 020 契约一致。
3. [ ] 运行 `make generate` 或 `make sqlc-generate`（如适用），确认 `git status --short` 干净。
4. [ ] 记录更新点并与 021 迁移保持兼容（无破坏性变更）。

## 交付物
- 占位表迁移与 schema 更新。
- 事件契约说明或类型定义。
