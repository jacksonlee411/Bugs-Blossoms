-- +goose Up
-- Legacy data repair: older position creation wrote UNSPECIFIED placeholders into
-- org_position_slices job_*_code columns. DEV-PLAN-072 migration requires these
-- codes to resolve to real job catalog rows for job_profile_id backfill.

-- +goose StatementBegin
DO $$
DECLARE
    t_id uuid;
    g_id uuid;
    f_id uuid;
BEGIN
    FOR t_id IN
        SELECT DISTINCT tenant_id
        FROM org_position_slices
        WHERE job_profile_id IS NULL
          AND job_family_group_code = 'UNSPECIFIED'
          AND job_family_code = 'UNSPECIFIED'
          AND job_role_code = 'UNSPECIFIED'
    LOOP
        INSERT INTO org_job_family_groups (tenant_id, code, name)
        VALUES (t_id, 'UNSPECIFIED', 'UNSPECIFIED')
        ON CONFLICT DO NOTHING;

        SELECT id
        INTO g_id
        FROM org_job_family_groups
        WHERE tenant_id = t_id AND code = 'UNSPECIFIED';

        INSERT INTO org_job_families (tenant_id, job_family_group_id, code, name)
        VALUES (t_id, g_id, 'UNSPECIFIED', 'UNSPECIFIED')
        ON CONFLICT DO NOTHING;

        SELECT id
        INTO f_id
        FROM org_job_families
        WHERE tenant_id = t_id
          AND job_family_group_id = g_id
          AND code = 'UNSPECIFIED';

        INSERT INTO org_job_roles (tenant_id, job_family_id, code, name)
        VALUES (t_id, f_id, 'UNSPECIFIED', 'UNSPECIFIED')
        ON CONFLICT DO NOTHING;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Best-effort no-op (rows may be referenced by migrated data).

