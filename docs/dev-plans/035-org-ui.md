# DEV-PLAN-035：组织机构模块 M1 前端界面

**状态**: 规划中 (2025-12-07 12:10 UTC)

## 背景
- 对应 `DEV-PLAN-020` 的步骤 7A，在组织机构模块的后端 API (`DEV-PLAN-026`) 交付后，需要构建一套完整的前端用户界面来管理组织、职位和分配。
- M1 里程碑的目标是提供一个最小可用但功能完备的主数据管理界面，让 HR 管理员可以直观地操作组织结构。
- 本计划将使用 `templ` 和 HTMX 技术栈，与后端 REST API 进行交互，实现一个响应式、体验流畅的管理界面。

## 前置依赖
- `DEV-PLAN-026` 已完成，提供了所有必需的后端 API，包括：
  - `GET /org/hierarchies`
  - `POST /org/nodes`, `PATCH /org/nodes/{id}`
  - `POST /org/assignments`, `PATCH /org/assignments/{id}`
  - `GET /org/assignments?subject=person:{id}`
- `DEV-PLAN-014` 和 `DEV-PLAN-015` 已提供统一的授权前端组件和模式，如 `pageCtx.CanAuthz` helper 和 `components/authorization/unauthorized.templ` 组件。
- 前端构建环境（`templ generate`, `make css`）正常工作。

## 目标
1.  **交付组织树视图**: 实现一个可交互的树形组件，用于展示和导航组织层级结构。
2.  **交付核心实体表单**: 为组织节点（OrgNode）、职位（Position）和分配（Assignment）提供完整的增删改查（CRUD）表单。
3.  **集成有效期选择器**: 在界面上提供一个全局或局部的日期选择器，允许用户查询和操作任意“生效日期” (`effective_date`) 的组织状态。
4.  **实现授权驱动的 UI**: 界面上的所有操作入口（按钮、链接）都必须根据用户的 Casbin 权限（`Org.Read/Write/Assign`）动态显示或隐藏。
5.  **确保流畅的用户体验**: 利用 HTMX 实现局部刷新，避免整页重新加载，提升表单提交和数据查询的响应速度。

## 实施步骤

### 1. 基础架构与路由搭建
-   **任务**: 在 `modules/org/presentation/` 目录下创建 `controllers`, `templates`, `viewmodels` 等必要结构。
-   **任务**: 在 `modules/org/module.go` 中注册新的前端路由，例如 `/org/nodes`, `/org/assignments`。
-   **任务**: 创建基础的页面布局 `layout.templ`，包含侧边栏导航、头部和内容区域，并集成全局的“生效日期”选择器。

### 2. 组织树视图 (`Tree View`)
-   **任务**: 开发一个可复用的 `org_tree.templ` 组件，通过 HTMX 调用 `GET /org/hierarchies?effective_date=...` API 来获取并渲染数据。
-   **关键**:
    -   组件应支持递归渲染，以展示任意深度的层级。
    -   实现加载中（loading）、空状态（empty state）和错误（error）的友好提示。
    -   树节点应包含基本信息（如名称、编码），并提供点击操作（如 `hx-get`）来在右侧区域加载该节点的详细信息或编辑表单。
    -   当全局“生效日期”选择器变化时，树形视图应能自动刷新。

### 3. 组织节点管理表单
-   **任务**: 创建 `node_form.templ`，用于新建和编辑组织节点。
-   **关键**:
    -   表单通过 `POST /org/nodes` (新建) 和 `PATCH /org/nodes/{id}` (编辑) 与后端交互。
    -   包含 M1 范围内的所有字段：编码、名称（支持多语言）、父组织（可通过树选择或搜索）、法律实体、地点、显示顺序、生效日期等。
    -   使用 HTMX 提交表单，成功后局部刷新组织树和当前表单区域。
    -   “创建”按钮只在用户拥有 `Org.Write` 权限时可见（使用 `pageCtx.CanAuthz` 判断）。

### 4. 人员分配管理
-   **任务**: 创建 `assignment_form.templ`，用于将员工分配到组织节点。
-   **关键**:
    -   表单需要一个“人员选择器”组件，可以通过 API 搜索并选择员工（依赖 HRM 模块的 `GET /api/hrm/employees` 接口）。
    -   表单提交到 `POST /org/assignments`，后端将处理“自动创建空壳职位”的逻辑。
    -   支持查看某员工的所有组织分配历史记录（时间线视图），调用 `GET /org/assignments?subject=person:{id}`。
    -   “分配人员”按钮只在用户拥有 `Org.Assign` 权限时可见。

### 5. 授权与无权限体验
-   **任务**: 在所有模板中，对涉及写操作的 UI 元素（如“新建”、“保存”、“删除”按钮）使用 `if pageCtx.CanAuthz(...)` 进行包裹。
-   **任务**: 当用户访问其无权查看的页面或数据时，控制器应返回 403 状态码，并渲染统一的 `components/authorization/unauthorized.templ` 组件。
-   **关键**: `unauthorized.templ` 组件应能展示缺失的权限信息（从 `pageCtx.AuthzState().MissingPolicies` 获取），并提供“申请权限”的入口（链接到 `DEV-PLAN-015` 实现的策略申请流程）。

### 6. 测试
-   **单元测试**: 为 `viewmodels` 和 `mappers` 编写 Go 单元测试。
-   **E2E 测试**: 在 `e2e/tests/org/` 目录下创建新的 Playwright 测试文件，覆盖以下核心场景：
    1.  使用管理员账户登录，可以成功查看组织树。
    2.  成功创建一个新的组织节点，并在树中看到它。
    3.  成功编辑一个已存在的节点信息。
    4.  成功将一名员工分配到一个组织节点。
    5.  使用一个没有 `Org.Write` 权限的账户登录，验证“创建节点”按钮不可见。
    6.  使用没有 `Org.Read` 权限的账户直接访问 `/org/nodes` 页面，验证页面显示 403 无权限提示。

## 里程碑
-   **M1**: 基础架构和只读的组织树视图完成。用户可以查看组织结构，并随生效日期变化而刷新。
-   **M2**: 组织节点和人员分配的表单开发完成。管理员可以完成核心的增删改查操作。
-   **M3**: 授权逻辑和 E2E 测试全面覆盖。所有 UI 元素均受权限控制，并有完整的端到端测试用例保障。

## 交付物
-   `modules/org/presentation/` 目录下的所有前端相关代码（controllers, templates, viewmodels）。
-   更新后的 `modules/org/module.go`，包含新的路由和导航链接。
-   `e2e/tests/org/` 目录下的 Playwright 测试脚本。
-   在 `docs/dev-records/` 中记录开发过程中的关键决策和 UI 截图。

## 验收标准
-   所有 M1 范围内的 UI 功能均已实现，并且可以在本地环境中流畅操作。
-   所有 UI 操作入口都经过了权限校验，无权限用户无法看到或执行相关操作。
-   `go test ./modules/org/...` 和 `pnpm exec playwright test e2e/tests/org/` 测试全部通过。
-   `templ generate && make css` 执行后，`git status --short` 保持干净，无未提交的生成文件。
-   相关文档（如 README）已更新，说明如何访问和使用新的组织管理界面。