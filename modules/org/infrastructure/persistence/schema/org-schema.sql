CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE EXTENSION IF NOT EXISTS ltree;

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE org_nodes (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    type text NOT NULL DEFAULT 'OrgUnit',
    code varchar(64) NOT NULL,
    is_root boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_nodes_type_check CHECK (type IN ('OrgUnit')),
    CONSTRAINT org_nodes_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_nodes_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE UNIQUE INDEX org_nodes_tenant_root_unique ON org_nodes (tenant_id)
WHERE
    is_root;

CREATE INDEX org_nodes_tenant_code_idx ON org_nodes (tenant_id, code);

CREATE TABLE org_node_slices (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    org_node_id uuid NOT NULL,
    name varchar(255) NOT NULL,
    i18n_names jsonb NOT NULL DEFAULT '{}' ::jsonb,
    status text NOT NULL DEFAULT 'active',
    legal_entity_id uuid NULL,
    company_code text NULL,
    location_id uuid NULL,
    display_order int NOT NULL DEFAULT 0,
    parent_hint uuid NULL,
    manager_user_id bigint NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_node_slices_status_check CHECK (status IN ('active', 'retired', 'rescinded')),
    CONSTRAINT org_node_slices_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_node_slices_parent_hint_not_self CHECK (parent_hint IS NULL OR parent_hint <> org_node_id),
    CONSTRAINT org_node_slices_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_node_slices_parent_hint_fk FOREIGN KEY (tenant_id, parent_hint) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, parent_hint gist_uuid_ops WITH =, lower(name) gist_text_ops WITH =, tstzrange( effective_date, end_date, '[)'
) WITH &&);

CREATE INDEX org_node_slices_tenant_node_effective_idx ON org_node_slices (tenant_id, org_node_id, effective_date);

CREATE INDEX org_node_slices_tenant_parent_effective_idx ON org_node_slices (tenant_id, parent_hint, effective_date);

CREATE TABLE org_edges (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    parent_node_id uuid NULL,
    child_node_id uuid NOT NULL,
    path ltree NOT NULL,
    depth int NOT NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_edges_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_edges_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_edges_parent_not_child CHECK (parent_node_id IS NULL OR parent_node_id <> child_node_id),
    CONSTRAINT org_edges_child_fk FOREIGN KEY (tenant_id, child_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_edges_parent_fk FOREIGN KEY (tenant_id, parent_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_edges
    ADD CONSTRAINT org_edges_single_parent_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, child_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_edges_tenant_path_gist_idx ON org_edges USING gist (tenant_id gist_uuid_ops, path);

CREATE INDEX org_edges_tenant_parent_effective_idx ON org_edges (tenant_id, parent_node_id, effective_date);

CREATE INDEX org_edges_tenant_child_effective_idx ON org_edges (tenant_id, child_node_id, effective_date);

CREATE OR REPLACE FUNCTION org_edges_set_path_depth_and_prevent_cycle ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    AS $$
DECLARE
    parent_path ltree;
    child_path ltree;
    child_key text;
BEGIN
    child_key := replace(lower(NEW.child_node_id::text), '-', '');
    IF NEW.parent_node_id IS NULL THEN
        NEW.path := child_key::ltree;
        NEW.depth := nlevel (NEW.path) - 1;
        RETURN NEW;
    END IF;
    SELECT
        e.path INTO parent_path
    FROM
        org_edges e
    WHERE
        e.tenant_id = NEW.tenant_id
        AND e.child_node_id = NEW.parent_node_id
        AND e.effective_date <= NEW.effective_date
        AND e.end_date > NEW.effective_date
    ORDER BY
        e.effective_date DESC
    LIMIT 1;
    IF parent_path IS NULL THEN
        RAISE EXCEPTION 'org_edges: parent path not found (tenant_id=%, parent_node_id=%, as_of=%)', NEW.tenant_id, NEW.parent_node_id, NEW.effective_date
            USING ERRCODE = 'foreign_key_violation';
        END IF;
        SELECT
            e.path INTO child_path
        FROM
            org_edges e
        WHERE
            e.tenant_id = NEW.tenant_id
            AND e.child_node_id = NEW.child_node_id
            AND e.effective_date <= NEW.effective_date
            AND e.end_date > NEW.effective_date
        ORDER BY
            e.effective_date DESC
        LIMIT 1;
        IF child_path IS NOT NULL AND parent_path <@ child_path THEN
            RAISE EXCEPTION 'org_edges: cycle detected (parent_node_id=% inside child_node_id=% subtree)', NEW.parent_node_id, NEW.child_node_id
                USING ERRCODE = 'integrity_constraint_violation';
            END IF;
            NEW.path := parent_path || child_key::ltree;
            NEW.depth := nlevel (NEW.path) - 1;
            RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION org_edges_prevent_key_updates ()
    RETURNS TRIGGER
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.hierarchy_type IS DISTINCT FROM OLD.hierarchy_type OR NEW.parent_node_id IS DISTINCT FROM OLD.parent_node_id OR NEW.child_node_id IS DISTINCT FROM OLD.child_node_id OR NEW.effective_date IS DISTINCT FROM OLD.effective_date THEN
        RAISE EXCEPTION 'org_edges: updating hierarchy keys is not allowed; use new edge slice + retire old slice'
            USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS org_edges_before_insert_set_path_depth ON org_edges;

CREATE TRIGGER org_edges_before_insert_set_path_depth
    BEFORE INSERT ON org_edges
    FOR EACH ROW
    EXECUTE FUNCTION org_edges_set_path_depth_and_prevent_cycle ();

DROP TRIGGER IF EXISTS org_edges_before_update_prevent_key_updates ON org_edges;

CREATE TRIGGER org_edges_before_update_prevent_key_updates
    BEFORE UPDATE ON org_edges
    FOR EACH ROW
    EXECUTE FUNCTION org_edges_prevent_key_updates ();

-- DEV-PLAN-029: Org deep-read derived tables (closure/snapshots).
CREATE TABLE org_hierarchy_closure_builds (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    build_id uuid NOT NULL DEFAULT gen_random_uuid (),
    status text NOT NULL DEFAULT 'building',
    is_active boolean NOT NULL DEFAULT FALSE,
    built_at timestamptz NOT NULL DEFAULT now(),
    source_request_id text NULL,
    notes text NULL,
    CONSTRAINT org_hierarchy_closure_builds_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_hierarchy_closure_builds_status_check CHECK (status IN ('building', 'ready', 'failed')),
    CONSTRAINT org_hierarchy_closure_builds_pkey PRIMARY KEY (tenant_id, hierarchy_type, build_id)
);

CREATE UNIQUE INDEX org_hierarchy_closure_builds_active_unique ON org_hierarchy_closure_builds (tenant_id, hierarchy_type)
WHERE
    is_active;

CREATE INDEX org_hierarchy_closure_builds_tenant_built_at_idx ON org_hierarchy_closure_builds (tenant_id, hierarchy_type, built_at DESC);

CREATE TABLE org_hierarchy_closure (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    build_id uuid NOT NULL,
    ancestor_node_id uuid NOT NULL,
    descendant_node_id uuid NOT NULL,
    depth int NOT NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    CONSTRAINT org_hierarchy_closure_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_hierarchy_closure_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_hierarchy_closure_depth_check CHECK (depth >= 0),
    CONSTRAINT org_hierarchy_closure_build_fk FOREIGN KEY (tenant_id, hierarchy_type, build_id) REFERENCES org_hierarchy_closure_builds (tenant_id, hierarchy_type, build_id) ON DELETE CASCADE,
    CONSTRAINT org_hierarchy_closure_ancestor_fk FOREIGN KEY (tenant_id, ancestor_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_hierarchy_closure_descendant_fk FOREIGN KEY (tenant_id, descendant_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_hierarchy_closure
    ADD CONSTRAINT org_hierarchy_closure_pair_window_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, hierarchy_type gist_text_ops WITH =, build_id gist_uuid_ops WITH =, ancestor_node_id gist_uuid_ops WITH =, descendant_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_hierarchy_closure_ancestor_range_gist_idx ON org_hierarchy_closure USING gist (tenant_id gist_uuid_ops, hierarchy_type gist_text_ops, build_id gist_uuid_ops, ancestor_node_id gist_uuid_ops, tstzrange(effective_date, end_date, '[)'));

CREATE INDEX org_hierarchy_closure_descendant_range_gist_idx ON org_hierarchy_closure USING gist (tenant_id gist_uuid_ops, hierarchy_type gist_text_ops, build_id gist_uuid_ops, descendant_node_id gist_uuid_ops, tstzrange(effective_date, end_date, '[)'));

CREATE INDEX org_hierarchy_closure_ancestor_btree_idx ON org_hierarchy_closure (tenant_id, hierarchy_type, build_id, ancestor_node_id, depth, descendant_node_id);

CREATE INDEX org_hierarchy_closure_descendant_btree_idx ON org_hierarchy_closure (tenant_id, hierarchy_type, build_id, descendant_node_id, depth, ancestor_node_id);

CREATE TABLE org_hierarchy_snapshot_builds (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    as_of_date date NOT NULL,
    build_id uuid NOT NULL DEFAULT gen_random_uuid (),
    status text NOT NULL DEFAULT 'building',
    is_active boolean NOT NULL DEFAULT FALSE,
    built_at timestamptz NOT NULL DEFAULT now(),
    source_request_id text NULL,
    notes text NULL,
    CONSTRAINT org_hierarchy_snapshot_builds_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_hierarchy_snapshot_builds_status_check CHECK (status IN ('building', 'ready', 'failed')),
    CONSTRAINT org_hierarchy_snapshot_builds_pkey PRIMARY KEY (tenant_id, hierarchy_type, as_of_date, build_id)
);

CREATE UNIQUE INDEX org_hierarchy_snapshot_builds_active_unique ON org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date)
WHERE
    is_active;

CREATE INDEX org_hierarchy_snapshot_builds_tenant_built_at_idx ON org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, built_at DESC);

CREATE TABLE org_hierarchy_snapshots (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    as_of_date date NOT NULL,
    build_id uuid NOT NULL,
    ancestor_node_id uuid NOT NULL,
    descendant_node_id uuid NOT NULL,
    depth int NOT NULL,
    CONSTRAINT org_hierarchy_snapshots_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_hierarchy_snapshots_depth_check CHECK (depth >= 0),
    CONSTRAINT org_hierarchy_snapshots_build_fk FOREIGN KEY (tenant_id, hierarchy_type, as_of_date, build_id) REFERENCES org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, build_id) ON DELETE CASCADE,
    CONSTRAINT org_hierarchy_snapshots_ancestor_fk FOREIGN KEY (tenant_id, ancestor_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_hierarchy_snapshots_descendant_fk FOREIGN KEY (tenant_id, descendant_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_hierarchy_snapshots_unique_pair UNIQUE (tenant_id, hierarchy_type, as_of_date, build_id, ancestor_node_id, descendant_node_id)
);

CREATE INDEX org_hierarchy_snapshots_ancestor_btree_idx ON org_hierarchy_snapshots (tenant_id, hierarchy_type, as_of_date, build_id, ancestor_node_id, depth, descendant_node_id);

CREATE INDEX org_hierarchy_snapshots_descendant_btree_idx ON org_hierarchy_snapshots (tenant_id, hierarchy_type, as_of_date, build_id, descendant_node_id, depth, ancestor_node_id);

-- DEV-PLAN-033: Org reporting snapshot table + active-build BI view.
CREATE TABLE org_reporting_nodes (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    hierarchy_type text NOT NULL DEFAULT 'OrgUnit',
    as_of_date date NOT NULL,
    build_id uuid NOT NULL,
    org_node_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    name text NOT NULL,
    status text NOT NULL,
    parent_node_id uuid NULL,
    depth int NOT NULL,
    path_node_ids uuid[] NOT NULL,
    path_codes text[] NOT NULL,
    path_names text[] NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}' ::jsonb,
    security_group_keys text[] NOT NULL DEFAULT '{}' ::text[],
    links jsonb NOT NULL DEFAULT '[]' ::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_reporting_nodes_hierarchy_type_check CHECK (hierarchy_type IN ('OrgUnit')),
    CONSTRAINT org_reporting_nodes_depth_check CHECK (depth >= 0),
    CONSTRAINT org_reporting_nodes_attributes_is_object_check CHECK (jsonb_typeof(attributes) = 'object'),
    CONSTRAINT org_reporting_nodes_links_is_array_check CHECK (jsonb_typeof(links) = 'array'),
    CONSTRAINT org_reporting_nodes_build_fk FOREIGN KEY (tenant_id, hierarchy_type, as_of_date, build_id) REFERENCES org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, build_id) ON DELETE CASCADE,
    CONSTRAINT org_reporting_nodes_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_reporting_nodes_pkey PRIMARY KEY (tenant_id, hierarchy_type, as_of_date, build_id, org_node_id)
);

CREATE INDEX org_reporting_nodes_lookup_idx ON org_reporting_nodes (tenant_id, hierarchy_type, as_of_date, org_node_id);

CREATE INDEX org_reporting_nodes_code_idx ON org_reporting_nodes (tenant_id, hierarchy_type, as_of_date, code);

CREATE OR REPLACE VIEW org_reporting AS
SELECT
    r.*
FROM
    org_reporting_nodes r
    JOIN org_hierarchy_snapshot_builds b ON b.tenant_id = r.tenant_id
        AND b.hierarchy_type = r.hierarchy_type
        AND b.as_of_date = r.as_of_date
        AND b.build_id = r.build_id
WHERE
    b.is_active = TRUE
    AND b.status = 'ready';

CREATE TABLE org_positions (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    org_node_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    title text NULL,
    status text NOT NULL DEFAULT 'active',
    is_auto_created boolean NOT NULL DEFAULT FALSE,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_positions_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_positions_status_check CHECK (status IN ('active', 'retired', 'rescinded')),
    CONSTRAINT org_positions_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_positions_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_positions
    ADD CONSTRAINT org_positions_code_unique_in_time
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, code WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_positions_tenant_node_effective_idx ON org_positions (tenant_id, org_node_id, effective_date);

CREATE INDEX org_positions_tenant_code_effective_idx ON org_positions (tenant_id, code, effective_date);

CREATE TABLE org_assignments (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    position_id uuid NOT NULL,
    subject_type text NOT NULL DEFAULT 'person',
    subject_id uuid NOT NULL,
    pernr text NOT NULL,
    assignment_type text NOT NULL DEFAULT 'primary',
    is_primary boolean NOT NULL DEFAULT TRUE,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_assignments_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_assignments_subject_type_check CHECK (subject_type IN ('person')),
    CONSTRAINT org_assignments_assignment_type_check CHECK (assignment_type IN ('primary', 'matrix', 'dotted')),
    CONSTRAINT org_assignments_primary_check CHECK ((assignment_type = 'primary') = is_primary),
    CONSTRAINT org_assignments_position_fk FOREIGN KEY (tenant_id, position_id) REFERENCES org_positions (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_primary_unique_in_time
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, subject_type gist_text_ops WITH =, subject_id gist_uuid_ops WITH =, assignment_type gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&)
WHERE (assignment_type = 'primary');

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_position_unique_in_time
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, position_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_assignments_tenant_subject_effective_idx ON org_assignments (tenant_id, subject_id, effective_date);

CREATE INDEX org_assignments_tenant_position_effective_idx ON org_assignments (tenant_id, position_id, effective_date);

CREATE INDEX org_assignments_tenant_pernr_effective_idx ON org_assignments (tenant_id, pernr, effective_date);

CREATE TABLE org_attribute_inheritance_rules (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    hierarchy_type text NOT NULL,
    attribute_name text NOT NULL,
    can_override boolean NOT NULL DEFAULT FALSE,
    inheritance_break_node_type text NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_attribute_inheritance_rules_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_attribute_inheritance_rules_effective_check CHECK (effective_date < end_date)
);

ALTER TABLE org_attribute_inheritance_rules
    ADD CONSTRAINT org_attribute_inheritance_rules_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, hierarchy_type gist_text_ops WITH =, attribute_name gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_attribute_inheritance_rules_tenant_hierarchy_attribute_effective_idx ON org_attribute_inheritance_rules (tenant_id, hierarchy_type, attribute_name, effective_date);

CREATE TABLE org_roles (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    code varchar(64) NOT NULL,
    name varchar(255) NOT NULL,
    description text NULL,
    is_system boolean NOT NULL DEFAULT TRUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_roles_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_roles_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE INDEX org_roles_tenant_name_idx ON org_roles (tenant_id, name);

CREATE TABLE org_role_assignments (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    role_id uuid NOT NULL,
    subject_type text NOT NULL DEFAULT 'user',
    subject_id uuid NOT NULL,
    org_node_id uuid NOT NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_role_assignments_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_role_assignments_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_role_assignments_subject_type_check CHECK (subject_type IN ('user', 'group')),
    CONSTRAINT org_role_assignments_role_fk FOREIGN KEY (tenant_id, role_id) REFERENCES org_roles (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT org_role_assignments_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_role_assignments
    ADD CONSTRAINT org_role_assignments_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, role_id gist_uuid_ops WITH =, subject_type gist_text_ops WITH =, subject_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_role_assignments_tenant_node_effective_idx ON org_role_assignments (tenant_id, org_node_id, effective_date);

CREATE INDEX org_role_assignments_tenant_subject_effective_idx ON org_role_assignments (tenant_id, subject_type, subject_id, effective_date);

CREATE TABLE org_change_requests (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    request_id text NOT NULL,
    requester_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    payload_schema_version int NOT NULL DEFAULT 1,
    payload jsonb NOT NULL,
    notes text NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_change_requests_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_change_requests_status_check CHECK (status IN ('draft', 'submitted', 'approved', 'rejected', 'cancelled')),
    CONSTRAINT org_change_requests_tenant_id_request_id_key UNIQUE (tenant_id, request_id)
);

CREATE INDEX org_change_requests_tenant_requester_status_updated_idx ON org_change_requests (tenant_id, requester_id, status, updated_at DESC);

-- DEV-PLAN-025: org_settings + org_audit_logs (schema SSOT; migrations/org applies the same DDL).
CREATE TABLE org_settings (
    tenant_id uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    freeze_mode text NOT NULL DEFAULT 'enforce',
    freeze_grace_days int NOT NULL DEFAULT 3,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_settings_freeze_mode_check CHECK (freeze_mode IN ('disabled', 'shadow', 'enforce')),
    CONSTRAINT org_settings_freeze_grace_days_check CHECK (freeze_grace_days >= 0 AND freeze_grace_days <= 31)
);

CREATE TABLE org_audit_logs (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    request_id text NOT NULL,
    transaction_time timestamptz NOT NULL,
    initiator_id uuid NOT NULL,
    change_type text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL,
    old_values jsonb NULL,
    new_values jsonb NOT NULL DEFAULT '{}' ::jsonb,
    meta jsonb NOT NULL DEFAULT '{}' ::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_audit_logs_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_audit_logs_entity_type_check CHECK (entity_type IN ('org_node', 'org_edge', 'org_position', 'org_assignment'))
);

CREATE INDEX org_audit_logs_tenant_transaction_time_desc_idx ON org_audit_logs (tenant_id, transaction_time DESC);

CREATE INDEX org_audit_logs_tenant_entity_transaction_time_desc_idx ON org_audit_logs (tenant_id, entity_type, entity_id, transaction_time DESC);

CREATE INDEX org_audit_logs_tenant_request_id_idx ON org_audit_logs (tenant_id, request_id);

-- DEV-PLAN-026: org_outbox (schema SSOT; migrations/org applies the same DDL).
CREATE TABLE org_outbox (
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    topic text NOT NULL,
    payload jsonb NOT NULL,
    event_id uuid NOT NULL,
    sequence bigserial
        NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        published_at timestamptz NULL,
        attempts int NOT NULL DEFAULT 0,
        available_at timestamptz NOT NULL DEFAULT now(),
        locked_at timestamptz NULL,
        last_error text NULL,
        CONSTRAINT org_outbox_pkey PRIMARY KEY (id),
        CONSTRAINT org_outbox_event_id_key UNIQUE (event_id),
        CONSTRAINT org_outbox_attempts_nonnegative CHECK (attempts >= 0)
);

CREATE INDEX org_outbox_pending_by_available ON org_outbox (available_at, SEQUENCE)
WHERE
    published_at IS NULL;

CREATE INDEX org_outbox_published_by_time ON org_outbox (published_at, SEQUENCE)
WHERE
    published_at IS NOT NULL;

CREATE INDEX org_outbox_tenant_published ON org_outbox (tenant_id, published_at, SEQUENCE);

-- DEV-PLAN-032: org_security_group_mappings + org_links (schema SSOT; migrations/org applies the same DDL).
CREATE TABLE org_security_group_mappings (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    org_node_id uuid NOT NULL,
    security_group_key text NOT NULL,
    applies_to_subtree boolean NOT NULL DEFAULT FALSE,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_security_group_mappings_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_security_group_mappings_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_security_group_mappings_security_group_key_check CHECK (char_length(trim(security_group_key)) > 0),
    CONSTRAINT org_security_group_mappings_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_security_group_mappings
    ADD CONSTRAINT org_security_group_mappings_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, security_group_key gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_security_group_mappings_tenant_node_effective_idx ON org_security_group_mappings (tenant_id, org_node_id, effective_date);

CREATE INDEX org_security_group_mappings_tenant_key_effective_idx ON org_security_group_mappings (tenant_id, security_group_key, effective_date);

CREATE TABLE org_links (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    org_node_id uuid NOT NULL,
    object_type text NOT NULL,
    object_key text NOT NULL,
    link_type text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}' ::jsonb,
    effective_date timestamptz NOT NULL,
    end_date timestamptz NOT NULL DEFAULT '9999-12-31',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_links_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_links_effective_check CHECK (effective_date < end_date),
    CONSTRAINT org_links_object_key_check CHECK (char_length(trim(object_key)) > 0),
    CONSTRAINT org_links_metadata_is_object_check CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT org_links_object_type_check CHECK (object_type IN ('project', 'cost_center', 'budget_item', 'custom')),
    CONSTRAINT org_links_link_type_check CHECK (link_type IN ('owns', 'uses', 'reports_to', 'custom')),
    CONSTRAINT org_links_org_node_fk FOREIGN KEY (tenant_id, org_node_id) REFERENCES org_nodes (tenant_id, id) ON DELETE RESTRICT
);

ALTER TABLE org_links
    ADD CONSTRAINT org_links_no_overlap
    EXCLUDE USING gist (tenant_id gist_uuid_ops WITH =, org_node_id gist_uuid_ops WITH =, object_type gist_text_ops WITH =, object_key gist_text_ops WITH =, link_type gist_text_ops WITH =, tstzrange(effective_date, end_date, '[)') WITH &&);

CREATE INDEX org_links_tenant_node_effective_idx ON org_links (tenant_id, org_node_id, effective_date);

CREATE INDEX org_links_tenant_object_effective_idx ON org_links (tenant_id, object_type, object_key, effective_date);

-- EOF
