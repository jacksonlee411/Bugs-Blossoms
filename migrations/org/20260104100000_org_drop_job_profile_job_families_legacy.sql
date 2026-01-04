-- +goose Up
-- DEV-PLAN-075 (Phase D): retire legacy org_job_profile_job_families (replaced by org_job_profile_slice_job_families).

DROP TRIGGER IF EXISTS org_job_profile_job_families_validate_trigger ON org_job_profile_job_families;
DROP FUNCTION IF EXISTS org_job_profile_job_families_validate();
-- atlas:nolint DS102
DROP TABLE IF EXISTS org_job_profile_job_families;

-- +goose Down
-- Best-effort: restore the legacy table shape (does not backfill historical rows).

CREATE TABLE IF NOT EXISTS org_job_profile_job_families (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    job_profile_id uuid NOT NULL,
    job_family_id uuid NOT NULL,
    is_primary boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_profile_job_families_pkey PRIMARY KEY (tenant_id, job_profile_id, job_family_id),
    CONSTRAINT org_job_profile_job_families_profile_fk FOREIGN KEY (tenant_id, job_profile_id) REFERENCES org_job_profiles (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT org_job_profile_job_families_family_fk FOREIGN KEY (tenant_id, job_family_id) REFERENCES org_job_families (tenant_id, id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS org_job_profile_job_families_primary_unique ON org_job_profile_job_families (tenant_id, job_profile_id)
WHERE
    is_primary = TRUE;

CREATE INDEX IF NOT EXISTS org_job_profile_job_families_tenant_family_profile_idx ON org_job_profile_job_families (tenant_id, job_family_id, job_profile_id);

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
