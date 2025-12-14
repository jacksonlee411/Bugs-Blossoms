## Gemini 使用说明（薄封装）

> 仓库级规则、变更触发器矩阵与文档门禁以 `AGENTS.md` 为主干 SSOT。本文件仅保留 Gemini/DevHub 工具相关提示，避免与 `AGENTS.md` 重复。

## DevHub（devhub.yml）辅助说明

DevHub 是本仓库的开发服务编排入口，配置见 `devhub.yml`。

### 常用工作流

- 服务未启动/端口不通：先对照 `devhub.yml` 的端口与依赖关系
- 模板或 CSS watcher 异常：优先重启对应 watcher（按 `devhub.yml` 的定义执行）

## 文档与规则入口

- 统一规则与门禁：`AGENTS.md`
- 贡献者入口：`docs/CONTRIBUTING.MD`
- 文档收敛方案：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`

