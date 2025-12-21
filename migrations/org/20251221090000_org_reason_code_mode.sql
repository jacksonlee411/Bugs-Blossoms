-- +goose Up
-- DEV-PLAN-059A: org_settings.reason_code_mode (disabled/shadow/enforce)

ALTER TABLE org_settings
    ADD COLUMN IF NOT EXISTS reason_code_mode text NOT NULL DEFAULT 'shadow';

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'org_settings_reason_code_mode_check'
    ) THEN
        ALTER TABLE org_settings
            ADD CONSTRAINT org_settings_reason_code_mode_check
            CHECK (reason_code_mode IN ('disabled', 'shadow', 'enforce'));
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE org_settings
    DROP CONSTRAINT IF EXISTS org_settings_reason_code_mode_check;

ALTER TABLE org_settings
    DROP COLUMN IF EXISTS reason_code_mode;

