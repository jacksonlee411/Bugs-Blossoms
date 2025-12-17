# DEV-PLAN-021：Org Schema & Constraints Readiness 记录

该记录用于 DEV-PLAN-021（`docs/dev-plans/021-org-schema-and-constraints.md`）实施过程中的可追溯验证：每次关键变更（schema/约束/触发器/迁移口径）都应在此记录执行命令与结果。

配套计划：`docs/dev-plans/021-org-schema-and-constraints.md`

## 环境信息（填写）
- 日期（UTC）：2025-12-17
- 分支 / PR：`feature/dev-plan-021-impl`
- Git Revision：`b42ee8f2`
- 数据库：
  - Postgres 版本：17.7（Docker 临时容器验证）
  - DSN/连接信息（脱敏后）：`postgres://postgres:***@localhost:55432/<db>?sslmode=disable`

## 门禁与命令记录

| 时间 (UTC) | 环境 | 命令 | 预期 | 实际 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 2025-12-17 14:55 UTC | 本地 | `make atlas-install goose-install` | 安装 Atlas/Goose CLI | 已安装 | 通过 |
| 2025-12-17 14:55 UTC | 本地 | `atlas migrate hash --dir file://migrations/org` | 生成/更新 `migrations/org/atlas.sum` 且工作区干净 | `migrations/org/atlas.sum` 已更新 | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `make org migrate up DB_PORT=55432 DB_NAME=iota_erp_org_atlas` | baseline + smoke 迁移可执行 | 迁移到 version 2 | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `GOOSE_STEPS=1 make org migrate down DB_PORT=55432 DB_NAME=iota_erp_org_atlas` | 可回滚最近一次迁移且不删除扩展 | 回滚 smoke（baseline 保留） | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `SELECT gen_random_uuid();` | `pgcrypto` 可用 | 可执行 | 通过 |
| 2025-12-17 14:55 UTC | Docker 临时 PG17 | `SELECT 'a.b'::ltree;` | `ltree` 可用 | 可执行 | 通过 |

## 核心验收用例（勾选 + 记录证据）

### 1) 约束（EXCLUDE/UNIQUE/CHECK）
- [x] `org_node_slices`：同 `org_node_id` 重叠 slice 被拒绝（已验证：`org_node_slices_no_overlap`）
- [ ] `org_edges`：同 `child_node_id` 重叠 edge slice 被拒绝（附 SQL）
- [ ] `org_node_slices`：同父同窗 `lower(name)` 重名被拒绝（附 SQL）
- [ ] `org_nodes`：同租户第二个 `is_root=true` 被拒绝（附 SQL）

### 2) ltree 触发器（path/depth + 环路拒绝 + 更新限制）
- [x] root edge：`path/depth` 正确（已验证：root edge depth=0 且 path=32hex）
- [ ] child edge：`path/depth` 正确（附 SQL）
- [ ] 环路拒绝：parent 在 child 子树内被拒绝（附 SQL）
- [x] 更新限制：直接 `UPDATE org_edges SET parent_node_id=...` 被拒绝（触发器报错符合预期）
