-- +goose Up
-- DEV-PLAN-075A: retire legacy columns from Job Catalog identity tables (SSOT is *_slices).

DROP INDEX IF EXISTS org_job_family_groups_tenant_active_code_idx;
DROP INDEX IF EXISTS org_job_families_tenant_group_active_code_idx;
DROP INDEX IF EXISTS org_job_levels_tenant_active_order_code_idx;
DROP INDEX IF EXISTS org_job_profiles_tenant_active_code_idx;

ALTER TABLE org_job_levels
    DROP CONSTRAINT IF EXISTS org_job_levels_display_order_check;

ALTER TABLE org_job_profiles
    DROP CONSTRAINT IF EXISTS org_job_profiles_external_refs_is_object_check;

-- atlas:nolint DS103
ALTER TABLE org_job_family_groups
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS is_active;

-- atlas:nolint DS103
ALTER TABLE org_job_families
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS is_active;

-- atlas:nolint DS103
ALTER TABLE org_job_levels
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS display_order,
    DROP COLUMN IF EXISTS is_active;

-- atlas:nolint DS103
ALTER TABLE org_job_profiles
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS external_refs;

-- +goose Down
-- Best-effort: restore legacy columns (does not backfill historical values from slices).

ALTER TABLE org_job_family_groups
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS org_job_family_groups_tenant_active_code_idx
    ON org_job_family_groups (tenant_id, is_active, code);

ALTER TABLE org_job_families
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS org_job_families_tenant_group_active_code_idx
    ON org_job_families (tenant_id, job_family_group_id, is_active, code);

ALTER TABLE org_job_levels
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS display_order int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT TRUE;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_job_levels_display_order_check'
    ) THEN
        ALTER TABLE org_job_levels
            ADD CONSTRAINT org_job_levels_display_order_check CHECK (display_order >= 0);
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS org_job_levels_tenant_active_order_code_idx
    ON org_job_levels (tenant_id, is_active, display_order, code);

ALTER TABLE org_job_profiles
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description text NULL,
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS external_refs jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_job_profiles_external_refs_is_object_check'
    ) THEN
        ALTER TABLE org_job_profiles
            ADD CONSTRAINT org_job_profiles_external_refs_is_object_check CHECK (jsonb_typeof(external_refs) = 'object');
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS org_job_profiles_tenant_active_code_idx
    ON org_job_profiles (tenant_id, is_active, code);
