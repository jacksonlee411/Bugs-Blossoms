# DEV-PLAN-064：生效日期（日粒度）统一：Valid Time=DATE，Audit Time=TIMESTAMPTZ

**状态**: Phase 1（Org）已落地（B1 双轨：date 列已引入并双写；legacy timestamp 列待清理）（2025-12-27 11:45 UTC）

> 目标：将“业务生效日期/有效期（Valid Time）”从 `timestamptz`（秒/微秒级）收敛为 **day（date）粒度**，对齐 SAP HCM（`BEGDA/ENDDA`）与 PeopleSoft（`EFFDT/EFFSEQ`）的 HR 习惯；同时明确 **时间戳（秒/微秒级）仅用于操作/审计时间（Audit/Tx Time）**（如 `created_at/updated_at/transaction_time`）。

## 0. 已完成事项（2025-12-27）
- Phase 1（Org）按 B1 双轨落地：DB 新增 `effective_on/end_on`（date），按 UTC 归一化回填，并增加 `daterange(effective_on, end_on + 1, '[)')` 的 EXCLUDE 约束与索引；Schema SSOT 已同步。
- Go 层写路径已双写 `effective_on/end_on`；读/输出契约已收敛为 `YYYY-MM-DD`（兼容 RFC3339 输入，但会归一化为 date 并回显为 day string）。
- Cursor（`effective_date:...:id:...`）已改为 day string，并在解析时兼容 legacy RFC3339。
- 已跑门禁并通过：`go fmt ./...`、`go vet ./...`、`make check lint`、`make test`、`make check doc`、`make org lint`、`GOOSE_TABLE=goose_db_version_org make org migrate up`、`make authz-test && make authz-lint`。
- 12.2 中 Phase 1 DEV-PLAN 清单收敛已完成：A 类/B 类文档已修订或复核，并在各文档头部记录“对齐更新”（以避免继续传播旧的 timestamp/半开区间口径）。
- 未完成：阶段 D（清理 legacy timestamp 列/旧约束）与阶段 E（停止接受 RFC3339 输入）仍待后续 PR。

## 1. 背景与上下文 (Context)
- **需求来源**：HR 用户对“结束日期当天是否有效”的一致性诉求；对齐 SAP HCM / PeopleSoft 的 day 粒度 effective dating 心智模型。
- **业务价值**：减少 off-by-one 误解与边界错误；让 Valid Time 与审计时间分层，降低后续演进（时区/报表/导入）成本。
- 当前实现（以 Org 时态模型为代表）将 `effective_date/end_date` 定义为 `timestamptz`，并在 DB/查询/写入算法中大量依赖半开区间 `[effective_date, end_date)`（例如 `end_date > asOf` 与 `tstzrange(...,'[)')` EXCLUDE 约束）。
- 业务侧（HR）对“生效日期”的理解通常是**按天**（Key Date / Effective Date），并期待“结束日期当天仍有效”（SAP 常见闭区间心智模型）。
- `timestamptz` 粒度会带来：
  - UI/导出“结束日期是否包含当天”的误解（off-by-one）。
  - 写入截断时对时间精度/边界点的隐式依赖（尤其当允许 RFC3339 输入时更危险）。
  - 未来若引入租户时区/本地日界线，会放大 timestamp 与 civil date 之间的转换歧义。
- 现状为何如此（不是“技术错误”，而是“业务语义不匹配”）：
  - 采用 `timestamptz + tstzrange('[)') + EXCLUDE` 能在 Postgres 层强约束“同键区间不重叠”，并让相邻切片 `prev.end == next.start` 自然拼接。
  - 该模型适合“连续时间”语义，但 HR 的核心心智模型是“按天生效（civil date）”；当 UI/API 以 `YYYY-MM-DD` 输入但底层仍是 timestamp，就会积累边界与误解成本。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] **定义并落地时间域分层（Time Domains）**：
  - [ ] **Valid Time（业务有效期）**：统一为 `date`（day 粒度）。
  - [ ] **Audit/Tx Time（操作/审计时间）**：保留 `timestamptz`（秒/微秒级），用于 `created_at/updated_at/transaction_time` 等字段。
- [ ] 将“业务生效日期/有效期（Valid Time）”相关字段从 `timestamptz` 收敛为 `date`（优先落地 Org/Position/Assignment 这条 HR 核心链路）。
- [ ] API/UI 契约：`effective_date` / `end_date` 的输入输出统一为 `YYYY-MM-DD`（date），不再鼓励以 RFC3339 timestamp 作为“生效日期”传入。
- [ ] 更新写入算法：以 day 粒度处理截断/续接，避免边界重叠与 off-by-one。
- [ ] 更新 DB 约束与索引：将 `tstzrange` 的时态防重叠约束迁移为 date 口径（`daterange`），并保持“同键区间不重叠”的强约束能力。
- [ ] **SSOT 对齐**：更新所有引用旧契约（UTC + 半开区间 `[effective_date,end_date)`）的文档/注释/代码约定，使“Valid Time=date（日粒度）”成为唯一权威表达，避免双 SSOT。

### 2.2 非目标 (Out of Scope)
- 不在本计划中引入“租户时区驱动的 today/as-of”语义（仍按项目既有 UTC 约定；如需按租户日历，另起计划）。
- 不在本计划中把系统改造成完整的双时态（Bi-temporal）模型（Valid Time 与 Transaction Time 的交叉维度暂不扩展）。
- 不在本计划中重做权限模型本身（仅确保权限映射等时态表随粒度调整而一致）。
- 不调整/不降级 `created_at/updated_at/transaction_time` 等 Audit/Tx Time 字段的精度（仍为 `timestamptz`）。

### 2.3 工具链与门禁（SSOT 引用）
- **触发器清单（本计划命中项）**：
  - [x] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] `.templ` / Tailwind（`make generate && make css`，并确保生成物提交）
  - [x] DB 迁移 / Schema（Org Atlas+Goose：`make org plan && make org lint && make org migrate up`，见 `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`）
  - [x] Authz（若触及 `org_role_assignments`/security group mappings 的契约或查询：`make authz-test && make authz-lint`）
  - [x] 文档门禁（`make check doc`）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口与脚本实现：`Makefile`
  - CI 门禁定义：`.github/workflows/quality-gates.yml`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.0 架构图 (Mermaid)
```mermaid
graph TD
    UI[Client: UI/HTMX/JSON] --> C[Controller]
    C -->|Parse YYYY-MM-DD| V[Valid Time: date]
    C --> S[Service]
    S --> R[Repository]
    R --> DB[(Postgres)]
    S --> A[Audit/Outbox]
    A --> DB

    subgraph Valid Time（业务日）
      V
    end
    subgraph Audit/Tx Time（操作时刻）
      T[created_at/updated_at/transaction_time: timestamptz]
    end
```

### 3.1 时间域定义（新全局契约）
1) **Valid Time = Civil Date**
   - `effective_date` / `end_date` 表示“业务日”粒度的有效期边界（date）。
   - 统一使用 `YYYY-MM-DD` 表示，不携带时区与时刻信息。

2) **Audit/Tx Time = Timestamp**
   - `created_at` / `updated_at` / `transaction_time` 等字段保留 `timestamptz`，记录真实操作发生时刻（允许秒/微秒级）。

### 3.2 `end_date` 语义（对齐 SAP 心智）
- 选定：Valid Time 使用**闭区间（按天）**语义：记录在 `effective_date` 当天生效，在 `end_date` 当天仍有效。
- 查询口径：`effective_date <= as_of_date AND end_date >= as_of_date`（全部为 date 比较）。
- 约束口径（用于 DB EXCLUDE 防重叠）：将闭区间映射为半开 range：
  - `valid_range = daterange(effective_date, end_date + 1, '[)')`
  - 解释：`end_date + 1` 是“次日 00:00”的上界，保持半开区间不重叠的不变量（避免边界点双命中）。

#### 3.2.1 评估：改造后仍可保持“切片自动化 + 自然拼接”
- **不重叠强约束不丢失**：Postgres 的 `daterange` 同样支持 `EXCLUDE USING gist (... WITH &&)`；将闭区间映射为半开 `valid_range` 后，不重叠约束能力与当前 `tstzrange('[)')` 等价。
- **“自然拼接”仍成立，但拼接条件随粒度变化**：
  - 现状（timestamp 半开）：相邻切片拼接条件是 `prev.end == next.start`。
  - 目标（date 闭区间）：相邻切片拼接条件是 `prev.end_date + 1 day == next.effective_date`，在约束映射下等价于 `upper(prev.valid_range) == lower(next.valid_range)`。
- **自动化写入算法可保持同一形态**：插入切片的“截断点”从 `end = D` 变为 `end_date = D - 1 day`；其余流程（锁定覆盖段、截断、插入、依赖 EXCLUDE 兜底）保持不变（见 6.1）。
- **前提（必须明确）**：若历史或未来业务允许“同一自然日内多次生效”（PeopleSoft 的 `EFFSEQ` 需求），则 date-only Valid Time 无法表达多段同日切片；需另起计划引入 `effective_seq` 或扩展为双时态（本计划不覆盖）。

### 3.3 截断/续接算法（day 粒度）
- 当在日期 `D` 插入新的切片时，上一切片应被界定为 `end_date = D - 1 day`（而不是 `D`）。
- 新切片的 `end_date`：
  - 若存在下一切片起始日 `N`：新切片 `end_date = N - 1 day`
  - 否则：`end_date = end_of_time`（默认 `9999-12-31`）

### 3.4 SSOT 对齐与替换（避免双权威表达）
本计划落地后，“Valid Time=date（日粒度）”成为唯一权威表达。以下文档/契约需要在实施过程中同步更新（以避免继续传播旧的“timestamptz + 半开区间”口径）：
- `docs/dev-plans/020-organization-lifecycle.md`（已对齐为 day 闭区间口径）
- `docs/dev-plans/021-org-schema-and-constraints.md`（已对齐为 Valid Time=date + `daterange(...,'[)')` 映射；legacy 双轨已标注）
- `docs/dev-plans/053-position-core-schema-service-api.md`（已对齐：以 `effective_on/end_on`（date）为准；legacy 双轨已标注）
原则：
- 文档层面：统一描述为“按天闭区间（最后有效日）”，并明确 DB 约束使用 `daterange(effective_date, end_date + 1, '[)')` 的映射方式。
- 代码层面：所有“as-of 判定/截断”必须以 date 口径集中实现，禁止在散落的 SQL/templ 中重复定义边界语义。

### 3.3 评估：改造后能否保持“自动切片 + 自然拼接”
结论：**可以**，并且仍可保留 Postgres 层的“同键区间不重叠”强约束；只是“自然拼接”的表达从 timestamp 的 `prev.end == next.start`，变为 day 闭区间下的 `prev.end_date + 1 day == next.effective_date`（在 range 映射层仍是 `upper(prev) == lower(next)`）。

关键点：
- **DB 强约束不变，只是 range 类型变化**：`tstzrange('[)') + EXCLUDE` → `daterange(effective_date, end_date + 1, '[)') + EXCLUDE`。依旧能在同一把 key（tenant + business key）下强制“区间不重叠”。
- **自然拼接仍成立（在 range 层保持半开）**：业务字段用闭区间更贴近 HR 心智，但 DB 约束/查询统一映射为半开 `[)` range；相邻切片只要满足 `prev.end_date + 1 == next.effective_date`，则映射后的 range 满足 `upper(prev) == lower(next)`，天然可拼接且不 overlap。
- **切片算法仍是同一个“拆分/截断/续接”模板**：以 `as_of` 的 day `D` 为切片边界，截断旧段为 `end_date = D - 1`，新段起点为 `effective_date = D`；若存在下一段 `N`，则新段终点为 `N.effective_date - 1`，否则 `9999-12-31`（等价于 PeopleSoft 的“end 由 next start - 1 推导”）。
- **必要前提：写入必须先归一化到 day**：兼容期允许 RFC3339 输入，但必须在边界处先归一化为 UTC day（只保留 `YYYY-MM-DD` 的日期归属），避免 “06:00 → 05:59:59.999999” 这类时间成分把 day 区间意外拉宽并触发 overlap。
- **迁移前置校验（避免历史数据时间成分导致 overlap）**：在加 `daterange` EXCLUDE 前，应检测是否存在 `effective_date/end_date` 非 UTC 00:00:00 的记录；若存在，需要先把 legacy timestamp 归一化到“UTC 00:00 边界”（或先停用 date 约束进入修复/回填阶段），否则从 timestamp→date 的映射可能会把相邻切片扩展到同一天而被 EXCLUDE 拦截。

### 3.5 Go 层权威类型与边界（建议）
为避免“date 被 `time.Time/timestamptz` 再次污染”，本计划要求为 Valid Time 选定唯一权威表达，并在边界集中转换：
- HTTP / JSON / Form：
  - 输入输出一律为 `YYYY-MM-DD`（string）。
  - 兼容期内允许 RFC3339，但必须先归一化到 UTC date 并回显为 `YYYY-MM-DD`（见 4.5 阶段 A）。
- Go（业务层）：
  - 推荐使用 `pkg/shared.DateOnly` 作为 Valid Time 的权威类型（其 form decoder 已支持 `YYYY-MM-DD`）。
  - 禁止在业务逻辑中把 Valid Time 当作“可带时刻”的 `time.Time` 做比较（除非先显式归一化为 date）。
- DB（持久化层）：
  - in-scope 表的 Valid Time 字段最终收敛为 Postgres `date`。
  - 在 SQL/Repository 层禁止依赖 DB session 时区；凡涉及 cast/归一化，必须用 `AT TIME ZONE 'UTC'`（见 4.4）。

## 4. 数据模型与约束 (Data Model & Constraints)
### 4.1 目标范围（Phase 1：Org 核心 HR 链路）
本计划 Phase 1 明确落地范围为 **modules/org** 中所有以 `effective_date/end_date` 表达 **Valid Time** 的表（字段含义为“业务有效期”），并将其类型迁移为 `date`。

已知在 `modules/org/infrastructure/persistence/schema/org-schema.sql` 中命中（后续以 schema SSOT 为准逐一核对）：
- `org_node_slices(effective_date, end_date)`
- `org_edges(effective_date, end_date)`
- `org_positions(effective_date, end_date)`
- `org_position_slices(effective_date, end_date)`
- `org_assignments(effective_date, end_date)`
- `org_hierarchy_closure(effective_date, end_date)`（以及 closure build/snapshot 中如存在对应字段）
- `org_attribute_inheritance_rules(effective_date, end_date)`
- `org_role_assignments(effective_date, end_date)`
- `org_security_group_mappings(effective_date, end_date)`
- `org_links(effective_date, end_date)`
- `org_audit_logs(effective_date, end_date)`：这里的 `transaction_time/created_at` 仍属 Audit/Tx Time（保留 `timestamptz`），但 `effective_date/end_date` 作为“变更影响的业务有效期窗口”归类为 Valid Time（迁移为 `date`）。
- `org_personnel_events(effective_date)`：人事事件的生效日属于 Valid Time（点时间），应收敛为 `date`；`created_at/updated_at` 仍为 Audit/Tx Time。

### 4.2 非范围（明确不做什么）
- 不调整 Audit/Tx Time 字段：`created_at/updated_at/transaction_time/available_at/...` 继续为 `timestamptz`。
- 不把“按天”语义扩展为“按租户时区日界线”（仍按既有 UTC 约定；本计划只定义“date 是权威输入输出形态”）。
- 不把所有模块一次性迁移为 date：Phase 1 只覆盖 Org 核心链路；其他模块若出现 Valid Time 字段，必须在后续明确纳入范围（新增 DEV-PLAN 或扩展本计划并列出表/字段清单）。

### 4.3 约束迁移（date 口径）
以当前 `tstzrange(...,'[)')` EXCLUDE 为例，迁移为：
- 原：`tstzrange(effective_date, end_date, '[)')`
- 新：`daterange(effective_date, end_date + 1, '[)')`

同时将有效期 check 从：
- 原：`CHECK (effective_date < end_date)`
- 新：`CHECK (effective_date <= end_date)`（允许“单日有效”）

触发器/函数内的 as-of 判定从：
- 原：`end_date > NEW.effective_date`
- 新：`end_date >= NEW.effective_date`

### 4.3.1 Schema 变更示例（以 `org_node_slices` 为例）
> 目的：把“类型/约束/索引策略”落到可执行级别，其他表按相同模式迁移（以 schema SSOT 为准逐一落地）。

1) 新增 date 列（B1 双轨）：
```sql
ALTER TABLE org_node_slices
  ADD COLUMN effective_on date NOT NULL,
  ADD COLUMN end_on date NOT NULL DEFAULT '9999-12-31';
```

2) 回填（UTC 归一化，规则见 4.4）：
```sql
UPDATE org_node_slices
SET
  effective_on = (effective_date AT TIME ZONE 'UTC')::date,
	  end_on = CASE
	    WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
	    ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
	  END;
```

3) 约束与索引（保持“同键区间不重叠”的强约束能力）：
```sql
ALTER TABLE org_node_slices
  ADD CONSTRAINT org_node_slices_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_node_slices
  ADD CONSTRAINT org_node_slices_no_overlap_on
  EXCLUDE USING gist (
    tenant_id WITH =,
    org_node_id WITH =,
    daterange(effective_on, end_on + 1, '[)') WITH &&
  );
```

> 注：上述示例省略了 `gist_uuid_ops` 等操作符类细节；实施时应与现有 `tstzrange` 约束的 key 列/操作符保持一致（以 `modules/org/infrastructure/persistence/schema/org-schema.sql` 为 SSOT）。

### 4.4 数据归一化规则（从 timestamptz 半开到 date 闭区间）
本计划需要将旧模型（`timestamptz` + 半开 `[effective_date,end_date)`）迁移为新模型（`date` + 闭区间 `[effective_date,end_date]`）。归一化规则必须确定且可复现：

- 统一以 **UTC** 作为“日期归属”的判定基准（避免 DB 会话时区造成漂移）。
- `effective_date`（旧：timestamptz）→ `effective_date`（新：date）：
  - `effective_date_date = (effective_date AT TIME ZONE 'UTC')::date`
- `end_date`（旧：timestamptz，exclusive）→ `end_date`（新：date，inclusive）：
  - 若为 open-ended（以 `(end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31'` 判断）：`end_date_date = '9999-12-31'::date`
  - 否则：`end_date_date = (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date`
    - 解释：对任意 `end_date`（exclusive）减去一个最小时间单位后再取 date，能正确覆盖“非 00:00 的 end_date”与“刚好 00:00 的边界”（00:00 会回落到前一日，符合“最后有效日”）。

### 4.5 迁移策略（选定：B1 双列迁移，最终收敛为 date-only）
为保证可回滚与可观测性，本计划选定 **B1（新增 date 列 + 双写/回填）** 为默认路径；B2（直接改列类型）仅允许在明确接受停机窗口与表规模评估通过时采用，且不作为默认执行路径。

1) **阶段 A：接口与类型先收敛为 day（兼容期）**
   - 输入：所有 `effective_date/end_date` 在 Controller/Service 边界统一归一化为 day（UTC date）。
   - 兼容：短期允许 RFC3339，但会被归一化为 UTC date 并回显为 `YYYY-MM-DD`；并打点观测调用方是否仍在传 timestamp。

2) **阶段 B：Schema 引入 date 列并回填（不改变读路径）**
   - 为每张 in-scope 表新增 `effective_on date NOT NULL`、`end_on date NOT NULL`（列名后续可在最终阶段重命名为 `effective_date/end_date`，以避免双语义混用）。
   - 按“4.4 数据归一化规则”回填 `effective_on/end_on`。
   - 为 `effective_on/end_on` 增加 check/exclude（使用 `daterange(effective_on, end_on + 1, '[)')`），但暂不删除旧的 `tstzrange` 约束（双轨并行，以便验证一致性）。

3) **阶段 C：应用双写并切换读路径（一次部署内保持可回滚）**
   - 写入：所有创建/截断/续接同时写 `timestamptz` 列与 `date` 列（`effective_on/end_on`）。
   - 读取：as-of 查询改为 date 口径（优先用 `effective_on/end_on`），并在关键链路增加一致性断言（例如在集成测试中对比 timestamp 口径与 date 口径结果）。

4) **阶段 D：清理旧列与旧约束（收敛）**
   - 删除旧的 `timestamptz effective_date/end_date` 约束与索引，迁移为 date-only 版本（`daterange`）。
   - 视代码可读性决定是否做列重命名（例如 `effective_on -> effective_date`，`end_on -> end_date`）并同步更新代码，最终做到“Valid Time 字段名与类型一致，不存在双轨”。

5) **阶段 E：收紧兼容（结束 RFC3339）**
   - API 层将 RFC3339 输入从“兼容归一化”升级为“422 拒绝”，确保外部契约不再漂移回 timestamp。

回滚策略（与阶段绑定）：
- 阶段 A/B：可直接回滚应用或回滚迁移（旧列仍为权威）。
- 阶段 C：读路径切换建议受控（配置开关或小步发布）；回滚可切回旧读路径。
- 阶段 D：一旦删旧列，回滚成本显著增加，因此必须在阶段 C 的观测与验证完成后再进入。

## 5. 接口契约 (API Contracts)
### 5.1 Query / Form / JSON（统一约定）
- `effective_date`: `YYYY-MM-DD`（required/optional 以各 endpoint 为准）
- `end_date`: `YYYY-MM-DD`（如暴露；表示“最后有效日”，闭区间）
- 禁止在 Valid Time 字段中承载时区/时刻；如收到 RFC3339：
  - 兼容期：规范化为 date（UTC 日期），并返回响应中以 date 形式回显。
  - 收敛期：返回 422，并提示使用 `YYYY-MM-DD`。

### 5.2 事件契约（保持 v1，永远不新增 v2）
已存在 topic（以 `modules/org/domain/events/v1.go` 为准）：
- `org.changed.v1`
- `org.assignment.changed.v1`
- `org.personnel_event.changed.v1`

决策：
- 当前确认无外部消费方，本计划**不做事件版本化**；topic 保持 `*.v1`，并将其视为仓库内契约的一部分。
- **禁止**通过新增 `*.v2` 来解决演化/兼容问题（永远不新增 v2）。
- 如需扩展：仅允许在 `*.v1` payload 中以“新增字段”的方式做向后兼容扩展；禁止破坏性字段语义变更（若不得已，必须先重新评审并更新本 dev-plan 的决策段落）。

`EffectiveWindow` 的表达（Valid Time=date，建议用 string 以避免时区歧义）：
  ```json
  {
    "effective_date": "2025-01-01",
    "end_date": "9999-12-31"
  }
  ```
说明：
- `transaction_time/created_at` 等 Audit/Tx Time 字段仍使用 RFC3339 timestamp（秒/微秒级）；仅 `effective_date/end_date` 表达 day 粒度。

### 5.3 错误码与失败路径（最小约定）
- 解析失败（格式不合法）：
  - HTTP 400：`effective_date`/`end_date` 必须为 `YYYY-MM-DD`（兼容期允许 RFC3339，但会被归一化后回显）。
- 语义校验失败（例如 `effective_date > end_date`，或违反冻结窗口/业务规则）：
  - HTTP 422/409：沿用既有业务错误码策略（以现有 controller/service 约定为准），但错误消息必须明确是“按天”的语义。

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 写入：切片插入/变更（伪代码）
1) 解析 `effective_date` 为 `date D`（day）。
2) 查当前覆盖 `D` 的切片 `S`（`S.effective_date <= D <= S.end_date`）。
3) 若需要插入新切片：
   - 截断旧切片：`S.end_date = D - 1`
   - 计算新切片结束 `E`：若存在下一切片 `N`，则 `E = N.effective_date - 1`，否则 `E = end_of_time`
   - 插入新切片 `[D, E]`
4) 依赖 DB EXCLUDE（使用 `daterange(D, E+1, '[)')`）兜底防重叠。

## 7. 安全与鉴权 (Security & Authz)
- 本计划不引入新的 Casbin policy，也不改变 object/action 语义；风险点集中在“时态表的生效判断口径”变化可能导致权限过宽/过窄。
- 重点关注：
  - `org_role_assignments`、`org_security_group_mappings` 等时态权限映射表：as-of 判定从 timestamp 口径迁移到 date 口径后，必须确保“结束日当天仍有效”的业务语义与 DB 约束一致。
  - 所有查询必须继续包含 `tenant_id` 过滤，避免跨租户泄漏。
- 验证：若触及上述表或其查询路径，必须执行 `make authz-test && make authz-lint`（入口见 `AGENTS.md`）。

## 8. 风险与回滚 (Risks & Rollback)
- **数据语义风险**：若历史数据存在非 00:00 UTC 的 `effective_date/end_date`，直接 cast 为 date 可能产生“日期漂移”。需先盘点并决定归一化策略。
- **同日多次变更风险**：若未来需要支持“同一 `effective_date` 下多次变更”的 HR 用例（类似 PeopleSoft `EFFSEQ`），仅用 date 作为 Valid Time 将无法表达同日内的顺序切片；需引入 `effective_seq` 或扩展为双时态（另起计划）。
- **约束重建风险**：EXCLUDE/GiST 约束迁移会导致写路径行为变化，必须用集成测试覆盖“相邻段/边界日”。
- **下游契约风险**：虽然当前确认无外部消费方，仍需避免引入隐式耦合（例如内部任务/脚本/导出工具）；事件 topic 保持 `*.v1`，扩展仅允许新增字段。
- **回滚**：
  - 采用 B1：阶段 D 之前均可回滚到旧列作为权威；阶段 D 之后回滚成本高，需额外“反向回填”迁移（不建议）。
  - 采用 B2：回滚成本高，且依赖额外迁移脚本；因此 B2 不作为默认路径。

## 9. 依赖与里程碑 (Dependencies & Milestones)
### 9.1 依赖（前置）
- Org DB 工具链与门禁：`docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
- 受影响契约 SSOT（需同步修订）：见 3.4 与 12.2

### 9.2 里程碑（执行顺序）
1. [x] 完成 Phase 1 范围核对：逐表确认 Valid Time 字段与 in-scope 清单（含 `org_audit_logs` 的字段归类）。
2. [x] 阶段 A：完成 Controller/DTO date-only 收敛（兼容期策略 + 观测指标）。
3. [x] 阶段 B：完成 Org schema/migrations：新增 `effective_on/end_on`（date）、回填、并行 check/exclude（daterange）。
4. [x] 阶段 C：完成 Repository/Service：双写 + 读路径切换为 date 口径；补齐边界日/相邻段测试。
5. [ ] 阶段 D：清理旧列/旧约束，收敛为 date-only；同步更新引用旧契约的 SSOT 文档（见 3.4）。
6. [ ] 阶段 E：收紧 API：停止接受 RFC3339 timestamp 作为 Valid Time 输入；事件保持 `*.v1`，并完成 `EffectiveWindow` day 粒度序列化收敛。
7. [x] Readiness：按 `AGENTS.md` 触发器运行并记录结果（见 0），更新状态为“准备就绪/已批准”。

## 10. 运维与监控 (Ops & Monitoring)
- 不引入开关切换/灰度模式：本项目仍处于初期、未发布上线，避免过度运维和监控（仓库级原则见 `AGENTS.md`）。
- 以门禁与测试为主：通过集成测试覆盖“相邻段/边界日/不重叠约束”等关键场景来验证一致性；若需要排查兼容期输入（RFC3339）等情况，优先用一次性脚本或临时日志定位，而非引入长期监控项。

## 11. 测试与验收标准 (Testing & Acceptance Criteria)
### 11.1 测试策略（最小集合）
- 迁移正确性：
  - 回填后，date 口径 as-of 查询与旧 timestamp 口径结果一致（除“结束日包含”的语义变化外，必须明确并可解释）。
  - 覆盖 open-ended（`9999-12-31`）与边界日（`D`、`D-1`）场景。
- 约束有效性：
  - 相邻段允许：`prev.end_on + 1 day == next.effective_on`
  - 重叠段拒绝：EXCLUDE 约束应阻止插入/更新产生 overlap。
- 兼容期行为：
  - RFC3339 输入会被归一化并回显为 `YYYY-MM-DD`；兼容期结束后返回 422/400（按 5.3）。

### 11.2 验收标准（不变量）
- [ ] 任意 as-of 日期（date）下，时态查询结果与“结束日当天仍有效”的业务预期一致。
- [ ] 相邻切片满足 `prev.end_date + 1 day == next.effective_date`（无重叠、无空档/或按各实体约束定义）。
- [ ] DB 层不重叠约束仍然有效（EXCLUDE + range overlap）。
- [ ] Audit/操作字段仍以 `timestamptz` 记录真实发生时刻；Valid Time 字段不再出现时刻信息。
- [ ] 事件 topic 保持 `*.v1`，不新增任何 `*.v2`。
- [ ] 通过本计划命中的 CI 门禁（以 `AGENTS.md` + `Makefile` 为准）。

## 12. 契约文档收敛计划（Contract First）
本计划变更的是“全局时间语义契约”（Valid Time=date（日粒度）），为避免文档漂移与实现阶段即兴决策，需要**有计划地分析并修订所有受影响的 dev-plan**，确保仓库内只有一种权威表达。

### 12.1 分析方法（可复现、可审计）
1. [x] 在 `docs/dev-plans/` 全量检索与 Valid Time 相关的关键字与模式（例如：`effective_date`、`end_date`、`半开区间`、`[effective_date,end_date)`、`tstzrange(`、`timestamptz`）。
2. [x] 对命中文档进行分类：
   - A 类（契约型）：定义字段类型/区间语义/约束/核心算法（必须修订为本计划口径）。
   - B 类（引用型）：仅引用字段名或作为 UI/路由参数/示例（需要复核并改为引用本计划口径，避免暗示 timestamp 语义）。
3. [x] 对 A 类文档执行最小但完整的契约修订：
   - Valid Time 类型：`timestamptz` → `date`
   - 区间语义：由 `[effective_date,end_date)`（半开）收敛为 day 闭区间 `[effective_date,end_date]`，并明确 DB EXCLUDE 映射为 `daterange(effective_date, end_date + 1, '[)')`
   - 截断规则：`end_date = D` → `end_date = D - 1 day`
   - 示例 JSON/SQL：`effective_date/end_date` 一律使用 `YYYY-MM-DD`
4. [x] 对 B 类文档执行“去歧义”修订：删除/替换暗示 `timestamptz` 的表述，并增加到本计划（DEV-PLAN-064）的链接作为 SSOT。
5. [x] 每次修订后运行 `make check doc`，并在对应 dev-plan 中记录变更与时间戳（遵循 `docs/dev-plans/000-docs-format.md`）。

### 12.2 需修订/复核的 DEV-PLAN 清单（Phase 1）
> 注：本清单以“命中 `effective_date/end_date/tstzrange/timestamptz/半开区间` 等关键字”为初始输入，修订过程中可增补，但不得遗漏已命中项。

**A 类（契约型，必须修订）**
1. [x] `docs/dev-plans/001-technical-design-template.md`（模板示例的类型/约束口径）
2. [x] `docs/dev-plans/020-organization-lifecycle.md`
3. [x] `docs/dev-plans/021-org-schema-and-constraints.md`
4. [x] `docs/dev-plans/022-org-placeholders-and-event-contracts.md`
5. [x] `docs/dev-plans/023-org-import-rollback-and-readiness.md`
6. [x] `docs/dev-plans/024-org-crud-mainline.md`
7. [x] `docs/dev-plans/025-org-time-and-audit.md`（Audit/Tx Time vs Valid Time 边界）
8. [x] `docs/dev-plans/026-org-api-authz-and-events.md`（事件 payload 的日期表达）
9. [x] `docs/dev-plans/028-org-inheritance-and-role-read.md`
10. [x] `docs/dev-plans/029-org-closure-and-deep-read-optimization.md`
11. [x] `docs/dev-plans/030-org-change-requests-and-preflight.md`
12. [x] `docs/dev-plans/031-org-data-quality-and-fixes.md`
13. [x] `docs/dev-plans/032-org-permission-mapping-and-associations.md`
14. [x] `docs/dev-plans/033-org-visualization-and-reporting.md`
15. [x] `docs/dev-plans/036-org-sample-tree-data.md`（复核：无需修订）
16. [x] `docs/dev-plans/052-position-contract-freeze-and-decisions.md`
17. [x] `docs/dev-plans/053-position-core-schema-service-api.md`
18. [x] `docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`（复核：无需修订）
19. [x] `docs/dev-plans/057-position-reporting-and-operations.md`
20. [x] `docs/dev-plans/058-assignment-management-enhancements.md`
21. [x] `docs/dev-plans/059-position-rollout-readiness-and-observability.md`
22. [x] `docs/dev-plans/061-org-position-person-bridge-and-minimal-personnel-events.md`
23. [x] `docs/dev-plans/061A-person-detail-hr-ux-improvements.md`（复核：无需修订）
24. [x] `docs/dev-plans/061A1-person-assignment-effective-date-and-action-type.md`
25. [x] `docs/dev-plans/062-job-data-entry-consolidation.md`（复核：无需修订）

**B 类（引用型，需要复核并去歧义）**
1. [x] `docs/dev-plans/020L-org-feature-catalog.md`
2. [x] `docs/dev-plans/020T1-org-test-gap-closure-plan.md`（复核：无需修订）
3. [x] `docs/dev-plans/027-org-performance-and-rollout.md`
4. [x] `docs/dev-plans/034-org-ops-monitoring-and-load.md`
5. [x] `docs/dev-plans/035-org-ui.md`
6. [x] `docs/dev-plans/035A-org-ui-ia-and-sidebar-integration.md`（复核：无需修订）
7. [x] `docs/dev-plans/037-org-ui-ux-audit.md`（复核：无需修订）
8. [x] `docs/dev-plans/037A-org-ui-verification-and-optimization.md`（复核：无需修订）
9. [x] `docs/dev-plans/043-ui-action-error-feedback.md`（复核：无需修订）
10. [x] `docs/dev-plans/053A-position-contract-fields-pass-through.md`（复核：无需修订）
11. [x] `docs/dev-plans/055-position-ui-org-integration.md`
