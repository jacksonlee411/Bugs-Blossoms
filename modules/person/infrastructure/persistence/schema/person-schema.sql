CREATE TABLE persons (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    person_uuid uuid NOT NULL DEFAULT gen_random_uuid (),
    pernr text NOT NULL,
    display_name text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (person_uuid),
    CONSTRAINT persons_tenant_id_person_uuid_key UNIQUE (tenant_id, person_uuid),
    CONSTRAINT persons_tenant_id_pernr_key UNIQUE (tenant_id, pernr),
    CONSTRAINT persons_pernr_trim_check CHECK (pernr = btrim(pernr) AND pernr <> ''),
    CONSTRAINT persons_status_check CHECK (status IN ('active', 'inactive'))
);

CREATE INDEX persons_tenant_pernr_idx ON persons (tenant_id, pernr);

CREATE INDEX persons_tenant_display_name_idx ON persons (tenant_id, display_name);

