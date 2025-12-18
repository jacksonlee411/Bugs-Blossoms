-- name: UpsertOrgChangeRequest :one
INSERT INTO org_change_requests (tenant_id, request_id, requester_id, status, payload_schema_version, payload, notes)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, request_id)
    DO UPDATE SET
        requester_id = EXCLUDED.requester_id, status = EXCLUDED.status, payload_schema_version = EXCLUDED.payload_schema_version, payload = EXCLUDED.payload, notes = EXCLUDED.notes, updated_at = now()
    RETURNING
        tenant_id, id, request_id, requester_id, status, payload_schema_version, payload, notes, created_at, updated_at;

-- name: UpdateOrgChangeRequestDraftByID :one
UPDATE org_change_requests
SET
    payload = $3,
    notes = $4,
    updated_at = now()
WHERE
    tenant_id = $1
    AND id = $2
    AND status = 'draft'
RETURNING
    tenant_id, id, request_id, requester_id, status, payload_schema_version, payload, notes, created_at, updated_at;

-- name: UpdateOrgChangeRequestStatusByID :one
UPDATE org_change_requests
SET
    status = $3,
    updated_at = now()
WHERE
    tenant_id = $1
    AND id = $2
RETURNING
    tenant_id, id, request_id, requester_id, status, payload_schema_version, payload, notes, created_at, updated_at;

-- name: GetOrgChangeRequestByRequestID :one
SELECT
    tenant_id,
    id,
    request_id,
    requester_id,
    status,
    payload_schema_version,
    payload,
    notes,
    created_at,
    updated_at
FROM
    org_change_requests
WHERE
    tenant_id = $1
    AND request_id = $2;

-- name: GetOrgChangeRequestByID :one
SELECT
    tenant_id,
    id,
    request_id,
    requester_id,
    status,
    payload_schema_version,
    payload,
    notes,
    created_at,
    updated_at
FROM
    org_change_requests
WHERE
    tenant_id = $1
    AND id = $2;

-- name: ListOrgChangeRequests :many
SELECT
    tenant_id,
    id,
    request_id,
    requester_id,
    status,
    payload_schema_version,
    payload,
    notes,
    created_at,
    updated_at
FROM
    org_change_requests
WHERE
    tenant_id = $1
    AND ($2::text = '' OR status = $2)
    AND (
        $3::timestamptz IS NULL
        OR $4::uuid IS NULL
        OR (updated_at, id) < ($3::timestamptz, $4::uuid)
    )
ORDER BY
    updated_at DESC, id DESC
LIMIT $5;
