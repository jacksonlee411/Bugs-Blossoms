# DEV-PLAN-069 Readiness

**状态**：Ready to Close（待 PR 合并）

## 关闭条件（本地已满足）
- 写入止血（A）：`MoveNode`/`CorrectMoveNode` 在事务内对后代未来切片做 `org_edges.path/depth` 前缀重写，并带 `ORG_PREFLIGHT_TOO_LARGE` 预算保护。
- 存量治理（C）：提供离线巡检 SQL + 单批修复脚本，可迭代收敛到 0。
- 回归测试：新增集成测试覆盖“先对后代制造未来切片，再回溯移动祖先导致 path 失真”的场景，验证 long_name 不缺段且巡检为 0。

## 证据
- 本地门禁：`go fmt ./... && go vet ./... && make check lint && make test`（通过）
- 新增巡检/修复脚本：
  - `scripts/org/069_org_edges_path_inconsistency.sql`
  - `scripts/org/069_org_edges_path_inconsistency_count.sql`
  - `scripts/org/069_fix_org_edges_path_one_batch.sql`
- 新增测试：
  - `modules/org/services/org_069_edges_path_consistency_integration_test.go`

