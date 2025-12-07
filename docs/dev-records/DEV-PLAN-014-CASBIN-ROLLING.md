# DEV-PLAN-014：`AUTHZ_ENFORCE` 灰度与回滚记录

按照 014 计划，Core → HRM → Logging 各阶段启用 `AUTHZ_ENFORCE` 前后都需要登记启停命令、受影响租户与观测指标。本文件提供统一模板，便于追踪分批灰度与回滚演练。

| 时间 (UTC) | 模块 × 租户 | 操作 | 命令 / Flag | 观测与验证 | 结果 |
|------------|-------------|------|-------------|-------------|------|
| _TODO_ | Core × dev 租户 | `shadow → enforce` | `AUTHZ_ENFORCE=core-dev`（示例） | `scripts/authz/verify --tenant ...`，关键流程未出现 4xx/5xx | _待记录_ |
| 2025-12-05 00:01 | Core × dev 租户 | Shadow readiness（UI capability gating） | `AUTHZ_MODE=shadow go test ./modules/core/...` | Users/Roles/Groups 页面操作、Quick Links、Spotlight 入口均依据 `pageCtx.CanAuthz` 渲染，403 统一输出 Unauthorized 组件；未观察到 legacy fallback diff | ✅ |
| 2025-12-05 11:10 | HRM × dev 租户 | 无灰度，直接 enforce | 按 AGENTS.md，不开启 shadow/回滚；使用当前 `policy.csv.rev` | HRM 页面已接入 `pageCtx.CanAuthz`，缺权用户 e2e（nohrm@example.com）返回 403 + Unauthorized 组件；CI#98 通过 | ✅（灰度 N/A） |

> 记录建议：
> 1. 操作列包含“启用 shadow/enforce”“关闭 flag”“回滚（git revert + flag reset）”等；如有多条命令，可在“命令 / Flag”列换行列出。
> 2. 观测列务必写明验证手段（`scripts/authz/verify`、HTMX 页面自测、日志/指标截图等）以及异常排查结论。
> 3. 若触发回滚，请追加新的行描述回滚动作，并在“结果”列注明“✅ 回滚成功”或“❌ 持续观察中”。

## 014D 公共层差距清单

| 模块/文件 | 位置 | 遗留点 | 计划动作 | 状态 |
|-----------|------|--------|----------|------|
| HRM / modules/hrm/module.go | Quick Link 注册 | Quick Link 仍保留 RequirePermissions 兜底 | 移除 legacy .RequirePermissions，仅保留 .RequireAuthz，并验证有/无权限账号可见性 | Done（移除兜底，待验证可见性） |
| Logging / modules/logging/presentation/controllers/authz_helpers.go | writeForbiddenResponse JSON | 403 JSON 仅输出 message/缺失 MissingPolicies，未包含 subject/domain/object/action/request/debug URL，字段命名与 014D 契约不一致 | 调整 403 payload 为统一结构（含 object/action/subject/domain/missing_policies/suggest_diff/request_url/debug_url），HX/REST 一致 | Done（单测覆盖 403 JSON 字段） |
| Core / modules/core/presentation/templates/pages/users/users.templ | 用户列表模板 | 仍存在 user.CanUpdate 组合判断，未完全移除 legacy 路径 | 用 pageCtx.CanAuthz 控制可见性/交互，移除 legacy CanUpdate 判定 | Done（模板与生成物已更新） |
| 文档 | README/CONTRIBUTING/AGENTS | 未纳入 014D 统一的 403 JSON 示例与 /core/api/authz/requests|debug 调用示例 | 补充公共层接入流程、示例 payload/链接，完成后记录命令与验证结论 | Done |
| 验证记录 | dev-records 本文件 | 导航/Quick Links/Spotlight 有/无权账号验证与 403 直访结果尚未登记 | 以有/无能力账号各验证一次可见性与 403 payload，填入本表或日志区 | Done（014D 完成：Core Nav 仅保留 AuthzObject/Action，QuickLinks capability 单测覆盖 allow/deny，Forbidden JSON + Unauthorized props 三模块统一） |

## 日志模块记录

| 时间 (UTC) | 模块 × 租户 | 操作 | 命令 / Flag | 观测与验证 | 结果 |
|------------|-------------|------|-------------|-------------|------|
| 2025-12-07 15:45 | Logging × all | repo/service authz guard 单测补齐（无租户/无用户/无权限/成功路径，ACTION_LOG_ENABLED 开启降级验证） | `go test ./modules/logging/...` | List/Count/Create 在缺租户或未登录时直接返回 `AUTHZ_FORBIDDEN`，仓储不触达 DB；ActionLog audit 在开启时遇到无 DB/无用户安全跳过；403 JSON/HX 继续输出 MissingPolicies/SuggestDiff/subject/domain/debug_url | ✅ |
| 2025-12-07 09:30 | Logging × all | controller/service/repo 单测，403 JSON 契约与审计降级验证（ACTION_LOG_ENABLED=true 场景覆盖） | `GOCACHE=/tmp/go-cache go test ./modules/logging/...`（`AUTHZ_MODE=enforce` + 含无 Session/无租户/无权限用例）；<br/>`GOCACHE=/tmp/go-cache make authz-lint` | 403 JSON 返回 subject/domain/missing_policies/suggest_diff/debug_url；无租户 fallback `global`，ActionLogMiddleware/unauthorized 审计在无 DB/未登录时安全跳过；authentication_logs/action_logs 写入均强制 tenant_id，policy pack 与 fixtures parity 通过 | ✅ |
| 2025-12-06 16:55 | Logging × all | 清理 core 平行 authlog；ActionLogMiddleware 保持默认关闭；新增 session handler 单测 | `go test ./modules/core/...`；`go test ./modules/logging/...`（`ACTION_LOG_ENABLED` 默认 false） | session.CreatedEvent 仅由 logging handler 写入 authentication_logs；action_logs 仅在 `ACTION_LOG_ENABLED=true` 时写入 | ✅ |
| 2025-12-06 09:35 | Logging × all | 014C 文档登记：补充已完成项与下一步待办（core 复用 logging 仓储、action_logs 开关、session handler 单测） | 文档更新 | 014C 计划已标记已完成项，并集中列出剩余待办，后续按表推进 | ✅（文档已同步） |

- Logging 授权/审计说明：
  - authentication_logs 缺少 tenant_id 时强制使用请求上下文的租户，避免跨租户写入；action_logs 同步使用 tenant_id、method/path/IP/UA。
  - 导出/清理：仓储 List/Count 支持 tenant + 时间窗口过滤，可直接复用过滤器导出；保留期建议默认 90 天（示例：`DELETE FROM authentication_logs WHERE tenant_id=$1 AND created_at < now() - interval '90 days';`），尚无自动任务，可由定时任务调用。
  - action_logs/Loki：`ACTION_LOG_ENABLED` 默认关闭；开启后写库失败降级为结构化日志。Mock/关闭路径：设置 env 为 false 或在 handler/service 注入 stub 仓储，Loki 采集保持可选。
  - 回滚/监控：`config/access/authz_flags.yaml` 已声明 logging 分段；回滚时先停用 AUTHZ_ENFORCE + ACTION_LOG_ENABLED，再视情况 revert policy pack，监控 403 比例与 action_logs 写失败率。
  - 权限种子/fixtures：`pkg/defaults.AllPermissions` 已包含 `logging.logs:view`（Logs.View），e2e/seed 的默认用户获取全量权限，`nohrm@example.com` 通过过滤前缀去除 logging 权限；policy 片段位于 `config/access/policies/logging/logs.csv` 并随 `make authz-pack` 校验。
  - Forbidden/审计：`ensureLoggingAuthz` 在 REST/HTMX/JSON 统一输出 object/action/subject/domain/missing_policies/suggest_diff/request_url/debug_url，未授权时记录结构化日志并在开启 `ACTION_LOG_ENABLED` 且有 DB/用户时写入 action_logs。
