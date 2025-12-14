# IOTA SDK - Claude 使用说明（薄封装）

> 仓库级规则、变更触发器矩阵与文档门禁以 `AGENTS.md` 为主干 SSOT。本文件仅保留 Claude 场景下的“任务编排/协作提示”，避免与 `AGENTS.md` 重复。

## 目的

- 让 Claude 在接到需求后，快速把任务拆分到正确的代码区域与执行顺序。
- 约束：不要在此复制命令/门禁细节；需要时直接引用 `AGENTS.md` 与 `Makefile`。

## 任务规模 → 执行策略

- 单纯阅读（1–3 文件）：直接读代码/文档即可，不引入复杂编排。
- 小改动（1–5 文件）：先定位入口与影响面，再一次性完成实现与自测。
- 中等特性（6–15 文件）：先列出依赖与影响范围，优先把“契约/接口/迁移”定下来。
- 跨模块/大改（15+ 文件）：先写实施计划（dev-plan），再分阶段合并，避免超大 PR。

## 变更边界提示

- 真实“命令/端口/服务编排”以 `Makefile` 与 `devhub.yml` 为准；文档不写死。
- 模板/样式/多语言/Authz/HRM 等专项工作流，按 `AGENTS.md` 的触发器矩阵执行本地门禁。
- 避免阅读 `*_templ.go`（由 templ 生成，信息噪声大）。

## 常见入口索引（按目录）

- 主服务入口：`cmd/server/main.go`
- Superadmin 入口：`cmd/superadmin/main.go`
- DevHub 编排：`devhub.yml`
- 文档收敛方案：`docs/dev-records/DEV-RECORD-001-DOCS-AUDIT.md`

