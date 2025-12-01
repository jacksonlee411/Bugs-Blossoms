-- name: CreateEmployee :one
INSERT INTO employees (tenant_id, first_name, last_name, middle_name, email, phone, salary, salary_currency_id, hourly_rate, coefficient, avatar_id, created_at, updated_at)
    VALUES (sqlc.arg (tenant_id), sqlc.arg (first_name), sqlc.arg (last_name), sqlc.arg (middle_name), sqlc.arg (email), sqlc.arg (phone), sqlc.arg (salary), sqlc.arg (salary_currency_id), sqlc.arg (hourly_rate), sqlc.arg (coefficient), sqlc.arg (avatar_id), sqlc.arg (created_at), sqlc.arg (updated_at))
RETURNING
    id;

-- name: CreateEmployeeMeta :exec
INSERT INTO employee_meta (employee_id, primary_language, secondary_language, tin, pin, notes, birth_date, hire_date, resignation_date)
    VALUES (sqlc.arg (employee_id), sqlc.arg (primary_language), sqlc.arg (secondary_language), sqlc.arg (tin), sqlc.arg (pin), sqlc.arg (notes), sqlc.arg (birth_date), sqlc.arg (hire_date), sqlc.arg (resignation_date));

-- name: UpdateEmployee :exec
UPDATE
    employees
SET
    first_name = sqlc.arg (first_name),
    last_name = sqlc.arg (last_name),
    middle_name = sqlc.arg (middle_name),
    email = sqlc.arg (email),
    phone = sqlc.arg (phone),
    salary = sqlc.arg (salary),
    salary_currency_id = sqlc.arg (salary_currency_id),
    hourly_rate = sqlc.arg (hourly_rate),
    coefficient = sqlc.arg (coefficient),
    avatar_id = sqlc.arg (avatar_id),
    updated_at = sqlc.arg (updated_at)
WHERE
    id = sqlc.arg (id)
    AND tenant_id = sqlc.arg (tenant_id);

-- name: UpdateEmployeeMeta :exec
UPDATE
    employee_meta
SET
    primary_language = sqlc.arg (primary_language),
    secondary_language = sqlc.arg (secondary_language),
    tin = sqlc.arg (tin),
    pin = sqlc.arg (pin),
    notes = sqlc.arg (notes),
    birth_date = sqlc.arg (birth_date),
    hire_date = sqlc.arg (hire_date),
    resignation_date = sqlc.arg (resignation_date)
WHERE
    employee_id = sqlc.arg (employee_id);

-- name: DeleteEmployeeMeta :exec
DELETE FROM employee_meta
WHERE employee_id = sqlc.arg (employee_id);

-- name: DeleteEmployee :exec
DELETE FROM employees
WHERE id = sqlc.arg (id)
    AND tenant_id = sqlc.arg (tenant_id);

