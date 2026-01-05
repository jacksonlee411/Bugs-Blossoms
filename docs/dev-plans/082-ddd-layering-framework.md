# DEV-PLAN-082：DDD 分层框架方案（对齐 CleanArchGuard + v4 DB Kernel）

**状态**: 草拟中（2026-01-05 03:00 UTC）

## 1. 背景与上下文 (Context)

本仓库已采用“模块化单体 + DDD 分层”的目录形态：`modules/{module}/{domain,infrastructure,services,presentation}/`，并用 CleanArchGuard（配置见 `.gocleanarch.yml`，入口 `make check lint`）自动阻断跨层非法依赖。

同时，`DEV-PLAN-077`～`DEV-PLAN-080` 明确了一条在 Org v4 系列中将“领域内核下沉到 DB（Kernel）”的路线：**DB 负责不变量与投射（权威），Go 只做鉴权/事务/调用与错误映射（Facade）**，并以 **One Door Policy（写入口唯一）** 防止实现期漂移。

上述两者叠加后，项目需要一份“可执行且可审查”的 DDD 分层框架，统一回答：
- 每层职责边界是什么？哪些代码应该放在哪里？
- 当领域逻辑位于 DB Kernel 时，Go 的 DDD 四层应如何表达“契约”而不重复实现？
- 模块之间如何共享能力而不破坏边界（尤其避免跨模块直接 import）？

### 1.1 本计划定位：Greenfield（从 0 开始重构）
本计划按 Greenfield（从 0 开始）口径编写：
- 本计划提供“新体系的默认形态、红线与模板”，不以“兼容/迁移/复用旧实现”为前置。
- 对存量模块的“如何迁移/如何退场/如何逐步替换”不在本计划范围内（如需承接，另立 dev-plan）。

## 2. 目标与非目标 (Goals & Non-Goals)

### 2.1 核心目标
- [ ] 形成一套与 `.gocleanarch.yml` 一致的 DDD 分层框架：职责、目录、依赖规则可直接用于 code review 与 lint。
- [ ] 明确“常规 Go DDD”与“DB Kernel + Go Facade（077-080）”两类落地形态的统一口径，避免同一能力出现两套权威表达。
- [ ] 明确“模块间共享”的推荐路径（优先 `pkg/**`），降低 CleanArchGuard allowlist/ignore 的增长压力。
- [ ] 给出可操作的落地清单（最小骨架/决策树/停止线）与验收标准，确保实现阶段不依赖“对话试错”。

### 2.2 非目标（明确不做）
- 不在本计划内重构现有模块的全部目录与代码形态（以框架与约束为主，实施另开子计划/PR 承接）。
- 不在本计划内修改/扩展 `.gocleanarch.yml` 的规则（如需新增层级别名或收紧策略，另立计划并给出影响评估）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划为“架构规范”类文档，命中的门禁以 SSOT 为准；本文不复制门禁矩阵与脚本实现。

- 触发器矩阵与本地必跑：`AGENTS.md`
- 分层与依赖门禁（CleanArchGuard）：`.gocleanarch.yml`（入口：`make check lint`）
- v4 DB Kernel 边界 SSOT：`docs/dev-plans/077-org-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/078-org-v4-full-replacement-no-compat.md`、`docs/dev-plans/079-position-v4-transactional-event-sourcing-synchronous-projection.md`、`docs/dev-plans/080-job-catalog-v4-transactional-event-sourcing-synchronous-projection.md`

## 3. 分层框架总览（DDD + Ports & Adapters）

### 3.1 概念层与物理层的对应关系

本仓库物理层级与 Clean Architecture/Hexagonal 的概念映射如下（与 `.gocleanarch.yml` 一致）：

- `domain/`：Domain Layer（模型/不变量/领域事件/端口定义）
- `services/`：Application Layer（用例编排，事务边界，调用端口，返回应用级结果）
- `presentation/`：Interface / Delivery（HTTP/HTMX/templ/controller/viewmodel/mapper）
- `infrastructure/`：Infrastructure / Adapters（DB/外部系统/缓存/消息等技术实现）
- `modules/{module}/module.go`、`modules/{module}/links.go` 等：Composition Root（组装与注册；允许依赖各层，但必须“薄”）

### 3.2 依赖规则（必须可用 lint 验证）

**硬规则（必须）**：
- `domain` 只能依赖：同模块 `domain`、以及 `pkg/**`（符合 `.gocleanarch.yml`）。
- `services` 只能依赖：同模块 `domain/services`、以及 `pkg/**`。
- `presentation` 只能依赖：同模块 `services/presentation`、以及 `pkg/**`。
- `infrastructure` 可以依赖：同模块 `domain/services/infrastructure`、以及 `pkg/**`、`internal/**`。

**软规则（建议）**（避免“可以但不该”的依赖扩散）：
- 模块间共享能力优先下沉 `pkg/**`，避免跨模块直接 import `modules/{other}/...`。
- 尽量避免 `infrastructure -> presentation` 的反向依赖；若需要“挂接”，优先在 Composition Root 组装或通过 `pkg/**` 提供稳定接口。

### 3.3 每层职责（Do / Don’t）

**Domain（`domain/`）**
- Do：表达业务概念（实体/值对象/聚合）、领域不变量、领域事件、以及端口（Repository/Gateway）接口。
- Don’t：依赖 DB/HTTP/框架；不要出现 pgx/sqlc/htmx/templ 等技术细节；不要把“用例流程”放到 domain。

**Application（`services/`）**
- Do：用例编排（权限/租户上下文、事务边界、调用端口、将错误映射为 `pkg/serrors` 等稳定错误语义）。
- Don’t：不要承载 UI 细节（templ/viewmodel），不要直接写 SQL（除非被明确选定为“DB Kernel 调用”且通过端口抽象/集中封装）。

**Presentation（`presentation/`）**
- Do：协议适配（HTTP/HTMX）、输入校验与绑定、DTO/VM 映射、调用 services。
- Don’t：不要包含业务规则；不要绕过 services 直接访问 infrastructure。

**Infrastructure（`infrastructure/`）**
- Do：实现端口（Repository/Gateway）、承载 SQL/模型映射、实现外部系统适配。
- Don’t：不要把业务流程写成“技术实现细节”；不要把领域不变量只写在 Go 而不给 DB 约束（除非该不变量不属于 DB 能表达的范围且已在计划文档中说明）。

**Composition Root（`module.go/links.go` 等）**
- Do：依赖注入、注册路由/控制器/服务、拼装 infra 实现。
- Don’t：不要堆业务逻辑；不要变成“第二个 services”。

## 4. 两种落地形态（统一口径）

### 4.1 形态 A：常规 Go DDD（Domain 在 Go）

适用场景：业务不变量主要在应用内表达即可，或 DB 约束只做兜底；读写路径不需要“可重放投射”作为核心能力。

推荐做法：
- Domain 中定义聚合与不变量（必要时配合 DB 约束）。
- services 中定义用例（Command/Query），通过端口接口操作持久化与外部系统。
- infrastructure 实现端口（repo/sqlc/pgx），并在 Composition Root 注入。

### 4.2 形态 B：DB Kernel + Go Facade（对齐 077-080）

适用场景：不变量高度依赖关系型约束/有效期切片/可重放投射；或系统选定 “DB=权威内核、Go=薄编排” 的简化路线。

**统一口径（必须与 077-080 一致）**：
- **DB = Projection Kernel（权威）**：事件写入（幂等）+ 同事务同步投射（delete+replay）+ 不变量裁决 + 可 replay。
- **Go = Command Facade**：鉴权/租户与操作者上下文 + 事务边界 + 调 Kernel + 错误映射到 `pkg/serrors`。
- **One Door Policy（写入口唯一）**：除 `submit_*_event` 与运维 replay 外，应用层不得直写事件表/versions 表/identity 表，不得直调 `apply_*_logic`。

**在四层目录中的表达方式（推荐）**：
- `infrastructure/persistence/schema/**`：DB Kernel 的 schema/函数/约束（SSOT 文件路径以对应模块工具链为准；如 Org 见 078）。
- `domain/`：承载“稳定业务概念与契约类型”（IDs、枚举、命令入参类型、稳定错误码常量等），但不复写 Kernel 的裁决逻辑。
- `services/`：只做 Facade：事务 + 调用 Kernel 端口 + 错误映射；避免在 Go 写“第二套投射/校验”。
- `infrastructure/`：提供 Kernel 端口实现（例如用 pgx 调 `submit_*_event`），并在 Composition Root 注入到 services。

> 备注：该形态本质上是“领域内核在 DB”，Go 的 domain 更接近“领域契约层（types + ports）”。这是有意识的取舍，必须用 One Door Policy 防止权威表达分裂。

### 4.3 形态选择决策树（Greenfield 默认）
> 目的：把“选 A 还是选 B”从实现期争论前移到计划期，避免实现期产生两套权威表达。

- 若满足任一条件，默认选 **形态 B（DB Kernel）**：
  - 有效期切片（Valid Time）为核心语义，且需要稳定可重放（retro）与 as-of 读一致性。
  - 不变量依赖关系型约束表达（no-overlap/gapless/acyclic/同日唯一）且希望 DB 作为最终裁判。
  - 写路径需要 One Door Policy（唯一写入口）来压缩边界与减少漂移风险。
- 否则可选 **形态 A（Go DDD）**：
  - 不变量主要是应用内规则，DB 约束仅做兜底；且不需要“事件 SoT + 同步投射”的固定机制。

## 5. Greenfield 最小骨架（模板）
> 目的：让新模块/新子域可以按模板开工，避免“先写能跑的再回填边界”。

### 5.1 最小骨架：所有模块通用（必须）
```
modules/<module>/
  domain/
  services/
  infrastructure/
  presentation/
  module.go
  links.go (若该模块需要路由挂载)
```

硬要求：
- 新代码不得新增非四层目录作为默认承载（见 §8 停止线）。
- `module.go` 仅做组装与注册，不允许承载用例逻辑。

### 5.2 形态 A（Go DDD）最小骨架（建议）
```
modules/<module>/domain/ports/
modules/<module>/domain/aggregates/ (可选)
modules/<module>/services/
modules/<module>/infrastructure/persistence/
modules/<module>/presentation/controllers/
```

### 5.3 形态 B（DB Kernel）最小骨架（建议）
```
modules/<module>/domain/ports/              # Kernel Port（接口）
modules/<module>/domain/types/              # 命令入参类型/枚举/稳定错误码（可选）
modules/<module>/services/                  # Facade：Tx + 调 Kernel + serrors 映射
modules/<module>/infrastructure/persistence/# Kernel Port 实现（pgx/sqlc 调用）
modules/<module>/infrastructure/persistence/schema/ # Kernel schema/函数/约束（SSOT）
modules/<module>/presentation/controllers/  # Delivery
```

红线（必须）：
- Go 不得实现任何“投射/裁决/重放”的第二套逻辑；只允许调用 Kernel 的唯一入口（对齐 077-080）。
- `apply_*_logic`（若存在）仅作为 Kernel 内部实现细节，禁止被应用角色直接调用。

## 6. 目录与命名约定（建议项）

> 目的：减少“每个模块各长一套目录”的漂移，让 reviewer 只看路径就能判断代码是否在正确层。

### 6.1 module 内推荐子目录
- `domain/aggregates/**`：聚合（可选）
- `domain/events/**`：领域事件（若使用）
- `domain/ports/**`：端口接口（repository/gateway/kernel port）
- `services/commands/**`、`services/queries/**`：用例（可选；模块小可不拆）
- `infrastructure/persistence/**`：repo、models、mappers、sqlc（若使用）
- `infrastructure/persistence/schema/**`：schema/函数（若使用 DB Kernel）
- `presentation/controllers/**`、`presentation/viewmodels/**`、`presentation/mappers/**`、`presentation/templates/**`、`presentation/locales/**`

### 6.2 非四层目录：Greenfield 默认禁止
为避免出现“绕过分层的逃逸口”，Greenfield 默认口径为：
- **默认禁止**新增 `modules/<module>/*` 下的非四层目录（不含 `module.go/links.go`）。
- 若确需新增，必须在对应 dev-plan 中声明：它属于哪一种 Adapter（Delivery / Infra / Composition），并给出为何不能放入四层目录的理由与验收方式。

## 7. 模块间共享策略（Greenfield 约束）

**优先级（从高到低）**：
1. `pkg/**`：仓库级共享库（推荐；例如跨模块 label 投影类能力已有先例）
2. `modules/core/**`：共享模块（仅当其语义确为“全局内核”）
3. 复制少量非常稳定的“纯类型”到各模块（最后手段，且需明确不变量与演进策略）

**禁止**：
- 跨模块直接依赖对方的 `infrastructure/**` 或 `presentation/**` 以复用实现细节。
- 跨模块依赖对方的 `internal/**`（若确需共享，应提升为 `pkg/**` 或明确 allowlist，并补充文档说明）。

### 7.1 `pkg/**` 准入规则（必须补齐，否则易退化为“万能抽屉”）
仅当满足全部条件时，允许新增/迁移到 `pkg/**`：
- 依赖约束：`pkg/**` 不得依赖 `modules/**`（避免倒灌形成隐式跨模块耦合）。
- 语义约束：只能承载横切能力/通用类型/协议与工具；不得承载某一 bounded context 的领域规则与裁决逻辑。
- 演进约束：必须给出“稳定 API 面”的边界（对外暴露什么、明确不承诺什么），并指定 owner（负责兼容性与重构）。
- 可替换性：调用方应只依赖接口/轻量类型；避免把实现细节（例如 SQL/schema）渗透到调用方。

## 8. 停止线（Stop Lines，按 DEV-PLAN-045）
> 任一命中则在评审阶段打回（拒绝“先写再说”）。

- 发现“第二写入口”：绕过 `submit_*_event`/Kernel 入口直写 events/versions/identity 表，或直调 `apply_*_logic`。
- 引入非四层目录但未在 dev-plan 明确其归属与验收方式。
- 为复用实现而新增跨模块 import（`modules/A` 依赖 `modules/B` 的实现细节），且未先尝试下沉 `pkg/**` 或定义端口。
- 为了“更容易实现”而引入两套权威表达（例如 Go 里再写一套投射/校验来“兜底” DB Kernel）。

## 9. 分步落地路径（每步可验证）
> 命令入口与门禁矩阵以 `AGENTS.md`/`Makefile`/CI 为 SSOT；此处只描述步骤与验收点。

1. [ ] 选择形态（A/B）并在子域 dev-plan 写明理由（对照 §4.3）。
2. [ ] 落模块最小骨架（§5），并确保 `make check lint` 通过（无新增 ignore/allowlist）。
3. [ ] 若为形态 B：先落 DB Kernel 的 schema/函数/约束（SSOT），并完成“唯一入口 + 权限/组织约束”的设计验收（对齐 077-080 的 One Door Policy）。
4. [ ] 落 services Facade（Tx + 调用 Kernel/Ports + `pkg/serrors` 错误映射），并确保调用链不绕过 Facade。
5. [ ] 落 presentation（controllers/templ/htmx）作为纯协议适配；禁止把业务裁决写回 delivery。
6. [ ] 若出现跨模块共享诉求：先按 §7.1 判断是否进 `pkg/**`；若不满足则保持在模块边界内。

## 10. 验收标准（本计划完成的判定）

- [ ] `DEV-PLAN-082` 明确描述：四层职责、依赖规则、两种落地形态（Go DDD / DB Kernel + Go Facade）及其统一口径。
- [ ] 文档中对 v4 Kernel 边界的描述与 077-080 一致（不引入“第二套权威表达”的例外分支）。
- [ ] 提供 Greenfield 可直接复用的：形态选择决策树、最小骨架模板、`pkg/**` 准入规则、停止线与分步验收点。
- [ ] 本文档已加入 `AGENTS.md` 的 Doc Map（可发现性门禁）。
