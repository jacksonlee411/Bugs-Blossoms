# DEV-PLAN-013：Casbin 基础设施实施记录

| 时间 (UTC) | 命令 | 预期 | 实际 | 结果 |
|------------|------|------|------|------|
| 2025-12-02 18:40 | `go get github.com/casbin/casbin/v2@v2.88.0` | 拉取 Casbin 依赖，供 `pkg/authz` 使用 | 依赖成功加入 `go.mod`/`go.sum` | ✅ |
| 2025-12-02 18:50 | `go test ./pkg/authz` | 新建 authz 包测试通过 | 所有用例通过 | ✅ |
| 2025-12-02 18:55 | `go run ./scripts/authz/pack_policies.go` | 将 `config/access/policies/**` 聚合为 `config/access/policy.csv` | 生成固定头的聚合文件 | ✅ |
| 2025-12-02 18:57 | `go run ./scripts/authz/verify_parity.go --fixtures config/access/fixtures/testdata.yaml` | 基于基线 fixture 验证 Casbin 判定与 legacy 结果一致 | Fixture 校验通过 | ✅ |

> 说明：导出脚本 `scripts/authz/export_legacy_policies.go` 需在受管环境下运行（`ALLOWED_ENV=production_export`），本次未在本地执行，仅完成 CLI 骨架与安全校验。
