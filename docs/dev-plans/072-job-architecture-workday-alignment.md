# DEV-PLAN-072：对标 Workday 的职位体系（Job Architecture）方案（按 DEV-PLAN-001 细化）

**状态**: 实施中（2025-12-29 18:00 UTC）

> 本文为“代码级详细设计（TDD）”规格：以 `DEV-PLAN-001` 为模板，目标是让实现阶段无需再补做关键设计决策即可直接编码。

## 1. 背景与上下文 (Context)
- **需求来源**：
  - 用户确认：采用 **Workday 风格**职位体系；`Job Profile` 中文名统一为 **职位模板**；**不再设置/维护 `Job Role`**。
  - 本仓库基线：Position/有效期/校验与 UI 入口主要来自 `DEV-PLAN-050/052/056/067`，术语对齐参考 `DEV-PLAN-060`。
- **当前痛点**：
  - 现状是“Job Catalog 四级（含 `Job Role`）+ Job Profile（绑定 Role）并存”，Position slice 既有 `job_*_code` 又有 `job_profile_id`；一致性靠“冲突拒绝”维持（见 `modules/org/services/position_catalog_validation_056.go`）。
  - 结果是同一事实两套表达，复杂度转移到 UI/接口/用户心智，且后续引入 effective-dated 主数据会进一步放大漂移与兼容成本。
- **业务价值**：
  - 收敛到 Workday 常见心智：以 **职位模板（Job Profile）**为唯一主入口，Position 只引用模板（可选职级），分类路径可派生，避免“双入口/双 SSOT”。
  - 支持业务提出的“一个职务可属于多个职种且比例合计=100%”需求，同时保持可派生的唯一主路径（主归属职种）。

**本计划冻结的关键决策（实现不得反复横跳）**：
- `Job Profile` = **职位模板**，并作为 Position 的唯一主引用对象与治理入口。
- **移除 `Job Role`**（表/接口/UI/校验链路全部退场）。
- 职位模板 ↔ 职种：**多对多 + 比例合计=100% + 主归属唯一**。
- Position 写入口：`job_profile_id`（必填）+ `job_level_code`（可选）；创建职位时默认从职位模板带出“归属与比例”，但允许在职位上覆盖；不再接受 `job_*_code` 作为写入参数。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] 明确并固化：职类/职种/职位模板/职级/职位 的语义边界、主从关系与不变量（单入口、比例合计=100%、主归属唯一）。
  - [ ] 统一术语：`Job Family Group=职类`、`Job Family=职种`、`Job Profile=职位模板`、`Job Level=职级`；并在 `DEV-PLAN-060` 标注映射（已补充 2.1.1）。
  - [ ] 以 schema 级约束 + 集中校验点强制不变量：避免把一致性分散到多处 if/else。
  - [ ] 给出可执行的迁移与破坏性更正路径：从“含 Role 的四级 Catalog”收敛到“Profile 中心”。
- **非目标 (Out of Scope)**：
  - 不在本计划内一次性引入 Job Architecture 主数据的 effective-dated（Phase 2 将另起 dev-plan，遵循 `DEV-PLAN-064` 的 Valid Time=DATE）。
  - 不在本计划内覆盖招聘全链路（Job Requisition/Offer/Hire）与薪酬体系（Comp Grade/Pay Plan）。

## 2.1 工具链与门禁（SSOT 引用）
> 只声明“本计划命中哪些触发器”，命令细节以 SSOT 为准，避免 drift。

- **触发器清单（本计划命中）**：
  - [x] Go 代码（触发器矩阵：`AGENTS.md`）
  - [x] `.templ` / Tailwind（`AGENTS.md`）
  - [x] 多语言 JSON（`AGENTS.md`）
  - [x] 路由治理（`docs/dev-plans/018-routing-strategy.md`）
  - [x] DB 迁移 / Schema（Org Atlas+Goose：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [ ] Authz（若涉及策略/对象调整：`AGENTS.md` 与相关 runbook）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Valid Time 语义：`docs/dev-plans/064-effective-date-day-granularity.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 术语与映射（Workday / PeopleSoft / 本项目）
- Workday `Job Family Group / Job Family / Job Profile / Job Level / Position`
  - `Job Family Group`：职类
  - `Job Family`：职种
  - `Job Profile`：职位模板（本项目 SSOT 采用）
  - `Job Level`：职级
  - `Position`：职位（席位实例，effective-dated）
- PeopleSoft 口径（参考 `DEV-PLAN-060`）：
  - PeopleSoft **职务（Job Code）** ≈ Workday **Job Profile** ≈ 本项目 **职位模板**
  - PeopleSoft **职级（Job Grade）** ≈ Workday **Job Level/Grade** ≈ 本项目 **职级**

### 3.2 架构图 (Mermaid)
```mermaid
graph TD
  JFG[职类 Job Family Group] --> JF[职种 Job Family]
  JP[职位模板 Job Profile] --> JPA[模板归属职种(比例+主归属)]
  JF --> JPA
  JL[职级 Job Level]
  POS[职位 Position(time slices)] --> JP
  POS --> JL
```

### 3.3 现状盘点（Research：当前为何会复杂）
- 当前实现/契约冻结（`DEV-PLAN-052/056`）：
  - Position slice 同时承载 `job_family_group_code/job_family_code/job_role_code/job_level_code` 与 `job_profile_id`。
  - 校验逻辑要求 Profile 与 Catalog 路径一致（不一致则拒绝），且 Job Level 必须“隶属 Role”（见 `modules/org/services/position_catalog_validation_056.go`）。
- 复杂度来源：
  - 双入口：既可从 Catalog 选路径再选 Profile，也可先选 Profile 再补 Catalog；任何不一致都变成“冲突拒绝”，把复杂度推给 UI 与用户。

### 3.4 差异比较：本项目 `Job Role` vs Workday `Job Profile`
- Workday `Job Profile` 是岗位定义/模板，是 Position 等对象的常用主引用对象（天然承担编码/职责/资格/能力/可用等级等治理入口）。
- 本项目 `Job Role` 更像“为了把 Catalog 做成四级路径插入的中间层”，与 `Job Profile` 语义重叠但无法成为唯一权威入口。

结论：**保留 `Job Profile=职位模板` 作为唯一主入口；移除 `Job Role`。**

### 3.5 关系模型（概念契约与不变量）
1) **职类 → 职种**：一对多（每个职种必须且仅归属一个职类）。
2) **职种 ↔ 职位模板**：多对多，但必须满足：
   - 每个职位模板必须配置“归属职种（含比例）”集合，且比例合计=**100%**；
   - 归属集合中必须且仅能有一个“**主归属职种**”，用于派生 Position 的唯一分类路径（职种/职类）；
   - 非主归属仅用于统计/治理（例如跨序列分摊），不作为 Position 的写入口。

示例：职位模板“人事行政主管”可同时归属“人力资源管理（60%）”与“行政管理（40%）”，其中“人力资源管理”为主归属职种。

### 3.6 关键设计决策（ADR 摘要）
- **决策 1：移除 `Job Role`**
  - 选项 A：保留 Role 作为四级 Catalog（现状）。缺点：双入口/双 SSOT，冲突校验复杂。
  - 选项 B：Role 仅做兼容层长期存在。缺点：历史包袱常驻，复杂度不可退场。
  - 选项 C（选定）：删除 Role（表/接口/UI/校验全退场），Profile 直连职种（多对多）。
- **决策 2：Position 写入口单入口**
  - 选项 A（选定）：仅接受 `job_profile_id`（必填）+ `job_level_code`（可选），不再接受 `job_*_code` 写入。
  - 选项 B：继续接受 `job_*_code` 但强制与派生一致。缺点：兼容面大，仍是“双入口”。
- **决策 3：归属比例合计=100% 的强制方式**
  - 选项 A：仅服务端校验。缺点：绕过服务（直写 DB/脚本）会产生脏数据。
  - 选项 B（选定）：服务端校验 + DB deferred constraint trigger（提交时校验 sum=100 与 primary 唯一）。
- **决策 4：职级（Job Level）为租户级独立主数据**
  - 选项 A：职级隶属 Role（现状）。缺点：Role 退场后无法成立。
  - 选项 B：职级隶属职种/职类。缺点：仍然耦合分类路径，且不符合“职级相对独立”的口径。
  - 选项 C（选定）：职级 tenant-global；Position 上可选填写；不建立“职位模板↔职级”的强绑定关系（与用户确认一致）。
- **决策 5：职位（Position slice）继承但可覆盖“归属与比例”**
  - 选项 A：仅在职位模板维护比例，Position 只读派生。优点：更接近 Workday 原教旨（避免实例侧漂移）；缺点：用户明确要求“创建职位实例时带出默认值但可修改覆盖”。
  - 选项 B（选定）：职位模板维护默认比例；Position slice 落地一份“归属与比例”（默认复制，可覆盖），并用 DB deferred constraint trigger 强制 `sum=100` + `primary` 唯一。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本节给出“最终状态”的 schema 规格；迁移以 Org Goose/Atlas 工具链落地（SSOT：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）。

### 4.1 目标模型（最终状态）
- 保留：
  - `org_job_family_groups`（职类）
  - `org_job_families`（职种，FK 到职类）
  - `org_job_profiles`（职位模板，移除 role 关联）
- 调整：
  - `org_job_levels`：移除 `job_role_id`，变更为 tenant-global（唯一约束收敛到 `(tenant_id, code)`）
- 新增：
  - `org_job_profile_job_families`：模板↔职种归属与比例（sum=100 + primary 唯一）
  - `org_position_slice_job_families`：职位切片↔职种归属与比例（默认从模板复制，可覆盖；sum=100 + primary 唯一）
- 删除：
  - `org_job_roles`

### 4.2 Schema 定义（SQL 规格）
#### 4.2.1 `org_job_profiles`（移除 `job_role_id`）
```sql
ALTER TABLE org_job_profiles
  DROP CONSTRAINT IF EXISTS org_job_profiles_role_fk;

ALTER TABLE org_job_profiles
  DROP COLUMN IF EXISTS job_role_id;

DROP INDEX IF EXISTS org_job_profiles_tenant_role_active_code_idx;

CREATE INDEX IF NOT EXISTS org_job_profiles_tenant_active_code_idx
  ON org_job_profiles (tenant_id, is_active, code);
```

#### 4.2.2 `org_job_profile_job_families`（新增：归属+比例+主归属）
```sql
CREATE TABLE IF NOT EXISTS org_job_profile_job_families (
  tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
  job_profile_id uuid NOT NULL,
  job_family_id uuid NOT NULL,
  allocation_percent int NOT NULL,
  is_primary boolean NOT NULL DEFAULT FALSE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT org_job_profile_job_families_pkey PRIMARY KEY (tenant_id, job_profile_id, job_family_id),
  CONSTRAINT org_job_profile_job_families_profile_fk FOREIGN KEY (tenant_id, job_profile_id)
    REFERENCES org_job_profiles (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT org_job_profile_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id)
    REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT org_job_profile_job_families_allocation_check CHECK (allocation_percent >= 1 AND allocation_percent <= 100)
);

CREATE UNIQUE INDEX IF NOT EXISTS org_job_profile_job_families_primary_unique
  ON org_job_profile_job_families (tenant_id, job_profile_id)
  WHERE is_primary = TRUE;

CREATE INDEX IF NOT EXISTS org_job_profile_job_families_tenant_family_profile_idx
  ON org_job_profile_job_families (tenant_id, job_family_id, job_profile_id);
```

#### 4.2.3 `org_job_profile_job_families` 约束触发器（deferred）
> 目标：允许在同一事务内“先删后插/批量调整比例”，并在事务提交时校验 sum=100 与 primary 恰好一个。

```sql
CREATE OR REPLACE FUNCTION org_job_profile_job_families_validate()
RETURNS trigger AS $$
DECLARE
  t_id uuid;
  p_id uuid;
  sum_pct int;
  primary_count int;
BEGIN
  t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
  p_id := COALESCE(NEW.job_profile_id, OLD.job_profile_id);

  SELECT
    COALESCE(SUM(allocation_percent), 0),
    COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
  INTO sum_pct, primary_count
  FROM org_job_profile_job_families
  WHERE tenant_id = t_id AND job_profile_id = p_id;

  IF sum_pct <> 100 THEN
    RAISE EXCEPTION 'job profile job families allocation must sum to 100 (tenant_id=%, job_profile_id=%, sum=%)',
      t_id, p_id, sum_pct
    USING ERRCODE = '23514';
  END IF;

  IF primary_count <> 1 THEN
    RAISE EXCEPTION 'job profile job families must have exactly one primary (tenant_id=%, job_profile_id=%, count=%)',
      t_id, p_id, primary_count
    USING ERRCODE = '23514';
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;

CREATE CONSTRAINT TRIGGER org_job_profile_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_job_profile_job_families_validate();
```

#### 4.2.4 `org_position_slice_job_families`（新增：职位归属+比例+主归属）
```sql
CREATE TABLE IF NOT EXISTS org_position_slice_job_families (
  tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
  position_slice_id uuid NOT NULL,
  job_family_id uuid NOT NULL,
  allocation_percent int NOT NULL,
  is_primary boolean NOT NULL DEFAULT FALSE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT org_position_slice_job_families_pkey PRIMARY KEY (tenant_id, position_slice_id, job_family_id),
  CONSTRAINT org_position_slice_job_families_slice_fk FOREIGN KEY (tenant_id, position_slice_id)
    REFERENCES org_position_slices (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT org_position_slice_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id)
    REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT org_position_slice_job_families_allocation_check CHECK (allocation_percent >= 1 AND allocation_percent <= 100)
);

CREATE UNIQUE INDEX IF NOT EXISTS org_position_slice_job_families_primary_unique
  ON org_position_slice_job_families (tenant_id, position_slice_id)
  WHERE is_primary = TRUE;

CREATE INDEX IF NOT EXISTS org_position_slice_job_families_tenant_family_slice_idx
  ON org_position_slice_job_families (tenant_id, job_family_id, position_slice_id);
```

#### 4.2.5 `org_position_slice_job_families` 约束触发器（deferred）
> 目标：允许在同一事务内“先删后插/批量调整比例”，并在事务提交时校验 sum=100 与 primary 恰好一个。

```sql
CREATE OR REPLACE FUNCTION org_position_slice_job_families_validate()
RETURNS trigger AS $$
DECLARE
  t_id uuid;
  s_id uuid;
  sum_pct int;
  primary_count int;
BEGIN
  t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
  s_id := COALESCE(NEW.position_slice_id, OLD.position_slice_id);

  SELECT
    COALESCE(SUM(allocation_percent), 0),
    COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
  INTO sum_pct, primary_count
  FROM org_position_slice_job_families
  WHERE tenant_id = t_id AND position_slice_id = s_id;

  IF sum_pct <> 100 THEN
    RAISE EXCEPTION 'position slice job families allocation must sum to 100 (tenant_id=%, position_slice_id=%, sum=%)',
      t_id, s_id, sum_pct
    USING ERRCODE = '23514';
  END IF;

  IF primary_count <> 1 THEN
    RAISE EXCEPTION 'position slice job families must have exactly one primary (tenant_id=%, position_slice_id=%, count=%)',
      t_id, s_id, primary_count
    USING ERRCODE = '23514';
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS org_position_slice_job_families_validate_trigger ON org_position_slice_job_families;

CREATE CONSTRAINT TRIGGER org_position_slice_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_position_slice_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_position_slice_job_families_validate();
```

#### 4.2.6 `org_job_levels`（tenant-global；移除 `job_role_id`）
```sql
ALTER TABLE org_job_levels
  DROP CONSTRAINT IF EXISTS org_job_levels_role_fk;

ALTER TABLE org_job_levels
  DROP CONSTRAINT IF EXISTS org_job_levels_tenant_id_role_code_key;

DROP INDEX IF EXISTS org_job_levels_tenant_role_active_order_code_idx;

ALTER TABLE org_job_levels
  DROP COLUMN IF EXISTS job_role_id;

ALTER TABLE org_job_levels
  ADD CONSTRAINT org_job_levels_tenant_id_code_key UNIQUE (tenant_id, code);

CREATE INDEX IF NOT EXISTS org_job_levels_tenant_active_order_code_idx
  ON org_job_levels (tenant_id, is_active, display_order, code);
```

#### 4.2.7 `org_position_slices`（Position 写入契约落地到 DB）
> 说明：本计划冻结 Position 写入口不再接受 `job_*_code`；职类/职种由 `org_position_slice_job_families`（主归属）派生（默认从职位模板复制，可覆盖）。

```sql
ALTER TABLE org_position_slices
  ALTER COLUMN job_profile_id SET NOT NULL;

ALTER TABLE org_position_slices
  DROP COLUMN IF EXISTS job_family_group_code,
  DROP COLUMN IF EXISTS job_family_code,
  DROP COLUMN IF EXISTS job_role_code;

ALTER TABLE org_position_slices
  ADD CONSTRAINT org_position_slices_job_profile_fk
  FOREIGN KEY (tenant_id, job_profile_id)
  REFERENCES org_job_profiles (tenant_id, id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS org_position_slices_tenant_profile_effective_idx
  ON org_position_slices (tenant_id, job_profile_id, effective_date);
```

#### 4.2.8 `org_job_roles`（删除）
```sql
DROP TABLE IF EXISTS org_job_roles;
```

#### 4.2.9 `org_job_profile_allowed_job_levels`（删除：不再维护模板↔职级关系）
```sql
DROP TABLE IF EXISTS org_job_profile_allowed_job_levels;
```

### 4.3 迁移策略（破坏性更正；Up 规格）
> 原则：项目初期按“破坏性更正（最彻底）”执行；迁移中若发现数据不满足新不变量，应直接失败并要求先修正数据。

1. 新增 `org_job_profile_job_families` 与约束触发器（允许先回填）。
2. 确保所有 `org_position_slices` 都有 `job_profile_id`（为后续 drop `job_*_code` 做准备）：
   - Preflight（空值检测）：
     ```sql
     SELECT COUNT(*) AS cnt
     FROM org_position_slices
     WHERE job_profile_id IS NULL;
     ```
   - 回填策略（确定性）：若 `job_profile_id` 为空，则根据旧 `job_family_group_code/job_family_code/job_role_code` 解析出 `org_job_roles.id`，并选择一个职位模板：
     - 若该 Role 下存在多个模板，按 `is_active DESC, created_at ASC, code ASC` 选取第一个（确定性）；
     - 若该 Role 下不存在模板，则创建一个“迁移生成的职位模板”，并写回 slices（模板 code 建议使用三段码避免冲突：`{job_family_group_code}-{job_family_code}-{job_role_code}`）。
3. 回填职位模板主归属职种（默认 100% 主归属）：
   - 依据旧关系：`org_job_profiles.job_role_id -> org_job_roles.job_family_id`
   - 为每个模板插入归属：`allocation_percent=100, is_primary=true`
   - 建议 SQL：
     ```sql
     INSERT INTO org_job_profile_job_families (tenant_id, job_profile_id, job_family_id, allocation_percent, is_primary)
     SELECT p.tenant_id, p.id, r.job_family_id, 100, TRUE
     FROM org_job_profiles p
     JOIN org_job_roles r ON r.tenant_id = p.tenant_id AND r.id = p.job_role_id;
     ```
4. 新增 `org_position_slice_job_families` 与约束触发器（允许先回填）。
5. 回填职位切片归属与比例（默认从模板复制，可覆盖）：
   - 对每个 Position slice，从其 `job_profile_id` 复制模板归属到实例归属：
     ```sql
     INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, allocation_percent, is_primary)
     SELECT s.tenant_id, s.id, jpf.job_family_id, jpf.allocation_percent, jpf.is_primary
     FROM org_position_slices s
     JOIN org_job_profile_job_families jpf
       ON jpf.tenant_id = s.tenant_id
      AND jpf.job_profile_id = s.job_profile_id;
     ```
6. 处理职级去 Role 绑定的冲突风险（必须在迁移中确定性解决）：
   - 目标约束变为 `(tenant_id, code)` 唯一。
   - 由于 Position 侧只引用 `job_level_code`（字符串），不引用 `job_level_id`，因此更合理的收口方式是“按 code 合并”：同租户同 code 的多行只保留一行，其余删除；然后再 drop `job_role_id` 并加唯一约束。
   - Preflight（重复检测）：
     ```sql
     SELECT tenant_id, code, COUNT(*) AS cnt
     FROM org_job_levels
     GROUP BY tenant_id, code
     HAVING COUNT(*) > 1;
     ```
   - 建议迁移步骤（先去重，再加唯一约束）：
     ```sql
     -- 保留一个 canonical row（优先启用，其次 display_order 更小，其次更早 created_at）
     WITH ranked AS (
       SELECT
         id,
         tenant_id,
         code,
         ROW_NUMBER() OVER (
           PARTITION BY tenant_id, code
           ORDER BY is_active DESC, display_order ASC, created_at ASC, id ASC
         ) AS rn
       FROM org_job_levels
     )
     DELETE FROM org_job_levels l
     USING ranked r
     WHERE l.id = r.id
       AND r.rn > 1;
     ```
7. `org_job_profiles`：删除 `job_role_id`（先 drop FK/索引，再 drop column）。
8. `org_job_levels`：删除 `job_role_id`，并添加新的唯一约束与索引。
9. 删除 `org_job_profile_allowed_job_levels` 表，以及相关 API/Service/Repo 代码（不再维护模板↔职级关系）。
10. 删除 `org_job_roles` 表，以及所有依赖它的查询、校验与 UI/API 路由。
11. `org_position_slices`：
   - 强制 `job_profile_id` NOT NULL（若仍存在 NULL，迁移应直接失败并要求先修正数据）
   - 删除 `job_family_*_code/job_role_code` 列
   - 加入 `job_profile_id` 外键与索引

> Down：生产通常不做破坏性 down；本项目早期如需回滚以“回滚迁移 + 数据重建/seed”为主（实现阶段在 readiness 记录具体命令与证据）。

### 4.4 Effective Dating（后续增量路线）
- Phase 1（本计划范围）：主数据保持 SCD1；Position 已是 effective-dated（遵循 `DEV-PLAN-064`）。
- Phase 2（另起 dev-plan）：为职类/职种/职位模板（必要时职级）引入有效期（date），并提供 `as_of` 读取视角以对齐 Workday 的行业共识。

## 5. 接口契约 (API Contracts)
> 约定：Org JSON API 前缀为 `/org/api`（见 `modules/org/presentation/controllers/org_api_controller.go`）；UI/HTMX 路由前缀为 `/org`。

### 5.1 JSON API：职位模板（Job Profile）
#### 5.1.1 `GET /org/api/job-profiles`
- Query（建议）：
  - `job_family_id`（可选）：筛选“主归属职种”为指定职种的模板
  - `q`（可选）：按 code/name 模糊查询
- Response（200）：
```json
[
  {
    "id": "uuid",
    "code": "HR-ADMIN-SUP",
    "name": "人事行政主管",
    "description": "…",
    "is_active": true,
    "job_families": [
      { "job_family_id": "uuid", "allocation_percent": 60, "is_primary": true },
      { "job_family_id": "uuid", "allocation_percent": 40, "is_primary": false }
    ]
  }
]
```

#### 5.1.2 `POST /org/api/job-profiles`
- Request：
```json
{
  "code": "HR-ADMIN-SUP",
  "name": "人事行政主管",
  "description": "…",
  "is_active": true,
  "job_families": [
    { "job_family_id": "uuid", "allocation_percent": 60, "is_primary": true },
    { "job_family_id": "uuid", "allocation_percent": 40, "is_primary": false }
  ]
}
```
- Error Codes（建议最小集）：
  - `400 ORG_INVALID_BODY`：缺少必填字段或比例非法（<1 或 >100）。
  - `409 ORG_JOB_PROFILE_CODE_CONFLICT`：code 冲突（沿用现有 pg 错误映射口径）。
  - `422 ORG_JOB_PROFILE_JOB_FAMILIES_INVALID`：比例合计≠100 或主归属不唯一。
  - `422 ORG_JOB_FAMILY_NOT_FOUND` / `ORG_JOB_FAMILY_INACTIVE`：归属职种不存在/停用。

#### 5.1.3 `PATCH /org/api/job-profiles/{id}`
- Request（允许更新）：
```json
{
  "name": "…",
  "description": "…",
  "is_active": false,
  "job_families": [
    { "job_family_id": "uuid", "allocation_percent": 100, "is_primary": true }
  ]
}
```
- Error Codes：同 `POST`；另加
  - `404 ORG_JOB_PROFILE_NOT_FOUND`

#### 5.1.4 `POST /org/api/job-profiles/{id}:set-allowed-levels`（删除）
> 用户确认“不需要指定职务与职级关系”，本计划不再维护“职位模板↔职级”的允许集合；实现阶段应删除该接口与对应表 `org_job_profile_allowed_job_levels`。

### 5.2 JSON API：职类/职种/职级（Job Catalog 子资源）
> `Job Role` 退场后：以下资源应收敛为三类主数据（职类/职种/职级），并在 UI 中以“职位分类”页聚合维护入口（其中 `Profiles` 页签为“职位模板”，作为主维护入口；见 `DEV-PLAN-072A`）。

#### 5.2.1 职类（Job Family Group）
- `GET /org/api/job-catalog/family-groups`
- `POST /org/api/job-catalog/family-groups`
  - Request：
    ```json
    { "code": "PROF", "name": "专业类", "is_active": true }
    ```
  - Error：
    - `409 ORG_JOB_CATALOG_CODE_CONFLICT`
    - `400 ORG_INVALID_BODY`
- `PATCH /org/api/job-catalog/family-groups/{id}`

#### 5.2.2 职种（Job Family）
- `GET /org/api/job-catalog/families?job_family_group_id=uuid`
- `POST /org/api/job-catalog/families`
  - Request：
    ```json
    { "job_family_group_id": "uuid", "code": "HRM", "name": "人力资源管理", "is_active": true }
    ```
  - Error：
    - `422 ORG_JOB_CATALOG_PARENT_NOT_FOUND`（职类不存在）
    - `409 ORG_JOB_CATALOG_CODE_CONFLICT`
    - `400 ORG_INVALID_BODY`
- `PATCH /org/api/job-catalog/families/{id}`

#### 5.2.3 职级（Job Level，tenant-global）
> 变更点：移除 `job_role_id` 依赖；唯一约束收敛到 `(tenant_id, code)`。

- `GET /org/api/job-catalog/levels`
- `POST /org/api/job-catalog/levels`
  - Request：
    ```json
    { "code": "L3", "name": "P3", "display_order": 30, "is_active": true }
    ```
  - Error：
    - `409 ORG_JOB_CATALOG_CODE_CONFLICT`（实现阶段需将新的约束名 `org_job_levels_tenant_id_code_key` 纳入 pg 错误映射）
    - `400 ORG_INVALID_BODY`
- `PATCH /org/api/job-catalog/levels/{id}`

#### 5.2.4 `Job Role` 资源（删除）
- 必须从路由与控制器移除：
  - `GET/POST/PATCH /org/api/job-catalog/roles`

### 5.3 HTMX UI：职位分类（`Job Catalog`）页面
> UI 入口：组织与职位 → `Job Catalog`，中文展示名统一为“职位分类”；其中 `Profiles` 页签中文名为“职位模板”（见 `DEV-PLAN-072A`）。

#### 5.3.1 路由（最终状态）
- 页面：`GET /org/job-catalog`
- 写操作（返回更新后的 `#org-job-catalog-page` 片段；错误时返回同一片段并填充错误提示）：
  - `POST /org/job-catalog/family-groups` / `PATCH /org/job-catalog/family-groups/{id}`
  - `POST /org/job-catalog/families` / `PATCH /org/job-catalog/families/{id}`
  - `POST /org/job-catalog/profiles` / `PATCH /org/job-catalog/profiles/{id}`（新增）
  - `POST /org/job-catalog/levels` / `PATCH /org/job-catalog/levels/{id}`
- Options（Combobox）：
  - `GET /org/job-catalog/family-groups/options`
  - `GET /org/job-catalog/families/options`
  - `GET /org/job-catalog/levels/options`
  - `GET /org/job-catalog/profiles/options`（新增；用于 Position/其他页面选择模板）
- 删除：
  - `POST/PATCH/GET /org/job-catalog/roles*`（含 options）

#### 5.3.2 页签与字段（中文建议）
- 页签（Tabs）：
  - `family-groups`：职类
  - `families`：职种
  - `profiles`：职位模板（新增页签，主维护入口）
  - `levels`：职级
  - `roles`：移除
- 字段中文建议（列表/表单）：
  - 职类：编码、名称、状态（启用/停用）
  - 职种：上级职类、编码、名称、状态（启用/停用）
  - 职位模板：
    - 基本信息：编码、名称、说明（或“职责说明”）、状态（启用/停用）
    - 归属与比例：主归属职种、职种归属比例（合计 100%）
  - 职级：编码、名称、排序、状态（启用/停用）

#### 5.3.3 表单字段（HTMX Form Data 规格）
- 职类：
  - `code`（编辑时只读）、`name`、`is_active`
- 职种：
  - `code`（编辑时只读）、`name`、`is_active`
  - 归属职类来自过滤器选择：`job_family_group_code`
- 职位模板：
  - 基本信息：`code`（编辑时只读）、`name`、`description`、`is_active`
  - 归属与比例（可重复多行）：
    - `job_family_id`（可重复，表示归属职种列表）
    - `allocation_percent`（可重复，与 `job_family_id` 一一对应）
    - `primary_job_family_id`（单值；必须等于某个 `job_family_id`）
#### 5.3.4 HTMX 错误响应约定
- `422`：返回带错误信息的表单片段（不做 JS 弹窗为主，遵循项目 UI 反馈口径）。

### 5.4 Position 写入口（Managed Position）
> 本计划采取“破坏性更正（最彻底）”口径：以本文作为唯一契约入口，并在实施前对 `DEV-PLAN-052` 追加“被 072 覆盖的合同变更”章节，明确删除 `job_*_code` 写入口、`job_profile_id` 必填等变化；不保留 v1/v2 并行。

- `POST /org/api/positions` / `PATCH /org/api/positions/{id}`（请求体变更点）：
  - 必填：`job_profile_id`
  - 可选：`job_level_code`
  - 可选：`job_families`（归属与比例；缺省从职位模板复制，可在职位上覆盖）
  - 移除写入：`job_family_group_code/job_family_code/job_role_code`
  - 示例：
    ```json
    {
      "code": "POS-0001",
      "org_node_id": "uuid",
      "effective_date": "2025-01-01",
      "lifecycle_status": "active",
      "position_type": "managed",
      "employment_type": "regular",
      "job_profile_id": "uuid",
      "job_level_code": "L3",
      "job_families": [
        { "job_family_id": "uuid", "allocation_percent": 60, "is_primary": true },
        { "job_family_id": "uuid", "allocation_percent": 40, "is_primary": false }
      ],
      "reason_code": "CORRECT"
    }
    ```
- 读响应（兼容建议）：
  - `job_family_group_code/job_family_code` 作为**派生只读字段**保留（由 6.3 查询派生）。
  - `job_role_code` 彻底退场（建议从响应移除；若需兼容则固定为 `null` 并在 v2 移除）。
- 错误码（沿用并扩展）：
  - `422 ORG_JOB_PROFILE_NOT_FOUND` / `ORG_JOB_PROFILE_INACTIVE`
  - `422 ORG_JOB_LEVEL_NOT_FOUND` / `ORG_JOB_LEVEL_INACTIVE`（新增）
  - `422 ORG_POSITION_JOB_FAMILIES_INVALID`（新增：比例合计≠100 或主归属不唯一）
  - `422 ORG_JOB_FAMILY_NOT_FOUND` / `ORG_JOB_FAMILY_INACTIVE`（归属职种不存在/停用）

### 5.5 错误码与 DB 约束映射（实现必须对齐）
> 目的：避免 DB 约束失败时泄漏为 `ORG_INTERNAL`，保持错误码稳定可测试。

- `org_job_profile_job_families_validate_trigger`（deferred trigger，`23514 check_violation`）：
  - 映射为 `422 ORG_JOB_PROFILE_JOB_FAMILIES_INVALID`
- `org_job_profile_job_families_primary_unique`（unique index，`23505 unique_violation`）：
  - 映射为 `422 ORG_JOB_PROFILE_JOB_FAMILIES_INVALID`
- `org_job_profile_job_families_family_fk`（`23503 foreign_key_violation`）：
  - 映射为 `422 ORG_JOB_FAMILY_NOT_FOUND`（或复用 `ORG_JOB_CATALOG_PARENT_NOT_FOUND`，实现阶段需选定其一并保持一致）
- `org_position_slice_job_families_validate_trigger`（deferred trigger，`23514 check_violation`）：
  - 映射为 `422 ORG_POSITION_JOB_FAMILIES_INVALID`
- `org_position_slice_job_families_primary_unique`（unique index，`23505 unique_violation`）：
  - 映射为 `422 ORG_POSITION_JOB_FAMILIES_INVALID`
- `org_position_slice_job_families_family_fk`（`23503 foreign_key_violation`）：
  - 映射为 `422 ORG_JOB_FAMILY_NOT_FOUND`（或复用 `ORG_JOB_CATALOG_PARENT_NOT_FOUND`，实现阶段需选定其一并保持一致）
- `org_job_levels_tenant_id_code_key`（unique，`23505 unique_violation`）：
  - 映射为 `409 ORG_JOB_CATALOG_CODE_CONFLICT`
- `org_position_slices_job_profile_fk`（`23503 foreign_key_violation`）：
  - 映射为 `422 ORG_JOB_PROFILE_NOT_FOUND`

实现提示（避免猜）：需在 `modules/org/services/pg_errors.go` 增补上述约束名映射，并补充对应测试用例（单测或集成测均可，按触发器矩阵执行）。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 职位模板归属与比例校验（服务端）
1. 校验 `job_families` 非空。
2. 校验每行：
   - `job_family_id` 非空且存在且启用；
   - `allocation_percent` ∈ [1,100]。
3. 校验不变量：
   - `sum(allocation_percent)=100`
   - `count(is_primary=true)=1`
4. 在同一事务内写入：
   - Upsert `org_job_profiles`
   - Replace `org_job_profile_job_families`（delete+insert，依赖 deferred trigger 在提交时最终校验）

### 6.2 Position 写入校验（Managed Position）
1. 开启事务并执行 Position time-slice 写入的既有有效期算法（遵循 `DEV-PLAN-064` 与现有 gap-free 触发器口径）。
2. 校验 `job_profile_id`：
   - 存在且 `is_active=true`
3. 处理 `job_families`（职位归属与比例）：
   - 若请求体提供 `job_families`：按 6.1 同样规则校验（sum=100、primary 唯一、职种存在且启用）。
   - 若请求体未提供：默认复制：
     - Create：从 `org_job_profile_job_families` 复制；
     - Update/Correct：默认复制“目标 as-of 覆盖的上一切片”；若本次变更同时修改了 `job_profile_id`，则改为从新模板复制。
   - 写入落点：`org_position_slice_job_families`（delete+insert；依赖 deferred trigger 在提交时最终校验）。
4. 若填写 `job_level_code`：
   - 解析为 `org_job_levels`（tenant-global）记录并校验启用；
5. 写入 Position slice：写入 `job_profile_id`、`job_level_code`；职类/职种从 `org_position_slice_job_families`（主归属）派生。

### 6.3 读取派生（用于 Position 列表/详情展示）
> 读模型派生“主归属职种/职类”建议使用主归属行 `is_primary=true`：

```sql
	SELECT
	  s.*,
	  jf.code AS job_family_code,
	  jfg.code AS job_family_group_code
	FROM org_position_slices s
	JOIN org_position_slice_job_families psjf
	  ON psjf.tenant_id = s.tenant_id
	 AND psjf.position_slice_id = s.id
	 AND psjf.is_primary = TRUE
	JOIN org_job_families jf
	  ON jf.tenant_id = s.tenant_id
	 AND jf.id = psjf.job_family_id
	JOIN org_job_family_groups jfg
	  ON jfg.tenant_id = s.tenant_id
	 AND jfg.id = jf.job_family_group_id
	WHERE s.tenant_id = $1;
```

## 7. 安全与鉴权 (Security & Authz)
- **Authz Object**（现有对象名，见 `modules/org/presentation/controllers/authz_helpers.go`）：
  - Job Catalog UI：`org.job_catalog`
  - Job Profiles API：`org.job_profiles`
  - Positions API：`org.positions`
- **策略原则**：
  - 维护类写操作（create/update）要求对应对象的 `admin`（或既有写动作）权限。
  - 所有 DB 查询必须带 `tenant_id` 条件（租户隔离为硬约束）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- Org Atlas+Goose 工具链与门禁：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- 路由治理：`docs/dev-plans/018-routing-strategy.md`
- Valid Time=DATE：`docs/dev-plans/064-effective-date-day-granularity.md`

### 8.2 里程碑（建议拆分 PR）
0. [x] 契约收口（文档）：对 `DEV-PLAN-052` 追加“被 072 覆盖的合同变更”章节；并在 `DEV-PLAN-056/067` 标注旧口径退场点，避免多 SSOT 漂移。
1. [ ] Schema/迁移：新增 `org_job_profile_job_families`、去 Role、职级去 Role、Position slice 收口（含数据回填与冲突重命名）。
2. [ ] Service/Repo：按 6 章实现校验与读派生查询；移除 `Job Role` 相关 service/repo/controller。
3. [ ] API/UI：Job Catalog 页签调整（roles 删除、profiles 新增）；i18n 统一“职位模板/职类/职种/职级”。
4. [ ] 测试与 Readiness：补齐单测/集测，并记录门禁执行证据（按 `DEV-PLAN-000` 规范）。

## 9. 测试与验收标准 (Acceptance Criteria)
### 9.1 最小验收用例（必须覆盖）
- [ ] 创建职位模板，配置两条归属（比例合计=100 且主归属唯一）；否则保存失败并返回稳定错误码。
- [ ] 停用职位模板后，无法创建/更新 Managed Position 引用它（`ORG_JOB_PROFILE_INACTIVE`）。
- [ ] Position 创建/更新必须提供 `job_profile_id`；不再允许通过 `job_*_code` 写入分类路径。
- [ ] Position 创建时若未显式提交 `job_families`，则默认从职位模板复制；若提交则允许覆盖，但必须满足比例合计=100 且主归属唯一（否则 `ORG_POSITION_JOB_FAMILIES_INVALID`）。
- [ ] Position 填写不存在/停用的 `job_level_code` 时被拒绝（`ORG_JOB_LEVEL_NOT_FOUND` / `ORG_JOB_LEVEL_INACTIVE`）。

### 9.2 自动化测试建议
- Repository 层：归属表 CRUD、deferred trigger 行为（事务提交时校验）。
- Service 层：归属比例校验、主归属唯一、停用引用拦截。
- 集成测试：迁移后 smoke（含旧数据回填与职级 code 冲突重命名）。

## 10. 运维与监控 (Ops & Monitoring)
- 本项目早期不引入新的 Feature Flag；本计划按“破坏性更正”直接收口。
- 仅要求关键写路径的结构化日志包含：`request_id/tenant_id/entity/id/change_type`，避免过度监控建设（对齐 `AGENTS.md` 运维约束）。

## 11. DEV-PLAN-045 评审记录（Simple > Easy Review）
**复审时间**: 2025-12-29 18:10 UTC  
**结论**: 通过（2 个警告，已在本文中显式化为实施必做项）。

### 结构（解耦/边界）
- [x] 去除 `Job Role`，避免“同一概念两套表达”导致纠缠
- [x] Position 写入口冻结为 `job_profile_id`（不再接受 `job_*_code`），并通过“模板默认 + 职位可覆盖”把归属比例收敛到单一不变量

### 演化（规格/确定性）
- [x] 明确不变量：归属比例合计=100%、主归属唯一、模板启停/引用规则
- [x] 给出 schema/迁移/接口/算法的可执行规格（对齐 `DEV-PLAN-001`）
- [!] 警告：涉及 052/056/067 的契约漂移风险，必须按 8.2 的“契约收口（文档）”先行处理，避免实现阶段出现双 SSOT

### 认知（本质/偶然复杂度）
- [x] 将“Catalog 四级路径（含 Role）”识别为历史偶然复杂度，并给出退场路径

### 维护（可理解/可解释）
- [x] 主流程 + 失败路径 + 读派生查询可被 reviewer 在 5 分钟内复述
- [!] 警告：DB 约束失败的错误码稳定性必须通过 5.5 的映射与测试保证，否则会退化为 `ORG_INTERNAL`
