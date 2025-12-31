-- +goose Up
-- DEV-PLAN-072: Workday-style Job Architecture.
-- - Remove Job Role (org_job_roles) from the core model
-- - Job Level becomes tenant-global
-- - Job Profile carries default Job Family allocations (sum=100, single primary)
-- - Position slice copies defaults, but can be overridden

-- 1) Job Profile -> Job Families (default allocations)
CREATE TABLE IF NOT EXISTS org_job_profile_job_families (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    job_profile_id uuid NOT NULL,
    job_family_id uuid NOT NULL,
    allocation_percent int NOT NULL,
    is_primary boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_profile_job_families_pkey PRIMARY KEY (tenant_id, job_profile_id, job_family_id),
    CONSTRAINT org_job_profile_job_families_profile_fk FOREIGN KEY (tenant_id, job_profile_id)
        REFERENCES org_job_profiles (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT org_job_profile_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id)
        REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_job_profile_job_families_allocation_check CHECK (allocation_percent >= 1 AND allocation_percent <= 100)
);

CREATE UNIQUE INDEX IF NOT EXISTS org_job_profile_job_families_primary_unique
    ON org_job_profile_job_families (tenant_id, job_profile_id)
    WHERE is_primary = TRUE;

CREATE INDEX IF NOT EXISTS org_job_profile_job_families_tenant_family_profile_idx
    ON org_job_profile_job_families (tenant_id, job_family_id, job_profile_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    p_id uuid;
    sum_pct int;
    primary_count int;
    parent_exists boolean;
BEGIN
    t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
    p_id := COALESCE(NEW.job_profile_id, OLD.job_profile_id);

    SELECT EXISTS (
        SELECT 1
        FROM org_job_profiles p
        WHERE p.tenant_id = t_id AND p.id = p_id
    ) INTO parent_exists;

    IF NOT parent_exists THEN
        RETURN NULL;
    END IF;

    SELECT
        COALESCE(SUM(allocation_percent), 0),
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO sum_pct, primary_count
    FROM org_job_profile_job_families
    WHERE tenant_id = t_id AND job_profile_id = p_id;

    IF sum_pct <> 100 THEN
        RAISE EXCEPTION 'job profile job families allocation must sum to 100 (tenant_id=%, job_profile_id=%, sum=%)',
            t_id, p_id, sum_pct
        USING ERRCODE = '23514';
    END IF;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'job profile job families must have exactly one primary (tenant_id=%, job_profile_id=%, count=%)',
            t_id, p_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;

CREATE CONSTRAINT TRIGGER org_job_profile_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_job_profile_job_families_validate();

-- 2) Position slice -> Job Families (effective allocations; default copy from profile, override allowed)
CREATE TABLE IF NOT EXISTS org_position_slice_job_families (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    position_slice_id uuid NOT NULL,
    job_family_id uuid NOT NULL,
    allocation_percent int NOT NULL,
    is_primary boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_position_slice_job_families_pkey PRIMARY KEY (tenant_id, position_slice_id, job_family_id),
    CONSTRAINT org_position_slice_job_families_slice_fk FOREIGN KEY (tenant_id, position_slice_id)
        REFERENCES org_position_slices (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT org_position_slice_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id)
        REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_position_slice_job_families_allocation_check CHECK (allocation_percent >= 1 AND allocation_percent <= 100)
);

CREATE UNIQUE INDEX IF NOT EXISTS org_position_slice_job_families_primary_unique
    ON org_position_slice_job_families (tenant_id, position_slice_id)
    WHERE is_primary = TRUE;

CREATE INDEX IF NOT EXISTS org_position_slice_job_families_tenant_family_slice_idx
    ON org_position_slice_job_families (tenant_id, job_family_id, position_slice_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_position_slice_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    s_id uuid;
    sum_pct int;
    primary_count int;
    parent_exists boolean;
BEGIN
    t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
    s_id := COALESCE(NEW.position_slice_id, OLD.position_slice_id);

    SELECT EXISTS (
        SELECT 1
        FROM org_position_slices s
        WHERE s.tenant_id = t_id AND s.id = s_id
    ) INTO parent_exists;

    IF NOT parent_exists THEN
        RETURN NULL;
    END IF;

    SELECT
        COALESCE(SUM(allocation_percent), 0),
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO sum_pct, primary_count
    FROM org_position_slice_job_families
    WHERE tenant_id = t_id AND position_slice_id = s_id;

    IF sum_pct <> 100 THEN
        RAISE EXCEPTION 'position slice job families allocation must sum to 100 (tenant_id=%, position_slice_id=%, sum=%)',
            t_id, s_id, sum_pct
        USING ERRCODE = '23514';
    END IF;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'position slice job families must have exactly one primary (tenant_id=%, position_slice_id=%, count=%)',
            t_id, s_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_position_slice_job_families_validate_trigger ON org_position_slice_job_families;

CREATE CONSTRAINT TRIGGER org_position_slice_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_position_slice_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_position_slice_job_families_validate();

-- 3) Backfill job_profile_id for legacy slices (required before dropping legacy job_*_code columns).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM org_position_slices
        WHERE job_profile_id IS NULL
          AND (job_family_group_code IS NULL OR job_family_code IS NULL OR job_role_code IS NULL)
    ) THEN
        RAISE EXCEPTION 'cannot migrate: org_position_slices has NULL job_profile_id but missing legacy job codes';
    END IF;
END $$;
-- +goose StatementEnd

-- Create a deterministic "one profile per role" baseline for roles referenced by legacy slices.
INSERT INTO org_job_profiles (tenant_id, code, name, description, job_role_id, is_active)
SELECT DISTINCT
    r.tenant_id,
    CONCAT(g.code, '-', f.code, '-', r.code) AS code,
    r.name,
    NULL,
    r.id,
    r.is_active
FROM org_position_slices s
JOIN org_job_family_groups g ON g.tenant_id = s.tenant_id AND g.code = s.job_family_group_code
JOIN org_job_families f ON f.tenant_id = s.tenant_id AND f.job_family_group_id = g.id AND f.code = s.job_family_code
JOIN org_job_roles r ON r.tenant_id = s.tenant_id AND r.job_family_id = f.id AND r.code = s.job_role_code
WHERE s.job_profile_id IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM org_job_profiles p
      WHERE p.tenant_id = r.tenant_id AND p.job_role_id = r.id
  );

-- Backfill slices to the selected profile under that role (deterministic selection if multiple profiles exist).
UPDATE org_position_slices s
SET job_profile_id = p.id
FROM org_job_family_groups g
JOIN org_job_families f ON f.tenant_id = g.tenant_id AND f.job_family_group_id = g.id
JOIN org_job_roles r ON r.tenant_id = f.tenant_id AND r.job_family_id = f.id
JOIN LATERAL (
    SELECT p.id
    FROM org_job_profiles p
    WHERE p.tenant_id = r.tenant_id AND p.job_role_id = r.id
    ORDER BY p.is_active DESC, p.created_at ASC, p.code ASC
    LIMIT 1
) p ON TRUE
WHERE s.job_profile_id IS NULL
  AND g.tenant_id = s.tenant_id
  AND g.code = s.job_family_group_code
  AND f.code = s.job_family_code
  AND r.code = s.job_role_code;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM org_position_slices WHERE job_profile_id IS NULL) THEN
        RAISE EXCEPTION 'cannot migrate: job_profile_id backfill failed (still NULL rows exist)';
    END IF;
END $$;
-- +goose StatementEnd

-- 4) Backfill profile default allocations (100% to the role's job_family).
INSERT INTO org_job_profile_job_families (tenant_id, job_profile_id, job_family_id, allocation_percent, is_primary)
SELECT p.tenant_id, p.id, r.job_family_id, 100, TRUE
FROM org_job_profiles p
JOIN org_job_roles r ON r.tenant_id = p.tenant_id AND r.id = p.job_role_id
ON CONFLICT (tenant_id, job_profile_id, job_family_id) DO NOTHING;

-- 5) Backfill position slice allocations by copying from the selected job profile.
INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, allocation_percent, is_primary)
SELECT s.tenant_id, s.id, jpf.job_family_id, jpf.allocation_percent, jpf.is_primary
FROM org_position_slices s
JOIN org_job_profile_job_families jpf
    ON jpf.tenant_id = s.tenant_id
   AND jpf.job_profile_id = s.job_profile_id
ON CONFLICT (tenant_id, position_slice_id, job_family_id) DO NOTHING;

-- 6) Remove "allowed job levels" (no longer maintained).
-- atlas:nolint DS102
DROP TABLE IF EXISTS org_job_profile_allowed_job_levels;

-- 7) Deduplicate job levels by (tenant_id, code) before making them tenant-global.
WITH ranked AS (
    SELECT
        id,
        tenant_id,
        code,
        ROW_NUMBER() OVER (
            PARTITION BY tenant_id, code
            ORDER BY is_active DESC, display_order ASC, created_at ASC, id ASC
        ) AS rn
    FROM org_job_levels
)
DELETE FROM org_job_levels l
USING ranked r
WHERE l.id = r.id
  AND r.rn > 1;

-- 8) Job profiles: remove job_role_id
-- atlas:nolint CD101
ALTER TABLE org_job_profiles
    DROP CONSTRAINT IF EXISTS org_job_profiles_role_fk;

-- atlas:nolint DS103
ALTER TABLE org_job_profiles
    DROP COLUMN IF EXISTS job_role_id;

DROP INDEX IF EXISTS org_job_profiles_tenant_role_active_code_idx;

CREATE INDEX IF NOT EXISTS org_job_profiles_tenant_active_code_idx
    ON org_job_profiles (tenant_id, is_active, code);

-- 9) Job levels: remove job_role_id and enforce tenant-global uniqueness on code
-- atlas:nolint CD101
ALTER TABLE org_job_levels
    DROP CONSTRAINT IF EXISTS org_job_levels_role_fk;

ALTER TABLE org_job_levels
    DROP CONSTRAINT IF EXISTS org_job_levels_tenant_id_role_code_key;

DROP INDEX IF EXISTS org_job_levels_tenant_role_active_order_code_idx;

-- atlas:nolint DS103
ALTER TABLE org_job_levels
    DROP COLUMN IF EXISTS job_role_id;

-- atlas:nolint MF101
-- Some legacy/dev DBs may already have this tenant-global uniqueness in place.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_job_levels_tenant_id_code_key'
    ) THEN
        ALTER TABLE org_job_levels
            ADD CONSTRAINT org_job_levels_tenant_id_code_key UNIQUE (tenant_id, code);
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS org_job_levels_tenant_active_order_code_idx
    ON org_job_levels (tenant_id, is_active, display_order, code);

-- 10) Drop Job Roles (after all dependencies removed).
-- atlas:nolint DS102
-- Legacy table (not part of DEV-PLAN-072 model). It can block dropping org_job_roles.
DROP TABLE IF EXISTS org_job_role_family_allocations;

DROP TABLE IF EXISTS org_job_roles;

-- 11) Position slices: require job_profile_id and remove legacy job_*_code columns.
-- org_position_slices has deferrable constraint triggers (e.g. gap-free). Ensure they are fired
-- before attempting ALTER TABLE in the same transaction.
SET CONSTRAINTS ALL IMMEDIATE;

-- atlas:nolint MF104
ALTER TABLE org_position_slices
    ALTER COLUMN job_profile_id SET NOT NULL;

-- atlas:nolint DS103
ALTER TABLE org_position_slices
    DROP COLUMN IF EXISTS job_family_group_code,
    DROP COLUMN IF EXISTS job_family_code,
    DROP COLUMN IF EXISTS job_role_code;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_position_slices_job_profile_fk'
    ) THEN
        ALTER TABLE org_position_slices
            ADD CONSTRAINT org_position_slices_job_profile_fk
            FOREIGN KEY (tenant_id, job_profile_id)
            REFERENCES org_job_profiles (tenant_id, id) ON DELETE RESTRICT;
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS org_position_slices_tenant_profile_effective_idx
    ON org_position_slices (tenant_id, job_profile_id, effective_date);

-- +goose Down
-- NOTE: This is a destructive migration; Down is best-effort and primarily restores columns/tables.

ALTER TABLE org_position_slices
    DROP CONSTRAINT IF EXISTS org_position_slices_job_profile_fk;

DROP INDEX IF EXISTS org_position_slices_tenant_profile_effective_idx;

ALTER TABLE org_position_slices
    ADD COLUMN IF NOT EXISTS job_family_group_code varchar(64) NULL,
    ADD COLUMN IF NOT EXISTS job_family_code varchar(64) NULL,
    ADD COLUMN IF NOT EXISTS job_role_code varchar(64) NULL;

ALTER TABLE org_position_slices
    ALTER COLUMN job_profile_id DROP NOT NULL;

-- Recreate legacy tables/columns (nullable; data cannot be fully restored).
ALTER TABLE org_job_profiles
    ADD COLUMN IF NOT EXISTS job_role_id uuid NULL;

ALTER TABLE org_job_levels
    ADD COLUMN IF NOT EXISTS job_role_id uuid NULL;

ALTER TABLE org_job_levels
    DROP CONSTRAINT IF EXISTS org_job_levels_tenant_id_code_key;

DROP INDEX IF EXISTS org_job_levels_tenant_active_order_code_idx;

CREATE TABLE IF NOT EXISTS org_job_roles (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    job_family_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_roles_pkey PRIMARY KEY (id),
    CONSTRAINT org_job_roles_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_job_roles_family_fk FOREIGN KEY (tenant_id, job_family_id) REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_job_roles_tenant_id_family_code_key UNIQUE (tenant_id, job_family_id, code)
);

CREATE INDEX IF NOT EXISTS org_job_roles_tenant_family_active_code_idx ON org_job_roles (tenant_id, job_family_id, is_active, code);

CREATE TABLE IF NOT EXISTS org_job_profile_allowed_job_levels (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    job_profile_id uuid NOT NULL,
    job_level_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_profile_allowed_job_levels_pkey PRIMARY KEY (tenant_id, job_profile_id, job_level_id)
);

DROP TRIGGER IF EXISTS org_position_slice_job_families_validate_trigger ON org_position_slice_job_families;
DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;

DROP TABLE IF EXISTS org_position_slice_job_families;
DROP TABLE IF EXISTS org_job_profile_job_families;

DROP FUNCTION IF EXISTS org_position_slice_job_families_validate();
DROP FUNCTION IF EXISTS org_job_profile_job_families_validate();
