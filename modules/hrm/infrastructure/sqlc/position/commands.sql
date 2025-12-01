-- name: CreatePosition :one
INSERT INTO positions (tenant_id, name, description)
VALUES (sqlc.arg (tenant_id), sqlc.arg (name), sqlc.arg (description))
RETURNING
    id;

-- name: UpdatePosition :exec
UPDATE
    positions
SET
    name = sqlc.arg (name),
    description = sqlc.arg (description)
WHERE
    id = sqlc.arg (id)
    AND tenant_id = sqlc.arg (tenant_id);

-- name: DeletePosition :exec
DELETE FROM positions
WHERE id = sqlc.arg (id)
    AND tenant_id = sqlc.arg (tenant_id);
