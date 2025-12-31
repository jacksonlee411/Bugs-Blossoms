-- +goose Up
-- DEV-PLAN-074: Remove allocation_percent from job profile / position slice job families.

-- 1) Job Profile -> Job Families: keep set + primary only.
DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;
DROP FUNCTION IF EXISTS org_job_profile_job_families_validate();

ALTER TABLE org_job_profile_job_families
    DROP COLUMN IF EXISTS allocation_percent;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    p_id uuid;
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
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO primary_count
    FROM org_job_profile_job_families
    WHERE tenant_id = t_id AND job_profile_id = p_id;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'job profile job families must have exactly one primary (tenant_id=%, job_profile_id=%, count=%)',
            t_id, p_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER org_job_profile_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_job_profile_job_families_validate();

-- 2) Position Slice -> Job Families: keep set + primary only.
DROP TRIGGER IF EXISTS org_position_slice_job_families_validate_trigger ON org_position_slice_job_families;
DROP FUNCTION IF EXISTS org_position_slice_job_families_validate();

ALTER TABLE org_position_slice_job_families
    DROP COLUMN IF EXISTS allocation_percent;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_position_slice_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    s_id uuid;
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
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO primary_count
    FROM org_position_slice_job_families
    WHERE tenant_id = t_id AND position_slice_id = s_id;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'position slice job families must have exactly one primary (tenant_id=%, position_slice_id=%, count=%)',
            t_id, s_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER org_position_slice_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_position_slice_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_position_slice_job_families_validate();

-- +goose Down
-- Best-effort: restore the column shape (does not restore historical allocation values; no sum=100 enforcement).

DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;
DROP FUNCTION IF EXISTS org_job_profile_job_families_validate();

ALTER TABLE org_job_profile_job_families
    ADD COLUMN IF NOT EXISTS allocation_percent int;

UPDATE org_job_profile_job_families
SET allocation_percent = 100
WHERE allocation_percent IS NULL AND is_primary = TRUE;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    p_id uuid;
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
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO primary_count
    FROM org_job_profile_job_families
    WHERE tenant_id = t_id AND job_profile_id = p_id;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'job profile job families must have exactly one primary (tenant_id=%, job_profile_id=%, count=%)',
            t_id, p_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER org_job_profile_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_job_profile_job_families_validate();

DROP TRIGGER IF EXISTS org_position_slice_job_families_validate_trigger ON org_position_slice_job_families;
DROP FUNCTION IF EXISTS org_position_slice_job_families_validate();

ALTER TABLE org_position_slice_job_families
    ADD COLUMN IF NOT EXISTS allocation_percent int;

UPDATE org_position_slice_job_families
SET allocation_percent = 100
WHERE allocation_percent IS NULL AND is_primary = TRUE;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_position_slice_job_families_validate()
RETURNS trigger AS $$
DECLARE
    t_id uuid;
    s_id uuid;
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
        COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
    INTO primary_count
    FROM org_position_slice_job_families
    WHERE tenant_id = t_id AND position_slice_id = s_id;

    IF primary_count <> 1 THEN
        RAISE EXCEPTION 'position slice job families must have exactly one primary (tenant_id=%, position_slice_id=%, count=%)',
            t_id, s_id, primary_count
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER org_position_slice_job_families_validate_trigger
AFTER INSERT OR UPDATE OR DELETE ON org_position_slice_job_families
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_position_slice_job_families_validate();
