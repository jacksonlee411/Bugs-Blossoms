#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

echo "[059A] smoke: running integration tests (requires reachable Postgres)"
echo

go test ./modules/org/services -run '^TestOrg059A' -v -count=1

cat <<'EOF'

[059A] 追溯信息（用于 psql/审计/outbox 复核）

测试会为每个用例创建独立 DB，DB 名=测试函数名：
  - TestOrg059AShadow_Position_MissingReasonCodeFillsLegacyAndMetaFlags
  - TestOrg059AEnforce_MissingReasonCodeBlocksPositionAndAssignment_NoAuditNoOutbox
  - TestOrg059ADisabled_Assignment_MissingReasonCodeKeepsEmptyAndMetaFlags
  - TestOrg059A_MigrationsIncludeReasonCodeModeColumn

固定 tenant_id:
  - 00000000-0000-0000-0000-000000000059

固定 request_id:
  - req-059a-shadow-pos
  - req-059a-enforce-pos-missing
  - req-059a-enforce-pos-ok
  - req-059a-enforce-asg-missing
  - req-059a-disabled-pos
  - req-059a-disabled-asg

复核 SQL（在对应 DB 中执行）：
  -- audit
  SELECT change_type, entity_type, entity_id, meta
  FROM org_audit_logs
  WHERE tenant_id='00000000-0000-0000-0000-000000000059' AND request_id='<REQ_ID>';

  -- outbox
  SELECT id, event_type, payload
  FROM org_outbox
  WHERE tenant_id='00000000-0000-0000-0000-000000000059' AND payload->>'request_id'='<REQ_ID>';
EOF

