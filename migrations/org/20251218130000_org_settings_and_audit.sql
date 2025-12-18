-- +goose Up
-- DEV-PLAN-025: org_settings + org_audit_logs

CREATE TABLE IF NOT EXISTS org_settings (
    tenant_id uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    freeze_mode text NOT NULL DEFAULT 'enforce',
    freeze_grace_days int NOT NULL DEFAULT 3,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_settings_freeze_mode_check CHECK (freeze_mode IN ('disabled', 'shadow', 'enforce')),
    CONSTRAINT org_settings_freeze_grace_days_check CHECK (freeze_grace_days >= 0 AND freeze_grace_days <= 31)
);

CREATE TABLE IF NOT EXISTS org_audit_logs (
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

CREATE INDEX IF NOT EXISTS org_audit_logs_tenant_transaction_time_desc_idx ON org_audit_logs (tenant_id, transaction_time DESC);

CREATE INDEX IF NOT EXISTS org_audit_logs_tenant_entity_transaction_time_desc_idx ON org_audit_logs (tenant_id, entity_type, entity_id, transaction_time DESC);

CREATE INDEX IF NOT EXISTS org_audit_logs_tenant_request_id_idx ON org_audit_logs (tenant_id, request_id);

-- +goose Down
DROP TABLE IF EXISTS org_audit_logs;

DROP TABLE IF EXISTS org_settings;

