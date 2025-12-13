# DEV-PLAN-025：Org 时间约束与审计

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 5，承接 021（schema 与约束）、022（事件契约）、023（导入/回滚）、024（CRUD 主链），需落地有效期校验、防重叠/防空档/防环，区分 Correct/Update，支持 Rescind，并执行冻结窗口审计与事件/审计补充字段。

## 目标
- 写入前有效期/层级校验完整，冻结窗口（默认月末+3 天，可租户覆盖）生效。
- Correct/Update/Rescind 行为审计可见，可区分变更类型。
- 审计/事件含 transaction_time/version/initiator 等字段，便于回放与对账。

## 范围与非目标
- 范围：OrgNode/OrgEdge/Position/OrgAssignment 的有效期校验（无重叠/无空档/无环）、冻结窗口校验、Correct/Update/Rescind 行为与审计记录、事件附带 transaction_time/version/initiator/EffectiveWindow。
- 非目标：不实现并行版本/retro 重放、不引入 workflow 审批，只提供写路径校验与审计；不调整 matrix/dotted 逻辑（仍按 024 默认关闭）。

## 依赖与里程碑
- 依赖：021 约束模型与有效期字段、022 事件契约字段、023 导入/回滚基线、024 CRUD 主链实现；遵守 020 冻结窗口策略。
- 里程碑（按提交时间填充）：校验与冻结窗口实现 -> Correct/Update/Rescind 分支与审计 -> 事件补充字段 -> 测试/性能/ready 记录。

## 设计决策
- 校验范围：写入时校验 OrgNode/OrgEdge/Position/OrgAssignment 的 `effective_date/end_date` 半开区间、无重叠、无空档（适用于强约束口径）、无环（OrgEdge/Node 一致）；同一 subject primary 唯一。
- 冻结窗口：默认“月末+3 天”拒绝修改历史（可租户覆盖），服务层统一检查，返回明确错误码/信息并记录审计。
- Correct vs Update：M1 阶段 Update **仅接受 `effective_date`**（禁止显式提交 `end_date`）；系统按 **Insert 语义** 自动计算 `end_date` 为“下一片段 `effective_date`（若存在）或 `9999-12-31`”，从设计上避免重叠并强制保留未来排程。Correct 原位修改需更高权限与审计标记（change_type=Correct），且不得变更时间字段。两者均需区分 initiator 与 transaction_time。
- ShiftBoundary（边界移动）：为解决“录错生效日期”的高频场景，M1 允许通过**原子地移动相邻时间片交界线**来纠错 `effective_date`：同步修改 `P.end_date` 与 `T.effective_date`，保持 `P.end_date == T.effective_date`，物理上避免 Gap/Overlap（非并发）。限制：禁止吞没前驱（`new_effective_date <= P.effective_date`）、禁止倒错（`new_effective_date >= T.end_date`）；冻结校验按受影响区间（`min(old_start, new_start)`）执行；写入必须加锁同一时间线并在同一事务内完成。该能力依赖 021 的 OrgNode 时间片键（如 `org_node_slices`）。
- Rescind：提供软撤销（状态标记 + 审计），与 Retire 区分；Rescind 需权限校验与事件记录。
- 审计与事件：审计记录包含 transaction_time、version、initiator_id、change_type、old/new snapshot；事件补充 `transaction_time`/`initiator`/`entity_version`/`effective_window`，对齐 022 契约，幂等键沿用 event_id/sequence。
- 性能与缓存：校验查询使用现有索引/视图，避免递归 CTE 热路径；必要时增加针对 `tstzrange` 的 GiST 索引检查，确保性能不退化。
- 权限与上下文：所有校验需建立在 Session+租户上下文；无 Session/tenant 直接拒绝。

### Update（Insert）算法（M1）
1. 令 `X = effective_date`。
2. 定位“当前片段” `S`：找到覆盖 `X` 的片段（`S.effective_date <= X < S.end_date`）；若找不到则拒绝（`ORG_GAP`/`ORG_NOT_FOUND_AT_DATE`，按实现选其一但必须稳定）。
3. 定位“下一片段” `N`：找到 `X` 之后最早片段（`N.effective_date > X`）。
4. 计算新片段结束时间 `Y`：
   - 若存在 `N`：强制 `Y = N.effective_date`（自动衔接，保留未来排程）。
   - 否则 `Y = 9999-12-31`。
5. 同一事务写入：若 `S.effective_date < X`，先将 `S.end_date` 截断为 `X`，再插入新片段 `[X, Y)`。
备注：由于 `end_date` 由系统自动计算且上限为 `N.effective_date`，非并发场景不会产生跨未来片段的重叠；并发冲突由锁/重试与 DB 兜底处理。

### ShiftBoundary（边界移动）算法（M1）
1. 输入：`target_slice_id`（要调整的时间片）与 `new_effective_date`。
2. 锁定并读取目标片段 `T`（`SELECT ... FOR UPDATE`），令 `old_start = T.effective_date`。
3. 基础校验：
   - `new_effective_date < T.end_date`，否则拒绝（倒错）。
4. 查找并锁定前驱片段 `P`：同一时间线中满足 `P.end_date == old_start` 的片段（并发下必须加锁，避免同时修改同一边界）。
5. 若存在 `P`：
   - 吞没校验：`new_effective_date > P.effective_date`，否则拒绝（吞没/合并不在 M1 支持范围）。
   - 冻结校验：若 `min(old_start, new_effective_date)` 落在冻结窗口内则拒绝（具体口径复用冻结窗口策略）。
   - 变更顺序（避免触发 EXCLUDE 重叠）：
     - Move Earlier（`new_effective_date < old_start`）：先更新 `P.end_date = new_effective_date`（缩短前驱），再更新 `T.effective_date = new_effective_date`。
     - Move Later（`new_effective_date > old_start`）：先更新 `T.effective_date = new_effective_date`（制造 gap），再更新 `P.end_date = new_effective_date`（填补 gap）。
6. 若不存在 `P`：
   - M1 默认仅支持 OrgNode/Position 等“允许调整首片开始”的时间线；对 OrgEdge/primary assignment 等强约束时间线（可能造成 orphan gap）默认拒绝并返回稳定错误码（避免隐式级联修改）。
7. 在同一事务内写审计：建议将两条记录以同一 `request_id` 串联，`change_type` 采用 `ShiftBoundary`（或 `Correct` + meta 标记）以便回放与排障。

## 任务清单与验收标准
1. [ ] 有效期/层级校验与冻结窗口：实现无重叠/无空档/无环校验与冻结窗口拒绝（可租户覆盖）。验收：测试覆盖正常写入、重叠拒绝、空档拒绝、环检测、冻结期拒绝。
2. [ ] Correct/Update/Rescind 分支与审计：区分 Update（Insert，仅提交 `effective_date`，`end_date` 自动计算）与 Correct（原位，需更高权限且不改时间字段）、Rescind（软撤销）并写审计（transaction_time/version/initiator/change_type/old/new）。验收：测试覆盖三类操作含权限路径与审计字段断言，并覆盖 Update 在存在未来片段时自动衔接 `end_date=N.effective_date`、保留未来排程且不触发 `ORG_OVERLAP`（非并发）。
3. [ ] ShiftBoundary：实现边界移动原子操作（至少覆盖 OrgNode 时间片；必要时扩展到 OrgEdge/primary assignment），含吞没/倒错/冻结校验与并发锁顺序，并写双审计。验收：测试覆盖 Move Earlier/Move Later、吞没拒绝、倒错拒绝、冻结窗口命中/放行、并发冲突返回稳定错误码；同时断言审计中两条记录可串联（同 request_id/transaction）。
4. [ ] 事件补充字段：事件 payload 补充 transaction_time/initiator/entity_version/effective_window，对齐 022 契约，幂等键 event_id/sequence 生效。验收：事件生成测试验证字段与幂等。
5. [ ] 性能校验：复用 020/027 基准或新增 bench，确认校验/查询在 1k 节点数据集下性能不退化（<200ms 读取基线，写入校验不超预期）；记录命令与结果。验收：记录在 `docs/dev-records/DEV-PLAN-025-READINESS.md`。
6. [ ] Readiness：执行 `make check lint`、`go test ./modules/org/...`（或影响路径），必要时 `make db lint`/`make check tr`；记录命令/耗时/结果到 `docs/dev-records/DEV-PLAN-025-READINESS.md`。

## 验证记录
- 在 `docs/dev-records/DEV-PLAN-025-READINESS.md` 记录校验/事件/性能/测试命令与结果，执行后确认 `git status --short` 干净。

## 风险与回滚/降级路径
- 性能风险：校验查询可能引入性能退化，需基准验证并提供降级方案（例如关闭空档校验或放宽窗口开关）。
- 行为风险：错误区分 Correct/Update 可导致数据覆盖；需通过权限与审计确保可追溯，必要时提供 Rescind 回滚。
- 发布回滚：如校验逻辑导致大面积拒绝，可临时关闭冻结窗口或回滚到 024 的简单 CRUD 版本；若迁移有影响，使用 org 迁移目录执行 `make db migrate down`（注意不影响 HRM）。

## 交付物
- 时间/审计能力代码与测试。
- 事件补充字段与校验。
- 冻结窗口策略与性能基准记录。
- Readiness 记录（`docs/dev-records/DEV-PLAN-025-READINESS.md`）。
