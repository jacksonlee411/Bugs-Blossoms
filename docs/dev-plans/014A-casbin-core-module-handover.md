# DEV-PLAN-014A：Core 模块 Casbin 收尾计划

**状态**: 草拟中（2025-12-04 19:20）

## 背景
- DEV-PLAN-014 主计划中 Core 模块的控制器、导航、PageContext 等基础设施已完成 Casbin 接入，但服务层、UI 长尾点位与灰度日志仍待补齐。
- 后续 HRM/Logging 改造前，需要先确保 Core 模块的授权链路可闭环（controller → service → UI → 日志/回滚），并沉淀成模板，供其他模块复用。
- 本子计划聚焦 Core 模块剩余工作，拉通时间表、跨阶段依赖与验收标准，避免与 HRM/Logging 并行时互相阻塞。

## 目标
1. 服务层（User/Role/Group/Upload/Excel Export 等）及后台任务的写操作全部接入 `authorizeCore`，新增/更新单元测试覆盖拒绝/允许场景。
2. 页面模板与组件全面使用 `pageCtx.CanAuthz` + `authz.ViewState.MissingPolicies`，替换所有 `user.Can`/内联权限判断，统一 403/Unauthorized 体验。
3. Quick Links、侧边栏之外的所有入口（批量操作、导入/导出、WebSocket 触发等）按照 capability 控制可见性，并暴露“申请权限”跳转。
4. 完成 Core 模块的 shadow → enforce 影子验证、启停/回滚日志登记，以及必要的 diff 复盘，确保 HRM/Logging 可以复用流程。

## 剩余工作拆解
- **服务层 & API 授权加固**
  - [ ] 按“资源 × 动作”建表，梳理 `modules/core/services` 下所有写操作是否调用 `authorizeCore`，Excel 导出/导入、批量操作等亦需覆盖；缺失项优先补齐，保持 shadow 阶段仍有 legacy fallback。
  - [ ] 为新增授权点补充单元测试，覆盖“无 context 用户”“shadow deny + legacy allow”“enforce deny”三类场景，确保未来切 enforce 时无回归。
  - [ ] 检查后台作业/事件订阅、定时任务、Websocket 推送等非 HTTP 流程，决定使用 system subject（示例：`system:core.job`）或隔离队列；决策与使用方式记录在 `docs/dev-records`，便于审计。
  - [ ] 盘点 Core 模块所有 REST/GraphQL API（除 `/core/api/authz/*` 外）在通过 middleware 后是否仍需 `ensureAuthz` 或 service guard；对返回 JSON 的接口建议统一 403 payload，并在文档/Swagger 标注依赖的 Casbin 权限。
- **UI/模板消费 ViewState**
  - [ ] 将 PageContext 的 `CanAuthz`/`AuthzState` 应用于所有用户/角色/组/上传页面，所有按钮/操作统一通过该接口控制可见性，彻底删除模板中的 `user.Can`。
  - [ ] 构建统一的 Unauthorized 组件：展示操作名称、`authz.ViewState.MissingPolicies`、`SuggestDiff` 结果及 `/core/api/authz/requests` 入口，shadow 阶段也需提示可申请策略。
  - [ ] 对嵌套组件（如 Quick Link 自定义项、批量操作对话框）进行排查，全部改为依赖 `pageCtx.CanAuthz` 或 Controller 注入的 capability 数据。
- **操作入口 & Quick Links**
  - [ ] 在 Quick Links/Spotlight 数据源中为所有 Core 入口声明 `RequireAuthz(object, action)`（或 fallback 权限），无权时直接隐藏；保留 legacy 体验仅在 shadow 阶段启用。
  - [ ] 页面内的批量删除、导入/导出、审批等按钮统一读取 `pageCtx.CanAuthz`，按钮可配合 `data-disabled`/tooltip，提示“申请权限”并跳转到 Unauthorized 组件。
- **灰度与文档**
  - [ ] 在 `docs/dev-records/DEV-PLAN-012-CASBIN-POC.md` 中补充最新 readiness 记录（含服务层单测结果），保持 shadow 阶段可追溯。
  - [ ] 启动 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`，每次 shadow/enforce 启停都记录命令、tenant、观测指标、diff 与回滚结论，形成重复可用模板。
  - [ ] 汇总上述改造经验，输出 HRM/Logging 可复用的 checklist（服务授权、模板接入、灰度流程），明确“Core M3 完成 = 可以移除 `user.Can` fallback 并通知主计划”的触发条件。

## 验收标准
- `go test ./modules/core/...` 与关键 service/controller 单测全部覆盖新增授权逻辑，shadow 模式输出符合预期。
- 模板/组件中不再出现 `user.Can`/legacy 权限分支，`pageCtx.CanAuthz` 用于所有可见性控制；Unauthorized 体验统一。
- Core 模块在 shadow → enforce 切换期间的命令、diff、回滚操作完整记录在 `docs/dev-records/DEV-PLAN-014-CASBIN-ROLLING.md`。
- 形成面向 HRM/Logging 的复用清单（服务授权 checklist、模板接入步骤、灰度流程），支撑主计划后续阶段。

## 与 014 主计划的衔接节点
- **M1（Core Ready）**：完成服务层 & API 授权补齐、关键单测与 readiness 记录，014 主计划对应“Core controller/service 完成”检查点，可通知 HRM/Logging 复刻代码层逻辑。
- **M2（UI Ready）**：模板/UI 全量换用 `pageCtx.CanAuthz` + Unauthorized 组件，所有入口能力可控；014 主计划可在此阶段同步 UI 资产（Unauthorized 组件、`pageCtx.CanAuthz` 用例）。
- **M3（灰度 Ready）**：完成 shadow → enforce 灰度、回滚演练、rolling log，并输出可复用 checklist，此时即可移除 Core 中 `user.Can` fallback 并通知主计划“Core 模板可直接复用”，HRM/Logging 启动时以该结果为基线。

> 示例：Excel 导出 job 将使用 subject `system:core.job` + object `core.exports` + action `export`，在 `docs/dev-records` 记录该策略与使用方式，供 HRM/Logging 借鉴系统 subject 写法。
