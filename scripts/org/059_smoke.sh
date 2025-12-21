#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

echo "[059] smoke: running a minimal end-to-end set of org integration tests (requires reachable Postgres)"
echo

go test ./modules/org/services -run '^TestOrg059PositionRescind_WritesAuditAndOutbox$' -v -count=1
go test ./modules/org/services -run '^TestOrg058_AssignmentCorrectAndRescind_WritesAuditAndOutbox$' -v -count=1
go test ./modules/org/services -run '^TestOrg057StaffingVacancies_ComputesVacancySince$' -v -count=1
go test ./modules/org/services -run '^TestOrg053ShiftBoundaryPosition_MovesAdjacentBoundary$' -v -count=1

cat <<'EOF'

[059] 追溯信息（用于 psql/审计/outbox 复核）

每个用例创建独立 DB，DB 名基于测试函数名 sanitize（可能带 hash 后缀）。

固定 tenant_id:
  - 059 smoke: 00000000-0000-0000-0000-000000000059

固定 request_id（059 smoke 新增）:
  - req-059-pos-create
  - req-059-pos-rescind

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

