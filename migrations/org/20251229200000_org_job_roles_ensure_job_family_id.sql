-- +goose Up
-- Backfill/repair for legacy dev databases where org_job_roles exists but
-- missed job_family_id due to CREATE TABLE IF NOT EXISTS semantics.

-- +goose StatementBegin
DO $$
DECLARE
    missing_count int;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'org_job_roles'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public'
              AND table_name = 'org_job_roles'
              AND column_name = 'job_family_id'
        ) THEN
            ALTER TABLE org_job_roles
                ADD COLUMN job_family_id uuid;
        END IF;

        IF EXISTS (
            SELECT 1
            FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = 'org_job_role_family_allocations'
        ) THEN
            WITH ranked AS (
                SELECT
                    tenant_id,
                    job_role_id,
                    job_family_id,
                    ROW_NUMBER() OVER (
                        PARTITION BY tenant_id, job_role_id
                        ORDER BY allocation_percent DESC, job_family_id
                    ) AS rn
                FROM org_job_role_family_allocations
            )
            UPDATE org_job_roles r
            SET job_family_id = ranked.job_family_id
            FROM ranked
            WHERE ranked.tenant_id = r.tenant_id
              AND ranked.job_role_id = r.id
              AND ranked.rn = 1
              AND r.job_family_id IS NULL;
        END IF;

        SELECT COUNT(*)
        INTO missing_count
        FROM org_job_roles
        WHERE job_family_id IS NULL;

        IF missing_count > 0 THEN
            RAISE EXCEPTION 'org_job_roles.job_family_id is required before DEV-PLAN-072 migration (missing=%). Either backfill via org_job_role_family_allocations or reset dev DB and re-run migrations.', missing_count;
        END IF;

        ALTER TABLE org_job_roles
            ALTER COLUMN job_family_id SET NOT NULL;

        IF EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'org_job_roles_tenant_id_code_key'
        ) THEN
            ALTER TABLE org_job_roles
                DROP CONSTRAINT org_job_roles_tenant_id_code_key;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'org_job_roles_family_fk'
        ) THEN
            ALTER TABLE org_job_roles
                ADD CONSTRAINT org_job_roles_family_fk
                FOREIGN KEY (tenant_id, job_family_id)
                REFERENCES org_job_families (tenant_id, id)
                ON DELETE RESTRICT;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'org_job_roles_tenant_id_family_code_key'
        ) THEN
            ALTER TABLE org_job_roles
                ADD CONSTRAINT org_job_roles_tenant_id_family_code_key
                UNIQUE (tenant_id, job_family_id, code);
        END IF;

        CREATE INDEX IF NOT EXISTS org_job_roles_tenant_family_active_code_idx
            ON org_job_roles (tenant_id, job_family_id, is_active, code);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Best-effort: revert to pre-repair shape (may fail if other migrations depend on it).

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'org_job_roles'
    ) THEN
        DROP INDEX IF EXISTS org_job_roles_tenant_family_active_code_idx;

        ALTER TABLE org_job_roles
            DROP CONSTRAINT IF EXISTS org_job_roles_tenant_id_family_code_key;

        ALTER TABLE org_job_roles
            DROP CONSTRAINT IF EXISTS org_job_roles_family_fk;

        ALTER TABLE org_job_roles
            DROP COLUMN IF EXISTS job_family_id;
    END IF;
END $$;
-- +goose StatementEnd

