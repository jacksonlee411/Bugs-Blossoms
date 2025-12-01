# DEV-PLAN-009：R200 工具链引入路线

**状态**: 规划中（2025-01-14 12:00）

## 背景
- R200 文档在 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md:207-470` 总结了 ERP 项目应优先采纳的工具链（编译期数据访问、Atlas 迁移规划、后台队列、Casbin 授权、事务性发件箱等）。
- 当前仓库虽已建立 DDD + 模块化单体架构（AGENTS.md:6-96）并运行质量门禁（README.MD:28-41），但在上述领域仍以手写脚本或 ad-hoc 方案为主，缺乏统一工具化支撑。
- 2025-12-01 完成的 DEV-PLAN-005/005T 为全仓库提供了统一的 lint/test/变更追踪脚本（`scripts/run-go-tests.sh`、`quality-gates`），后续在此基础上引入 R200 工具链，可直接挂到既有 Makefile/CI 路径上，避免重复造轮子。
- 为避免重复造轮子，需要制定一份与 R200 指南对齐的引入路线图，明确评估、PoC、集成与文档同步步骤。

## 目标
- 分阶段评估并引入 R200 建议的关键工具/模式（sqlc、Atlas+goose、Asynq、Casbin、Transactional Outbox）。
- 让每项引入都有可复现的 PoC、CI/Makefile 集成步骤与文档更新计划。
- 在 dev-plan 体系中持续记录决策与实施结果，供团队和新成员追踪。

## 重点方案
1. **数据访问生成器（sqlc）**
   - 依据 `docs/dev-records/R200r-Go语言ERP系统最佳实践.md:207-320`，已决定以 sqlc（SQL-first，零运行时开销）作为事实来源工具，暂不评估 Ent，保持 DBA 友好的 SQL 生命周期。
   - 任务：归档需要生成的 SQL 查询、在 Makefile/CI 中增加 `sqlc generate` 流程，并在 CONTRIBUTING 中说明如何同步生成代码及更新 `sqlc.yaml`。

2. **Atlas + goose/golang-migrate 联动**
   - R200 建议使用 Atlas diff 自动生成 up/down SQL，再由 goose/golang-migrate 执行（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:247-320`）。
   - 任务：编写 `schema.hcl`（或声明式 schema 文件）+ Atlas 配置，新增 `make db plan`/`make db apply` 等目标，并在 CI 中运行 atlas 计划校验，减少手写 down 造成的风险。

3. **持久化后台任务队列（Asynq）**
   - 文档指出 ERP 中的报表/导入/通知应由可靠队列驱动，禁止在 HTTP 请求中直接 `go func`，结合当前技术栈已决定采用 Asynq（Go 原生，依赖 Redis），暂不评估 Faktory（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:320-420`）。
   - 任务：梳理现有后台任务，完成 Asynq PoC，固化 producer/worker 模式，在 README/运营文档中说明 Redis/Asynq 依赖、监控方式与回滚策略。

4. **Casbin 授权引擎**
   - R200 强调 ERP 权限需要 RBAC+ABAC 组合，推荐 Casbin（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:420-470`）。
   - 任务：盘点现有权限逻辑，确定模型（如 RBAC with domains + ABAC 表达式），在 `modules/*/permissions` 或 `config/access` 维护 model/policy 文件，并在 `pkg/authz` 构建统一的 Casbin 适配层。

5. **事务性发件箱（Transactional Outbox）**
   - 为保证模块间异步事件与数据库状态一致，R200 建议实现 outbox 表 + relay（`docs/dev-records/R200r-Go语言ERP系统最佳实践.md:360-420`）。
   - 任务：设计 outbox schema、插入/轮询逻辑（可先接内存事件总线），在至少一个业务流程（例如 Inventory→Finance）完成端到端验证，并在文档中记录落地指南。
   - 可选方案：`github.com/ThreeDotsLabs/watermill` SQL Outbox 组件（Postgres/MySQL 轮询 + Relay）、Debezium Outbox Event Router（依赖 Kafka Connect WAL/binlog，将 outbox 表自动投递到 Kafka）、`github.com/looplab/eventhorizon` 等事件溯源框架自带的 outbox/relay，或项目内自研轻量实现（遵循 R200 推荐的事务 + relay 模式并补上幂等/重试）。

## 风险
- 同期推进多项工具会增加学习与维护成本，需明确优先级并逐步集成。
- 引入第三方 CLI（sqlc/atlas/asynq）需在 tools.go 固定版本，避免 CI/本地环境差异。
- Outbox、队列等方案涉及数据库和系统流程变更，需要完善回滚与监控策略。

## 实施步骤
1. **sqlc 基线**
   - 梳理各模块现有 SQL，挑选优先场景完成首个 sqlc PoC。
   - 建立 `sqlc.yaml`、在 `go.mod/tools.go` 和 Makefile/CI 中加入 `sqlc generate`，同步 CONTRIBUTING/README 的生成指引。
2. **Atlas 联动**
   - 在 sqlc 生成流程稳定后，编写 `schema.hcl` 并串联 Atlas diff → goose/golang-migrate。
   - 新增 `make db plan/apply`、CI 校验步骤，以及 schema 变更回滚指南。
3. **Asynq 队列**
   - 选定一个长耗时业务流程完成 Asynq PoC，固化 producer/worker 模式。
   - 复用 Redis 部署，添加 Asynq CLI/监控配置，扩展 README/运维文档说明依赖与回滚策略。
4. **Casbin 授权**
   - 审核现有权限模型，制定统一的 Casbin model/policy，并实现 `pkg/authz` 适配层。
   - 在相关模块中替换旧逻辑，配套测试、迁移脚本与文档更新。
5. **Transactional Outbox**
   - 设计 outbox schema + relay 服务，优先在一个跨模块流程中验证端到端一致性。
   - 将插入/轮询逻辑纳入服务层与监控体系，记录上线/回滚流程并更新 dev-plan。

## 里程碑
- M1：完成优先项评估与 PoC 结论归档。
- M2：首个工具（预计为 sqlc 或 Atlas）纳入 Makefile/CI 并在模块中落地。
- M3：后台队列、Casbin、Outbox 至少完成一个模块级实践，并形成复盘文档。

## 交付物
- 本 dev-plan 文档及后续更新记录。
- 各工具所需配置（如 `sqlc.yaml`、`schema.hcl`、`casbin/model.conf` 等）。
- 更新后的 README/AGENTS/CONTRIBUTING 章节。
- PoC 与验证输出（命令记录、日志、示意图），证明方案与 R200 指南一致。
