# DEV-PLAN-075 Readiness

**状态**：Phase A/B/C 已合并（PR #178/#179/#180）；Phase D（选项 A：退场与清理）进行中

## 关闭条件（进行中）

- [x] Phase B/C：CI 全绿并合并到 `main`
- [x] Read/Write：Job Catalog 四类对象（Group/Family/Level/Profile）读写均以 slices 为 SSOT
- [x] 性能：关键读路径无 N+1（批量 resolver / 单条联表 SQL）
- [ ] Phase D（选项 A）：完成 legacy `org_job_profile_job_families` 退场（代码与 schema 不再依赖；迁移 drop 落地）

## 证据

- 方案：`docs/dev-plans/075-job-catalog-effective-dated-attributes.md`
- PR：
  - Phase A：#178
  - Phase B/C：#179
  - 导入/测试迁移补齐：#180
- 本地验证（Phase D / 退场与清理，2026-01-04 11:05 本地）：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`
  - `atlas migrate hash --dir file://migrations/org --dir-format goose`
  - `make org lint`
  - `make org migrate up`
