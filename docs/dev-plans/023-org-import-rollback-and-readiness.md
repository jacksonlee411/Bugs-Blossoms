# DEV-PLAN-023：Org 导入/回滚脚本与 Readiness

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 3，需要导入/导出/回滚脚本雏形，并完成开发前 readiness（lint/test）。

## 目标
- 提供可运行的导入/回滚脚本雏形，支持最小验证。
- `make check lint` 与 `go test ./modules/org/...`（或相关路径）通过，失败有回滚方案。

## 实施步骤
1. [ ] 编写初版导入/导出脚本及回滚脚本（含 dry-run 选项），覆盖核心表。
   - 如需基准/种子验证，先提交 bench/seed 脚本空壳，保持与 020 的性能/校验占位一致。
2. [ ] 执行 `make check lint`，修正问题。
3. [ ] 执行 `go test ./modules/org/...` 或当前路径的等效测试，记录结果/时间戳。
4. [ ] 记录失败场景下的回滚/清理命令。
5. [ ] 导入后输出对账报告并更新 `docs/dev-records/DEV-PLAN-020-ORG-PILOT.md`，保留命令与结果记录。

## 交付物
- 导入/回滚脚本雏形与使用说明。
- readiness 结果记录（lint/test/命令）。
