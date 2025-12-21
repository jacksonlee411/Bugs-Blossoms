package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func maybeLogFrozenWindowRejected(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, changeType string, entityType string, entityID uuid.UUID, effectiveDate time.Time, freeze FreezeCheckResult, err error, extra logrus.Fields) {
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) || svcErr.Code != "ORG_FROZEN_WINDOW" {
		return
	}

	fields := logrus.Fields{
		"tenant_id":         tenantID.String(),
		"request_id":        requestID,
		"change_type":       changeType,
		"entity_type":       entityType,
		"effective_date":    effectiveDate.UTC().Format(time.RFC3339),
		"mode":              freeze.Mode,
		"freeze_cutoff_utc": freeze.CutoffUTC.UTC().Format(time.RFC3339),
		"freeze_violation":  freeze.Violation,
		"error_code":        svcErr.Code,
	}
	if initiatorID != uuid.Nil {
		fields["initiator_id"] = initiatorID.String()
	}
	if entityID != uuid.Nil {
		fields["entity_id"] = entityID.String()
	}
	for k, v := range extra {
		fields[k] = v
	}

	logWithFields(ctx, logrus.WarnLevel, "org.frozen_window.rejected", fields)
}

func maybeLogModeRejected(ctx context.Context, msg string, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, changeType string, entityType string, entityID uuid.UUID, effectiveDate time.Time, mode string, err error, extra logrus.Fields) {
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		return
	}

	fields := logrus.Fields{
		"tenant_id":      tenantID.String(),
		"request_id":     requestID,
		"change_type":    changeType,
		"entity_type":    entityType,
		"effective_date": effectiveDate.UTC().Format(time.RFC3339),
		"mode":           mode,
		"error_code":     svcErr.Code,
	}
	if initiatorID != uuid.Nil {
		fields["initiator_id"] = initiatorID.String()
	}
	if entityID != uuid.Nil {
		fields["entity_id"] = entityID.String()
	}
	for k, v := range extra {
		fields[k] = v
	}

	logWithFields(ctx, logrus.WarnLevel, msg, fields)
}
