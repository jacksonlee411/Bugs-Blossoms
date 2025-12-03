-- +migrate Up
CREATE TABLE IF NOT EXISTS policy_change_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status VARCHAR(32) NOT NULL,
    requester_id UUID NOT NULL,
    approver_id UUID,
    tenant_id UUID NOT NULL,
    subject TEXT NOT NULL,
    domain TEXT NOT NULL,
    action TEXT NOT NULL,
    object TEXT NOT NULL,
    reason TEXT,
    diff JSONB NOT NULL,
    base_policy_revision TEXT NOT NULL,
    applied_policy_revision TEXT,
    applied_policy_snapshot JSONB,
    pr_link TEXT,
    bot_job_id TEXT,
    bot_lock TEXT,
    bot_locked_at TIMESTAMPTZ,
    bot_attempts INT DEFAULT 0 NOT NULL,
    error_log TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_policy_change_requests_status_updated_at
    ON policy_change_requests (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_policy_change_requests_tenant_status
    ON policy_change_requests (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_policy_change_requests_bot_lock
    ON policy_change_requests (bot_lock);

-- +migrate Down
DROP TABLE IF EXISTS policy_change_requests;
