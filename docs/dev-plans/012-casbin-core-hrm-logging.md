# DEV-PLAN-012：Casbin 授权基线（Core/HRM/Logging）

**状态**: 草拟中（2025-01-15 10:00）

## 背景
- DEV-PLAN-009 要求构建统一的 Casbin 授权层，替代当前 `user.Can()` + 手工权限列表的分散实现，并支持 RBAC with domains + ABAC（`docs/dev-plans/009-r200-tooling-alignment.md:29-31`）。
- 现状：Core/HRM/Logging 模块均仅依赖登录态判定。Core 模块声明了用户/角色/组等敏感权限但没有统一校验；HRM 员工控制器未做任何授权；Logging 模块包含日志读取权限常量，却对所有登陆用户开放。
- 为逐步替换 `user.Can` 体系、降低风险，需要选定模块试点 Casbin Enforcer、policy 管理与 `pkg/authz` 适配层。本计划以 Core、HRM、Logging 为首批模块，验证“多资源 RBAC + 基础 ABAC”能力，再为 CRM/仓储等后续模块铺路。

## 目标
1. 建立仓库级 Casbin 配置：`config/access/model.conf`、`config/access/policy.csv`（或 DB adapter），引入 `pkg/authz` 包封装 Enforcer 获取、上下文注入（tenant/user）、ABAC 属性映射。
2. 在 Core 模块：
   - 针对用户、角色、组、上传控制器/服务引入 Casbin 检查（替换现有 `user.Can`/无校验逻辑）。
   - 更新导航显示、API 入口、服务层对资源的判定，并记录测试用例（至少覆盖用户管理 CRUD、角色/权限变更）。
3. 在 HRM 模块：
   - 为员工 CRUD 页面和服务（列表、详情、新建、编辑、删除）增加 Casbin 校验（使用 `Employee.*` 权限），确保 HR 数据访问与修改都必须具备对应策略。
   - 更新 HRM controller/service 流程及文案（未授权反馈），并补充测试或 e2e 脚本验证授权失败路径。
4. 在 Logging 模块：
   - 通过 Casbin 控制日志查看页面、导航入口及相关 API/middleware，仅允许 `Logs.View` 拥有者访问（包括 superadmin）。
   - 同步导航、API 文档与 `pkg/middleware/sidebar` 等入口，确保未授权访问返回 403 并记录审计日志。
5. UI 集成：扩展 Super Admin/Core 权限管理界面，提供 policy 编辑、角色分配与审核能力，确保 Casbin policy 有官方管理面而不仅是文件操作。
6. 文档与运维：更新 README/CONTRIBUTING/AGENTS，说明如何维护 Casbin model/policy、如何引入新模块、如何进行策略变更回滚；在 dev-plan 体系记录 PoC 结果与后续推广计划。
7. 旧权限迁移：编写脚本将现有 `user.Can`/数据库中的租户角色、权限分配导出为 Casbin policy，并提供验证与回滚指引。

## 风险
- **框架引入风险**：Casbin Enforcer、adapter 版本差异可能导致本地/CI 行为不一致；需要在 `tools.go` 或 `go.mod` 中明确依赖版本，并在 `pkg/authz` 捕获初始化错误。
- **策略迁移风险**：从 `user.Can` 切换到 Casbin 需要确保 policy 与旧逻辑完全等价，否则容易出现放行或误拒。需要制定映射表，逐步验证每个资源/动作。
- **性能影响**：在高频 API（用户列表、员工分页）中新增 Enforcer 检查可能带来延迟，如使用 file adapter 应缓存 Enforcer、使用 watcher 减少热更新成本。
- **测试覆盖**：必需为授权失败/成功路径建立单元或集成测试，避免未来重构破坏策略。

## 实施步骤
1. **[ ] Casbin 基础设施**
   - 在 `pkg/authz` 新建适配层：加载配置（model/policy）、封装 `Authorize(ctx, subject, object, action, attrs...)`，并提供 middleware/helper 给控制器调用。
   - 建立“旧权限 → Casbin policy”映射表：逐项列出 `permission.Permission` 与 policy 行，编写自动化脚本/单元测试比对 `user.Can()` 与 Casbin 决策一致，并将该测试纳入 CI。
   - 定义统一的 Casbin subject/object/action/domain/ABAC 属性命名规范：
     - subject：`tenant:{tenantID}:user:{userID}`，保留 `tenant:global` 代表 superadmin。
     - object：`module.resource`（例如 `core.users`、`hrm.employees`、`logging.logs`），action 采用 CRUD/自定义动词。
     - domain：与 tenant ID 一致，可选 `global` 用于跨租户操作；ABAC 属性包括 `tenant_id`、`role_slugs`、`ownership` 等，文档化后所有模块复用。
   - 实现旧权限导出与校验 CLI：
     - `scripts/authz/export` 读取角色/用户映射与 `permission.Permission` 列表，生成 Casbin policy 行并写入 `config/access/policy.csv`。
     - `scripts/authz/verify` 使用同一批租户/用户调用 `user.Can` 与 `authz.Check`，确保迁移前后判定一致；校验失败阻断切换。
     - 文档化回滚路径（重新导出旧版本或 `git revert`，并提供 sample 命令）。
   - 选择持久化方式：PoC 及近期阶段使用 Git 管理的 `config/access/model.conf` + `policy.csv`（file adapter），预留 tenant/domain hook；回滚通过 `git revert`/release patch 完成，并在文档中记录“生产环境快速恢复”流程。
   - 在 `tools.go` / `go.mod` 引入 `github.com/casbin/casbin/v2` 及所需 adapter；在 `Makefile`/CI 增加 `authz-test`、`authz-lint` 等任务（检查 model/policy 语法、排序、重复项），确保 policy diff 被纳入 PR。
   - 扩展 Super Admin/Core 管理界面：UI 编辑只作为“变更发起入口”，通过 API 生成 policy patch 并提示走 PR/代码审查流程，避免直接写数据库造成不可追踪变更；必要时提供临时 override 但要求后续同步到 Git。
2. **[ ] Core 模块改造**
   - 梳理 Core 控制器/服务的授权点（用户、角色、组、上传）并替换为 `pkg/authz` 调用。
   - 迁移 `pkg/types/navigation` 等处对 `user.Can` 的依赖，改为在模板渲染阶段调用 authz helper 或由 controller 注入“是否可见”布尔值。
   - 编写/更新测试：`modules/core/presentation/controllers/*` 应覆盖授权失败返回 403；`modules/core/services` 检查 service 层 guard（如角色更新需要特定权限）。
3. **[ ] HRM 模块改造**
   - 在 `EmployeeController` 的列表、新建、编辑、删除入口添加授权检查；扩展服务层以便在后台作业或 API 中复用 `pkg/authz`。
   - 将 HRM quick link/nav 相关代码更新为基于 Casbin 的可见性判定。
   - 补充 HRM 集成测试或 e2e 脚本，验证具有/不具有 `Employee.*` 权限的用户访问结果。
4. **[ ] Logging 模块改造**
   - 为日志查看页面/API 添加 Casbin 校验，确保只有 `Logs.View` 角色可访问；在 `modules/logging/links.go` 及 controller 处统一调用 authz helper。
   - 更新相关模板/组件，展示“无权限”提示。
5. **[ ] 文档、UI 与推广**
   - 在 README/CONTRIBUTING/AGENTS 新增“Casbin 授权指南”章节，涵盖 model/policy 结构、如何添加新资源、如何运行 `make authz-test/authz-lint`、如何通过 Super Admin UI 生成 policy 变更（含 PR 提交流程）。
   - 在 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 记录 PoC：按“时间 → 命令 → 预期 → 实际 → 结果/回滚”模板撰写，并附 `enforcer` 日志或测试截图。
   - 制定后续推广列表（CRM、Warehouse、Projects 等），列出需要满足的前置条件（Core/HRM/Logging 完成、文档稳定、监控到位）。
   - 在 `quality-gates` workflow 新增 Casbin 规则校验（policy 格式/排序、`make authz-test`、`make authz-lint`、policy diff 提示），确保 CI 能阻止策略遗漏。
   - 具体界面现代化细节：
     - **角色编辑页（modules/core/.../roles）**：
       - 界面分为“角色元数据”（名称、描述）与“Casbin 绑定”两块。
       - “Casbin 绑定”改用 policy 网格：列出当前角色对应的 `g, role_slug, subject/domain` 与 `p, role_slug, object, action, domain` 规则。数据来源为新的 `/core/api/roles/:id/policies` API，后端从 `config/access/policy.csv` 解析并附带差异标记。
       - 支持在 UI 中新增/删除 g/p 规则、按 tenant/domain、资源、动作过滤，同时展示“将生成的 diff”预览；表单提交只写入 `policy_change_requests` 草稿，由 UI→Git 流程生成 PR。
       - 若当前登录用户对某资源没有 `Roles.Update` 权限，则 Casbin section 以只读标签渲染，并显示“无权限编辑此策略（请求权限）”提示按钮。
     - **用户编辑页（modules/core/.../users）**：
       - Permissions 标签拆成三个面板：① 继承角色（source=角色列表）；② 直接策略（列出该用户的 `p` 规则）；③ Domain 赋权（`g` 规则，表示 user ↔ role/domain 绑定）。
       - 每个面板调用 `/core/api/users/:id/policies` API，返回结构：`directPolicies`, `inheritedPolicies`, `domains`。UI 可筛选 tenant/domain，并提供“添加 domain-scoped role”“添加单条 policy”对话框。
       - 所有变更（勾选、删除、添加）都会汇总为 diff，并通过“提交策略草稿”按钮写入 `policy_change_requests`；旧的直接写数据库的权限表单被废弃。
       - 若用户缺少 `Users.UpdatePermissions`，面板自动隐藏操作按钮，只保留只读列表与“请求权限”入口。
     - **HRM 页面**：
       - 在列表/表单视图顶部展示 `authz.Check(ctx,"employee",action)` 的实时状态（绿勾=有权，红叉=无权），文案使用 `HRM.Authorization.ActionX` 译文。
       - 新增/导入/删除按钮在渲染时读取该状态决定是否禁用；禁用时悬浮提示“请申请 Employee.<action> 权限”并提供按钮触发权限申请（写入 `policy_change_requests`，生成描述为“HRM 页面请求权限”）。
       - Page 403 空态统一引用 `components/authorization/unauthorized.templ`，内部带“返回 HRM 首页”和“申请权限”操作，保证体验一致。
     - **Logging 页面**：
       - 侧栏/导航在渲染时调用 `authz.Check(..., Logs.View)`，若失败则隐藏入口；若直接访问 URL，controller 返回 403 并渲染同样的 unauthorized 组件。
       - 页面顶部挂出“当前策略”徽章（显示 subject/domain），方便管理员确认使用哪个租户域；在无权限时提供“生成权限申请草稿”按钮。
     - **策略来源面板**：
       - 每个需要权限的页面都附带一个仅对管理员可见的抽屉组件（`PolicyInspector`），由 `/core/api/authz/debug?subject=&object=&action=&domain=` 提供数据：包含命中的 policy、匹配链路、ABAC 属性。
       - 当授权失败时，面板自动打开并展示“缺失策略建议”，例如：`p, tenant:123:user:456, hrm.employees, read`；管理员可一键“生成草稿”，跳到 policy 草稿表单。
       - API 只对拥有 `Authz.Debug` 权限的用户开放，避免普通用户看到完整策略列表。
   - 设计 UI→Git 的策略变更闭环：
     - UI 仅支持创建“策略变更草稿”，写入 `policy_change_requests` 表并附上 diff。
     - 后端触发 bot（或 CLI）将 diff 转成 Git branch + PR，并在 PR 中引用原始请求。
     - Merge 后通过流水线同步 `config/access/policy.csv`，CI 校验成功才能部署；失败时 UI 回传状态。
     - 回滚流程由 UI 触发“生成 revert PR”，保持所有策略变更可审计。
6. **[ ] 分批迁移与验证**
   - 引入 `AUTHZ_ENFORCE` feature flag：默认以“旁路模式”并行运行 Casbin 与 `user.Can`，对比日志记录所有差异。
   - 以模块×租户为最小迁移批次：先选择测试租户启用强制校验，确认无误后逐步扩展至全部租户与模块。
   - 为每个批次准备回滚剧本：关闭 feature flag + 恢复旧 policy 导出；在 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 写明启停时间、命令与监控指标。
   - 在 Core/HRM/Logging 控制器中保留 `authz.Check` 和 `user.Can` 双分支，待迁移完成后再移除旧逻辑，确保随时可切回旧授权。

## 里程碑
- M1：`pkg/authz` + Casbin 配置完成，Core/HRM/Logging 引入 helper（尚未替换控制器），README 初稿完成。
- M2：Core 控制器/服务全面使用 Casbin；HRM、Logging 完成授权覆盖，单元/集成测试全部通过。
- M3：CI 增加 Casbin policy 规范检查；文档/PoC 记录完成；准备推广到其他模块。

## 交付物
- `pkg/authz` 适配层、`config/access/model.conf`、`config/access/policy.csv`（或 adapter 配置）。
- Core/HRM/Logging 控制器/服务的授权改造代码及测试。
- 更新后的 README/CONTRIBUTING/AGENTS、`docs/dev-records/DEV-PLAN-012-CASBIN-POC.md`。
- 待推广模块列表及策略映射表。
- 旧权限导出脚本、策略命名规范、UI→Git 自动化流程文档。
