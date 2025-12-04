# DEV-PLAN-014：`AUTHZ_ENFORCE` 灰度与回滚记录

按照 014 计划，Core → HRM → Logging 各阶段启用 `AUTHZ_ENFORCE` 前后都需要登记启停命令、受影响租户与观测指标。本文件提供统一模板，便于追踪分批灰度与回滚演练。

| 时间 (UTC) | 模块 × 租户 | 操作 | 命令 / Flag | 观测与验证 | 结果 |
|------------|-------------|------|-------------|-------------|------|
| _TODO_ | Core × dev 租户 | `shadow → enforce` | `AUTHZ_ENFORCE=core-dev`（示例） | `scripts/authz/verify --tenant ...`，关键流程未出现 4xx/5xx | _待记录_ |

> 记录建议：
> 1. 操作列包含“启用 shadow/enforce”“关闭 flag”“回滚（git revert + flag reset）”等；如有多条命令，可在“命令 / Flag”列换行列出。
> 2. 观测列务必写明验证手段（`scripts/authz/verify`、HTMX 页面自测、日志/指标截图等）以及异常排查结论。
> 3. 若触发回滚，请追加新的行描述回滚动作，并在“结果”列注明“✅ 回滚成功”或“❌ 持续观察中”。
