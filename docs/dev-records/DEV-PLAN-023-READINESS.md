# DEV-PLAN-023 Readiness

本记录用于复现 `docs/dev-plans/023-org-import-rollback-and-readiness.md` 的最小可用交付（`cmd/org-data`）与门禁验证路径。

## 1. 本次交付范围（MVP）

- Go CLI：`cmd/org-data`（`import/export/rollback`），默认 dry-run。
- 仅支持：`import --backend db --mode seed`（空租户种子导入）。
- `subject_id` 计算：复用 `modules/org/domain/subjectid.NormalizedSubjectID`（SSOT 见 026 §7.3）。

## 2. 本地验证命令

- Go 门禁：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`
- 文档门禁：
  - `make check doc`

## 3. 手工冒烟（示例）

1. 准备 CSV：
   - `docs/samples/org/nodes.csv`
   - `docs/samples/org/positions.csv`
   - `docs/samples/org/assignments.csv`
2. Dry-run（不落库）：
   - `go run ./cmd/org-data import --tenant <tenant_uuid> --input docs/samples/org`
3. Apply（落库 + manifest）：
   - `go run ./cmd/org-data import --tenant <tenant_uuid> --input docs/samples/org --apply --output /tmp/org-data-out`
4. Export：
   - `go run ./cmd/org-data export --tenant <tenant_uuid> --output /tmp/org-data-export --as-of 2025-01-01`
5. Rollback（按 manifest 精确回滚）：
   - `go run ./cmd/org-data rollback --tenant <tenant_uuid> --manifest /tmp/org-data-out/import_manifest_*.json --apply --yes`

