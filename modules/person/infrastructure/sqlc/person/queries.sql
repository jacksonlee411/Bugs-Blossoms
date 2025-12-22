-- name: ListPersonsPaginated :many
SELECT tenant_id, person_uuid, pernr, display_name, status, created_at, updated_at
FROM persons
WHERE tenant_id = $1
  AND (
    $2::text = '' OR
    pernr ILIKE ('%' || $2 || '%') OR
    display_name ILIKE ('%' || $2 || '%')
  )
ORDER BY pernr ASC
OFFSET $3
LIMIT $4;

-- name: CountPersons :one
SELECT COUNT(1)
FROM persons
WHERE tenant_id = $1
  AND (
    $2::text = '' OR
    pernr ILIKE ('%' || $2 || '%') OR
    display_name ILIKE ('%' || $2 || '%')
  );

-- name: GetPersonByUUID :one
SELECT tenant_id, person_uuid, pernr, display_name, status, created_at, updated_at
FROM persons
WHERE tenant_id = $1 AND person_uuid = $2
LIMIT 1;

-- name: GetPersonByPernr :one
SELECT tenant_id, person_uuid, pernr, display_name, status, created_at, updated_at
FROM persons
WHERE tenant_id = $1 AND pernr = $2
LIMIT 1;

