# DEV-PLAN-012：Casbin 改造 Readiness 记录

该记录用于 DEV-PLAN-014 启动任一模块改造前的“authz readiness”回溯。每次进入 Core/HRM/Logging 改动前，请按计划执行 `make authz-test authz-lint && go test ./pkg/authz/...`，并将命令、期望与结果补充到下表。

| 时间 (UTC) | 命令 | 预期 | 实际 | 结果 |
|------------|------|------|------|------|
| 2025-01-15 14:45 | `make authz-test` | readiness 检查：`pkg/authz` + `scripts/authz/internal/...` 测试需通过 | 所有用例通过（分支 `feature/dev-plan-014`） | ✅ |
| 2025-01-15 14:45 | `make authz-lint` | readiness 检查：打包策略并使用 fixtures 执行 parity 验证 | `authz-pack` 成功生成聚合文件，fixture parity 通过 | ✅ |
| 2025-12-05 00:14 | `make authz-test` | readiness 检查：`pkg/authz` + `scripts/authz/internal/...` 用例需通过 | 所有 authz 相关单测通过（Core UI 授权改造前置检查） | ✅ |
| 2025-12-05 00:14 | `make authz-lint` | readiness 检查：策略打包 + fixture parity 需通过 | `authz-pack` 生成最新策略，`scripts/authz/verify --fixtures` 零 diff | ✅ |
| 2025-12-05 06:52 | `GOCACHE=/tmp/go-cache make authz-test` | readiness 检查：`pkg/authz` + `scripts/authz/internal/...` 用例需通过 | 所有相关用例通过 | ✅ |
| 2025-12-05 06:52 | `GOCACHE=/tmp/go-cache make authz-lint` | readiness 检查：策略打包 + fixture parity 需通过 | `authz-pack` 生成成功，`scripts/authz/verify --fixtures` 通过 | ✅ |
| 2025-12-06 00:02 | `GOCACHE=/tmp/go-cache make authz-test` | readiness 检查：`pkg/authz` + `scripts/authz/internal/...` 用例需通过 | 所有相关用例通过（014C 前置检查） | ✅ |
| 2025-12-06 00:02 | `GOCACHE=/tmp/go-cache make authz-lint` | readiness 检查：策略打包 + fixture parity 需通过 | `authz-pack` 生成成功，`scripts/authz/verify --fixtures` 通过 | ✅ |
| 2025-12-06 00:02 | `GOCACHE=/tmp/go-cache go test ./pkg/authz/...` | readiness 补充：重复验证 `pkg/authz` 及子包单测 | 所有相关用例通过 | ✅ |
| 2025-12-07 09:30 | `GOCACHE=/tmp/go-cache go test ./modules/logging/...` | Logging 控制器/服务/仓储单测覆盖无 Session/无租户/无权限/成功路径 | 所有 logging 包用例通过（含 action log 开关降级场景） | ✅ |
| 2025-12-07 09:30 | `GOCACHE=/tmp/go-cache make authz-lint` | readiness 补充：policy 打包 + fixtures parity（含 logging.view 基线） | `authz-pack` + fixtures parity 通过 | ✅ |

> 提示：若命令失败，请先记录“❌ + 原因/堆栈”，完成修复后再追加新的“✅”行，确保整个 014 改造期间具备可追踪的 readiness 审计链路。
