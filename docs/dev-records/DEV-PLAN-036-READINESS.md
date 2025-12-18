# DEV-PLAN-036 Readiness

本记录用于复现 `docs/dev-plans/036-org-sample-tree-data.md` 的示例组织树数据集导入、对账与回滚路径（对齐 020/023 的验证闭环）。

## 1. 数据集

- `dataset_id`: `org-036-manufacturing`
- CSV 目录：`docs/samples/org/036-manufacturing/`
  - `nodes.csv`（部门树）
  - `positions.csv`（可选，用于演示）
  - `assignments.csv`（可选，用于演示）
- 口径：root=第 1 级；DB `max_depth=16` 即“最深 17 级”（见 `docs/dev-plans/036-org-sample-tree-data.md`）。

## 2. 本地验证命令（SSOT）

- 文档门禁：
  - `make check doc`
- 如需同时验证导入工具（可选，复用 023 的 readiness）：
  - `go fmt ./...`
  - `go vet ./...`
  - `make check lint`
  - `make test`

## 3. 复现步骤（导入前校验 → 导入 → 对账 → 回滚）

> 说明：`cmd/org-data import` 默认 dry-run；显式 `--apply` 才写库。seed 模式要求目标租户为空租户（见 023 安全网）。

1. 选择租户：
   - `TENANT_ID=<tenant_uuid>`
2. Dry-run（导入前校验，不落库）：
   - `go run ./cmd/org-data import --tenant $TENANT_ID --input docs/samples/org/036-manufacturing`
3. Apply（落库 + manifest）：
   - `OUT_DIR=/tmp/org-036-$TENANT_ID`
   - `go run ./cmd/org-data import --tenant $TENANT_ID --input docs/samples/org/036-manufacturing --apply --output $OUT_DIR`
4. 导入后对账（至少记录以下字段）：
   - `tenant_id=$TENANT_ID`
   - `as_of_date=2025-01-01`
   - `nodes_total>=200`
   - `max_depth=16`
   - `root_name=飞虫与鲜花`
   - `root_children` 包含：`房地产`、`物业管理`、`互联网行业`
   - `manifest_path=$OUT_DIR/import_manifest_*.json`
5. Rollback（manifest 精确回滚）：
   - `go run ./cmd/org-data rollback --tenant $TENANT_ID --manifest $OUT_DIR/import_manifest_*.json --apply --yes`

## 4. 实际跑通记录（待填写）

- 时间：2025-12-18 12:44 UTC
- DB：`postgres://postgres@localhost:5438/iota_erp`（容器：`bugs-blossoms-db-1`，PG17）
- tenant：`45458d75-2018-470c-9655-2bc8ec498f20`（name=`org-036-manufacturing-20251218124219`）
- 预置：执行 `DB_HOST=localhost DB_PORT=5438 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=iota_erp make org migrate up` 以补齐 Org schema（本地环境准备，不属于 036 的代码改动）。
- import dry-run 输出（stdout JSON 一行）：
  - `{"status":"dry_run","run_id":"eae6c4c0-4ad0-4e26-a875-d3bd38a5b360","tenant_id":"45458d75-2018-470c-9655-2bc8ec498f20","backend":"db","mode":"seed","apply":false,"input_dir":"docs/samples/org/036-manufacturing","output_dir":"docs/samples/org/036-manufacturing","counts":{"nodes_rows":254,"positions_rows":5,"assignments_rows":5}}`
- import apply 输出（stdout JSON 一行）：
  - `{"status":"applied","run_id":"170b730d-6466-4eb0-b5fa-0385317c99ea","tenant_id":"45458d75-2018-470c-9655-2bc8ec498f20","backend":"db","mode":"seed","apply":true,"input_dir":"docs/samples/org/036-manufacturing","output_dir":"/tmp/org-036-45458d75-2018-470c-9655-2bc8ec498f20","manifest_version":1,"counts":{"nodes_rows":254,"positions_rows":5,"assignments_rows":5}}`
- 导入后对账摘要（JSON，一行）：
  - `{"dataset_id":"org-036-manufacturing","tenant_id":"45458d75-2018-470c-9655-2bc8ec498f20","as_of_date":"2025-01-01T00:00:00Z","nodes_total":254,"max_depth":16,"root_name":"飞虫与鲜花","root_children":["房地产","物业管理","互联网行业"],"manifest_path":"/tmp/org-036-45458d75-2018-470c-9655-2bc8ec498f20/import_manifest_20251218T124239Z_170b730d-6466-4eb0-b5fa-0385317c99ea.json"}`
- rollback 输出（stdout JSON 一行）：
  - `{"status":"applied","mode":"manifest","run_id":"170b730d-6466-4eb0-b5fa-0385317c99ea","tenant_id":"45458d75-2018-470c-9655-2bc8ec498f20","counts":{"org_nodes":254,"org_node_slices":254,"org_edges":254,"org_positions":5,"org_assignments":5}}`
- 回滚后校验（psql 计数均为 0）：
  - `org_nodes=0`
  - `org_node_slices=0`
  - `org_edges=0`
  - `org_positions=0`
  - `org_assignments=0`

## 5. 实际导入到默认租户（保留数据）

- 时间：2025-12-18 12:49 UTC
- tenant：`00000000-0000-0000-0000-000000000001`（Default Tenant）
- import dry-run 输出（stdout JSON 一行）：
  - `{"status":"dry_run","run_id":"e9d67c3e-cf3f-4f67-b4c0-bfe262e01489","tenant_id":"00000000-0000-0000-0000-000000000001","backend":"db","mode":"seed","apply":false,"input_dir":"docs/samples/org/036-manufacturing","output_dir":"docs/samples/org/036-manufacturing","counts":{"nodes_rows":254,"positions_rows":5,"assignments_rows":5}}`
- import apply 输出（stdout JSON 一行）：
  - `{"status":"applied","run_id":"6129123c-2ef3-4076-9d3f-1044d4a0dc52","tenant_id":"00000000-0000-0000-0000-000000000001","backend":"db","mode":"seed","apply":true,"input_dir":"docs/samples/org/036-manufacturing","output_dir":"/tmp/org-036-default-tenant","manifest_version":1,"counts":{"nodes_rows":254,"positions_rows":5,"assignments_rows":5}}`
- 导入后对账摘要（JSON，一行）：
  - `{"dataset_id":"org-036-manufacturing","tenant_id":"00000000-0000-0000-0000-000000000001","as_of_date":"2025-01-01T00:00:00Z","nodes_total":254,"max_depth":16,"root_name":"飞虫与鲜花","root_children":["房地产","物业管理","互联网行业"],"manifest_path":"/tmp/org-036-default-tenant/import_manifest_20251218T124901Z_6129123c-2ef3-4076-9d3f-1044d4a0dc52.json"}`
- 说明：本次导入用于“保留示例数据”，未执行回滚；如需清理，请按 023 使用 manifest 回滚。
