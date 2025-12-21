package services

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrJobCatalogInactiveOrMissing = errors.New("job catalog inactive or missing")
	ErrJobCatalogInvalidHierarchy  = errors.New("job catalog invalid hierarchy")
)

type JobFamilyGroupRow struct {
	ID       uuid.UUID `json:"id"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	IsActive bool      `json:"is_active"`
}

type JobFamilyGroupCreate struct {
	Code     string
	Name     string
	IsActive bool
}

type JobFamilyGroupUpdate struct {
	Name     *string
	IsActive *bool
}

type JobFamilyRow struct {
	ID               uuid.UUID `json:"id"`
	JobFamilyGroupID uuid.UUID `json:"job_family_group_id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	IsActive         bool      `json:"is_active"`
}

type JobFamilyCreate struct {
	JobFamilyGroupID uuid.UUID
	Code             string
	Name             string
	IsActive         bool
}

type JobFamilyUpdate struct {
	Name     *string
	IsActive *bool
}

type JobRoleRow struct {
	ID          uuid.UUID `json:"id"`
	JobFamilyID uuid.UUID `json:"job_family_id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	IsActive    bool      `json:"is_active"`
}

type JobRoleCreate struct {
	JobFamilyID uuid.UUID
	Code        string
	Name        string
	IsActive    bool
}

type JobRoleUpdate struct {
	Name     *string
	IsActive *bool
}

type JobLevelRow struct {
	ID           uuid.UUID `json:"id"`
	JobRoleID    uuid.UUID `json:"job_role_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
}

type JobLevelCreate struct {
	JobRoleID    uuid.UUID
	Code         string
	Name         string
	DisplayOrder int
	IsActive     bool
}

type JobLevelUpdate struct {
	Name         *string
	DisplayOrder *int
	IsActive     *bool
}

type JobProfileRow struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	JobRoleID   uuid.UUID `json:"job_role_id"`
	IsActive    bool      `json:"is_active"`
}

type JobProfileCreate struct {
	Code        string
	Name        string
	Description *string
	JobRoleID   uuid.UUID
	IsActive    bool
}

type JobProfileUpdate struct {
	Name        *string
	Description **string
	IsActive    *bool
}

type JobProfileAllowedLevelsSet struct {
	JobLevelIDs []uuid.UUID
}

type JobProfileRef struct {
	ID        uuid.UUID
	JobRoleID uuid.UUID
	IsActive  bool
}

type JobCatalogCodes struct {
	JobFamilyGroupCode string
	JobFamilyCode      string
	JobRoleCode        string
	JobLevelCode       string
}

type JobCatalogResolvedPath struct {
	JobFamilyGroupID uuid.UUID
	JobFamilyID      uuid.UUID
	JobRoleID        uuid.UUID
	JobLevelID       uuid.UUID
}

func (s *OrgService) ListJobFamilyGroups(ctx context.Context, tenantID uuid.UUID) ([]JobFamilyGroupRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobFamilyGroupRow, error) {
		return s.repo.ListJobFamilyGroups(txCtx, tenantID)
	})
}

func (s *OrgService) CreateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, in JobFamilyGroupCreate) (JobFamilyGroupRow, error) {
	if tenantID == uuid.Nil {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyGroupRow, error) {
		row, err := s.repo.CreateJobFamilyGroup(txCtx, tenantID, JobFamilyGroupCreate{
			Code:     code,
			Name:     name,
			IsActive: in.IsActive,
		})
		return row, mapPgError(err)
	})
}

func (s *OrgService) UpdateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobFamilyGroupUpdate) (JobFamilyGroupRow, error) {
	if tenantID == uuid.Nil {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.IsActive == nil {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "no fields to update", nil)
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyGroupRow, error) {
		row, err := s.repo.UpdateJobFamilyGroup(txCtx, tenantID, id, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) ListJobFamilies(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupID uuid.UUID) ([]JobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobFamilyGroupID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_family_group_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobFamilyRow, error) {
		rows, err := s.repo.ListJobFamilies(txCtx, tenantID, jobFamilyGroupID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobFamily(ctx context.Context, tenantID uuid.UUID, in JobFamilyCreate) (JobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if in.JobFamilyGroupID == uuid.Nil {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_group_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	in.Code = code
	in.Name = name
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyRow, error) {
		row, err := s.repo.CreateJobFamily(txCtx, tenantID, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) UpdateJobFamily(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobFamilyUpdate) (JobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.IsActive == nil {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "no fields to update", nil)
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyRow, error) {
		row, err := s.repo.UpdateJobFamily(txCtx, tenantID, id, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) ListJobRoles(ctx context.Context, tenantID uuid.UUID, jobFamilyID uuid.UUID) ([]JobRoleRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobFamilyID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_family_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobRoleRow, error) {
		rows, err := s.repo.ListJobRoles(txCtx, tenantID, jobFamilyID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobRole(ctx context.Context, tenantID uuid.UUID, in JobRoleCreate) (JobRoleRow, error) {
	if tenantID == uuid.Nil {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if in.JobFamilyID == uuid.Nil {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	in.Code = code
	in.Name = name
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobRoleRow, error) {
		row, err := s.repo.CreateJobRole(txCtx, tenantID, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) UpdateJobRole(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobRoleUpdate) (JobRoleRow, error) {
	if tenantID == uuid.Nil {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.IsActive == nil {
		return JobRoleRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "no fields to update", nil)
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobRoleRow, error) {
		row, err := s.repo.UpdateJobRole(txCtx, tenantID, id, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) ListJobLevels(ctx context.Context, tenantID uuid.UUID, jobRoleID uuid.UUID) ([]JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobRoleID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_role_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobLevelRow, error) {
		rows, err := s.repo.ListJobLevels(txCtx, tenantID, jobRoleID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobLevel(ctx context.Context, tenantID uuid.UUID, in JobLevelCreate) (JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if in.JobRoleID == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_role_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if in.DisplayOrder < 0 {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "display_order must be >= 0", nil)
	}
	in.Code = code
	in.Name = name
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobLevelRow, error) {
		row, err := s.repo.CreateJobLevel(txCtx, tenantID, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) UpdateJobLevel(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobLevelUpdate) (JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.IsActive == nil && in.DisplayOrder == nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "no fields to update", nil)
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if in.DisplayOrder != nil && *in.DisplayOrder < 0 {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "display_order must be >= 0", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobLevelRow, error) {
		row, err := s.repo.UpdateJobLevel(txCtx, tenantID, id, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) ListJobProfiles(ctx context.Context, tenantID uuid.UUID, jobRoleID *uuid.UUID) ([]JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileRow, error) {
		rows, err := s.repo.ListJobProfiles(txCtx, tenantID, jobRoleID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobProfile(ctx context.Context, tenantID uuid.UUID, in JobProfileCreate) (JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if in.JobRoleID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_role_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	in.Code = code
	in.Name = name
	in.Description = trimOptionalText(in.Description)
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobProfileRow, error) {
		row, err := s.repo.CreateJobProfile(txCtx, tenantID, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) UpdateJobProfile(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobProfileUpdate) (JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.Description == nil && in.IsActive == nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "no fields to update", nil)
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if in.Description != nil && *in.Description != nil {
		trimmed := strings.TrimSpace(**in.Description)
		if trimmed == "" {
			*in.Description = nil
		} else {
			v := trimmed
			*in.Description = &v
		}
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobProfileRow, error) {
		row, err := s.repo.UpdateJobProfile(txCtx, tenantID, id, in)
		return row, mapPgError(err)
	})
}

func (s *OrgService) SetJobProfileAllowedLevels(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, in JobProfileAllowedLevelsSet) error {
	if tenantID == uuid.Nil {
		return newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobProfileID == uuid.Nil {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_profile_id is required", nil)
	}
	unique := make([]uuid.UUID, 0, len(in.JobLevelIDs))
	seen := make(map[uuid.UUID]struct{}, len(in.JobLevelIDs))
	for _, id := range in.JobLevelIDs {
		if id == uuid.Nil {
			return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_level_ids contains invalid uuid", nil)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	_, err := inTx(ctx, tenantID, func(txCtx context.Context) (struct{}, error) {
		profile, err := s.repo.GetJobProfileRef(txCtx, tenantID, jobProfileID)
		if err != nil {
			return struct{}{}, mapPgError(err)
		}
		for _, levelID := range unique {
			ok, err := s.repo.JobLevelExistsUnderRole(txCtx, tenantID, profile.JobRoleID, levelID)
			if err != nil {
				return struct{}{}, mapPgError(err)
			}
			if !ok {
				return struct{}{}, newServiceError(http.StatusUnprocessableEntity, "ORG_JOB_PROFILE_LEVEL_NOT_UNDER_ROLE", "job_level_id is not under job role", nil)
			}
		}
		if err := s.repo.SetJobProfileAllowedLevels(txCtx, tenantID, jobProfileID, JobProfileAllowedLevelsSet{JobLevelIDs: unique}); err != nil {
			return struct{}{}, mapPgError(err)
		}
		return struct{}{}, nil
	})
	return err
}
