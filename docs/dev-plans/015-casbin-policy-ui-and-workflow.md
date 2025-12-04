# DEV-PLAN-015：Casbin 策略管理与 UI 工作流（母计划）

**状态**: 拆分中（2025-01-15 10:35）

## 拆分背景
- 原 DEV-PLAN-015 同时涵盖策略草稿 API/bot 与角色、用户、业务页面等 UI 流程，导致与 DEV-PLAN-014（模块授权改造）存在交叉依赖：014 需要 API/组件才能交付良好体验，而 015 需要 014 的授权底座验证 UI 功能。
- 为降低耦合、允许 014 先行接入授权逻辑，现将 015 分解为两个子计划：
  - **DEV-PLAN-015A：Casbin 策略平台（API、数据模型与 Bot 工作流）** – 提供 `policy_change_requests` 表、REST API、Authz.Debug、PolicyDraftService、UI→Git bot/CLI 以及配套文档、SLA。
  - **DEV-PLAN-015B：Casbin 策略 UI 与授权体验** – 构建角色/用户策略管理界面、业务页面 Unauthorized 组件、PolicyInspector、权限申请入口与多语言文案。
- 本母计划保留高层目标与依赖关系，用于跟踪两个子计划的整体验收。

## 目标
1. 确保 015A/015B 输出覆盖原计划的五大目标（策略展示/编辑、草稿审批、业务授权提示、PolicyInspector、文档培训）。
2. 澄清与 DEV-PLAN-014 的关系：014 可先在 `AUTHZ_ENFORCE` 下完成模块级授权，再根据 015A/015B 的交付节奏逐步启用 UI/申请流程。
3. 提供统一进度视图，确保 015A 完成后即可解锁 014 的依赖，015B 在 014 验真后完善体验。

## 子计划概览
| 子计划 | 关键交付 | 依赖 | 状态 |
| --- | --- | --- | --- |
| [DEV-PLAN-015A](015A-casbin-policy-platform.md) | `policy_change_requests`、REST API、Authz.Debug、PolicyDraftService、bot/CLI、README/AGENTS 更新 | 013 输出的 `pkg/authz`、Feature Flag、数据库迁移、Git bot 凭证 | ✅ 已完成（2025-12-04 08:18） |
| [DEV-PLAN-015B](015B-casbin-policy-ui-and-experience.md) | 角色/用户策略 UI、Unauthorized 组件、PolicyInspector、HRM/Logging 体验、翻译/文档 | 015A API、014 模块授权、`authz.ViewState` 注入 | 草拟 |

## 与 DEV-PLAN-014 的依赖解决方式
- 014 需要的策略申请入口、PolicyInspector、统一 Unauthorized 组件由 015B 提供；在 015B 交付前，014 可先使用占位提示，并通过 015A 的 API 提供最小可用流程（例如直接跳转草稿列表）。
- 015A 完成后即可为 014 提供 `policy_change_requests` 和 `/core/api/authz/debug` 等接口，支撑灰度和排查需求。
- 015B 在实现 UI 时以 015A 提供的契约为准，同时复用 014 已建立的 `authz.Authorize` 与 `AUTHZ_ENFORCE` 环境，避免重复建设。

## 里程碑
- **M1（015A 完成）**：数据库/服务/API/bot 在 dev 环境跑通；README/AGENTS/record 文档更新；014 可直接调用 API。
- **M2（015B 完成）**：角色/用户 UI、Unauthorized/PolicyInspector、HRM/Logging 体验上线；UI→Git 闭环实测稳定。
- **M3（母计划收官）**：014 的 Core/HRM/Logging 授权改造与 015A/015B 的能力相互验证通过，docs/dev-records 更新完毕，授权申请流程在 UI 中稳定运行 ≥2 周。

## 交付物
- DEV-PLAN-015A 与 015B 文档、代码、脚本、模板、bot、翻译与记录。
- 本母计划的依赖追踪表、进度记录，以及与 DEV-PLAN-014 的同步说明。

## 验收标准
- 015A/015B 各自的验收标准全部满足（详见子计划文件）。
- 014 在启用 `AUTHZ_ENFORCE` enforce 模式时，能通过 015A API 获取策略调试信息，并在 UI 中使用 015B 的组件触发草稿。
- dev-records 中至少包含一次“014 启用 + 015A API + 015B UI”联调记录，证明依赖解决方案有效。
