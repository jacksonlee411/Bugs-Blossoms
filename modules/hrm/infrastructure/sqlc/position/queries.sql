SELECT
    id,
    tenant_id,
    name,
    description,
    created_at,
    updated_at
FROM
    positions
WHERE
    tenant_id = sqlc.arg (tenant_id)
ORDER BY
    id
LIMIT sqlc.arg (row_limit) OFFSET sqlc.arg (row_offset);

-- name: ListPositionsByTenant :many
SELECT
    id,
    tenant_id,
    name,
    description,
    created_at,
    updated_at
FROM
    positions
WHERE
    tenant_id = sqlc.arg (tenant_id)
ORDER BY
    id;

-- name: GetPositionByID :one
SELECT
    id,
    tenant_id,
    name,
    description,
    created_at,
    updated_at
FROM
    positions
WHERE
    id = sqlc.arg (id)
    AND tenant_id = sqlc.arg (tenant_id);

-- name: CountPositions :one
SELECT
    COUNT(*)
FROM
    positions
WHERE
    tenant_id = sqlc.arg (tenant_id);

