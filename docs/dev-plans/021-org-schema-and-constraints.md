# DEV-PLAN-021：Org 核心表与约束

**状态**: 进行中（2025-12-09 更新）

## 进度速记
- ✅ 范围/目标/约束已定稿（单租户单树、ltree 防环、EXCLUDE 防重叠/双亲、唯一根）。
- 🔄 工具链与迁移仍待落地（Atlas 配置、迁移生成、lint/上下行验证未执行）。
- 🆕 补充两项行动：落地执行记录模板、补充本地 DSN/env 示例便于跑 atlas/goose。

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
- `org_nodes`：`id uuid pk`、`type text check in ('OrgUnit')`、`code varchar(64)`、`name varchar(255)`、`i18n_names jsonb default '{}'`、`status text check in ('active','retired','rescinded')`、`legal_entity_id uuid null`、`company_code text null`、`location_id uuid null`、`display_order int default 0`、`parent_hint uuid null`、`manager_user_id uuid null`。约束：`unique (tenant_id, code)`；`exclude using gist (tenant_id with =, parent_hint with =, lower(name) with =, tstzrange(effective_date, end_date) with &&)` 防同父同窗重名；`unique (tenant_id) where parent_hint is null` 保证唯一根；`check (parent_hint is null or parent_hint <> id)` 防自环；`fk (tenant_id, manager_user_id) -> core.users (tenant_id, id) on delete restrict`（`core.users` 为示例路径）。索引：`gin index on i18n_names`。
- `org_edges`：`id uuid pk`、`hierarchy_type text default 'OrgUnit' check`、`parent_node_id uuid not null`、`child_node_id uuid not null`、`path ltree`、`depth int`、有效期列。约束：`fk (tenant_id,parent_node_id) -> org_nodes on delete restrict` / `fk (tenant_id,child_node_id) -> org_nodes on delete restrict`；`exclude using gist (tenant_id with =, child_node_id with =, tstzrange(effective_date, end_date) with &&)` 防双亲；`check (parent_node_id <> child_node_id)`；触发器维护 `path/depth` 并拒绝 `path @> subpath(child)` 形成环；索引：`gist (tenant_id, path)`、`btree index on (tenant_id, parent_node_id, effective_date)`、`btree index on (tenant_id, child_node_id, effective_date)`。
- `positions`：`id uuid pk`、`org_node_id uuid not null`、`code varchar(64)`、`title text`、`status text check in ('active','retired','rescinded')`、`is_auto_created bool default false`、有效期列。约束：`fk (tenant_id, org_node_id) -> org_nodes on delete restrict`；`exclude using gist (tenant_id with =, code with =, tstzrange(effective_date, end_date) with &&)` 保证职位编码在租户内带时效唯一，允许多岗位。索引：`btree index on (tenant_id, org_node_id, effective_date)`。
- `org_assignments`：`id uuid pk`、`position_id uuid not null`、`subject_type text default 'person' check in ('person')`、`subject_id uuid not null`、`pernr text`、`assignment_type text default 'primary' check in ('primary','matrix','dotted')`、`is_primary bool default true`（校验与 assignment_type 一致）以及有效期列。约束：`fk (tenant_id, position_id) -> positions on delete restrict`；`exclude using gist (tenant_id with =, subject_type with =, subject_id with =, assignment_type with =, tstzrange(effective_date, end_date) with &&) where assignment_type = 'primary'` 保证同主体仅一个 primary；`exclude using gist (tenant_id with =, position_id with =, tstzrange(effective_date, end_date) with &&)` 保证同 Position 同窗仅一个占用（矩阵可后续特性开关放宽）。索引：`btree index on (tenant_id, subject_id, effective_date)`、`btree index on (tenant_id, position_id, effective_date)`。
- 扩展：迁移 `up` 需 `create extension if not exists ltree; create extension if not exists btree_gist;`，`down` 保持幂等（不删除扩展）。

## 约束实现要点（含设计决策）
- 触发器与移动策略：`org_edges` 触发器在 `INSERT` 时读取父节点 path，拼接 `path/depth`，并在写前检查 `new_path` 是否形成环；禁止直接 `UPDATE parent_node_id`，移动节点通过“将旧边失效、创建新边”实现，触发器需覆盖该流程的子树更新与防环兜底。
- 时间线（无空档）：数据库 EXCLUDE 兜底“无重叠”，服务层在新增时间片时需加锁当前有效记录、截断 `end_date` 后插入新片段，保持“无空档”。`Correct` 与 `Update` 的区分沿用该事务模式。
- 根节点创建：统一通过 API `POST /org/tenants/{tenant_id}/root-node` 创建首个根节点（示例 payload：`{code,name,effective_date}`），若租户已存在根节点则返回冲突；如需初始租户种子，由 seeding 脚本调用同一 API，避免绕过业务校验。
- 外键与软删：所有 FK 采用 `ON DELETE RESTRICT` / 默认 `RESTRICT`，与软删 `status='rescinded'` 一致，禁止硬删被引用记录，强制走业务归档。
- 查询性能与索引：GiST EXCLUDE 保证约束；B-Tree 索引覆盖 `positions` 按 org_node_id+有效期、`org_assignments` 按 subject_id/position_id+有效期、`org_edges` 按 parent_node_id/child_node_id+有效期，path 查询走 GiST。

## 实施步骤
1. [ ] 目录/配置：创建 `modules/org/infrastructure/atlas/`、`migrations/org/`；更新根 `atlas.hcl` 增加 org 环境（dev/test/ci，dir `migrations/org`，state `migrations/org/atlas.sum`，URL 复用 `DB_*`）。如需临时库，设置 `ATLAS_DEV_DB_NAME`。
2. [ ] Schema 描述：在 `modules/org/infrastructure/atlas/schema.hcl` 写明上述表/约束/扩展/索引（按聚合拆分 include 亦可），保持 `(tenant_id, …)` 复合键。
3. [ ] 生成迁移：`atlas migrate diff --env dev --dir file://migrations/org --to file://modules/org/infrastructure/atlas/schema.hcl`，产出 `changes_<unix>.{up,down}.sql` 与 `atlas.sum`。命令执行前确保 Postgres 可连（`DB_*`/`ATLAS_DEV_DB_NAME` 已导出）。
4. [ ] 触发器实现与测试：编写并测试 `org_edges` PL/pgSQL 触发器，覆盖环路拒绝、`path/depth` 维护、直接 `UPDATE parent_node_id` 被拒、移动节点（失效旧边+新边）后子树更新。
5. [ ] 根节点初始化：实现 `POST /org/tenants/{tenant_id}/root-node` API，定义请求/响应与冲突返回；如需种子，复用该 API，禁止绕过业务校验。
6. [ ] Lint：运行 `make db lint` 或 `atlas migrate lint --env ci --git-base origin/main --dir file://migrations/org`，保证无破坏性/依赖问题。
7. [ ] 上下行验证：使用 goose 执行 `goose -dir migrations/org postgres "$DSN" up` / `goose -dir migrations/org postgres "$DSN" down`（$DSN 复用 `postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable`），记录时间戳与输出。
8. [ ] 生成物清理：若触发 `make generate`/`make sqlc-generate`，执行后确认 `git status --short` 干净。
9. [ ] 记录模板：在 `docs/dev-records/DEV-PLAN-021-READINESS.md` 起草执行记录模板（命令、耗时、结果、日志路径），供 lint/上下行验证复用。
10. [ ] 环境示例：在文档补充本地/CI 环境变量示例（`DB_*`/`ATLAS_DEV_DB_NAME`/`HRM_MIGRATIONS` 无需改动，但明确 org 迁移的 DSN 和 `atlas migrate diff --env dev` 调用姿势）。

## 交付物与验收
- 更新后的 `modules/org/infrastructure/atlas/schema.hcl`、`migrations/org/changes_<unix>.{up,down}.sql`、`migrations/org/atlas.sum`。
- `make db lint` 与 goose 上下行的执行记录（命令、开始/结束时间、结果）落盘，如 `docs/dev-records/DEV-PLAN-021-READINESS.md`。
- **验收用例**:
  - 约束校验：重名/重叠/双亲写入被 `EXCLUDE` 约束阻断。
  - 触发器与移动策略校验：`org_edges` 上的环路写入被触发器拒绝；直接 `UPDATE parent_node_id` 的操作被拒绝；通过“失效旧边+创建新边”的模式移动节点后，子树的 `path` 和 `depth` 正确更新。
  - 根节点校验：为同一租户创建第二个根节点的 API 请求被 `unique` 约束拒绝。
  - `parent_hint` 失配写入被拒；验证 SQL/测试脚本随记录提交。
