# DEV-PLAN-062：任职记录（Job Data）入口收敛：唯一写入口（底层仍在 Org 模块）

**状态**: 规划中（2025-12-27 00:00 UTC）

> 本计划目标：参考 PeopleSoft 的“组织管理 / 人事事件 / Job Data（任职记录）”边界，将“人员分配/任职维护”的**写入口**收敛为单一入口；底层数据与服务仍归属 `modules/org`。

## 1. 背景与上下文 (Context)
- **现状（入口重复）**：
  - `/org/assignments` 提供“雇用/调动/离职”写入路径。
  - `Person 详情页`内嵌 `/org/assignments/form` 表单与提交按钮，也可完成相同写入。
  - 结果：同一业务能力有两个“可提交表单”的入口，增加维护成本与权限误配风险（尤其是“组织架构管理员 ≠ 任职管理员”的业务常识容易被 UI 入口误导）。
- **PeopleSoft 参考口径**（见 `docs/dev-plans/060-peoplesoft-corehr-menu-reference.md`）：
  - 组织结构维护（Organization Management）与任职事实（Job Data）相对独立；
  - 入转调离属于“人事事件/Action（Effective Dating）”驱动 Job Data 产生新切片；
  - 多视角入口允许存在（从人/从岗/从组织），但**写流程与权威事实只有一处**。
- **Simple > Easy（DEV-PLAN-045）**：
  - “任职写入”应只有一种权威表达与一套失败路径；其它页面只做只读展示与带上下文跳转，避免双入口导致的隐式契约增殖（HTMX/OOB、URL 参数、权限判定分叉）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] **唯一写入口**：任职的创建/调动/离职（Hire/Transfer/Termination）只在一个页面完成（单一可提交表单入口）。
- [ ] **只读可见 + 带上下文跳转**：人员详情页保留任职摘要/时间线（只读），并提供跳转到唯一写入口（预填 `pernr/effective_date`）。
- [ ] **权限语义清晰**：组织架构管理员可维护组织结构，但默认不具备任职写权限；UI 入口与导航不再暗示“Org Admin = Staffing Admin”。
- [ ] **底层不迁移**：数据表、服务、路由处理仍在 `modules/org`（本计划只做 IA/入口与权限收敛）。

### 2.2 非目标
- 不引入完整工作流/审批/回滚引擎（另起 dev-plan）。
- 不调整 `org_assignments/org_personnel_events` 的核心语义与数据结构（除非发现阻断性缺陷，需另起计划并走迁移门禁）。
- 不要求把实现移动到 `modules/person`（明确保持在 `modules/org`）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划预期命中：Go 代码、`.templ`、多语言 JSON、Authz。命令细节以 `AGENTS.md` / `Makefile` / CI 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 边界与术语（对齐 PeopleSoft）
- **人员主数据（Person）**：身份与基本信息（工号/姓名/状态等）。
- **任职记录（Job Data，权威事实）**：人员在某组织/职位上的时间线切片（对应当前实现的 assignment timeline）。
- **人事事件（Action）**：雇用/调动/离职等语义（用于审计/解释），驱动任职记录产生新切片。
- **组织结构维护（Org Structure）**：组织节点、层级、边关系、职位主数据等结构性配置。

## 4. 入口收敛方案（核心）
> “唯一性”定义：**只有一个可提交写入任职的入口**；允许多个页面提供“跳转到该入口”的按钮（带上下文预填），这与 PeopleSoft 的多视角入口一致。
>
> **硬约束（防止第二写入口以 partial/按钮形式回流）**：人员详情页及其嵌入式/summary 渲染（例如 `include_summary=1`）必须始终为**只读**；即使用户具备 `org.assignments:assign`，也只能通过 `/org/assignments` 全页完成写入。

### 4.0 页面变更/新增图示（现状 → 目标）

#### 4.0.1 现状：两个“可提交写表单”的入口
```mermaid
flowchart TD
  %% 现状：同一写能力存在两个入口（Org 分配页 + Person 详情内嵌表单）
  subgraph Person_UI[Person UI（人员）]
    PList[/person/persons 列表/]
    PDetail[/person/persons/{uuid} 详情/]
    PList --> PDetail
    PDetail -->|step=assignment 或点击按钮| PEmbed[内嵌任职表单\n/org/assignments/form?include_summary=1]
  end

  subgraph Org_UI[Org UI（组织/任职）]
    APage[/org/assignments 任职页/]
    AForm[任职表单（页面内）]
    APage --> AForm
  end

  PEmbed -->|POST/PATCH| Write[任职写入\n/org/assignments / /org/assignments/{id}:transition]
  AForm -->|POST/PATCH| Write
  Write --> Facts[(org_assignments 任职事实)]
  Write --> Events[(org_personnel_events 人事事件)]
  Facts --> PDetail
  Facts --> APage
```

#### 4.0.2 目标：唯一写入口（Job Data）+ 其它页面仅跳转/只读
```mermaid
flowchart TD
  %% 目标：写入只在 /org/assignments；Person 详情仅只读展示与跳转
  subgraph Person_UI[Person UI（人员）]
    PCreate[创建人员\nPOST /person/persons]
    PDetail[/person/persons/{uuid} 详情（只读）/]
    PReadonly[当前任职摘要 + 任职经历（只读）]
    PDetail --> PReadonly
  end

  subgraph Org_UI[Org UI（任职记录 / Job Data，唯一写入口）]
    APage[/org/assignments（唯一写入口）/]
    AWrite[任职写表单\n（hire/transfer/termination）]
    APage --> AWrite
  end

  PCreate --> Decision{有 org.assignments:assign?}
  Decision -->|Yes| APage
  Decision -->|No| PDetail
  PDetail -->|打开任职记录（跳转）| APage
  AWrite -->|POST/PATCH| Write[任职写入\n/org/assignments / /org/assignments/{id}:transition]
  Write --> Facts[(org_assignments 任职事实)]
  Write --> Events[(org_personnel_events 人事事件)]
  Facts --> PReadonly
```

#### 4.0.3 页面变更清单（对照表）
| 页面/入口 | 现状 | 062 目标 | 写能力 |
| --- | --- | --- | --- |
| `/org/assignments` | 任职页（含写表单） | **保留并确立为唯一写入口**；UI 文案向“任职记录/Job Data”收敛 | 写 |
| `/person/persons/{uuid}` | 详情页含内嵌任职写表单（`include_summary=1`） | **移除写表单**；保留只读摘要/时间线 + “打开任职记录”跳转 | 只读 |
| 创建人员成功后的引导 | 跳转到 `?step=assignment` 并在 Person 页内写入 | 按权限分流：有 `org.assignments:assign` → 跳 `/org/assignments`；否则留在 Person 页只读提示 | 无 |
| 侧栏（人员管理）/ `任职记录` | 无（仅能通过 Org 页内 Tab 或 Person 详情跳转） | 增加二级入口：`人员管理 → 任职记录` 跳转到 `/org/assignments`（便于从“人事”语义进入，但不引入第二写入口） | 跳转 |
| 组织结构页（`/org/nodes` 等） | 可能提供进入 Assignments 的导航 | 保留“前往任职记录/查看任职”跳转，但不提供可提交写表单 | 跳转 |

### 4.1 唯一写入口（保持在 Org 模块）
- 以 `/org/assignments` 作为任职写入口（后端仍归属 `modules/org`）。
- UI 命名建议：
  - 页面标题从“人员分配”逐步收敛为“任职记录（Job Data）/任职变更”（减少与“组织结构管理”的心理绑定）。
  - 不改变底层 object 名称（仍为 `org.assignments`），但在 UI/导航文案中强化“Staffing/Job Data”语义。

### 4.2 Person 详情页（只读 + 跳转）
- 移除 Person 详情页中的**任何任职写入口**（包括内嵌的 Create/Transition 操作入口、任何对 `/org/assignments/form` 的请求触发，以及 `step=assignment` 自动展开写表单）。
- 保留：
    - 当前部门/职位摘要（只读）
    - 任职经历时间线（只读；summary 模式仅用于 OOB 更新摘要，不得渲染写按钮/写表单）
    - 单一主按钮：`打开任职记录`（跳转到 `/org/assignments?effective_date=...&pernr=...`）
- 兼容性：
    - 对历史链接 `?step=assignment`：不再在 Person 页内打开写表单；按权限分流：
      - 有 `org.assignments:read`：302/HTMX redirect 到 `/org/assignments?effective_date=...&pernr=...&intent=hire`
      - 无 `org.assignments:read`：移除 `step` 并停留在 Person 详情页，展示清晰的“无任职访问权限/去申请权限”提示（避免直接跳转导致 403 困惑）

### 4.3 新建人员后的引导（对齐“先有人，再任职”的现状）
- 创建 Person 成功后不再进入 Person 详情页内嵌任职写表单。
- 建议策略：
    - 若当前用户具备 `org.assignments:assign`：直接跳转到 `/org/assignments?effective_date=...&pernr=...&intent=hire`（预填；`intent` 仅为 UI 聚焦/默认事件类型提示，不作为后端写契约）。
    - 若不具备任职写权限：跳转到 Person 详情页并展示“未分配任职 + 去申请/联系管理员”的明确提示（只读，不尝试进入写入口以免 403 造成困惑）。

### 4.4 Org 侧（结构管理页的入口表述）
- 允许组织/职位页面提供“查看任职（只读）/前往任职记录”的跳转入口，但不提供可提交写表单。
- 导航上建议将“任职记录”从“组织结构管理”的默认子导航中弱化或明确标识为“Staffing”，避免组织管理员误解其权限边界。

## 5. 权限建议（避免“组织架构管理员自动获得任职权限”）
> 本节目标：让 UI 与权限边界一致，避免“为了组织结构管理员只能授 core.superadmin”导致的通配权限外溢。

- 建议角色拆分（示例）：
    - `org.structure.viewer`：覆盖 `org.hierarchies:read`（可选增加 `org.positions:read` 用于结构页展示），不包含任职写能力。
    - `org.structure.editor/admin`：在 `viewer` 基础上增加 `org.nodes:write`、`org.edges:write`（当前代码主要检查 `write`；`admin` 暂作为预留层级）。
    - `org.staffing.viewer/editor/admin`：覆盖 `org.assignments`（read/assign/admin）与任职页展示所需只读对象（如 `org.job_catalog/org.job_profiles`），且不默认包含 `org.nodes/org.edges` 的写权限。
      - 说明：当前实现中 `org.staffing.*` 已包含 `org.positions write/admin`（见 `config/access/policies/org/staffing.csv`）；062 暂不调整该事实，只保证“组织结构写权限”不因 staffing 角色外溢。若要进一步收敛 positions 权限，另起 dev-plan。
- 保持现有 `org.staffing.*` 能力不变（见 `config/access/policies/org/staffing.csv`），新增 `org.structure.*` 用于承接“组织架构管理员”真实角色，避免滥用 `core.superadmin`。

### 5.1 角色 × 对象 × 动作矩阵（建议）
> 本矩阵用于让“导航可见性/页面入口/后端鉴权”对齐同一套契约；实现时以 `config/access/policies/**` 与代码里的 `ensure*Authz*` 检查为准。
>
> 动作约定（口径）：
> - `read`：访问/查看
> - `write`：新增/编辑
> - `admin`：高权限操作（例如删除/全量管理；是否存在以代码检查为准）
> - `assign`：任职写入（Hire/Transfer/Termination 等写动作）

#### 5.1.1 结构角色（`org.structure.*`）
| 对象（Object） | `org.structure.viewer` | `org.structure.editor` | `org.structure.admin` |
| --- | --- | --- | --- |
| `org.hierarchies` | `read` | `read` | `read` |
| `org.nodes` | — | `write` | `write` |
| `org.edges` | — | `write` | `write` |
| `org.positions` | `read`（可选） | `read`（可选） | `read`（可选） |
| `org.assignments` | — | — | — |

#### 5.1.2 任职角色（`org.staffing.*`，现状对齐 `config/access/policies/org/staffing.csv`）
| 对象（Object） | `org.staffing.viewer` | `org.staffing.editor` | `org.staffing.admin` | `org.staffing.reports` | `org.staffing.masterdata.admin` |
| --- | --- | --- | --- | --- | --- |
| `org.assignments` | `read` | `read, assign` | `read, assign, admin` | — | — |
| `org.positions` | `read` | `read, write` | `read, write, admin` | — | — |
| `org.job_catalog` | `read` | `read` | `read` | — | `read, admin` |
| `org.job_profiles` | `read` | `read` | `read` | — | `read, admin` |
| `org.position_reports` | — | — | — | `read` | — |
| `org.position_restrictions` | — | — | — | — | `read, admin` |

## 6. 验收标准 (Acceptance Criteria)
- 入口唯一性：
    - [ ] 任职写操作（创建/调动/离职）只能在 `/org/assignments` 完成。
    - [ ] Person 详情页（包含其 summary/嵌入式渲染）不再出现任职写表单、提交按钮或 Transition 按钮；仅保留跳转。
    - [ ] 即使用户具备 `org.assignments:assign`，在 Person 详情页也不能发起任职写入（必须通过 `/org/assignments` 全页）。
- 权限语义：
    - [ ] 组织架构管理员（structure role）可进入 `/org/nodes` 等结构页，但访问 `/org/assignments` 至少在写操作上被拒绝（read/assign 按策略分别控制）。
    - [ ] 任职管理员（staffing role）可完成任职写入，但不因该角色自动获得组织结构写权限。
- 兼容性：
    - [ ] 历史 URL（如 `?step=assignment`）不会导致用户卡在“第二写入口”，而是被引导到唯一写入口或清晰的无权限提示。
- 测试：
    - [ ] E2E/单测更新覆盖：创建人员后的下一步、只读时间线展示、跳转到任职记录、无任职权限的提示、以及“具备 assign 权限但 Person 页仍只读”的场景。

## 7. 实施步骤（任务清单）
1. [ ] 明确 IA 命名与导航归属：`人员分配` → `任职记录（Job Data）` 的 UI 文案与入口位置（不改底层实现归属）；侧栏在 `人员管理` 下增加 `任职记录` 二级入口指向 `/org/assignments`。
2. [ ] Person 详情页只读化：移除 Create/Transition 写入口与 `/org/assignments/form` 嵌入；删除 `step=assignment` 写流程，改为“只读 + 跳转”，并确保 summary/嵌入式渲染不包含任何写按钮。
3. [ ] 新建人员后重定向策略：按是否具备 `org.assignments:assign` 选择跳转目标与提示文案。
4. [ ] 权限拆分落地：新增 `org.structure.*` policy 与 UI 导航可见性对齐；运行 Authz 门禁并生成聚合 policy（按 `AGENTS.md` 工作流）。
5. [ ] 测试与门禁：补齐/更新 E2E 用例并按触发器矩阵执行本地校验；必要时新增 readiness 记录。

## 8. 风险与回滚
- 风险：用户习惯在 Person 详情页直接“补齐任职”，移除后可能增加一次跳转。
    - 缓解：跳转按钮默认可见且预填 `pernr/effective_date`，并支持 `intent=hire` 自动聚焦表单。
- 风险：部分用户“可建人但无任职权限”，创建后若直接跳唯一写入口会遇到 403。
    - 缓解：创建后按权限分流；无权限时留在 Person 详情页展示明确提示与权限申请入口。
- 回滚：保留 `step=assignment` 的兼容重定向（而非恢复第二写入口），必要时可临时在配置层加开关恢复旧 UX（需另起小计划说明退场策略）。
