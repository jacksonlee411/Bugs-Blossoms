## 变更说明

<!-- 请用 2-5 句说明：做了什么、为什么做、影响范围 -->

## 自检清单（提交前）

- [ ] 已按变更类型执行必要命令（见 `AGENTS.md` 的“变更触发器矩阵”）
- [ ] 未修改冻结模块：`modules/billing`、`modules/crm`、`modules/finance`
- [ ] `.templ`/Tailwind 生成物已提交（如适用：`make generate && make css` + `git status --short` 为空）

## 路由策略（DEV-PLAN-018）

- [ ] 新增路由已归类到明确命名空间：UI（`/{module}/...`）/ 内部 API（`/{module}/api/*`）/ 对外 API（`/api/v1/*`）/ webhook / ops / test / dev-only
- [ ] 新增顶层例外/legacy 前缀已同步更新 `config/routing/allowlist.yaml`（否则 route-lint 会失败）
- [ ] `/{module}/api/*` 与 `/api/v1/*` 不返回 HTML（JSON-only，包括 403/404/405/500）
- [ ] 如引入 dev-only 路由（如 `/_dev/*`、`/playground`），已受 `ENABLE_DEV_ENDPOINTS` 开关保护（默认生产关闭）

