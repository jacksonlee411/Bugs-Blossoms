# DEV-PLAN-086：引入 Astro（AHA Stack）到 HTMX + Alpine 的 HRMS v4 UI 方案（077-084）

**状态**: 草拟中（2026-01-05 04:18 UTC）

## 1. 背景与上下文 (Context)

仓库当前 UI 技术栈为 **Templ + HTMX + Alpine.js + Tailwind CSS**（见 `docs/ARCHITECTURE.md`）。与此同时，`DEV-PLAN-077`～`DEV-PLAN-084` 定义了 HRMS v4 的核心模块与契约：
- v4 内核边界：DB=Projection Kernel（权威），Go=Command Facade（编排），One Door Policy（唯一写入口）（见 077/079/080/084）。
- Valid Time=DATE，读模型使用 `daterange` 且统一 `[start,end)`（左闭右开）（见 077/079/080/084、以及 064）。
- UI 合同：任职记录（Job Data / Assignments）**仅显示 `effective_date`**（不展示 `end_date`），但底层继续沿用 `daterange [start,end)`（见 084）。
- Greenfield 模块划分（仅实现 077-084 所覆盖模块）：`orgunit/jobcatalog/staffing/person`（见 083）。

本计划目标是在不改变“HTMX + Alpine 的交互范式”的前提下，引入 **Astro（AHA Stack：Astro + HTMX + Alpine）**，用于：
- 统一页面壳（shell）、导航与信息架构（IA）
- 统一 UI 组件、视觉规范与布局系统（Design System）
- 降低页面级模板复杂度与重复，提高可复用性

### 1.1 适用范围：全新代码仓库（Greenfield）
本计划按“另起一个全新代码仓库”口径制定：
- 不存在旧 UI 的“迁移/兼容/回退到旧页面”问题；
- 回退策略应基于**发布版本回滚**（上一版构建产物/上一版镜像），而非在同一路由下并存两套实现。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标
- [ ] 只实现 077-084 的 4 个模块的 UI：`OrgUnit`、`Job Catalog`、`Staffing`、`Person`；左侧导航栏布局与目前一致。
- [ ] 模块为一级菜单；模块下子模块为二级菜单；不引入更多层级。
- [ ] 在 HTMX + Alpine 的基础上引入 Astro：Astro 负责页面壳/组件编译与静态资源组织；交互仍以 HTMX 为主、Alpine 为辅。
- [ ] 明确 i18n、Authz、as-of（有效日期）等全局 UI 能力在新架构下的边界与集成方式。
- [ ] 给出可执行的落地步骤与验收标准（避免 “Easy but not Simple”）。

### 2.2 非目标（明确不做）
- 不在本计划内替换 DB Kernel/领域实现（077-084 的后端契约不在此计划内变更）。
- 不在本计划内把系统改成 SPA；不引入前端状态管理框架（React/Vue/Redux 等）。
- 不在本计划内把系统改造成“前端渲染为主”；业务 HTML 仍以服务端渲染 + HTMX swap 为主。

## 2.3 工具链与门禁（SSOT 引用）
- DDD 分层框架：`docs/dev-plans/082-ddd-layering-framework.md`
- HR 模块骨架（4 模块）：`docs/dev-plans/083-greenfield-hr-modules-skeleton.md`
- 任职记录 v4 UI 合同（仅显示 effective_date、保持 `[start,end)`）：`docs/dev-plans/084-greenfield-assignment-job-data-v4.md`
- 分层/依赖门禁：`.gocleanarch.yml`（入口：`make check lint`）
- 样式与生成入口：`Makefile`（Tailwind/生成物以 SSOT 为准）
- 文档门禁：`make check doc`

## 3. 信息架构（IA）与导航（左侧布局不变）

### 3.1 一级菜单（模块）
仅提供 4 个一级模块（与 083 对齐）：
- `OrgUnit`（组织架构）
- `Job Catalog`（职位分类）
- `Staffing`（职位 + 任职）
- `Person`（人员）

### 3.2 二级菜单（子模块）
二级菜单以“页面入口”维度定义，且不跨模块混放：

**OrgUnit**
- 组织结构（Tree）：`/org/nodes`
- 组织节点详情（Panel/Details）：`/org/nodes/{id}`（可复用现有信息架构）

**Job Catalog**
- 职类组（Job Family Groups）：`/org/job-catalog#family-groups`
- 职类（Job Families）：`/org/job-catalog#families`
- 职级（Job Levels）：`/org/job-catalog#levels`
- 职位模板（Job Profiles）：`/org/job-catalog#profiles`

**Staffing**
- 职位（Positions）：`/org/positions`
- 任职记录（Job Data / Assignments）：`/org/assignments`

**Person**
- 人员列表（Persons）：`/person/persons`
- 人员详情（Person Details）：`/person/persons/{person_uuid}`

> 说明：路由沿用 083 的“人机入口稳定”建议（仍使用 `/org/*` 与 `/person/*`），但导航以“模块”维度组织，解决当前“Person 里挂 org/assignments”造成的 IA 混乱。

### 3.3 全局 as-of（有效日期）交互（统一入口）
为对齐 v4 的 valid-time 语义，页面壳提供一个全局 “As-of 日期”控件：
- 作为全局 query 参数（例如 `?as_of=YYYY-MM-DD`）或隐藏字段由 HTMX `hx-include` 统一携带。
- 对任职记录页面：仅展示 `effective_date`（即 `lower(validity)`）；as-of 用于筛选/定位快照，不引入 `end_date` 展示（对齐 084）。

## 4. UI 技术架构：Astro + HTMX + Alpine（AHA）

### 4.1 总体原则
- **Astro = 壳与组件编译层**：负责 layout/导航组件/页面框架/静态资源与 build pipeline；尽量不承载业务数据渲染。
- **HTMX = 业务交互与数据驱动渲染**：业务页面内容以 server-rendered HTML partial 为主，依赖 `hx-get/hx-post/hx-target` 做局部刷新。
- **Alpine = 局部状态与微交互**：导航折叠、快捷键、弹窗、表单局部校验提示等；不做跨页面状态管理。

### 4.2 “壳（Shell）”与“内容（Content）”分离
引入一个统一的 App Shell：
- 左侧导航（与现有布局一致）
- 顶部栏（As-of 日期、搜索入口、用户/租户/语言）
- 主内容区（由 HTMX 拉取模块内容并 swap）

核心约束：**Shell 负责结构与导航，Content 负责业务**。Shell 允许是 Astro 产物；Content 仍可由 Go（Templ/handlers）渲染，逐步迁移不强制一次到位。

### 4.3 Authz / i18n 集成方式（不把动态信息固化到 Astro）
为避免“静态壳无法感知用户权限/语言”的矛盾：
- 导航与页面标题的最终渲染仍由服务端输出 HTML（可复用现有本地化与权限判定），Astro 壳只提供容器与样式。
- Astro 壳在加载时通过 HTMX 拉取：
  - `/ui/nav`：当前用户可见的导航 HTML（含二级菜单）
  - `/ui/topbar`：包含 As-of 控件与用户信息
  - `/ui/flash`：统一错误/成功提示（对齐 043 类 UI 反馈规范时可复用）

> 这保持了“权威表达在服务端”的简单性：权限/语言不在前端复制一套判断逻辑。

## 5. 页面与组件规范（只覆盖 077-084 模块）

### 5.1 通用页面框架
- 主内容区统一：`PageHeader`（标题+说明+操作按钮） + `AsOfBar`（如该页需要） + `ContentPanel`（列表/表单/详情）。
- 所有列表页：
  - 支持 `as_of`（可选）与 `q`（搜索，可选）
  - 列表行点击打开右侧/下方详情面板（HTMX swap），减少全页跳转

### 5.2 任职记录（Assignments）v4 UI 合同落地
对齐 084 的强约束：
- 列表/时间线**只显示 `effective_date`**（生效日期），不显示 `end_date`。
- 时间线分段用 “事件/动作”标识（CREATE/UPDATE/TRANSFER/TERMINATE…），但不引入 `effseq`。
- 任何“删除某日变更”类操作不允许直接操作 versions；必须走事件入口（One Door Policy 对齐 077-080/084）。

### 5.3 Job Catalog / Positions / OrgUnit 的一致性体验
统一约定：
- 所有有效期类对象：同样用 as-of 控制当前视图，不在 UI 混入 end_date。
- 所有“选项下拉”（组织节点、职位、职位模板等）：统一使用 HTMX options endpoint + 输入搜索（避免把大字典塞到前端）。

## 6. 落地步骤（可执行）

### Phase 0：先打通 AHA 基础链路（最小可运行）
1. [ ] 新仓库新增 `apps/web`（或 `ui/astro`）作为 Astro 工程，建立 Tailwind 与 Alpine/HTMX 的资源打包入口。
2. [ ] 定义 App Shell（Astro）：包含 `<aside>` 导航容器与 `<main>` 内容容器，预留 HTMX `hx-get/hx-trigger` 挂载点。
3. [ ] 后端提供最小 UI partial：`/ui/nav`、`/ui/topbar`、`/ui/flash` 与一个占位内容页（例如 `/app/home`），验证：
   - 静态资源可用（Astro build 产物）
   - HTMX swap 正常
   - Alpine 初始化不与 HTMX 冲突

### Phase 1：按模块逐个接入内容（不改 Shell）
4. [ ] 按 083 的 4 模块顺序接入页面与二级菜单入口（未实现模块不出现在导航中）：
   - OrgUnit（`/org/nodes`）
   - JobCatalog（`/org/job-catalog`）
   - Staffing（`/org/positions`、`/org/assignments`）
   - Person（`/person/persons`，并通过 HTMX 组合 Staffing 的任职时间线）
5. [ ] 导航 SSOT：二级菜单定义仍由各模块 `links.go`（或等价结构）提供，服务端聚合并按 Authz/i18n 渲染 `/ui/nav`，Astro 不维护第二份导航规则。

### Phase 2：硬化与验收（对齐 077-084 契约）
6. [ ] 全局 as-of 透传策略收敛为单一口径（query 参数或 `hx-include`，不得混用）。
7. [ ] 任职记录页严格执行：只展示 `effective_date`，不展示 `end_date`（对齐 084），但底层有效期仍为 `daterange [start,end)`。
8. [ ] E2E：为“导航层级 + as-of 参数透传 + 任职仅展示 effective_date”补齐可视化验收用例（对齐 044 的 UI 验收口径）。

### 回退策略（新仓库口径）
9. [ ] 回退以“发布版本回滚”为唯一手段：上一版构建产物/上一版镜像；不在运行时引入旧页面并存或 feature flag 分流。

## 7. 验收标准（Acceptance Criteria）
- [ ] 左侧导航布局与现有一致；一级仅 4 模块；二级菜单与 §3.2 完全一致。
- [ ] 任职记录页面不展示 `end_date`，只展示 `effective_date`；且 as-of 参数在页面间保持一致透传。
- [ ] 不引入 SPA；所有业务交互仍可用 HTMX 解释（5 分钟可复述：入口 → 请求 → swap → 失败提示）。
- [ ] 权限与本地化不在前端复制实现：导航与操作按钮可随用户权限变化。
- [ ] 文档与门禁：本计划加入 Doc Map，且 `make check doc` 通过。

## 8. Simple > Easy Review（DEV-PLAN-045）

### 8.1 边界
- Astro 只负责 Shell/组件编译；业务内容仍由服务端 HTML partial 提供，避免引入“第二套前端权威表达”。

### 8.2 不变量
- 有效期语义：Valid Time=DATE，`daterange [start,end)`；Assignments 仅显示 `effective_date`（不展示 end_date）。

### 8.3 可解释性
- 主流程：加载 Shell → HTMX 拉取 nav/topbar → 用户点击二级菜单 → HTMX 拉取模块内容 → swap 更新。
- 失败路径：统一走 `/ui/flash` 或现有错误反馈组件，不散落在多处 JS 分支。
