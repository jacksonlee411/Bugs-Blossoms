# DEV-PLAN-021：Org 核心表与约束

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 计划步骤 1，需先落地 `org_nodes`、`org_edges`、`positions`、`org_assignments` 等核心表，并在数据库层确保有效期/层级/唯一性约束。

## 目标
- 通过 Atlas/Goose 生成并验证迁移，包含 EXCLUDE 重叠约束、ltree 防环、防双亲、code/name 唯一。
- 迁移上下行可用，`make db lint` 通过。
- 所有唯一/EXCLUDE/外键约束均按 `(tenant_id, …)` 复合键落地，确保多租户隔离与查询路径一致。
- Schema 层体现“单租户单树 + 唯一根”约束，`parent_hint` 仅作缓存字段，写路径需能校验与 `org_edges` 一致。

## 实施步骤
1. [ ] 依据 020 口径编写 atlas schema 与 migration，生成 core 表及索引/约束（EXCLUDE、ltree、唯一）。
2. [ ] 执行 `make db lint`（含 `atlas migrate lint`）验证迁移规范。
3. [ ] 在本地运行 `make db migrate up HRM_MIGRATIONS=1` 与 `make db migrate down HRM_MIGRATIONS=1`，记录命令/结果/时间戳。
4. [ ] 若涉及 sqlc/atlas 生成，执行 `make generate`/`make sqlc-generate` 并确保 `git status --short` 干净。

## 交付物
- 迁移文件（上下行可用）、更新的 schema.hcl。
- readiness 记录（lint、上下行命令与结果）。
