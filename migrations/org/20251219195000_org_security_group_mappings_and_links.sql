-- +goose Up

-- DEV-PLAN-032: Org security group mappings + business object links.

CREATE TABLE IF NOT EXISTS org_security_group_mappings (
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

CREATE TABLE IF NOT EXISTS org_links (
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

-- +goose Down

DROP TABLE IF EXISTS org_links;
DROP TABLE IF EXISTS org_security_group_mappings;

