# DEV-PLAN-083：Greenfield HR 模块骨架与契约（OrgUnit/JobCatalog/Staffing/Person）

**状态**: 草拟中（2026-01-05 03:38 UTC）

## 1. 背景与上下文 (Context)

当前仓库的 HR 相关能力在结构上呈现“UI/Schema 集中、人员单独拆出”的形态：
- `modules/org` 同时承载：组织架构（OrgUnit）、职位（Position）、职位分类（Job Catalog）、任职记录（Assignment/Job Data），并在同一个 UI controller 下注册路由（`/org/nodes`、`/org/positions`、`/org/job-catalog`、`/org/assignments`）。
- `modules/person` 承载：人员管理（Person），但 UI 会链接/嵌入任职页面（跨模块 UI 组合）。
- 持久化层存在跨域耦合点：Org 的 repo 会直接查询 `persons` 表做 pernr→person_uuid 解析。

结合 `DEV-PLAN-082`（DDD 分层框架）与 `DEV-PLAN-077`～`DEV-PLAN-080`（v4：DB Kernel + Go Facade + One Door Policy），本计划以 **Greenfield（从 0 开始）** 口径提出新的 `modules/*` 骨架与跨模块契约，以降低边界漂移风险并提升可演化性。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标
- [ ] 为组织架构、职位管理、职位分类、任职记录、人员管理给出清晰的 bounded context 与模块归属（“谁拥有数据与写入口”）。
- [ ] 给出每个模块的最小目录骨架（对齐 `.gocleanarch.yml` 四层），并明确“DB Kernel + Go Facade”在模块内的落点。
- [ ] 定义跨模块交互的最小契约：只依赖 `pkg/**` 共享类型；跨模块 UI 用 HTMX/HTTP 组合；避免 Go 代码跨模块 import。
- [ ] 明确关键跨域不变量的归属与裁决策略（防止出现两套权威表达）。

### 2.2 非目标（明确不做）
- 不在本计划内做存量代码迁移、旧模块拆分、数据迁移脚本与回滚路径（另立 dev-plan 承接）。
- 不在本计划内新增数据库表或提交迁移；本文只冻结“模块边界与契约”。

## 2.3 工具链与门禁（SSOT 引用）
- 分层与依赖门禁：`.gocleanarch.yml`（入口：`make check lint`）
- DDD 分层框架（Greenfield）：`docs/dev-plans/082-ddd-layering-framework.md`
- v4 DB Kernel 边界 SSOT：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`
- 工具链入口：`Makefile`、CI：`.github/workflows/quality-gates.yml`

## 3. Bounded Context 划分（建议）

### 3.1 目标模块（已采纳：4 模块，降低跨模块不变量成本）

> 理由：Position 与 Assignment 之间存在强跨聚合不变量（例如容量/FTE 约束、停用约束等；见 079 的跨聚合不变量），若拆成两个模块，会把“最终裁判”拆散，容易形成双权威表达；因此推荐把它们收敛到同一 bounded context。

- `modules/orgunit`：组织单元树（OrgUnit）
- `modules/jobcatalog`：职位分类主数据（Job Catalog：Job Family Group/Family/Level/Profile）
- `modules/staffing`：用工/任职（Staffing：Position + Assignment/Job Data）
- `modules/person`：人员（Person Identity）

## 4. 数据所有权与写入口（One Door Policy）

### 4.1 数据所有权（Who owns the write side）

推荐口径（与 077-080 同构）：
- 每个业务模块拥有自己的 **Write Side（events）** 与 **Read Model（versions）**，并通过 `submit_*_event` 作为唯一写入口。
- `*_versions` 为衍生数据，可丢弃重建；`*_events` 为 SoT（append-only）。

模块所有权建议：
- `orgunit`：org unit 事件与 versions（树与路径/长名等投射）
- `jobcatalog`：各 job_* 实体的事件与 versions（effective-dated attributes）
- `staffing`：positions/assignments 的事件与 versions（含跨聚合不变量裁决）
- `person`：persons（身份锚点；可选是否采用事件化，本文不强制）

### 4.2 跨模块引用的最小契约（避免 Go import）

- **ID 类型**：跨模块只通过 `uuid`（或 `pkg/**` 的强类型别名）传递 ID，不跨模块 import 领域对象。
- **存在性校验**：
  - `staffing` 引用 `person`：写路径输入统一使用 `person_uuid`（而非 pernr）；pernr→uuid 的解析应由 `person` 模块提供 options/read API（UI 通过 HTMX/HTTP 获取），避免 `staffing` 直接查询 `persons` 表形成隐式耦合。
  - Person Identity 最小合同与 pernr 约束（1-8 位数字字符串）见：`docs/dev-plans/085-person-minimal-identity-for-staffing.md`
  - `staffing` 引用 `jobcatalog`：建议以 `job_profile_id/job_level_id/...` 作为输入，并在 write side 记录必要的 label 快照（避免“改字典=改历史”与跨域查询耦合；对齐 080 的动机）。
  - `staffing` 引用 `orgunit`：建议以 `org_unit_id` 作为输入；组织路径/长名等展示型数据通过 `pkg/orglabels` 或独立 read API 提供（不把投射逻辑复制到 staffing）。

## 5. 路由与 UI 组合（跨模块以 HTMX/HTTP 拼装）

### 5.1 路由归属（建议保持人机入口稳定）
- Org 相关 UI：
  - `orgunit`：`/org/nodes`（组织架构）
  - `staffing`：`/org/positions`、`/org/assignments`（职位/任职）
  - `jobcatalog`：`/org/job-catalog`（职位分类）
- Person UI：
  - `person`：`/person/persons`

### 5.2 Person 详情页的任职时间线（组合方式）
`person` 模块不应依赖 `staffing` 的 Go 包；通过 HTMX 请求 `staffing` 暴露的 partial（例如 `/org/assignments?...&include_summary=1`）实现 UI 组合（与当前模式一致，但模块边界更清晰）。

## 6. `modules/*` 目录骨架（推荐模板）

> 以 `DEV-PLAN-082` 的“形态 B：DB Kernel + Go Facade”为默认模板；若某模块短期不需要 Kernel，可按形态 A 收敛。

### 6.1 通用骨架（每个模块必须）
```
modules/<module>/
  domain/
  services/
  infrastructure/
  presentation/
  module.go
  links.go
```

### 6.2 DB Kernel 模块骨架（orgunit/jobcatalog/staffing 推荐）
```
modules/<module>/
  domain/ports/                       # Kernel Port 接口
  domain/types/                       # IDs/枚举/稳定错误码（可选）
  services/                           # Facade：Tx + 调 Kernel + serrors 映射
  infrastructure/persistence/         # pgx/sqlc adapter（实现 ports）
  infrastructure/persistence/schema/  # Kernel schema/函数/约束（SSOT）
  presentation/controllers/
  presentation/templates/
  presentation/locales/
  module.go
  links.go
```

### 6.3 Person 模块骨架（保持现有风格）
```
modules/person/
  domain/aggregates/person/
  services/
  infrastructure/persistence/schema/
  presentation/controllers/
  presentation/templates/
  presentation/locales/
  module.go
  links.go
```

## 7. `pkg/**` 共享包建议（支撑拆模块）

> 目标：共享“横切能力与稳定类型”，避免跨模块 import（对齐 082 的 `pkg/**` 准入规则）。

- `pkg/validtime`：Valid Time（date）相关工具与类型（对齐 064）
- `pkg/hrids`（建议新增）：跨模块 ID 强类型（PersonID/OrgUnitID/PositionID/JobProfileID…）
- `pkg/serrors`、`pkg/htmx`、`pkg/orglabels`：继续作为跨模块复用基础设施

## 8. 停止线（按 DEV-PLAN-045）
- [ ] 任一模块出现第二写入口（绕过 `submit_*_event`/Kernel 入口直写 SoT/versions/identity）。
- [ ] 为复用实现引入 Go 层跨模块 import（应先下沉 `pkg/**` 或定义 ports）。
- [ ] `pkg/**` 引入对 `modules/**` 的依赖（禁止）。

## 9. 交付物
- [ ] 本文档冻结目标模块划分与每个模块的骨架模板。
- [ ] 后续每个模块的实现 dev-plan 必须引用本计划，并声明其 Kernel 边界与唯一写入口（One Door Policy）如何落地。

## 10. Simple > Easy Review（DEV-PLAN-045）

### 10.1 结构（解耦/边界）
- 通过：bounded context 与模块数量收敛为 4（`orgunit/jobcatalog/staffing/person`），把 Position↔Assignment 强不变量收敛在 `staffing`，避免跨模块“双权威表达”。
- 通过：跨模块交互口径收敛为 `pkg/**` 类型 + HTTP/HTMX 组合，避免 Go import 形成隐式耦合。
- 需警惕：`staffing` 容易演化为“万能模块”；必须通过 ports/目录与 One Door Policy 维持边界（本计划已给出停止线，实施时要严格执行）。

### 10.2 演化（规格驱动/确定性）
- 通过：已冻结“采用 4 模块”的关键决策，避免实现期反复拉扯。
- 待补齐（由各子域 dev-plan 承接）：每个模块的 schema SSOT 路径、`submit_*_event` 函数签名与错误契约（SQLSTATE/constraint/stable code）必须在实现计划中明确（对齐 077-080）。

### 10.3 认知（本质/偶然复杂度）
- 通过：把跨聚合不变量（capacity/停用/有效期一致性等）明确归属到 `staffing` 的 DB Kernel 裁决，属于本质复杂度，不在应用层“缝补”。
- 需警惕：`pkg/**` 共享包若无准入规则会退化为“万能抽屉”；实施必须遵守 `DEV-PLAN-082` 的 `pkg/**` 准入与停止线。

### 10.4 维护（可理解/可解释）
- 通过：每个模块都给出最小骨架模板，reviewer 可用路径快速判断“代码是否放对层”。
- 待补齐（由各子域 dev-plan 承接）：为每个模块提供 1 条“端到端叙事”（controller → facade → kernel port → submit_*_event → 错误映射），作为验收材料，确保 5 分钟可复述。
