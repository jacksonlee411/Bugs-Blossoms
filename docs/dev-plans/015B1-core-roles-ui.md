# DEV-PLAN-015B1：Core 角色管理 UI

**状态**: 草拟中（2025-12-07 01:36 UTC）  
**范围**: `modules/core` 角色页面的策略可视化、编辑暂存与草稿提交

## 目标
- 提供可读/可编辑的策略矩阵，含 tenant/domain/policyType 过滤与 `p/g` 来源区分。
- 支持无权限只读模式，并提供“请求权限”入口。
- 通过 Session 暂存 + 分页能力，保证大规模策略的可用性与刷新安全。

## 实施步骤
1. [ ] 构建策略矩阵 —— `modules/core/presentation/templates/pages/roles` 新增矩阵，HTMX 调 `GET /core/api/authz/policies?subject=role:<id>`，区分 `p/g`、来源、active/draft；无 `Roles.Update` 权限时只读。过滤器形态：Domain 下拉（含默认租户/全局），policyType 下拉（p/g/全部），可选关键字搜索（object/action 模糊）。
2. [ ] 新增规则交互 —— 采用模态或抽屉形式的表单，字段：Domain（下拉/默认）、Object（下拉或搜索）、Action（下拉或自由文本，标准化为 normalize 结果）、Effect（allow/deny 单选）；提交 HTMX 到 `POST /core/api/authz/policies/stage`，成功后局部刷新矩阵。
3. [ ] 暂存与提交链路 —— `POST/DELETE /core/api/authz/policies/stage`（按 subject/domain 隔离，最多 50 条）；HTMX 返回最新 partial，高亮暂存状态（待添加：淡绿背景；待移除：淡红+删除线，右侧标记“暂存”标签）；`POST /core/api/authz/requests` 读取暂存为 `suggested_diff` 并清空，切换 subject/domain 自动 reset，刷新不丢暂存。
4. [ ] 分页与性能 —— `GET /core/api/authz/policies` 支持分页/排序/过滤参数：`subject=role:<id>&domain=<d>&type=<p|g>&page=<n>&limit=<m>&sort=<field:asc|desc>`，默认 limit=50，最大 200；分页控件用 `hx-get`/`hx-target` 局部刷新，可选列表尾 `hx-trigger="revealed"` 追加“加载更多”。
5. [ ] 只读体验 —— 无编辑权限时矩阵锁定，保留“请求权限”按钮（提交空 diff 草稿）。

## 分阶段交付
1. [ ] 后端接口扩展（阶段 1）—— 为 `/core/api/authz/policies` 补充 subject/domain/type/page/limit/sort 过滤契约；新增 `/core/api/authz/policies/stage`（POST/DELETE）基于 Session 的暂存实现（按 subject/domain 分区，上限 50 条，刷新不丢）。补充控制器/服务与单元测试。
2. [ ] 基础矩阵 UI（阶段 2）—— 角色详情页新增策略矩阵，接入分页/过滤，授权不足时只读；HTMX 部分渲染与 loader/空态。
3. [ ] 暂存交互（阶段 3）—— 新增规则模态/抽屉表单，HTMX 提交到 stage 接口；矩阵展示暂存高亮（淡绿/淡红+删除线/“暂存”标签）；支持删除暂存项。
4. [ ] 草稿提交串联（阶段 4）—— 提交按钮从暂存区构建 `suggested_diff` 调用 `/core/api/authz/requests`，成功后清空暂存；增加错误反馈、授权校验与必要的集成测试。

## 依赖
- 015A API/`/core/api/authz/policies|requests` 已可用。
- Session 存储可用，HTMX/templ 构建链路正常。

## 交付物
- 角色矩阵模板、controller/viewmodel 支撑的 HTMX partial。
- 暂存接口与分页参数实现、对应单元/集成测试。

## 验收
- 角色页面可分页展示策略；有权限可暂存并提交草稿，刷新后暂存不丢；无权限只读且可发起申请。
