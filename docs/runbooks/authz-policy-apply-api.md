# Authz Policy Apply API（015C）

本手册描述 015C 之后的授权管理口径：**管理员直接维护生效策略**，不再存在“requests/审批/bot”链路。

## 端点

- `GET /core/api/authz/policies`：读取当前生效策略（需 `Authz.Debug`）。
- `POST/DELETE /core/api/authz/policies/stage`：暂存变更（UI 体验用，需 `Authz.Requests.Write`）。
- `POST /core/api/authz/policies/apply`：将变更**直接写入** `AUTHZ_POLICY_PATH`（默认 `config/access/policy.csv`）并更新 `.rev`，随后 `ReloadPolicy`（需 `Authz.Requests.Write`）。
- `GET /core/api/authz/debug`：诊断策略命中（需 `Authz.Debug`，带限流）。

## 典型 UI 流程（推荐）

1. 进入 `/roles/{id}/policies` 或 `/users/{id}/policies`
2. 暂存 add/remove 变更（Stage）
3. 点击 `Apply now/立即生效`

## 传统打包流程（仍保留）

修改 `config/access/policies/**` 后仍可运行：

- `make authz-pack`
- `make authz-test && make authz-lint`

用于生成/校验基线聚合文件（`config/access/policy.csv` 与 `config/access/policy.csv.rev`）。

