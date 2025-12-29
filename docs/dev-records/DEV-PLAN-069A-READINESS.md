# DEV-PLAN-069A Readiness

**状态**：已合并（2025-12-29 03:27 UTC）— path-driven long_name 已落地到 main（PR #159）

## 关闭条件（已满足）

- [X] 默认 long_name 查询已为 path-driven：`pkg/orglabels/org_node_long_name.go`
- [X] 对照一致性测试已存在：`modules/org/services/org_069A_long_name_path_driven_equivalence_test.go`
- [X] 依赖 069 Gate：`docs/dev-plans/069-org-long-name-parent-display-mismatch-investigation.md`（A/C 已合并）

## 证据

- 实现：`pkg/orglabels/org_node_long_name.go`（`mixedAsOfQuery` 拆 `target.path` 再 join `org_node_slices`）
- 测试：`modules/org/services/org_069A_long_name_path_driven_equivalence_test.go`
- 合并：PR #159
