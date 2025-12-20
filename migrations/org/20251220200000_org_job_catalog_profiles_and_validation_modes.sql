-- +goose Up
-- DEV-PLAN-056: Job Catalog / Job Profile + tenant-level validation mode switches.

CREATE TABLE IF NOT EXISTS org_job_family_groups (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    code varchar(64) NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_family_groups_pkey PRIMARY KEY (id),
    CONSTRAINT org_job_family_groups_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_job_family_groups_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE INDEX IF NOT EXISTS org_job_family_groups_tenant_active_code_idx ON org_job_family_groups (tenant_id, is_active, code);

CREATE TABLE IF NOT EXISTS org_job_families (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    job_family_group_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_families_pkey PRIMARY KEY (id),
    CONSTRAINT org_job_families_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_job_families_family_group_fk FOREIGN KEY (tenant_id, job_family_group_id) REFERENCES org_job_family_groups (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_job_families_tenant_id_group_code_key UNIQUE (tenant_id, job_family_group_id, code)
);

CREATE INDEX IF NOT EXISTS org_job_families_tenant_group_active_code_idx ON org_job_families (tenant_id, job_family_group_id, is_active, code);

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

CREATE TABLE IF NOT EXISTS org_job_levels (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    job_role_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    name text NOT NULL,
    display_order int NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_levels_pkey PRIMARY KEY (id),
    CONSTRAINT org_job_levels_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_job_levels_display_order_check CHECK (display_order >= 0),
    CONSTRAINT org_job_levels_role_fk FOREIGN KEY (tenant_id, job_role_id) REFERENCES org_job_roles (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_job_levels_tenant_id_role_code_key UNIQUE (tenant_id, job_role_id, code)
);

CREATE INDEX IF NOT EXISTS org_job_levels_tenant_role_active_order_code_idx ON org_job_levels (tenant_id, job_role_id, is_active, display_order, code);

CREATE TABLE IF NOT EXISTS org_job_profiles (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    code varchar(64) NOT NULL,
    name text NOT NULL,
    description text NULL,
    job_role_id uuid NOT NULL,
    is_active boolean NOT NULL DEFAULT TRUE,
    external_refs jsonb NOT NULL DEFAULT '{}' ::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_profiles_pkey PRIMARY KEY (id),
    CONSTRAINT org_job_profiles_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_job_profiles_external_refs_is_object_check CHECK (jsonb_typeof(external_refs) = 'object'),
    CONSTRAINT org_job_profiles_role_fk FOREIGN KEY (tenant_id, job_role_id) REFERENCES org_job_roles (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_job_profiles_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE INDEX IF NOT EXISTS org_job_profiles_tenant_role_active_code_idx ON org_job_profiles (tenant_id, job_role_id, is_active, code);

CREATE TABLE IF NOT EXISTS org_job_profile_allowed_job_levels (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    job_profile_id uuid NOT NULL,
    job_level_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_job_profile_allowed_job_levels_pkey PRIMARY KEY (tenant_id, job_profile_id, job_level_id),
    CONSTRAINT org_job_profile_allowed_job_levels_profile_fk FOREIGN KEY (tenant_id, job_profile_id) REFERENCES org_job_profiles (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT org_job_profile_allowed_job_levels_level_fk FOREIGN KEY (tenant_id, job_level_id) REFERENCES org_job_levels (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_settings
    ADD COLUMN IF NOT EXISTS position_catalog_validation_mode text NOT NULL DEFAULT 'shadow',
    ADD COLUMN IF NOT EXISTS position_restrictions_validation_mode text NOT NULL DEFAULT 'shadow';

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_settings_position_catalog_validation_mode_check'
    ) THEN
        ALTER TABLE org_settings
            ADD CONSTRAINT org_settings_position_catalog_validation_mode_check
            CHECK (position_catalog_validation_mode IN ('disabled', 'shadow', 'enforce'));
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_settings_position_restrictions_validation_mode_check'
    ) THEN
        ALTER TABLE org_settings
            ADD CONSTRAINT org_settings_position_restrictions_validation_mode_check
            CHECK (position_restrictions_validation_mode IN ('disabled', 'shadow', 'enforce'));
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE org_settings
    DROP CONSTRAINT IF EXISTS org_settings_position_restrictions_validation_mode_check;

ALTER TABLE org_settings
    DROP CONSTRAINT IF EXISTS org_settings_position_catalog_validation_mode_check;

ALTER TABLE org_settings
    DROP COLUMN IF EXISTS position_restrictions_validation_mode;

ALTER TABLE org_settings
    DROP COLUMN IF EXISTS position_catalog_validation_mode;

DROP TABLE IF EXISTS org_job_profile_allowed_job_levels;
DROP TABLE IF EXISTS org_job_profiles;
DROP TABLE IF EXISTS org_job_levels;
DROP TABLE IF EXISTS org_job_roles;
DROP TABLE IF EXISTS org_job_families;
DROP TABLE IF EXISTS org_job_family_groups;
