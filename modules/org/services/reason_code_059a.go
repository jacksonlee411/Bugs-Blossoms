package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type ReasonCodeInfo struct {
	Mode            string
	OriginalMissing bool
	Filled          bool
}

func logReasonCodeRejected(ctx context.Context, tenantID uuid.UUID, requestID, operation string, info ReasonCodeInfo, svcErr *ServiceError) {
	if svcErr == nil {
		return
	}
	logWithFields(ctx, logrus.WarnLevel, "org.reason_code.rejected", logrus.Fields{
		"tenant_id":                    tenantID.String(),
		"request_id":                   requestID,
		"operation":                    operation,
		"error_code":                   svcErr.Code,
		"reason_code_mode":             info.Mode,
		"reason_code_original_missing": info.OriginalMissing,
		"reason_code_filled":           info.Filled,
	})
}

func addReasonCodeMeta(meta map[string]any, info ReasonCodeInfo) {
	meta["reason_code_mode"] = info.Mode
	meta["reason_code_original_missing"] = info.OriginalMissing
	meta["reason_code_filled"] = info.Filled
}

func normalizeReasonCode(settings OrgSettings, input string) (string, ReasonCodeInfo, *ServiceError) {
	mode := normalizeValidationMode(settings.ReasonCodeMode)
	raw := strings.TrimSpace(input)
	missing := raw == ""

	info := ReasonCodeInfo{
		Mode:            mode,
		OriginalMissing: missing,
		Filled:          false,
	}

	switch mode {
	case "enforce":
		if missing {
			return "", info, newServiceError(400, "ORG_INVALID_BODY", "reason_code is required", nil)
		}
		return raw, info, nil
	case "shadow":
		if missing {
			info.Filled = true
			return "legacy", info, nil
		}
		return raw, info, nil
	case "disabled":
		if missing {
			return "", info, nil
		}
		return raw, info, nil
	default:
		if missing {
			info.Filled = true
			return "legacy", info, nil
		}
		return raw, info, nil
	}
}
