package services

import (
	"fmt"
	"net/http"
	"time"
)

type FreezeCheckResult struct {
	Mode          string
	GraceDays     int
	CutoffUTC     time.Time
	Violation     bool
	TransactionAt time.Time
}

func computeFreezeCutoffUTC(txTime time.Time, graceDays int) time.Time {
	txTime = txTime.UTC()
	monthStart := time.Date(txTime.Year(), txTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	graceEnd := monthStart.Add(time.Duration(graceDays) * 24 * time.Hour)
	if txTime.Before(graceEnd) {
		prevMonth := monthStart.AddDate(0, -1, 0)
		return prevMonth
	}
	return monthStart
}

func (s *OrgService) freezeCheck(settings OrgSettings, txTime time.Time, affectedAt time.Time) (FreezeCheckResult, error) {
	txTime = txTime.UTC()
	affectedAt = affectedAt.UTC()

	mode := settings.FreezeMode
	if mode == "" {
		mode = "enforce"
	}
	graceDays := settings.FreezeGraceDays
	if graceDays < 0 {
		graceDays = 3
	}

	cutoff := computeFreezeCutoffUTC(txTime, graceDays)
	violation := affectedAt.Before(cutoff)

	switch mode {
	case "disabled":
		return FreezeCheckResult{Mode: mode, GraceDays: graceDays, CutoffUTC: cutoff, Violation: violation, TransactionAt: txTime}, nil
	case "shadow":
		return FreezeCheckResult{Mode: mode, GraceDays: graceDays, CutoffUTC: cutoff, Violation: violation, TransactionAt: txTime}, nil
	case "enforce":
		if violation {
			return FreezeCheckResult{Mode: mode, GraceDays: graceDays, CutoffUTC: cutoff, Violation: true, TransactionAt: txTime},
				newServiceError(http.StatusConflict, "ORG_FROZEN_WINDOW", fmt.Sprintf("affected_at=%s is before cutoff=%s", affectedAt.Format(time.RFC3339), cutoff.Format(time.RFC3339)), nil)
		}
		return FreezeCheckResult{Mode: mode, GraceDays: graceDays, CutoffUTC: cutoff, Violation: false, TransactionAt: txTime}, nil
	default:
		return FreezeCheckResult{}, fmt.Errorf("invalid freeze mode: %s", mode)
	}
}
