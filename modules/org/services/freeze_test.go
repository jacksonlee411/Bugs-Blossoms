package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeFreezeCutoffUTC_WithinGrace_AllowsPreviousMonth(t *testing.T) {
	txTime := time.Date(2025, 12, 2, 12, 0, 0, 0, time.UTC)
	cutoff := computeFreezeCutoffUTC(txTime, 3)
	require.Equal(t, time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC), cutoff)
}

func TestComputeFreezeCutoffUTC_AfterGrace_AllowsCurrentMonthOnly(t *testing.T) {
	txTime := time.Date(2025, 12, 10, 12, 0, 0, 0, time.UTC)
	cutoff := computeFreezeCutoffUTC(txTime, 3)
	require.Equal(t, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), cutoff)
}

func TestFreezeCheck_EnforceRejectsBeforeCutoff(t *testing.T) {
	svc := &OrgService{}
	settings := OrgSettings{FreezeMode: "enforce", FreezeGraceDays: 3}
	txTime := time.Date(2025, 12, 10, 12, 0, 0, 0, time.UTC)
	affectedAt := time.Date(2025, 11, 30, 0, 0, 0, 0, time.UTC)

	_, err := svc.freezeCheck(settings, txTime, affectedAt)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 409, svcErr.Status)
	require.Equal(t, "ORG_FROZEN_WINDOW", svcErr.Code)
}
