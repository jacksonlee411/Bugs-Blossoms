-- name: ListEmployeesPaginated :many
SELECT
    e.id,
    e.tenant_id,
    e.first_name,
    e.last_name,
    e.middle_name,
    e.email,
    e.phone,
    e.salary,
    e.salary_currency_id,
    e.hourly_rate,
    e.coefficient,
    e.avatar_id,
    e.created_at,
    e.updated_at,
    em.primary_language,
    em.secondary_language,
    em.tin,
    em.pin,
    em.notes,
    em.birth_date,
    em.hire_date,
    em.resignation_date
FROM employees e
LEFT JOIN employee_meta em ON e.id = em.employee_id
WHERE e.tenant_id = sqlc.arg(tenant_id)
ORDER BY e.id
LIMIT sqlc.arg(row_limit)
OFFSET sqlc.arg(row_offset);

-- name: ListEmployeesByTenant :many
SELECT
    e.id,
    e.tenant_id,
    e.first_name,
    e.last_name,
    e.middle_name,
    e.email,
    e.phone,
    e.salary,
    e.salary_currency_id,
    e.hourly_rate,
    e.coefficient,
    e.avatar_id,
    e.created_at,
    e.updated_at,
    em.primary_language,
    em.secondary_language,
    em.tin,
    em.pin,
    em.notes,
    em.birth_date,
    em.hire_date,
    em.resignation_date
FROM employees e
LEFT JOIN employee_meta em ON e.id = em.employee_id
WHERE e.tenant_id = sqlc.arg(tenant_id)
ORDER BY e.id;

-- name: GetEmployeeByID :one
SELECT
    e.id,
    e.tenant_id,
    e.first_name,
    e.last_name,
    e.middle_name,
    e.email,
    e.phone,
    e.salary,
    e.salary_currency_id,
    e.hourly_rate,
    e.coefficient,
    e.avatar_id,
    e.created_at,
    e.updated_at,
    em.primary_language,
    em.secondary_language,
    em.tin,
    em.pin,
    em.notes,
    em.birth_date,
    em.hire_date,
    em.resignation_date
FROM employees e
LEFT JOIN employee_meta em ON e.id = em.employee_id
WHERE e.id = sqlc.arg(id)
  AND e.tenant_id = sqlc.arg(tenant_id);

-- name: CountEmployees :one
SELECT COUNT(*)
FROM employees
WHERE tenant_id = sqlc.arg(tenant_id);
