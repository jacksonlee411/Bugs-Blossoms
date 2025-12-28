# DEV-PLAN-065：组织架构详情页显示“组织长名称”（TDD）

**状态**: 已完成（2025-12-27 10:20 UTC）

## 1. 背景与上下文 (Context)
- Org UI 的“组织架构”入口（`/org/nodes`）右侧面板当前仅展示节点的短名称（`name`）与少量字段（code/status 等）。
- 在多层级组织、或存在同名部门/多次 Move/Rename 的场景下，仅靠短名称难以判断节点的真实上下文（属于哪条路径、上级链路是什么）。
- 现有后端能力已提供“组织长名称（long_name）”读时派生能力（DEV-PLAN-068，落地在 `pkg/orglabels`），可用于在 UI 以 as-of day 快照渲染 root→self 的路径名称串；无需引入冗余存储字段。

**相关 SSOT/依赖**
- 路径查询契约：`docs/dev-plans/033-org-visualization-and-reporting.md`（`GET /org/api/nodes/{id}:path`）
- Org UI（树 + 右侧详情面板）：`docs/dev-plans/035-org-ui.md`
- 组织长名称投影（SSOT）：`docs/dev-plans/068-org-node-long-name-projection.md`

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 核心目标
- [ ] 在 Org 节点详情面板中新增只读字段“组织长名称”，展示 root→当前节点的路径名称串（示例：`Company / Engineering / Platform`）。
- [ ] “组织长名称”随页面 `effective_date`（as-of）变化：同一节点在不同 as-of 下应展示对应时间切片的路径名称。
- [ ] 不存储该字段（不引入写放大）；仅在读时由路径查询结果拼接得到。
- [ ] 失败路径不阻断渲染：路径查询失败时页面不 500，仍可展示既有详情（长名称置空或兜底为短名称）。

### 2.2 非目标 (Out of Scope)
- 不新增/调整 DB 表结构与迁移（不新增任何 `long_name` 持久化字段）。
- 不新增新的 JSON API endpoint（复用 033 的路径查询能力/服务内部方法）。
- 不在本计划内引入“面包屑导航/可点击跳转链”（本期只展示长名称；交互增强另开计划）。
- 不在本计划内改造 i18n 名称策略（长名称展示复用当前 as-of 的 `name` 字段；多语言优先级与回退规则保持不变）。

## 2.3 工具链与门禁（SSOT 引用）
> 本计划涉及 UI（`.templ`）、Go（controller/viewmodel）、以及 i18n JSON；命令细节以 SSOT 为准。

- **触发器清单（实施阶段将命中）**：
  - [ ] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] `.templ` / Tailwind（`make generate && make css`，并确保生成物提交）
  - [ ] 多语言 JSON（`make check tr`）
  - [X] 文档（本计划）：`make check doc`
- **SSOT 链接**：
  - 触发器矩阵与本地必跑：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 方案概述 (Approach)
### 3.1 展示定义
- **字段名（UI 文案）**：`组织长名称`
- **字段含义**：以 root→self 的路径顺序拼接当前节点的 as-of 名称（`name`），分隔符为 ` / `。
- **示例**：`Company / Engineering / Platform`

### 3.2 数据来源（复用既有能力）
- 使用 `pkg/orglabels.ResolveOrgNodeLongNamesAsOf(...)` 的投影结果，以 page as-of 的 `effective_date` 作为查询点（对齐 DEV-PLAN-068 的契约与拼接/兜底规则）：
  - UI 侧直接调用 `orglabels`，避免 UI 再走 HTTP 回环调用自身 API。
  - **边界约束**：仅在渲染“节点详情面板”的 handler 中为“当前选中节点”补齐一次 `LongName`；不要把该调用下沉到 `getNodeDetails(...)`（该函数被多处复用，容易引入隐性额外查询与 N+1）。

### 3.3 拼接规则（消除歧义）
对路径节点数组 `path.nodes[]` 做如下处理后拼接：
1. 取 `name`，并 `strings.TrimSpace`；
2. 若 `name` 为空，回退到 `code`；
3. 若 `code` 也为空，回退到 `id`（UUID 字符串）；
4. 用 ` / ` 连接为最终展示值。

## 4. 契约（UI / ViewModel）
### 4.1 ViewModel 扩展
- 在 `modules/org/presentation/viewmodels/node.go` 的 `OrgNodeDetails` 增加字段：
  - `LongName string`：组织长名称（默认空字符串）。

### 4.2 模板渲染
- 在 `modules/org/presentation/templates/components/orgui/node_details.templ` 中：
  - 在节点标题区新增一行展示“组织长名称”（若 `LongName` 为空则显示 `—`）。

### 4.3 多语言 keys
- 在 `modules/org/presentation/locales/*.json` 中新增：
  - `Org.UI.Node.Fields.LongName`

## 5. 实施步骤 (Tasks)
1. [X] ViewModel：为 `OrgNodeDetails` 增加 `LongName` 字段，并调整相关 mapper/表单回显初始化（仅影响详情展示的字段可置空）。
2. [X] Controller：在渲染节点详情面板时，为“当前选中节点”调用 `pkg/orglabels.ResolveOrgNodeLongNamesAsOf(...)` 获取 `LongName`；失败路径仅置空/兜底，不返回错误（避免页面 500）。
3. [X] Templ：在 `node_details.templ` 渲染 `LongName`，并补齐 i18n 文案。
4. [X] 验证：本地门禁已通过（`make generate && make css && go fmt ./... && go vet ./... && make check lint && make test && make check doc && make check tr`，2025-12-27 10:20 UTC）。

## 6. 验收标准 (Acceptance Criteria)
- [ ] `/org/nodes?effective_date=YYYY-MM-DD&node_id=<id>` 的节点详情面板中可见“组织长名称”，值为 root→self 的路径拼接串。
- [ ] 切换 `effective_date` 后，“组织长名称”随 as-of 变化（至少覆盖：节点或其祖先存在历史重命名切片的场景）。
- [ ] 路径查询失败时不影响页面其他字段渲染：不 500，长名称显示 `—` 或短名称兜底。

## 7. 回滚方案 (Rollback)
- [ ] 代码回滚：`git revert` 回滚本变更 PR/commit。
- [ ] 数据回滚：无数据变更。

## 8. DEV-PLAN-045 评审（Simple > Easy）
### 结构（解耦/边界）
- [X] 变更局部：仅在 Org 节点详情面板新增只读展示字段；不引入新 service/新模型/新 API。
- [X] 边界清晰：`LongName` 只属于展示层 ViewModel，不进入写模型与持久化。

### 演化（规格/确定性）
- [X] 有对应 Spec（本 dev-plan）：定义拼接规则、失败路径、验收与回滚口径。
- [X] 依赖复用且可替换：复用 033 的 `GetNodePath` 能力；若后续需要从 `GetHierarchyAsOf` 结果直接推导，可在不改 DB/契约的情况下替换实现。

### 认知（本质/偶然复杂度）
- [X] 不变量明确：不存储长名称；长名称随 `effective_date`（as-of）变化；失败不阻断页面渲染。

### 维护（可理解/可解释）
- [X] 5 分钟可解释：取节点详情（短名）→ 取节点路径（root→self）→ 拼接 → 渲染/兜底。

结论：通过（注意：不要把 `GetNodePath` 下沉到通用 `getNodeDetails(...)`，避免扩大影响面与隐性查询成本）。
