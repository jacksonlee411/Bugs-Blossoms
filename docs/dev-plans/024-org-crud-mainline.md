# DEV-PLAN-024：Org 主链 CRUD（Person→Position→Org）

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 4，需交付核心 CRUD，包含自动创建空壳 Position，且全链路强制 Session+租户隔离。

## 目标
- Person→Position→Org 单树 CRUD 可用。
- 未登录/无租户访问直接拒绝。
- 自动创建一对一空壳 Position 的逻辑生效。
- M1 仅开放 primary assignment 写入，matrix/dotted 保持只读占位或特性开关关闭。
- 全链路遵守 DDD 分层/cleanarch 约束，禁止跨层耦合。
- 所有接口/服务接受 `effective_date` 参数，缺省按 `time.Now()` 处理。

## 实施步骤
1. [ ] 实现 service/repo 层 CRUD，强制租户过滤与 Session 校验，遵守 DDD 分层/cleanarch 依赖方向。
2. [ ] 添加自动建空壳 Position 流程（缺 position_id 时创建并绑定），并软校验 person_id/pernr（跨 SOR 仅提示），强校验 position_id 归属 OrgNode。
3. [ ] 补充单测覆盖无 Session/无租户/有租户场景，并确保 matrix/dotted 写入被拒绝或受特性开关保护；验证 `effective_date` 默认值逻辑。
4. [ ] 提供基础树/表单 templ 页面（节点/Position/Assignment），执行 `templ generate && make css`，更新模块文档说明入口与约束。

## 交付物
- CRUD 实现与测试。
- 文档更新（使用说明/约束）。
