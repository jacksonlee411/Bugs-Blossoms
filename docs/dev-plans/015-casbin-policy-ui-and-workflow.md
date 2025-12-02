# DEV-PLAN-015：Casbin 策略管理与 UI 工作流

**状态**: 草拟中（2025-01-15 10:15）

## 背景
- DEV-PLAN-013/014 分别交付基础设施与模块改造，但若缺乏官方 UI/流程来查看、编辑、审批 Casbin policy，将迫使管理员直接修改 Git 文件，易导致操作门槛高、配置漂移、缺乏审计。
- 现有 Core 模块的角色/用户界面基于旧的 permission set，无法展示 subject/domain/action，也不支持 policy diff、策略来源追踪。
- HRM 与 Logging 页面在未授权时只展示 403，无法帮助用户快速申请权限，也没有“策略来源面板”辅助排查问题。
- 因此需要单独的计划来构建 Casbin policy 管理 UI、对应 API、以及 UI→Git 草稿→PR→部署的自动化闭环。

## 目标
1. 提供面向角色/用户/租户的 Casbin 策略查看与编辑 UI，支持 domain 过滤、直接策略与继承策略展示。
2. 设计并实现 policy 草稿表、审批 API、bot/CLI，确保所有 UI 变更都会转换为 Git PR 并经 CI 校验后上线。
3. 更新 HRM、Logging 等业务页面的授权提示，让用户在无权限时能看到所需策略、触发申请流程。
4. 为管理员提供 `PolicyInspector` 抽屉/面板，实时展示当前 subject/object/action 的匹配链路与缺失策略建议。
5. 补充操作文档、培训材料、dev-records，确保产品/研发/运营团队理解 UI 功能与限制。

## 风险
- 若 UI 与 Git policy 不一致（例如缓存延迟），可能导致用户认为已批准的策略未生效，需要清晰的状态反馈。
- 暴露策略细节给非管理员会导致安全风险；需要严格的鉴权与审计。
- bot/CLI 若失败或 Git PR 未及时合并，会阻塞业务变更，需要应急处理流程。
- PolicyInspector 调试接口若性能不佳，可能对热点页面造成额外负担。
- 多语言翻译、可用性设计工作量大，需协调设计/前端资源。

## 实施步骤
1. **[ ] API & 数据模型**
   - 设计 `policy_change_requests` 表结构（status、diff、requester、approver、related_subject/domain 等）。
   - 提供 REST API：列出策略、预览 diff、提交草稿、审批/拒绝、触发 bot；API 权限由 Casbin `Authz.Manage` 控制。
   - 新增只读调试 API：`/core/api/authz/debug?subject=&object=&action=&domain=`，返回命中的 rule、ABAC 属性、缺失建议；仅对具备 `Authz.Debug` 的用户开放。
2. **[ ] 角色管理 UI**
   - 更新 `modules/core/.../roles` 页面：角色元数据保留，权限区域替换为 Casbin policy 网格（数据来自 API）。
   - 支持 tenant/domain 过滤、添加/删除 `p`/`g` 规则、查看 diff；提交操作调用“创建草稿”API，提示用户到 PR 查看进度。
   - 提供只读模式（无 `Roles.Update` 时），显示“申请权限”按钮，可直接生成草稿请求。
3. **[ ] 用户管理 UI**
   - `modules/core/.../users` 的 Permissions 标签改为三分区（继承角色、直接策略、domain 绑定），与 API 数据结构一致。
   - 提供“添加 domain-scoped role”“添加单条策略”“从角色继承”对话框，所有操作生成 policy diff 草稿。
   - 列表中显示策略生效状态（Awaiting PR、Pending Deploy、Active），并链接到相应 PR/变更记录。
4. **[ ] 业务界面改造**
   - HRM、Logging 等页面在 403/未授权时渲染统一 `UnauthorizedComponent`：展示缺失策略、申请按钮、参考文档。
   - `PolicyInspector` 抽屉在管理员开启时显示当前 subject/object/action/domain、命中规则、ABAC 属性；支持“一键生成申请”。
   - 将“申请权限”按钮与 policy 草稿 API 连接，自动填入 object/action/domain、原因、来源页面。
5. **[ ] UI→Git 工作流**
   - 构建 bot/CLI：监听草稿状态变化，读取 diff，生成 Git 分支 + PR（自动更新 `config/access/policy.csv`），并将 PR 链接写回草稿记录。
   - CI 成功后 bot 更新草稿状态为 `Merged`，并通知申请人；失败则回写日志供 UI 展示。
   - 提供“生成 Revert PR”操作，用于快速回滚策略。
   - 文档中记录 SLA、审批人、异常处理流程。

## 里程碑
- M1：API/数据模型上线，角色与用户页面可以以只读方式展示 Casbin 策略。
- M2：角色/用户 UI 支持提交草稿并与 bot 生成 PR，PolicyInspector/Unauthorized 组件可用。
- M3：UI→Git 闭环稳定运行 2 周，培训材料与文档完成，HRM/Logging 页面可发起权限申请。

## 交付物
- `policy_change_requests` 数据模型、API、Authz.Debug 端点。
- 更新后的角色/用户页面、Unauthorized 组件、PolicyInspector 抽屉、权限申请对话框。
- bot/CLI 源码（或配置）、CI 集成脚本。
- README/CONTRIBUTING/AGENTS/培训材料更新、`docs/dev-records/DEV-PLAN-015-CASBIN-UI.md`。
