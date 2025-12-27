-- +goose Up
-- DEV-PLAN-064: Introduce day-granularity (date) columns for Valid Time.

-- 1) Add date columns (nullable first).
ALTER TABLE org_node_slices
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_edges
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_hierarchy_closure
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_positions
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_position_slices
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_assignments
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_attribute_inheritance_rules
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_role_assignments
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_security_group_mappings
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_links
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_audit_logs
    ADD COLUMN IF NOT EXISTS effective_on date,
    ADD COLUMN IF NOT EXISTS end_on date;

ALTER TABLE org_personnel_events
    ADD COLUMN IF NOT EXISTS effective_on date;

-- 2) Backfill (derive day-level inclusive end from the existing half-open timestamps).
UPDATE org_node_slices
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_edges
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_hierarchy_closure
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_positions
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_position_slices
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_assignments
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_attribute_inheritance_rules
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_role_assignments
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_security_group_mappings
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_links
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_audit_logs
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date,
    end_on = CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE (((end_date AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
    END
WHERE effective_on IS NULL OR end_on IS NULL;

UPDATE org_personnel_events
SET
    effective_on = (effective_date AT TIME ZONE 'UTC')::date
WHERE effective_on IS NULL;

-- 3) Enforce NOT NULL + defaults (for open-ended end_on).
ALTER TABLE org_node_slices
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_edges
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_hierarchy_closure
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_positions
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_position_slices
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_assignments
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_attribute_inheritance_rules
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_role_assignments
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_security_group_mappings
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_links
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_audit_logs
    ALTER COLUMN effective_on SET NOT NULL,
    ALTER COLUMN end_on SET NOT NULL,
    ALTER COLUMN end_on SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_personnel_events
    ALTER COLUMN effective_on SET NOT NULL;

-- 4) Add day-granularity constraints (no overlaps; allow single-day slices).
ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_node_slices_tenant_node_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        org_node_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        parent_hint gist_uuid_ops WITH =,
        lower(name) gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_edges_tenant_child_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        child_node_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_positions_tenant_code_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        code WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_position_slices_tenant_position_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_assignments_tenant_subject_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    ),
    ADD CONSTRAINT org_assignments_tenant_position_subject_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_attribute_inheritance_rules_tenant_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        hierarchy_type gist_text_ops WITH =,
        attribute_name gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_role_assignments_tenant_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        role_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        org_node_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_security_group_mappings_tenant_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        org_node_id gist_uuid_ops WITH =,
        security_group_key gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_links
    ADD CONSTRAINT org_links_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_links_tenant_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        org_node_id gist_uuid_ops WITH =,
        object_type gist_text_ops WITH =,
        object_key gist_text_ops WITH =,
        link_type gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_audit_logs
    ADD CONSTRAINT org_audit_logs_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_effective_on_check CHECK (effective_on <= end_on),
    ADD CONSTRAINT org_hierarchy_closure_tenant_no_overlap_on EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        hierarchy_type gist_text_ops WITH =,
        build_id gist_uuid_ops WITH =,
        ancestor_node_id gist_uuid_ops WITH =,
        descendant_node_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

-- 5) Add date indexes for common as-of access patterns.
CREATE INDEX IF NOT EXISTS org_node_slices_tenant_node_effective_on_idx ON org_node_slices (tenant_id, org_node_id, effective_on);
CREATE INDEX IF NOT EXISTS org_node_slices_tenant_parent_effective_on_idx ON org_node_slices (tenant_id, parent_hint, effective_on);
CREATE INDEX IF NOT EXISTS org_edges_tenant_child_effective_on_idx ON org_edges (tenant_id, child_node_id, effective_on);
CREATE INDEX IF NOT EXISTS org_edges_tenant_parent_effective_on_idx ON org_edges (tenant_id, parent_node_id, effective_on);
CREATE INDEX IF NOT EXISTS org_positions_tenant_node_effective_on_idx ON org_positions (tenant_id, org_node_id, effective_on);
CREATE INDEX IF NOT EXISTS org_positions_tenant_code_effective_on_idx ON org_positions (tenant_id, code, effective_on);
CREATE INDEX IF NOT EXISTS org_position_slices_tenant_position_effective_on_idx ON org_position_slices (tenant_id, position_id, effective_on);
CREATE INDEX IF NOT EXISTS org_assignments_tenant_subject_effective_on_idx ON org_assignments (tenant_id, subject_id, effective_on);
CREATE INDEX IF NOT EXISTS org_assignments_tenant_position_effective_on_idx ON org_assignments (tenant_id, position_id, effective_on);
CREATE INDEX IF NOT EXISTS org_assignments_tenant_pernr_effective_on_idx ON org_assignments (tenant_id, pernr, effective_on);
CREATE INDEX IF NOT EXISTS org_personnel_events_tenant_person_effective_on_idx ON org_personnel_events (tenant_id, person_uuid, effective_on DESC);

-- +goose Down
DROP INDEX IF EXISTS org_personnel_events_tenant_person_effective_on_idx;
DROP INDEX IF EXISTS org_assignments_tenant_pernr_effective_on_idx;
DROP INDEX IF EXISTS org_assignments_tenant_position_effective_on_idx;
DROP INDEX IF EXISTS org_assignments_tenant_subject_effective_on_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_position_effective_on_idx;
DROP INDEX IF EXISTS org_positions_tenant_code_effective_on_idx;
DROP INDEX IF EXISTS org_positions_tenant_node_effective_on_idx;
DROP INDEX IF EXISTS org_edges_tenant_parent_effective_on_idx;
DROP INDEX IF EXISTS org_edges_tenant_child_effective_on_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_parent_effective_on_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_node_effective_on_idx;

ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_tenant_no_overlap_on;
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_effective_on_check;

ALTER TABLE org_audit_logs DROP CONSTRAINT IF EXISTS org_audit_logs_effective_on_check;

ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_tenant_no_overlap_on;
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_effective_on_check;

ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_tenant_no_overlap_on;
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_effective_on_check;

ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_tenant_no_overlap_on;
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_effective_on_check;

ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_tenant_no_overlap_on;
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_effective_on_check;

ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_position_subject_no_overlap_on;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_subject_no_overlap_on;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_effective_on_check;

ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_tenant_position_no_overlap_on;
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_effective_on_check;

ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_tenant_code_no_overlap_on;
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_effective_on_check;

ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_tenant_child_no_overlap_on;
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_effective_on_check;

ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_tenant_node_no_overlap_on;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_sibling_name_unique_on;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_effective_on_check;

ALTER TABLE org_personnel_events DROP COLUMN IF EXISTS effective_on;

ALTER TABLE org_audit_logs
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_links
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_security_group_mappings
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_role_assignments
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_attribute_inheritance_rules
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_assignments
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_position_slices
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_positions
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_hierarchy_closure
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_edges
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

ALTER TABLE org_node_slices
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;
