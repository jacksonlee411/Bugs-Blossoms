-- +goose Up
CREATE TABLE IF NOT EXISTS org_personnel_events (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id text NOT NULL,
    initiator_id uuid NOT NULL,
    event_type text NOT NULL,
    person_uuid uuid NOT NULL,
    pernr text NOT NULL,
    effective_date timestamptz NOT NULL,
    reason_code text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_personnel_events_request_not_blank CHECK (btrim(request_id) <> ''),
    CONSTRAINT org_personnel_events_event_type_check CHECK (event_type IN ('hire', 'transfer', 'termination')),
    CONSTRAINT org_personnel_events_pernr_not_blank CHECK (btrim(pernr) <> ''),
    CONSTRAINT org_personnel_events_reason_code_not_blank CHECK (btrim(reason_code) <> ''),
    CONSTRAINT org_personnel_events_payload_is_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT org_personnel_events_tenant_request_uq UNIQUE (tenant_id, request_id)
);

CREATE INDEX IF NOT EXISTS org_personnel_events_tenant_person_effective_idx
    ON org_personnel_events (tenant_id, person_uuid, effective_date DESC);

-- +goose Down
DROP TABLE org_personnel_events;

