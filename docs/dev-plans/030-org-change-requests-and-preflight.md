# DEV-PLAN-030：Org 变更请求与预检

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 10，需提供 change_requests 草稿/提交（workflow 未启用时仅存草稿+审计）与 Pre-flight 影响预检 API。

## 目标
- change_requests 支持草稿/提交存档，记录审计。
- Pre-flight API 可用，强制权限/租户校验，并输出影响分析。
- 单测覆盖无权/有权/租户隔离路径。

## 实施步骤
1. [ ] 实现 change_requests 草稿/提交接口，未接入 workflow 时不触发路由但保留审计。
2. [ ] 实现 Pre-flight 影响预检 API，输出影响摘要（节点/assignment/事件等），并做权限/租户校验。
3. [ ] 单测覆盖无权限/有权限/租户隔离与审计记录。
4. [ ] 文档说明当前限制与后续对接 workflow 的接口预留。

## 交付物
- change_requests 草稿/提交实现与审计。
- Pre-flight API 与测试。
