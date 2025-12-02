# DEV-PLAN-013：Casbin 基础设施与迁移基线

**状态**: 草拟中（2025-01-15 10:05）

## 背景
- DEV-PLAN-012 将 Casbin 推广拆分为基础设施、模块改造、UI 管理三大块；其中最关键的前置条件是统一的 `pkg/authz`、model/policy 配置以及旧权限映射，否则 Core/HRM/Logging 无法进入改造阶段。
- 现有授权仍依赖 `user.Can` + `permission.Permission` 常量，策略存放在多处数据库记录中，缺乏统一命名规范，也没有脚本能够导出/回放到 Git 管理的策略文件。
- 缺失 Ci/CD 级别的 Casbin 校验，也没有 Feature Flag 帮助我们在生产环境并行比对 `user.Can` 与 Casbin 判定，为逐步切换提供安全网。

## 目标
1. 在仓库根目录交付 `pkg/authz` 包、`config/access/model.conf`、`config/access/policy.csv`，并定义 subject/object/action/domain/ABAC 的统一命名规范。
2. 编写 `scripts/authz/export_legacy_policies.go`、`scripts/authz/verify_parity.go` 等 CLI，能够从现有数据库导出策略、生成 Casbin policy、比对 `user.Can` 与 `authz.Check` 的判定一致性。
3. 引入 `AUTHZ_ENFORCE` Feature Flag，支持“旁路模式（仅记录差异）→ 强制模式”切换，配套监控指标与日志格式。
4. 在 Makefile/quality-gates 中新增 `authz-test`、`authz-lint`、policy diff 检查，任何对 `config/access/**` 或 `scripts/authz/**` 的修改都必须通过 CI 约束。
5. 更新 README/CONTRIBUTING/AGENTS/dev-records，形成“导出旧策略 → 校验 → 提交 Git → 打开 Feature Flag”的标准流程，并记录回滚方法。

## 风险
- 旧权限模型与 Casbin 命名不一致，映射表若设计不当会导致策略错误或难以维护。
- 导出脚本需要访问生产级数据库，若缺乏审计或脱敏，可能泄露敏感信息。
- Feature Flag 并行模式会额外消耗 CPU/内存；若日志量过大，可能影响核心服务。
- CI 校验需要稳定的工具链（casbin fmt/lint、自研脚本），一旦版本漂移将导致误报。
- 若没有明确回滚路径，Casbin policy 的 Git 管理方式可能阻塞紧急修复。

## 实施步骤
1. **[ ] 命名规范与 `pkg/authz`**
   - 在 `pkg/authz` 中封装 Enforcer 创建、上下文注入（tenant/user/attributes）、`Authorize(ctx, subject, object, action, attrs...)` API，并提供 middleware/helper。
   - 固化命名规范：`subject=tenant:{tenantID}:user:{userID}`、`object=module.resource`、`action` 采用 CRUD + 自定义动词、`domain=tenantID`（`global` 代表跨租户）、ABAC 属性包含 `tenant_id`、`role_slugs`、`ownership`、`department_ids` 等，写入 README/AGENTS。
   - 在 `tools.go` 引入 `github.com/casbin/casbin/v2` 及所需 adapter（file adapter），Makefile 新增 `authz-test`（运行 `go test ./pkg/authz/...` + parity tests）与 `authz-lint`（检查 policy/model 格式、排序、重复项）。
2. **[ ] 策略存储与导出 CLI**
   - 创建 `config/access/model.conf`（RBAC with domains + ABAC 模型）与 `config/access/policies/` 目录：按模块/租户拆分 policy 文件（如 `config/access/policies/core/global.csv`、`hrm/tenant-uuid.csv`），并提供 `make authz-pack` 聚合器生成 `policy.csv`；统一排序规则（subject→object→action→domain）写入 CONTRIBUTING，避免合并冲突。
   - `scripts/authz/export_legacy_policies.go`：仅允许在公司 bastion/CI runner 上执行（通过 `ALLOWED_ENV=production_export` 校验），使用专用只读服务账号连接数据库、写入加密的 `policy_export_<timestamp>.gz`，默认对用户/租户标识进行 hash 脱敏；脚本输出操作审计（操作者、时间、命令、数据总量）并推送到 `docs/dev-records/DEV-PLAN-013-CASBIN-INFRA.md`。
   - `scripts/authz/verify_parity.go`：读取拆分后的 policy 文件，加载 Casbin Enforcer，按照“全量 superadmin + 每租户至少 20% 用户（最少 50 个，最多 500 个）+ 覆盖所有模块动作”策略采样；可配置允许的差异阈值（默认 0，除非在配置文件显式豁免），输出 subject/object/action、旧值、新值、建议策略，并支持 `--emit-metrics` 向 Prometheus 发送统计。
   - 文档中记录脚本参数、示例命令、输出格式、脱敏配置、审计存档路径，并要求在变更评审前附上 `export` 与 `parity` 的摘要。
3. **[ ] Feature Flag 与监控**
   - 在集中配置服务（或 `config/authz_flags.yaml`）中管理 `AUTHZ_ENFORCE`（`disabled`/`shadow`/`enforce`），所有应用实例通过配置下发或 etcd/consul watcher 保持秒级一致；flag 更新时写入审计日志并触发健康检查，确保实例未停留在旧状态。
   - shadow 模式下记录差异日志（结构化字段：subject/object/action/attrs/legacy_result/casbin_result），差异数据写入集中日志仓库并保留 14 天；同时在 Grafana/Prometheus 上报差异计数、判定耗时、flag 状态分布，设置告警阈值。
   - 定义开启/关闭 Flag 的 runbook：包含配置变更步骤、验证指标、回滚命令、通讯模板，并要求 SRE/产品双签审。
4. **[ ] CI 与质量门禁**
   - 在 `.github/workflows/quality-gates.yml` 新增 `authz` 过滤器（命中 `pkg/authz/**`、`config/access/**`、`scripts/authz/**`、`Makefile` 中相关 target 时触发），执行：`go test ./pkg/authz/...`、`make authz-lint`、`scripts/authz/verify_parity.go --sample smoke --fixtures config/access/fixtures/testdata.yaml`；CI 环境只使用脱敏的 fixtures，不直接访问生产数据库。
   - `make generate`/`make check lint` 中加入 policy 格式化（按 subject/object/action/domain 排序）与 `git status` 检查，依赖 `make authz-pack` 聚合后的 `policy.csv` 校验 diff，确保 PR 不遗留未排序或未打包的策略。
   - 为导出脚本提供环境变量模板（`export AUTHZ_EXPORT_DSN=postgres://...`），要求开发者在本地只能连接 sandbox 数据库；生产导出需通过专用 bastion 脚本触发并写入审计。
5. **[ ] 文档、记录与回滚**
   - README/CONTRIBUTING/AGENTS 补充“Casbin 基础设施指南”：命名规范、目录结构、脚本用法、Feature Flag 流程、故障排查。
   - 在 `docs/dev-records/DEV-PLAN-013-CASBIN-INFRA.md` 记录 PoC 过程（命令、预期、实际、结果），附上 parity 测试截图与差异统计。
   - 编写回滚说明：如何恢复到旧策略（重新导出上一版本 policy、切回 `user.Can`、关闭 `AUTHZ_ENFORCE`），以及如何在 Git 中 revert policy 变更。

## 里程碑
- M1：`pkg/authz`、命名规范、model/policy 结构落地，`make authz-lint`/`authz-test` 可运行且 CI 集成。
- M2：导出与 parity 脚本完成，针对至少 3 个租户/模块跑通流程，差异清单归档。
- M3：`AUTHZ_ENFORCE` 阶段性推广方案、监控、回滚 runbook 完成，quality-gates 中的 `authz` 过滤器连续 3 次绿灯。

## 交付物
- `pkg/authz` 包、`config/access/model.conf`、`config/access/policy.csv`。
- `scripts/authz/export_legacy_policies.go`、`scripts/authz/verify_parity.go` 及其文档。
- `AUTHZ_ENFORCE` Feature Flag 配置、监控仪表盘、差异日志格式定义。
- Makefile 与 `.github/workflows/quality-gates.yml` 中的 `authz-test`、`authz-lint`、policy diff 检查。
- README/CONTRIBUTING/AGENTS 更新、`docs/dev-records/DEV-PLAN-013-CASBIN-INFRA.md`（或同等 PoC 记录）。
