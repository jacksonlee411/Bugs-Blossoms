-- name: UpsertOrgChangeRequest :one
INSERT INTO org_change_requests (tenant_id, request_id, requester_id, status, payload_schema_version, payload, notes)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, request_id)
    DO UPDATE SET
        requester_id = EXCLUDED.requester_id, status = EXCLUDED.status, payload_schema_version = EXCLUDED.payload_schema_version, payload = EXCLUDED.payload, notes = EXCLUDED.notes, updated_at = now()
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

-- name: ListOrgChangeRequestsByRequester :many
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
    AND requester_id = $2
ORDER BY
    updated_at DESC;

