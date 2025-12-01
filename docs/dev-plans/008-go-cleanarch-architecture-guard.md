# DEV-PLAN-008：集成 go-cleanarch 的架构依赖守护

**状态**: 已完成（2025-12-01 18:14）

## 背景
- R200 文档指出“不要依赖开发者自觉维护模块边界，应在 CI 中引入 go-cleanarch 自动阻断非法依赖”（docs/dev-records/R200r-Go语言ERP系统最佳实践.md:201-205）。
- 仓库已按模块化单体 + DDD 分层组织代码（AGENTS.md:6-96），并通过 README.MD:28-41 所述质量门禁运行 go fmt/vet、lint、测试等常规检查，但尚无自动化工具验证诸如“domain 不依赖 presentation、跨模块禁止导入 internal”等约束。
- DEV-PLAN-005/005T 已于 2025-12-01 12:41 正式收官，`quality-gates` workflow 与 `scripts/run-go-tests.sh` 统一的本地/CI 流程已经上线，为 go-cleanarch “只在合法依赖通过时合并”提供运行土壤。
- 当前完全依赖 code review 识别非法 import，随着 finance、warehouse、crm 等模块扩张，边界侵蚀风险持续累积，需要工具化守护。

## 目标
- 在仓库根目录新增 `.gocleanarch.yml`，描述 domain/infrastructure/services/presentation/pkg/cmd 等层的允许依赖，并声明跨模块 import 限制。
- 本地 `make check lint` 自动运行 go-cleanarch，让开发者在提交前即可发现违规。
- `quality-gates` workflow 在 Go 代码变更时执行同样的检查，确保 PR 若存在非法依赖会直接失败。
- README、AGENTS 及相关文档同步 go-cleanarch 要求，形成“规范 + 工具 + 门禁”闭环。

## 风险
- 规则过严可能阻塞合法场景（如共享 DTO、公共事件包），需提供 allowlist 并记录原因。
- go-cleanarch 会增加 `make check lint` / CI 用时，需要评估触发条件与缓存策略。
- 规则依赖准确包路径划分，初版配置可能产生误报，需与各模块负责人共同校正。

## 实施步骤
1. [x] **规范定义与配置文件**
   - 梳理层级：`cmd`（组装/依赖注入）、`modules/*/domain`（聚合/实体）、`modules/*/infrastructure`、`modules/*/services`、`modules/*/presentation`、`pkg/**`（共享库）。
   - 在根目录创建 `.gocleanarch.yml`：
     - 限制 domain 仅依赖 `pkg` 与同模块 domain；
     - services 可依赖 domain + pkg；
     - infrastructure 依赖 domain + pkg（实现仓储接口）；
     - presentation 仅依赖 services + pkg；
     - 禁止 `modules/{A}` 直接导入 `modules/{B}/internal`，如确需跨模块共享则通过 allowlist 明确列出。

2. [x] **本地命令集成**
   - 在 `tools.go` 中空白导入 `_ "github.com/roblaszczak/go-cleanarch"` 固定工具版本。
   - 更新 `Makefile` 的 `check lint`（或新增 `check arch`）执行 `go run github.com/roblaszczak/go-cleanarch -config .gocleanarch.yml ./...`，并在 README/AGENTS 说明该命令默认会跑 go-cleanarch。

3. [x] **CI 集成**
   - 修改 `.github/workflows/quality-gates.yml`，在 lint job 中加入 go-cleanarch 步骤，并沿用 changed-files 逻辑，仅在 Go 代码或 `.gocleanarch.yml` 变更时运行。
   - 若检测失败，直接让 job fail，并输出违规 import，确保 PR 无法跳过。

4. [x] **验证与知识沉淀**
   - 构造演示用非法依赖（如 presentation 直接 import 其他模块 domain）验证报错效果，然后还原改动。
   - 在 `docs/dev-records/` 或 PR 描述中记录验证命令与结果，便于追溯。
   - 在贡献指南、PR 模板或周会纪要中公告 go-cleanarch 已纳入必跑检查。

## 里程碑
- M1：`.gocleanarch.yml` 与规则初稿落地，`make check lint` 成功运行 go-cleanarch。
- M2：`quality-gates` workflow 集成新步骤，PR 可见违规依赖信息。
- M3：文档/团队同步完成，例外流程与 allowlist 策略清晰可查。

## 交付物
- `.gocleanarch.yml` 配置文件及必要 allowlist。
- 更新后的 `tools.go`、`Makefile`、`.github/workflows/quality-gates.yml`、README、AGENTS。
- 架构检查验证记录（命令输出或截图），证明 go-cleanarch 能识别非法跨层/跨模块 import。
