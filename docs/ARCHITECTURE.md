# Architecture（活体）

本文件用于承载可持续维护的“架构约定与关键链接”，作为 `AGENTS.md` 的补充参考；避免把实现细节散落在多个文档导致漂移。

## 1. 分层与约束

- 模块分层遵循 DDD：`modules/{module}/{domain,infrastructure,services,presentation}/`
- 依赖约束由 `.gocleanarch.yml` 定义，执行入口为 `make check lint`

## 2. UI 技术栈

- Server-side rendering：Templ
- 交互：HTMX + Alpine.js
- 样式：Tailwind CSS（生成入口见 `Makefile`）

## 3. Authz（Casbin）

- 策略碎片：`config/access/policies/**`
- 聚合产物：`config/access/policy.csv` 与 `config/access/policy.csv.rev`（由 `make authz-pack` 生成）
- Bot/应急：`docs/runbooks/AUTHZ-BOT.md`

## 4. HRM 数据与迁移

- sqlc：`docs/runbooks/hrm-sqlc.md`
- Atlas+Goose：`docs/runbooks/hrm-atlas-goose.md`

## 5. 历史快照（Archived）

归档文档用于保留决策快照，不作为活体 SSOT：

- `docs/Archived/index.md`
