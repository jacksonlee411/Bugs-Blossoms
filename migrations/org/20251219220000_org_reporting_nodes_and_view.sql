-- +goose Up
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
SELECT r.*
FROM org_reporting_nodes r
    JOIN org_hierarchy_snapshot_builds b ON b.tenant_id = r.tenant_id
    AND b.hierarchy_type = r.hierarchy_type
    AND b.as_of_date = r.as_of_date
    AND b.build_id = r.build_id
WHERE
    b.is_active = TRUE
    AND b.status = 'ready';

-- +goose Down
DROP VIEW IF EXISTS org_reporting;
DROP TABLE IF EXISTS org_reporting_nodes;

