# DEV-PLAN-069B：在 066 删除/边界变更场景保持 `org_edges.path` 一致性（详细设计）

**状态**: 规划中（2025-12-29 00:15 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**：
  - DEV-PLAN-069：`org_edges.path/depth` 作为结构索引 SSOT，并通过 “preflight + prefix rewrite” 修复回溯移动祖先导致的跨切片 `path` 失真问题。
  - DEV-PLAN-066：引入对 `org_edges` 时间片的删除（Auto-Stitch / merge-into-prev），以及（可能的）边界调整能力。
- **当前痛点**：若 066 对 `org_edges` 的写入口仅做 `DELETE/UPDATE(end_date)` 而不联动修正后代 future slice 的 `path/depth`，将复现 069 的根因：父边 `path` 已更新/恢复，但子孙边仍沿用旧前缀，导致 long_name/祖先匹配缺段（`pkg/orglabels/org_node_long_name.go` 的 `e.path @> t.path` 匹配会截断）。
- **业务价值**：把“结构索引一致性”从 Move/CorrectMove 扩展到 066 的删除/边界变更入口，使 long_name/祖先/子树查询在更多操作下保持稳定正确。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 让 DEV-PLAN-069 的一致性修复在 DEV-PLAN-066 的 `org_edges` 写入口下同样成立：任何“从某天起改变 child 祖先链”的写操作，都必须维护后代 future slices 的 `org_edges.path/depth` 前缀一致性。
- [ ] 固化 066 的 `org_edges` 删除入口接入 069 prefix rewrite 的契约（触发条件、`E/old_prefix/new_prefix` 取值、preflight 预算与失败策略、事务顺序）。
- [ ] 补齐集成测试：覆盖“先制造 future slice，再通过 066 删除改变祖先链”的回归场景；验收 long_name 不缺段且巡检为 0。

### 2.2 非目标 (Out of Scope)
- 不新增数据库表/大规模 schema 改造（如需新增表/迁移必须另开计划并获得手工确认）。
- 不把 long_name 持久化到写模型（仍为读时派生）。
- 本计划 v1 **不实现** 新的 “edge 生效日期/边界调整 API”；若 066 v1 未来确实落地该能力，再补充本计划的扩展小节（见 6.5）。

## 2.3 工具链与门禁（SSOT 引用）
- **触发器清单（勾选本计划命中的项）**：
  - [x] Go 代码（`AGENTS.md` / `Makefile` / CI 门禁）
  - [ ] DB 迁移 / Schema（本计划预期不涉及；若涉及按 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`
  - 069 详细设计：`docs/dev-plans/069-org-long-name-parent-display-mismatch-investigation.md`
  - 066 详细设计：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 关键决策（ADR 摘要）
- **决策 1（选定）：复用 069 的 prefix rewrite 原语，不新增新机制**
  - 结论：066 的 `org_edges` 删除/边界变更写入口必须调用 069 的 COUNT/UPDATE 原语（同一事务），避免出现第二套“修 path”的实现与漂移。
- **决策 2（选定）：删除场景的 `new_prefix` 必须用 `LockEdgeAt(as-of=E)` 读回**
  - 原因：merge-into-prev 删除的切片就是 `effective_date==E` 的 `T`，删除后 as-of=E 生效的边可能来自 `effective_date < E` 的 `P`；因此不能用 `LockEdgeStartingAt(effective_date=E)`。
- **决策 3（选定）：preflight 必须在任何数据改动前完成（保证超限零副作用）**
  - 原因：超限即返回稳定错误码 `ORG_PREFLIGHT_TOO_LARGE`；如果先 `DELETE/UPDATE` 再超限会引入补偿与审计漂移，属于偶然复杂度。
- **决策 4（选定）：对 `org_edges` 删除 v1 增加“必须存在上一片 P”约束**
  - 原因：删除最早切片会制造“结构缺口”（child 在某段日期内无 edge），并可能与后代 edges 同时存在，产生更强不一致；该语义更接近“删除结构历史”，不属于 066 v1 的“撤销一次变更”定义。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划不引入 schema 变更；约束层面只声明“写路径必须维护的结构不变量”。

### 4.1 关键不变量（必须强制达成）
- 对任意 `org_edges` 切片 `c`（parent 不为空），在 `as-of=c.effective_date` 能找到父边切片 `p`，并满足：`p.path @> c.path`（父边 path 是子边 path 的前缀）。
- 对 069 rewrite 范围内的后代 future slices：当祖先链从 `E` 起改变后，不允许出现“父边 path 已变但后代仍保留旧前缀”的跨切片失真。

## 5. 接口契约 (API Contracts)
> 本计划本身不新增对外 HTTP API；它约束 066 的 Service 写入口实现方式与错误码。

### 5.1 Service 层接口（对齐 066 建议）
在 066 中实现（或扩展）：
- `DeleteEdgeSliceAndStitch(tenantID, hierarchyType, childNodeID, targetEffectiveDate, reasonCode, reasonNote)`（名称以 066 最终决策为准）

### 5.2 错误码（v1 固化）
- `422 ORG_PREFLIGHT_TOO_LARGE`：复用 069（影响行数超限，拒绝在线写入；必须零副作用回滚）。
- `422 ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`：**本计划新增**（禁止删除最早 edge slice；message：`cannot delete the first edge slice (no previous slice to stitch)`）。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 触发条件（统一判定口径）
对 `org_edges` 而言，写操作只要满足下述条件之一，就视为“从 `E` 起改变该 child 的祖先链”，必须触发 prefix rewrite：
- 删除某个 edge slice `T` 并缝补到上一片 `P`（066 / merge-into-prev），其效果等价于：从 `E=T.effective_date` 起，child 的父链恢复为 `P` 所代表的父链。
- （扩展项，见 6.5）调整 edge slice 的 `effective_date` / shift boundary 导致“生效开始日”变化。

### 6.2 Rewrite 参数取值（必须可复现）
统一用 069 的 A 原语（COUNT + UPDATE prefix rewrite）：
- `E`：变更生效日（删除/缝补场景为 `T.effective_date`）。
- `old_prefix`：变更前，该 child 在 as-of=`E` 的 `org_edges.path`（删除场景为 `T.path`）。
- `new_prefix`：变更后，该 child 在 as-of=`E` 的 `org_edges.path`（**必须用 `LockEdgeAt(as-of=E)` 读回**）。

### 6.3 Preflight（预算）与失败策略（对齐 069）
- 预算对象：以 rewrite UPDATE 将要更新的 `org_edges` 行数（rows）为口径，COUNT 与 UPDATE 的 WHERE 条件必须一致（避免 drift）。
- 初始阈值：与 069 保持一致（`maxEdgesPathRewrite=5000`）。
- 超限行为：返回 `422 ORG_PREFLIGHT_TOO_LARGE`，message 至少包含 `affected_edges/limit/effective_date/child_node_id`。

### 6.4 删除/缝补（066 merge-into-prev）的事务内伪代码（v1 固化）
> 目标：保证“先预算、再改动、再 rewrite”，并确保 `new_prefix` 读取方式无歧义。

1. 开启事务；获取 066 的 timeline 锁（key：`(tenant_id,hierarchy_type,child_node_id)`）。
2. 锁定目标 edge 切片 `T`（`effective_date == target_effective_date`，`FOR UPDATE`），并保存：
   - `E = T.effective_date`
   - `old_prefix = T.path`
3. 锁定上一片 `P`（同一时间线 key，且满足 `P.end_date + 1 day == T.effective_date`，`FOR UPDATE`）。
4. 校验：`P` 必须存在；否则返回 `422 ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`（事务回滚）。
5. **Preflight（在任何写入前执行）**：
   - `affected_edges = CountDescendantEdgesNeedingPathRewriteFrom(fromDate=E, oldPrefix=old_prefix)`
   - 若 `affected_edges > limit`：返回 `422 ORG_PREFLIGHT_TOO_LARGE`（事务回滚，无任何数据改动）。
6. 执行 066 的 stitch 写入（同一事务）：
   - `DELETE T`
   - `UPDATE P SET end_date = T.end_date`（延长窗口；merge-into-prev）
7. 读回最终 `new_prefix`：`new_prefix = LockEdgeAt(as-of=E).Path`（以读回为准）。
8. 执行 prefix rewrite：`RewriteDescendantEdgesPathPrefixFrom(fromDate=E, oldPrefix=old_prefix, newPrefix=new_prefix)`。
9. 写入审计日志/发布事件/提交事务（均按 066 计划模板执行）。

### 6.5 扩展：若 066 v1 落地“edge 边界调整”能力
> 本节仅声明扩展点：若 066 v1 真的实现该入口，本计划需补充完整伪代码与验收；否则 v1 不实现、不评审、不承诺。

- 触发条件：任何会改变 `parent_node_id` 的有效期语义、或使“从某天起祖先链变化”的边界调整操作。
- 参数口径：`E = min(old_start, new_start)`；`old_prefix/new_prefix` 必须在事务内通过 as-of 查询读回（避免依赖“猜测”）。

## 7. 安全与鉴权 (Security & Authz)
- 本计划不新增 policy；066 若对外暴露 API，应沿用 org 模块既有权限模型与审计策略。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - 依赖 066 的 `DeleteEdgeSliceAndStitch` 写入口落地（本计划只定义其必须履行的 069 一致性契约）。
  - 依赖 069 已存在的 repo 原语（COUNT + prefix rewrite）作为复用点。
- **里程碑**：
  1. [ ] 066 的 edge 删除入口接入 069 prefix rewrite（含 preflight 事务顺序）。
  2. [ ] 错误码 `ORG_CANNOT_DELETE_FIRST_EDGE_SLICE` 在 066 中落地，并同步写入 066 文档契约。
  3. [ ] 新增集成测试覆盖回归与失败路径（见验收标准）。
  4. [ ] Readiness：补齐 `docs/dev-records/DEV-PLAN-069B-READINESS.md` 并通过门禁（Go + doc gate）。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] 回归测试：构造 `root -> X -> Y -> Z`，先对 `Z` 生成 future slice，再对 `Y` 执行 066 删除/缝补，使祖先链从 `E` 起变化；验证：
  - `pkg/orglabels.ResolveOrgNodeLongNamesAsOf` 的结果不缺段（长名称包含完整祖先链）。
  - 069 巡检（父边 path 为子边 path 前缀）为 0。
- [ ] 失败路径：当 preflight 超限触发 `ORG_PREFLIGHT_TOO_LARGE` 时：
  - `org_edges` 时间线（child 的切片）与子树 future slices 的 `path/depth` 均无任何改动（零副作用）。
- [ ] 失败路径：删除最早切片返回 `ORG_CANNOT_DELETE_FIRST_EDGE_SLICE`，且不会产生“结构缺口”。

## 10. 运维与监控 (Ops & Monitoring)
- 本项目仍处于早期阶段，不引入额外监控/开关。
- 回滚策略：与 066 一致，优先代码回滚；不引入自动化数据回滚脚本（如出现误修复，使用巡检/修复任务继续收敛）。
