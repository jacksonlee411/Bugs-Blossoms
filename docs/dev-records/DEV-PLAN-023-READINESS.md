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

## 4. 实际跑通记录（本地 DB）

> 说明：`pkg/configuration` 使用 `godotenv.Load`，不会覆盖已存在的环境变量；为避免进程环境里已有 `DB_*` 导致连接到错误 DB，本次冒烟显式设置了 `DB_HOST/DB_PORT/DB_NAME/...`。

- 时间：2025-12-18
- DB：`postgres://postgres@localhost:5439/iota_erp_015b4`
- tenant：`15533733-2ced-4083-9dc2-bbe216d42b11`
- import dry-run 输出（stdout JSON line）：
  - `{"status":"dry_run","run_id":"94f195d3-cc52-432f-8279-82140e5f8df1","tenant_id":"15533733-2ced-4083-9dc2-bbe216d42b11","backend":"db","mode":"seed","apply":false,"input_dir":"docs/samples/org","output_dir":"docs/samples/org","counts":{"nodes_rows":2,"positions_rows":1,"assignments_rows":1}}`
- import apply 输出（stdout JSON line，manifest_version=1）：
  - `{"status":"applied","run_id":"70e0da90-ae65-445c-aa5c-2416912e0365","tenant_id":"15533733-2ced-4083-9dc2-bbe216d42b11","backend":"db","mode":"seed","apply":true,"input_dir":"docs/samples/org","output_dir":"/tmp/org-data-out-15533733-2ced-4083-9dc2-bbe216d42b11","manifest_version":1,"counts":{"nodes_rows":2,"positions_rows":1,"assignments_rows":1}}`
- export 输出（stdout JSON line）：
  - `{"status":"exported","tenant_id":"15533733-2ced-4083-9dc2-bbe216d42b11"}`
- rollback（manifest）输出（stdout JSON line）：
  - `{"status":"applied","mode":"manifest","run_id":"70e0da90-ae65-445c-aa5c-2416912e0365","tenant_id":"15533733-2ced-4083-9dc2-bbe216d42b11","counts":{"org_nodes":2,"org_node_slices":2,"org_edges":2,"org_positions":1,"org_assignments":1}}`
- 回滚后校验（psql 计数均为 0）：
  - `org_nodes=0`
  - `org_node_slices=0`
  - `org_edges=0`
  - `org_positions=0`
  - `org_assignments=0`
