# DEV-PLAN-002：DevContainer 退场与本地原生开发准备

**状态**: 已完成（2025-11-30 12:17）

## 背景
受宿主硬件限制（`.devcontainer/devcontainer.json:113-117` 要求 8 vCPU/32 GB/64 GB），DevContainer 无法稳定运行，需要切换为本地原生开发流程。本计划列出所需准备工作，确保开发体验与容器内一致。

## 实施步骤

1. [X] **工具链同步**  
   - 安装 Go 1.24.x（与 `go.mod`、Dockerfile 同步）与 Node.js 20（推荐 `nvm` 管理），并手动安装 Tailwind CLI v3.4.15。  
   - 运行 `go install` 安装 templ@v0.3.857、air@v1.61.5、golangci-lint@v1.64.8、goimports@v0.31.0、goda@v0.5.11、devhub@v0.0.2，保持与 `.devcontainer/Dockerfile:7-13` 相同的工具集。  
   - 验证：`go version`、`node -v`、`tailwindcss --version`、`templ -v`、`air -v` 等命令输出满足所需版本。

2. [X] **系统依赖**  
   - 在宿主系统安装 `make`、`postgresql-client`、`redis-tools` 等 CLI（`.devcontainer/Dockerfile:15-20`），并保留 Docker Engine/Compose 以运行依赖服务；如需要 tailscale，单独安装命令行客户端。  
   - 验证：`make --version`、`psql --version`、`redis-cli --version`、`docker --version`、`docker compose version`。

3. [X] **VS Code 扩展与设置**  
   - 手动在 VS Code 中安装以下扩展：`golang.go`、`a-h.templ`、`bradlc.vscode-tailwindcss`、`esbenp.prettier-vscode`、`redhat.vscode-yaml`、`mtxr.sqltools`、`mtxr.sqltools-driver-pg`、`mikestead.dotenv`、`foxundermoon.shell-format`、`EditorConfig.EditorConfig`、`eamodio.gitlens`、`usernamehw.errorlens`、`streetsidesoftware.code-spell-checker`，并保持与 `.devcontainer/devcontainer.json:37-88` 一致的格式化/Lint 设置，确保编辑体验一致。  
   - 验证：VS Code 中打开工作区后，检查已安装扩展列表，确认 `golang.go`、`a-h.templ`、`bradlc.vscode-tailwindcss` 等已启用。

4. [X] **环境初始化**  
   - 在项目根运行 `go mod download`、`npm install -g @anthropic-ai/claude-code`（视实际需求），并将 `.env.example` 复制为 `.env` 后配置数据库、Redis、端口等变量，此前由 `postCreateCommand` 自动执行。  
   - 验证：`go list ./...` 无缺失依赖，`.env` 文件存在且内容符合本地配置。

5. [X] **后台服务**  
   - 利用 `docker compose -f compose.dev.yml up -d` 启动 Postgres（5438）和 Redis（6379），挂载 `sdk-data`、`sdk-redis` 卷（`compose.dev.yml:1-27`）。如改用宿主服务，请在 `.env` 中调整连接信息，并执行 `make db seed|migrate` 处理数据。  
   - 验证：`docker ps` 可看到对应容器，应用可连接数据库/Redis；`make db seed` 成功执行。

6. [X] **Git 安全设置**  
   - 运行 `git config --global --add safe.directory /home/shangmeilin/Bugs-Blossoms`（`postStartCommand` 原处理步骤），避免 VS Code/Git 针对多用户目录的安全提示。  
   - 验证：`git config --global --get-all safe.directory` 包含项目路径。

完成上述准备后，可继续使用 `air -c .air.toml`、`make dev watch`、`make css` 等命令在本地直接开发。***
