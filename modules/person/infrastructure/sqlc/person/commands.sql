-- name: CreatePerson :one
INSERT INTO persons (tenant_id, pernr, display_name, status)
VALUES ($1, $2, $3, $4)
RETURNING tenant_id, person_uuid, pernr, display_name, status, created_at, updated_at;

