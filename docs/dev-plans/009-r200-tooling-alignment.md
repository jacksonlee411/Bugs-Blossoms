# DEV-PLAN-009：R200 工具链引入路线

**状态**: 规划中（2025-01-14 12:00）

## 背景
- R200 文档在 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md:207-470` 总结了 ERP 项目应优先采纳的工具链（编译期数据访问、Atlas 迁移规划、后台队列、Casbin 授权、事务性发件箱等）。
- 当前仓库虽已建立 DDD + 模块化单体架构（AGENTS.md:6-96）并运行质量门禁（README.MD:28-41），但在上述领域仍以手写脚本或 ad-hoc 方案为主，缺乏统一工具化支撑。
- 为避免重复造轮子，需要制定一份与 R200 指南对齐的引入路线图，明确评估、PoC、集成与文档同步步骤。

## 目标
- 分阶段评估并引入 R200 建议的关键工具/模式（sqlc/Ent、Atlas+goose、Asynq/Faktory、Casbin、Transactional Outbox）。
- 让每项引入都有可复现的 PoC、CI/Makefile 集成步骤与文档更新计划。
- 在 dev-plan 体系中持续记录决策与实施结果，供团队和新成员追踪。

## 重点方案
1. **数据访问生成器（sqlc 或 Ent）**
   - 依据 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md:207-320`，优先在新模块试点 sqlc（SQL-first，零运行时开销）或 Ent（Go-first，重构友好）。
   - 任务：确定事实来源（SQL vs Go）、整理示例查询、在 Makefile/CI 中增加 `sqlc generate` 或 `ent generate` 流程，并在 CONTRIBUTING 中说明如何同步生成代码。

2. **Atlas + goose/golang-migrate 联动**
   - R200 建议使用 Atlas diff 自动生成 up/down SQL，再由 goose/golang-migrate 执行（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:247-320`）。
   - 任务：编写 `schema.hcl`（或声明式 schema 文件）+ Atlas 配置，新增 `make db plan`/`make db apply` 等目标，并在 CI 中运行 atlas 计划校验，减少手写 down 造成的风险。

3. **持久化后台任务队列（Asynq / Faktory）**
   - 文档指出 ERP 中的报表/导入/通知应由可靠队列驱动，禁止在 HTTP 请求中直接 `go func`（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:320-420`）。
   - 任务：梳理现有后台任务，选择 Asynq（依赖 Redis，Go 原生）或 Faktory（独立 server，支持多语言），编写 PoC 并在 README/运营文档中说明部署依赖与监控方式。

4. **Casbin 授权引擎**
   - R200 强调 ERP 权限需要 RBAC+ABAC 组合，推荐 Casbin（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:420-470`）。
   - 任务：盘点现有权限逻辑，确定模型（如 RBAC with domains + ABAC 表达式），在 `modules/*/permissions` 或 `config/access` 维护 model/policy 文件，并在 `pkg/authz` 构建统一的 Casbin 适配层。

5. **事务性发件箱（Transactional Outbox）**
   - 为保证模块间异步事件与数据库状态一致，R200 建议实现 outbox 表 + relay（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:360-420`）。
   - 任务：设计 outbox schema、插入/轮询逻辑（可先接内存事件总线），在至少一个业务流程（例如 Inventory→Finance）完成端到端验证，并在文档中记录落地指南。

## 风险
- 同期推进多项工具会增加学习与维护成本，需明确优先级并逐步集成。
- 引入第三方 CLI（sqlc/ent/atlas/asynq）需在 tools.go 固定版本，避免 CI/本地环境差异。
- Outbox、队列等方案涉及数据库和系统流程变更，需要完善回滚与监控策略。

## 实施步骤
1. [ ] 召开评审会议，按业务痛点确定引入顺序（建议：先 sqlc/Ent + Atlas，再队列、Casbin、Outbox）。
2. [ ] 为每项撰写 PoC / spike issue，评估依赖与工作量，并记录在 docs/dev-records 中。
3. [ ] 根据 PoC 结论输出决策（采纳/暂缓）并更新本计划。
4. [ ] 正式集成：更新 go.mod/tools.go、Makefile、CI workflow，补充迁移/测试/回滚指南。
5. [ ] 同步文档：在 README、AGENTS、CONTRIBUTING 等处新增“工具链要求”说明，确保新成员了解命令与依赖。

## 里程碑
- M1：完成优先项评估与 PoC 结论归档。
- M2：首个工具（预计为 sqlc/Ent 或 Atlas）纳入 Makefile/CI 并在模块中落地。
- M3：后台队列、Casbin、Outbox 至少完成一个模块级实践，并形成复盘文档。

## 交付物
- 本 dev-plan 文档及后续更新记录。
- 各工具所需配置（如 `sqlc.yaml`、`ent/schema`、`schema.hcl`、`casbin/model.conf` 等）。
- 更新后的 README/AGENTS/CONTRIBUTING 章节。
- PoC 与验证输出（命令记录、日志、示意图），证明方案与 R200 指南一致。
