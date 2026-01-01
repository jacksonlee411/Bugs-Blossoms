# DEV-PLAN-075：职位分类（Job Catalog）主数据属性 Effective Dating：切片化 + 同步展示 + 复用抽象

**状态**: 规划中（2025-12-31 23:44 UTC）

## 1. 背景与上下文 (Context)
- **现状**：Job Catalog / Job Profile 主数据（职类/职种/职级/职位模板）当前为 SCD1（直接 `UPDATE` 覆盖），缺少 Valid Time 维度；Org UI 已预留 `effective_date` 参数，但目前不参与读写与校验。
- **问题**：
  - **历史口径不可复现**：名称/启停/归属关系变更会“回写历史”，导致 Position/Assignment 时间线展示漂移。
  - **引用展示不一致**：引用方无法按 `as_of_date` 解析被引用属性，无法满足“在生效日期上同步更新展示”。
  - **扩展风险**：随着职位分类属性增加（未来可能引入更多维度），若每个属性都各写一套切片写入与 as-of join，维护复杂度会快速膨胀。
- **依赖与 SSOT**：
  - Valid Time 语义（day）：`docs/dev-plans/064-effective-date-day-granularity.md`
  - Job Architecture（明确 Phase 2 需要主数据 effective-dated）：`docs/dev-plans/072-job-architecture-workday-alignment.md`
  - “无 gap”切片心智模型参考（删除场景的 auto-stitch 方向）：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md`

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] Job Catalog 各“属性对象”切片化（Valid Time=date）：`Job Family Group / Job Family / Job Level / Job Profile` 均支持 `effective_date/end_date`，并对齐 Org/Position 的切片机制（自然拼接 + no-overlap）。
- [ ] **无空档（gap-free）**：同一对象的时间线一旦存在切片，必须从首片 `effective_date` 连续覆盖到 `end_of_time`（默认 `9999-12-31`），相邻切片满足 `prev.end_date + 1 = next.effective_date`。
- [ ] **同步展示**：任一主数据属性在日期 D 发生变化时，所有引用该属性的读路径（职位页、任职时间线、options 下拉、校验）在 `as_of_date=D` 及之后展示/校验自动按切片解析，**无需回写引用方数据**。
- [ ] **复用抽象**：提供可复用的“Effective-Dated Master Data”读/写模板，使新增一个新的职位分类维度时，只需补齐 schema + adapter，不需要复制粘贴整套算法与 SQL 片段。

### 2.2 非目标
- 不引入 PeopleSoft `EFFSEQ`（同一自然日多次生效）的表达能力；仍遵循 `DEV-PLAN-064` 的 date-only 前提。
- 不在本计划内新增新的 Job Catalog 维度/表（例如新增 Job Function/Job Code）。本计划只切片化既有维度，并交付可复用框架供后续计划复用。
- 不在本计划内做跨模块同步/外部系统集成变更（仅影响 `modules/org` 内主数据、校验与展示）。

### 2.3 工具链与门禁（SSOT 引用）
> 本节只声明“本计划命中哪些触发器”；命令细节以 SSOT 为准，避免 drift（`AGENTS.md`/`Makefile`/CI）。

- **触发器清单（本计划命中项）**：
  - [ ] Go 代码（触发器矩阵：`AGENTS.md`）
  - [ ] `.templ` / Tailwind（触发器矩阵：`AGENTS.md`）
  - [ ] 多语言 JSON（触发器矩阵：`AGENTS.md`）
  - [ ] Org DB 迁移 / Schema（Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [ ] 文档（`make check doc`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)

### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  Browser[Browser] -->|HTMX| UI[Org UI Controller]
  APIClient[API Client] --> API[Org API Controller]

  UI --> S[OrgService (Job Catalog)]
  API --> S

  S --> Engine[Slice Engine (Service)]
  Engine --> Adapter[Slice Adapter (per entity)]
  Adapter --> Repo[OrgRepository]
  S --> Repo

  Repo --> DB[(PostgreSQL)]
  S --> Audit[Audit Log]
  S --> Outbox[Outbox (optional)]
  S --> Cache[Invalidate Tenant Cache]
```

### 3.2 关键设计决策（ADR 摘要）
- **决策 1：Identity + Slices（选定）**
  - 选项 A：继续 SCD1（直接 `UPDATE` 覆盖）。缺点：历史不可复现、引用展示漂移。
  - 选项 B：把 `effective_date/end_date` 塞进现有 identity 表做“原地 SCD2”。缺点：主键/唯一性/外键语义会被重写，迁移与回滚风险高。
  - 选项 C（选定）：Identity 承载稳定引用，Slices 承载可变属性与 Valid Time（对齐 Org Node/Position 的成熟模式）。
- **决策 2：Valid Time 使用 `date`（选定）**
  - 选项 A：`timestamptz`（秒/微秒）。缺点：与 HR 口径（day）不一致，边界与展示更复杂。
  - 选项 B（选定）：`date`（day 闭区间），并用 `daterange(effective_date, end_date + 1, '[)')` 表达 no-overlap（SSOT：`DEV-PLAN-064`）。
- **决策 3：无空档（gap-free）采用“写入算法 + DB 断言”双保险（选定）**
  - 选项 A：仅靠写入算法保持 gap-free。缺点：一旦实现 bug 或手工数据修复引入空档，读口径会出现“找不到 slice/展示漂移”。
  - 选项 B：仅靠 DB 断言（DEFERRABLE CONSTRAINT TRIGGER）。缺点：仍然需要写入算法去产生正确的切片边界；否则错误会在提交时集中爆炸，定位更难。
  - 选项 C（选定）：写路径统一使用 slice engine 的“截断 + carry-forward 插入”，并新增与 `org_node_slices_gap_free` 同风格的 commit-time gap-free gate（SSOT：`DEV-PLAN-066` 与 `modules/org/infrastructure/persistence/schema/org-schema.sql`）；DB 同时强制 no-overlap（EXCLUDE）。
- **决策 4：同步展示通过 `as_of_date` 解析（选定）**
  - 选项 A：把 label/名称冗余回写到 Position/Assignment（或投影表）。缺点：回写链路长、容易 drift、历史修复困难。
  - 选项 B（选定）：引用方只保存稳定 id/code；展示与校验统一用同一个 `as_of_date` join Job Catalog slices（无需回写引用方数据）。
- **决策 5：读侧避免 N+1（选定）**
  - 选项 A：controller/templ 内 for-loop 逐行 `Get*AsOf`。缺点：天然 N+1，数据规模上来后不可控。
  - 选项 B（选定）：repo helper + 批量 resolver（mixed-as-of `unnest + join lateral`）或单条联表 SQL（见 8.2/11.1）。
- **决策 6：抽象克制（选定）**
  - 选项 A：做“巨型通用引擎”吞掉 freeze/authz/audit/outbox。缺点：边界膨胀、难替换、难解释（违背 `DEV-PLAN-045`）。
  - 选项 B（选定）：engine 只做边界运算/锁序/截断插入；业务校验与副作用留在具体 service（见 8.1/8.4）。
- **决策 7：更新契约使用 `write_mode`（选定）**
  - 选项 A：仿 Node/Position，新增 `:correct` API/路由。优点：与既有风格一致；缺点：需要新增多组路由与 UI wiring。
  - 选项 B（选定）：保留现有 `PATCH` 路由形态，通过 `write_mode=correct|update_from_date` 明确写意图；仍保持 `ORG_USE_CORRECT` 失败路径可解释（见 6.2）。
- **决策 8：鉴权对象“单一来源”（选定）**
  - 选项 A：UI 用 `org.job_catalog`，API 用 `org.job_profiles`（现状）。缺点：语义分裂；权限矩阵一旦分叉会出现同一能力在 UI 与 API 表现不一致的外部行为漂移。
  - 选项 B：双校验（同时要求 `org.job_catalog` 与 `org.job_profiles`）。缺点：隐式收紧权限、兼容面不透明；需要额外 ADR/迁移说明。
  - 选项 C（选定）：按资源维度统一：Job Profile 的读/写一律使用 `org.job_profiles:read|admin`；其余 Job Catalog 维度使用 `org.job_catalog:read|admin`。UI 聚合页继续存在，但每个 tab 的写入口使用对应 object；若发现现有/自定义 policy 依赖旧行为，必须通过明确的 policy 迁移与门禁（`make authz-test && make authz-lint`）来保证不回归（见 6.5）。

### 3.3 关键术语与不变量 (Invariants & Definitions)
- **Valid Time**：`effective_date/end_date`（`date`，day 闭区间）。读口径：`effective_date <= as_of_date AND end_date >= as_of_date`；在 **no-overlap** 前提下可实现为 `effective_date <= as_of_date ORDER BY effective_date DESC LIMIT 1`（必要时再断言 `end_date >= as_of_date`）。DB 防重叠：`daterange(effective_date, end_date + 1, '[)')`（SSOT：`DEV-PLAN-064`）。
- **自然拼接（Natural Stitch）**：`prev.end_date + 1 day == next.effective_date`。
- **end_of_time**：`9999-12-31`。
- **时间线 key**：`(tenant_id, <entity_id>)`（Job Level 另见 5.3）。
- **无空档（gap-free）**：时间线非空时，从首片 `effective_date` 起连续覆盖到 `end_of_time`，且无 overlap。

## 4. 方案概览 (Proposal)
本计划采用 Org/Position 已验证的模式：**稳定身份表（Identity） + 属性切片表（Slices）**，并让所有读路径通过 `as_of_date` join 到 slices：
- Identity 表承载稳定引用（UUID id、code、不可变关系）。
- Slices 表承载可随时间变化的属性（name/is_active/…）与有效期边界。
- 多值关系（Job Profile ↔ Job Families）采用“切片拥有者 + slice-items”的模式：`job_profile_slices` + `job_profile_slice_job_families`，复用 Position 的 `position_slices` + `position_slice_job_families` 既有模式。

## 5. 数据模型与约束 (Data Model & Constraints)
> 本节为目标状态规格；实现以 Org Goose/Atlas 工具链落地（SSOT：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）。
>
> **注意**：本计划会新增多张表；按仓库规则，实施前需获得用户手工确认（见 `AGENTS.md` 3.2）。

### 5.1 Job Family Group（职类）
- Identity（现有）：`org_job_family_groups`
  - Schema（现状，SSOT）：`modules/org/infrastructure/persistence/schema/org-schema.sql`
  - 约束：`unique (tenant_id, code)`；`code` 不可变（见 5.5）
  - 备注：`name/is_active` 在 Phase B 后视为 legacy（SSOT= slices；见 5.6/10.2）
- Slices（新增）：`org_job_family_group_slices`
  - 目的：承载 `name/is_active` 的 Valid Time（day）切片
  - DDL（草案，字段/约束精确到 Postgres）：
    ```sql
    CREATE TABLE org_job_family_group_slices (
        tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
        id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
        job_family_group_id uuid NOT NULL,
        name text NOT NULL,
        is_active boolean NOT NULL DEFAULT TRUE,
        effective_date date NOT NULL,
        end_date date NOT NULL DEFAULT DATE '9999-12-31',
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        CONSTRAINT org_job_family_group_slices_tenant_id_id_key UNIQUE (tenant_id, id),
        CONSTRAINT org_job_family_group_slices_effective_check CHECK (effective_date <= end_date),
        CONSTRAINT org_job_family_group_slices_group_fk FOREIGN KEY (tenant_id, job_family_group_id)
            REFERENCES org_job_family_groups (tenant_id, id) ON DELETE RESTRICT
    );
    
    ALTER TABLE org_job_family_group_slices
        ADD CONSTRAINT org_job_family_group_slices_no_overlap
        EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, job_family_group_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);
    
    CREATE INDEX org_job_family_group_slices_tenant_group_effective_idx ON org_job_family_group_slices (tenant_id, job_family_group_id, effective_date);
    ```

### 5.2 Job Family（职种）
- Identity（现有）：`org_job_families`
  - Schema（现状，SSOT）：`modules/org/infrastructure/persistence/schema/org-schema.sql`
  - 约束：`(tenant_id, job_family_group_id) -> org_job_family_groups` 外键；`unique (tenant_id, job_family_group_id, code)`；`code` 不可变（见 5.5）
  - v1 冻结：`job_family_group_id` 视为不可变，避免引入“移动职种到另一个职类”的额外复杂度；如确需支持，另起 dev-plan（见 5.5）
  - 备注：`name/is_active` 在 Phase B 后视为 legacy（SSOT= slices；见 5.6/10.2）
- Slices（新增）：`org_job_family_slices`
  - 目的：承载 `name/is_active` 的 Valid Time（day）切片
  - DDL（草案）：
    ```sql
    CREATE TABLE org_job_family_slices (
        tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
        id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
        job_family_id uuid NOT NULL,
        name text NOT NULL,
        is_active boolean NOT NULL DEFAULT TRUE,
        effective_date date NOT NULL,
        end_date date NOT NULL DEFAULT DATE '9999-12-31',
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        CONSTRAINT org_job_family_slices_tenant_id_id_key UNIQUE (tenant_id, id),
        CONSTRAINT org_job_family_slices_effective_check CHECK (effective_date <= end_date),
        CONSTRAINT org_job_family_slices_family_fk FOREIGN KEY (tenant_id, job_family_id)
            REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT
    );
    
    ALTER TABLE org_job_family_slices
        ADD CONSTRAINT org_job_family_slices_no_overlap
        EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, job_family_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);
    
    CREATE INDEX org_job_family_slices_tenant_family_effective_idx ON org_job_family_slices (tenant_id, job_family_id, effective_date);
    ```

### 5.3 Job Level（职级）
- Identity（现有）：`org_job_levels`
  - Schema（现状，SSOT）：`modules/org/infrastructure/persistence/schema/org-schema.sql`
  - 约束：`unique (tenant_id, code)`；`display_order >= 0`；`code` 不可变（见 5.5）
  - 备注：`name/display_order/is_active` 在 Phase B 后视为 legacy（SSOT= slices；见 5.6/10.2）
- Slices（新增）：`org_job_level_slices`
  - 目的：承载 `name/display_order/is_active` 的 Valid Time（day）切片
  - DDL（草案）：
    ```sql
    CREATE TABLE org_job_level_slices (
        tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
        id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
        job_level_id uuid NOT NULL,
        name text NOT NULL,
        display_order int NOT NULL DEFAULT 0,
        is_active boolean NOT NULL DEFAULT TRUE,
        effective_date date NOT NULL,
        end_date date NOT NULL DEFAULT DATE '9999-12-31',
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        CONSTRAINT org_job_level_slices_tenant_id_id_key UNIQUE (tenant_id, id),
        CONSTRAINT org_job_level_slices_effective_check CHECK (effective_date <= end_date),
        CONSTRAINT org_job_level_slices_display_order_check CHECK (display_order >= 0),
        CONSTRAINT org_job_level_slices_level_fk FOREIGN KEY (tenant_id, job_level_id)
            REFERENCES org_job_levels (tenant_id, id) ON DELETE RESTRICT
    );
    
    ALTER TABLE org_job_level_slices
        ADD CONSTRAINT org_job_level_slices_no_overlap
        EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, job_level_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);
    
    CREATE INDEX org_job_level_slices_tenant_level_effective_idx ON org_job_level_slices (tenant_id, job_level_id, effective_date);
    ```
- 读路径：Position/Assignment 仍以 `job_level_code` 为入口，因此需要 `job_level_code -> job_level_id -> slices(as_of)` 两段解析（建议通过 repo 层批量 resolver 统一解析，见 8.2）。

### 5.4 Job Profile（职位模板）
- Identity（现有）：`org_job_profiles`
  - Schema（现状，SSOT）：`modules/org/infrastructure/persistence/schema/org-schema.sql`
  - 约束：`unique (tenant_id, code)`；`external_refs` 必须为 object（现状已有 CHECK）；`code` 不可变（见 5.5）
  - 备注：`name/description/is_active/external_refs` 在 Phase B 后视为 legacy（SSOT= slices；见 5.6/10.2）
- Slices（新增）：`org_job_profile_slices`
  - 目的：承载 `name/description/is_active/external_refs` 的 Valid Time（day）切片
  - DDL（草案）：
    ```sql
    CREATE TABLE org_job_profile_slices (
        tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
        id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
        job_profile_id uuid NOT NULL,
        name text NOT NULL,
        description text NULL,
        is_active boolean NOT NULL DEFAULT TRUE,
        external_refs jsonb NOT NULL DEFAULT '{}' ::jsonb,
        effective_date date NOT NULL,
        end_date date NOT NULL DEFAULT DATE '9999-12-31',
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        CONSTRAINT org_job_profile_slices_tenant_id_id_key UNIQUE (tenant_id, id),
        CONSTRAINT org_job_profile_slices_effective_check CHECK (effective_date <= end_date),
        CONSTRAINT org_job_profile_slices_external_refs_is_object_check CHECK (jsonb_typeof(external_refs) = 'object'),
        CONSTRAINT org_job_profile_slices_profile_fk FOREIGN KEY (tenant_id, job_profile_id)
            REFERENCES org_job_profiles (tenant_id, id) ON DELETE RESTRICT
    );
    
    ALTER TABLE org_job_profile_slices
        ADD CONSTRAINT org_job_profile_slices_no_overlap
        EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, job_profile_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);
    
    CREATE INDEX org_job_profile_slices_tenant_profile_effective_idx ON org_job_profile_slices (tenant_id, job_profile_id, effective_date);
    ```
- 多值关系（Profile ↔ Families，切片化）（新增）：`org_job_profile_slice_job_families`
  - 目的：让 Profile 在不同有效期可以对应不同 families 集合与 primary
  - DDL（草案，复用现有 `org_job_profile_job_families` 的约束风格）：
    ```sql
    CREATE TABLE org_job_profile_slice_job_families (
        tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
        job_profile_slice_id uuid NOT NULL,
        job_family_id uuid NOT NULL,
        is_primary boolean NOT NULL DEFAULT FALSE,
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        CONSTRAINT org_job_profile_slice_job_families_pkey PRIMARY KEY (tenant_id, job_profile_slice_id, job_family_id),
        CONSTRAINT org_job_profile_slice_job_families_slice_fk FOREIGN KEY (tenant_id, job_profile_slice_id)
            REFERENCES org_job_profile_slices (tenant_id, id) ON DELETE CASCADE,
        CONSTRAINT org_job_profile_slice_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id)
            REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT
    );
    
    CREATE UNIQUE INDEX org_job_profile_slice_job_families_primary_unique ON org_job_profile_slice_job_families (tenant_id, job_profile_slice_id)
    WHERE
        is_primary = TRUE;
    
    CREATE INDEX org_job_profile_slice_job_families_tenant_family_slice_idx ON org_job_profile_slice_job_families (tenant_id, job_family_id, job_profile_slice_id);
    
    CREATE OR REPLACE FUNCTION org_job_profile_slice_job_families_validate ()
        RETURNS TRIGGER
        AS $$
    DECLARE
        t_id uuid;
        s_id uuid;
        primary_count int;
        parent_exists boolean;
    BEGIN
        t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
        s_id := COALESCE(NEW.job_profile_slice_id, OLD.job_profile_slice_id);
        SELECT
            EXISTS (
                SELECT
                    1
                FROM
                    org_job_profile_slices s
                WHERE
                    s.tenant_id = t_id
                    AND s.id = s_id) INTO parent_exists;
        IF NOT parent_exists THEN
            RETURN NULL;
        END IF;
        SELECT
            COALESCE(SUM(
                    CASE WHEN is_primary THEN
                        1
                    ELSE
                        0
                    END), 0) INTO primary_count
        FROM
            org_job_profile_slice_job_families
        WHERE
            tenant_id = t_id
            AND job_profile_slice_id = s_id;
        IF primary_count <> 1 THEN
            RAISE EXCEPTION
                USING ERRCODE = '23000', CONSTRAINT = 'org_job_profile_slice_job_families_invalid_body', MESSAGE = format('job profile slice job families must have exactly one primary (tenant_id=%s job_profile_slice_id=%s count=%s)', t_id, s_id, primary_count);
        END IF;
        RETURN NULL;
    END;
    $$
    LANGUAGE plpgsql;
    
    DROP TRIGGER IF EXISTS org_job_profile_slice_job_families_validate_trigger ON org_job_profile_slice_job_families;
    
    CREATE CONSTRAINT TRIGGER org_job_profile_slice_job_families_validate_trigger
        AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_slice_job_families DEFERRABLE INITIALLY DEFERRED
        FOR EACH ROW
        EXECUTE FUNCTION org_job_profile_slice_job_families_validate ();
    ```
  - 迁移：用该表替代现有 `org_job_profile_job_families`（现表不具备时间语义；见 10.1/10.4）

### 5.5 不可变字段（v1 冻结）
为保证引用稳定与变更局部性，本计划明确冻结以下规则（后续若要突破，必须另起 dev-plan）：
- `Job Family Group / Job Family / Job Level / Job Profile` 的 `code` 视为不可变；“改 code”不是 Update，而是新建 + 结束旧对象（避免回写所有引用方）。
- `org_job_families.job_family_group_id` 视为不可变（v1 不支持“移动职种到另一个职类”）。
- 引用链路稳定性：
  - Position slice 持有 `job_profile_id`（UUID），不受 Profile code/name 变更影响。
  - Position slice 持有 `job_level_code`（string），因此 Job Level 的 `code` 必须不可变，否则需要全库回写。

### 5.6 SSOT 与 legacy 退场（避免双权威表达）
目标：让 “Job Catalog 的可变属性（name/is_active/…）” 只有一种权威表达：slices。

- **SSOT**：切片表（`*_slices` 与 `*_slice_*`）是唯一读写入口，所有查询按 `as_of_date` 解析。
- **Identity 表定位**：仅承载稳定身份与不可变字段（`id/code` 以及 v1 冻结的稳定关系），不再作为“可变属性”的权威来源。
- **legacy 字段/旧表退场策略**：
  - 迁移期会从现有表回填基线 slices（见 10.1）。
  - 切换后：代码层禁止再读取 identity 表中的 `name/is_active/description/display_order` 等字段；这些 legacy 列是否在同一计划中删除，取决于实施风险评估（见 10.4）。

### 5.7 DB 级 gap-free gate（DEFERRABLE CONSTRAINT TRIGGER）
目标：与 Org Node/Position 同级的 commit-time 强约束——同一时间线 key 的 slices 必须：
1) 相邻自然拼接（`prev.end_date + 1 = next.effective_date`）
2) 末片 `end_date = DATE '9999-12-31'`

实现方式（对齐 `DEV-PLAN-066` 与现有 `org_*_slices_gap_free` 风格）：
- 为每张 `*_slices` 表新增：
  - `{table}_gap_free_assert(tenant_id, entity_id)`：扫描并断言 gap-free
  - `{table}_gap_free_trigger()`：在 INSERT/UPDATE/DELETE 后触发断言
  - `CREATE CONSTRAINT TRIGGER {table}_gap_free ... DEFERRABLE INITIALLY DEFERRED`
- 约束命名要求：constraint 名称必须以 `_gap_free` 结尾，以复用 `mapPgErrorToServiceError` 中对 `ORG_TIME_GAP` 的稳定映射（`modules/org/services/pg_errors.go`）。

覆盖表（本计划新增）：
- `org_job_family_group_slices`：key `(tenant_id, job_family_group_id)`
- `org_job_family_slices`：key `(tenant_id, job_family_id)`
- `org_job_level_slices`：key `(tenant_id, job_level_id)`
- `org_job_profile_slices`：key `(tenant_id, job_profile_id)`

DDL 模板（草案；以下以 `org_job_family_group_slices` 为例，其余表按 table/key 同构替换）：
```sql
CREATE OR REPLACE FUNCTION org_job_family_group_slices_gap_free_assert (p_tenant_id uuid, p_job_family_group_id uuid)
    RETURNS void
    AS $$
DECLARE
    has_gap boolean;
    row_count bigint;
    last_end date;
BEGIN
    WITH ordered AS (
        SELECT
            effective_date,
            end_date,
            lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date
        FROM
            org_job_family_group_slices
        WHERE
            tenant_id = p_tenant_id
            AND job_family_group_id = p_job_family_group_id
        ORDER BY
            effective_date
)
    SELECT
        EXISTS (
            SELECT
                1
            FROM
                ordered
            WHERE
                prev_end_date IS NOT NULL
                AND (prev_end_date + 1) <> effective_date),
            (
                SELECT
                    COUNT(*)
                FROM
                    ordered),
                (
                    SELECT
                        end_date
                    FROM
                        ordered
                    ORDER BY
                        effective_date DESC
                    LIMIT 1) INTO has_gap,
                row_count,
                last_end;
    IF row_count > 0 AND (has_gap OR last_end <> DATE '9999-12-31') THEN
        RAISE EXCEPTION
            USING ERRCODE = '23000', CONSTRAINT = 'org_job_family_group_slices_gap_free', MESSAGE = format('time slices must be gap-free (tenant_id=%s job_family_group_id=%s)', p_tenant_id, p_job_family_group_id);
        END IF;
END;
$$
LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION org_job_family_group_slices_gap_free_trigger ()
    RETURNS TRIGGER
    AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM
            org_job_family_group_slices_gap_free_assert (OLD.tenant_id, OLD.job_family_group_id);
        RETURN NULL;
    END IF;
    IF TG_OP = 'UPDATE' AND (OLD.tenant_id,
        OLD.job_family_group_id) IS DISTINCT FROM (NEW.tenant_id,
    NEW.job_family_group_id) THEN
        PERFORM
            org_job_family_group_slices_gap_free_assert (OLD.tenant_id, OLD.job_family_group_id);
    END IF;
    PERFORM
        org_job_family_group_slices_gap_free_assert (NEW.tenant_id, NEW.job_family_group_id);
    RETURN NULL;
END;
$$
LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS org_job_family_group_slices_gap_free ON org_job_family_group_slices;

CREATE CONSTRAINT TRIGGER org_job_family_group_slices_gap_free
    AFTER INSERT OR UPDATE OR DELETE ON org_job_family_group_slices DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION org_job_family_group_slices_gap_free_trigger ();
```

## 6. 契约（UI/API/HTMX/错误码）
> 目的：让实现阶段“按规格实现”，避免在 controller/service 内即兴补丁式扩展。

### 6.1 UI 读契约（as-of）
- 页面入口：`GET /org/job-catalog?effective_date=YYYY-MM-DD&tab=<tab>&job_family_group_code=<code>&edit_id=<uuid>`
  - `effective_date`：本页唯一 `as_of_date`；若缺失则默认 `today`（UTC day）；若非法则页面返回 400 但仍以 `today` 渲染（对齐现状 controller 行为）。
  - **强约束**：所有列表、下拉 options、校验均必须使用同一个 `effective_date` 解析 slices。
  - `tab`：`family-groups|families|profiles|levels`（现状已有）。
- options 入口（示例）：`GET /org/job-catalog/family-groups/options?effective_date=YYYY-MM-DD&q=...&include_inactive=0|1`
  - `effective_date`：必填（现状缺失会返回 400）。
  - **强约束**：options 必须按 `effective_date` 解析“该日有效切片”，不得读取 legacy 列。

### 6.2 UI 写契约（Correct vs UpdateFromDate）
写入口（现状路由，语义升级为 effective-dated）：
- Create：
  - `POST /org/job-catalog/family-groups`
  - `POST /org/job-catalog/families`
  - `POST /org/job-catalog/levels`
  - `POST /org/job-catalog/profiles`
- Update：
  - `PATCH /org/job-catalog/family-groups/{id}`
  - `PATCH /org/job-catalog/families/{id}`
  - `PATCH /org/job-catalog/levels/{id}`
  - `PATCH /org/job-catalog/profiles/{id}`

请求（Form Data）：
- 通用（Create/Update 均需要）：
  - `effective_date`：必填，`YYYY-MM-DD`（day）
  - `tab/edit_id/job_family_group_code`：用于回到同一视图（现状已存在，不作为领域数据）
- Create（POST）：
  - 不需要 `write_mode`（Create 不是 Correct/UpdateFromDate 的二选一）
  - 语义：创建 identity + 首条 slice（`effective_date` -> `end_of_time`）
- Update（PATCH）：
  - `write_mode`：必填，`correct|update_from_date`（由 UI 明确选择，禁止后端“自动猜”）
  - `patch fields`：对应各对象的 slices 字段集合（见 5.1-5.4）
  - 若 `write_mode=update_from_date` 且 `effective_date == current_slice.effective_date`：返回 `422 ORG_USE_CORRECT`（必须显式走 Correct）
  - Job Profile 的 families：
    - Create 必须提交 `job_families` 集合（至少 1 条且恰好 1 个 primary）
    - Update 若变更 families，则必须提交完整集合（不支持“隐式部分更新”）

响应（HTMX 行为）：
- Success：redirect 到 canonical job-catalog URL（保留 `effective_date/tab/job_family_group_code`）；HTMX 请求使用 `HX-Redirect`，非 HTMX 使用 `302 Found`。
- Error：返回完整 `JobCatalogPage`（包含错误提示），HTTP 状态码跟随 `ServiceError`（常见：400/409/422）。

补充（与 gap-free 不变量一致）：
- 本计划不提供对 Job Catalog identity 的硬删除；停用/启用与改名等变更均通过 slices 表达（保持时间线连续且可复现）。

### 6.3 JSON API 契约（本仓库内同步升级）
> 说明：若存在仓库外消费者，需要另立 dev-plan 做版本化/兼容；本计划默认“仓库内同步升级”，不提供向后兼容。

通用约定：
- List API 支持 query：`effective_date=YYYY-MM-DD`（可选；缺失则默认 `today`/UTC day）
- Write API 必须提供 body：`effective_date=YYYY-MM-DD`（必填）
- Response 只返回 “as-of 视角”的视图行（不返回全量 slices timeline）

列表（as-of）：
- `GET /org/api/job-catalog/family-groups?effective_date=YYYY-MM-DD`
- `GET /org/api/job-catalog/families?job_family_group_id=<uuid>&effective_date=YYYY-MM-DD`
- `GET /org/api/job-catalog/levels?effective_date=YYYY-MM-DD`
- `GET /org/api/job-catalog/profiles?effective_date=YYYY-MM-DD`
- Response：`{"items":[...]}`（字段对齐现有 Row DTO；例如 level 包含 `display_order`，family 包含 `job_family_group_id`）

写入（Create：创建 identity + 首条 slice；Update：Correct 或 UpdateFromDate）：
- Family Groups：
  - `POST /org/api/job-catalog/family-groups`（201）
    - Request：`code/name/is_active?/effective_date`
  - `PATCH /org/api/job-catalog/family-groups/{id}`（200）
    - Request：`effective_date/write_mode/name?/is_active?`
- Families：
  - `POST /org/api/job-catalog/families`（201）
    - Request：`job_family_group_id/code/name/is_active?/effective_date`
  - `PATCH /org/api/job-catalog/families/{id}`（200）
    - Request：`effective_date/write_mode/name?/is_active?`
- Levels：
  - `POST /org/api/job-catalog/levels`（201）
    - Request：`code/name/display_order/is_active?/effective_date`
  - `PATCH /org/api/job-catalog/levels/{id}`（200）
    - Request：`effective_date/write_mode/name?/display_order?/is_active?`
- Profiles：
  - `POST /org/api/job-catalog/profiles`（201）
    - Request：`code/name/description?/is_active?/job_families/effective_date`
  - `PATCH /org/api/job-catalog/profiles/{id}`（200）
    - Request：
      - `effective_date`（required）
      - `write_mode`（required）
      - `name`（optional）
      - `description`（optional tri-state：字段缺失=不改；`null`=清空；string=设置）
      - `is_active`（optional）
      - `job_families`（optional；若出现则必须是完整集合）
    - `job_families` 若出现则必须是完整集合（至少 1 条且恰好 1 个 primary）

示例：UpdateFromDate（family-group）
```json
{
  "effective_date": "2025-01-01",
  "write_mode": "update_from_date",
  "name": "New Name"
}
```

### 6.4 错误码与失败路径（最小集合）
> 目标：失败可解释、可稳定处理；避免 DB 异常直接冒泡成 500。

- `400 ORG_NO_TENANT`：缺少租户上下文
- `400 ORG_INVALID_BODY`：缺少必填字段/非法输入（包含 `_invalid_body` 约束兜底返回）
- `400 ORG_INVALID_QUERY`：非法 id / query
- `404 ORG_NOT_FOUND`：目标 id 不存在（或理论上不应发生的“as-of 找不到 slice”）
- `409 ORG_JOB_CATALOG_CODE_CONFLICT`：Job Catalog code 冲突（unique violation；现状已有）
- `409 ORG_JOB_PROFILE_CODE_CONFLICT`：Job Profile code 冲突（unique violation；现状已有）
- `422 ORG_JOB_CATALOG_PARENT_NOT_FOUND`：Job Catalog parent 外键不存在（现状已有）
- `422 ORG_USE_CORRECT`：`write_mode=update_from_date` 但 `effective_date == current_slice.effective_date`
- `409 ORG_FROZEN_WINDOW`：affected_at 落在冻结窗口之前（若 OrgSettings 开启 freeze enforce；对齐 Org Node/Position 写入语义）
- `409 ORG_OVERLAP`：EXCLUDE no-overlap 触发（time window overlap）
- `409 ORG_TIME_GAP`：commit-time gap-free gate 触发（constraint 名以 `_gap_free` 结尾；见 5.7）
- `500 ORG_INTERNAL`：未预期的 DB 错误（应通过 service 层校验与 pg_errors 映射将其收敛到可解释错误）

补充（primary 约束的失败路径）：
- Profile↔Families 的 “至少 1 条 + 恰好 1 个 primary” 必须在 service 层显式校验并返回 `400 ORG_INVALID_BODY`（对齐现有 `validateJobProfileJobFamiliesSet` 的风格）。
- DB 兜底：deferrable constraint trigger 必须 `RAISE ... ERRCODE='23000' CONSTRAINT='*_invalid_body'`，并在 `modules/org/services/pg_errors.go` 增加 `*_invalid_body -> 400 ORG_INVALID_BODY` 的稳定映射，避免绕过 service/写路径分叉时冒泡 500。

### 6.5 鉴权与租户隔离（Casbin）
- 本计划采用“按资源维度单一对象”的口径（见 **决策 8**）：
  - Job Catalog（职类/职种/职级）：
    - UI/Page/Options/List：`org.job_catalog:read`
    - UI/API 写入：`org.job_catalog:admin`
  - Job Profile（职位模板）：
    - UI/API 读：`org.job_profiles:read`
    - UI/API 写入：`org.job_profiles:admin`
- 兼容性与变更边界：
  - `/org/job-catalog` 仍为聚合页面，但 Job Profile tab 的写入口不得再用 `org.job_catalog:admin` 代替（避免未来权限矩阵分叉时产生外部行为漂移）。
  - 若发现现有/自定义 policy 仅授予 `org.job_catalog:*` 或仅授予 `org.job_profiles:*`，必须通过明确的 policy 迁移与门禁（`make authz-test && make authz-lint`）保持预期能力不回归；禁止用“隐式双校验/或校验”在代码里掩盖权限设计问题。
- 租户隔离：
  - 所有 Repo/SQL 必须包含 `tenant_id = $1`（以及复合外键均带 `tenant_id`）；禁止跨租户引用（DB 外键 + service 校验双保险）。

## 7. 读路径：as-of 解析与“同步展示” (Read Path)
核心规则：**引用方在日期 D 展示什么，就用同一个 D 去解析被引用主数据切片**。

- Job Catalog 页面：所有列表/下拉/options 均以页面 `effective_date` 作为 `as_of_date`，显示该日有效的名称/启停状态。
- Position/Assignment 时间线：
  - 每行渲染 label 的 `as_of_date` 必须与页面“观察日”保持一致：若页面 `effective_date` 落在该行有效期内，则取页面 `effective_date`；否则取该行 `effective_date`（对齐现有 Org UI 的 label 语义）。
  - 先通过 `(position_id, as_of_date)` 找到 position slice（现状），再对 `job_profile_id/job_level_code`（以及 Profile↔Families）做 as-of 解析到 Job Catalog slices，渲染 label（确保主数据属性变更在生效日同步体现在引用展示上）。
- 校验逻辑（写入口）：
  - 创建/更新 position slice 于日期 D：校验 `job_profile_id` 在 D 有效且 active；`job_level_code`（如有）在 D 有效且 active；Profile↔Families 的 primary 在 D 存在。

## 8. 抽象与复用（避免属性爆炸）
### 8.1 统一的 Slice 写入引擎（Service 层）
目标：把“边界运算 + 锁序 + 截断/插入”集中到一处，禁止各属性复制同一套时态写算法。

- 对外提供（概念接口）：
  - `CorrectInPlace(ctx, adapter, as_of_date, patch)`
  - `UpdateFromDate(ctx, adapter, effective_date, patch)`

#### 8.1.1 职责边界（必须保持 Simple）
- engine 负责：
  - day 归一化（UTC 00:00）
  - timeline 级 `pg_advisory_xact_lock`（对齐既有 `lockTimeline` 思路）
  - 锁定 current slice（覆盖 `as_of_date` / `effective_date`）
  - 查询 next slice 起点并计算 `new_end_date`
  - 按固定顺序执行“先制造 gap 再补齐”，避免 transient overlap（对齐 Org/Position 的边界移动经验）
  - 返回 `old/new SliceMeta + affected_at`，供上层写审计/事件与做 cache invalidation 决策
- engine 不负责（必须保留在具体 Service，避免抽象吞掉稳定 contract）：
  - freezeCheck / authz / reason code / audit log / outbox / cache invalidation
  - 错误码/ServiceError 映射与兼容策略

#### 8.1.2 Adapter 最小接口（避免膨胀）
engine 仅依赖“边界与写入原语”，建议 adapter 收敛为以下能力（命名可调整，但职责不变）：
- `LockSliceAt(ctx, tenantID, entityID, as_of_day) (SliceMeta, CurrentPayload, error)`
- `NextSliceEffectiveDate(ctx, tenantID, entityID, after_day) (time.Time, bool, error)`
- `CorrectSliceInPlace(ctx, tenantID, sliceID, patch) error`
- `TruncateSliceEndDate(ctx, tenantID, sliceID, new_end_day) error`
- `InsertSlice(ctx, tenantID, entityID, insert) (SliceMeta, error)`

约束：
- `CurrentPayload/insert/patch` 的字段 trim、默认继承（carry-forward）、业务字段校验由 adapter/上层 service 承担；engine 只编排锁序与 `effective_date/end_date` 的边界操作。

#### 8.1.3 多值关系（Profile↔Families）的原子写入
`job_profile_slices + job_profile_slice_job_families` 属于“一片多行”的组合写入，必须与 slice 边界变更保持同一事务内原子一致：
- 选定：items 视为 slice 的组成部分。
  - Correct：更新当前 slice 的 items（全量替换或差量），依赖 deferrable constraint trigger 兜底校验 `primary_count == 1`（并保证至少一条）。
  - UpdateFromDate：创建新 profile slice 后，将 items 从 current slice carry-forward 并应用 patch 写入新 slice。
- 为保持 engine 通用性：多表写入由 adapter 的 `InsertSlice/CorrectSliceInPlace` 内部实现（或通过显式 hook 机制实现），但不得把业务校验逻辑塞进 engine。

### 8.2 统一的 As-Of 读模型（Repository 层）
目标：杜绝 as-of 口径散落在 controller/templ；同时避免 N+1。

- 选定：默认使用 repo helper（函数 + 明确 SQL 模板）与批量 resolver；**不**以 `*_as_of` 视图作为主方案。
  - 原因：`as_of_date` 在列表中可能“每行不同”（例如任职时间线按每行 `effective_date` 解析），view 不适合表达 per-row as-of。
- 单对象 as-of 查询模板（no-overlap 前提）：
  - `WHERE tenant_id=$1 AND <entity_id>=$2 AND effective_date <= $3 ORDER BY effective_date DESC LIMIT 1`
  - 必要时补 `AND end_date >= $3` 作为防御（尤其在迁移窗口或存在脏数据时）。
- 批量 (id, as_of_day) 解析模板：
  - 使用 `unnest($2::uuid[], $3::date[])` 形成输入对，并通过 `JOIN LATERAL (...)` 为每对取“最新有效 slice”（参考 `pkg/orglabels` 的 mixed-as-of query 思路）。
  - 输出形态建议为 `map[key]value`，由 service 一次性回填，避免 per-row roundtrip。
- 多值关系读模板（Profile slice ↔ Families）：
  1) 先解析 `job_profile_slice_id`（as-of）
  2) join `job_profile_slice_job_families`
  3) 对 `job_family/job_family_group` 的 name/is_active 同样按 **同一个 as_of_day** 解析 slices（保证“属性变化同步展示”）
- 禁止 N+1：
  - 任职/职位列表展示必须使用“单条联表 SQL”或“批量 resolver + map 回填”；不得在 `for` 循环中逐行调用 `Get*AsOf`。
  - Job Catalog API/UI 的典型 N+1 风险点必须在 Phase B 一并消除（举例）：
    - `GET /org/api/job-catalog/profiles`：不得对每个 profile 再调用一次 `ListJobProfileJobFamilies`（必须批量一次性取齐）。
    - `JobFamilyIDOptions`：不得对每个 group 再调用一次 `ListJobFamilies`（必须批量/单条 SQL）；并必须按 `effective_date` 做 as-of。
    - `/org/assignments`：渲染多行时不得 per-row 查询 Job Profile/Job Level/Job Family(Group) label，应使用 mixed-as-of 批量 resolver。

### 8.3 新属性接入清单（Definition of Done）
新增一个新的职位分类维度时，满足以下 DoD（可通过代码审查客观验证）：
1) Schema：新增 identity/slices（含 `no-overlap` + 必要索引），并提供基线 slices 回填策略（避免读路径空洞导致展示不可用）。
2) Write：仅新增该维度的 adapter；不得新增新的“截断/续接/锁序/计算 new_end”算法实现文件；写路径必须复用统一 slice engine。
3) Read：提供 `Get*AsOf` 与（或）`Resolve*AsOf`（批量）repo helper；controller/templ 不直接写 as-of 条件与边界语义。
4) UI：复用统一 `effective_date` 参数；交互上明确区分“更正（Correct）/从某日生效（UpdateFromDate）”两类动作。
5) Tests：覆盖 Correct + UpdateFromDate + 引用方同步展示（以及多值关系的 primary 约束失败路径）。

### 8.4 合理性与可行性评估（聚焦“抽象与复用”）
- **为什么需要抽象**：Org Node/Position 已存在同构的切片写入逻辑（`Lock*SliceAt + Next*SliceEffectiveDate + Truncate + Insert + ORG_USE_CORRECT`）。若 Job Catalog 四类对象继续复制，会快速形成“同一算法多处实现”，并且未来新增属性维度时容易产生 drift（边界计算、并发锁序、错误码映射、测试覆盖的不一致）。
- **为什么该抽象不会膨胀**：engine 明确只做“边界运算 + 锁序 + 截断/插入编排”，把 freeze/authz/audit/outbox/错误码留在具体 service；adapter 只暴露原语，不允许把业务校验塞进 engine（以职责边界约束 Simple）。
- **落地路径（可控、可回退）**：先只在 Job Catalog 维度使用 slice engine，不强制重构 Node/Position；待稳定后再选择性收敛重复代码（可选，不作为本计划验收项）。若未来出现超出 Correct/UpdateFromDate 的复杂操作（例如跨子树移动、delete+stitch），应另起 dev-plan 并保持专用实现，避免把偶然复杂度塞进 engine。

## 9. 写路径：切片写入算法 (Write Path)
对每个 effective-dated 对象提供与 Org/Position 一致的两类操作：
1) **Correct（更正）**：对 `as_of_date` 命中的 slice 原地更新属性，不改变边界（等价 PeopleSoft “Correct History”）。
2) **Update（从某日生效）**：当 `effective_date=D` 落在某个 slice 内但不等于其起点时：
   - 截断当前 slice：`current.end_date = D - 1 day`
   - 插入新 slice：`new.effective_date = D`、`new.end_date = min(current.old_end_date, next_slice.start_date - 1 day)`
   - 新 slice 的属性默认“继承 current”并应用 patch（carry-forward），确保 gap-free 且不 overlap
   - 若 `D == current.effective_date`：返回 `422 ORG_USE_CORRECT`，必须显式走 Correct（避免“后端猜测写意图”导致不可解释）

并发控制与一致性：
- 事务内对时间线 key 获取 `pg_advisory_xact_lock`（对齐 `lockTimeline` 既有思路），串行化同一对象的切片写入，避免竞态导致 overlap/gap。

### 9.1 伪代码：Create（创建 identity + 首片）
1) `tx := inTx(...)`
2) `effective_date := normalizeValidDateUTC(effective_date)`
3) `settings := repo.GetOrgSettings(tx)`
4) `freezeCheck(settings, txTime, effective_date)`（affected_at=首片起点）
5) `entityID := repo.InsertIdentity(...)`
6) `repo.InsertSlice(entityID, payload, effective_date, end_of_time)`
7) `if entity == job_profile: repo.InsertSliceItems(sliceID, job_families)`（服务层先校验“至少 1 条 + 恰好 1 个 primary”）
8) `repo.InsertAuditLog(..., Operation="Create", AffectedAtUTC=effective_date)`（可选：enqueue outbox）
9) `commit`；`InvalidateTenantCacheWithReason("write_commit")`

### 9.2 伪代码：Correct（更正 In-Place）
1) `tx := inTx(...)`
2) `as_of := normalizeValidDateUTC(as_of_date)`
3) `lockTimeline(tableName, tenantID, timelineKey)`
4) `current := adapter.LockSliceAt(tenantID, entityID, as_of)`（返回 slice meta + payload；拿到 `current.effective_date`）
5) `settings := repo.GetOrgSettings(tx)`
6) `freezeCheck(settings, txTime, current.effective_date)`（Correct 影响整片，affected_at=片起点）
7) `adapter.CorrectSliceInPlace(sliceID, patch)`
8) `if entity == job_profile && patch includes job_families: replace slice-items set`
9) `repo.InsertAuditLog(..., Operation="Correct", AffectedAtUTC=current.effective_date)`（可选：enqueue outbox）
10) `commit`（DB 会在 commit-time 执行 5.7 的 gap-free gate）；invalidate cache

### 9.3 伪代码：UpdateFromDate（从某日生效）
1) `tx := inTx(...)`
2) `effective_date := normalizeValidDateUTC(effective_date)`
3) `lockTimeline(tableName, tenantID, timelineKey)`
4) `current := adapter.LockSliceAt(tenantID, entityID, effective_date)`
5) `if current.effective_date == effective_date: return 422 ORG_USE_CORRECT`
6) `settings := repo.GetOrgSettings(tx)`；`freezeCheck(settings, txTime, effective_date)`
7) `nextStart, hasNext := adapter.NextSliceEffectiveDate(tenantID, entityID, effective_date)`
8) `newEnd := current.end_date`
   - `if hasNext: newEnd = min(newEnd, truncateEndDateFromNewEffectiveDate(nextStart))`
9) `adapter.TruncateSliceEndDate(current.id, truncateEndDateFromNewEffectiveDate(effective_date))`
10) `insertPayload := carryForward(current.payload, patch)`
11) `newSlice := adapter.InsertSlice(entityID, insertPayload, effective_date, newEnd)`
12) `if entity == job_profile: carry-forward current slice-items -> new slice-items, then apply patch`
13) `repo.InsertAuditLog(..., Operation="Update", AffectedAtUTC=effective_date)`（可选：enqueue outbox）
14) `commit`（DB gap-free gate + no-overlap EXCLUDE）；invalidate cache

### 9.4 必测边界（消除歧义）
- **夹在两片之间的 UpdateFromDate**：若存在未来片 `nextStart`，新片 `end_date` 必须用 `nextStart - 1 day`（否则会与 next overlap，触发 `ORG_OVERLAP`）。
- **gap-free 兜底**：任何写入导致空档或末片 `end_date != end_of_time`，commit-time 会触发 `ORG_TIME_GAP`（见 5.7）。
- **Correct 的冻结口径**：affected_at 必须取被更正 slice 的 `effective_date`，而不是表单 `as_of_date`（对齐 Org Node Correct 的语义）。

## 10. 迁移策略（Org Goose/Atlas）
### 10.0 受影响入口清单（必须覆盖，否则会制造“identity-only”的脏状态）
> 目的：防止出现“identity 有记录但 slices 缺失”的状态。一旦 Phase B 把读侧切到 slices，这类数据会在 UI/API 侧表现为“消失/404/展示漂移”。

- 导入/脚本直接写表（必须改造为 slices 写入或在切换期禁止执行）：
  - `cmd/org-data/import_cmd.go:1194`（`ensurePositionsImportJobProfileID` 直接 `INSERT` `org_job_*` 与 `org_job_profile_job_families`）
- Repo（读写口径将从 identity 切到 slices）：
  - `modules/org/infrastructure/persistence/org_056_job_catalog_repository.go:14`
- API（包含已确认的 N+1）：
  - `modules/org/presentation/controllers/org_api_controller_056_job_catalog.go:292`（`ListJobProfiles` per-profile 查询 families）
- UI Options（包含已确认的 N+1 与 as-of 漏用）：
  - `modules/org/presentation/controllers/org_ui_controller.go:751`（`JobFamilyGroupOptions` 读取了 `effective_date` 但未用于查询）
  - `modules/org/presentation/controllers/org_ui_controller.go:905`（`JobFamilyIDOptions` N+1：for group -> `ListJobFamilies`；且未按 `effective_date` as-of）
- 测试/seed 插数：必须 `rg \"INSERT INTO org_job_\"` / `rg \"org_job_profile_job_families\"` 全量扫描并迁移到 slices（或改为通过 service/repo helper 写入）。

### 10.1 Phase A：引入 slices + 基线回填（只增量，不改读写）
- [ ] Schema：新增 slices 表与必要索引/约束（no-overlap EXCLUDE + gap-free gate + Profile slice families trigger）；同步更新 `modules/org/infrastructure/persistence/schema/org-schema.sql`（避免 drift）。
- [ ] 基线回填（单次、确定性）：
  - [ ] 所有 Job Catalog identity 对象回填首条 slice：`effective_date = 1900-01-01`，`end_date = end_of_time`。
  - [ ] Profile 的首条 slice 同时回填 slice-job-families（从现有 `org_job_profile_job_families` 复制；primary 约束必须满足）。

### 10.2 Phase B：读切换（全部走 slices as-of）
> 重要：Phase B（读切换）与 Phase C（写切换）必须在同一部署内完成；否则 Phase B 期间新增/更新的 Job Catalog 数据会缺失 slices，导致读侧“消失”。
- [ ] Repo：为 Job Family Group/Family/Level/Profile 提供 `List*AsOf`/`Get*AsOf`/批量 resolver，并替换所有读路径（页面、options、校验、联表展示）。
- [ ] 验收：grep/代码审查确保不再读取 identity 表的 legacy `name/is_active/description/display_order` 字段。
- [ ] 性能与 N+1：修复已确认的 N+1 入口（见 10.0），并把“effective_date 参与查询”作为 options/列表的强约束。

### 10.3 Phase C：写切换（Correct/UpdateFromDate）
- [ ] Service：对 Job Catalog 各对象的 Create/Update 写入口切到 slice engine（并实现 6.2 的 `write_mode` 契约）。
- [ ] Controller/UI/API：落地 `write_mode` 传参，且对 `ORG_USE_CORRECT` 给出可读提示。
- [ ] 导入/脚本/seed：改造所有直接写 `org_job_*` identity 的路径为 slices 写入（至少包含 `cmd/org-data/import_cmd.go:1194`）；禁止出现仅写 identity 不写 slices 的分叉。

### 10.4 Phase D：退场与清理（避免双 SSOT 长期存在）
> 选项在实施时二选一（必须明确选择并执行，否则视为未完成）。

- 选项 A（推荐，完整退场）：在确认所有读写都已切到 slices 后：
  - [ ] 删除/下线旧表 `org_job_profile_job_families`（已完成数据迁移到 `org_job_profile_slice_job_families`）。
  - [ ] 评估并删除 identity 表中不再使用的 legacy 列（若存在未知依赖则延期，但必须登记退场计划与搜索证据）。
- 选项 B（风险更低，分步退场）：本计划只保证“逻辑 SSOT= slices”，legacy 列保留但不再被代码读取；并在后续 `DEV-PLAN-075B`（待立）中完成列/表清理。

## 11. 测试与验收标准 (Acceptance Criteria)
### 11.1 验收覆盖（对应本计划 3 个诉求）
- [ ] （1）无空档：对任一 Job Family Group/Family/Level/Profile，在不同日期做 2 次 Update，最终 slices 满足 no-overlap 且自然拼接；不存在空档。
- [ ] （1）DB 兜底：新建的 `*_slices_gap_free` DEFERRABLE CONSTRAINT TRIGGER 生效；若写入导致 gap-free 断言失败，应返回 `409 ORG_TIME_GAP`（而非 silent drift）。
- [ ] （2）同步展示：对某 Job Family Group 在 D2 改名；在 `/org/assignments` 选 `effective_date=D1` 与 `D2` 时，职类列展示不同名称且符合切片；无需改动任职/职位数据。
- [ ] （2）同步校验：将某 Job Level 在 D2 设为 inactive；创建 position slice 于 D2 选择该 `job_level_code` 必须拒绝，但历史 D1 不受影响。
- [ ] （2）冻结窗口：若 OrgSettings 开启 freeze enforce，则对冻结窗口之前的 effective_date 做 Correct/UpdateFromDate 必须拒绝并返回 `409 ORG_FROZEN_WINDOW`。
- [ ] （2）primary 约束稳定失败：Profile↔Families 的 “至少 1 条 + 恰好 1 个 primary” 若失败必须返回 `400 ORG_INVALID_BODY`；无论由 service 校验触发还是 DB trigger 兜底触发，都不得冒泡 `500 ORG_INTERNAL`。
- [ ] （2）无 N+1：
  - `/org/assignments`（以及复用入口）在多行渲染时，Job Catalog 的 as-of 解析必须采用“单条联表 SQL”或“批量 resolver”，不得出现 per-row 查询（以代码审查作为验收基线）。
  - `GET /org/api/job-catalog/profiles` 返回 profiles+families 时不得 per-profile 查询（必须批量/单条联表 SQL）。
  - UI Options（例如 `JobFamilyIDOptions`）不得出现 for group -> `ListJobFamilies` 的 N+1；并且必须按 `effective_date` 做 as-of。
- [ ] （3）复用抽象：在完成 4 类对象后，新增第 5 类维度时，不新增新的切片边界算法实现文件；变更集应主要由 schema + adapter + as-of repo helper + tests 构成。

### 11.2 门禁与回归
- [ ] 按触发器矩阵执行并通过（SSOT：`AGENTS.md`）；特别是 Org 迁移门禁与文档门禁。
- [ ] Readiness：创建并填写 `docs/dev-records/DEV-PLAN-075-READINESS.md`（记录关键命令/结果与时间戳；避免把“验证证据”散落在对话里）。

## 12. 依赖与里程碑 (Dependencies & Milestones)
### 12.1 依赖
- Valid Time 日粒度统一（SSOT）：`docs/dev-plans/064-effective-date-day-granularity.md`
- gap-free gate（commit-time DEFERRABLE TRIGGER）的既有落地（参考实现）：`docs/dev-plans/066-auto-stitch-time-slices-on-delete.md` 与 `modules/org/infrastructure/persistence/schema/org-schema.sql`
- Org Atlas+Goose 工具链与门禁（SSOT）：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- Job Architecture（背景约束）：`docs/dev-plans/072-job-architecture-workday-alignment.md`

### 12.2 里程碑
1. [ ] Schema 评审：确认新增表清单与字段；**获得“新增表”手工确认后**落地迁移（10.1）
2. [ ] Phase A：落地 slices + 基线回填（10.1）
3. [ ] Phase B：读切换（10.2）——包括 `/org/job-catalog`、options、Position/Assignment 展示与写入口校验
4. [ ] Phase C：写切换（10.3）——落地 `write_mode` 契约与 `ORG_USE_CORRECT` 失败路径
5. [ ] 性能回归：按 11.1 的 “无 N+1” 约束完成代码审查与必要的批量 resolver
6. [ ] Phase D：完成退场与清理（10.4，必须二选一）
7. [ ] 验证：按 SSOT 门禁全量通过，并填写 `docs/dev-records/DEV-PLAN-075-READINESS.md`（含时间戳）

## 13. 运维与回滚 (Ops & Rollback)
- Feature Flag：不引入（对齐仓库约束：早期阶段避免过度运维/开关切换；见 `AGENTS.md` 3.6）。
- 日志与审计：
  - 复用现有 `ServiceError` + request_id/tenant_id 的结构化日志。
  - 若本计划在实现阶段补齐 Job Catalog 写入的 audit log/outbox，则必须保证字段与 Org 既有审计口径一致（不在 slice engine 内实现）。
- 监控：不新增专用监控；依赖既有 API instrumentation（`instrumentAPI`）与门禁回归。
- 回滚策略（按 Phase）：
  - Phase A（仅增量 schema + 基线回填）：可通过代码回滚 + 保留新增表（不影响现有读写）；开发环境可执行 down/清理。
  - Phase B（读切换）：可通过代码回滚恢复读取 legacy（slices 仍保留，作为旁路数据）。
  - Phase C（写切换）：原则上视为“切换点”，避免在未完成验证前进入；若必须回滚到 legacy 写入，需要明确“历史能力退回 SCD1”的代价，并提供一次性脚本把 `as_of=today` 的 slices 同步到 legacy 列用于临时止血（不作为默认路径）。
  - Phase D（退场清理）：包含破坏性删除；仅在 Phase B/C 稳定后执行，且执行前必须完成 readiness 记录与回滚评审。
