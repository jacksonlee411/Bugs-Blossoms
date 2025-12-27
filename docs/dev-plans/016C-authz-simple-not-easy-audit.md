# DEV-PLAN-016C：Authz（Casbin）模块“简单而非容易”评审发现与整改计划

**状态**: 草拟中（2025-12-27 03:01 UTC）

> 本文档定位：依据 `docs/dev-plans/045-simple-not-easy-review-guide.md` 的“Simple > Easy”评审准则，对仓库 Authz（Casbin）相关实现进行结构化审查，并把需要收敛的**契约（边界/不变量/语义）**先固化为文档，再进入实现整改（契约文档优先）。本文结构对齐 `docs/dev-plans/001-technical-design-template.md`（TDD Template）。

## 1. 背景与上下文 (Context)
Authz 是跨模块横切能力：一旦“边界/语义”发生漂移，短期可能仍可运行（尤其在 shadow 模式下），但长期会积累为“可运行但不可理解/不可演化”的债务。

本计划针对当前 Authz 代码与接入层出现的结构性复杂度（重复逻辑、语义漂移、边界泄漏）做收敛，目标是把系统拉回“简单（Simple）”而不是“容易（Easy）”。

**范围（本计划可能命中的代码/配置）**：
- `pkg/authz/**`（mode/评估入口/边界约束）
- `modules/core/authzutil/**`（ViewState/Forbidden payload/通用 helper）
- `modules/*/presentation/controllers/*authz*`（接入层的统一与去漂移）
- `config/access/authz_flags.yaml`（表达与实现对齐）
- `pkg/htmx/**`（结构化 header 构造）

**相关计划/文档（语义对齐的事实源）**：
- shadow/enforce 的语义与灰度目标：`docs/dev-plans/013-casbin-infrastructure-and-migration.md`
- Core/HRM/Logging 落地与历史决策：`docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`
- 策略草稿/生效与运维入口：`docs/runbooks/authz-policy-apply-api.md`

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [X] 补齐并细化本计划为 TDD 级可执行方案（对齐 `docs/dev-plans/001-technical-design-template.md`）—— 2025-12-27 03:01 UTC
- [ ] 固化 Authz 的关键契约：Domain 语义、ViewState 语义、Mode 行为、不变量与错误契约。
- [ ] 消除重复与漂移：收敛多处重复的 `enforceRequest`，避免“同一概念多套实现”。
- [ ] 修复高风险语义问题：避免 shadow 下“看似可用”、enforce 下“突然全拒”的隐性缺陷。
- [ ] 收紧边界：避免外部代码通过 Enforcer 指针写入策略导致并发与一致性不可证明。

### 2.2 非目标
- 不在本计划内更换 Casbin 或重写授权模型（`config/access/model.conf` 保持兼容）。
- 不在本计划内重做 UI/交互（仅在必要时做最小契约修正）。
- 不在本计划内引入“复杂表达式”的分段配置（例如多层级条件、运行时 DSL）；但允许实现与现有 `authz_flags.yaml` 结构一致的**最小 segment mode**，以消除表达/实现漂移（见 §5.5）。

### 2.3 工具链与门禁（SSOT 引用）
> 本节只声明触发器与事实源；命令细节以 `AGENTS.md` / `Makefile` / CI 为准。

- **触发器清单（本计划命中）**：
  - [ ] Authz（`config/access/**` / `pkg/authz/**` / `scripts/authz/**` 等）
  - [ ] Go 代码（若实施阶段修改实现与测试）
  - [X] 文档（本 dev-plan）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`
  - Authz 运维与 apply API：`docs/runbooks/authz-policy-apply-api.md`
  - Simple > Easy 评审准则：`docs/dev-plans/045-simple-not-easy-review-guide.md`

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 当前授权链路（概览）
```mermaid
graph TD
    A[HTTP Request] --> B[modules/*/presentation/controllers/*authz*]
    B --> C[modules/core/authzutil EnsureViewState/ForbiddenPayload]
    B --> D[pkg/authz.Service Authorize/Check]
    D --> E[casbin.Enforcer Enforce]
    C --> F[pkg/authz.ViewState (Tenant/Capabilities/MissingPolicies)]
```

### 3.2 评审发现（聚焦“漂移点”）
- **重复逻辑**：`enforceRequest` 在多个模块 controller helper 中同构复制（Core/Person/Logging），变更不具备局部性。
  - `modules/core/presentation/controllers/authz_helpers.go`
  - `modules/person/presentation/controllers/authz_helpers.go`
  - `modules/logging/presentation/controllers/authz_helpers.go`
- **shadow/enforce 行为不一致**：部分模块在 `ModeShadow` 下仍会阻断请求；另一些模块按“shadow 不阻断，仅记录 MissingPolicies”实现。
  - 阻断型：`modules/person/...`、`modules/logging/...`
  - 旁路型：`modules/org/presentation/controllers/authz_helpers.go`（可作为基线实现）
- **ViewState 语义污染（高风险）**：曾有接入点将模块名写入 `authz.ViewState.Tenant`，导致 debug URL、MissingPolicies 的 domain 语义漂移；已在 PR-1 中移除（见 §8.2）。
  - `modules/person/presentation/controllers/authz_helpers.go`（已移除模块名写入）
  - `modules/logging/presentation/controllers/authz_helpers.go`（已移除模块名写入）
- **表达能力大于实现能力**：`config/access/authz_flags.yaml` 含 `segments`，但当前 `pkg/authz.FileFlagProvider` 仅消费 `mode`。
- **“半支持”路径**：stage/store 入口拒绝 `g2`，但预览/selector 仍保留 `g2` 分支；同时 testkit seed 仍写入 `g2`。
  - `modules/core/presentation/controllers/policy_stage_store.go`（reject `g2`）
  - `modules/core/presentation/controllers/authz_selector_options.go`（读取 `g2`）
  - `modules/testkit/services/populate_service.go`（写入 `g2`）
- **边界泄漏**：`pkg/authz.Service.Enforcer()` 暴露指针且被外部写入策略，违背 read-only 注释契约。
- **结构化 header 拼接**：`pkg/htmx` 的 `Hx-Trigger` 等采用字符串拼接，缺少明确的输入约束/escape 策略。

### 3.3 关键设计决策（ADR 摘要）
- **决策 1：以 `pkg/authz` 的 mode 语义为准**：shadow 不阻断、enforce 阻断、disabled 旁路；模块如需“强制保护”，应通过 mode 配置实现，而不是在 controller 层绕过 mode。
- **决策 2：`authz.ViewState.Tenant` 固定为 Casbin domain**：即 tenant domain（`authz.DomainFromTenant(tenantID)`）或显式 `global`；模块/segment 的展示需求通过独立字段承载（见 §5.2）。
- **决策 3：引入统一“单次评估”入口**：在 `pkg/authz` 增加 `(*Service).Decide(ctx, req)`（或等价 helper）用于返回 `(mode, allowed)` 并在 enforce 下产出标准 forbidden error，从根源消除 `Authorize` + `Check` 双重评估。
- **决策 4：消除配置漂移**：对 `authz_flags.yaml` 的 `segments` 采取“实现最小 segment mode”的策略（object 前缀作为 segment），使配置表达与运行时行为一致（见 §5.5）。
- **决策 5：g2 的定位**：保留 runtime model 的 `g2`（用于全局角色/系统内部用例），但 **policy stage/apply UI 默认不支持编辑 g2**；实现必须做到“明确拒绝 + 无死代码分支”（见 §5.6）。
- **决策 6：收紧 Enforcer 可变性边界**：禁止业务代码获取 enforcer 指针；如确需写入（测试/seed），提供受控 API 并显式约束使用范围（见 §5.7）。

## 4. 数据模型与约束 (Data Model & Constraints)
> 本计划不引入数据库 schema 变更。

- **DB/Migrations**：无新增/修改迁移；无 schema 变更。
- **策略文件**：`config/access/model.conf` 保持兼容；策略聚合文件（如 `policy.csv`）仅在“实施阶段”按现有工作流变更（不在本文复制生成细节）。

## 5. 接口契约 (API Contracts)
> 以下契约是本计划的“验收口径”，实现必须向其对齐。

### 5.1 Domain（`authz.Request.Domain`）语义契约
- **契约**：`authz.Request.Domain` 表示 Casbin domain（tenant domain），其值必须由 `authz.DomainFromTenant(tenantID)` 推导（或显式 `"global"`）。
- **禁止**：把模块名（如 `"logging"`/`"person"`）作为 domain 传入授权判断。

### 5.2 ViewState 语义契约
- **契约**：`authz.ViewState.Tenant` 表示用于授权判断/调试的 domain（tenant domain）。
- **禁止**：把模块名写入 `ViewState.Tenant` 来做 UI 展示；如需展示模块/segment 信息，使用独立字段：
  - 推荐：为 `authz.ViewState` 增加受控 API（例如 `SetMeta(key, val)` / `Meta(key)`），把模块信息写入 `meta["segment"]`（或等价字段）。

### 5.3 Mode 行为契约（shadow/enforce/disabled）
- **契约**：对同一个请求，在一次授权路径中应只做一次“决定性评估”。
  - enforce：未授权必须返回标准 forbidden 错误（阻断）。
  - shadow：不因 Casbin 判定而阻断（最终 Outcome 由 legacy/其他 guard/segment 配置决定），但必须能得到“允许/拒绝”的诊断结果并记录（用于 MissingPolicies/日志/指标）。
  - disabled：不做授权判断（decided=false），也不产生 MissingPolicies。

### 5.4 Forbidden payload / Debug URL 契约
- **契约**：Forbidden payload 中 `domain` 与 debug URL 的 `domain` 参数必须是 Casbin domain（tenant domain/global）。
- **禁止**：Forbidden payload/debug URL 出现模块名 domain，导致“补策略补错域”。

### 5.5 `authz_flags.yaml`（表达/实现对齐）契约
- **契约**：若 `config/access/authz_flags.yaml` 存在 `segments`，则运行时必须能按 segment 生效（至少支持 mode 覆盖）。
- **segment 定义**：默认以 `authz.Request.Object` 的前缀（`<segment>.<resource>`）作为 segment（全小写）。
- **fallback**：segment 未配置时使用全局 `mode`。
- **优先级**：`segments.<segment>.mode` > 顶层 `mode` > 代码内默认值（`shadow`）。
- **无效 object**：当 `object` 为空或不含 `.` 时，segment 视为 `global`（只使用顶层 `mode`）。
- **忽略字段（明确声明）**：除 `mode` 与 `segments.*.mode` 之外的字段（例如 `segments.*.flags/rollback/monitor`）均视为“文档注释”，运行时解析应忽略它们；避免“文件写了但实现没消费”的漂移。
- **最小 YAML 子集（示意）**：
  ```yaml
  mode: shadow # disabled|shadow|enforce
  segments:
    core:
      mode: shadow
    logging:
      mode: enforce
  ```

### 5.6 Policy stage/apply 契约
- **契约**：stage/apply 支持的策略类型必须单一且明确（`p/g`），并在入口处校验不变量（subject/domain/object/action 等）。
- **g2 策略**：
  - policy stage/apply UI：明确拒绝 `g2`（返回一致的 4xx 错误），并移除任何 stage-only 的 `g2` 死代码分支。
  - runtime：保持 `g2` 在 model 中的支持（用于系统内部/seed），但不通过 UI 管理。

### 5.7 Enforcer 可变性边界契约
- **契约**：业务代码不得直接持有并修改 casbin enforcer 指针来写策略；策略写入必须通过受控入口（并明确并发/持久化/回滚语义）。
- **受控入口最低要求**：写入时持有写锁（与 `ReloadPolicy` 同级别保护），并明确“仅用于 testkit/seed”。

### 5.8 决策矩阵（SSOT）
> 目的：在进入实施前，先冻结“mode × fallback × 记录行为”的口径，避免 shadow/enforce 的语义继续分叉。

**术语**：
- **Casbin 判定**：对 `authz.Request` 做一次 Enforce 得到的 allow/deny（不等于最终是否阻断）。
- **最终阻断（Outcome）**：controller/service 最终是否返回 403/阻断请求。
- **legacy 兜底**：历史权限体系（例如 `user.Can(legacyPerm)`）对最终 Outcome 的影响，仅用于 shadow 过渡期保持行为稳定。

**统一口径（推荐接入策略）**：

| 场景 | mode | legacy 兜底 | Casbin 判定 | 最终阻断 | MissingPolicies | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 旁路 | `disabled` | 不适用 | 不评估 | 否 | 否 | `decided=false`，不产生观测噪音 |
| 影子（有兜底） | `shadow` | 有 | allow/deny | 由 legacy 决定 | deny 时记录 | deny+legacy allow 视为“缺口”，必须可被补策略定位 |
| 影子（无兜底） | `shadow` | 无 | allow/deny | **原则上不应使用** | deny 时记录 | 若必须保护资源，改用 segment `enforce`（见 §5.5） |
| 强制 | `enforce` | 无 | allow/deny | deny 则阻断 | deny 时记录 | deny 必须返回标准 `AUTHZ_FORBIDDEN`（见 §6.1） |

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 统一评估入口：`Service.Decide`
> 目标：单次 Enforce 得到 `(mode, allowed)`，并在 enforce 下生成标准 forbidden error；shadow 下记录 deny 日志；disabled 旁路。

**Go 签名草案（建议）**：
```go
type Decision struct {
	Mode    Mode
	Allowed bool
	Decided bool // disabled=false
}

func (s *Service) Decide(ctx context.Context, req Request) (Decision, error)
```

伪代码（示意）：
1. 解析 `mode := ModeFor(req)`（支持 segment override，见 §5.5）。
2. 若 `mode == disabled`：返回 `(mode, allowed=true, decided=false, err=nil)`。
3. 执行一次 `Check`（单次 Enforce）。
4. 若 `mode == enforce && !allowed`：返回 forbidden error（错误码 `AUTHZ_FORBIDDEN`）。
5. 若 `mode == shadow && !allowed`：记录结构化日志（含 subject/domain/object/action）。
6. 返回 `(mode, allowed, decided=true, err=nil)`。

### 6.2 接入层统一处理（shadow 记录 MissingPolicies，enforce 阻断）
接入层应遵循以下统一流程：
1. 确保 `ViewState` 存在（`authzutil.EnsureViewStateOrAnonymous`）。
2. 构造 `authz.Request`：domain 一律来自 tenant domain（见 §5.1）。
3. 调用统一评估入口得到 `(mode, allowed)`：
   - enforce：若返回 forbidden error，则写 403（JSON/页面），并补齐 MissingPolicies。
   - shadow：不因 Casbin 判定阻断；若存在 legacy 兜底，则最终 Outcome 由 legacy 决定（见 §5.8）；若 `allowed=false`，则 `state.AddMissingPolicy(...)` 用于诊断与后续补策略。
   - disabled：不做 authz，也不记录 MissingPolicies（避免噪音）。

## 7. 安全与鉴权 (Security & Authz)
- 本计划本质是“减少歧义与漂移”，但任何整改不得扩大访问范围：
  - 若某模块没有 legacy fallback 且必须保持强制保护，则其 segment 必须配置为 `enforce`（见 §5.5），而不是依赖 controller 绕过 mode。
  - shadow 的目的仅是观测缺口与补齐策略，不应成为“默默放行”的长期状态。
- g2 作为全局角色能力保留在 model，但其管理入口必须收口（见 §5.6），避免 UI 误用导致跨租户风险。

## 8. 依赖与里程碑 (Dependencies & Milestones)
### 8.1 依赖
- `docs/dev-plans/013-casbin-infrastructure-and-migration.md`（mode/shadow 语义与灰度目标）
- `docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`（模块落地与历史决策）
- `docs/runbooks/authz-policy-apply-api.md`（运维入口）

### 8.2 里程碑（建议按 PR 切片）
1. [X] **PR-1：统一 ViewState/domain 语义**（移除模块名写入 `ViewState.Tenant`；修正 MissingPolicies 的 domain；保证 forbidden payload/debug URL 语义一致）。

   PR-1 Checklist（逐条勾选验收）：
   - [X] `authz.ViewState.Tenant` 语义冻结为 Casbin domain（tenant domain/global），禁止写入模块名（见 §5.2）。
   - [X] 移除 `modules/person/presentation/controllers/authz_helpers.go` 与 `modules/logging/presentation/controllers/authz_helpers.go` 中对 `state.Tenant` 的模块名赋值（`person`/`logging`）。
   - [X] `MissingPolicy.Domain` 一律使用 tenant domain（推荐通过 `authz.DomainFromTenant(tenantID)` 推导），Forbidden payload/debug URL 不出现模块名 domain（见 §5.4）。
   - [X] 当前不需要展示“模块/segment”，暂不引入 `ViewState.meta["segment"]`；如将来需要，必须按此方式实现，不得污染 domain（见 §5.2）。
   - [X] 补齐/更新最小测试：覆盖 forbidden payload/debug URL 的 `domain` 参数与 `missing_policies[].domain` 的语义稳定性（至少覆盖 core + logging/person 任一模块）。
2. [ ] **PR-2：引入单次评估入口**（新增 `Service.Decide` 或等价 helper；接入层不再出现 `Authorize` + `Check` 双重评估）。
3. [ ] **PR-3：收敛 controller helper**（移除/合并多份 `enforceRequest`；各模块对齐同一套接入流程）。
4. [ ] **PR-4：对齐 `authz_flags.yaml` 的 `segments`**（实现最小 segment mode 或删除误导字段并补充说明；本文默认选择“实现最小 segment mode”）。
5. [ ] **PR-5：清理 stage/apply 半支持路径**（明确 `p/g`；移除 stage-only 的 `g2` 分支；错误信息对齐）。
6. [ ] **PR-6：收紧 Enforcer 可变性边界**（替换外部写 enforcer 的路径为受控 API；更新 testkit/seed）。
7. [ ] **PR-7：结构化 header 安全构造**（`Hx-Trigger` 等统一 JSON marshal；补充输入约束与测试）。

## 9. 测试与验收标准 (Acceptance Criteria)
- [ ] 所有授权判断的 domain 语义一致：tenant domain（或 global），Forbidden payload/debug URL 不出现模块名 domain。
- [ ] shadow/enforce/disabled 行为一致且可解释：shadow 不因 Casbin 判定阻断但能记录 MissingPolicies（最终 Outcome 对齐 §5.8）；enforce 阻断并返回 `AUTHZ_FORBIDDEN`；disabled 旁路且不产生 MissingPolicies 噪音。
- [ ] 接入层不存在 `Authorize` + `Check` 的双重 Enforce；同一请求在一次授权路径中只做一次决定性评估。
- [ ] controller helper 不再存在多份同构 `enforceRequest` 实现（或已收敛为单一实现）。
- [ ] stage/apply 对 `p/g` 的校验规则一致，且不存在 stage-only 的 `g2` 残留分支；对 `g2` 的拒绝是明确且一致的。
- [ ] 不存在通过 `Enforcer()` 指针在业务代码中写入策略的路径（或已明确隔离为受控/unsafe 且仅用于 testkit/seed）。
- [ ] 命中触发器的门禁在实施阶段全部通过，并在 dev-plan/dev-records 中记录执行时间与结果（SSOT：`AGENTS.md`/`Makefile`）。

## 10. 运维与监控 (Ops & Monitoring)
- **开关策略**：
  - 全局 `mode` 作为默认值；对必须强制保护的模块，用 `segments.<segment>.mode` 覆盖为 `enforce`（见 §5.5）。
- **关键日志**：
  - shadow deny 日志必须包含 `subject/domain/object/action/mode`；便于按租户/模块聚合缺口。
- **回滚**：
  - 以“小步 PR”推进；若出现行为回归，优先回滚单个 PR。
  - mode 回滚：可通过配置将某 segment 从 `enforce → shadow/disabled` 快速止血；策略回滚按 `docs/runbooks/authz-policy-apply-api.md` 执行。

## 11. 参考 (References)
- `docs/dev-plans/001-technical-design-template.md`
- `docs/dev-plans/045-simple-not-easy-review-guide.md`
- `docs/dev-plans/013-casbin-infrastructure-and-migration.md`
- `docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`
- `docs/runbooks/authz-policy-apply-api.md`
