package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *OrgService) validateJobProfileAndLevel(ctx context.Context, tenantID uuid.UUID, asOf time.Time, jobProfileID uuid.UUID, jobLevelCode *string) error {
	if jobProfileID == uuid.Nil {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_profile_id is required", nil)
	}
	if asOf.IsZero() {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)

	profile, err := s.repo.GetJobProfileRef(ctx, tenantID, jobProfileID, asOf)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_NOT_FOUND", "job_profile_id not found", nil)
		}
		return mapPgError(err)
	}
	if !profile.IsActive {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_INACTIVE", "job profile is inactive", nil)
	}

	families, err := s.repo.ListJobProfileJobFamilies(ctx, tenantID, jobProfileID, asOf)
	if err != nil {
		return mapPgError(err)
	}
	if len(families) == 0 {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_ALLOCATION_MISSING", "job profile must have job family allocations", nil)
	}

	if jobLevelCode == nil {
		return nil
	}
	code := strings.TrimSpace(*jobLevelCode)
	if code == "" {
		return nil
	}

	level, err := s.repo.GetJobLevelByCode(ctx, tenantID, code, asOf)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_LEVEL_NOT_FOUND", "job_level_code not found", nil)
		}
		return mapPgError(err)
	}
	if !level.IsActive {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_LEVEL_INACTIVE", "job level is inactive", nil)
	}
	return nil
}

func normalizeValidationMode(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "disabled", "shadow", "enforce":
		return v
	default:
		return "shadow"
	}
}

func isPositionCatalogShadowableError(err *ServiceError) bool {
	if err == nil {
		return false
	}
	switch err.Code {
	case "ORG_JOB_PROFILE_INACTIVE", "ORG_JOB_LEVEL_NOT_FOUND", "ORG_JOB_LEVEL_INACTIVE":
		return true
	default:
		return false
	}
}
