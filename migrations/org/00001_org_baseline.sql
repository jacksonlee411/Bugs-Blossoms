-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS org_nodes (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type text NOT NULL DEFAULT 'OrgUnit',
    code varchar(64) NOT NULL,
    is_root boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_nodes_type_check CHECK (type IN ('OrgUnit')),
    CONSTRAINT org_nodes_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT org_nodes_tenant_id_code_key UNIQUE (tenant_id, code)
);

CREATE UNIQUE INDEX IF NOT EXISTS org_nodes_tenant_root_unique ON org_nodes (tenant_id) WHERE is_root;
CREATE INDEX IF NOT EXISTS org_nodes_tenant_code_idx ON org_nodes (tenant_id, code);

CREATE TABLE IF NOT EXISTS org_node_slices (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_node_id uuid NOT NULL,
    name varchar(255) NOT NULL,
    i18n_names jsonb NOT NULL DEFAULT '{}'::jsonb,
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
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        org_node_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

ALTER TABLE org_node_slices
    ADD CONSTRAINT org_node_slices_sibling_name_unique
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        parent_hint gist_uuid_ops WITH =,
        lower(name) gist_text_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

CREATE INDEX IF NOT EXISTS org_node_slices_tenant_node_effective_idx ON org_node_slices (tenant_id, org_node_id, effective_date);
CREATE INDEX IF NOT EXISTS org_node_slices_tenant_parent_effective_idx ON org_node_slices (tenant_id, parent_hint, effective_date);

CREATE TABLE IF NOT EXISTS org_edges (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
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
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        child_node_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

CREATE INDEX IF NOT EXISTS org_edges_tenant_path_gist_idx ON org_edges USING gist (tenant_id gist_uuid_ops, path);
CREATE INDEX IF NOT EXISTS org_edges_tenant_parent_effective_idx ON org_edges (tenant_id, parent_node_id, effective_date);
CREATE INDEX IF NOT EXISTS org_edges_tenant_child_effective_idx ON org_edges (tenant_id, child_node_id, effective_date);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_edges_set_path_depth_and_prevent_cycle()
RETURNS trigger
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
        NEW.depth := nlevel(NEW.path) - 1;
        RETURN NEW;
    END IF;

    SELECT e.path
    INTO parent_path
    FROM org_edges e
    WHERE e.tenant_id = NEW.tenant_id
      AND e.child_node_id = NEW.parent_node_id
      AND e.effective_date <= NEW.effective_date
      AND e.end_date > NEW.effective_date
    ORDER BY e.effective_date DESC
    LIMIT 1;

    IF parent_path IS NULL THEN
        RAISE EXCEPTION 'org_edges: parent path not found (tenant_id=%, parent_node_id=%, as_of=%)',
            NEW.tenant_id, NEW.parent_node_id, NEW.effective_date
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    SELECT e.path
    INTO child_path
    FROM org_edges e
    WHERE e.tenant_id = NEW.tenant_id
      AND e.child_node_id = NEW.child_node_id
      AND e.effective_date <= NEW.effective_date
      AND e.end_date > NEW.effective_date
    ORDER BY e.effective_date DESC
    LIMIT 1;

    IF child_path IS NOT NULL AND parent_path <@ child_path THEN
        RAISE EXCEPTION 'org_edges: cycle detected (parent_node_id=% inside child_node_id=% subtree)',
            NEW.parent_node_id, NEW.child_node_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    NEW.path := parent_path || child_key::ltree;
    NEW.depth := nlevel(NEW.path) - 1;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_edges_prevent_key_updates()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.hierarchy_type IS DISTINCT FROM OLD.hierarchy_type
        OR NEW.parent_node_id IS DISTINCT FROM OLD.parent_node_id
        OR NEW.child_node_id IS DISTINCT FROM OLD.child_node_id
        OR NEW.effective_date IS DISTINCT FROM OLD.effective_date
    THEN
        RAISE EXCEPTION 'org_edges: updating hierarchy keys is not allowed; use new edge slice + retire old slice'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_edges_before_insert_set_path_depth ON org_edges;
CREATE TRIGGER org_edges_before_insert_set_path_depth
    BEFORE INSERT ON org_edges
    FOR EACH ROW
    EXECUTE FUNCTION org_edges_set_path_depth_and_prevent_cycle();

DROP TRIGGER IF EXISTS org_edges_before_update_prevent_key_updates ON org_edges;
CREATE TRIGGER org_edges_before_update_prevent_key_updates
    BEFORE UPDATE ON org_edges
    FOR EACH ROW
    EXECUTE FUNCTION org_edges_prevent_key_updates();

CREATE TABLE IF NOT EXISTS org_positions (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_node_id uuid NOT NULL,
    code varchar(64) NOT NULL,
    title text NULL,
    status text NOT NULL DEFAULT 'active',
    is_auto_created boolean NOT NULL DEFAULT false,
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
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        code WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

CREATE INDEX IF NOT EXISTS org_positions_tenant_node_effective_idx ON org_positions (tenant_id, org_node_id, effective_date);
CREATE INDEX IF NOT EXISTS org_positions_tenant_code_effective_idx ON org_positions (tenant_id, code, effective_date);

CREATE TABLE IF NOT EXISTS org_assignments (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    position_id uuid NOT NULL,
    subject_type text NOT NULL DEFAULT 'person',
    subject_id uuid NOT NULL,
    pernr text NOT NULL,
    assignment_type text NOT NULL DEFAULT 'primary',
    is_primary boolean NOT NULL DEFAULT true,
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
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        subject_type gist_text_ops WITH =,
        subject_id gist_uuid_ops WITH =,
        assignment_type gist_text_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    )
    WHERE (assignment_type = 'primary');

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_position_unique_in_time
    EXCLUDE USING gist (
        tenant_id gist_uuid_ops WITH =,
        position_id gist_uuid_ops WITH =,
        tstzrange(effective_date, end_date, '[)') WITH &&
    );

CREATE INDEX IF NOT EXISTS org_assignments_tenant_subject_effective_idx ON org_assignments (tenant_id, subject_id, effective_date);
CREATE INDEX IF NOT EXISTS org_assignments_tenant_position_effective_idx ON org_assignments (tenant_id, position_id, effective_date);
CREATE INDEX IF NOT EXISTS org_assignments_tenant_pernr_effective_idx ON org_assignments (tenant_id, pernr, effective_date);

-- +goose Down
DROP TABLE IF EXISTS org_assignments;
DROP TABLE IF EXISTS org_positions;
DROP TRIGGER IF EXISTS org_edges_before_update_prevent_key_updates ON org_edges;
DROP TRIGGER IF EXISTS org_edges_before_insert_set_path_depth ON org_edges;
DROP TABLE IF EXISTS org_edges;
DROP FUNCTION IF EXISTS org_edges_prevent_key_updates;
DROP FUNCTION IF EXISTS org_edges_set_path_depth_and_prevent_cycle;
DROP TABLE IF EXISTS org_node_slices;
DROP TABLE IF EXISTS org_nodes;
