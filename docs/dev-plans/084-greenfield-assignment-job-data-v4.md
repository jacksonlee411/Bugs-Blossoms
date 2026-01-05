# DEV-PLAN-084：任职记录（Job Data / Assignments）v4 全新实现（Staffing，事件 SoT + 同步投射）

**状态**: 草拟中（2026-01-05 03:38 UTC）

## 1. 背景与上下文 (Context)

本计划目标是**全新实现**“任职记录（Job Data / Assignments）”能力（Greenfield），并对齐：
- v4 技术路线（`DEV-PLAN-077`～`DEV-PLAN-080`）：**DB=Projection Kernel（权威）**、同步投射（同事务 delete+replay）、One Door Policy（唯一写入口）、Valid Time=DATE、`daterange` 统一使用左闭右开 `[start,end)`。
- DDD/分层与模块骨架（`DEV-PLAN-082`、`DEV-PLAN-083`）：采用 4 模块（`orgunit/jobcatalog/staffing/person`），其中任职记录归属 `modules/staffing`。

同时，本仓库当前在 `modules/org` 内已存在一套“任职记录”实现（非 v4 事件 SoT 模式），包含 UI、API、DB 表与若干约束/运维手段；本计划需要先**完整登记现有功能**，再定义 v4 方案如何承接（保留/替代/显式不做）。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标
- [ ] 在 `modules/staffing` 内全新实现任职记录 v4：**事件 SoT（`*_events`）+ 同步投射（`*_versions`）**，并由 DB Kernel 强制关键不变量。
- [ ] UI/列表展示层：任职记录**仅显示生效日期（effective date）**；底层仍使用 `daterange` 的左闭右开 `[start,end)` 表达有效期（不改为闭区间）。
- [ ] 以清晰契约替代“隐式耦合”：
  - 写路径输入统一使用 `person_uuid`（对齐 083），不再以 pernr 作为写侧主键；
  - person 的 pernr→uuid 解析由 `modules/person` 提供 options/read API，`staffing` 不直读 `persons` 表（Person Identity 合同见 `DEV-PLAN-085`）。
- [ ] 完整登记当前任职记录已实现功能，并逐项给出 v4 方案的落地方式（或明确不做）。
- [ ] 实现需满足 082/083 的 DDD 分层与 One Door Policy：Go=Facade（鉴权/事务/调用/错误映射），DB=Kernel（裁决/投射/重放）。

### 2.2 非目标（明确不做）
- 不做存量 `modules/org` 的迁移/替换 cutover（`DEV-PLAN-078` 类交付不在本计划范围内）。
- 不引入 `effseq`，同一实体同日最多一条事件（对齐 077/079/080）。
- 不在本计划内实现“跨模块异步事件/旧 outbox/audit/settings 支撑能力”的兼容；如需要另立计划。

## 2.3 工具链与门禁（SSOT 引用）
- DDD 分层框架（Greenfield）：`docs/dev-plans/082-ddd-layering-framework.md`
- Greenfield HR 模块骨架（4 模块）：`docs/dev-plans/083-greenfield-hr-modules-skeleton.md`
- v4 Kernel 边界与 daterange 口径：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/064-effective-date-day-granularity.md`
- 分层/依赖门禁：`.gocleanarch.yml`（入口：`make check lint`）
- 命令入口与 CI：`Makefile`、`.github/workflows/quality-gates.yml`

## 3. 现状功能盘点（已实现的任职记录能力，存量参考）

> 本节仅登记事实（当前系统已有的能力与约束），不代表 v4 一定保留；v4 方案见 §6。

### 3.1 路由与交互（UI / HTMX）
- 页面与表单（`modules/org/presentation/controllers/org_ui_controller.go`）：
  - `GET /org/assignments`：任职列表/时间线（支持 `effective_date`、`pernr`、`include_summary` 等组合）。
  - `GET /org/assignments/form`：新建任职表单（HTMX）。
  - `POST /org/assignments`：创建任职。
  - `GET /org/assignments/{id}/edit` + `PATCH /org/assignments/{id}`：编辑任职。
  - `GET /org/assignments/{id}/transition` + `POST /org/assignments/{id}:transition`：任职“转移/终止”流程（见 §3.3）。
- 人员详情页嵌入任职时间线（`modules/person/presentation/templates/pages/persons/detail.templ`）：
  - 通过 HTMX 请求 `/org/assignments` 与 `/org/assignments/form`，并以 `pernr` 作为筛选参数。

### 3.2 API（JSON）
- `modules/org/presentation/controllers/org_api_controller.go`：
  - `GET /org/api/assignments`（list）
  - `POST /org/api/assignments`（create）
  - `PATCH /org/api/assignments/{id}`（update）
  - `POST /org/api/assignments/{id}:correct`（correct）
  - `POST /org/api/assignments/{id}:rescind`（rescind）
  - `POST /org/api/assignments/{id}:delete-slice`（delete+stitch）

### 3.3 业务动作/能力（services）
- 创建任职：`modules/org/services/org_service.go:1108`
  - 输入：`pernr`、`effective_date`、`assignment_type`（`primary/matrix/dotted`）、`allocated_fte`、`position_id` 或 `org_node_id`（自动建岗）
  - 行为要点：
    - pernr→person_uuid 解析（目前由 org repo 直接查 `persons` 表）
    - primary 冲突检测（时间窗口重叠）
    - “再入职（rehire）”场景：若当前 primary 片段为 inactive，则在新生效日截断旧 end_date
    - 可选“自动建岗”（auto position），并依赖 Job Catalog 至少存在一个 job_profile
- 更新任职：`modules/org/services/org_service.go:1476`
  - 创建新 slice（新 id）并截断旧 slice，写审计并发 outbox（`assignment.updated`）
- 就地更正：`modules/org/services/org_service_025.go:727`
  - 允许修正 pernr/subject_id/position_id 等字段（in-place patch）
- 撤销（rescind）：`modules/org/services/org_service_025.go:904`
  - 在指定 `effective_date` 进行撤销/截断/补 inactive 等（实现较复杂，需在 v4 里明确取舍）
- 删除切片并缝补（delete+stitch）：`modules/org/services/org_service_066.go:614`
  - 仅支持 primary timeline；删除某 slice，并把前一片段 end_date 延长以保持 gap-free
- 转移/终止（transition）：`modules/org/services/assignment_transition_061a1.go:30`
  - 事件类型：`transfer`/`termination`
  - 依赖 `org_personnel_events` 的 upsert，并在同事务中触发 update/rescind
- 约束与错误映射：
  - primary 重叠：由 DB exclusion 或服务层判定，映射为 `ORG_PRIMARY_CONFLICT`（`modules/org/services/pg_errors.go:47`）
  - gap-free（primary）：DB 端 constraint trigger `org_assignments_gap_free`（`modules/org/infrastructure/persistence/schema/org-schema.sql:1183`）

### 3.4 数据与查询（DB）
- 表：`org_assignments`（`modules/org/infrastructure/persistence/schema/org-schema.sql:653`）
  - 字段：`position_id`、`subject_id`（uuid）、`pernr`、`assignment_type`、`allocated_fte`、`employment_status`、`effective_date`、`end_date` 等
  - 约束：
    - primary 唯一性（时间窗口排他，EXCLUDE）
    - subject/position no-overlap（EXCLUDE）
    - primary gap-free（commit-time constraint trigger，仅对 primary）
- 读模型查询：
  - timeline 查询会 join Position/Org/Job Catalog 多表，并附带 personnel_event 的起止事件类型（`modules/org/infrastructure/persistence/org_crud_repository.go:786` 起）

## 4. 核心设计约束（v4 合同，必须遵守）

### 4.1 Valid Time 与 `daterange` 口径（强制）
- Valid Time 粒度：`date`（对齐 `DEV-PLAN-064`）。
- 所有有效期区间：使用 `daterange` 且统一左闭右开 `[start,end)`。
- 展示层：任职记录**仅展示 `effective_date`**（即 `lower(validity)`），不展示 `end_date`；避免把 `[start,end)` 再转回闭区间造成语义混乱。

### 4.2 One Door Policy（写入口唯一）
- 应用层不得直写任一 SoT/versions/identity 表；只能调用 DB Kernel 的 `submit_*_event(...)`。
- Kernel 内部函数（如 `apply_*_logic`）不得被应用角色直接执行。

## 5. 目标架构（modules/staffing，DB Kernel + Go Facade）

### 5.1 模块归属
任职记录 v4 归属 `modules/staffing`（对齐 `DEV-PLAN-083`：Position+Assignment 收敛以承载跨聚合不变量）。

### 5.2 目录骨架（对齐 082/083）
```
modules/staffing/
  domain/ports/                        # AssignmentKernelPort（与 PositionKernelPort 同模块）
  domain/types/                        # 稳定枚举/错误码/输入 DTO（可选）
  services/                            # Facade：Tx + 调 Kernel + serrors 映射
  infrastructure/persistence/          # pgx adapter（调用 submit_*_event）
  infrastructure/persistence/schema/   # staffing-schema.sql（SSOT，含 assignment v4）
  presentation/controllers/            # /org/assignments（UI）与 /org/api/assignments（API）
  presentation/templates/
  presentation/locales/
  module.go
  links.go
```

### 5.3 Kernel 边界（与 077-080 同构）
- **DB = Projection Kernel（权威）**：插入事件（幂等）→ 同事务全量重放生成 versions → 裁决不变量。
- **Go = Command Facade**：鉴权/事务边界 + 调 Kernel + 错误映射到 `pkg/serrors`。

## 6. v4 方案（新实现方式：事件 SoT + 同步投射）

### 6.1 领域建模：以“时间线聚合（timeline aggregate）”作为写侧单位

本计划将“任职记录”建模为 `person_uuid + assignment_type` 的时间线聚合（至少覆盖 `primary`；扩展类型可选）：
- 聚合标识：`assignment_timeline_id`（可用 uuid，或从 `tenant_id+person_uuid+assignment_type` 派生稳定 uuid）
- 时间线内的每个有效片段（version slice）记录岗位、组织、FTE、就业状态等业务属性。

优势：
- 与“同日最多一条事件/无 overlap/gapless/最后一段无穷”天然对齐。
- 转移/终止/再入职等都成为“时间线上的事件”，由 Kernel 统一裁决。

### 6.2 数据模型（草案；DDL 以实施时 schema SSOT 为准）

> 注意：本节仅为目标合同草案，不在本计划内落盘；新增表/迁移须另开实施计划并获得手工确认（见仓库约束）。

**Write Side（SoT）**
- `assignment_timeline_events`
  - `tenant_id uuid`
  - `event_id uuid`（幂等键，unique）
  - `timeline_id uuid`
  - `event_type text`（例如 `CREATE/UPDATE/TRANSFER/TERMINATE/CORRECT/RESCIND`，以 v4 合同冻结）
  - `effective_date date`（同日唯一：`(tenant_id,timeline_id,effective_date)` unique）
  - `payload jsonb`（变化字段、reason_code/note 等）
  - `request_id text`、`initiator_id uuid`、`transaction_time timestamptz`

**Read Model（Projection）**
- `assignment_timeline_versions`
  - `tenant_id uuid`
  - `timeline_id uuid`
  - `validity daterange`（`[start,end)`，最后一段 `upper_inf=true`）
  - `position_id uuid`（Staffing 内部实体/引用）
  - `org_unit_id uuid`（引用 orgunit）
  - `allocated_fte numeric`
  - `employment_status text`（`active/inactive/...`）
  - `meta jsonb`（可选：保存必要的 label 快照）
  - 约束：
    - `EXCLUDE USING gist (tenant_id, timeline_id, validity &&)`（no-overlap）
    - gapless（commit-time gate）：相邻切片严丝合缝，最后一段 infinity

### 6.3 Kernel 入口（唯一写入口）
- `submit_assignment_timeline_event(...)`：
  - 幂等：同 `event_id` 重试不重复写入
  - 插入事件后：`DELETE FROM assignment_timeline_versions WHERE tenant_id=? AND timeline_id=?`，再基于 events 全量重放
  - 同事务内执行 `validate_assignment_timeline_*`（gapless/no-overlap/跨聚合不变量等）

### 6.4 只展示生效日期（UI/读接口形状）
- Timeline 列表行只渲染：
  - `effective_date = lower(validity)`
  - 以及岗位/组织/职类等**业务属性**（允许显示）
  - 不展示 `end_date`/`upper(validity)`（避免闭区间混用）
- as-of 查询仍使用 `validity @> $as_of::date` 保证语义一致。

## 7. 功能映射：存量能力 → v4 方案

> 本节把 §3 的存量能力逐项映射到 v4 的实现方式（保留/替代/不做）。

1) 创建任职（CreateAssignment）
- v4：通过 `submit_assignment_timeline_event(event_type='CREATE', effective_date=...)` 创建/更新时间线的第一个版本或新版本。
- 输入主键：使用 `person_uuid`（pernr 仅用于 UI 查询与展示，不进入写侧合同）。

2) 更新任职（UpdateAssignment）
- v4：统一为事件写入（例如 `event_type='UPDATE'`），由 Kernel 决定切片 split/截断。
- Go 不再“先查当前 slice 再补丁式截断”；避免第二套时间线算法。

3) Correct（就地更正）
- v4：定义为 `event_type='CORRECT'`，但仍遵循“同日唯一”规则；若需要同日多次修正，必须提升为不同的业务事件（本计划不引入 effseq）。

4) Rescind（撤销）
- v4：定义为 `event_type='RESCIND'`，由 Kernel 将某日之后的状态切片化为 `inactive` 或回滚到前态；具体语义需在子域实现计划中冻结（避免隐式复杂分支）。

5) Delete slice + stitch
- v4：不暴露“直接删 versions”的能力；如需“删除某日变更”，通过 `RESCIND` 或 `CORRECT` 事件表达（One Door Policy）。

6) Transfer / Termination（transition）
- v4：不再依赖独立的 `org_personnel_events` 表作为裁决入口；转移/终止应成为 assignment timeline 的事件类型（或成为 staffing 内的人员事件，但必须同构为 v4 事件 SoT）。

7) Primary gap-free / no-overlap
- v4：以 versions 的 `daterange [)` + DB gate 强制；展示层只用 `effective_date`。

8) 与 Position/OrgUnit/Job Catalog 的 join 与显示
- v4：读侧允许 join，但写侧必须避免跨模块“隐式查询”：
  - orgunit/jobcatalog 的 label 建议走 read API 或 `pkg/orglabels` 类共享投射能力；
  - 如确需快照，为保证历史一致性，可把必要 label 写入 versions 的 `meta`（需在实现计划中明确范围，避免无限膨胀）。

## 8. 里程碑与验收（Plan → Implement 的承接）

1. [ ] 冻结 v4 的事件类型枚举、payload 合同、错误契约（SQLSTATE/constraint/stable code）。
2. [ ] 冻结 `modules/staffing` 的路由（UI+API）与输入输出契约（只展示 effective_date）。
3. [ ] 冻结 DB Kernel：`submit_*_event` + `replay_*_versions` + `validate_*` 的职责矩阵（对齐 077-080）。
4. [ ] 定义最小测试集：
   - 同日唯一
   - no-overlap
   - gapless（最后一段 infinity）
   - 转移/终止/撤销（失败路径可解释）
5. [ ] 通过相关门禁（引用 `AGENTS.md` 触发器矩阵）。
