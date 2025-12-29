# DEV-PLAN-069B Readiness

**状态**：待 PR 合并（2025-12-29 04:45 UTC）— 066 删除/缝补入口已接入 069 “preflight + prefix rewrite” 原语

## 关闭条件（待 PR 合并）

- [X] `org_edges` Delete+Stitch 写入口接入 069 原语：preflight（COUNT）→ delete+stitch → `LockEdgeAt(as-of=E)` 读回 `new_prefix` → prefix rewrite（UPDATE）
- [X] 错误码固化：
  - `422 ORG_PREFLIGHT_TOO_LARGE`（复用 069）
  - `422 ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`（禁止删除最早 edge slice）
- [X] 集成测试覆盖“删除导致祖先链恢复”与“禁止删除第一片”：
  - `modules/org/services/org_069B_edges_path_consistency_delete_integration_test.go`
- [X] 文档登记（避免实现/文档漂移）：
  - `docs/dev-plans/069B-org-edges-path-consistency-for-delete-and-boundary-changes.md`
  - `docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`
  - `AGENTS.md` Doc Map 增加 readiness 链接

## 证据

- 实现：`modules/org/services/org_service_066.go`
- 测试：`modules/org/services/org_069B_edges_path_consistency_delete_integration_test.go`
