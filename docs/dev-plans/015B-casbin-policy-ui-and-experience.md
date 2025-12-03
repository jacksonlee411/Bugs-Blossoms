# DEV-PLAN-015B：Casbin 策略 UI 与授权体验

**状态**: 草拟中（2025-01-15 10:30）

## 背景
- DEV-PLAN-015A 将交付策略草稿 API、`policy_change_requests`、bot/CLI 与 Authz.Debug。为了让 Core/HRM/Logging 用户真正使用这些能力，需要一套覆盖角色/用户管理、业务页面以及 PolicyInspector/Unauthorized 组件的统一 UI 体验。
- DEV-PLAN-014 的模块改造要求页面提供策略申请按钮、调试信息、统一 403 空态；若缺少 015B 的界面与本地化支持，授权改造将缺乏用户入口。

## 前置条件
- DEV-PLAN-015A 的 API、`policy_change_requests` 表、Authz.Debug、UI→Git bot 已在 dev 环境可用，并提供稳定契约。
- Core/HRM/Logging 控制器已集成 `authz.Authorize` 并在 403 响应中可注入 `MissingPolicies`/`authz.ViewState`。
- 前端构建链路（`templ generate && make css`、Tailwind、htmx）正常；翻译校验 `make check tr` 可执行。
- 具备 `feature/dev-plan-015` 分支或等效工作分支，供 UI 迭代。

## 目标
1. 为角色与用户管理页面提供 Casbin policy 可视化、过滤、编辑/草稿提交流程，支持只读模式与权限申请按钮。
2. 在 HRM、Logging 等业务页面实现统一 Unauthorized 组件、PolicyInspector 抽屉、权限申请入口，展示缺失策略与调试链路。
3. 将页面上的申请操作与 015A API 串联，保证 diff/草稿状态及时反馈，SLA 可视化。
4. 补充多语言翻译、文档示例、dev-records 记录，使运营/管理员可自助体验。

## 实施步骤
1. **[ ] Core 角色管理 UI**
   - `modules/core/presentation/templates/pages/roles`：新增“策略矩阵”组件，使用 HTMX 调用 `GET /core/api/authz/policies?subject=role:<id>`。
   - 支持 tenant/domain/policyType 过滤；行区分 `p` 与 `g` 规则，包含来源（直接/继承）、状态（active/draft）。
   - “新增规则”弹窗/HTMX partial 收集输入后加入前端 diff 列表；“生成草稿”调用 `POST /core/api/authz/requests`，回显 `request_id`。
   - 无 `Roles.Update` 权限时矩阵只读，并显示“请求权限”按钮（diff 为空的草稿）。
2. **[ ] Core 用户管理 UI**
   - Permissions Tab 拆成继承角色、直接策略、Domain Overrides 三列，调用 `GET /core/api/authz/policies?subject=user:<id>` 获取数据。
   - 每列提供“添加”对话框，生成 `g/p` diff；列表展示 `Awaiting PR/Pending Deploy/Active` 状态（来自 `GET /core/api/authz/requests`）。
   - 支持批量撤销、筛选 domain，操作均落地为草稿。
   - 增加 controller/viewmodel/templ 测试，保证授权失败时显示只读视图。
3. **[ ] 业务界面（HRM、Logging 等）体验**
   - 实现 `components/authorization/unauthorized.templ`：渲染缺失策略、参考文档、“申请权限”按钮，按钮 HTMX 调 `POST /core/api/authz/requests`，后台将 `suggested_diff` 写入草稿。
   - `PolicyInspector` 抽屉：只有 `Authz.Debug` 权限可见，调用 `GET /core/api/authz/debug?subject=&object=&action=&domain=`，展示命中规则、ABAC 属性、evaluation latency；提供“一键生成草稿”。
   - 在 HRM/Logging controllers 中，当 403 时传入 `MissingPolicies` 给模板，确保 unauthorized 组件获得上下文；Quick Links/Sidebar 根据 `authz.ViewState` 决定可见性。
   - 添加 e2e/集成测试覆盖“有权限 vs 无权限”场景。
4. **[ ] UI 串联与反馈**
   - 所有草稿操作提供 toast/HTMX snippet，包含 `request_id`、SLA 倒计时（根据 015A 配置）。
   - 页面提供“查看草稿”链接跳转 `/core/authz/requests/{id}`（新建或现有页面），展示 diff、审批状态、bot 日志。
   - PolicyInspector/Unauthorized 组件联动 bot 状态，如 `status=failed` 时展示“重试 bot”链接。
5. **[ ] 文档与翻译**
   - 更新 README/CONTRIBUTING/AGENTS：新增“从 UI 发起策略草稿”“PolicyInspector 使用方法”“HRM/Logging 授权提示”章节。
   - `modules/*/presentation/locales/{en,ru,uz}.json` 增加 Unauthorized、PolicyInspector、SLA、按钮文案；提交前运行 `make check tr`。
   - `docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 记录 UI 里程碑、截图、命令、遇到的授权差异。

## 里程碑
- **M1**：角色/用户页面以 API 只读方式展示策略，Unauthorized 组件就绪。
- **M2**：角色/用户页面支持提交草稿、查看状态；PolicyInspector 可在管理员视角调试；HRM/Logging 页面接入申请按钮。
- **M3**：UI → bot → PR → 状态回写闭环稳定运行 ≥2 周；docs/dev-records、README/AGENTS、多语言更新完成。

## 交付物
- 更新后的角色/用户模板、Controller、ViewModel、HTMX partials，以及对应测试。
- `components/authorization/unauthorized.templ`、PolicyInspector 抽屉、权限申请 helper。
- HRM/Logging 页面改造、Quick Links/导航可见性逻辑、e2e 测试。
- README/CONTRIBUTING/AGENTS 文档补充、`docs/dev-records/DEV-PLAN-015-CASBIN-UI.md` 记录、翻译文件更新。

## 验收标准
- 在 dev 环境，角色/用户页面可通过 UI 创建草稿并在 5 分钟内看到 bot 状态更新；UI 中 `request_id` 与 015A 日志匹配。
- 未授权访问 HRM/Logging 页面时会显示统一组件并成功发起草稿；管理员在 PolicyInspector 中能看到命中链路并生成申请。
- `templ generate && make css`, `make check tr`, `go test ./modules/core/... ./modules/hrm/...` 均通过；`git status --short` 在生成命令后保持干净。
- README/AGENTS 包含 UI 操作示例（含截图或分步描述），用户可按文档复现授权申请流程。
