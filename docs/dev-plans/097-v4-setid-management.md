# DEV-PLAN-097：SetID 管理（V4，Greenfield）

**状态**: 草拟中（2026-01-05 11:16 UTC）

> 适用范围：**077 以及之后计划的 V4 Greenfield 全新实现**。  
> 本文研究 PeopleSoft 的 SetID 机制，并提出 V4 引入 SetID 的最小可执行方案：在同一租户内实现“主数据按业务单元共享/隔离”的配置能力，且可被门禁验证，避免实现期各模块各写一套数据共享规则导致漂移。

## 1. PeopleSoft SetID 机制：作用与目的（摘要）

### 1.1 SetID 是什么
- **SetID** 是一个短标识（PeopleSoft 习惯为 5 位字符），作为大量“基础主数据表”的关键字段之一，用于把主数据划分为不同的数据集（Set）。
- **同一个 SetID 下的主数据**可以被多个业务单元（Business Unit）共享；不同 SetID 之间则天然隔离（可存在“同编码不同含义”的并行配置）。

### 1.2 解决的问题（为什么需要）
- **共享 vs 隔离**：同一集团内，多 BU 需要共享一套通用字典（如 Job Code、Location），同时又要允许某些 BU 拥有本地化差异（如部门、工资等级规则）。
- **避免复制**：不用为每个 BU 复制整套字典表；通过 SetID 选择“用哪一套”即可。
- **可控的一致性**：通过中心化配置（Set Control）约束每个 BU 在每类主数据上使用哪个 SetID，减少“自由组合”导致的数据漂移。

### 1.3 关键配套概念（PeopleSoft 的核心结构）
- **Set Control Value**：用于“选择 SetID”的控制维度，PeopleSoft 常用 Business Unit 作为 set control value。
- **Record Group**：把一组主数据表归为同一类（同一组共享同一个 SetID 选择），避免每张表单独配置。
- **Set Control（映射）**：`(business_unit_id, record_group) -> setid` 的确定性映射，保证每次查询/写入都能稳定得到唯一 SetID。

> V4 引入 SetID 的目标是复用上述“确定性映射 + 共享/隔离”的思想，而不是复刻 PeopleSoft 的全部页面/术语与历史包袱。

## 2. 背景与上下文 (Context)

- V4 采用 Greenfield 全新实施（077+），需要在早期冻结“主数据共享/隔离”的权威机制，否则后续各模块会用不同的方式表达同一需求（例如：用 orgunit path、用 tenant 级别全局表、用 hardcode 前缀等）。
- SetID 属于“数据建模/组织治理”的横切能力，主要影响 `jobcatalog/orgunit/staffing` 的主数据读取与配置 UI。

## 3. 目标与非目标 (Goals & Non-Goals)

### 3.1 核心目标

- [ ] 在 V4 引入 SetID 作为“同租户内的主数据数据集”能力：同一编码可在不同 SetID 下并行存在。
- [ ] 引入 **Set Control**：对每个控制值（后续对齐 Business Unit）和每个 Record Group，稳定映射到唯一 SetID（无歧义、可测试）。
- [ ] 为 V4 的主数据表提供一致的建模约束：`tenant_id + setid + business_key + valid_time(date)`。
- [ ] 提供最小管理入口（API + UI）：创建/禁用 SetID、配置 set control value、维护映射矩阵。
- [ ] 将关键约束固化为可执行门禁（tests/gates），避免实现期 drift。

### 3.2 非目标（明确不做）

- 不做跨租户共享 SetID（RLS/tenant 是硬边界，SetID 仅用于同租户内共享/隔离）。
- 不做“多 SetID 合并视图/union”（一次查询只使用一个解析出的 SetID；不引入层级继承或叠加规则）。
- 不做 PeopleSoft 全量 UI 复刻；只保留 V4 需要的最小配置与可验证性。

## 4. 工具链与门禁（SSOT 引用）

> 本计划不复制命令矩阵；触发器与门禁以 SSOT 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`
- V4 模块边界（将影响哪些模块）：`docs/dev-plans/083-greenfield-hr-modules-skeleton.md`
- V4 Tenancy/AuthN 与主体模型：`docs/dev-plans/088-tenant-and-authn-v4.md`
- V4 RLS 强租户隔离：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`
- Valid Time=DATE 口径：`docs/dev-plans/064-effective-date-day-granularity.md`

## 5. 关键决策（ADR 摘要）

### 5.0 5 分钟主流程（叙事）

1) 业务写入时，调用方必须显式提供 `business_unit_id`（Set Control Value）与业务 payload。  
2) 系统按 `(tenant_id, business_unit_id, record_group) -> setid` 解析得到唯一 `setid`（映射无缺省洞）。  
3) 写入的主数据记录必须落库 `setid`；读取列表按解析出的 `setid` 过滤；读取单条以记录自身 `setid` 为准（不重新解析）。  
4) Set Control 映射不做有效期：变更只影响“未来的解析结果”，历史记录因已落库 `setid` 而可复现。  
5) 解析入口必须单点权威（禁止模块自建回退/默认值逻辑），否则门禁阻断。

### 5.1 SetID 的边界（选定）

- **选定**：SetID 是 **tenant 内** 的“数据集选择”机制，不承担租户隔离或权限边界。
- **选定**：所有 setid-controlled 的数据表必须包含 `tenant_id`（RLS）与 `setid`（共享/隔离），且 SetID 不参与 RLS 策略（避免把治理机制误用为安全机制）。

### 5.2 SetID 字段合同（选定：5 字符、全大写）

- **选定**：`setid` 为 `CHAR(5)`（或等价约束），值为 `[A-Z0-9]{1,5}`，存储为全大写。
- **保留字**：`SHARE` 作为每个 tenant 的默认 SetID（不可删除；可用于“无 BU 差异”的主数据）。

### 5.3 Record Group（选定：稳定枚举，禁止运行时自由造组）

> 目的：把“哪些表受 SetID 控制”收敛为可审计的列表，避免模块各自发明分类导致不可验证。

- **选定**：Record Group 为稳定枚举（代码侧 + DB 侧约束），新增 group 必须走 dev-plan 并补齐门禁。
- **V4 MVP group**（可扩展，但必须从最小集合开始）：
  - `jobcatalog`：职位分类主数据（Job Family/Job Profile/Level 等）
  - `orgunit`：组织基础主数据（部门/地点等，按实际建模落地）

### 5.4 Set Control Value（选定：抽象为“控制值”，后续对齐 Business Unit）

- **选定（冻结）**：Set Control Value 即 **Business Unit**，以稳定标识 `business_unit_id` 表达，用于驱动映射：
  - `(tenant_id, business_unit_id, record_group) -> setid`
- **约束**：
  - `business_unit_id` 必须可枚举、可在 UI 中显式选择；禁止在业务代码里“从路径/会话/环境推导”隐式生成。
  - `business_unit_id` 的来源与生命周期由 OrgUnit/租户治理承接（本计划只冻结：写入/读取必须显式携带该字段）。

### 5.5 SetID 解析算法（选定：确定性、无缺省洞）

- **选定**：Set Control 映射必须“无缺省洞”：每个 `(business_unit_id, record_group)` 都有映射（初始化时自动填充为 `SHARE`），从而避免运行时出现“缺映射时怎么办”的分支漂移。

### 5.6 Set Control 映射的时间语义（选定：不做有效期）

- **选定（冻结）**：Set Control 映射不做有效期（不引入 `effective_on/end_on`）。
- **后果（必须接受）**：
  - 映射变更会影响“未来的解析结果”（例如后续创建/读取列表的默认集合）。
  - 历史可复现性必须依赖“业务记录落库 `setid`”这一不变量：任何对单条记录的读取都以记录自身 `setid` 为准。

## 6. 数据契约（Schema/约束级别）

> 本节定义“横切不变量”；具体表名与落点由各模块实现 dev-plan 承接。

### 6.1 基础表（最小集合）

- `setids`：`(tenant_id, setid, name, status, created_at, updated_at)`
- `set_control_values`：`(tenant_id, business_unit_id, name, status, created_at, updated_at)`
- `set_control_mappings`：`(tenant_id, business_unit_id, record_group, setid, created_at, updated_at)`

> 说明：**实施阶段新增表/迁移前必须获得手工确认**（遵循仓库合约），本计划仅冻结契约与字段语义。

### 6.2 主数据表通用约束（所有 setid-controlled 表必须满足）

- 主键/唯一性必须包含：`tenant_id`、`setid`、业务键（如 `code`）、Valid Time（`effective_on/end_on`，date 粒度）。
- `setid` 的值必须存在于 `setids`，且写入时需通过 set control 解析得到（禁止调用方任意填 setid）。

## 7. API 与 UI（最小管理面）

### 7.1 路由归属与命名空间（对齐 DEV-PLAN-094）

- SetID 属于“组织治理/主数据治理”横切配置，但其控制维度是 Business Unit；为避免出现多处 owner，**选定管理面归属 `orgunit`**，内部 API 使用 `/{module}/api/*`：
  - `GET/POST /orgunit/api/setids`
  - `GET/POST /orgunit/api/business-units`（即 set control values）
  - `GET/PUT /orgunit/api/setid-mappings`（批量矩阵更适合 PUT）
- 写请求必须显式携带 `business_unit_id`（禁止从 path/session 推导）。

### 7.2 UI（最小交互）

- SetID 列表：创建/禁用/重命名（禁用需校验是否被映射引用）。
- Set Control Values 列表：创建/禁用（代表 BU 或其他控制维度）。
- 映射矩阵：按 record group 展示一张“控制值 × group -> setid”的矩阵，默认初始化为 `SHARE`。

## 8. 门禁（Routing/Data/Contract Gates）(Checklist)

1. [ ] SetID 合同门禁：`setid` 只能是 1-5 位大写字母/数字；`SHARE` 必存在且不可删除。
2. [ ] 映射完整性门禁：任意启用的 `business_unit_id` 对每个启用的 record group 必须存在映射（无缺省洞）。
3. [ ] 写入口门禁：所有 setid-controlled 写入必须走“解析 setid + 写入”的单一入口（避免绕过映射直接写 setid）。
4. [ ] 引用完整性门禁：`setid-mappings` 不得指向 `disabled setid`；禁用/删除 SetID 时若仍被引用必须阻断。

## 9. 实施步骤 (Checklist)

1. [ ] 明确 record group 的初始清单（`jobcatalog/orgunit`），并在实现模块的 dev-plan 中声明“哪些表受控于哪个 group”。
2. [ ] 实现 SetID 管理的最小 DB/Kernel/Facade（或等价）与 API。
3. [ ] 为主数据写入路径接入 setid 解析（先从 `jobcatalog` 起步，形成样板）。
4. [ ] 落地门禁（tests/gates）并接入 CI required checks。
5. [ ] 补齐 UI 管理面（最小可用），并在后续模块实现中复用同一套解析入口与 contracts。

## 10. 验收标准 (Acceptance Criteria)

- [ ] 同一 tenant 内可配置多个 SetID，并能在同一业务键下并行维护多套主数据（按 SetID 隔离）。
- [ ] 给定 `(business_unit_id, record_group)`，系统能稳定解析出唯一 SetID（无缺省洞）。
- [ ] 任何绕过解析入口直接写 setid 的路径会被门禁阻断。
- [ ] `jobcatalog` 至少一个主数据实体完成端到端接入（解析→写入→读取→UI 展示）。
- [ ] 示例验收：同一 `code` 在 `setid=A0001` 与 `setid=B0001` 并存；BU1 映射到 A0001、BU2 映射到 B0001；两 BU 的列表读取互不串数据；单条读取能通过记录自身 `setid` 精确定位。

## 11. Simple > Easy（DEV-PLAN-045）停止线

- [ ] 任何模块各自实现“SetID 解析/默认值/回退规则”，而不是复用单一权威入口。
- [ ] 允许调用方自由传入 setid 并绕过 set control（会导致不可审计的漂移）。
- [ ] 把 SetID 误用为安全隔离（跨租户/权限）机制，破坏 RLS/Authz 的边界。
