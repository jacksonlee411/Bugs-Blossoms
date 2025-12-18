# DEV-PLAN-035A：Org UI 信息架构（IA）与页面框架/左侧目录集成

**状态**: 草拟中（2025-12-18 10:59 UTC）— 承接 [DEV-PLAN-035](035-org-ui.md)，补齐 IA 决策、可编码 UI 线框与侧栏集成契约

## 1. 背景与上下文 (Context)
- [DEV-PLAN-035](035-org-ui.md) 定义了 Org M1 UI 的可编码契约（templ + HTMX）：树 + 节点表单 + 分配时间线，并明确了 `effective_date`、403 统一输出与 object/action 权限口径。
- 本文档（035A）不改变 035 的后端契约；目标是补齐两类“UI 落地前必须先定”的内容：
  1) 页面信息架构：`/org/nodes` 的右侧面板是否要做 Tabs（节点/分配）？还是拆成 `/org/nodes` 与 `/org/assignments` 两个独立页面？
  2) 与当前项目页面框架的关系：如何复用统一的 `layouts.Authenticated` 壳、以及如何集成进左侧目录栏（sidebar）。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 在文档中“可视化” 035 的 UI：用线框示意图描述实施后页面结构（不要求像素级 UI spec）。
- [ ] 给出 Tabs vs 独立页面的建议与取舍依据（行业做法 + 可维护性）。
- [ ] 明确 Org 模块 UI 与项目现有页面框架/左侧目录栏的集成方式（数据来源、权限过滤、i18n、路由策略）。

### 2.2 非目标 (Out of Scope)
- 不在 035A 内新增 API、表结构、Authz 策略或 HTMX 细节契约（以 035/024/025/026 为 SSOT）。
- 不规定具体 Tailwind class/组件实现细节（实现阶段按已有组件库与 `components/` 复用）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划主要是文档与 IA/集成契约补齐；实现阶段门禁以 035 为准，此处只声明“会命中哪些触发器”，不复制命令细节。

- **触发器清单（本计划/对应实现会命中的项）**：
  - [ ] `.templ` / Tailwind（实现 035 UI 时命中；SSOT：`AGENTS.md` 与 035 §2/§9）
  - [ ] 多语言 JSON（新增 Org UI 文案 keys 时命中；SSOT：`AGENTS.md`）
  - [ ] Authz（为侧栏/页面补齐 capability 与 403 体验时命中；SSOT：`AGENTS.md` 与 035 §7）
  - [ ] 路由治理（若新增 `/org/nodes`、`/org/assignments` 等 UI 路由需要 allowlist 时命中；SSOT：`AGENTS.md`）
  - [ ] 文档门禁（本计划已命中；SSOT：`AGENTS.md`）

- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 035 可编码契约（页面/partial/403/权限口径）：`docs/dev-plans/035-org-ui.md`
  - Org 主链与时间片语义/错误码：`docs/dev-plans/024-org-crud-mainline.md`、`docs/dev-plans/025-org-time-and-audit.md`
  - Org API/Authz/outbox：`docs/dev-plans/026-org-api-authz-and-events.md`

## 3. UI 示意图（基于 035 的契约）
> 说明：下列示意图旨在表达信息架构与交互区域划分，实际 UI 细节以实现为准。

### 3.1 主入口：组织树 + 节点详情（`GET /org/nodes?effective_date=YYYY-MM-DD`）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 顶部（Authenticated Layout）：Navbar + Spotlight + 用户菜单                   │
│ 生效日期 effective_date: [ YYYY-MM-DD ▼ ]  (切换：hx-get + hx-push-url)       │
└─────────────────────────────────────────────────────────────────────────────┘
┌───────────────────────────────┬─────────────────────────────────────────────┐
│ 左侧：组织树（OrgUnit 单树）   │ 右侧：节点详情/表单面板                      │
│ - GET /org/hierarchies?...     │ - GET /org/nodes/{id}?effective_date=...    │
│ - 折叠/展开/选中高亮           │ - 新建/编辑/MoveNode（按权限显示按钮）       │
│                               │ - 写入成功：详情更新 + OOB 刷新树             │
│  ▾ 公司A                       │                                             │
│    ▾ 事业部B                   │  标题：{Node Name}  Code:{code}             │
│       • 团队C [选中]           │  字段：LegalEntity / Location / Order ...    │
│       • 团队D                  │  操作：[新建子节点] [编辑] [变更上级]         │
│                               │                                             │
│                               │  （区域下半部：加载表单/错误提示/空状态）      │
└───────────────────────────────┴─────────────────────────────────────────────┘
```

### 3.2 节点编辑/新建（Insert 时间片语义；`POST /org/nodes`、`PATCH /org/nodes/{id}`）

```
┌──────────────────────────────────────────────────────────────┐
│ 节点表单（effective_date=YYYY-MM-DD）                         │
│ Name: [____________]  i18n_names: [ JSON ... ]                │
│ Legal Entity: [____▼]  Location: [____▼]                      │
│ Display Order: [____]                                         │
│                                                              │
│ [保存] [取消]                                                 │
│ - 422：字段级错误直接显示在表单内                              │
│ - 409：时间线重叠/冻结窗口/唯一性冲突（错误码 SSOT：024/025）   │
└──────────────────────────────────────────────────────────────┘
```

### 3.3 变更上级（MoveNode；`POST /org/nodes/{id}:move`）

```
┌──────────────────────────────────────────────────────────────┐
│ 变更上级（MoveNode）                                          │
│ New Parent: [ 选择一个节点… ▼ ]                               │
│ Effective Date: [ YYYY-MM-DD ]                                │
│ [确认移动] [取消]                                             │
│ - 无权限：入口不可见 + 服务端强制 403                          │
└──────────────────────────────────────────────────────────────┘
```

### 3.4 分配时间线（`GET /org/assignments?subject=person:{pernr}&effective_date=...`）
> 035 的主链是“按人员（subject=person）查看/创建分配”；Position 在 M1 只读展示。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 分配（Assignments）                                                            │
│ effective_date: [ YYYY-MM-DD ▼ ]                                               │
│ 人员：subject=person:[ pernr 手动输入 | （可选）HRM 人员选择器 ]                │
└─────────────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────────────┐
│ 时间线列表（HTMX partial）                                                     │
│ - 展示每条 Assignment：生效区间 / OrgNode / Position(只读) 等                  │
│ - 写入：POST/PATCH 成功后刷新时间线片段                                       │
│                                                                              │
│ 新建/编辑分配：                                                               │
│ pernr:[____]  org_node_id:[____▼/树选择]  position_id(optional):[____]        │
│ [保存]                                                                        │
│ - position_id 为空可触发“自动创建空壳 Position”，不额外要求 org.nodes write    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.5 未授权（403，Full page + HTMX 一致）
```
┌──────────────────────────────────────────────────────────────┐
│ 403 Forbidden                                                  │
│ 你没有权限访问此资源（复用统一 Unauthorized 组件/输出契约）     │
└──────────────────────────────────────────────────────────────┘
```

## 4. 架构与关键决策 (Architecture & Decisions)
### 4.1 IA 决策：右侧面板 Tabs vs 两个独立页面
#### 4.1.1 选项 A：`/org/nodes` 右侧面板做 Tabs（节点/分配）
**定义**：树页右侧面板内提供 Tabs：Tab1=节点详情/表单，Tab2=分配时间线/写入。

**优点（常见诉求）**
- “一个页面完成所有 Org 操作”的观感更强，减少显式跳转。
- 可把 `effective_date` 的 UI 与状态管理收敛在一个页面容器内。

**缺点（行业实践与可维护性角度）**
- 业务语义容易混淆：035 的分配是按人员（`subject=person:{pernr}`）管理，而树是按 OrgNode 浏览；把“人员分配”塞进“节点详情右侧”会迫使 UI 人为构造关联（例如：选中节点时该展示哪位人员？还是展示节点下所有人员？），从而偏离 035 的最小主链。
- 权限与错误处理变复杂：`org.hierarchies read`、`org.nodes write`、`org.assignments assign` 的组合会导致“Tab 可见但内容 403/空”的边界更多，测试矩阵更大。
- 移动端适配更差：树 + 详情已是典型 master-detail，叠加 Tabs 会进一步增加布局与交互负担。

#### 4.1.2 选项 B：拆分两个独立页面（`/org/nodes` 与 `/org/assignments`）
**定义**：结构管理（树/节点）与分配管理（人员时间线）分别为两个页面；共享同一 `effective_date` 体验与 Authenticated Layout。

**优点（更贴近行业做法）**
- 更符合 HR/主数据系统的常见导航：组织结构（Org Structure）与人员分配/任职（Assignments/Employment）通常是两个“主工作台”，分别优化其工作流。
- URL 可分享、可收藏、更利于审计与排障：`/org/nodes?...` 与 `/org/assignments?...` 的 query 清晰表达上下文（as-of + subject）。
- 权限隔离更自然：没有 `org.assignments read/assign` 的用户不需要看到分配入口（减少 UI 内部的“半可用”状态）。

**缺点**
- 左侧目录可能出现多个入口（如果都挂在 sidebar），需要控制信息噪音。

#### 4.1.3 建议（选定）
推荐采用 **选项 B（两个独立页面）**，并用“二级导航（tabs/segmented control）”做一致入口，但**不要**把分配塞进 `/org/nodes` 的右侧面板。

**推荐落地形态（兼顾行业做法与侧栏简洁）**
- Sidebar 只保留一个“组织架构”入口，默认落到 `/org/nodes`（结构是主入口）。
- 在 Org 模块页面内容区顶部放置二级导航 Tabs（不是右侧面板 Tabs）：
  - Tab1：组织结构 → `/org/nodes?effective_date=...`
  - Tab2：人员分配 → `/org/assignments?effective_date=...&subject=person:{pernr}`
- Tabs 的可见性与可点击性同样受 `pageCtx.CanAuthz(...)` 控制；即使隐藏按钮，服务端仍需强制鉴权并统一 403 输出（SSOT：035 §7）。

### 4.2 页面间状态与 URL 传递（选定）
- `effective_date` 必须始终以 query 参数为准（两个页面都遵循 035 的 `YYYY-MM-DD` 输入约束），不依赖前端内存状态。
- 在内容区顶部的二级 Tabs 切换时必须“透传”当前 `effective_date`：
  - 从 `/org/nodes?effective_date=2025-12-18` 切换到分配页时，跳转 URL 至少为 `/org/assignments?effective_date=2025-12-18`。
  - 从分配页切换回结构页时，保留同一 `effective_date`。
- 分配页的 `subject`（`subject=person:{pernr}`）同样固化在 URL：允许分享/收藏/复现。

### 4.3 移动端与窄屏布局策略（选定）
- 避免“全局 Sidebar + 页面内树”的双侧栏挤压：
  - 在 `lg` 以下：页面内“组织树”折叠为可打开的 drawer/弹层（或置顶为一个“选择节点”按钮）。
  - 在 `lg` 以上：采用 master-detail 双栏（树 + 面板）。
- 分配页不强制依赖组织树常驻；`org_node_id` 选择器应独立可用（见 4.4）。

### 4.4 分配页 `org_node_id` 选择器（选定）
为保持 M1 可用性与实现成本可控，分配表单中 `org_node_id` 建议采用“可搜索下拉/弹窗选择”两阶段策略：
- M1 默认：可搜索下拉（按 code/name 展示），数据源由后端提供（可复用 hierarchies/nodes 的读模型或新增轻量 search endpoint；若新增 endpoint，契约需回写到 035）。
- M1 可选增强：弹窗内嵌“迷你树选择器”（只用于选择，不承载编辑），选择后回填 `org_node_id`。

### 4.5 路由落点与重定向策略（选定）
- 侧栏入口落点建议统一到 `/org/nodes`（与 035 的主入口一致）。
- 若保留 `/org`（模块根路径），建议做 302/内部跳转到 `/org/nodes`，避免出现“/org 是占位页、/org/nodes 才是工作台”的割裂体验。

## 5. Org 模块与当前项目页面框架的关系
### 5.1 统一页面壳（Authenticated Layout）
当前项目的“应用壳”由 `layouts.Authenticated` 提供（包含：顶部 Navbar + 左侧 Sidebar + 内容区）。
- 关键文件：`modules/core/presentation/templates/layouts/authenticated.templ`
- Org UI 页面应与现有模块一致：页面模板最外层使用 `@layouts.Authenticated(...) { ... }`，保证：
  - 侧栏/顶栏一致（跨模块体验一致）
  - 全局能力可用（Spotlight、Authz workspace、Toast 等）

### 5.2 Sidebar 数据来源与注入方式
Sidebar 的 props 由中间件注入到 request context，再被 layout 读取：
- 注入中间件：`middleware.NavItems()`（`pkg/middleware/sidebar.go`）
  - 从 `app.NavItems(localizer)` 读取全局导航项
  - 结合权限/能力（Permission 或 `AuthzObject/AuthzAction`）过滤
  - 构建 `sidebar.Props` 并写入 `constants.SidebarPropsKey`
- Layout 读取：`layouts.MustUseSidebarProps(ctx)`（`modules/core/presentation/templates/layouts/composables.go`）

因此，Org UI 控制器注册路由时必须包含 `middleware.NavItems()`（与现有控制器保持一致），否则 `layouts.Authenticated` 会缺少 sidebar props 并 panic。

### 5.3 Org UI 控制器建议中间件组合
参考当前 Org UI controller 骨架：`modules/org/presentation/controllers/org_ui_controller.go`。
建议保持与其他 UI controller 一致的基础组合（最小集合）：
- `middleware.Authorize()`
- `middleware.RedirectNotAuthenticated()`
- `middleware.ProvideUser()`
- `middleware.ProvideDynamicLogo(app)`
- `middleware.ProvideLocalizer(app)`
- `middleware.NavItems()`
- `middleware.WithPageContext()`

## 6. 如何集成到左侧目录栏（Sidebar）
### 6.1 当前状态（已具备的集成点）
- Org 模块已经声明导航项：`modules/org/links.go`（`OrgLink`）
- 全局导航聚合：`modules/load.go` 将 `org.NavItems` 拼入 `modules.NavLinks`
- Org 模块已有 i18n：`modules/org/presentation/locales/*.json`（如 `NavigationLinks.Org`）

### 6.2 建议的侧栏入口与路由策略（对齐 4.5）
为与 035 的页面契约一致，建议将侧栏入口指向“可用的默认工作台”：
- 建议将 Org 侧栏链接的落点设为 `/org/nodes`（或保留 `/org` 并在 `/org` 做 302/内部跳转到 `/org/nodes`）。
- 若未来需要在侧栏展开子项，可将 Org 作为 group（`Children` 非空），子项例如：
  - 组织结构：`/org/nodes`
  - 人员分配：`/org/assignments`
  -（后续里程碑）变更请求/预检：`/org/change-requests`（见 030）

### 6.3 侧栏可见性（建议补齐 capability 约束）
目前 `OrgLink` 未设置 `AuthzObject/AuthzAction` 且 `Permissions=nil`，会导致“所有登录用户都能看到 Org 入口”。
建议在实现阶段将侧栏入口与 035 的最小权限集对齐（只影响“是否展示入口”，不替代服务端鉴权）：
- Org 入口（或“组织结构”子项）建议绑定：`AuthzObject="org.hierarchies"`、`AuthzAction="read"`
- “人员分配”子项建议绑定：`AuthzObject="org.assignments"`、`AuthzAction="read"`

这样 `middleware.NavItems()` 会在渲染侧栏前按 capability 过滤导航项，避免“点进去才 403”的体验。

## 7. 依赖、验收与里程碑（SSOT 引用）
- 依赖与里程碑 SSOT：见 035 §8（Dependencies & Milestones）。
- 验收标准与 E2E SSOT：见 035 §9（Acceptance Criteria）。
- readiness 记录 SSOT：见 035 §9.3，并在实现阶段将命令与结果写入 `docs/dev-records/DEV-PLAN-035-READINESS.md`。

## 8. 待办清单（供实现 035 时引用）
1. [ ] 确认采用“两个页面 + 顶部二级 Tabs”的 IA（本文件 §4.1.3）。
2. [ ] 明确侧栏入口落点：`/org/nodes` vs `/org` 重定向（本文件 §4.5 与 §6.2）。
3. [ ] 在 `modules/org/links.go` 为入口/子项补齐 `AuthzObject/AuthzAction`（本文件 §6.3）。
4. [ ] 为新增导航文案补齐 i18n keys（`modules/org/presentation/locales/*.json`）。
5. [ ] 明确分配页 `org_node_id` 选择器形态（本文件 §4.4），如需新增 search endpoint 需回写到 035 的接口契约（035 §5）。
