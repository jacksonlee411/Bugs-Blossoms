# DEV-PLAN-002：DevContainer 退场与本地原生开发准备

## 背景
受宿主硬件限制（`.devcontainer/devcontainer.json:113-117` 要求 8 vCPU/32 GB/64 GB），DevContainer 无法稳定运行，需要切换为本地原生开发流程。本计划列出所需准备工作，确保开发体验与容器内一致。

## 准备事项
- **工具链同步**  
  - 安装 Go 1.24.x（与 `go.mod`、Dockerfile 同步）与 Node.js 20（推荐 `nvm` 管理），并手动安装 Tailwind CLI v3.4.15。  
  - 运行 `go install` 安装 templ@v0.3.857、air@v1.61.5、golangci-lint@v1.64.8、goimports@v0.31.0、goda@v0.5.11、devhub@v0.0.2，保持与 `.devcontainer/Dockerfile:7-13` 相同的工具集。
- **系统依赖**  
  - 在宿主系统安装 `make`、`postgresql-client`、`redis-tools` 等 CLI（`.devcontainer/Dockerfile:15-20`），并保留 Docker Engine/Compose 以运行依赖服务；如需要 tailscale，单独安装命令行客户端。
- **VS Code 扩展与设置**  
  - 手动在 VS Code 中安装 `.devcontainer/devcontainer.json:37-88` 列出的扩展及格式化/Lint 配置（Go 语言服务器、templ 格式化、Tailwind 支持等），确保编辑体验一致。
- **环境初始化**  
  - 在项目根运行 `go mod download`、`npm install -g @anthropic-ai/claude-code`（视实际需求），并将 `.env.example` 复制为 `.env` 后配置数据库、Redis、端口等变量，此前由 `postCreateCommand` 自动执行。
- **后台服务**  
  - 利用 `docker compose -f compose.dev.yml up -d` 启动 Postgres（5438）和 Redis（6379），挂载 `sdk-data`、`sdk-redis` 卷（`compose.dev.yml:1-27`）。如改用宿主服务，请在 `.env` 中调整连接信息，并执行 `make db seed|migrate` 处理数据。
- **Git 安全设置**  
  - 运行 `git config --global --add safe.directory /home/shangmeilin/Bugs-Blossoms`（`postStartCommand` 原处理步骤），避免 VS Code/Git 针对多用户目录的安全提示。

完成上述准备后，可继续使用 `air -c .air.toml`、`make dev watch`、`make css` 等命令在本地直接开发。***
