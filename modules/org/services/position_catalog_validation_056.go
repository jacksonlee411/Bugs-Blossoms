package services

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *OrgService) validatePositionCatalogAndProfile(ctx context.Context, tenantID uuid.UUID, codes JobCatalogCodes, jobProfileID *uuid.UUID) (JobCatalogResolvedPath, error) {
	path, err := s.repo.ResolveJobCatalogPathByCodes(ctx, tenantID, codes)
	if err != nil {
		switch {
		case errors.Is(err, ErrJobCatalogInactiveOrMissing):
			return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_CATALOG_INACTIVE_OR_MISSING", "job catalog is inactive or missing", nil)
		case errors.Is(err, ErrJobCatalogInvalidHierarchy):
			return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_CATALOG_INVALID_HIERARCHY", "job catalog hierarchy is invalid", nil)
		default:
			return JobCatalogResolvedPath{}, mapPgError(err)
		}
	}

	if jobProfileID == nil || *jobProfileID == uuid.Nil {
		return path, nil
	}

	profile, err := s.repo.GetJobProfileRef(ctx, tenantID, *jobProfileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_NOT_FOUND", "job_profile_id not found", nil)
		}
		return JobCatalogResolvedPath{}, mapPgError(err)
	}
	if !profile.IsActive {
		return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_INACTIVE", "job profile is inactive", nil)
	}
	if profile.JobRoleID != path.JobRoleID {
		return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_CONFLICT", "job profile conflicts with job catalog", nil)
	}
	allowed, _, err := s.repo.JobProfileAllowsLevel(ctx, tenantID, *jobProfileID, path.JobLevelID)
	if err != nil {
		return JobCatalogResolvedPath{}, mapPgError(err)
	}
	if !allowed {
		return JobCatalogResolvedPath{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_CONFLICT", "job profile conflicts with job catalog", nil)
	}
	return path, nil
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
