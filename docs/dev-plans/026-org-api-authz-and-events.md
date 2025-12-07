# DEV-PLAN-026：Org API、Authz 与事件发布

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 6，在 CRUD 与时间/审计就绪后，需提供 REST API、事件发布，并统一接入 `pkg/authz`（Org.Read/Org.Write/Org.Assign/Org.Admin），提交策略片段并通过 authz 门禁。

## 目标
- API 覆盖节点/职位/分配操作，返回值含租户隔离与有效期语义。
- 事件 `OrgChanged` / `OrgAssignmentChanged` 在写入成功后发布。
- `make authz-test authz-lint authz-pack` 通过，策略片段提交。
- 所有入口接受 `effective_date` 参数（默认 `time.Now()`），响应/查询遵循时间线语义。
- 树/分配读写配套缓存键（含层级/tenant/effective_date）与事件驱动失效/重建策略。

## 实施步骤
1. [ ] 实现 `/org/**` REST API，统一 Session+租户校验，调用 `pkg/authz` 判定对应动作；所有读/写接口接受 `effective_date`（默认 `time.Now()`）。
2. [ ] 将策略片段写入 `config/access/policies/org/**`，执行 `make authz-test authz-lint authz-pack` 并记录结果。
3. [ ] 在写路径挂载事件发布，确保包含 assignment_type/tenant/change_type/initiator/version/幂等键等字段。
4. [ ] 设计并实现树/分配缓存键与事件驱动的失效/重建脚本，记录回滚/全量重建命令。
5. [ ] 单测覆盖 403/200、租户隔离、effective_date 默认、缓存失效触发与事件发布（可 mock）。

## 交付物
- API/路由与权限片段。
- 事件发布实现。
- authz 门禁执行记录。
