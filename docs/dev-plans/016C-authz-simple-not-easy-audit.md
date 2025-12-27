# DEV-PLAN-016C：Authz（Casbin）模块“简单而非容易”评审发现与整改计划

**状态**: 规划中（2025-12-27 00:00 UTC）

> 本文档定位：依据 `DEV-PLAN-045` 的“Simple > Easy”评审准则，对仓库 Authz（Casbin）相关实现进行结构化审查，并把需要收敛的**契约（边界/不变量/语义）**先固化为文档，再进入实现整改（契约文档优先）。

## 1. 背景与上下文 (Context)
Authz 是跨模块横切能力：一旦“边界/语义”发生漂移，短期可能仍可运行（尤其在 shadow 模式下），但长期会积累为“可运行但不可理解/不可演化”的债务。

本计划针对当前 Authz 代码与接入层出现的结构性复杂度（重复逻辑、语义漂移、边界泄漏）做收敛，目标是把系统拉回“简单（Simple）”而不是“容易（Easy）”。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 固化 Authz 的关键契约：Domain 语义、ViewState 语义、Mode 行为、不变量与错误契约。
- [ ] 消除重复与漂移：收敛多处重复的 `enforceRequest`，避免“同一概念多套实现”。
- [ ] 修复高风险语义问题：避免 shadow 下“看似可用”、enforce 下“突然全拒”的隐性缺陷。
- [ ] 收紧边界：避免外部代码通过 Enforcer 指针写入策略导致并发与一致性不可证明。

### 2.2 非目标
- 不在本计划内更换 Casbin 或重写授权模型（`config/access/model.conf` 保持兼容）。
- 不在本计划内重做 UI/交互（仅在必要时做最小契约修正）。
- 不在本计划内引入复杂的“分段配置”运行时逻辑（除非能证明比现状更简单且可验证）。

### 2.3 工具链与门禁（SSOT 引用）
> 本节只声明触发器与事实源；命令细节以 `AGENTS.md` / `Makefile` / CI 为准。

- **触发器清单（本计划命中）**：
  - [ ] Authz（`config/access/**` / `pkg/authz/**` / `scripts/authz/**` 等）
  - [ ] Go 代码（若实施阶段修改实现与测试）
  - [X] 文档（本 dev-plan）
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - Authz 运维与 apply API：`docs/runbooks/authz-policy-apply-api.md`
  - Simple > Easy 评审准则：`docs/dev-plans/045-simple-not-easy-review-guide.md`

## 3. 评审发现（按 DEV-PLAN-045 四维）
### 3.1 结构维度（解耦 vs 纠缠）
- **重复逻辑**：`enforceRequest` 在多个模块 controller helper 中同构复制（Core/Person/Logging），导致变更不具备局部性。
- **边界泄漏**：`pkg/authz` 暴露 `Enforcer()` 指针且被外部用于写入策略（测试/seed），与“read-only”注释契约冲突。
- **Domain 语义漂移（高风险）**：部分接入点把 domain 固定为模块名（例如 logging），与主线 tenant domain 语义不一致，容易造成 shadow→enforce 行为突变。

### 3.2 演化维度（规格驱动 vs 对话驱动）
- `config/access/authz_flags.yaml` 存在“表达能力大于实现能力”的漂移：文件包含分段信息，但当前 flag provider 仅消费 `mode`；若不澄清，review/运维会被误导。

### 3.3 认知维度（本质逻辑 vs 偶然模式）
- Policy stage / apply 链路存在“半支持”痕迹（例如 `g2` 的残留判断与分支），属于偶然复杂度：读者难以判断系统到底支持什么。

### 3.4 维护维度（可理解 vs 仅可运行）
- shadow 模式在接入层存在“重复评估”的倾向（`Authorize` + `Check`），增加理解成本与潜在性能浪费。
- 部分响应头 JSON 采用字符串拼接（如 `Hx-Trigger`），长期可维护性与鲁棒性偏弱（需明确约束或收敛为安全构造）。

## 4. 需要先固化的契约（Contract First）
> 以下契约是本计划的“验收口径”，实现必须向其对齐。

### 4.1 Domain（`Request.Domain`）语义契约
- **契约**：`Request.Domain` 表示 Casbin domain（tenant domain），其值必须由 `authz.DomainFromTenant(tenantID)` 推导（或显式 `"global"`）。
- **禁止**：把模块名（如 `"logging"`/`"person"`）作为 domain 传入授权判断。

### 4.2 ViewState 语义契约
- **契约**：`authz.ViewState.Tenant` 表示用于授权判断/调试的 domain（tenant domain）。
- **禁止**：把模块名写入 `ViewState.Tenant` 来做 UI 展示；如需展示域/模块信息，应通过独立字段或参数承载（避免污染授权语义）。

### 4.3 Mode 行为契约（shadow/enforce/disabled）
- **契约**：对同一个请求，在一次授权路径中应只做一次“决定性评估”，并且评估语义对 reviewer 可解释：
  - enforce：未授权必须返回 forbidden 错误（阻断）。
  - shadow：不阻断，但必须能得到“允许/拒绝”的诊断结果并记录（用于 MissingPolicies/日志/指标）。
  - disabled：不做授权判断。

### 4.4 Policy stage/apply 契约
- **契约**：stage/apply 支持的策略类型必须单一且明确（例如仅 `p/g`），并在入口处校验不变量（subject/domain/object/action 等）。
- **禁止**：出现“入口拒绝但内部仍按支持处理”的分支残留（例如 `g2` 的半支持状态）。

### 4.5 Enforcer 可变性边界
- **契约**：业务代码不得直接持有并修改 casbin enforcer 指针来写策略；策略写入必须通过受控入口（并明确并发/持久化/回滚语义）。

## 5. 实施步骤（Plan）
1. [ ] **收敛 Domain 语义**：统一所有接入点以 tenant domain 进行授权判断；补充最小测试覆盖 shadow→enforce 的一致性。
2. [ ] **消除重复 enforce 逻辑**：将 controller helper 的 `enforceRequest` 收敛为单一实现（优先放在可复用且依赖方向合理的位置），并避免 shadow 下重复评估。
3. [ ] **收敛 ViewState 语义**：停止将模块名写入 `ViewState.Tenant`；若 UI 需要展示域信息，新增/调整独立字段承载（保持 forbidden payload/debug URL 语义一致）。
4. [ ] **清理 stage/apply 偶然复杂度**：明确支持的 policy 类型（`p/g`），删除 `g2` 残留分支；在 stage/store 与 apply/service 两端对齐校验规则。
5. [ ] **收紧 Enforcer 边界**：禁止外部直接写入 enforcer；为测试/seed 提供受控替代（或明确标注 Unsafe API 并限制使用范围）。
6. [ ] **加固响应头构造**：对 `Hx-Trigger` 等结构化 header，统一使用安全构造（JSON marshal）或明确输入约束，避免字符串拼接隐患。

## 6. 验收标准 (Acceptance Criteria)
- [ ] 所有授权判断的 domain 语义一致：tenant domain（或 global），不存在模块名 domain。
- [ ] shadow/enforce 行为可解释且一致：shadow 仅改变“是否阻断”，不改变“判定语义”。
- [ ] controller helper 不再存在多份同构 `enforceRequest` 实现。
- [ ] stage/apply 支持的策略类型与校验规则一致，且无 `g2` 残留分支。
- [ ] 不存在通过 `Enforcer()` 指针在业务代码中写入策略的路径（或已明确隔离为受控/unsafe）。
- [ ] 命中触发器的门禁在实施阶段全部通过（按 `AGENTS.md` 与 `Makefile` 入口执行并在本计划内记录）。

## 7. 风险与回滚 (Risks & Rollback)
- 风险：收敛 domain/viewstate 语义可能影响部分模块的 shadow 观测与 Debug URL 参数；需要用测试与小步合并降低风险。
- 回滚：实现阶段以“小步 PR”推进；若出现行为回归，优先回滚单个 PR，并保持策略文件/`.rev` 与运行时 Reload 语义一致。

## 8. 参考 (References)
- `docs/dev-plans/045-simple-not-easy-review-guide.md`
- `docs/runbooks/authz-policy-apply-api.md`
- `docs/dev-plans/014-casbin-core-hrm-logging-rollout.md`

