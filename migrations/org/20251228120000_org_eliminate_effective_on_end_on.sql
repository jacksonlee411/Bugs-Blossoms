-- +goose Up
-- DEV-PLAN-064A: Eliminate effective_on/end_on dual-track columns (Org) and drop legacy timestamptz Valid Time.

-- NOTE: This migration intentionally keeps Audit/Tx Time as timestamptz.

-- 1) org_node_slices
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_no_overlap;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_sibling_name_unique;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_tenant_node_no_overlap_on;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_sibling_name_unique_on;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_effective_check;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_effective_on_check;
DROP INDEX IF EXISTS org_node_slices_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_parent_effective_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_node_effective_on_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_parent_effective_on_idx;

ALTER TABLE org_node_slices
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_node_slices ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, parent_hint gist_uuid_ops WITH =, lower(name) gist_text_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_node_slices_tenant_node_effective_idx ON org_node_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX org_node_slices_tenant_parent_effective_idx ON org_node_slices (tenant_id, parent_hint, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_node_slices
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 2) org_edges
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_single_parent_no_overlap;
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_tenant_child_no_overlap_on;
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_effective_check;
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_effective_on_check;
DROP INDEX IF EXISTS org_edges_tenant_parent_effective_idx;
DROP INDEX IF EXISTS org_edges_tenant_child_effective_idx;
DROP INDEX IF EXISTS org_edges_tenant_child_effective_on_idx;
DROP INDEX IF EXISTS org_edges_tenant_parent_effective_on_idx;

ALTER TABLE org_edges
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_edges ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_single_parent_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, child_node_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_edges_tenant_parent_effective_idx ON org_edges (tenant_id, parent_node_id, effective_date);
CREATE INDEX org_edges_tenant_child_effective_idx ON org_edges (tenant_id, child_node_id, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_edges
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 3) org_hierarchy_closure
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_pair_window_no_overlap;
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_tenant_no_overlap_on;
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_effective_check;
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_effective_on_check;
DROP INDEX IF EXISTS org_hierarchy_closure_ancestor_range_gist_idx;
DROP INDEX IF EXISTS org_hierarchy_closure_descendant_range_gist_idx;

ALTER TABLE org_hierarchy_closure
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_hierarchy_closure ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_pair_window_no_overlap
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        hierarchy_type gist_text_ops WITH =,
        build_id gist_uuid_ops WITH =,
        ancestor_node_id gist_uuid_ops WITH =,
        descendant_node_id gist_uuid_ops WITH =,
        daterange(effective_date, end_date + 1, '[)') WITH &&
    );

CREATE INDEX org_hierarchy_closure_ancestor_range_gist_idx ON org_hierarchy_closure USING gist (
    tenant_id gist_uuid_ops,
    hierarchy_type gist_text_ops,
    build_id gist_uuid_ops,
    ancestor_node_id gist_uuid_ops,
    daterange(effective_date, end_date + 1, '[)')
);

CREATE INDEX org_hierarchy_closure_descendant_range_gist_idx ON org_hierarchy_closure USING gist (
    tenant_id gist_uuid_ops,
    hierarchy_type gist_text_ops,
    build_id gist_uuid_ops,
    descendant_node_id gist_uuid_ops,
    daterange(effective_date, end_date + 1, '[)')
);

-- atlas:nolint DS103
ALTER TABLE org_hierarchy_closure
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 4) org_positions
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_code_unique_in_time;
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_tenant_code_no_overlap_on;
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_effective_check;
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_effective_on_check;
DROP INDEX IF EXISTS org_positions_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_positions_tenant_code_effective_idx;
DROP INDEX IF EXISTS org_positions_tenant_node_effective_on_idx;
DROP INDEX IF EXISTS org_positions_tenant_code_effective_on_idx;

ALTER TABLE org_positions
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_positions ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_code_unique_in_time
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, code WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_positions_tenant_node_effective_idx ON org_positions (tenant_id, org_node_id, effective_date);
CREATE INDEX org_positions_tenant_code_effective_idx ON org_positions (tenant_id, code, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_positions
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 5) org_position_slices
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_no_overlap;
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_tenant_position_no_overlap_on;
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_effective_check;
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_effective_on_check;
DROP INDEX IF EXISTS org_position_slices_tenant_position_effective_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_reports_to_effective_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_position_effective_on_idx;

ALTER TABLE org_position_slices
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_position_slices ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, position_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_position_slices_tenant_position_effective_idx ON org_position_slices (tenant_id, position_id, effective_date);
CREATE INDEX org_position_slices_tenant_node_effective_idx ON org_position_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX org_position_slices_tenant_reports_to_effective_idx ON org_position_slices (tenant_id, reports_to_position_id, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_position_slices
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 6) org_assignments
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_primary_unique_in_time;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_subject_position_unique_in_time;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_subject_no_overlap_on;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_position_subject_no_overlap_on;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_effective_check;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_effective_on_check;
DROP INDEX IF EXISTS org_assignments_tenant_subject_effective_idx;
DROP INDEX IF EXISTS org_assignments_tenant_position_effective_idx;
DROP INDEX IF EXISTS org_assignments_tenant_pernr_effective_idx;
DROP INDEX IF EXISTS org_assignments_tenant_subject_effective_on_idx;
DROP INDEX IF EXISTS org_assignments_tenant_position_effective_on_idx;
DROP INDEX IF EXISTS org_assignments_tenant_pernr_effective_on_idx;

ALTER TABLE org_assignments
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_assignments ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_primary_unique_in_time
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_date, end_date + 1, '[)') WITH &&
    )
WHERE (assignment_type = 'primary');

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_subject_position_unique_in_time
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_date, end_date + 1, '[)') WITH &&
    );

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_tenant_subject_no_overlap
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_date, end_date + 1, '[)') WITH &&
    );

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_tenant_position_subject_no_overlap
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_date, end_date + 1, '[)') WITH &&
    );

CREATE INDEX org_assignments_tenant_subject_effective_idx ON org_assignments (tenant_id, subject_id, effective_date);
CREATE INDEX org_assignments_tenant_position_effective_idx ON org_assignments (tenant_id, position_id, effective_date);
CREATE INDEX org_assignments_tenant_pernr_effective_idx ON org_assignments (tenant_id, pernr, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_assignments
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 7) org_attribute_inheritance_rules
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_no_overlap;
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_tenant_no_overlap_on;
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_effective_check;
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_effective_on_check;
DROP INDEX IF EXISTS org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx;

ALTER TABLE org_attribute_inheritance_rules
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_attribute_inheritance_rules ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, hierarchy_type gist_text_ops WITH =, attribute_name gist_text_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx ON org_attribute_inheritance_rules (tenant_id, hierarchy_type, attribute_name, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_attribute_inheritance_rules
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 8) org_role_assignments
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_no_overlap;
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_tenant_no_overlap_on;
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_effective_check;
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_effective_on_check;
DROP INDEX IF EXISTS org_role_assignments_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_role_assignments_tenant_subject_effective_idx;

ALTER TABLE org_role_assignments
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_role_assignments ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, role_id gist_uuid_ops WITH =, subject_type gist_text_ops WITH =, subject_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_role_assignments_tenant_node_effective_idx ON org_role_assignments (tenant_id, org_node_id, effective_date);
CREATE INDEX org_role_assignments_tenant_subject_effective_idx ON org_role_assignments (tenant_id, subject_type, subject_id, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_role_assignments
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 9) org_security_group_mappings
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_no_overlap;
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_tenant_no_overlap_on;
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_effective_check;
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_effective_on_check;
DROP INDEX IF EXISTS org_security_group_mappings_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_security_group_mappings_tenant_key_effective_idx;

ALTER TABLE org_security_group_mappings
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_security_group_mappings ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, security_group_key gist_text_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_security_group_mappings_tenant_node_effective_idx ON org_security_group_mappings (tenant_id, org_node_id, effective_date);
CREATE INDEX org_security_group_mappings_tenant_key_effective_idx ON org_security_group_mappings (tenant_id, security_group_key, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_security_group_mappings
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 10) org_links
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_no_overlap;
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_tenant_no_overlap_on;
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_effective_check;
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_effective_on_check;
DROP INDEX IF EXISTS org_links_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_links_tenant_object_effective_idx;

ALTER TABLE org_links
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_links ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_links
    ADD CONSTRAINT org_links_effective_check CHECK (effective_date <= end_date);

ALTER TABLE org_links
    ADD CONSTRAINT org_links_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, object_type gist_text_ops WITH =, object_key gist_text_ops WITH =, link_type gist_text_ops WITH =, daterange(effective_date, end_date + 1, '[)') WITH &&);

CREATE INDEX org_links_tenant_node_effective_idx ON org_links (tenant_id, org_node_id, effective_date);
CREATE INDEX org_links_tenant_object_effective_idx ON org_links (tenant_id, object_type, object_key, effective_date);

-- atlas:nolint DS103
ALTER TABLE org_links
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 11) org_audit_logs
ALTER TABLE org_audit_logs DROP CONSTRAINT IF EXISTS org_audit_logs_effective_check;
ALTER TABLE org_audit_logs DROP CONSTRAINT IF EXISTS org_audit_logs_effective_on_check;

ALTER TABLE org_audit_logs
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date,
    ALTER COLUMN end_date TYPE date USING CASE
        WHEN (end_date AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
        ELSE ((end_date AT TIME ZONE 'UTC') - interval '1 microsecond')::date
    END;
ALTER TABLE org_audit_logs ALTER COLUMN end_date SET DEFAULT DATE '9999-12-31';

ALTER TABLE org_audit_logs
    ADD CONSTRAINT org_audit_logs_effective_check CHECK (effective_date <= end_date);

-- atlas:nolint DS103
ALTER TABLE org_audit_logs
    DROP COLUMN IF EXISTS effective_on,
    DROP COLUMN IF EXISTS end_on;

-- 12) org_personnel_events
DROP INDEX IF EXISTS org_personnel_events_tenant_person_effective_idx;
DROP INDEX IF EXISTS org_personnel_events_tenant_person_effective_on_idx;

ALTER TABLE org_personnel_events
    ALTER COLUMN effective_date TYPE date USING (effective_date AT TIME ZONE 'UTC')::date;

CREATE INDEX org_personnel_events_tenant_person_effective_idx ON org_personnel_events (tenant_id, person_uuid, effective_date DESC);

-- atlas:nolint DS103
ALTER TABLE org_personnel_events
    DROP COLUMN IF EXISTS effective_on;

-- +goose Down
-- Restore dual-track columns: legacy timestamptz effective_date/end_date + date effective_on/end_on.

-- 1) org_node_slices
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_no_overlap;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_sibling_name_unique;
ALTER TABLE org_node_slices DROP CONSTRAINT IF EXISTS org_node_slices_effective_check;
DROP INDEX IF EXISTS org_node_slices_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_node_slices_tenant_parent_effective_idx;

ALTER TABLE org_node_slices RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_node_slices RENAME COLUMN end_date TO end_on;

ALTER TABLE org_node_slices
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_node_slices
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, parent_hint gist_uuid_ops WITH =, lower(name) gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_tenant_node_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, parent_hint gist_uuid_ops WITH =, lower(name) gist_text_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_node_slices_tenant_node_effective_idx ON org_node_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX org_node_slices_tenant_parent_effective_idx ON org_node_slices (tenant_id, parent_hint, effective_date);
CREATE INDEX org_node_slices_tenant_node_effective_on_idx ON org_node_slices (tenant_id, org_node_id, effective_on);
CREATE INDEX org_node_slices_tenant_parent_effective_on_idx ON org_node_slices (tenant_id, parent_hint, effective_on);

-- 2) org_edges
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_single_parent_no_overlap;
ALTER TABLE org_edges DROP CONSTRAINT IF EXISTS org_edges_effective_check;
DROP INDEX IF EXISTS org_edges_tenant_parent_effective_idx;
DROP INDEX IF EXISTS org_edges_tenant_child_effective_idx;

ALTER TABLE org_edges RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_edges RENAME COLUMN end_date TO end_on;

ALTER TABLE org_edges
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_edges
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_single_parent_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, child_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_tenant_child_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, child_node_id gist_uuid_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_edges_tenant_parent_effective_idx ON org_edges (tenant_id, parent_node_id, effective_date);
CREATE INDEX org_edges_tenant_child_effective_idx ON org_edges (tenant_id, child_node_id, effective_date);
CREATE INDEX org_edges_tenant_child_effective_on_idx ON org_edges (tenant_id, child_node_id, effective_on);
CREATE INDEX org_edges_tenant_parent_effective_on_idx ON org_edges (tenant_id, parent_node_id, effective_on);

-- 3) org_hierarchy_closure
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_pair_window_no_overlap;
ALTER TABLE org_hierarchy_closure DROP CONSTRAINT IF EXISTS org_hierarchy_closure_effective_check;
DROP INDEX IF EXISTS org_hierarchy_closure_ancestor_range_gist_idx;
DROP INDEX IF EXISTS org_hierarchy_closure_descendant_range_gist_idx;

ALTER TABLE org_hierarchy_closure RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_hierarchy_closure RENAME COLUMN end_date TO end_on;

ALTER TABLE org_hierarchy_closure
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_hierarchy_closure
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_pair_window_no_overlap
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        hierarchy_type gist_text_ops WITH =,
        build_id gist_uuid_ops WITH =,
        ancestor_node_id gist_uuid_ops WITH =,
        descendant_node_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_tenant_no_overlap_on
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        hierarchy_type gist_text_ops WITH =,
        build_id gist_uuid_ops WITH =,
        ancestor_node_id gist_uuid_ops WITH =,
        descendant_node_id gist_uuid_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

CREATE INDEX org_hierarchy_closure_ancestor_range_gist_idx ON org_hierarchy_closure USING gist (
    tenant_id gist_uuid_ops,
    hierarchy_type gist_text_ops,
    build_id gist_uuid_ops,
    ancestor_node_id gist_uuid_ops,
    tstzrange(effective_date, end_date, '[)')
);

CREATE INDEX org_hierarchy_closure_descendant_range_gist_idx ON org_hierarchy_closure USING gist (
    tenant_id gist_uuid_ops,
    hierarchy_type gist_text_ops,
    build_id gist_uuid_ops,
    descendant_node_id gist_uuid_ops,
    tstzrange(effective_date, end_date, '[)')
);

-- 4) org_positions
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_code_unique_in_time;
ALTER TABLE org_positions DROP CONSTRAINT IF EXISTS org_positions_effective_check;
DROP INDEX IF EXISTS org_positions_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_positions_tenant_code_effective_idx;

ALTER TABLE org_positions RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_positions RENAME COLUMN end_date TO end_on;

ALTER TABLE org_positions
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_positions
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_code_unique_in_time
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, code WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_tenant_code_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, code WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_positions_tenant_node_effective_idx ON org_positions (tenant_id, org_node_id, effective_date);
CREATE INDEX org_positions_tenant_code_effective_idx ON org_positions (tenant_id, code, effective_date);
CREATE INDEX org_positions_tenant_node_effective_on_idx ON org_positions (tenant_id, org_node_id, effective_on);
CREATE INDEX org_positions_tenant_code_effective_on_idx ON org_positions (tenant_id, code, effective_on);

-- 5) org_position_slices
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_no_overlap;
ALTER TABLE org_position_slices DROP CONSTRAINT IF EXISTS org_position_slices_effective_check;
DROP INDEX IF EXISTS org_position_slices_tenant_position_effective_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_position_slices_tenant_reports_to_effective_idx;

ALTER TABLE org_position_slices RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_position_slices RENAME COLUMN end_date TO end_on;

ALTER TABLE org_position_slices
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_position_slices
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, position_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_position_slices
    ADD CONSTRAINT org_position_slices_tenant_position_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, position_id gist_uuid_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_position_slices_tenant_position_effective_idx ON org_position_slices (tenant_id, position_id, effective_date);
CREATE INDEX org_position_slices_tenant_node_effective_idx ON org_position_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX org_position_slices_tenant_reports_to_effective_idx ON org_position_slices (tenant_id, reports_to_position_id, effective_date);
CREATE INDEX org_position_slices_tenant_position_effective_on_idx ON org_position_slices (tenant_id, position_id, effective_on);

-- 6) org_assignments
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_primary_unique_in_time;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_subject_position_unique_in_time;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_subject_no_overlap;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_tenant_position_subject_no_overlap;
ALTER TABLE org_assignments DROP CONSTRAINT IF EXISTS org_assignments_effective_check;
DROP INDEX IF EXISTS org_assignments_tenant_subject_effective_idx;
DROP INDEX IF EXISTS org_assignments_tenant_position_effective_idx;
DROP INDEX IF EXISTS org_assignments_tenant_pernr_effective_idx;

ALTER TABLE org_assignments RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_assignments RENAME COLUMN end_date TO end_on;

ALTER TABLE org_assignments
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_assignments
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_primary_unique_in_time
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    )
WHERE (assignment_type = 'primary');

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

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_tenant_subject_no_overlap_on
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_tenant_position_subject_no_overlap_on
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        daterange(effective_on, end_on + 1, '[)') WITH &&
    );

CREATE INDEX org_assignments_tenant_subject_effective_idx ON org_assignments (tenant_id, subject_id, effective_date);
CREATE INDEX org_assignments_tenant_position_effective_idx ON org_assignments (tenant_id, position_id, effective_date);
CREATE INDEX org_assignments_tenant_pernr_effective_idx ON org_assignments (tenant_id, pernr, effective_date);
CREATE INDEX org_assignments_tenant_subject_effective_on_idx ON org_assignments (tenant_id, subject_id, effective_on);
CREATE INDEX org_assignments_tenant_position_effective_on_idx ON org_assignments (tenant_id, position_id, effective_on);
CREATE INDEX org_assignments_tenant_pernr_effective_on_idx ON org_assignments (tenant_id, pernr, effective_on);

-- 7) org_attribute_inheritance_rules
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_no_overlap;
ALTER TABLE org_attribute_inheritance_rules DROP CONSTRAINT IF EXISTS org_attribute_inheritance_rules_effective_check;
DROP INDEX IF EXISTS org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx;

ALTER TABLE org_attribute_inheritance_rules RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_attribute_inheritance_rules RENAME COLUMN end_date TO end_on;

ALTER TABLE org_attribute_inheritance_rules
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_attribute_inheritance_rules
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, hierarchy_type gist_text_ops WITH =, attribute_name gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_tenant_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, hierarchy_type gist_text_ops WITH =, attribute_name gist_text_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx ON org_attribute_inheritance_rules (tenant_id, hierarchy_type, attribute_name, effective_date);

-- 8) org_role_assignments
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_no_overlap;
ALTER TABLE org_role_assignments DROP CONSTRAINT IF EXISTS org_role_assignments_effective_check;
DROP INDEX IF EXISTS org_role_assignments_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_role_assignments_tenant_subject_effective_idx;

ALTER TABLE org_role_assignments RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_role_assignments RENAME COLUMN end_date TO end_on;

ALTER TABLE org_role_assignments
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_role_assignments
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, role_id gist_uuid_ops WITH =, subject_type gist_text_ops WITH =, subject_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_tenant_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, role_id gist_uuid_ops WITH =, subject_type gist_text_ops WITH =, subject_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_role_assignments_tenant_node_effective_idx ON org_role_assignments (tenant_id, org_node_id, effective_date);
CREATE INDEX org_role_assignments_tenant_subject_effective_idx ON org_role_assignments (tenant_id, subject_type, subject_id, effective_date);

-- 9) org_security_group_mappings
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_no_overlap;
ALTER TABLE org_security_group_mappings DROP CONSTRAINT IF EXISTS org_security_group_mappings_effective_check;
DROP INDEX IF EXISTS org_security_group_mappings_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_security_group_mappings_tenant_key_effective_idx;

ALTER TABLE org_security_group_mappings RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_security_group_mappings RENAME COLUMN end_date TO end_on;

ALTER TABLE org_security_group_mappings
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_security_group_mappings
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, security_group_key gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_tenant_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, security_group_key gist_text_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_security_group_mappings_tenant_node_effective_idx ON org_security_group_mappings (tenant_id, org_node_id, effective_date);
CREATE INDEX org_security_group_mappings_tenant_key_effective_idx ON org_security_group_mappings (tenant_id, security_group_key, effective_date);

-- 10) org_links
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_no_overlap;
ALTER TABLE org_links DROP CONSTRAINT IF EXISTS org_links_effective_check;
DROP INDEX IF EXISTS org_links_tenant_node_effective_idx;
DROP INDEX IF EXISTS org_links_tenant_object_effective_idx;

ALTER TABLE org_links RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_links RENAME COLUMN end_date TO end_on;

ALTER TABLE org_links
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_links
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_links
    ADD CONSTRAINT org_links_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_links
    ADD CONSTRAINT org_links_effective_on_check CHECK (effective_on <= end_on);

ALTER TABLE org_links
    ADD CONSTRAINT org_links_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, object_type gist_text_ops WITH =, object_key gist_text_ops WITH =, link_type gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_links
    ADD CONSTRAINT org_links_tenant_no_overlap_on
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, object_type gist_text_ops WITH =, object_key gist_text_ops WITH =, link_type gist_text_ops WITH =, daterange(effective_on, end_on + 1, '[)') WITH &&);

CREATE INDEX org_links_tenant_node_effective_idx ON org_links (tenant_id, org_node_id, effective_date);
CREATE INDEX org_links_tenant_object_effective_idx ON org_links (tenant_id, object_type, object_key, effective_date);

-- 11) org_audit_logs
ALTER TABLE org_audit_logs DROP CONSTRAINT IF EXISTS org_audit_logs_effective_check;

ALTER TABLE org_audit_logs RENAME COLUMN effective_date TO effective_on;
ALTER TABLE org_audit_logs RENAME COLUMN end_date TO end_on;

ALTER TABLE org_audit_logs
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC'),
    ADD COLUMN end_date timestamptz NOT NULL DEFAULT '9999-12-31';

UPDATE org_audit_logs
SET
    effective_date = (effective_on::timestamp AT TIME ZONE 'UTC'),
    end_date = CASE
        WHEN end_on = DATE '9999-12-31' THEN '9999-12-31'::timestamptz
        ELSE ((end_on + 1)::timestamp AT TIME ZONE 'UTC')
    END;

ALTER TABLE org_audit_logs
    ADD CONSTRAINT org_audit_logs_effective_check CHECK (effective_date < end_date);
ALTER TABLE org_audit_logs
    ADD CONSTRAINT org_audit_logs_effective_on_check CHECK (effective_on <= end_on);

-- 12) org_personnel_events
DROP INDEX IF EXISTS org_personnel_events_tenant_person_effective_idx;

ALTER TABLE org_personnel_events RENAME COLUMN effective_date TO effective_on;

ALTER TABLE org_personnel_events
    ADD COLUMN effective_date timestamptz NOT NULL DEFAULT (effective_on::timestamp AT TIME ZONE 'UTC');

UPDATE org_personnel_events
SET effective_date = (effective_on::timestamp AT TIME ZONE 'UTC');

CREATE INDEX org_personnel_events_tenant_person_effective_idx ON org_personnel_events (tenant_id, person_uuid, effective_date DESC);
CREATE INDEX org_personnel_events_tenant_person_effective_on_idx ON org_personnel_events (tenant_id, person_uuid, effective_on DESC);
