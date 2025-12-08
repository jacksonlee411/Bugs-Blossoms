# DEV-PLAN-021：Org 核心表与约束

**状态**: 规划中（2025-01-15 12:00 UTC）

## 范围与输入
- 覆盖 020 计划步骤 1 的 schema 落地，限定在单一 Organization Unit 树 + Position + Assignment 主链（不含编制/矩阵/角色，占位留给 022+）。
- 有效期统一使用 UTC、半开区间 `[effective_date, end_date)`；所有约束/索引均带 `tenant_id`，PostgreSQL 17，需启用 `ltree` 与 `btree_gist` 扩展。

## 目标
- 使用 Atlas 描述式 schema + Goose 迁移生成核心表与约束（EXCLUDE 防重叠、ltree 防环、防双亲、code/name 唯一）。
- 迁移上/下行可执行，`make db lint`（atlas lint）通过。
- Schema 层落地“单租户单树 + 唯一根”，`parent_hint` 与 `org_edges` 一致性可校验。

## Schema 明细（Atlas → Goose）
- 目录：`modules/org/infrastructure/atlas/schema.hcl`（声明式）；迁移输出 `migrations/org/changes_<unix>.{up,down}.sql`，state 文件 `migrations/org/atlas.sum`；`atlas.hcl` 需新增 org 环境（dev/test/ci 复用 `DB_*`，`dir` 指向 `migrations/org`，`dev` 可用 `ATLAS_DEV_DB_NAME`）。
- 公共列：`tenant_id uuid not null`、`effective_date timestamptz not null`、`end_date timestamptz not null default '9999-12-31'`、`created_at/updated_at timestamptz default now()`；检查 `effective_date < end_date`；`tstzrange(effective_date, end_date)` 采用 `[,)`。
- `org_nodes`：`id uuid pk`、`type text check in ('OrgUnit')`、`code varchar(64)`、`name varchar(255)`、`i18n_names jsonb default '{}'`、`status text check in ('active','retired','rescinded')`、`legal_entity_id uuid null`、`company_code text null`、`location_id uuid null`、`display_order int default 0`、`parent_hint uuid null`、`manager_user_id uuid null`。约束：`unique (tenant_id, code)`；`exclude using gist (tenant_id with =, parent_hint with =, lower(name) with =, tstzrange(effective_date, end_date) with &&)` 防同父同窗重名；`unique (tenant_id) where parent_hint is null` 保证唯一根；`check (parent_hint is null or parent_hint <> id)` 防自环。
- `org_edges`：`id uuid pk`、`hierarchy_type text default 'OrgUnit' check`、`parent_node_id uuid not null`、`child_node_id uuid not null`、`path ltree`、`depth int`、有效期列。约束：`fk (tenant_id,parent_node_id)` / `(tenant_id,child_node_id)` → `org_nodes`；`exclude using gist (tenant_id with =, child_node_id with =, tstzrange(effective_date, end_date) with &&)` 防双亲；`check (parent_node_id <> child_node_id)`；触发器维护 `path/depth` 并拒绝 `path @> subpath(child)` 形成环；索引 `gist (tenant_id, path)`。
- `positions`：`id uuid pk`、`org_node_id uuid not null`、`code varchar(64)`、`title text`、`status text check in ('active','retired','rescinded')`、`is_auto_created bool default false`、有效期列。约束：`unique (tenant_id, code)`；`fk (tenant_id, org_node_id) -> org_nodes`；`exclude using gist (tenant_id with =, org_node_id with =, tstzrange(effective_date, end_date) with &&)` 限定同 OrgNode 有效期不重叠（确保一对一空壳可持续）。
- `org_assignments`：`id uuid pk`、`position_id uuid not null`、`subject_type text default 'person' check in ('person')`、`subject_id uuid not null`、`pernr text`、`assignment_type text default 'primary' check in ('primary','matrix','dotted')`、`is_primary bool default true`（校验与 assignment_type 一致）以及有效期列。约束：`fk (tenant_id, position_id) -> positions`；`exclude using gist (tenant_id with =, subject_type with =, subject_id with =, assignment_type with =, tstzrange(effective_date, end_date) with &&) where assignment_type = 'primary'` 保证同主体仅一个 primary；`exclude using gist (tenant_id with =, position_id with =, tstzrange(effective_date, end_date) with &&)` 保证同 Position 同窗仅一个占用（矩阵可后续特性开关放宽）。
- 扩展：迁移 `up` 需 `create extension if not exists ltree; create extension if not exists btree_gist;`，`down` 保持幂等（不删除扩展）。

## 约束实现要点
- `parent_hint` 校验：写入/更新 `org_nodes` 时通过触发器检查当前有效窗口下的父节点是否与 `org_edges` 一致，不一致则拒绝；`parent_hint` 仅缓存，不作为真相源。
- 防环/防双亲：`org_edges` 写入触发器根据父路径生成 `path/depth`，使用 ltree 函数阻断环；EXCLUDE 确保同一子节点在重叠窗口内仅有一条父边。
- 唯一根：`parent_hint is null` 的节点在同一租户唯一；迁移需要插入 root 时校验。
- 有效期：所有写路径在服务层应校验无重叠/无空档，EXCLUDE 提供数据库兜底。

## 实施步骤
1. [ ] 目录/配置：创建 `modules/org/infrastructure/atlas/`、`migrations/org/`；更新根 `atlas.hcl` 增加 org 环境（dev/test/ci，dir `migrations/org`，state `migrations/org/atlas.sum`，URL 复用 `DB_*`）。如需临时库，设置 `ATLAS_DEV_DB_NAME`。
2. [ ] Schema 描述：在 `modules/org/infrastructure/atlas/schema.hcl` 写明上述表/约束/扩展/索引（按聚合拆分 include 亦可），保持 `(tenant_id, …)` 复合键。
3. [ ] 生成迁移：`atlas migrate diff --env dev --dir file://migrations/org --to file://modules/org/infrastructure/atlas/schema.hcl`，产出 `changes_<unix>.{up,down}.sql` 与 `atlas.sum`。命令执行前确保 Postgres 可连（`DB_*`/`ATLAS_DEV_DB_NAME` 已导出）。
4. [ ] Lint：运行 `make db lint` 或 `atlas migrate lint --env ci --git-base origin/main --dir file://migrations/org`，保证无破坏性/依赖问题。
5. [ ] 上下行验证：使用 goose 直接执行 `goose -dir migrations/org postgres "$DSN" up` / `goose -dir migrations/org postgres "$DSN" down`（$DSN 复用 `postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable`），记录时间戳与输出；验证 parent/root/EXCLUDE/ltree 约束是否按预期拒绝异常数据。
6. [ ] 生成物清理：若触发 `make generate`/`make sqlc-generate`，执行后确认 `git status --short` 干净。

## 交付物与验收
- 更新后的 `modules/org/infrastructure/atlas/schema.hcl`、`migrations/org/changes_<unix>.{up,down}.sql`、`migrations/org/atlas.sum`。
- `make db lint` 与 goose 上下行的执行记录（命令、开始/结束时间、结果）落盘，如 `docs/dev-records/DEV-PLAN-021-READINESS.md`。
- 校验用例：重名/重叠/双亲/环路写入被 EXCLUDE/ltree 阻断；同租户仅一 root；`parent_hint` 失配写入被拒；验证 SQL/测试脚本随记录提交。
