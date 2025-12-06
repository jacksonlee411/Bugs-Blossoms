# DEV-PLAN-014：`AUTHZ_ENFORCE` 灰度与回滚记录

按照 014 计划，Core → HRM → Logging 各阶段启用 `AUTHZ_ENFORCE` 前后都需要登记启停命令、受影响租户与观测指标。本文件提供统一模板，便于追踪分批灰度与回滚演练。

| 时间 (UTC) | 模块 × 租户 | 操作 | 命令 / Flag | 观测与验证 | 结果 |
|------------|-------------|------|-------------|-------------|------|
| _TODO_ | Core × dev 租户 | `shadow → enforce` | `AUTHZ_ENFORCE=core-dev`（示例） | `scripts/authz/verify --tenant ...`，关键流程未出现 4xx/5xx | _待记录_ |
| 2025-12-05 00:01 | Core × dev 租户 | Shadow readiness（UI capability gating） | `AUTHZ_MODE=shadow go test ./modules/core/...` | Users/Roles/Groups 页面操作、Quick Links、Spotlight 入口均依据 `pageCtx.CanAuthz` 渲染，403 统一输出 Unauthorized 组件；未观察到 legacy fallback diff | ✅ |
| 2025-12-05 11:10 | HRM × dev 租户 | 无灰度，直接 enforce | 按 AGENTS.md，不开启 shadow/回滚；使用当前 `policy.csv.rev` | HRM 页面已接入 `pageCtx.CanAuthz`，缺权用户 e2e（nohrm@example.com）返回 403 + Unauthorized 组件；CI#98 通过 | ✅（灰度 N/A） |

## 014D 公共层差距清单

| 模块/文件 | 位置 | 遗留点 | 计划动作 | 状态 |
|-----------|------|--------|----------|------|
| HRM / modules/hrm/module.go | Quick Link 注册 | Quick Link 仍保留 `RequirePermissions` 兜底 | 移除 legacy `.RequirePermissions`，仅保留 `.RequireAuthz`，并验证有/无权限账号可见性 | Done（移除兜底，待验证可见性） |
| Logging / modules/logging/presentation/controllers/authz_helpers.go | `writeForbiddenResponse` JSON | 403 JSON 仅输出 message/缺失 MissingPolicies，未包含 subject/domain/object/action/request/debug URL，字段命名与 014D 契约不一致 | 调整 403 payload 为统一结构（含 object/action/subject/domain/missing_policies/suggest_diff/request_url/debug_url），HX/REST 一致 | Done（403 JSON/HX 统一，含 MissingPolicies/SuggestDiff） |
| 文档 | README/CONTRIBUTING/AGENTS | 未纳入 014D 统一的 403 JSON 示例与 `/core/api/authz/requests|debug` 调用示例 | 补充公共层接入流程、示例 payload/链接，完成后记录命令与验证结论 | Done（补充统一 403 示例与指引） |
| 验证记录 | dev-records 本文件 | 导航/Quick Links/Spotlight 有/无权账号验证与 403 直访结果尚未登记 | 以有/无能力账号各验证一次可见性与 403 payload，填入本表或日志区；当前 e2e 种子仅区分 HRM 权限，需额外创建无 `logging.logs:view` 的账号用于验证 | In progress（待无日志权限账号验证并登记） |

> 记录建议：
> 1. 操作列包含“启用 shadow/enforce”“关闭 flag”“回滚（git revert + flag reset）”等；如有多条命令，可在“命令 / Flag”列换行列出。
> 2. 观测列务必写明验证手段（`scripts/authz/verify`、HTMX 页面自测、日志/指标截图等）以及异常排查结论。
> 3. 若触发回滚，请追加新的行描述回滚动作，并在“结果”列注明“✅ 回滚成功”或“❌ 持续观察中”。
