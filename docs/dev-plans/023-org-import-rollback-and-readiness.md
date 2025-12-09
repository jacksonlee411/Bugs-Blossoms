# DEV-PLAN-023：Org 导入/回滚脚本与 Readiness

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 3，承接 021（schema 落地）与 022（占位表/事件契约），需提供导入/导出/回滚脚本雏形并完成 readiness（lint/test），为 024 CRUD 主链上线前做数据与执行面准备。

## 目标
- 提供可运行的导入/回滚脚本雏形（含 dry-run）、最小对账报告输出。
- `make check lint` 与 `go test ./modules/org/...`（或相关路径）通过，失败有回滚/清理方案。

## 范围与非目标
- 范围：为 org 主链提供导入/导出/回滚脚本（含 dry-run）、最小对账报告、验证命令与记录；覆盖 021/022 已落地的表（org_nodes/edges/positions/assignments、org_attribute_inheritance_rules、org_roles、org_role_assignments、change_requests 占位）。
- 非目标：不实现最终 UI、审批/流程、矩阵/继承逻辑执行，只提供数据导入与清理脚本；不交付生产级自动化灰度，仅最小可用路径。

## 依赖与里程碑
- 依赖：基于 DEV-PLAN-021 schema 与 DEV-PLAN-022 占位/契约，确保导入数据与现有约束一致；在 024 CRUD 主链前完成 readiness。
- 里程碑（按提交时间填充）：脚本雏形 -> dry-run 与对账 -> lint/test 通过 -> 记录更新。

## 设计决策
- 脚本形态与入口：放置于 `scripts/org/`（如 `import.sh`/`export.sh`/`rollback.sh`），支持 `--input`/`--tenant`/`--dry-run`/`--window`（有效期）/`--tables`（覆盖范围）参数；默认 dry-run，不带 `--apply` 不落库。
- 数据格式：统一使用 CSV（每张表一文件，UTF-8，无 BOM），在 README/usage 中给出模板列顺序与示例；导出生成同格式，便于回放与对账，避免 CSV/JSON 双栈维护。
- 数据覆盖：导入/导出涵盖 `org_nodes`、`org_edges`、`positions`、`org_assignments`、`org_attribute_inheritance_rules`、`org_roles`、`org_role_assignments`、`change_requests` 草稿；必要时可通过 `--tables` 选择子集。
- 校验与对账：导入前执行唯一/重叠/父子/租户校验（复用 SQL/视图）；导入后生成对账报告（节点数、边数、有效期冲突数、导入/跳过/失败计数），输出到 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 或临时文件再汇总。
- 幂等与回滚：导入以幂等键（tenant_id + code/id）驱动，重复导入仅更新相同窗口数据；`rollback.sh` 接受 `--tenant`+`--token`/`--since`（快照或时间窗）删除导入批次，必须提供 dry-run 与确认提示，批次 token 写入日志/对账报告便于追溯。
- Schema/版本兼容：执行前检查 `modules/org/infrastructure/atlas/schema.hcl` 与对应迁移已落地（`atlas migrate status --env dev` 无待应用），与 021/022 生成的 schema rev 一致，若检测到差异或 pending 迁移则拒绝导入并提示先同步。
- Readiness 检查：最小集为 `make check lint` + `go test ./modules/org/...`（按影响路径可缩小范围），必要时补充 `make db lint` 以验证迁移可回滚；所有命令与结果记录到文档。

## 任务清单与验收标准
1. [ ] 脚本雏形：在 `scripts/org/` 提供 `import.sh`/`export.sh`/`rollback.sh`（含 `--dry-run`/`--apply`/`--tables`/`--tenant`/`--window`），README 或脚本内 `usage` 说明 CSV 模板（列顺序/示例）、样例命令、默认 dry-run。验收：本地 dry-run 跑通并输出校验结果，不修改数据库。
2. [ ] 数据校验与对账：实现导入前校验（唯一/重叠/父子/租户）与导入后报告（导入/跳过/失败计数、冲突明细）。验收：报告写入 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md`，含时间戳、输入源、批次 token；如使用临时文件需在记录中引用。
3. [ ] 幂等与回滚路径：导入支持重复执行不破坏既有数据；`rollback.sh` 支持按 tenant+时间窗/批次 token 清理导入批次并提供 dry-run。验收：给出示例命令、风险提示，dry-run 输出记录到 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md` 或 023 readiness 文档。
4. [ ] Schema/版本前置校验：脚本执行前校验 atlas 状态/迁移 rev（基于 021/022 生成的版本），若存在 pending 或 schema diff 则中止并提示同步；记录校验结果。验收：校验命令与结果写入 `docs/dev-records/DEV-PLAN-023-READINESS.md`。
5. [ ] Readiness：执行 `make check lint` 与 `go test ./modules/org/...`（或影响子路径），必要时 `make db lint` 验证迁移可上下行；记录命令、耗时、结果到 `docs/dev-records/DEV-PLAN-023-READINESS.md`，确保 `git status --short` 干净。

## 验证记录
- 将导入/校验/回滚/测试命令与结果写入 `docs/dev-records/DEV-PLAN-023-READINESS.md`，对账报告写入或链接至 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md`；执行后确认 `git status --short` 干净。

## 风险与回滚/降级路径
- 导入风险：错误数据可能破坏唯一/时间约束，默认 dry-run 并提示差异；失败后可用 `rollback.sh --dry-run` 预览，再带确认参数执行清理，必要时配合 `make db migrate down`（针对 org 迁移目录，勿使用 HRM 标志位）撤回最新迁移。
- 依赖风险：若 021/022 schema 变动未同步，导入脚本需检查 schema 版本或报错退出；README 中注明兼容版本与如何更新迁移。

## 交付物
- `scripts/org/import.sh`/`export.sh`/`rollback.sh`（含 usage/dry-run/示例命令）。
- 对账报告与命令记录（`docs/dev-records/DEV-PLAN-020-ORG-PILOT.md`）。
- Readiness 记录（`docs/dev-records/DEV-PLAN-023-READINESS.md`，含 lint/test/db lint/回滚 dry-run 结果）。
