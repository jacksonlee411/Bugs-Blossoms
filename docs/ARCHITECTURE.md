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
- 管理员直接维护生效（015C）：`docs/runbooks/authz-policy-apply-api.md`

## 4. Person（人员）数据与迁移

- sqlc：`docs/runbooks/person-sqlc.md`
- Atlas+Goose：`docs/runbooks/person-atlas-goose.md`

## 5. 已废弃/遗留表（归档保留）

以下表在 DEV-PLAN-040 删除 `modules/finance`/`modules/billing`/`modules/crm`/`modules/projects` 后，可能仍存在于数据库中（仅作历史数据归档保留）：

- 原则：新代码不得新增对这些表的依赖；如需清理/归档/重基线，必须另立 dev-plan 并给出回滚策略。
- 清单（按模块，非穷尽）：
  - Finance：`counterparty`、`counterparty_contacts`、`inventory`、`expense_categories`、`money_accounts`、`transactions`、`expenses`、`payment_categories`、`payments`、`debts`、`payment_attachments`、`expense_attachments`
  - Billing：`billing_transactions`
  - CRM：`clients`、`client_contacts`、`chats`、`chat_members`、`messages`、`message_media`、`message_templates`
  - Projects：`projects`、`project_stages`、`project_stage_payments`
- 参考：`docs/dev-plans/040-remove-finance-billing-crm.md`

## 6. 历史快照（Archived）

归档文档用于保留决策快照，不作为活体 SSOT：

- `docs/Archived/index.md`
