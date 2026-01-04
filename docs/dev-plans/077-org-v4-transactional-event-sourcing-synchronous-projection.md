# DEV-PLAN-077：Org v4（事务性事件溯源 + 同步投射）完整方案（Greenfield）

**状态**: 草拟中（2026-01-04 02:03 UTC）

> 本计划是“干净/完整”的 v4 方案设计稿：以 **`org_events` 为 SoT**，以 **同步投射** 在同一事务内维护 **`org_unit_versions` 读模型**，并提供强一致读、可重放重建与并发互斥策略。  
> **暂不考虑迁移与兼容**：不要求与现有 `modules/org` 的 schema/API/事件契约兼容；也不提供双写/灰度/回滚路径（另开计划承接）。

## 0. 进度速记
1. [X] 明确 v4 目标边界（SoT=events，ReadModel=versions，Engine=DB，Safety=advisory lock，Rebuild=replay）。
2. [X] 输出完整 schema、核心 DB 函数、Go 事务调用形状、查询封装与运维重建流程（可直接编码，无猜测）。
3. [ ] （非本计划）迁移/兼容/灰度：必须另开子计划（建议 077A/077B），并遵守仓库红线（新增表需手工确认）。

## 1. 背景与上下文 (Context)
HR SaaS 的组织架构场景常见约束：
- 深层级（20+）、高频读取（树、祖先链、子树、长名称）、强一致性（写后立刻读到正确状态）。
- 强时态（Effective Dating）：同一节点/边在不同业务日拥有不同父链与名称。
- 写操作存在“子树级联影响”（Move 需要重算后代路径），需要强并发互斥以避免死锁与路径失真。

本计划以 “一次事务，双重写入（事件+投射），即时读取，随时重放” 为核心理念，定义一套可在 Postgres 17 上落地的 v4 架构。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 定义 **事件表 SoT**：`org_events`（append-only），记录业务意图与必要元数据（tenant/request/initiator/tx_time）。
- [ ] 定义 **读模型表**：`org_unit_versions`（ltree path + daterange validity + no-overlap），支持毫秒级 as-of 查询与子树/祖先链查询。
- [ ] 定义 **DB 投射引擎**：单入口函数 `submit_org_event(...)` + `apply_*_logic(...)`，在同一事务内完成事件写入与读模型更新。
- [ ] 定义 **并发安全策略**：Postgres advisory lock，串行化同一棵组织树的写入（fail-fast 可选）。
- [ ] 定义 **读模型封装**：`get_org_snapshot(...)`（含长名称拼接），提供“参数化视图”体验。
- [ ] 定义 **可重放重建**：提供 rebuild 流程（truncate versions + replay events）与安全守卫（维护锁/互斥）。

### 2.2 非目标（明确不做）
- 不考虑与现有 Org 模块的兼容、迁移与灰度（不做双写/回填/回滚策略）。
- 不覆盖岗位/任职/权限映射等扩展域（本计划仅定义 **OrgUnit 树（节点+父子关系+名称/状态）** 的 v4）。
- 不引入额外监控/开关切换（仓库原则：早期阶段避免过度运维；必要的健康信息在后续计划补齐）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划是设计稿；进入实施时，触发器与门禁以 `AGENTS.md` / `Makefile` / CI workflow 为 SSOT。本文只勾选“将会命中”的类别。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码（`AGENTS.md`）
  - [ ] DB 迁移 / Schema（新增表/函数/索引；按 Org 工具链门禁执行）
  - [ ] Outbox（若在实施阶段选择发布 integration events，则按 `DEV-PLAN-017`）
  - [X] 文档（本计划）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - 时间语义（Valid Time=DATE）：`docs/dev-plans/064-effective-date-day-granularity.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  API[OrgService (Go)] -->|Tx| DB[(Postgres)]
  DB --> E[(org_events\nSoT)]
  DB --> V[(org_unit_versions\nRead Model)]
  DB --> F[submit_org_event/apply_*_logic\n(plpgsql)]
  API -->|read| Q[get_org_snapshot / queries]
  Q --> V
```

### 3.2 关键设计决策（ADR 摘要）
1. **SoT=事件表（选定）**
   - `org_events` 为 append-only；所有写入必须通过 `submit_org_event`。
2. **同步投射（选定）**
   - 同一事务内：插入事件 + 更新 `org_unit_versions`；写后读取强一致。
3. **Valid Time（选定）**
   - 业务有效期使用 `date`；读模型使用 `daterange`（左闭右开 `[start,end)`），并用 EXCLUDE 约束防重叠。
4. **路径表示（选定）**
   - `node_path ltree`，label 使用 `uuid` 的 32 位 hex（`replace(lower(uuid::text), '-', '')`），避免 `-` 等非法字符（对齐仓库既有约定）。
5. **并发互斥（选定）**
   - 写入按“树维度”加 advisory lock：`pg_try_advisory_xact_lock(hashtext(lock_key))`（fail-fast）或 `pg_advisory_xact_lock`（阻塞）。
6. **高性能读索引（选定）**
   - `GiST(tenant_id, node_path, validity)` 实现 “Path + Time” 联合过滤；
   - `no-overlap`：`EXCLUDE USING gist (tenant_id, org_id, validity &&)`。
7. **可重放（选定）**
   - 版本表可丢弃重建：`TRUNCATE org_unit_versions` + replay `org_events`。

## 4. 数据模型与约束 (Data Model & Constraints)
> 约定：PostgreSQL 17；多租户隔离通过 `tenant_id` 强制；RLS（若启用）需 fail-closed（具体策略在实施阶段随迁移落盘）。

### 4.1 必备扩展
```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS btree_gist;
```

### 4.2 辅助函数（ltree label 编解码 + path_ids）
```sql
-- uuid -> ltree label（32 hex）
CREATE OR REPLACE FUNCTION org_ltree_label(p_id uuid)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT replace(lower(p_id::text), '-', '');
$$;

-- 32 hex -> uuid
CREATE OR REPLACE FUNCTION org_uuid_from_hex32(p_hex text)
RETURNS uuid
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT (
    substr(p_hex, 1, 8) || '-' ||
    substr(p_hex, 9, 4) || '-' ||
    substr(p_hex, 13, 4) || '-' ||
    substr(p_hex, 17, 4) || '-' ||
    substr(p_hex, 21, 12)
  )::uuid;
$$;

-- ltree path -> uuid[]（用于长名称拼接/祖先 join）
CREATE OR REPLACE FUNCTION org_path_ids(p_path ltree)
RETURNS uuid[]
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
  parts text[];
  out uuid[] := ARRAY[]::uuid[];
  part text;
BEGIN
  parts := string_to_array(p_path::text, '.');
  FOREACH part IN ARRAY parts LOOP
    out := out || org_uuid_from_hex32(part);
  END LOOP;
  RETURN out;
END;
$$;
```

### 4.3 `org_events`（Write Side / SoT）
```sql
CREATE TABLE org_events (
  id               bigserial PRIMARY KEY,
  event_id         uuid NOT NULL DEFAULT gen_random_uuid(), -- 幂等键（建议由应用传入；默认生成）
  tenant_id        uuid NOT NULL,
  hierarchy_type   text NOT NULL DEFAULT 'OrgUnit',

  org_id           uuid NOT NULL,                 -- 目标节点
  event_type       text NOT NULL,                 -- CREATE/MOVE/RENAME/DISABLE
  effective_date   date NOT NULL,                 -- 生效日（Valid Time）
  payload          jsonb NOT NULL DEFAULT '{}'::jsonb,

  request_id       text NOT NULL,
  initiator_id     uuid NOT NULL,
  transaction_time timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT org_events_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
  CONSTRAINT org_events_event_type_check CHECK (event_type IN ('CREATE','MOVE','RENAME','DISABLE'))
);

CREATE UNIQUE INDEX org_events_event_id_unique ON org_events (event_id);
CREATE INDEX org_events_tenant_org_effective_idx ON org_events (tenant_id, org_id, effective_date, id);
CREATE INDEX org_events_tenant_type_effective_idx ON org_events (tenant_id, hierarchy_type, effective_date, id);
```

> 说明：`event_id` 作为幂等键；重试时传相同 `event_id` 可确保“只写一次 + 不重复投射”。

### 4.4 `org_unit_versions`（Read Side / Projection）
```sql
CREATE TABLE org_unit_versions (
  id            bigserial PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  hierarchy_type text NOT NULL DEFAULT 'OrgUnit',

  org_id        uuid NOT NULL,
  parent_id     uuid NULL,

  node_path     ltree NOT NULL,
  validity      daterange NOT NULL,     -- [start, end) day-range
  path_ids      uuid[] GENERATED ALWAYS AS (org_path_ids(node_path)) STORED,

  name          varchar(255) NOT NULL,
  status        text NOT NULL DEFAULT 'active',  -- active/disabled
  manager_id    uuid NULL,

  last_event_id bigint NOT NULL REFERENCES org_events(id),

  CONSTRAINT org_unit_versions_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
  CONSTRAINT org_unit_versions_status_check CHECK (status IN ('active','disabled')),
  CONSTRAINT org_unit_versions_validity_check CHECK (lower(validity) < upper(validity)),

  -- 防止同一节点版本重叠
  CONSTRAINT org_unit_versions_no_overlap
    EXCLUDE USING gist (tenant_id WITH =, hierarchy_type WITH =, org_id WITH =, validity WITH &&)
);

-- 核心联合索引：Path + Time
CREATE INDEX org_unit_versions_search_gist
  ON org_unit_versions
  USING gist (tenant_id gist_uuid_ops, hierarchy_type gist_text_ops, node_path, validity);

-- 点查：某节点 as-of（配合 validity @> date）
CREATE INDEX org_unit_versions_lookup_btree
  ON org_unit_versions (tenant_id, hierarchy_type, org_id, lower(validity));

-- path_ids 加速（长名称/祖先 join）
CREATE INDEX org_unit_versions_path_ids_gin
  ON org_unit_versions
  USING gin (path_ids);
```

## 5. 核心计算逻辑（DB Engine）
> 目标：应用层只做鉴权/锁/事务边界；投射逻辑在 DB 内同步完成。

### 5.1 并发互斥（Advisory Lock）
**锁粒度（选定）**：同一 `tenant_id + hierarchy_type` 串行化写入，避免并发 Move/Correct 导致死锁与 path 漂移。

锁 key（文本）：`org:v4:<tenant_id>:<hierarchy_type>`

在事务内调用（阻塞版）：
```sql
SELECT pg_advisory_xact_lock(hashtext($1));
```
或 fail-fast：
```sql
SELECT pg_try_advisory_xact_lock(hashtext($1));
```

### 5.2 统一入口：`submit_org_event`
**职责**：插入事件（幂等）+ 调用对应 `apply_*` 投射函数；同一事务提交。

函数签名（建议）：
```sql
CREATE OR REPLACE FUNCTION submit_org_event(
  p_event_id uuid,
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_event_type text,
  p_effective_date date,
  p_payload jsonb,
  p_request_id text,
  p_initiator_id uuid
) RETURNS bigint;
```

关键步骤（伪代码）：
1) `pg_advisory_xact_lock(hashtext(lock_key))`
2) `INSERT INTO org_events ... ON CONFLICT (event_id) DO NOTHING RETURNING id INTO v_event_db_id`
   - 若 conflict：读取既有 `id` 并直接返回（确保幂等，不重复投射）
3) `CASE p_event_type`：
   - `CREATE` → `PERFORM apply_create_logic(..., v_event_db_id)`
   - `MOVE` → `PERFORM apply_move_logic(..., v_event_db_id)`
   - `RENAME` → `PERFORM apply_rename_logic(..., v_event_db_id)`
   - `DISABLE` → `PERFORM apply_disable_logic(..., v_event_db_id)`
4) 返回 `v_event_db_id`

### 5.3 `apply_create_logic`
payload（v1）：
```json
{
  "parent_id": "uuid|null",
  "name": "string",
  "manager_id": "uuid|null"
}
```

关键约束：
- `parent_id` 非空时：parent 在 `p_effective_date` 必须 active（版本存在且 status=active）。
- `org_id` 在 `p_effective_date` 不得已有版本（避免重复创建）。
- 根节点（`parent_id=null`）在一个 tenant/hierarchy 内只能存在一个（本计划不引入 root 表；根唯一性在实现阶段可通过约束或显式检查实现）。

投射策略：
- 计算 `node_path`：
  - root：`org_ltree_label(p_org_id)::ltree`
  - child：`parent_path || org_ltree_label(p_org_id)::ltree`
- 插入 `org_unit_versions(org_id, parent_id, node_path, validity=[effective_date, 'infinity'), name, status='active', manager_id, last_event_id)`

### 5.4 `apply_move_logic`（Split & Graft）
payload（v1）：
```json
{ "new_parent_id": "uuid" }
```

算法目标：
- 从 `p_effective_date` 起改变 `p_org_id` 的父链；
- 对“仍在旧子树下”的后代版本做前缀重写；
- 对跨越生效日的版本做 split（旧段截断 + 新段插入），并保持 no-overlap。

伪代码（与 v4 核心一致，略去细节）：
1) `SELECT node_path INTO v_old_path FROM org_unit_versions WHERE org_id=p_org_id AND validity @> p_effective_date FOR UPDATE`
2) `SELECT node_path INTO v_new_parent_path FROM org_unit_versions WHERE org_id=p_new_parent_id AND validity @> p_effective_date`
3) 防环：若 `v_new_parent_path <@ v_old_path` 则拒绝（new parent 在旧子树内）
4) `v_new_prefix := v_new_parent_path || org_ltree_label(p_org_id)::ltree`
5) 对子树版本做 split（覆盖 `node_path <@ v_old_path` 且 `validity` 覆盖 `p_effective_date` 的行）：
   - 截断旧段：`validity = daterange(lower(validity), p_effective_date, '[)')`
   - 插入新段：`validity = daterange(p_effective_date, upper(old_validity), '[)')`
     - `node_path = v_new_prefix || subpath(old.node_path, nlevel(v_old_path))`
     - `parent_id = p_new_parent_id`（仅当 `org_id=p_org_id`；后代保持原 parent_id）
     - `last_event_id = p_event_db_id`
6) 对 “从 p_effective_date 起开始的未来版本” 执行前缀 rewrite（同样限定 `node_path <@ v_old_path`）：
   - `node_path = v_new_prefix || subpath(node_path, nlevel(v_old_path))`
   - `parent_id = p_new_parent_id`（仅目标节点）
   - `last_event_id = p_event_db_id`

> 说明：`node_path <@ v_old_path` 使算法天然具备“不会误改已 moved-out 的后代版本”的性质。

### 5.5 `apply_rename_logic`
payload（v1）：
```json
{ "new_name": "string" }
```

关键点：rename 必须影响 **从 effective_date 起** 的所有版本（包括未来由 Move 等产生的版本），但不能覆盖未来的下一次 rename。

选定策略：
1) 先 split 目标节点在 `effective_date` 所在版本（若跨越）；
2) 计算 `stop_date`（若存在未来 rename）：`MIN(effective_date) WHERE event_type='RENAME' AND effective_date > p_effective_date`；
3) 批量更新 `org_unit_versions`：
   - `WHERE org_id=p_org_id`
   - 且 `lower(validity) >= p_effective_date`
   - 且（若 `stop_date` 非空）`lower(validity) < stop_date`
   - `SET name = new_name, last_event_id = p_event_db_id`

### 5.6 `apply_disable_logic`
payload（v1）：
```json
{ "status": "disabled" }
```
策略与 rename 类似：split + 计算 stop_date（若未来允许 enable，则以 enable 的 effective_date 作为 stop；本计划 v1 不引入 enable，默认 disable 永久）。

## 6. 读模型封装与查询
### 6.1 `get_org_snapshot`（含长名称）
函数签名（建议）：
```sql
CREATE OR REPLACE FUNCTION get_org_snapshot(p_tenant_id uuid, p_query_date date)
RETURNS TABLE (
  org_id uuid,
  name varchar,
  full_name_path text,
  depth int,
  manager_id uuid
);
```

实现要点（与 v4 一致）：
- `snapshot`：`FROM org_unit_versions WHERE tenant_id=p_tenant_id AND validity @> p_query_date AND status='active'`
- `full_name_path`：用 `path_ids` + ordinality join 到祖先版本（`validity @> p_query_date`），`string_agg(name, ' / ' order by idx)`
- `depth`：`nlevel(node_path)-1`

### 6.2 查询示例
- 查某日部门列表（带长名称前缀过滤）：
```sql
SELECT *
FROM get_org_snapshot($1::uuid, '2026-01-01'::date)
WHERE full_name_path LIKE '总公司 / 产研中心%';
```

- 查某节点在某日的子树：
```sql
WITH target AS (
  SELECT node_path
  FROM org_unit_versions
  WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND org_id=$2 AND validity @> $3::date
  LIMIT 1
)
SELECT *
FROM org_unit_versions v
WHERE v.tenant_id=$1
  AND v.hierarchy_type='OrgUnit'
  AND v.validity @> $3::date
  AND v.node_path <@ (SELECT node_path FROM target);
```

## 7. Go 应用层集成（事务 + 锁 + 调用 DB）
> 应用层只负责：鉴权 →（可选 try-lock）→ 开事务 → 调 `submit_org_event` → 提交。

建议形状（伪代码）：
```go
func (s *OrgServiceV4) MoveOrg(ctx context.Context, tenantID uuid.UUID, cmd MoveCmd) error {
  return composables.InTx(ctx, func(txCtx context.Context) error {
    tx, _ := composables.UseTx(txCtx)

    lockKey := fmt.Sprintf("org:v4:%s:%s", tenantID, "OrgUnit")
    var locked bool
    if err := tx.QueryRow(txCtx, "SELECT pg_try_advisory_xact_lock(hashtext($1))", lockKey).Scan(&locked); err != nil {
      return err
    }
    if !locked {
      return serrors.New("ORG_BUSY", "组织树正在变更中，请稍后再试")
    }

    payload := map[string]any{"new_parent_id": cmd.NewParentID}
    _, err := tx.Exec(txCtx, "SELECT submit_org_event($1,$2,$3,$4,$5,$6,$7,$8,$9)",
      cmd.EventID, tenantID, "OrgUnit", cmd.TargetOrgID, "MOVE", cmd.EffectiveDate, payload, cmd.RequestID, cmd.InitiatorID,
    )
    return err
  })
}
```

> 说明：锁既可在 Go 层 fail-fast，也可在 DB `submit_org_event` 内强制加锁（双保险）；实现阶段二选一或同时保留（以减少误用风险）。

## 8. 运维与灾备（Rebuild / Replay）
### 8.1 重建目标
当投射逻辑缺陷导致 `org_unit_versions` 错误时，可通过重放 `org_events` 重建读模型。

### 8.2 Rebuild 流程（建议）
1) 获取维护互斥锁：`pg_advisory_lock(hashtext('org:v4:rebuild:<tenant_id>:OrgUnit'))`
2) `TRUNCATE TABLE org_unit_versions;`
3) 按 `org_events.id ASC` 读取事件并逐条调用 `apply_*_logic`（或复用 `submit_org_event` 的内部 apply 分发，但需跳过重复 insert）
4) 释放锁

> 注：本计划不引入“维护模式开关”；如需在线运行 rebuild，必须另开计划定义安全窗口与影响面控制。

## 9. 测试与验收标准 (Acceptance Criteria)
- 正确性（必须）：
  - [ ] Create→Move→Rename→Disable 的组合在任意 as-of 日期下可得到唯一且无重叠的版本窗（EXCLUDE 约束验证）。
  - [ ] Move 触发子树 path 重写后，祖先/子树查询与长名称拼接一致且不缺段。
  - [ ] 并发写：同一 tenant/hierarchy 下同时发起两次写入，第二个在 fail-fast 模式下稳定返回“busy”错误码。
- 性能（建议）：
  - [ ] `get_org_snapshot` 在 1k/10k 节点规模下 query 次数为常数（1 次），并可通过索引命中保持稳定延迟。

## 10. 里程碑（实施拆分建议）
> 本计划不实施，但给出可落地的拆分顺序（进入 077A/077B 时复用）。
1) Schema：扩展 + `org_events/org_unit_versions` + 约束/索引/函数（最小可跑）。
2) Engine：`submit_org_event` + `apply_create/move/rename/disable`。
3) Read：`get_org_snapshot` + 子树/祖先查询形状。
4) Go：service/repo/错误码 + 最小 API（仅 org unit）。
5) Rebuild：replay 工具（CLI 或 SQL），并补齐验收与 Readiness 记录。

