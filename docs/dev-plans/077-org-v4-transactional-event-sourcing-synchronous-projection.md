# DEV-PLAN-077：Org v4（事务性事件溯源 + 同步投射）完整方案（Greenfield）

**状态**: 草拟中（2026-01-04 04:40 UTC）

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
- [ ] 定义 **DB 投射引擎（选定：同事务全量重放）**：单入口函数 `submit_org_event(...)` 在同一事务内完成：事件写入（幂等）+ **全量重放**（删除并重建 `org_unit_versions`）+ 不变量校验。
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
  - 多租户隔离（RLS）：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`（对齐 `docs/dev-plans/019-multi-tenant-toolchain.md` / `docs/dev-plans/019A-rls-tenant-isolation.md`）

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  API[OrgService (Go)] -->|Tx| DB[(Postgres)]
  DB --> E[(org_events\nSoT)]
  DB --> V[(org_unit_versions\nRead Model)]
  DB --> F[submit_org_event/replay_org_unit_versions\n(plpgsql)]
  API -->|read| Q[get_org_snapshot / queries]
  Q --> V
```

### 3.2 关键设计决策（ADR 摘要）
1. **SoT=事件表（选定）**
   - `org_events` 为 append-only；所有写入必须通过 `submit_org_event`。
2. **同步投射（选定：同事务全量重放）**
   - 同一事务内：插入事件（幂等）+ **删除并重建 `org_unit_versions`**；写后读取强一致。
3. **Valid Time（选定）**
   - 业务有效期使用 `date`；读模型使用 `daterange`（左闭右开 `[start,end)`），并用 EXCLUDE 约束防重叠。
4. **路径表示（选定）**
   - `node_path ltree`，label 使用 `uuid` 的 32 位 hex（`replace(lower(uuid::text), '-', '')`），避免 `-` 等非法字符（对齐仓库既有约定）。
5. **并发互斥（选定）**
   - 写入按“树维度”加 advisory lock：`pg_try_advisory_xact_lock(hashtextextended(lock_key, 0))`（fail-fast）或 `pg_advisory_xact_lock`（阻塞）。
   - 不使用 `hashtext`（32-bit）作为锁键哈希，避免高并发下的哈希碰撞导致“误互斥”。
6. **同日事件唯一性（选定）**
   - 明确规则：同一 `tenant_id + hierarchy_type + org_id + effective_date` **只能存在一条事件**（不引入 `EFFSEQ`，不支持同日多次变更）。
   - 落地方式：在 `org_events` 上施加唯一性约束（见 4.4），把“事件顺序”从隐式约定变成可验证不变量。
7. **gapless（选定，纳入合同）**
   - `org_unit_versions` 必须无间隙：相邻切片必须满足 `upper(prev.validity)=lower(next.validity)`，最后一段 `upper(validity)='infinity'`；停用用 `status='disabled'` 表达而不是制造空洞。
8. **高性能读索引（选定）**
   - `GiST(tenant_id, node_path, validity)` 实现 “Path + Time” 联合过滤；
   - `no-overlap`：`EXCLUDE USING gist (tenant_id, org_id, validity &&)`。
   - 对“按日取全量快照（validity @> day）且只取 active”的场景，可额外提供 `validity` 维度的 partial GiST（见 4.5）。
9. **可重放（选定）**
   - 版本表可丢弃重建：任何时刻可通过 **全量重放** `org_events` 重建 `org_unit_versions`（运维入口与步骤见 §8）。

### 3.3 边界与可替换性（防止实现期漂移）
> 同步投射把“正确性”集中到 DB 内核（Kernel）是一条**简单但不易**的路：一旦边界不清，实施期最常见的漂移是“在 Go 再写一套隐式投射/校验”，导致权威表达分裂。

**本计划选定的边界（必须遵守）**：
- **DB = Org Projection Kernel（权威）**：负责“写入原子性 + 集合更新 + 不变量强制 + 可重放”。
- **Go = Command Facade（编排）**：负责“鉴权/会话上下文 + 事务边界 + 调用 Kernel + 错误映射/UX”。

**One Door Policy（写入口唯一）**：
- 除了 `submit_org_event(...)` 与 rebuild/replay 的维护入口外，应用层不得：
  - 直接写 `org_events`/`org_unit_versions`；
  - 直接调用 `replay_org_unit_versions(...)`（它是 Kernel 的内部实现细节，而非公共 API；只允许通过 `submit_org_event` 驱动）。
- 实施阶段建议把该规则落为“可执行的约束”：例如 DB 权限隔离（仅授予应用角色执行 `submit_org_event`）、或将 `apply_*_logic` 置于单独 schema 并不暴露执行权限。

**职责矩阵（Must / Should / Won’t）**：

| 关注点 | DB（Kernel） | Go（Facade） |
| --- | --- | --- |
| 事件写入 + 读模更新的原子性 | Must | Won’t |
| 全量重放（删除并重建 versions） | Must | Won’t |
| 不变量强制（唯一性/不重叠/防环/根唯一） | Must | Should（仅做 UX 预校验） |
| 并发互斥（advisory lock） | Must（最终裁判） | Should（可先 try-lock 以 fail-fast） |
| 鉴权/租户上下文/操作者身份 | Won’t | Must |
| 错误映射到 `pkg/serrors` | Should（提供稳定可识别的 DB 错误） | Must |
| 读接口形状（snapshot/子树/祖先链） | Should（STABLE SQL + 索引对齐） | Should（封装调用/分页/缓存策略等） |

**不变量归属（DB is the Judge）**：
- 同日事件唯一性：`org_events_one_per_day_unique`（见 4.4）。
- 版本窗不重叠：`org_unit_versions_no_overlap`（见 4.5）。
- gapless：由重放算法生成并在事务内校验（见 §5.3）。
- root 唯一：`org_trees`（见 4.3）+ 重放期校验。
- 防环 move：重放期校验（cycle detection）。
- 多租户隔离：`tenant_id` 强制；v4 表默认启用 RLS（fail-closed；落盘口径见 `DEV-PLAN-081`），因此访问 v4 的运行态必须 `RLS_ENFORCE=enforce`（并使用非 superuser 且 `NOBYPASSRLS` 的 DB role）。

## 4. 数据模型与约束 (Data Model & Constraints)
> 约定：PostgreSQL 17；多租户隔离通过 `tenant_id` 强制；v4 表默认启用 RLS（fail-closed；落盘口径见 `DEV-PLAN-081`）。

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
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT array_agg(org_uuid_from_hex32(t.part) ORDER BY t.ord)
  FROM unnest(string_to_array(p_path::text, '.')) WITH ORDINALITY AS t(part, ord);
$$;
```

### 4.3 `org_trees`（每租户单树锚点）
> 目的：在“无迁移/无兼容”的 greenfield 里，把 **root 唯一性**从“约定”落为可验证事实源。

```sql
CREATE TABLE org_trees (
  tenant_id      uuid NOT NULL,
  hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
  root_org_id    uuid NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, hierarchy_type),
  CONSTRAINT org_trees_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit'))
);
```

### 4.4 `org_events`（Write Side / SoT）
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
  CONSTRAINT org_events_event_type_check CHECK (event_type IN ('CREATE','MOVE','RENAME','DISABLE')),

  -- 不变量：同一节点同一生效日只允许一条事件（不引入 effseq）
  CONSTRAINT org_events_one_per_day_unique UNIQUE (tenant_id, hierarchy_type, org_id, effective_date)
);

CREATE UNIQUE INDEX org_events_event_id_unique ON org_events (event_id);
CREATE INDEX org_events_tenant_org_effective_idx ON org_events (tenant_id, org_id, effective_date, id);
CREATE INDEX org_events_tenant_type_effective_idx ON org_events (tenant_id, hierarchy_type, effective_date, id);
```

> 说明：
> - `event_id` 作为幂等键；重试时传相同 `event_id` 可确保“只写一次 + 不重复投射”。
> - 若对同一 `org_id` 在同一 `effective_date` 再提交另一条事件（不同 `event_id`），将触发唯一性约束失败并被拒绝（见 7.2 错误契约）。

### 4.5 `org_unit_versions`（Read Side / Projection）
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
  CONSTRAINT org_unit_versions_validity_check CHECK (NOT isempty(validity)),
  CONSTRAINT org_unit_versions_validity_bounds_check CHECK (lower_inc(validity) AND NOT upper_inc(validity)),

  -- 防止同一节点版本重叠
  CONSTRAINT org_unit_versions_no_overlap
    EXCLUDE USING gist (
      tenant_id gist_uuid_ops WITH =,
      hierarchy_type gist_text_ops WITH =,
      org_id gist_uuid_ops WITH =,
      validity WITH &&
    )
);

-- 核心联合索引：Path + Time
CREATE INDEX org_unit_versions_search_gist
  ON org_unit_versions
  USING gist (tenant_id gist_uuid_ops, hierarchy_type gist_text_ops, node_path, validity);

-- 快照场景（无 node_path 条件）：按 day 过滤 active 行
CREATE INDEX org_unit_versions_active_day_gist
  ON org_unit_versions
  USING gist (tenant_id gist_uuid_ops, hierarchy_type gist_text_ops, validity)
  WHERE status = 'active';

-- 点查：某节点 as-of（配合 validity @> date）
CREATE INDEX org_unit_versions_lookup_btree
  ON org_unit_versions (tenant_id, hierarchy_type, org_id, lower(validity));

-- 说明：
-- - `org_unit_versions_no_overlap` 的 EXCLUDE 会生成一份 GiST 索引（tenant_id/hierarchy_type/org_id/validity），可被点查复用；
-- - `org_unit_versions_lookup_btree` 是否需要，应以实际查询与 `EXPLAIN (ANALYZE, BUFFERS)` 验证（避免重复索引与写放大）。

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
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));
```
或 fail-fast：
```sql
SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0));
```

### 5.2 统一入口：`submit_org_event`
**职责**：插入事件（幂等）+ 调用 `replay_org_unit_versions` 触发**同事务全量重放**（删除并重建 `org_unit_versions`）；同一事务提交。

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

实现（可直接执行的 plpgsql；内部调用 `replay_org_unit_versions` 全量重放）：
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
) RETURNS bigint
LANGUAGE plpgsql
AS $$
DECLARE
  v_lock_key text;
  v_event_db_id bigint;
  v_payload jsonb;
  v_existing org_events%ROWTYPE;
BEGIN
  PERFORM assert_current_tenant(p_tenant_id);

  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;

  IF p_event_type NOT IN ('CREATE','MOVE','RENAME','DISABLE') THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported event_type: %s', p_event_type);
  END IF;

  v_lock_key := format('org:v4:%s:%s', p_tenant_id, p_hierarchy_type);
  PERFORM pg_advisory_xact_lock(hashtextextended(v_lock_key, 0));

  v_payload := COALESCE(p_payload, '{}'::jsonb);

  INSERT INTO org_events (
    event_id,
    tenant_id,
    hierarchy_type,
    org_id,
    event_type,
    effective_date,
    payload,
    request_id,
    initiator_id
  ) VALUES (
    p_event_id,
    p_tenant_id,
    p_hierarchy_type,
    p_org_id,
    p_event_type,
    p_effective_date,
    v_payload,
    p_request_id,
    p_initiator_id
  )
  ON CONFLICT (event_id) DO NOTHING
  RETURNING id INTO v_event_db_id;

  -- 幂等：已存在同 event_id，则直接返回既有 event 的 DB id，且不重复投射。
  IF v_event_db_id IS NULL THEN
    SELECT * INTO v_existing
    FROM org_events
    WHERE event_id = p_event_id;

    -- 防止“同一幂等键复用但参数不同”被静默吞掉
    IF v_existing.tenant_id <> p_tenant_id
      OR v_existing.hierarchy_type <> p_hierarchy_type
      OR v_existing.org_id <> p_org_id
      OR v_existing.event_type <> p_event_type
      OR v_existing.effective_date <> p_effective_date
      OR v_existing.payload <> v_payload
      OR v_existing.request_id <> p_request_id
      OR v_existing.initiator_id <> p_initiator_id THEN
      RAISE EXCEPTION USING
        MESSAGE = 'ORG_IDEMPOTENCY_REUSED',
        DETAIL = format('idempotency key reused with different payload (event_id=%s)', p_event_id);
    END IF;

    RETURN v_existing.id;
  END IF;

  -- 同日事件唯一性：
  -- - 若同一 org_id + effective_date 已存在另一事件（不同 event_id），INSERT 将因约束失败而直接报错并回滚（无投射副作用）。

  -- 全量重放：删除并重建 versions（同一事务内完成，写后读强一致）
  PERFORM replay_org_unit_versions(p_tenant_id, p_hierarchy_type);

  RETURN v_event_db_id;
END;
$$;
```

### 5.3 重放引擎：`replay_org_unit_versions`
**职责**：在同一事务内，以 `(effective_date, id)` 顺序重放 `org_events`，从零重建：
- `org_trees`（root 锚点）
- `org_unit_versions`（versions 投射）
并在事务内校验 gapless 等不变量（纳入合同）。

> 顺序说明：由于“同日事件唯一性”，同一 `org_id` 不存在同一天多事件，因此 replay 的顺序在业务意义上由 `effective_date` 决定；`id` 仅作为全局稳定次序（便于实现与排障）。

实现（可直接执行的 plpgsql；复用 `apply_*_logic` 作为每条事件的投射步骤）：
```sql
CREATE OR REPLACE FUNCTION replay_org_unit_versions(
  p_tenant_id uuid,
  p_hierarchy_type text
) RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_lock_key text;
  v_event org_events%ROWTYPE;
  v_payload jsonb;
  v_parent_id uuid;
  v_new_parent_id uuid;
  v_name text;
  v_new_name text;
  v_manager_id uuid;
BEGIN
  PERFORM assert_current_tenant(p_tenant_id);

  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;

  v_lock_key := format('org:v4:%s:%s', p_tenant_id, p_hierarchy_type);
  PERFORM pg_advisory_xact_lock(hashtextextended(v_lock_key, 0));

  DELETE FROM org_unit_versions
  WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type;

  -- org_trees 作为 root 锚点/不变量载体：在全量重放中同样由事件重建
  DELETE FROM org_trees
  WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type;

  FOR v_event IN
    SELECT *
    FROM org_events
    WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type
    ORDER BY effective_date, id
  LOOP
    v_payload := COALESCE(v_event.payload, '{}'::jsonb);

    IF v_event.event_type = 'CREATE' THEN
      v_parent_id := NULLIF(v_payload->>'parent_id', '')::uuid;
      v_name := NULLIF(btrim(v_payload->>'name'), '');
      v_manager_id := NULLIF(v_payload->>'manager_id', '')::uuid;
      PERFORM apply_create_logic(p_tenant_id, p_hierarchy_type, v_event.org_id, v_parent_id, v_event.effective_date, v_name, v_manager_id, v_event.id);
    ELSIF v_event.event_type = 'MOVE' THEN
      v_new_parent_id := NULLIF(v_payload->>'new_parent_id', '')::uuid;
      PERFORM apply_move_logic(p_tenant_id, p_hierarchy_type, v_event.org_id, v_new_parent_id, v_event.effective_date, v_event.id);
    ELSIF v_event.event_type = 'RENAME' THEN
      v_new_name := NULLIF(btrim(v_payload->>'new_name'), '');
      PERFORM apply_rename_logic(p_tenant_id, p_hierarchy_type, v_event.org_id, v_event.effective_date, v_new_name, v_event.id);
    ELSIF v_event.event_type = 'DISABLE' THEN
      PERFORM apply_disable_logic(p_tenant_id, p_hierarchy_type, v_event.org_id, v_event.effective_date, v_event.id);
    ELSE
      RAISE EXCEPTION USING
        MESSAGE = 'ORG_INVALID_ARGUMENT',
        DETAIL = format('unsupported event_type: %s', v_event.event_type);
    END IF;
  END LOOP;

  -- gapless 校验（纳入合同）：相邻切片必须严丝合缝，且最后一段必须到 infinity
  IF EXISTS (
    WITH ordered AS (
      SELECT
        org_id,
        validity,
        lag(validity) OVER (PARTITION BY org_id ORDER BY lower(validity)) AS prev_validity
      FROM org_unit_versions
      WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type
    )
    SELECT 1
    FROM ordered
    WHERE prev_validity IS NOT NULL
      AND lower(validity) <> upper(prev_validity)
    LIMIT 1
  ) THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_VALIDITY_GAP',
      DETAIL = 'org_unit_versions must be gapless';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM (
      SELECT DISTINCT ON (org_id) org_id, validity
      FROM org_unit_versions
      WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type
      ORDER BY org_id, lower(validity) DESC
    ) last
    WHERE NOT upper_inf(last.validity)
    LIMIT 1
  ) THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_VALIDITY_NOT_INFINITE',
      DETAIL = 'last version validity must be unbounded (infinity)';
  END IF;
END;
$$;
```

### 5.4 `apply_create_logic`
payload（v1）：
```json
{
  "parent_id": "uuid|null",
  "name": "string",
  "manager_id": "uuid|null"
}
```

关键约束：
- `parent_id` 非空时：parent 在 `p_effective_date` 必须 active（版本存在且 `status='active'`）。
- 同一 `org_id` 只允许 create 一次（greenfield 简化约束）。
- 根节点（`parent_id=null`）在一个 tenant/hierarchy 内只能存在一个（通过 `org_trees` 固化）。

投射策略：
- 计算 `node_path`：
  - root：`org_ltree_label(p_org_id)::ltree`
  - child：`parent_path || org_ltree_label(p_org_id)::ltree`
- 插入 `org_unit_versions(org_id, parent_id, node_path, validity=[effective_date, 'infinity'), name, status='active', manager_id, last_event_id)`

SQL 实现（v1）：
```sql
CREATE OR REPLACE FUNCTION apply_create_logic(
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_parent_id uuid,
  p_effective_date date,
  p_name text,
  p_manager_id uuid,
  p_event_db_id bigint
) RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_parent_path ltree;
  v_node_path ltree;
  v_root_org_id uuid;
BEGIN
  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;
  IF p_name IS NULL THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = 'name is required';
  END IF;

  -- 同一 org_id 只允许 create 一次（greenfield 简化约束）
  IF EXISTS (
    SELECT 1
    FROM org_unit_versions
    WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type AND org_id = p_org_id
  ) THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_ALREADY_EXISTS',
      DETAIL = format('org already exists: %s', p_org_id);
  END IF;

  -- root 唯一性（通过 org_trees 固化）
  IF p_parent_id IS NULL THEN
    SELECT t.root_org_id INTO v_root_org_id
    FROM org_trees t
    WHERE t.tenant_id = p_tenant_id AND t.hierarchy_type = p_hierarchy_type
    FOR UPDATE;

    IF v_root_org_id IS NOT NULL THEN
      RAISE EXCEPTION USING
        MESSAGE = 'ORG_ROOT_ALREADY_EXISTS',
        DETAIL = format('root already exists: %s', v_root_org_id);
    END IF;

    INSERT INTO org_trees (tenant_id, hierarchy_type, root_org_id)
    VALUES (p_tenant_id, p_hierarchy_type, p_org_id);

    v_node_path := org_ltree_label(p_org_id)::ltree;
  ELSE
    -- 子节点要求 root 已初始化（保证树锚点存在）
    SELECT t.root_org_id INTO v_root_org_id
    FROM org_trees t
    WHERE t.tenant_id = p_tenant_id AND t.hierarchy_type = p_hierarchy_type;

    IF v_root_org_id IS NULL THEN
      RAISE EXCEPTION USING
        MESSAGE = 'ORG_TREE_NOT_INITIALIZED',
        DETAIL = format('tree root not initialized (tenant_id=%s)', p_tenant_id);
    END IF;

    SELECT v.node_path INTO v_parent_path
    FROM org_unit_versions v
    WHERE v.tenant_id = p_tenant_id
      AND v.hierarchy_type = p_hierarchy_type
      AND v.org_id = p_parent_id
      AND v.status = 'active'
      AND v.validity @> p_effective_date
    LIMIT 1;

    IF v_parent_path IS NULL THEN
      RAISE EXCEPTION USING
        MESSAGE = 'ORG_PARENT_NOT_FOUND_AS_OF',
        DETAIL = format('parent not found at date (parent_id=%s, as_of=%s)', p_parent_id, p_effective_date);
    END IF;

    v_node_path := v_parent_path || org_ltree_label(p_org_id)::ltree;
  END IF;

  INSERT INTO org_unit_versions (
    tenant_id,
    hierarchy_type,
    org_id,
    parent_id,
    node_path,
    validity,
    name,
    status,
    manager_id,
    last_event_id
  ) VALUES (
    p_tenant_id,
    p_hierarchy_type,
    p_org_id,
    p_parent_id,
    v_node_path,
    daterange(p_effective_date, 'infinity'::date, '[)'),
    p_name,
    'active',
    p_manager_id,
    p_event_db_id
  );
END;
$$;
```

### 5.5 `apply_move_logic`（Split & Graft）
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

SQL 实现（v1）：
```sql
CREATE OR REPLACE FUNCTION apply_move_logic(
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_new_parent_id uuid,
  p_effective_date date,
  p_event_db_id bigint
) RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_old_path ltree;
  v_new_parent_path ltree;
  v_new_prefix ltree;
  v_old_level int;
  v_root_org_id uuid;
BEGIN
  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;
  IF p_new_parent_id IS NULL THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = 'new_parent_id is required';
  END IF;
  IF p_new_parent_id = p_org_id THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = 'new_parent_id cannot equal org_id';
  END IF;

  -- root 不允许被移动（root 固化在 org_trees）
  SELECT t.root_org_id INTO v_root_org_id
  FROM org_trees t
  WHERE t.tenant_id = p_tenant_id AND t.hierarchy_type = p_hierarchy_type;
  IF v_root_org_id = p_org_id THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_ROOT_CANNOT_BE_MOVED',
      DETAIL = format('root cannot be moved: %s', p_org_id);
  END IF;

  -- 锁定并获取旧路径
  SELECT v.node_path INTO v_old_path
  FROM org_unit_versions v
  WHERE v.tenant_id = p_tenant_id
    AND v.hierarchy_type = p_hierarchy_type
    AND v.org_id = p_org_id
    AND v.status = 'active'
    AND v.validity @> p_effective_date
  FOR UPDATE;

  IF v_old_path IS NULL THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_NOT_FOUND_AS_OF',
      DETAIL = format('target org not found at date (org_id=%s, as_of=%s)', p_org_id, p_effective_date);
  END IF;

  -- 获取新 Parent 路径
  SELECT v.node_path INTO v_new_parent_path
  FROM org_unit_versions v
  WHERE v.tenant_id = p_tenant_id
    AND v.hierarchy_type = p_hierarchy_type
    AND v.org_id = p_new_parent_id
    AND v.status = 'active'
    AND v.validity @> p_effective_date
  LIMIT 1;

  IF v_new_parent_path IS NULL THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_PARENT_NOT_FOUND_AS_OF',
      DETAIL = format('new parent not found at date (parent_id=%s, as_of=%s)', p_new_parent_id, p_effective_date);
  END IF;

  -- 防环：新 parent 落在旧子树内
  IF v_new_parent_path <@ v_old_path THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_CYCLE_MOVE',
      DETAIL = format('cycle move is not allowed (org_id=%s -> new_parent_id=%s)', p_org_id, p_new_parent_id);
  END IF;

  v_new_prefix := v_new_parent_path || org_ltree_label(p_org_id)::ltree;
  v_old_level := nlevel(v_old_path);

  -- 1) split：覆盖生效日的版本（子树内所有节点）
  WITH split AS (
    SELECT *
    FROM org_unit_versions
    WHERE tenant_id = p_tenant_id
      AND hierarchy_type = p_hierarchy_type
      AND node_path <@ v_old_path
      AND validity @> p_effective_date
      AND lower(validity) < p_effective_date
  ),
  upd AS (
    UPDATE org_unit_versions v
    SET validity = daterange(lower(v.validity), p_effective_date, '[)')
    FROM split s
    WHERE v.id = s.id
    RETURNING s.*
  )
  INSERT INTO org_unit_versions (
    tenant_id,
    hierarchy_type,
    org_id,
    parent_id,
    node_path,
    validity,
    name,
    status,
    manager_id,
    last_event_id
  )
  SELECT
    u.tenant_id,
    u.hierarchy_type,
    u.org_id,
    CASE WHEN u.org_id = p_org_id THEN p_new_parent_id ELSE u.parent_id END,
    CASE
      WHEN u.org_id = p_org_id THEN v_new_prefix
      ELSE v_new_prefix || subpath(u.node_path, v_old_level)
    END,
    daterange(p_effective_date, upper(u.validity), '[)'),
    u.name,
    u.status,
    u.manager_id,
    p_event_db_id
  FROM upd u;

  -- 2) rewrite：未来版本（从 effective_date 起开始的段）
  UPDATE org_unit_versions v
  SET node_path = CASE
        WHEN v.org_id = p_org_id THEN v_new_prefix
        ELSE v_new_prefix || subpath(v.node_path, v_old_level)
      END,
      parent_id = CASE WHEN v.org_id = p_org_id THEN p_new_parent_id ELSE v.parent_id END,
      last_event_id = p_event_db_id
  WHERE v.tenant_id = p_tenant_id
    AND v.hierarchy_type = p_hierarchy_type
    AND v.node_path <@ v_old_path
    AND lower(v.validity) >= p_effective_date;
END;
$$;
```

### 5.6 `apply_rename_logic`
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

> 约束说明：由于 3.2 已选定“同日事件唯一性”，同一 `org_id` 不会出现同一天多条事件，因此本策略不需要引入 `EFFSEQ` 或“同日排序”规则。

SQL 实现（v1）：
```sql
-- 单节点 split helper：把覆盖 p_effective_date 的版本切成 [start, effective) 与 [effective, end)
CREATE OR REPLACE FUNCTION split_org_unit_version_at(
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_effective_date date,
  p_event_db_id bigint
) RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_row org_unit_versions%ROWTYPE;
BEGIN
  SELECT * INTO v_row
  FROM org_unit_versions
  WHERE tenant_id = p_tenant_id
    AND hierarchy_type = p_hierarchy_type
    AND org_id = p_org_id
    AND validity @> p_effective_date
    AND lower(validity) < p_effective_date
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN;
  END IF;

  UPDATE org_unit_versions
  SET validity = daterange(lower(validity), p_effective_date, '[)')
  WHERE id = v_row.id;

  INSERT INTO org_unit_versions (
    tenant_id,
    hierarchy_type,
    org_id,
    parent_id,
    node_path,
    validity,
    name,
    status,
    manager_id,
    last_event_id
  ) VALUES (
    v_row.tenant_id,
    v_row.hierarchy_type,
    v_row.org_id,
    v_row.parent_id,
    v_row.node_path,
    daterange(p_effective_date, upper(v_row.validity), '[)'),
    v_row.name,
    v_row.status,
    v_row.manager_id,
    p_event_db_id
  );
END;
$$;

CREATE OR REPLACE FUNCTION apply_rename_logic(
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_effective_date date,
  p_new_name text,
  p_event_db_id bigint
) RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_stop_date date;
BEGIN
  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;
  IF p_new_name IS NULL THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = 'new_name is required';
  END IF;

  -- 必须存在任一版本覆盖 effective_date
  IF NOT EXISTS (
    SELECT 1
    FROM org_unit_versions
    WHERE tenant_id = p_tenant_id
      AND hierarchy_type = p_hierarchy_type
      AND org_id = p_org_id
      AND status = 'active'
      AND validity @> p_effective_date
  ) THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_NOT_FOUND_AS_OF',
      DETAIL = format('org not found at date (org_id=%s, as_of=%s)', p_org_id, p_effective_date);
  END IF;

  PERFORM split_org_unit_version_at(p_tenant_id, p_hierarchy_type, p_org_id, p_effective_date, p_event_db_id);

  SELECT MIN(e.effective_date) INTO v_stop_date
  FROM org_events e
  WHERE e.tenant_id = p_tenant_id
    AND e.hierarchy_type = p_hierarchy_type
    AND e.org_id = p_org_id
    AND e.event_type = 'RENAME'
    AND e.effective_date > p_effective_date;

  UPDATE org_unit_versions v
  SET name = p_new_name,
      last_event_id = p_event_db_id
  WHERE v.tenant_id = p_tenant_id
    AND v.hierarchy_type = p_hierarchy_type
    AND v.org_id = p_org_id
    AND lower(v.validity) >= p_effective_date
    AND (v_stop_date IS NULL OR lower(v.validity) < v_stop_date);
END;
$$;
```

### 5.7 `apply_disable_logic`
payload（v1）：
```json
{ "status": "disabled" }
```
策略与 rename 类似：split + 计算 stop_date（若未来允许 enable，则以 enable 的 effective_date 作为 stop；本计划 v1 不引入 enable，默认 disable 永久）。

SQL 实现（v1）：
```sql
CREATE OR REPLACE FUNCTION apply_disable_logic(
  p_tenant_id uuid,
  p_hierarchy_type text,
  p_org_id uuid,
  p_effective_date date,
  p_event_db_id bigint
) RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
  IF p_hierarchy_type <> 'OrgUnit' THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_INVALID_ARGUMENT',
      DETAIL = format('unsupported hierarchy_type: %s', p_hierarchy_type);
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM org_unit_versions
    WHERE tenant_id = p_tenant_id
      AND hierarchy_type = p_hierarchy_type
      AND org_id = p_org_id
      AND status = 'active'
      AND validity @> p_effective_date
  ) THEN
    RAISE EXCEPTION USING
      MESSAGE = 'ORG_NOT_FOUND_AS_OF',
      DETAIL = format('org not found at date (org_id=%s, as_of=%s)', p_org_id, p_effective_date);
  END IF;

  PERFORM split_org_unit_version_at(p_tenant_id, p_hierarchy_type, p_org_id, p_effective_date, p_event_db_id);

  UPDATE org_unit_versions v
  SET status = 'disabled',
      last_event_id = p_event_db_id
  WHERE v.tenant_id = p_tenant_id
    AND v.hierarchy_type = p_hierarchy_type
    AND v.org_id = p_org_id
    AND lower(v.validity) >= p_effective_date;
END;
$$;
```

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

实现（SQL，STABLE；与索引/查询形状对齐：`validity @> date` + `path_ids`）：
```sql
CREATE OR REPLACE FUNCTION get_org_snapshot(p_tenant_id uuid, p_query_date date)
RETURNS TABLE (
  org_id uuid,
  name varchar,
  full_name_path text,
  depth int,
  manager_id uuid
)
LANGUAGE sql
STABLE
AS $$
  WITH snapshot AS (
    SELECT v.*
    FROM org_unit_versions v
    WHERE v.tenant_id = p_tenant_id
      AND v.hierarchy_type = 'OrgUnit'
      AND v.status = 'active'
      AND v.validity @> p_query_date
  )
  SELECT
    s.org_id,
    s.name,
    (
      SELECT string_agg(a.name, ' / ' ORDER BY t.idx)
      FROM unnest(s.path_ids) WITH ORDINALITY AS t(uid, idx)
      JOIN org_unit_versions a
        ON a.tenant_id = p_tenant_id
       AND a.hierarchy_type = 'OrgUnit'
       AND a.org_id = t.uid
       AND a.validity @> p_query_date
    ) AS full_name_path,
    nlevel(s.node_path) - 1 AS depth,
    s.manager_id
  FROM snapshot s;
$$;
```

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

> 多租户隔离（RLS，见 `DEV-PLAN-081`）：v4 表默认启用 RLS，事务必须在第一条 SQL 前注入 `app.current_tenant`（复用 `composables.ApplyTenantRLS`）；`RLS_ENFORCE` 为 `disabled` 将导致 fail-closed（属于配置错误，而非“降级模式”）。

建议形状（伪代码）：
```go
func (s *OrgServiceV4) MoveOrg(ctx context.Context, tenantID uuid.UUID, cmd MoveCmd) error {
  return composables.InTx(ctx, func(txCtx context.Context) error {
    tx, _ := composables.UseTx(txCtx)

    lockKey := fmt.Sprintf("org:v4:%s:%s", tenantID, "OrgUnit")
    var locked bool
    if err := tx.QueryRow(txCtx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))", lockKey).Scan(&locked); err != nil {
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

> 说明（与 3.3 边界一致）：DB 必须在 `submit_org_event` 内执行互斥锁（作为一致性策略的最终裁判），Go 可选先 `pg_try_advisory_xact_lock(...)` 做 fail-fast（拿到锁后再调用 `submit_org_event` 不会产生额外等待）。

### 7.2 错误契约（DB → Go → serrors）
> 目标：让失败路径“可解释且可映射”，避免实现期靠字符串匹配与试错收敛。

约定（实现阶段建议遵守）：
- Go 侧对 Postgres 错误优先用 `SQLSTATE`（例如 `23505`、`23P01`）+ `ConstraintName` 做稳定映射；
- 对于业务级拒绝（cycle/not-found/idempotency-reused/invalid-argument 等），DB 必须使用“机器可识别”的异常：
  - `RAISE EXCEPTION USING MESSAGE = '<STABLE_CODE>', DETAIL = '<dynamic details>'`；
  - `MESSAGE` 必须是稳定 code（不拼接动态内容），动态信息放在 `DETAIL`；
  - Go 侧只解析 `MESSAGE` 做映射，不依赖自然语言与字符串包含关系；
- 对“同日事件冲突”这类不变量，允许直接依赖唯一性约束报错（`23505`）做映射。
- 多租户隔离（RLS）相关失败路径与稳定映射对齐 `DEV-PLAN-081`（fail-closed 缺 tenant 上下文 / tenant mismatch / policy 拒绝）。

最小映射表（v1）：

| 场景 | DB 侧来源 | 识别方式（建议） | Go `serrors` code |
| --- | --- | --- | --- |
| 组织树被占用（fail-fast lock） | `pg_try_advisory_xact_lock` 返回 false | 应用层布尔结果 | `ORG_BUSY` |
| 幂等键复用但参数不同 | `submit_org_event` 明确拒绝 | DB exception `MESSAGE` | `ORG_IDEMPOTENCY_REUSED` |
| 同一节点同日重复事件 | `org_events_one_per_day_unique` | `23505` + constraint name | `ORG_EVENT_CONFLICT_SAME_DAY` |
| 参数非法 / 不支持的类型/事件 | `submit_org_event/replay_org_unit_versions/apply_*_logic` 参数校验 | DB exception `MESSAGE` | `ORG_INVALID_ARGUMENT` |
| 重复创建同一节点 | `apply_create_logic` | DB exception `MESSAGE` | `ORG_ALREADY_EXISTS` |
| root 已存在 / root 未初始化 | `apply_create_logic` | DB exception `MESSAGE` | `ORG_ROOT_ALREADY_EXISTS` / `ORG_TREE_NOT_INITIALIZED` |
| as-of 找不到目标 | `apply_*_logic` | DB exception `MESSAGE` | `ORG_NOT_FOUND_AS_OF` |
| as-of 找不到父节点 | `apply_*_logic` | DB exception `MESSAGE` | `ORG_PARENT_NOT_FOUND_AS_OF` |
| move 形成环 | `apply_move_logic` | DB exception `MESSAGE` | `ORG_CYCLE_MOVE` |
| root 不允许 move | `apply_move_logic` | DB exception `MESSAGE` | `ORG_ROOT_CANNOT_BE_MOVED` |
| 有效期重叠（破坏 no-overlap） | `org_unit_versions_no_overlap` | `23P01` + constraint name | `ORG_VALIDITY_OVERLAP` |
| gapless 被破坏（出现间隙/末段非 infinity） | `replay_org_unit_versions` 校验失败 | DB exception `MESSAGE` | `ORG_VALIDITY_GAP` / `ORG_VALIDITY_NOT_INFINITE` |

> 注：上表的 code 命名仅为示例，进入 077A/077B 时应以模块现有 `pkg/serrors` 规范与错误码表为准统一收敛。

## 8. 运维与灾备（Rebuild / Replay）
### 8.1 重建目标
当投射逻辑缺陷导致 `org_unit_versions` 错误时，可通过重放 `org_events` 重建读模型。

### 8.2 Rebuild 流程（建议）
> 多租户隔离（RLS，见 `DEV-PLAN-081`）：rebuild/replay 必须在**显式事务**内先注入 `app.current_tenant`，否则会 fail-closed。

1) `BEGIN;`
2) 注入租户上下文：`SELECT set_config('app.current_tenant', '<tenant_id>', true);`
3) 获取维护互斥锁（复用写锁 key）：`SELECT pg_advisory_xact_lock(hashtextextended('org:v4:<tenant_id>:OrgUnit', 0));`
4) 执行全量重放：`SELECT replay_org_unit_versions('<tenant_id>'::uuid, 'OrgUnit');`
5) `COMMIT;`

> 注：本计划不引入“维护模式开关”；如需在线运行 rebuild，必须另开计划定义安全窗口与影响面控制。

> 顺序说明：由于“同日事件唯一性”，同一 `org_id` 不存在同一天多事件，因此 replay 的顺序在业务意义上由 `effective_date` 唯一决定；`id` 仅作为全局稳定次序（便于实现与排障）。

## 9. 测试与验收标准 (Acceptance Criteria)
- 正确性（必须）：
  - [ ] Create→Move→Rename→Disable 的组合在任意 as-of 日期下可得到唯一且无重叠的版本窗（EXCLUDE 约束验证）。
  - [ ] Move 触发子树 path 重写后，祖先/子树查询与长名称拼接一致且不缺段。
  - [ ] 并发写：同一 tenant/hierarchy 下同时发起两次写入，第二个在 fail-fast 模式下稳定返回“busy”错误码。
  - [ ] 同一 `org_id` 在同一 `effective_date` 第二次提交（不同 `event_id`）稳定失败，并映射为 `ORG_EVENT_CONFLICT_SAME_DAY`。
  - [ ] RLS（对齐 `DEV-PLAN-081`）：缺失 `app.current_tenant` 时对 v4 表的读写必须 fail-closed（不得以“空结果”掩盖注入遗漏）。
  - [ ] RLS（对齐 `DEV-PLAN-081`）：`app.current_tenant` 与 `p_tenant_id` 不一致时，`submit_org_event/replay_org_unit_versions` 必须稳定失败（tenant mismatch）。
- 性能（建议）：
  - [ ] `get_org_snapshot` 在 1k/10k 节点规模下 query 次数为常数（1 次），并可通过索引命中保持稳定延迟。
  - [ ] `EXPLAIN (ANALYZE, BUFFERS)` 显示 `get_org_snapshot` 的 snapshot 过滤命中 `org_unit_versions_active_day_gist`（或等价的 GiST/EXCLUDE 索引），避免全表 Seq Scan。

## 10. 里程碑（实施拆分建议）
> 本计划不实施，但给出可落地的拆分顺序（进入 077A/077B 时复用）。
1) Schema：扩展 + `org_events/org_unit_versions` + 约束/索引/函数（最小可跑）。
2) Engine：`submit_org_event` + `replay_org_unit_versions`（全量重放；内部复用 `apply_*_logic`）。
3) Read：`get_org_snapshot` + 子树/祖先查询形状。
4) Go：service/repo/错误码 + 最小 API（仅 org unit）。
5) Rebuild：replay 工具（CLI 或 SQL），并补齐验收与 Readiness 记录。
