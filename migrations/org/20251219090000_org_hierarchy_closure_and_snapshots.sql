-- +goose Up
-- DEV-PLAN-029: org_hierarchy_closure + org_hierarchy_snapshots (deep read derived tables)

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

-- +goose Down
DROP TABLE IF EXISTS org_hierarchy_snapshots;

DROP TABLE IF EXISTS org_hierarchy_snapshot_builds;

DROP TABLE IF EXISTS org_hierarchy_closure;

DROP TABLE IF EXISTS org_hierarchy_closure_builds;

