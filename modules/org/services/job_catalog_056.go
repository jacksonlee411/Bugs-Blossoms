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

type JobLevelRow struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
}

type JobLevelCreate struct {
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
	IsActive    bool      `json:"is_active"`
}

type JobProfileCreate struct {
	Code        string
	Name        string
	Description *string
	IsActive    bool
	JobFamilies JobProfileJobFamiliesSet
}

type JobProfileUpdate struct {
	Name        *string
	Description **string
	IsActive    *bool
	JobFamilies *JobProfileJobFamiliesSet
}

type JobProfileRef struct {
	ID       uuid.UUID
	IsActive bool
}

type JobProfileJobFamilyRow struct {
	JobFamilyID        uuid.UUID `json:"job_family_id"`
	JobFamilyCode      string    `json:"job_family_code"`
	JobFamilyName      string    `json:"job_family_name"`
	IsPrimary          bool      `json:"is_primary"`
	JobFamilyGroupID   uuid.UUID `json:"job_family_group_id"`
	JobFamilyGroupCode string    `json:"job_family_group_code"`
	JobFamilyGroupName string    `json:"job_family_group_name"`
}

type JobProfileJobFamiliesSet struct {
	Items []JobProfileJobFamilySetItem
}

type JobProfileJobFamilySetItem struct {
	JobFamilyID uuid.UUID
	IsPrimary   bool
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

func (s *OrgService) ListJobLevels(ctx context.Context, tenantID uuid.UUID) ([]JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobLevelRow, error) {
		rows, err := s.repo.ListJobLevels(txCtx, tenantID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobLevel(ctx context.Context, tenantID uuid.UUID, in JobLevelCreate) (JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
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

func (s *OrgService) ListJobProfiles(ctx context.Context, tenantID uuid.UUID) ([]JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileRow, error) {
		rows, err := s.repo.ListJobProfiles(txCtx, tenantID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobProfile(ctx context.Context, tenantID uuid.UUID, in JobProfileCreate) (JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if err := validateJobProfileJobFamiliesSet(in.JobFamilies); err != nil {
		return JobProfileRow{}, err
	}
	in.Code = code
	in.Name = name
	in.Description = trimOptionalText(in.Description)
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobProfileRow, error) {
		row, err := s.repo.CreateJobProfile(txCtx, tenantID, in)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		if err := s.repo.SetJobProfileJobFamilies(txCtx, tenantID, row.ID, in.JobFamilies); err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		return row, nil
	})
}

func (s *OrgService) UpdateJobProfile(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobProfileUpdate) (JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if id == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "id is required", nil)
	}
	if in.Name == nil && in.Description == nil && in.IsActive == nil && in.JobFamilies == nil {
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
	if in.JobFamilies != nil {
		if err := validateJobProfileJobFamiliesSet(*in.JobFamilies); err != nil {
			return JobProfileRow{}, err
		}
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobProfileRow, error) {
		row, err := s.repo.UpdateJobProfile(txCtx, tenantID, id, in)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		if in.JobFamilies != nil {
			if err := s.repo.SetJobProfileJobFamilies(txCtx, tenantID, id, *in.JobFamilies); err != nil {
				return JobProfileRow{}, mapPgError(err)
			}
		}
		return row, nil
	})
}

func (s *OrgService) ListJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) ([]JobProfileJobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobProfileID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_profile_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileJobFamilyRow, error) {
		rows, err := s.repo.ListJobProfileJobFamilies(txCtx, tenantID, jobProfileID)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) SetJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, in JobProfileJobFamiliesSet) error {
	if tenantID == uuid.Nil {
		return newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobProfileID == uuid.Nil {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_profile_id is required", nil)
	}
	if err := validateJobProfileJobFamiliesSet(in); err != nil {
		return err
	}

	_, err := inTx(ctx, tenantID, func(txCtx context.Context) (struct{}, error) {
		if err := s.repo.SetJobProfileJobFamilies(txCtx, tenantID, jobProfileID, in); err != nil {
			return struct{}{}, mapPgError(err)
		}
		return struct{}{}, nil
	})
	return err
}

func validateJobProfileJobFamiliesSet(in JobProfileJobFamiliesSet) error {
	if len(in.Items) == 0 {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_families are required", nil)
	}

	primaryCount := 0
	seen := make(map[uuid.UUID]struct{}, len(in.Items))
	for i := range in.Items {
		it := in.Items[i]
		if it.JobFamilyID == uuid.Nil {
			return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_id is required", nil)
		}
		if _, ok := seen[it.JobFamilyID]; ok {
			return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_id duplicated", nil)
		}
		seen[it.JobFamilyID] = struct{}{}

		if it.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		return newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "exactly one primary is required", nil)
	}
	return nil
}
