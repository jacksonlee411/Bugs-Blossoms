# DEV-PLAN-064A：Valid Time 双轨引入复盘：收益、复杂度与收敛方案

**状态**: 已完成（Org 已 date-only 收敛；阶段 D/E 已合并）（2025-12-28 UTC）

> 本文定位：作为 DEV-PLAN-064 的补充调查（A），复盘 Phase 1 采用“date 列 + legacy timestamptz 列”双轨迁移的动机与代价，并记录最终收敛决策与验收标准。

## 1. 结论（TL;DR）
- **最终权威（SSOT）**：DB 仅保留 `date effective_date/end_date`（day 粒度闭区间）作为 Valid Time 的唯一权威表达。
- **约束与查询口径统一**：no-overlap 统一为 `daterange(effective_date, end_date + 1, '[)')`；as-of 统一为 `effective_date <= d AND d <= end_date`。
- **收敛动作**：删除 legacy `timestamptz effective_date/end_date`，移除双轨期间的过渡列与派生逻辑；运行时代码/测试/SSOT 文档不再包含双轨字段名与旧的 timestamp 半开区间口径。
- **API 契约**：Valid Time 输入仅接受 `YYYY-MM-DD`，不再接受 RFC3339 timestamp。

## 2. 为什么 Phase 1 会选择双轨
- **可回滚**：在不立刻破坏既有写路径与历史数据的前提下，引入 date 口径能力，允许灰度切换并保留回退空间。
- **可观测**：双轨期间可以对比两种口径在关键链路（as-of 查询、报表、深读）的一致性，降低一次性切换风险。
- **强约束保持**：仍然依赖 Postgres range + EXCLUDE 的强约束能力，避免业务层“补丁式”边界处理堆叠。

## 3. 双轨带来的复杂度与风险
- **写入漂移风险**：任一路径若绕过集中 helper，只写其中一套口径，将造成窗口不一致与难排障问题。
- **读路径分裂**：repo/sql/query budget/integration tests 需要同时理解两套列与边界语义，维护成本显著上升。
- **字段语义混用**：同名字段在不同阶段承载不同粒度（timestamp vs date）会导致 reviewer 与调用方误解，放大“结束日是否包含”类问题。

## 4. 收敛方案（已实施）
### 4.1 Schema
- in-scope 表的 Valid Time 字段统一为 `date effective_date/end_date`。
- EXCLUDE/索引/check 统一以 date 口径重建，移除旧的 `tstzrange` 约束与相关索引。

### 4.2 Go（Repository / Service / Controller）
- repo as-of 判定统一使用闭区间（`end_date >= as_of_date`），且参数以 date 形式传入（day 归一化）。
- 截断/续接统一按 day 闭区间处理：截断到新生效日 `D` 时，旧段应变为 `end_date = D - 1 day`。
- API 层仅接受 `YYYY-MM-DD`，避免“时间成分”再次污染 Valid Time。

### 4.3 测试与文档
- 集成测试与 seed SQL 统一以 date 写入，不再依赖“减 1 微秒”的窗口派生规则。
- 受影响 dev-plan 文档中与 schema/查询相关的示例统一为最终字段名与口径。

## 5. 验收标准（Gate）
- **Schema**：Org in-scope 表的列定义中不存在任何双轨遗留字段；Valid Time 字段类型为 `date`。
- **代码**：运行时代码与测试中不存在任何双轨遗留字段名；Valid Time 相关 SQL 不依赖 `tstzrange`。
- **接口**：对外/对内 API 的 Valid Time 输入只接受 `YYYY-MM-DD`，并对非法输入返回 422。
