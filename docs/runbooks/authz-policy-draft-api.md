# Authz Policy Draft API（策略草稿）操作手册

本手册聚焦“如何通过 `/core/api/authz/**` 创建/审批/调试策略草稿”，用于研发自助申请权限与策略变更的日常操作。

## 前置说明

- 修改 `config/access/policies/**` 后运行 `make authz-pack`（会生成 `config/access/policy.csv` 与 `config/access/policy.csv.rev`，不要手动编辑聚合文件）。
- Authz 相关改动的本地门禁请以 `AGENTS.md` 为准（例如 `make authz-test` / `make authz-lint`）。

## 常用端点

- `GET /core/api/authz/policies`：当前聚合策略列表（仅 `Authz.Debug` 可见）。
- `POST /core/api/authz/requests`：创建策略草稿。
- `POST /core/api/authz/requests/{id}/approve|reject|cancel|trigger-bot|revert`：草稿生命周期操作。
- `GET /core/api/authz/debug?subject=...&object=...&action=...`：调试接口（仅 `Authz.Debug` 可见，支持 `attr.<key>=<value>` 注入 ABAC 属性；默认限流 20 req/min/IP）。

## 403 契约（HX/REST 统一）

未经授权返回 `application/json`，字段包含：

- `error/object/action/subject/domain/missing_policies/suggest_diff/request_url/debug_url`

## 示例：创建策略草稿

```bash
REVISION=$(jq -r .revision config/access/policy.csv.rev)
curl -X POST http://localhost:3200/core/api/authz/requests \
  -H "Content-Type: application/json" \
  -b sid=<session_cookie> \
  -d '{
    "object": "core.users",
    "action": "read",
    "reason": "Grant reporting access",
    "base_revision": "'"$REVISION"'",
    "diff": [{"op":"add","path":"/p/-","value":["role:reporting","core.users","read","global","allow"]}]
  }'
```

## 示例：Authz Debug（建议携带 request id）

```bash
curl "http://localhost:3200/core/api/authz/debug?subject=role:core.superadmin&domain=global&object=core.users&action=list&attr.ip=10.0.0.1" \
  -H "X-Request-Id: authz-debug-sample" \
  -b sid=<session_cookie>
```

## Bot 流程

Bot 的启动、锁处理、应急流程请见 `docs/runbooks/AUTHZ-BOT.md`。

