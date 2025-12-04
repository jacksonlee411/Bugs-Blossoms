# DEV-PLAN-014A：Core 模块 Casbin 收尾计划

**状态**: 草拟中（2025-12-04 19:20）

## 背景
- DEV-PLAN-014 主计划中 Core 模块的控制器、导航、PageContext 等基础设施已完成 Casbin 接入，但服务层、UI 长尾点位与灰度日志仍待补齐。
- 后续 HRM/Logging 改造前，需要先确保 Core 模块的授权链路可闭环（controller → service → UI → 日志/回滚），并沉淀成模板，供其他模块复用。
- 本子计划聚焦 Core 模块剩余工作，拉通责任、时间表与验收标准，避免与 HRM/Logging 并行时互相阻塞。

## 目标
1. 服务层（User/Role/Group/Upload/Excel Export 等）及后台任务的写操作全部接入 `authorizeCore`，新增/更新单元测试覆盖拒绝/允许场景。
2. 页面模板与组件全面使用 `pageCtx.CanAuthz` + `authz.ViewState.MissingPolicies`，替换所有 `user.Can`/内联权限判断，统一 403/Unauthorized 体验。
3. Quick Links、侧边栏之外的所有入口（批量操作、导入/导出、WebSocket 触发等）按照 capability 控制可见性，并暴露“申请权限”跳转。
4. 完成 Core 模块的 shadow → enforce 影子验证、启停/回滚日志登记，以及必要的 diff 复盘，确保 HRM/Logging 可以复用流程。

## 剩余工作拆解
- **服务层授权加固**
  - [ ] 梳理 `modules/core/services` 下所有写操作，确认是否已有 `authorizeCore` 调用；缺失项依优先级补齐（含 Excel Export 等间接写操作）。
  - [ ] 为新增授权点补充单元测试，覆盖“无 context 用户”“shadow deny + legacy allow”“enforce deny”三类场景。
  - [ ] 检查后台作业/事件订阅（如 realtime updates）在无 HTTP context 下的授权策略，必要时通过 system subject/attributes 控制。
- **UI/模板消费 ViewState**
  - [ ] 将 PageContext 的 `CanAuthz`/`AuthzState` 透传到用户详情、角色编辑、组管理等页面，对所有按钮/动作进行控制。
  - [ ] 实现统一的 Unauthorized 组件，展示 `MissingPolicies`、`SuggestDiff` 与 `/core/api/authz/requests` 入口；在 015B 交付前提供临时兜底。
  - [ ] 替换模板中的 `user.Can` 判断（包含嵌套组件、Quick Link 自定义项等），实现一次性收敛。
- **操作入口 & Quick Links**
  - [ ] 在 Quick Links/Spotlight 数据源中为所有 Core 入口声明 `RequireAuthz` 或 fallback 权限，兼容 legacy 体验。
  - [ ] 页面内的批量删除、导入、导出、审批等入口统一读取 `pageCtx.CanAuthz`，并在无权时显示“申请权限”提示。
- **灰度与文档**
  - [ ] 在 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 中补充最新 readiness 记录（含 service 层测试结果）。
  - [ ] 启动 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`，登记 Core 模块的 shadow/enforce 启停命令、观测、回滚演练。
  - [ ] 汇总上述改造经验，形成可供 HRM/Logging 复用的 check list / best practice。

## 验收标准
- `go test ./modules/core/...` 与关键 service/controller 单测全部覆盖新增授权逻辑，shadow 模式输出符合预期。
- 模板/组件中不再出现 `user.Can`/legacy 权限分支，`pageCtx.CanAuthz` 用于所有可见性控制；Unauthorized 体验统一。
- Core 模块在 shadow → enforce 切换期间的命令、diff、回滚操作完整记录在 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`。
- 形成面向 HRM/Logging 的复用清单（服务授权 checklist、模板接入步骤、灰度流程），支撑主计划后续阶段。

## 后续节点
- M1：完成服务层授权补齐 + 单测，通过 readiness 记录。
- M2：模板/UI 全量换用 `pageCtx.CanAuthz` + Unauthorized 组件。
- M3：完成 shadow → enforce 灰度及回滚演练，沉淀 checklist，准备 HRM/Logging 复用。
