# Authz Bot Runbook

本 Runbook 记录 Authz Bot 在手动/应急场景下的操作步骤、凭证要求与回滚方法，适用于 DEV-PLAN-015A 的 Stage Epsilon 运维项。

## 前置条件
- 数据库：本地或 CI Postgres（默认 `localhost:5432`，用户/密码 `postgres`）已启动且完成 `make db migrate up`。
- GitHub：具备 `repo` 权限的 PAT，导出为环境变量：
  ```bash
  export AUTHZ_BOT_GITHUB_TOKEN=<PAT>
  export AUTHZ_BOT_GIT_TOKEN="x-access-token:<PAT>"   # remote 仍为 HTTPS/SSH 时用于推送
  ```
- 工作树干净：执行 `git status --short` 无 diff。若 `scripts/authz/bot.sh run` 过程中临时生成文件，请在结束后恢复/清理。

## 常用命令
- 单次运行（抓取一个草稿）：
  ```bash
  scripts/authz/bot.sh run --once
  ```
  脚本会自动执行：
  1. `make authz-pack && make authz-test`
  2. 应用 JSON diff、回写 `applied_policy_revision/snapshot`
  3. 推送分支并调用 GitHub API 创建 PR
- 连续运行（监听直到中断）：
  ```bash
  scripts/authz/bot.sh run
  ```
- 释放卡死锁（`bot_lock` 长时间占用）：
  ```bash
  scripts/authz/bot.sh force-release <request-id>
  ```
  该命令会清空 `policy_change_requests.bot_lock/bot_locked_at`，适用于 `exit status 128` 等推送失败场景。

## 故障处理
- **推送/PR 失败**：检查远程 URL 是否被注入了过期 token；必要时执行 `git remote set-url origin https://github.com/...` 后重新导出 `AUTHZ_BOT_GIT_TOKEN`。
- **`error_log` 非空**：通过 `psql` 查询请求，若 `status=failed` 但 diff 仍有效，可手动 `UPDATE ... SET status='approved', error_log=NULL` 再次运行 bot。
- **缓存策略**：`make authz-pack`/`authz-test` 会复用 `go env GOPATH` 和 `downloaded/tailwindcss` 等缓存；在 CI 中建议启用 Go/Tailwind 缓存以减少重复下载。
- **保持工作树干净**：脚本执行完毕会删除临时分支；若中途退出请手动 `git checkout feature/dev-plan-015a && git branch -D authz/bot/...` 并确认 `git status` 为空。

## 回滚流程
1. 如需撤销某次策略变更，可直接在 GitHub PR 中点击 `Revert`，或手动 `git revert <merge-commit>`。
2. 运行 `scripts/authz/bot.sh run --once` 处理 revert 草稿；bot 会生成新的 PR 并回写 `applied_policy_snapshot`。
3. 若必须从已有请求生成逆向草稿，可调用 `/core/api/authz/requests/{id}/revert`（需要 `Authz.Requests.Review` 权限）。

## 审计与记录
- 每次 bot 运行完成后，请在 `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 的表格中记录 `request_id/pr_link/locker/result`。
- 若手动运行时跳过 CI，可补执行 `make authz-test`、`make authz-lint`、`go test ./cmd/authzbot` 确认代码仍通过质量门槛。
