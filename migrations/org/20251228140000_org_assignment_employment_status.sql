-- +goose Up
-- DEV-PLAN-066: Add employment_status slices to keep assignment timelines gap-free.

ALTER TABLE org_assignments
    ADD COLUMN IF NOT EXISTS employment_status text NOT NULL DEFAULT 'active';

ALTER TABLE org_assignments
    ADD CONSTRAINT org_assignments_employment_status_check
    CHECK (employment_status IN ('active', 'inactive'));

-- +goose Down
ALTER TABLE org_assignments
    DROP CONSTRAINT IF EXISTS org_assignments_employment_status_check;

ALTER TABLE org_assignments
    DROP COLUMN IF EXISTS employment_status;

