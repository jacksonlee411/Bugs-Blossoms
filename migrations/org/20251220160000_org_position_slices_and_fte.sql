-- +goose Up
-- DEV-PLAN-053: org_position_slices + allocated_fte + constraints for partial fill.

CREATE TABLE IF NOT EXISTS org_position_slices (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    position_id uuid NOT NULL,
    org_node_id uuid NOT NULL,
    title text NULL,
    lifecycle_status text NOT NULL DEFAULT 'active',
    position_type text NULL,
    employment_type text NULL,
    capacity_fte numeric(9, 2) NOT NULL DEFAULT 1.0,
    capacity_headcount int NULL,
    reports_to_position_id uuid NULL,
    job_family_group_code varchar(64) NULL,
    job_family_code varchar(64) NULL,
    job_role_code varchar(64) NULL,
    job_level_code varchar(64) NULL,
    job_profile_id uuid NULL,
    cost_center_code varchar(64) NULL,
    profile jsonb NOT NULL DEFAULT '{}'::jsonb,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_position_slices_pkey PRIMARY KEY (id),
    CONSTRAINT org_position_slices_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_position_slices_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_position_slices_lifecycle_status_check CHECK (lifecycle_status IN ('planned', 'active', 'inactive', 'rescinded')),
    CONSTRAINT org_position_slices_capacity_fte_check CHECK (capacity_fte > 0),
    CONSTRAINT org_position_slices_capacity_headcount_check CHECK (capacity_headcount IS NULL OR capacity_headcount >= 0),
    CONSTRAINT org_position_slices_reports_to_self_check CHECK (reports_to_position_id IS NULL OR reports_to_position_id <> position_id),
    CONSTRAINT org_position_slices_profile_is_object_check CHECK (jsonb_typeof(profile) = 'object'),
    CONSTRAINT org_position_slices_position_fk FOREIGN KEY (tenant_id, position_id) REFERENCES org_positions (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_position_slices_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_position_slices_reports_to_fk FOREIGN KEY (tenant_id, reports_to_position_id) REFERENCES org_positions (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_no_overlap
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

CREATE INDEX IF NOT EXISTS org_position_slices_tenant_position_effective_idx ON org_position_slices (tenant_id, position_id, effective_date);
CREATE INDEX IF NOT EXISTS org_position_slices_tenant_node_effective_idx ON org_position_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX IF NOT EXISTS org_position_slices_tenant_reports_to_effective_idx ON org_position_slices (tenant_id, reports_to_position_id, effective_date);

-- Backfill: treat existing org_positions rows as the first slice for that stable position_id.
INSERT INTO org_position_slices (
    tenant_id,
    position_id,
    org_node_id,
    title,
    lifecycle_status,
    capacity_fte,
    effective_date,
    end_date,
    created_at,
    updated_at
)
SELECT
    p.tenant_id,
    p.id AS position_id,
    p.org_node_id,
    p.title,
    CASE
        WHEN p.status = 'active' THEN 'active'
        WHEN p.status = 'retired' THEN 'inactive'
        WHEN p.status = 'rescinded' THEN 'rescinded'
        ELSE 'active'
    END AS lifecycle_status,
    1.0::numeric(9, 2) AS capacity_fte,
    p.effective_date,
    p.end_date,
    p.created_at,
    p.updated_at
FROM org_positions p
WHERE NOT EXISTS (
    SELECT 1
    FROM org_position_slices s
    WHERE s.tenant_id = p.tenant_id
      AND s.position_id = p.id
      AND s.effective_date = p.effective_date
      AND s.end_date = p.end_date
);

-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE org_positions
        ADD CONSTRAINT org_positions_tenant_id_code_key UNIQUE (tenant_id, code);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END;
$$;
-- +goose StatementEnd

ALTER TABLE org_assignments
    ADD COLUMN IF NOT EXISTS allocated_fte numeric(9, 2) NOT NULL DEFAULT 1.0;

-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE org_assignments
        ADD CONSTRAINT org_assignments_allocated_fte_check CHECK (allocated_fte > 0);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END;
$$;
-- +goose StatementEnd

ALTER TABLE org_assignments
    DROP CONSTRAINT IF EXISTS org_assignments_position_unique_in_time;

-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE org_assignments
        ADD CONSTRAINT org_assignments_subject_position_unique_in_time
        EXCLUDE USING gist (
            tenant_id gist_uuid_ops WITH =,
            position_id gist_uuid_ops WITH =,
            subject_type gist_text_ops WITH =,
            subject_id gist_uuid_ops WITH =,
            assignment_type gist_text_ops WITH =,
            tstzrange(effective_date, end_date, '[)') WITH &&
        );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE org_assignments
    DROP CONSTRAINT IF EXISTS org_assignments_subject_position_unique_in_time;

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_position_unique_in_time
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

ALTER TABLE org_assignments
    DROP CONSTRAINT IF EXISTS org_assignments_allocated_fte_check;

ALTER TABLE org_assignments
    DROP COLUMN IF EXISTS allocated_fte;

ALTER TABLE org_positions
    DROP CONSTRAINT IF EXISTS org_positions_tenant_id_code_key;

DROP TABLE IF EXISTS org_position_slices;
