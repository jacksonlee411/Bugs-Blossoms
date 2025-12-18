-- +goose Up
-- DEV-PLAN-026: org_outbox

CREATE TABLE IF NOT EXISTS org_outbox (
    id uuid NOT NULL DEFAULT gen_random_uuid (),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    topic text NOT NULL,
    payload jsonb NOT NULL,
    event_id uuid NOT NULL,
    sequence bigserial NOT NULL,
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

CREATE INDEX IF NOT EXISTS org_outbox_pending_by_available
    ON org_outbox (available_at, sequence)
    WHERE published_at IS NULL;

CREATE INDEX IF NOT EXISTS org_outbox_published_by_time
    ON org_outbox (published_at, sequence)
    WHERE published_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS org_outbox_tenant_published
    ON org_outbox (tenant_id, published_at, sequence);

-- +goose Down
DROP TABLE IF EXISTS org_outbox;

