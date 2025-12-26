# DEV-PLAN-016A：Core 用户权限页信息架构（IA）与交互契约优化

**状态**: 已完成（2025-12-14 10:14 UTC）

## 1. 背景与上下文 (Context)
- **需求来源**: Core 用户详情页（Edit user）Permissions 面板的可用性反馈与截图复盘。
- **当前痛点**:
  - 信息层级不清：同屏同时呈现 Effective + Inherited/Direct/Overrides 多表，且各自重复 Search/筛选/分页。
  - 主操作分散：Create/Submit/Discard 等入口分散在多处，难以形成“先看清楚，再改”的操作路径。
  - 长 Subject/Domain/UUID 破坏可读性，表格横向滚动与可扫描性差。
  - 只读/可编辑边界不清：Inherited（继承语义）与可编辑列混在同一栅格，用户容易误以为可直接改继承。
- **业务价值**:
  - 降低权限排查与变更的理解成本，提高“看清楚来源 → 做变更 → 立即生效”的完成率。
  - 统一 UI 交互范式，向主流 IAM/RBAC 控制台（AWS/Azure/GCP/Okta/GitHub 类）靠拢。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**:
  - [X] Permissions 页面从“同屏多表”调整为“Tab 分屏”，默认落在 Effective（生效权限汇总）。
  - [X] 顶部统一筛选条（Domain + Search + Clear），各表不再重复 Search/筛选表单。
  - [X] 主操作收敛为单一入口：右上一个主 CTA（Create 下拉：Direct / Overrides），避免区块内重复 Create。
  - [X] Effective 表可读可钻取：列聚焦 Domain/Object/Action/Effect；Sources 以抽屉方式展开展示 Direct/Role chain。
  - [X] Inherited 明确为只读解释型：提示“在用户信息中改角色/组”，并在操作列显示只读文案。
  - [X] 长 ID 默认折叠并可复制：Subject/Domain 显示短格式 + Copy 按钮，保留 title/hover 展示全量。
  - [X] 通过 `make check lint`、`make check tr`、`make check doc` 等门禁。
- **非目标 (Out of Scope)**:
  - 不新增后端 API（沿用 015A/015B 既有 `/core/api/authz/*` 与 `/users/{id}/policies`）。
  - 不引入新的权限模型/策略语义变更（不改变 Casbin 的 p/g 语义）。
  - 不在本计划内实现“Effect/Sources 多条件过滤”与“Domain 下拉字典化”；仅保留 Domain 文本输入（后续计划再做）。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
graph TD
  U[User Edit Page] --> P[Permissions Tab]
  P --> PB[UserPolicyBoard (templ)]
  PB --> EP[Effective Table (templ partial)]
  PB --> CP[Columns (templ partial)]
  PB --> AW[AuthzWorkspace (sticky footer)]
  EP --> API1[GET /users/{id}/policies?column=effective&domain&q&page&limit]
  CP --> API2[GET /users/{id}/policies?column=direct|overrides|inherited&domain&q&page&limit]
  PB --> API3[POST/DELETE /core/api/authz/policies/stage]
	  AW --> API4[POST /core/api/authz/policies/apply]
```

### 3.2 关键设计决策 (ADR 摘要)
- **决策 1：用 Tab 替代同屏多列**
  - 选项 A：维持同屏 4 表，做栅格与视觉优化（风险：信息密度仍高，主操作分散问题难根治）。
  - 选项 B（选定）：Permissions 内二级 Tab（Effective/Direct/Overrides/Inherited），默认 Effective（收益：先读后写，降低噪音）。
- **决策 2：Sources 以抽屉钻取**
  - 选项 A：Sources 列内塞满 chips（风险：拥挤、横向滚动、信息溢出）。
  - 选项 B（选定）：Sources 摘要 + 右侧抽屉详情（收益：主视图可扫描，细节可钻取）。

## 4. 数据模型与约束 (Data Model & Constraints)
本计划不涉及数据库 Schema/迁移变更。

## 5. 接口契约 (API Contracts)
> 说明：本计划仅调整 UI 交互契约，不新增 API；以下列出 UI 依赖的既有接口行为。

### 5.1 HTMX：权限板整体刷新
- **Action**: 顶部筛选条变化（Domain/Search/Clear）。
- **Request**: `GET /users/{id}/policies`（可含 `domain=<string>&q=<string>`）。
- **Response (200 OK)**: 返回更新后的 `#user-policy-board` HTML（`hx-target="#user-policy-board"`，`hx-swap="outerHTML"`）。
- **URL 行为**: 筛选变更需要 push 到地址栏（`hx-push-url="true"`），便于分享与回溯。

### 5.2 HTMX：各 Tab 表格刷新与分页
- **Effective**:
  - `GET /users/{id}/policies?column=effective&page&limit&domain&q`
  - 返回 `UserEffectivePolicies` HTML 片段。
- **Direct/Overrides/Inherited**:
  - `GET /users/{id}/policies?column=direct|overrides|inherited&page&limit&domain&q`
  - 返回 `UserPolicyColumn` HTML 片段。

### 5.3 Stage 与提交
- **Stage**: `POST /core/api/authz/policies/stage`（JSON/表单），`DELETE /core/api/authz/policies/stage`（按 id/ids）。
- **Apply Now**: `POST /core/api/authz/policies/apply`（由 `AuthzWorkspace` sticky footer 触发，直接写入生效策略并 `ReloadPolicy`）。

## 6. 核心逻辑与交互 (UI Contracts)
### 6.1 页面信息架构（Permissions 二级 Tab）
- 默认 Tab：`Effective`。
- Tab 列表：`Effective / Direct / Overrides / Inherited`。
- Inherited 的交互语义：
  - 内容仍可浏览与分页；
  - 明确提示“只读（在用户信息中改角色/组）”；
  - 不提供 Create/Stage 编辑入口。

### 6.2 顶部全局筛选条
- 输入项：
  - Domain（文本输入，默认值为 DefaultDomain 或当前筛选值）。
  - Search（文本输入，作为 `q`）。
  - Clear（一键清空，回到 baseURL）。
- 筛选结果影响：Effective 与各列请求的 `domain/q` 参数。

### 6.3 主 CTA（Create 下拉）
- 入口位置：Permissions 右上角。
- 可见性：仅当 `CanStage==true` 显示；否则保持申请入口或提示。
- 下拉项：
  - Direct（打开 direct stage 抽屉）
  - Overrides（打开 overrides stage 抽屉）

### 6.4 Effective：Sources 抽屉
- Sources 列展示摘要：
  - Direct badge（若 `entry.Direct`）
  - 角色来源数量摘要
  - 放大镜按钮打开抽屉（右侧）。
- 抽屉展示：
  - 当前规则（Domain/Object/Action/Effect）
  - Sources 列表（Direct、各 Role chain，role policies URL 可跳转）

### 6.5 Subject/Domain 长 ID 展示
- 顶部 meta：短格式（如 `tenant:...:user:0000…0001`）+ Copy 按钮。
- 表格字段（Subject/Object/Action/Domain）：等宽字体 + `break-all`，保留 `title` 展示全量。

## 7. 安全与鉴权 (Security & Authz)
- **读权限**：
  - 用户页 `view` 权限通过 `ensureUsersAuthz`；
  - 权限板 Debug 能力仍受 `core.authz/debug` 控制（无 debug 时降级为 Unauthorized 视图）。
- **写权限**：
  - `CanStage` 为 true 才显示 Create 下拉与可编辑操作；
  - Inherited 永远只读（即使 CanStage 为 true）。
- **无权限反馈**：
  - 以禁用/隐藏按钮为主；必要处使用 tooltip/Unauthorized 组件说明缺失的权限点。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - DEV-PLAN-015A/015B2 已提供的 `/core/api/authz/*` 与用户页 policies partial。
- **里程碑**：
  1. [X] 更新 Permissions 信息架构（Tab + 全局筛选条 + 主 CTA）。
  2. [X] Sources 抽屉与长 ID 折叠复制（含 i18n key）。
  3. [X] 生成物与门禁回归：`make generate && make css && make check tr && make check lint && make check doc`。

## 9. 测试与验收标准 (Acceptance Criteria)
- 默认进入 Permissions 时，首先看到 Effective，且页面不再同屏展示 4 张表。
- 顶部筛选条的 Domain/Search 对所有 Tab 生效；Clear 能回到默认状态并 push URL。
- 页面仅保留一个 Create 主入口（下拉），各 Tab 内不再出现散落 Create。
- Effective 主视图无横向滚动或显著拥挤；Sources 通过抽屉可钻取并能看到 role chain。
- Inherited 明确只读且提示“去用户信息改角色/组”；不提供编辑入口。
- Subject/Domain/UUID 不再压垮布局：短展示可复制，全量可通过 title/hover 获取。
- 门禁通过：`make check lint`、`make check tr`、`make check doc`。

## 10. 运维与监控 (Ops & Monitoring)
- 不新增 Feature Flag。
- 继续复用既有 SLA 轮询（最长 5 分钟）与超时提示；支持复制 request_id 以便排障。

## 实施记录（2025-12-14 10:14 UTC）
### 已交付（代码）
- `modules/core/presentation/templates/pages/users/policy_board.templ`：Permissions 二级 Tab、顶部全局筛选条、Create 下拉、Effective Sources 抽屉、长 ID 折叠/复制、Inherited 只读边界。
- `modules/core/presentation/templates/pages/users/edit.templ`：底部 Save/Delete 操作栏仅在“用户信息”Tab 显示，避免与权限草稿操作混淆。
- `modules/core/presentation/locales/{en,zh,ru,uz}.json`：补齐 `Clear` 与 Inherited 只读提示文案。
- `AGENTS.md`：Doc Map 增补本计划链接。

### 已执行（生成与校验）
1. [X] `.templ` 与 Tailwind 生成：`make generate && make css`（提示：`caniuse-lite` 更新提示不影响构建结果）
2. [X] i18n 校验：`make check tr`
3. [X] Go 基础校验：`go fmt ./... && go vet ./...`
4. [X] Lint/架构约束：`make check lint`
5. [X] 文档门禁：`make check doc`
