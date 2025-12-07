# DEV-PLAN-015B1：Core 角色管理 UI

**状态**: 进行中（2025-12-07 13:25 UTC）  
**范围**: `modules/core` 角色页面的策略可视化、编辑暂存与草稿提交

## 目标
- 提供可读/可编辑的策略矩阵，含 tenant/domain/policyType 过滤与 `p/g` 来源区分。
- 支持无权限只读模式，并提供“请求权限”入口。
- 通过 Session 暂存 + 分页能力，保证大规模策略的可用性与刷新安全。

## 实施步骤
1. [X] 构建策略矩阵 —— `modules/core/presentation/templates/pages/roles` 新增矩阵，HTMX 调 `GET /roles/{id}/policies?subject=role:<slug>`（内部调用 `/core/api/authz/policies`），区分 `p/g`，支持 Domain/type/search 过滤与分页；无 `Roles.Update` 权限时只读；`Authz.Debug` 权限用于查看矩阵。
2. [X] 新增规则交互 —— 采用模态或抽屉形式的表单，字段：Domain（下拉/默认）、Object（下拉或搜索）、Action（下拉或自由文本，标准化为 normalize 结果）、Effect（allow/deny 单选）；提交 HTMX 到 `POST /core/api/authz/policies/stage`，成功后局部刷新矩阵。
3. [X] 暂存与提交链路 —— `POST/DELETE /core/api/authz/policies/stage`（按 subject/domain 隔离，最多 50 条）；HTMX 返回最新 partial，高亮暂存状态（淡绿新增、淡红删除线+“暂存”标签）；`POST /core/api/authz/requests` 可在 `diff` 缺省时从暂存构建 `suggested_diff` 并清空，切换 subject/domain 自动 reset，刷新不丢暂存；新增集成测试验证成功链路。
4. [X] 分页与性能 —— `GET /core/api/authz/policies` 支持分页/排序/过滤参数：`subject=role:<id>&domain=<d>&type=<p|g>&page=<n>&limit=<m>&sort=<field:asc|desc>`，默认 limit=50，最大 200；分页控件用 `hx-get`/`hx-target` 局部刷新，可选列表尾 `hx-trigger="revealed"` 追加“加载更多”。
5. [X] 只读体验 —— 无编辑权限时矩阵锁定，保留“请求权限”按钮（提交空 diff 草稿、空 diff 转为 [] 并弹 toast）；缺权限用户可通过 HX 表单提交访问申请，保留只读文案提示。

## 分阶段交付
1. [X] 后端接口扩展（阶段 1）—— 为 `/core/api/authz/policies` 补充 subject/domain/type/page/limit/sort 过滤契约；新增 `/core/api/authz/policies/stage`（POST/DELETE）基于 Session 的暂存实现（按 subject/domain 分区，上限 50 条，刷新不丢）。补充控制器/服务与单元测试。
2. [X] 基础矩阵 UI（阶段 2）—— 角色详情页新增策略矩阵，接入分页/过滤，授权不足时只读；HTMX 部分渲染与 loader/空态。
3. [X] 暂存交互（阶段 3）—— 新增规则模态/抽屉表单，HTMX 提交到 stage 接口；矩阵展示暂存高亮（淡绿/淡红+删除线/“暂存”标签）；支持删除暂存项。
4. [X] 草稿提交串联（阶段 4）—— 提交按钮可在 `diff` 缺省时从暂存区构建 `suggested_diff` 调用 `/core/api/authz/requests` 并清空暂存；HTMX/权限/校验失败返回 toast 提示，补充空暂存与无权限链路集成测试。

## 依赖
- 015A API/`/core/api/authz/policies|requests` 已可用。
- Session 存储可用，HTMX/templ 构建链路正常。

## 交付物
- 角色矩阵模板、controller/viewmodel 支撑的 HTMX partial。
- 暂存接口与分页参数实现、对应单元/集成测试。

## 验收
- 角色页面可分页展示策略；有权限可暂存并提交草稿，刷新后暂存不丢；无权限只读且可发起申请。

## 后续待办
- [X] 只读入口文案/状态优化：区分请求中禁用按钮并提示。
- [X] 申请链路异常测试补充：空 object、BaseRevision 失配用例。
- [X] 暂存 StageKind 支持删除场景（UI/后端）并完善高亮与提交 diff 语义。
- [ ] 视图提示优化：提交/删除时在矩阵局部展示计数/提示，避免只靠 toast。
