# DEV-PLAN-092：V4 Authz（Casbin）工具链与实施方案（Greenfield）

**状态**: 草拟中（2026-01-05 08:57 UTC）

> 适用范围：**全新实现的 V4 新代码仓库（Greenfield）**。  
> 本文冻结 V4 的授权（Authz）契约与工具链口径：`subject/object/action/domain` 命名、policy 的 SSOT 与发布方式、CI 门禁，以及与 `DEV-PLAN-081/088`（RLS/AuthN/Tenancy）的边界关系，避免实现期“各模块各写一套”导致漂移。

## 1. 背景与上下文 (Context)

- V4 选择 Greenfield 全新实施（077+），不承担存量 `user.Can`/旧权限映射的兼容包袱。
- 现仓库已具备一套 Casbin 工具链与门禁（`DEV-PLAN-013`、`DEV-PLAN-016C`、`docs/runbooks/authz-policy-apply-api.md`），但 V4 的主体模型在 `DEV-PLAN-088` 已选定为 `principal`（而非 `user`），需要尽早冻结命名与边界，避免后续改名或双轨并存。
- V4 同时选定：RLS 做强租户隔离（`DEV-PLAN-081`），Casbin 做“是否允许做事”（Authz），两者边界必须明确：**RLS 圈地 != Casbin 授权**。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标

- [ ] 冻结 V4 Authz 合同：`subject/object/action/domain` 的命名规范与不变量（可用于 code review 与测试断言）。
- [ ] 给出 V4 的 policy SSOT 与发布口径（Git 管理 vs Apply API），并明确“生产可复现”的约束。
- [ ] 给出模块级接入模板（controller/service 如何调用 `authz.Authorize`，以及 403/forbidden payload 口径）。
- [ ] 形成 V4 的工具链门禁清单（触发器、CI 入口、生成物与验收标准），避免实现期临时拼装。

### 2.2 非目标（明确不做）

- 不在本计划内交付企业 SSO（Jackson）或多 IdP 编排；相关工作属于 AuthN/SSO 计划（参考 `DEV-PLAN-019C`）。
- 不在本计划内引入“复杂 ABAC DSL/表达式”或策略编排语言；仅保留最小 ABAC 字段（如确需）。
- 不在本计划内迁移现仓库 policy/roles 到 V4；V4 以最小角色集重新定义。
- 不在本计划内新增数据库表存储 policy（避免把配置与数据耦合、并触发迁移门禁）；V4 baseline 以文件策略为主。

## 3. 工具链与门禁（SSOT 引用）

> 本计划不复制命令矩阵；触发器与门禁以 SSOT 为准。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`
- 现仓库 Authz（Casbin）基础设施基线：`docs/dev-plans/013-casbin-infrastructure-and-migration.md`
- Authz “Simple > Easy” 审计与契约收敛：`docs/dev-plans/016C-authz-simple-not-easy-audit.md`
- Policy Apply API（可选能力）：`docs/runbooks/authz-policy-apply-api.md`、`docs/dev-plans/015C-authz-direct-effective-policy-management.md`
- V4 Tenancy/AuthN 与主体模型（principal）：`docs/dev-plans/088-tenant-and-authn-v4.md`
- V4 RLS 强租户隔离：`docs/dev-plans/081-pg-rls-for-org-position-job-catalog-v4.md`
- V4 技术栈与工具链版本（Casbin 版本基线等）：`docs/dev-plans/087-v4-tech-stack-and-toolchain-versions.md`

## 4. 关键决策（ADR 摘要）

### 4.1 继续采用 Casbin（选定）

- **选定**：V4 继续采用 Casbin 作为授权引擎（RBAC with domains + 最小 ABAC），沿用现仓库的 `config/access/model.conf` 思路。
- **理由**：工具链与门禁已成熟（pack/lint/test/apply）；V4 更需要“冻结契约、防漂移”，而非替换引擎。

### 4.2 Domain 语义（选定：tenant UUID / global）

- **选定**：Casbin `domain` 仅有两类：
  - 租户域：`strings.ToLower(tenant_id.String())`（对齐 `pkg/authz.DomainFromTenant` 的语义）
  - 全局域：`global`
- **禁止**：把模块名、路由 segment、hostname 写入 domain（对齐 `DEV-PLAN-016C` 的 stopline）。

### 4.3 Subject 语义（选定：role 为授权主体；principal 为审计标识）

> `DEV-PLAN-088` 选定“本地主体”为 `principal`。但 V4 Greenfield 若把“principal↔role 绑定”也写进 Casbin policy（`g`/`g2`），会导致“创建用户/租户 = 修改 policy 文件”的运维耦合。为保持简单性：**V4 MVP 的 Casbin 授权以 role 为主体**；principal 仍保留为审计/日志/诊断的稳定标识。

- **审计标识（principal id，非 Casbin Enforce 输入）**：
  - tenant principal：`tenant:{tenant_id}:principal:{principal_id}`
  - global principal（仅控制面）：`global:principal:{principal_id}`
- **授权主体（Effective Subject，Casbin Enforce 输入）**：
  - role：`role:{slug}`（全小写，slug 作为稳定标识）
- **V4 MVP 约束（选定以保持简单）**：每个 session 恰好一个 `role_slug`（不做多角色 OR 判定）；如未来需要“多角色/继承/组”，另起子计划并给出边界与回滚。
- **不做兼容**：不支持 `tenant:{id}:user:{id}` 作为 V4 输入格式；如未来需要兼容旧系统迁移，另起计划并明确双轨期限。

### 4.4 Object 命名（选定：module.resource）

- **选定**：object 采用 `module.resource`（全小写）。
- **V4 模块建议前缀**（与 `DEV-PLAN-083/088` 对齐）：
  - `iam.*`（tenancy/authn/session/principal 等平台域）
  - `orgunit.*`
  - `jobcatalog.*`
  - `staffing.*`
  - `person.*`
  - `superadmin.*`（仅控制面；与 tenant app 隔离）

### 4.5 Action 命名（选定：最小动词集合）

- **选定**：action 以最小集合起步，避免同义动词泛滥：
  - CRUD：`read/create/update/delete`
  - 管理：`admin`
  - 诊断（仅 debug/诊断端点）：`debug`
- 后续新增 action 必须在对应 dev-plan 中声明（并补齐 policy + 测试），不得在代码里“随手造词”。

### 4.6 Policy SSOT 与发布方式（选定：Git 管理 + pack）

- **选定（V4 baseline）**：policy 以 Git 管理为 SSOT：
  - 源文件：`config/access/policies/**`（按模块拆分）
  - 生成物：`config/access/policy.csv` 与 `config/access/policy.csv.rev`（由 pack 生成，必须提交）
- **暂不纳入**：管理员在线 Apply（015C）作为 V4 MVP 的必选能力。若未来引入，必须补齐“容器内写文件的持久化策略、审计与回滚”，另起子计划（建议 092A）。

### 4.7 与 RLS 的边界（选定：分层防御）

- RLS（`DEV-PLAN-081`）：只负责“同租户可见性”（圈地），不表达“是否允许操作”。
- Casbin：只负责“是否允许做事”（按 subject/object/action/domain），不得替代 tenant 解析或 RLS 注入。
- **禁止**：为了 superadmin 跨租户需求放宽 RLS policy；跨租户必须走控制面边界与专用 DB role（对齐 `DEV-PLAN-088`）。

### 4.8 运行态模式（选定：`AUTHZ_MODE` + `authz_flags.yaml`，无 segments）

> 目标：给实现一个单一可解释的“开关”，并且行为可测、可回滚。

- **选定**：沿用现仓库 `pkg/authz` 的三态模式：`disabled|shadow|enforce`（环境变量 `AUTHZ_MODE` 可覆盖配置文件）。
- **SSOT**：`config/access/authz_flags.yaml` 仅允许包含 `mode` 字段；**禁止**出现 `segments` 等扩展字段（避免“写了但运行时不生效”的漂移，问题在 `DEV-PLAN-016C` 已发生过）。
- 行为合同：

| mode | denied 时的行为 | 记录缺口（missing policy） | 典型用途 |
| --- | --- | --- | --- |
| `disabled` | 不做授权判断 | 否 | 本地排障/短期止血 |
| `shadow` | 不中断请求（继续执行） | 是（日志/诊断） | 新模块接入期 |
| `enforce` | 直接拒绝（统一 403） | 是（日志/诊断） | 默认（生产） |

### 4.9 V4 最小角色与策略包（选定：3 角色 + 只用 `read/admin/debug`）

> 目标：V4 先把“能跑通的最小权限闭环”冻结为可执行规格；后续新增能力必须显式扩展本表。

- **选定角色**：
  - `role:superadmin`（控制面）
  - `role:tenant_admin`（租户管理员）
  - `role:tenant_viewer`（租户只读）
- **动作口径（MVP）**：只允许使用 `read/admin/debug`；`create/update/delete` 保留但不在 MVP 使用（避免早期拆得过细造成策略爆炸与漂移）。

策略矩阵（MVP，建议从此起步）：

| object（module.resource） | `tenant_viewer` | `tenant_admin` | `superadmin` |
| --- | --- | --- | --- |
| `orgunit.nodes` | `read` | `read, admin` | — |
| `jobcatalog.catalog` | `read` | `read, admin` | — |
| `staffing.positions` | `read` | `read, admin` | — |
| `staffing.assignments` | `read` | `read, admin` | — |
| `person.persons` | `read` | `read, admin` | — |
| `superadmin.tenants` | — | — | `read, admin` |
| `superadmin.authz`（可选） | — | — | `debug` |

## 5. 新仓库落地形态（目录与产物）

> 以下为 V4 新仓库建议结构；现仓库实现可作为参考，但不要求一字不差照搬。

- `pkg/authz/**`：enforcer 构造、请求类型、subject/object/action/domain 规范化、403 payload helper。
- `config/access/model.conf`：Casbin 模型。
- `config/access/policies/**`：策略碎片（模块维度）。
- `config/access/policy.csv`、`config/access/policy.csv.rev`：聚合产物（pack 生成）。
- `scripts/authz/**`：pack/lint/verify 辅助脚本（以 `Makefile` 为入口）。

### 5.1 授权主流程与失败路径（10 句可复述）

> 目标：reviewer 能用 5 分钟复述“为什么会放行/为什么会拒绝”，而不需要翻多处实现细节。

1. 请求进入后，先由 AuthN/session 中间件解析 `principal_id`（或 superadmin principal），并明确是否为匿名。
2. tenant app 的请求必须先解析 tenant（Host → tenant_id，fail-closed），并把 `tenant_id` 放入上下文（对齐 `DEV-PLAN-088`）。
3. 计算 Casbin `domain`：tenant app 用 `DomainFromTenant(tenant_id)`；superadmin 控制面固定为 `global`。
4. 计算 Casbin `subject`（Effective Subject）：从 session 读取 `role_slug`，映射为 `role:{slug}`（MVP 单角色）。
5. 计算 `object/action`：由模块级 helper 把路由/handler 映射为固定的 `module.resource` + `read/admin/debug`（对齐 §4.9）。
6. 调用 `authz.Authorize(ctx, Request{subject, domain, object, action})` 得到 allow/deny（由 `AUTHZ_MODE` 决定是否阻断）。
7. `AUTHZ_MODE=enforce` 且 deny：直接返回统一 403（payload/组件口径统一），并记录缺口（missing policy）。
8. `AUTHZ_MODE=shadow` 且 deny：不阻断请求，但记录缺口（日志/诊断），用于补齐策略与收敛 object/action 漂移。
9. `AUTHZ_MODE=disabled`：跳过授权判断（仅用于本地排障/短期止血）；不得在生产作为常态。
10. 无论 Casbin 是否放行，RLS 仍是租户数据隔离的最终兜底；任何跨租户旁路必须走控制面边界与专用 DB role（`DEV-PLAN-081/088`）。

## 6. 实施步骤（Plan → Implement）

1. [ ] 冻结 contracts：在 V4 新仓库落地 `pkg/authz` 的 V4 版本（含 principal subject），并补齐单测覆盖命名规范与 normalize 行为。
2. [ ] 落地 policy SSOT：建立 `config/access/model.conf`、`config/access/policies/**` 与 pack 生成物，并把生成物纳入 CI diff 检查（防止漏提交）；同时落地 `config/access/authz_flags.yaml`（仅 `mode`）。
3. [ ] 接入最小授权点：
   - [ ] `modules/iam`：tenant console（创建/禁用租户、绑定域名、bootstrap）—— 仅 superadmin 可用。
   - [ ] HR 4 模块 UI/API（`orgunit/jobcatalog/staffing/person`）的 read/admin 最小集。
4. [ ] 统一 403/forbidden 输出契约：控制器侧不自造 JSON/HTML；统一走 `pkg/serrors`/通用组件（沿用现仓库口径）。
5. [ ] 建立可复用的“模块 authz helpers”模板（对齐 `DEV-PLAN-016C` 的收敛要求），避免各模块自写 subject/domain 推导逻辑。
6. [ ] 文档与门禁对齐：把 Authz 的触发器、命令入口、以及 policy 维护工作流写入新仓库的 `AGENTS.md`/`CONTRIBUTING.MD`。

## 7. 测试与覆盖率（V4 新仓库 100% 门禁）

> 对齐 `DEV-PLAN-088` 的 100% 覆盖率要求：Authz 代码必须通过“可测性设计”达成，而不是靠豁免目录。

- 覆盖率口径（待新仓库 SSOT 冻结）：默认 line coverage；统计范围应包含 `pkg/authz` 与各模块的授权接入层。
- 排除项原则：仅允许排除生成代码/第三方；不允许排除“难测的业务分支”。
- 最小用例集（必须覆盖）：
  - `role_slug → Effective Subject` 的映射规则（MVP 单角色）
  - subject/domain/object/action 的 normalize 与不变量校验
  - `AUTHZ_MODE` 三态行为（disabled/shadow/enforce）
  - allow/deny 与错误映射（含 missing policy 的诊断信息，若保留）
  - tenant app 与 superadmin 的边界：不同 domain、不同 object 前缀不可互相放行

## 8. 风险与缓解

- **命名漂移**（principal vs user）：缓解——本计划在 §4.3 冻结为 principal，并要求 helpers 模板统一推导。
- **策略分散**（每模块自造 object/action）：缓解——本计划冻结 module 前缀与 action 最小集合；新增必须走 dev-plan。
- **运维复杂度**（apply 在线写文件）：缓解——V4 baseline 不纳入 apply；需要时另起 092A 并补齐持久化与回滚。

## 9. 验收标准（本计划完成定义）

- [ ] V4 的 `subject/object/action/domain` 命名规范在文档与代码中一致，并有测试兜底。
- [ ] policy SSOT 清晰：策略碎片可追踪、聚合产物可复现、CI 能阻止漏提交。
- [ ] 最小授权闭环可演示：登录（088）→ 进入租户 → 访问受保护页面/API → 无权返回统一 403。
- [ ] `AUTHZ_MODE` 三态行为符合 §4.8，且 `authz_flags.yaml` 不允许出现 `segments`。
- [ ] 触发器与门禁在新仓库可执行（以 SSOT：`Makefile`/CI workflow 为准）。

## 10. Simple > Easy Review（DEV-PLAN-045）

### 10.1 边界
- Authz 只做“是否允许做事”；RLS 只做“同租户可见性”；不要互相越界代偿。

### 10.2 不变量
- domain 只能是 tenant UUID / global。
- subject 必须是 principal/role 的规范形式，禁止模块自造。

### 10.3 停止线（命中即打回）
- [ ] 在模块里手写 subject/domain 推导（应复用 `pkg/authz` helpers）。
- [ ] 用放宽 RLS policy 实现跨租户控制面需求（必须走控制面边界与专用 role）。
- [ ] 引入 `segments` 或其他“看起来可配置但运行时不生效”的 flags 扩展（必须先实现并加测试，再引入配置）。
- [ ] 引入第二套 policy SSOT（例如同时以 DB 与 Git 为权威）而无明确迁移与回滚计划。
