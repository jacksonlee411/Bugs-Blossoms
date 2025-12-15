# DEV-PLAN-021：Org 核心表与约束

**状态**: 进行中（2025-12-13 更新）

## 进度速记
- ✅ 范围/目标/约束已定稿（单租户单树、ltree 防环、EXCLUDE 防重叠/双亲、唯一根）。
- 🔄 工具链与迁移仍待落地（Atlas 配置、迁移生成、lint/上下行验证未执行）。
- 🆕 补充两项行动：落地执行记录模板、补充本地 DSN/env 示例便于跑 atlas/goose。

## 范围与输入
- 覆盖 020 计划步骤 1 的 schema 落地，限定在单一 Organization Unit 树 + Position + Assignment 主链（不含编制/矩阵/角色，占位留给 022+）。
- 有效期统一使用 UTC、半开区间 `[effective_date, end_date)`；所有约束/索引均带 `tenant_id`，PostgreSQL 17，需启用 `ltree` 与 `btree_gist` 扩展。

## 时间约束与 end_date 管理（评审补充）
- **时间约束类型**：采用 Valid Time / Effective Dating。物理存储为 `effective_date/end_date` 两列，语义为 UTC 半开区间 `[effective_date, end_date)`；在约束/索引/查询中用 `tstzrange(effective_date, end_date)` 表达该区间（无需额外存储 range 字段）。
- **DB 层强约束**：通过 `GiST + EXCLUDE USING gist (..., tstzrange(...) WITH &&)` 在数据库层强制“同键区间不重叠”（同节点 slice、防双亲、同父同窗重名、编码时效唯一等），并用 `check (effective_date < end_date)` 防止非法区间。
- **end_date 管理策略（M1）**：`end_date` 默认 `'9999-12-31'`；写路径按 Insert（Update）语义处理时**仅接受 `effective_date`**，由系统自动计算 `end_date = 下一片段的 effective_date（若存在）否则 9999-12-31`，并在同一事务内截断当前覆盖片段的 `end_date = effective_date` 后插入新片段 `[effective_date, end_date)`，以避免人工计算导致的 overlap/gap 并保留未来排程。实现细节与并发锁顺序见 `docs/dev-plans/025-org-time-and-audit.md`。
- **是否可自动管理**：可以且建议强制自动管理。原因：时态数据一旦允许客户端显式写 `end_date`，极易引入重叠/空档并污染审计；M1 统一通过 Service 层加锁与算法计算维护时间线连续性（“无空档”口径至少适用于 `org_node_slices` 强约束实体，其它时间片表默认只强制无重叠，是否无空档按业务语义收敛）。

## 目标
- 使用 Atlas 描述式 schema + Goose 迁移生成核心表与约束（EXCLUDE 防重叠、ltree 防环、防双亲、code 唯一、同父同窗重名）。
- 迁移上/下行可执行，`make db lint`（atlas lint）通过。
- Schema 层落地“单租户单树 + 唯一根”，并支持 OrgNode 以时间片方式演进（为 025 的 Insert/ShiftBoundary 提供键基础），`parent_hint` 与 `org_edges` 一致性可校验。

## Schema 明细（Atlas → Goose）
- 目录：`modules/org/infrastructure/atlas/schema.hcl`（声明式）；迁移输出 `migrations/org/changes_<unix>.{up,down}.sql`，state 文件 `migrations/org/atlas.sum`；`atlas.hcl` 需新增 org 环境（dev/test/ci 复用 `DB_*`，`dir` 指向 `migrations/org`，`dev` 可用 `ATLAS_DEV_DB_NAME`）。
- 公共列：
  - 实体表：`tenant_id uuid not null`、`id uuid pk`、`created_at/updated_at timestamptz default now()`。
  - 时间片表：`tenant_id uuid not null`、`effective_date timestamptz not null`、`end_date timestamptz not null default '9999-12-31'`、`created_at/updated_at timestamptz default now()`；检查 `effective_date < end_date`；`tstzrange(effective_date, end_date)` 采用 `[,)`。
- `org_nodes`（实体表）：**稳定标识**，用于被 `org_edges/positions` 外键引用。字段：`id uuid pk`、`type text check in ('OrgUnit')`、`code varchar(64)`、`is_root bool default false`。约束：`unique (tenant_id, code)`；`unique (tenant_id) where is_root` 保证单租户唯一根（M1 不支持根随时间迁移）。索引：`btree (tenant_id, code)`。
- `org_node_slices`（时间片表）：**可演进时间片**，写路径以 slice 为单位（Insert/ShiftBoundary/Correct）。字段：`id uuid pk`、`org_node_id uuid not null`、`name varchar(255)`、`i18n_names jsonb default '{}'`、`status text check in ('active','retired','rescinded')`、`legal_entity_id uuid null`、`company_code text null`、`location_id uuid null`、`display_order int default 0`、`parent_hint uuid null`、`manager_user_id uuid null`、有效期列。约束：`fk (tenant_id, org_node_id) -> org_nodes on delete restrict`；`fk (tenant_id, parent_hint) -> org_nodes on delete restrict`（允许 null）；`exclude using gist (tenant_id with =, org_node_id with =, tstzrange(effective_date, end_date) with &&)` 防同一节点时间片重叠；`exclude using gist (tenant_id with =, parent_hint with =, lower(name) with =, tstzrange(effective_date, end_date) with &&)` 防同父同窗重名；`check (parent_hint is null or parent_hint <> org_node_id)` 防自环；`fk (tenant_id, manager_user_id) -> core.users (tenant_id, id) on delete restrict`（`core.users` 为示例路径）。索引：`btree (tenant_id, org_node_id, effective_date)`、`gin index on i18n_names`。
- `org_edges`：`id uuid pk`、`hierarchy_type text default 'OrgUnit' check`、`parent_node_id uuid not null`、`child_node_id uuid not null`、`path ltree`、`depth int`、有效期列。约束：`fk (tenant_id,parent_node_id) -> org_nodes on delete restrict` / `fk (tenant_id,child_node_id) -> org_nodes on delete restrict`；`exclude using gist (tenant_id with =, child_node_id with =, tstzrange(effective_date, end_date) with &&)` 防双亲；`check (parent_node_id <> child_node_id)`；触发器维护 `path/depth` 并拒绝 `path @> subpath(child)` 形成环；索引：`gist (tenant_id, path)`、`btree index on (tenant_id, parent_node_id, effective_date)`、`btree index on (tenant_id, child_node_id, effective_date)`。
- **时态完整性（Service 层/触发器增强）**：虽然 FK 保证了 parent_node_id 存在，但需补充校验“父节点在 Edge 有效期内必须处于 Active 状态”。M1 阶段在 Service 写操作中强制检查：`Edge.interval ⊆ Parent.ActiveIntervals`。若父节点退休（Retire），需校验是否存在有效子边，存在则拒绝（M1 策略）或级联截断。
- `positions`：`id uuid pk`、`org_node_id uuid not null`、`code varchar(64)`、`title text`、`status text check in ('active','retired','rescinded')`、`is_auto_created bool default false`、有效期列。约束：`fk (tenant_id, org_node_id) -> org_nodes on delete restrict`；`exclude using gist (tenant_id with =, code with =, tstzrange(effective_date, end_date) with &&)` 保证职位编码在租户内带时效唯一，允许多岗位。索引：`btree index on (tenant_id, org_node_id, effective_date)`。
- `org_assignments`：`id uuid pk`、`position_id uuid not null`、`subject_type text default 'person' check in ('person')`、`subject_id uuid not null`、`pernr text`、`assignment_type text default 'primary' check in ('primary','matrix','dotted')`、`is_primary bool default true`（校验与 assignment_type 一致）以及有效期列。约束：`fk (tenant_id, position_id) -> positions on delete restrict`；`exclude using gist (tenant_id with =, subject_type with =, subject_id with =, assignment_type with =, tstzrange(effective_date, end_date) with &&) where assignment_type = 'primary'` 保证同主体仅一个 primary；`exclude using gist (tenant_id with =, position_id with =, tstzrange(effective_date, end_date) with &&)` 保证同 Position 同窗仅一个占用（矩阵可后续特性开关放宽）。索引：`btree index on (tenant_id, subject_id, effective_date)`、`btree index on (tenant_id, position_id, effective_date)`。
- 扩展：迁移 `up` 需 `create extension if not exists ltree; create extension if not exists btree_gist;`，`down` 保持幂等（不删除扩展）。

## 约束实现要点（含设计决策）
- 触发器与移动策略：`org_edges` 触发器在 `INSERT` 时读取父节点 path，拼接 `path/depth`，并在写前检查 `new_path` 是否形成环；禁止直接 `UPDATE parent_node_id`，移动节点通过“将旧边失效、创建新边”实现，触发器需覆盖该流程的子树更新与防环兜底。
- 时间线（无空档）：数据库 EXCLUDE 兜底“无重叠”，服务层在新增时间片时需加锁当前有效记录、截断 `end_date` 后插入新片段，保持“无空档”。OrgNode 使用 `org_node_slices` 做时间片演进，`org_nodes` 仅承载稳定标识（供外键引用）；`Correct/Update/ShiftBoundary` 的写语义在 025 定义并实现。
- 根节点创建：统一通过内部 API `POST /org/api/tenants/{tenant_id}/root-node` 创建首个根节点（示例 payload：`{code,name,effective_date}`），若租户已存在根节点则返回冲突；如需初始租户种子，由 seeding 脚本调用同一 API，避免绕过业务校验。
- 外键与软删：所有 FK 采用 `ON DELETE RESTRICT` / 默认 `RESTRICT`，与软删 `status='rescinded'` 一致，禁止硬删被引用记录，强制走业务归档。
- 查询性能与索引：GiST EXCLUDE 保证约束；B-Tree 索引覆盖 `positions` 按 org_node_id+有效期、`org_assignments` 按 subject_id/position_id+有效期、`org_edges` 按 parent_node_id/child_node_id+有效期，path 查询走 GiST。

## 实施步骤
1. [ ] 目录/配置：创建 `modules/org/infrastructure/atlas/`、`migrations/org/`；更新根 `atlas.hcl` 增加 org 环境（dev/test/ci，dir `migrations/org`，state `migrations/org/atlas.sum`，URL 复用 `DB_*`）。如需临时库，设置 `ATLAS_DEV_DB_NAME`。
2. [ ] Schema 描述：在 `modules/org/infrastructure/atlas/schema.hcl` 写明上述表/约束/扩展/索引（按聚合拆分 include 亦可），保持 `(tenant_id, …)` 复合键。
3. [ ] 生成迁移：`atlas migrate diff --env dev --dir file://migrations/org --to file://modules/org/infrastructure/atlas/schema.hcl`，产出 `changes_<unix>.{up,down}.sql` 与 `atlas.sum`。命令执行前确保 Postgres 可连（`DB_*`/`ATLAS_DEV_DB_NAME` 已导出）。
4. [ ] 触发器实现与测试：编写并测试 `org_edges` PL/pgSQL 触发器，覆盖环路拒绝、`path/depth` 维护、直接 `UPDATE parent_node_id` 被拒、移动节点（失效旧边+新边）后子树更新。
5. [ ] 根节点初始化：实现 `POST /org/api/tenants/{tenant_id}/root-node` API，定义请求/响应与冲突返回；如需种子，复用该 API，禁止绕过业务校验。
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
