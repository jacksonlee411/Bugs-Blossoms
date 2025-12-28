# DEV-PLAN-064A：`effective_on/end_on` 双轨列引入复盘：收益、复杂度与收敛方案

**状态**: 规划中（2025-12-28 00:00 UTC）

> 本文定位：作为 DEV-PLAN-064 的补充调查（A），聚焦“为什么引入 `effective_on/end_on`（date）并保留 legacy `effective_date/end_date`（timestamptz）”、“它解决了什么问题”、“它带来的架构偏移/复杂度是什么”，以及“如何尽快收敛回单一权威表达（SSOT）”。

## 1. 背景与问题陈述 (Context)
DEV-PLAN-064 的目标是将 Valid Time（业务有效期）统一收敛到 **day（date）粒度**，并明确时间戳仅用于 Audit/Tx Time。

在 Org Phase 1 的落地中，为了降低迁移风险、保留回滚与可观测性，采用了 **B1 双轨策略**：在多张 Org 时态表中新增 `effective_on/end_on`（date），同时保留 legacy `effective_date/end_date`（timestamptz）。

随着更多功能开发（例如 066：删除自动缝补），双轨列的存在开始表现为：
- 概念层面：同一“有效期窗口”出现两套字段表达，容易形成“隐式 SSOT 分裂”。
- 实现层面：写路径/读路径/约束可能混用两套字段，引入额外复杂度与误用风险。

本文需要回答四个问题：
1) `effective_on/end_on` 是因为什么原因引入的？（决策依据）
2) 它解决了哪些真实问题？（收益）
3) 它是否造成架构偏移与额外复杂度？风险具体在哪里？（代价）
4) 推荐的收敛方案是什么？（退出双轨、回到单一 SSOT）

## 2. 事实与证据 (Facts & Evidence)
### 2.1 直接证据：迁移脚本标注来源
- `migrations/org/20251227090000_org_valid_time_day_granularity.sql` 文件头明确标注：`-- DEV-PLAN-064: Introduce day-granularity (date) columns for Valid Time.`，并在 Org 多张表上新增 `effective_on/end_on`（date）与 backfill/约束。

### 2.2 设计证据：064 的 B1 决策与退出计划
- `docs/dev-plans/064-effective-date-day-granularity.md` 明确选定 **B1（新增 date 列 + 双写/回填）** 作为默认迁移策略，并在后续阶段（D/E）规划“清理 legacy timestamp 列/旧约束 + 收紧输入”，最终收敛为 date-only（避免长期双 SSOT）。

### 2.3 实现证据：当前代码仍处于混用/双写期
- Org 写路径普遍仍写入 `effective_date/end_date`（timestamptz），并派生写入 `effective_on/end_on`（date），例如 `modules/org/infrastructure/persistence/valid_time_day.go`。
- 读路径中同时存在 timestamp 口径与 day 口径的查询/关联（不同位置使用不同字段），显示系统处于“过渡期”。

### 2.4 Org Phase 1：in-scope 表清单（B1 双轨落地点）
> 目的：避免“in-scope 表”口径漂移，便于后续阶段 D（删旧/改名）与影响面评估。

证据来源：`migrations/org/20251227090000_org_valid_time_day_granularity.sql`（以及对应的 schema SSOT）。

- 引入 `effective_on/end_on`（date）双列：
  - `org_node_slices`
  - `org_edges`
  - `org_hierarchy_closure`
  - `org_positions`
  - `org_position_slices`
  - `org_assignments`
  - `org_attribute_inheritance_rules`
  - `org_role_assignments`
  - `org_security_group_mappings`
  - `org_links`
  - `org_audit_logs`
- 仅引入 `effective_on`（date）单列：
  - `org_personnel_events`

## 3. 为什么引入 `effective_on/end_on`？（决策原因）
结论：`effective_on/end_on` 的引入是 DEV-PLAN-064 迁移策略 B1 的必然产物，其核心动机不是“多一套字段更好”，而是为了在不破坏既有行为的前提下**可观测、可回滚、可逐步切换**。

主要原因：
1) **迁移风险控制（避免 B2 大改列类型）**
   - 直接把 `effective_date/end_date` 从 `timestamptz` 改为 `date`（B2）是高风险、强破坏性操作：涉及大量 SQL/索引/约束/比较语义切换，出错面大，回滚成本高。
   - B1 允许“先引入新列 + 回填 + 并行约束/索引 + 双写”，让变更可以分阶段验证。

2) **语义映射的桥接层：闭区间（业务日）→ 半开 range（约束不重叠）**
   - 业务语义（HR）需要 day 粒度闭区间：`[effective_date, end_date]`（结束日当天仍有效；Phase 1 为避免与 legacy `effective_date/end_date`（timestamptz）同名混用，暂用 `effective_on/end_on` 承载 day 口径，阶段 D 后回到 `effective_date/end_date`（date））。
   - DB 的强约束依然希望使用半开区间避免边界双命中：`daterange(effective_date, end_date + 1, '[)')`（Phase 1 对应为 `daterange(effective_on, end_on + 1, '[)')`）。
   - 新列提供了一个“明确的 day 粒度 SSOT 承载点”，避免继续用 `timestamptz` 在各处做隐式截断/减微秒。

3) **兼容期并行验证（防漂移）**
   - 保留 legacy 列意味着可以在过渡期用测试与对账逻辑验证：timestamp 口径与 date 口径在关键路径上是否一致（除“结束日包含”语义变化外）。
   - 这为最终删列/切换提供证据与信心。

## 4. 它解决了什么问题？（收益）
1) **把 Valid Time 从“连续时间”拉回“业务日（civil date）”**
   - 减少 off-by-one：结束日期的“含/不含”不再由 `23:59:59.999999` 这类隐式技巧决定。
   - 对齐 SAP HCM / PeopleSoft 的 effective dating 心智模型。

2) **让“不重叠强约束”保留在 Postgres 层**
   - 通过 `daterange(effective_date, end_date + 1, '[)')` 继续使用 `EXCLUDE USING gist`，保持强约束能力（同键区间不重叠）。

3) **为渐进式迁移提供落脚点**
   - 允许先在 schema 层落地新约束/新索引，再逐步切换读写与输入输出契约，而不是一次性“全系统硬切”。

## 5. 是否造成架构偏移与额外复杂度？（代价与风险）
结论：**会**。双轨列是“为了迁移安全而引入的暂时复杂度”，但如果缺少清晰的权威表达与退场计划，会演化为长期架构偏移。

### 5.1 复杂度类型
1) **认知复杂度（概念重复）**
   - 同一业务概念“有效期窗口”出现两套字段名，容易导致新代码随意选用，形成隐式分裂 SSOT。

2) **实现复杂度（读写混用）**
   - 写入：需要双写或派生写，必须保证一致性。
   - 读取：容易出现“某些查询用 timestamp、某些查询用 date”，导致边界语义不一致或难以推理。

3) **数据复杂度（持久化负担）**
   - 多一套列/索引/约束会带来额外存储与迁移成本；更重要的是增加“违反一致性”的可能性。

4) **架构偏移风险（边界泄漏）**
   - 如果 Service/Domain 层不得不理解两套字段与映射规则，就会把迁移期偶然复杂度泄漏进业务逻辑，违背 DEV-PLAN-064 的“Valid Time=date 是唯一权威表达”方向。

### 5.2 最危险的失败模式（必须避免）
- **写入漂移**：某条路径只更新了 date 口径列（`effective_on/end_on`）或只更新了 legacy timestamp 列（`timestamptz effective_date/end_date`），导致同一记录出现互相矛盾的窗口。
- **查询漂移**：同一业务语义在不同查询里使用不同字段，导致“某些页面/接口显示的有效期与另一些不一致”。
- **无退出计划**：双轨列长期存在并持续扩散，最终变成“永久复杂度”。

### 5.3 过渡期停止线（Review Stop Lines）
> 命中任意一条，优先打回（除非变更属于 064 阶段 D/E 且在 064 主计划中已明确范围与回滚策略）。

- 在 Domain/Service/Presentation 层引入或新增使用 `timestamptz` 作为 Valid Time 的输入/输出/判断依据（回流 timestamp 口径）。
- 新增/修改查询继续以 legacy `timestamptz effective_date/end_date` 作为 as-of/overlap 的主要判断字段，但未与阶段 D 收敛同步交付（把偶然复杂度固化）。
- 阶段 D 合并后，任何层仍出现 `effective_on` / `end_on`（运行时代码、SQL、schema SSOT），直接打回（历史迁移文件除外）。
- 任一写路径出现“只写一套列”的情况，或绕过集中 helper 自己派生 `effective_on/end_on`（导致一致性不可控）。
- 引入第三套 Valid Time 表达（新字段名/新 DTO/新范围语义），而不是收敛到既有单一权威表达。
- 在 DB EXCLUDE 约束中不使用 `daterange(effective_date, end_date + 1, '[)')` 的半开映射来表达 day 闭区间（或仍依赖 `tstzrange`/“减微秒”技巧），导致边界语义漂移。

## 6. 解决方案：如何收敛并避免长期复杂度？（推荐）
### 6.1 指导原则（Simple > Easy）
- 双轨列只允许作为“迁移期偶然复杂度”，必须有明确的**退场路径**与**禁止扩散**的规则。
- 在过渡期必须明确单一 SSOT：实现必须能回答“到底哪一套字段是权威表达”。

#### 6.1.1 过渡期 SSOT 与边界（必须执行）
- **Valid Time 的权威表达（SSOT）**：最终在 DB 表中仅保留 `effective_date/end_date`（`date`，day 粒度闭区间）作为 Valid Time 的唯一权威表达；`effective_on/end_on` 只是 B1 过渡列，必须在阶段 D 通过“删旧 + 改名”收敛，使 schema 中不再出现 `effective_on/end_on`。
- **legacy timestamp 列的定位**：阶段 D 之前遗留的 `effective_date/end_date`（`timestamptz`）仅用于兼容与回滚锚点；阶段 D 将其删除，并由 `date` 口径接管同名字段（避免长期双 SSOT）。
- **边界隔离**：迁移期的“双轨/派生/归一化”应被隔离在 Infrastructure/persistence 内；Domain/Service/Presentation 不应理解两套字段与映射规则。
- **集中派生点**：双写/派生应统一经过 `modules/org/infrastructure/persistence/valid_time_day.go` 的 helper（避免到处散落“减微秒/截断为 date”的隐式技巧）。
- **新增/修改查询规则**：凡涉及 as-of、overlap、相邻缝补等 Valid Time 语义，必须以 **day 口径（date）** 实现并以阶段 D 后的最终字段为准（`effective_date/end_date`）。在阶段 D 合并前，禁止新增任何 `effective_on/end_on` 引用；如必须改动 Valid Time 逻辑，需与阶段 D 同步交付并在合并后完成“无 `effective_on/end_on` 残留”的验收。

#### 6.1.2 映射规则与不变量（可对账）
> 这些规则是 B1 双轨期间“保持一致性”的最小共识；阶段 D 收敛后应只保留 date 口径。

- **窗口语义（最终 DB 字段）**：date 口径为闭区间 `[effective_date, end_date]`；用于 no-overlap 约束时，映射为半开区间 `daterange(effective_date, end_date + 1, '[)')`。
- **派生（timestamp → date）**：
  - 阶段 D 前（仍存在 legacy `timestamptz effective_date/end_date` 时）：`effective_on = (effective_date AT TIME ZONE 'UTC')::date`
  - 阶段 D 前（仍存在 legacy `timestamptz effective_date/end_date` 时）：`end_on = ((end_date AT TIME ZONE 'UTC') - 1 microsecond)::date`（但当 `end_date` 为 open-ended sentinel 时，`end_on` 保持为 `9999-12-31`）
- **不变量**：
  - 阶段 D 前：`effective_on <= end_on`；相邻片段允许：`prev.end_on + 1 day == next.effective_on`。
  - 阶段 D 后：`effective_date <= end_date`；相邻片段允许：`prev.end_date + 1 day == next.effective_date`。
  - 不要用“减微秒/23:59:59.999999”在业务层表达边界语义。

### 6.2 方案 A（推荐）：完成 064 阶段 D/E，收敛到 date-only 单列表达
目标：最终在 in-scope 表中只保留一套 Valid Time 字段，并使其类型/命名与语义一致（`effective_date/end_date` = `date`）。

建议步骤（与 064 的 D/E 对齐，细节以 064 为 SSOT）：
1) **过渡期防扩散（立即生效的规则）**
   - 新增/修改查询优先使用 day 口径（date 列）表达 Valid Time。
   - 写入路径只允许采用一种权威写法（在 dual-write 阶段统一通过集中 helper 派生写另一套列），避免出现第三套写法。

2) **收紧输入（对应 064 阶段 E）**
   - 停止接受 RFC3339 timestamp 作为 Valid Time 输入（只接受 `YYYY-MM-DD`），从源头消除时刻污染与边界漂移。

3) **删旧与改名（对应 064 阶段 D）**
   - 删除 legacy `timestamptz effective_date/end_date` 与 `tstzrange` 相关约束/索引。
   - 将 `effective_on -> effective_date`、`end_on -> end_date`（类型为 `date`），实现“字段名=语义=类型”一致，彻底消除双轨。
   - 同步更新 Repository/Service/测试与文档引用，确保仓库内只有一种权威表达。

验收标准（最小集合）：
- [ ] schema 中不再存在 `effective_on/end_on` 与 `timestamptz effective_date/end_date` 的并存；Valid Time 仅以 `date effective_date/end_date` 表达。
- [ ] schema 中不再出现任何 `effective_on/end_on` 列与其衍生约束/索引命名（例如 `*_no_overlap_on` / `*_effective_on_check`）。
- [ ] 所有 as-of 判断与 no-overlap 约束均以 date 口径实现（`daterange(effective_date, end_date + 1, '[)')`）。
- [ ] 代码库不再出现“Valid Time 用 timestamp 比较”的新路径（避免回流）。

### 6.3 方案 B（不推荐，仅作为备选）：长期保留双轨并强制一致性
如果因外部依赖不得不长期保留两套列，必须额外引入强一致性机制（例如 DB CHECK/触发器/或统一写入视图）来防漂移。

不推荐理由：
- 这会把“迁移期偶然复杂度”固化为长期复杂度，与 DEV-PLAN-045 的 Simple 原则冲突。
- 触发器/隐式同步会增加调试成本与意外副作用，且与现有“Service-first 写逻辑”习惯不一致。

## 7. 详细设计（TDD，按 DEV-PLAN-001）
> 本节把 064A 的“复盘结论”细化为可直接编码/迁移的实施蓝图；关键决策以 `docs/dev-plans/064-effective-date-day-granularity.md` 为 SSOT。

### 7.1 目标与非目标 (Goals & Non-Goals)
**核心目标**
- [ ] **DB（Org in-scope）**：所有 Valid Time 列最终仅使用 `effective_date` / `end_date`（`date`，day 粒度闭区间；事件表可仅 `effective_date`），不得再出现 `effective_on/end_on`（列名/约束名/索引名均不允许）。
- [ ] **DB（清理 legacy）**：删除 legacy `timestamptz effective_date/end_date`，并移除所有基于 `tstzrange(effective_date, end_date, '[)')` 的约束与索引。
- [ ] **代码（统一口径）**：Org 代码库内不再存在以 timestamp 口径实现 Valid Time 的比较（例如 `end_date > $asOf` / `- 1 microsecond`）；所有 as-of/overlap/相邻判断统一以 date 口径实现。
- [ ] **阶段 E（收紧输入）**：对外契约停止接受 RFC3339 timestamp 作为 Valid Time 输入，仅允许 `YYYY-MM-DD`（失败返回 422；与 064 对齐）。
- [ ] **门禁通过**：按 `AGENTS.md` 触发器矩阵与 `Makefile` 执行并通过（本节只声明触发器，不复制脚本细节）。

**非目标（Out of Scope）**
- 不改变 Audit/Tx Time 字段（`created_at/updated_at/transaction_time` 等仍为 `timestamptz`）。
- 不引入 DB 触发器/隐式同步机制/视图写入（避免把迁移期偶然复杂度固化为长期复杂度）。
- 不扩展 Org 之外模块的 schema 收敛（本计划仅覆盖 2.4 清单）。

### 7.2 工具链与门禁（SSOT 引用）
> 目的：声明本计划命中的触发器；命令细节以 SSOT 为准，避免 drift。

- [ ] Go 代码触发器：见 `AGENTS.md`、`Makefile`。
- [ ] Org DB 迁移 / Schema 触发器：见 `AGENTS.md` 与 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`。
- [ ] 文档门禁：`make check doc`（仅在文档变更时；SSOT 见 `AGENTS.md`）。
- SSOT 链接：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

### 7.3 架构与关键决策 (Architecture & Decisions)
- **单一权威表达（DB 层）**：Valid Time 的字段名与类型在 DB 中必须一致：`effective_date/end_date` 均为 `date`（或事件表仅 `effective_date`），不再存在 `effective_on/end_on`。
- **语义（闭区间 + range 映射）**：业务语义为闭区间 `[effective_date, end_date]`；DB EXCLUDE 约束使用 `daterange(effective_date, end_date + 1, '[)')` 的半开映射（与 064 一致）。
- **边界隔离**：迁移期的派生/归一化逻辑只允许出现在 `modules/org/infrastructure/persistence/` 内；Domain/Service/Presentation 仅理解 day 粒度 Valid Time（`YYYY-MM-DD`）。
- **不引入新概念**：阶段 D/E 只做收敛（删旧 + 改名 + 收紧输入），不再发明第三套字段/DTO/范围语义（命中 5.3 停止线直接打回）。

### 7.4 数据模型与约束（最终形态，Schema SSOT）
> 最终 schema 以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为 SSOT；本节描述必须实现的“最终形态”。

#### 7.4.1 时态窗口表（`effective_date/end_date`）
适用表：2.4 中所有“窗口表”（引入 `effective_on/end_on` 的表）。

- 字段：
  - `effective_date date NOT NULL`
  - `end_date date NOT NULL DEFAULT DATE '9999-12-31'`
- 约束：
  - `CHECK (effective_date <= end_date)`
  - 时间不重叠：按各表既有 key 列保持不变，统一使用 `EXCLUDE USING gist (..., daterange(effective_date, end_date + 1, '[)') WITH &&)`。
- 索引：
  - 保留/重建“按 key + effective_date”查询的 btree 索引（例如 `(tenant_id, <key>, effective_date)`）；索引名中不得出现 `_effective_on_`。

#### 7.4.2 事件表（`org_personnel_events`）
适用表：2.4 中“仅引入 `effective_on` 的表”。

- 字段：
  - `effective_date date NOT NULL`
- 索引：
  - `(tenant_id, person_uuid, effective_date DESC)`（索引名不得出现 `_effective_on_`）。

### 7.5 迁移策略（阶段 D：删旧 + 改名，Goose）
> 目标：一次迁移完成“删旧列（timestamp）+ 列改名（on→date）+ 约束/索引去 `_on` 命名”，并确保迁移失败时可快速定位原因。

#### 7.5.1 迁移文件与交付物
- 新增 Org Goose 迁移（文件名按时间戳生成，示例：`migrations/org/2025xxxxxx_org_valid_time_converge_date_only.sql`）。
- 同步更新 schema SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`（移除 `effective_on/end_on` 与 legacy timestamp 列/约束/索引）。

#### 7.5.2 Up：执行顺序（必须保持）
0) **迁移前置校验（Fail-fast）**
   - 对 2.4 的“窗口表”（存在 `effective_on/end_on`）执行一致性检查：确认其与 legacy `timestamptz effective_date/end_date` 的 UTC 归一化结果一致（不一致则 `RAISE EXCEPTION` 阻止进入破坏性删列）。
   - 对 `org_personnel_events`（仅有 `effective_on`）执行一致性检查：确认 `effective_on` 与 legacy `timestamptz effective_date` 的 UTC 归一化结果一致（该表无 `end_on/end_date`）。
   - 校验公式（与 064 的 4.4 归一化规则一致）：
     - 窗口表：`effective_on = (effective_date AT TIME ZONE 'UTC')::date`
     - 窗口表：`end_on = CASE WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31' ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date END`
     - `org_personnel_events`：仅校验 `effective_on = (effective_date AT TIME ZONE 'UTC')::date`
   - 参考实现（按表复制一份；命中即失败）：
     ```sql
     DO $$
     BEGIN
       IF EXISTS (
         SELECT 1
         FROM org_node_slices
         WHERE effective_on IS DISTINCT FROM (effective_date AT TIME ZONE 'UTC')::date
            OR end_on IS DISTINCT FROM CASE
                 WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
                 ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
               END
       ) THEN
         RAISE EXCEPTION 'valid_time_day preflight failed: org_node_slices has drift between timestamp/date columns';
       END IF;
     END $$;
     ```
   - `org_personnel_events` 参考实现（命中即失败）：
     ```sql
     DO $$
     BEGIN
       IF EXISTS (
         SELECT 1
         FROM org_personnel_events
         WHERE effective_on IS DISTINCT FROM (effective_date AT TIME ZONE 'UTC')::date
       ) THEN
         RAISE EXCEPTION 'valid_time_day preflight failed: org_personnel_events has drift between timestamp/date columns';
       END IF;
     END $$;
     ```
   - 若 preflight 失败：先修复漂移，再进入阶段 D（破坏性删列）。修复策略应优先选择“以 timestamp 列回填 date 列”（复用 Phase 1 的回填逻辑），并定位是哪条写路径绕过了集中派生点导致不一致。

1) **删除 legacy timestamp 约束/索引（依赖 effective_date/end_date 的对象）**
   - 删除所有 `tstzrange(effective_date, end_date, '[)')` 相关 EXCLUDE/索引。
   - 删除所有基于 timestamp `effective_date` 的 btree 索引（例如 `*_effective_idx`）。
   - 删除 timestamp 版本的 `*_effective_check`（通常为 `effective_date < end_date`）。

2) **删旧列（timestamp）并改名（date）**
   - 对窗口表：`DROP COLUMN effective_date; DROP COLUMN end_date; RENAME COLUMN effective_on TO effective_date; RENAME COLUMN end_on TO end_date`。
   - 对 `org_personnel_events`：`DROP COLUMN effective_date; RENAME COLUMN effective_on TO effective_date`。

3) **约束/索引收敛命名（去 `_on`）**
   - 约束：将所有 `*_effective_on_check` / `*_no_overlap_on` / `*_unique_on` 重命名为去 `_on` 的版本。
   - 索引：将所有 `*_effective_on_idx` 重命名为去 `_on` 的版本。
   - 约束与索引的 key 列、operator class、range 表达式均保持与现有 “*_on” 版本一致，仅变更列名与对象名。

##### 7.5.2.A 对象映射（按表，必须逐项处理）
> 目的：把“改哪些对象”写死，避免实现阶段靠搜索/猜测。对象名以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为 SSOT；如发现 drift，先修 SSOT 再改迁移。

- `org_node_slices`
  - Drop（timestamp）：`org_node_slices_effective_check` / `org_node_slices_no_overlap` / `org_node_slices_sibling_name_unique` / `org_node_slices_tenant_node_effective_idx` / `org_node_slices_tenant_parent_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_node_slices_effective_on_check→org_node_slices_effective_check`、`org_node_slices_tenant_node_no_overlap_on→org_node_slices_no_overlap`、`org_node_slices_sibling_name_unique_on→org_node_slices_sibling_name_unique`；索引 `org_node_slices_tenant_node_effective_on_idx→org_node_slices_tenant_node_effective_idx`、`org_node_slices_tenant_parent_effective_on_idx→org_node_slices_tenant_parent_effective_idx`
- `org_edges`
  - Drop（timestamp）：`org_edges_effective_check` / `org_edges_single_parent_no_overlap` / `org_edges_tenant_parent_effective_idx` / `org_edges_tenant_child_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_edges_effective_on_check→org_edges_effective_check`、`org_edges_tenant_child_no_overlap_on→org_edges_single_parent_no_overlap`；索引 `org_edges_tenant_child_effective_on_idx→org_edges_tenant_child_effective_idx`、`org_edges_tenant_parent_effective_on_idx→org_edges_tenant_parent_effective_idx`
- `org_hierarchy_closure`
  - Drop（timestamp）：`org_hierarchy_closure_effective_check` / `org_hierarchy_closure_pair_window_no_overlap` / `org_hierarchy_closure_ancestor_range_gist_idx` / `org_hierarchy_closure_descendant_range_gist_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_hierarchy_closure_effective_on_check→org_hierarchy_closure_effective_check`、`org_hierarchy_closure_tenant_no_overlap_on→org_hierarchy_closure_pair_window_no_overlap`
  - Recreate（date range gist）：用 `daterange(effective_date, end_date + 1, '[)')` 重建 `org_hierarchy_closure_ancestor_range_gist_idx` / `org_hierarchy_closure_descendant_range_gist_idx`（保持索引名不变，仅替换 range 表达式）。
- `org_positions`
  - Drop（timestamp）：`org_positions_effective_check` / `org_positions_code_unique_in_time` / `org_positions_tenant_node_effective_idx` / `org_positions_tenant_code_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_positions_effective_on_check→org_positions_effective_check`、`org_positions_tenant_code_no_overlap_on→org_positions_code_unique_in_time`；索引 `org_positions_tenant_node_effective_on_idx→org_positions_tenant_node_effective_idx`、`org_positions_tenant_code_effective_on_idx→org_positions_tenant_code_effective_idx`
- `org_position_slices`
  - Drop（timestamp）：`org_position_slices_effective_check` / `org_position_slices_no_overlap` / `org_position_slices_tenant_position_effective_idx` / `org_position_slices_tenant_node_effective_idx` / `org_position_slices_tenant_reports_to_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_position_slices_effective_on_check→org_position_slices_effective_check`、`org_position_slices_tenant_position_no_overlap_on→org_position_slices_no_overlap`；索引 `org_position_slices_tenant_position_effective_on_idx→org_position_slices_tenant_position_effective_idx`
  - Recreate（btree）：用 date 列重建 `org_position_slices_tenant_node_effective_idx`、`org_position_slices_tenant_reports_to_effective_idx`（保持索引名与列序不变，仅列类型变化）。
- `org_assignments`
  - Drop（timestamp）：`org_assignments_effective_check` / `org_assignments_primary_unique_in_time` / `org_assignments_subject_position_unique_in_time` / `org_assignments_tenant_subject_effective_idx` / `org_assignments_tenant_position_effective_idx` / `org_assignments_tenant_pernr_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_assignments_effective_on_check→org_assignments_effective_check`、`org_assignments_tenant_subject_no_overlap_on→org_assignments_tenant_subject_no_overlap`、`org_assignments_tenant_position_subject_no_overlap_on→org_assignments_tenant_position_subject_no_overlap`；索引 `org_assignments_tenant_subject_effective_on_idx→org_assignments_tenant_subject_effective_idx`、`org_assignments_tenant_position_effective_on_idx→org_assignments_tenant_position_effective_idx`、`org_assignments_tenant_pernr_effective_on_idx→org_assignments_tenant_pernr_effective_idx`
  - 备注：阶段 D 不再保留 timestamp 版的 partial EXCLUDE；以现有 date 版 EXCLUDE（按 `assignment_type` 作为 key）为准，保持与阶段 C 一致。
- `org_attribute_inheritance_rules`
  - Drop（timestamp）：`org_attribute_inheritance_rules_effective_check` / `org_attribute_inheritance_rules_no_overlap` / `org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_attribute_inheritance_rules_effective_on_check→org_attribute_inheritance_rules_effective_check`、`org_attribute_inheritance_rules_tenant_no_overlap_on→org_attribute_inheritance_rules_no_overlap`
  - Recreate（btree）：用 date 列重建 `org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx`（保持索引名与列序不变）。
- `org_role_assignments`
  - Drop（timestamp）：`org_role_assignments_effective_check` / `org_role_assignments_no_overlap` / `org_role_assignments_tenant_node_effective_idx` / `org_role_assignments_tenant_subject_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_role_assignments_effective_on_check→org_role_assignments_effective_check`、`org_role_assignments_tenant_no_overlap_on→org_role_assignments_no_overlap`
  - Recreate（btree）：用 date 列重建 `org_role_assignments_tenant_node_effective_idx`、`org_role_assignments_tenant_subject_effective_idx`（保持索引名与列序不变）。
- `org_security_group_mappings`
  - Drop（timestamp）：`org_security_group_mappings_effective_check` / `org_security_group_mappings_no_overlap` / `org_security_group_mappings_tenant_node_effective_idx` / `org_security_group_mappings_tenant_key_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_security_group_mappings_effective_on_check→org_security_group_mappings_effective_check`、`org_security_group_mappings_tenant_no_overlap_on→org_security_group_mappings_no_overlap`
  - Recreate（btree）：用 date 列重建 `org_security_group_mappings_tenant_node_effective_idx`、`org_security_group_mappings_tenant_key_effective_idx`（保持索引名与列序不变）。
- `org_links`
  - Drop（timestamp）：`org_links_effective_check` / `org_links_no_overlap` / `org_links_tenant_node_effective_idx` / `org_links_tenant_object_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_links_effective_on_check→org_links_effective_check`、`org_links_tenant_no_overlap_on→org_links_no_overlap`
  - Recreate（btree）：用 date 列重建 `org_links_tenant_node_effective_idx`、`org_links_tenant_object_effective_idx`（保持索引名与列序不变）。
- `org_audit_logs`
  - Drop（timestamp）：`org_audit_logs_effective_check`
  - Rename（date）：列 `effective_on→effective_date`、`end_on→end_date`；约束 `org_audit_logs_effective_on_check→org_audit_logs_effective_check`
- `org_personnel_events`
  - Drop（timestamp）：列 `effective_date`（timestamptz）；索引 `org_personnel_events_tenant_person_effective_idx`
  - Rename（date）：列 `effective_on→effective_date`；索引 `org_personnel_events_tenant_person_effective_on_idx→org_personnel_events_tenant_person_effective_idx`

#### 7.5.3 Down：仅用于本地（可选）
阶段 D 为破坏性迁移；如需要 Down，仅建议用于本地开发/回归：
- 反向新增 `timestamptz effective_date/end_date` 并从 date 列回填（以 UTC 00:00 为基准；`end_date_ts = (end_date + 1 day) @ 00:00 UTC`，open-ended 仍使用 `9999-12-31` sentinel 口径）。
- 再次引入 `effective_on/end_on` 不作为长期方案，仅用于回滚验证；成功回滚后应尽快恢复到阶段 C 之前的版本。

### 7.6 阶段 E：接口契约（结束 RFC3339）
> 仅针对 Valid Time 输入；Audit/Tx Time 的 timestamp 字段不受影响。

- Valid Time 输入（Query/Form/JSON）：
  - `effective_date`: `YYYY-MM-DD`
  - `end_date`: `YYYY-MM-DD`（如暴露）
- 行为：
  - RFC3339 timestamp（例如 `2025-01-01T00:00:00Z`）一律返回 422，并提示使用 `YYYY-MM-DD`。
  - 响应体中的 Valid Time 一律回显为 `YYYY-MM-DD`（不得回退为 RFC3339）。

### 7.7 代码变更清单（可直接落地）
#### 7.7.1 Persistence（SQL 与类型）
- [ ] 将所有 SQL 中的 `effective_on/end_on` 列引用替换为 `effective_date/end_date`（date），并删除对应列的 insert/update/select 字段。
- [ ] 将所有 as-of 判断统一为 date 闭区间：`effective_date <= $asOf::date AND end_date >= $asOf::date`（禁止混用 `daterange(...) @>` 与 legacy timestamp 半开比较）。
- [ ] 删除/替换 `tstzrange(effective_date, end_date, '[)')` 的查询与派生（阶段 D 后 schema 不再存在该语义）。

#### 7.7.2 Service/Presentation（输入输出）
- [ ] 将 Valid Time 的格式化统一为 `YYYY-MM-DD`；仅 Audit/Tx Time 使用 RFC3339（避免把审计时间当作生效日期传播）。
- [ ] 移除“先 parse `YYYY-MM-DD`，失败再 parse RFC3339”的兼容逻辑（阶段 E）。

#### 7.7.3 重点文件与搜索入口（帮助落地）
> 目的：给实现者一份“从哪里下手”的确定入口，避免漏改或误改。

- DB：
  - 迁移：`migrations/org/`（新增阶段 D 迁移；参考已存在的 `migrations/org/20251227090000_org_valid_time_day_granularity.sql`）
  - Schema SSOT：`modules/org/infrastructure/persistence/schema/org-schema.sql`
- Persistence（高频命中 `effective_on/end_on` 的写路径）：
  - `modules/org/infrastructure/persistence/org_crud_repository.go`
  - `modules/org/infrastructure/persistence/org_032_repository.go`
  - `modules/org/infrastructure/persistence/org_025_repository.go`
  - `modules/org/infrastructure/persistence/org_053_repository.go`
  - `modules/org/infrastructure/persistence/org_personnel_events_repository.go`
  - `modules/org/infrastructure/persistence/org_deep_read_build_repository.go`（存在从 timestamp 派生 date 的 SQL 片段，阶段 D 后应删除/改写）
  - `modules/org/infrastructure/persistence/valid_time_day.go`（阶段 D 后可能需要重命名/删减，但“date-only”编码能力仍可能保留）
- 输入解析（阶段 E，常见“先 date 再 RFC3339”兼容逻辑）：
  - `modules/org/presentation/controllers/org_api_controller.go`
  - `modules/org/services/org_tree_query_budget_test.go`（如用于兼容性测试，阶段 E 需同步调整）

### 7.8 依赖与里程碑 (Dependencies & Milestones)
**依赖**
- [ ] `docs/dev-plans/064-effective-date-day-granularity.md` 阶段 C 的一致性验证已完成并具备信心进入阶段 D（阶段 D 破坏性强，必须先满足此条件）。

**里程碑（建议按 PR 切分）**
1. [ ] PR-1（阶段 E）：停止接受 RFC3339 输入；更新相关 controller/service 校验与错误返回；保持输出为 `YYYY-MM-DD`。
2. [ ] PR-2（阶段 D）：Org 迁移（删旧 timestamp + 改名 + 约束/索引去 `_on`）+ 同步更新 `modules/org/infrastructure/persistence/schema/org-schema.sql` + 更新所有 Org SQL/Go 代码以匹配新列名与 date 语义。
3. [ ] PR-3（清理）：删除遗留 helper/字段/测试中对 `effective_on/end_on` 与 timestamp 口径的引用；确保全仓检索无残留（命中 5.3 停止线的项应归零）。

### 7.9 测试与验收标准 (Acceptance Criteria)
**最小验收（必须全部满足）**
- [ ] Schema：`information_schema.columns` 中不存在任何 `effective_on` / `end_on`（Org 相关表）。
- [ ] Schema：不存在任何 `tstzrange(effective_date, end_date, '[)')` 相关约束/索引（Org 相关表）。
- [ ] Code：`rg -n "\\beffective_on\\b|\\bend_on\\b" modules/org` 无输出（历史迁移文件允许存在于 `migrations/org/`，但运行时代码必须归零）。
- [ ] 行为：as-of 查询符合“结束日当天仍有效”的 day 闭区间语义；相邻段满足 `prev.end_date + 1 day == next.effective_date`；重叠段被 EXCLUDE 阻止。
- [ ] 门禁：按 `AGENTS.md` 的触发器矩阵执行并通过（含 Org schema/迁移门禁与 Go lint/test）。

### 7.10 回滚与风险 (Rollback & Risks)
- **阶段 E（接口收紧）**：回滚仅涉及应用代码，直接回滚对应 PR 即可。
- **阶段 D（删列 + 改名）**：破坏性强；回滚成本高，必须在执行前：
  - [ ] 具备可恢复手段（至少本地/环境级快照或可重复重建的 seed）。
  - [ ] 通过 7.5.2 的一致性 preflight（不通过则禁止进入删列步骤）。
  - [ ] 明确“回滚后停留在哪个阶段”（建议回到阶段 C，并暂缓再次进入阶段 D，直到不一致原因被定位并修复）。

### 7.11 安全与鉴权 (Security & Authz)
- 本计划不新增/修改 Casbin Policy。
- DB/SQL 层仍必须以 `tenant_id` 作为隔离边界；阶段 D 的重命名不得引入任何跨租户查询/写入路径。

### 7.12 运维与监控 (Ops & Monitoring)
- 不引入 Feature Flag/灰度/长期监控项（仓库级原则见 `AGENTS.md`）；以门禁与测试覆盖为主。
