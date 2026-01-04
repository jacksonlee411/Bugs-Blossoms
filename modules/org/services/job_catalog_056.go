package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrJobCatalogInactiveOrMissing = errors.New("job catalog inactive or missing")
	ErrJobCatalogInvalidHierarchy  = errors.New("job catalog invalid hierarchy")
)

const (
	WriteModeCorrect        = "correct"
	WriteModeUpdateFromDate = "update_from_date"
)

func normalizeWriteMode(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", WriteModeCorrect:
		return WriteModeCorrect
	case WriteModeUpdateFromDate:
		return v
	default:
		return WriteModeCorrect
	}
}

func prevValidDayUTC(t time.Time) time.Time {
	t = normalizeValidTimeDayUTC(t)
	return t.AddDate(0, 0, -1)
}

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
	// EffectiveDate is the first day of the slice and must be day-granularity UTC.
	EffectiveDate time.Time
}

type JobFamilyGroupUpdate struct {
	Name          *string
	IsActive      *bool
	EffectiveDate time.Time
	WriteMode     string
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
	EffectiveDate    time.Time
}

type JobFamilyUpdate struct {
	Name          *string
	IsActive      *bool
	EffectiveDate time.Time
	WriteMode     string
}

type JobLevelRow struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
}

type JobLevelCreate struct {
	Code          string
	Name          string
	DisplayOrder  int
	IsActive      bool
	EffectiveDate time.Time
}

type JobLevelUpdate struct {
	Name          *string
	DisplayOrder  *int
	IsActive      *bool
	EffectiveDate time.Time
	WriteMode     string
}

type JobProfileRow struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
}

type JobProfileCreate struct {
	Code          string
	Name          string
	Description   *string
	IsActive      bool
	JobFamilies   JobProfileJobFamiliesSet
	EffectiveDate time.Time
}

type JobProfileUpdate struct {
	Name          *string
	Description   **string
	IsActive      *bool
	JobFamilies   *JobProfileJobFamiliesSet
	EffectiveDate time.Time
	WriteMode     string
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

type JobProfileListItem struct {
	JobProfileRow
	JobFamilies []JobProfileJobFamilyRow `json:"job_families"`
}

type JobFamilyGroupSliceRow struct {
	ID               uuid.UUID
	JobFamilyGroupID uuid.UUID
	Name             string
	IsActive         bool
	EffectiveDate    time.Time
	EndDate          time.Time
}

type JobFamilyGroupSliceInPlacePatch struct {
	Name     *string
	IsActive *bool
}

type JobFamilySliceRow struct {
	ID            uuid.UUID
	JobFamilyID   uuid.UUID
	Name          string
	IsActive      bool
	EffectiveDate time.Time
	EndDate       time.Time
}

type JobFamilySliceInPlacePatch struct {
	Name     *string
	IsActive *bool
}

type JobLevelSliceRow struct {
	ID            uuid.UUID
	JobLevelID    uuid.UUID
	Name          string
	DisplayOrder  int
	IsActive      bool
	EffectiveDate time.Time
	EndDate       time.Time
}

type JobLevelSliceInPlacePatch struct {
	Name         *string
	DisplayOrder *int
	IsActive     *bool
}

type JobProfileSliceRow struct {
	ID            uuid.UUID
	JobProfileID  uuid.UUID
	Name          string
	Description   *string
	IsActive      bool
	ExternalRefs  []byte
	EffectiveDate time.Time
	EndDate       time.Time
}

type JobProfileSliceInPlacePatch struct {
	Name         *string
	Description  **string
	IsActive     *bool
	ExternalRefs *[]byte
}

type JobProfileSliceJobFamilySetItem struct {
	JobFamilyID uuid.UUID
	IsPrimary   bool
}

func (s *OrgService) ListJobFamilyGroups(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]JobFamilyGroupRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobFamilyGroupRow, error) {
		return s.repo.ListJobFamilyGroups(txCtx, tenantID, asOf)
	})
}

func (s *OrgService) CreateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, in JobFamilyGroupCreate) (JobFamilyGroupRow, error) {
	if tenantID == uuid.Nil {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if code == "" || name == "" {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if effectiveDate.IsZero() {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyGroupRow, error) {
		row, err := s.repo.CreateJobFamilyGroup(txCtx, tenantID, JobFamilyGroupCreate{
			Code:     code,
			Name:     name,
			IsActive: in.IsActive,
			// EffectiveDate is handled by slices.
			EffectiveDate: effectiveDate,
		})
		if err != nil {
			return JobFamilyGroupRow{}, mapPgError(err)
		}
		if _, err := s.repo.InsertJobFamilyGroupSlice(txCtx, tenantID, row.ID, name, in.IsActive, effectiveDate, endOfTime); err != nil {
			return JobFamilyGroupRow{}, mapPgError(err)
		}
		return JobFamilyGroupRow{
			ID:       row.ID,
			Code:     strings.TrimSpace(row.Code),
			Name:     name,
			IsActive: in.IsActive,
		}, nil
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
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if effectiveDate.IsZero() {
		return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	mode := normalizeWriteMode(in.WriteMode)
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyGroupRow, error) {
		current, err := s.repo.LockJobFamilyGroupSliceAt(txCtx, tenantID, id, effectiveDate)
		if err != nil {
			return JobFamilyGroupRow{}, mapPgError(err)
		}

		newName := strings.TrimSpace(current.Name)
		if in.Name != nil {
			newName = strings.TrimSpace(*in.Name)
		}
		if newName == "" {
			return JobFamilyGroupRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "name is required", nil)
		}
		newActive := current.IsActive
		if in.IsActive != nil {
			newActive = *in.IsActive
		}

		switch mode {
		case WriteModeUpdateFromDate:
			if effectiveDate.Equal(normalizeValidTimeDayUTC(current.EffectiveDate)) {
				return JobFamilyGroupRow{}, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
			}
			if err := s.repo.TruncateJobFamilyGroupSlice(txCtx, tenantID, current.ID, prevValidDayUTC(effectiveDate)); err != nil {
				return JobFamilyGroupRow{}, mapPgError(err)
			}
			if _, err := s.repo.InsertJobFamilyGroupSlice(txCtx, tenantID, id, newName, newActive, effectiveDate, normalizeValidTimeDayUTC(current.EndDate)); err != nil {
				return JobFamilyGroupRow{}, mapPgError(err)
			}
		default:
			if err := s.repo.UpdateJobFamilyGroupSliceInPlace(txCtx, tenantID, current.ID, JobFamilyGroupSliceInPlacePatch{
				Name:     in.Name,
				IsActive: in.IsActive,
			}); err != nil {
				return JobFamilyGroupRow{}, mapPgError(err)
			}
		}

		identityRow, err := s.repo.UpdateJobFamilyGroup(txCtx, tenantID, id, in)
		if err != nil {
			return JobFamilyGroupRow{}, mapPgError(err)
		}
		return JobFamilyGroupRow{
			ID:       identityRow.ID,
			Code:     strings.TrimSpace(identityRow.Code),
			Name:     newName,
			IsActive: newActive,
		}, nil
	})
}

func (s *OrgService) ListJobFamilies(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupID uuid.UUID, asOf time.Time) ([]JobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobFamilyGroupID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_family_group_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobFamilyRow, error) {
		rows, err := s.repo.ListJobFamilies(txCtx, tenantID, jobFamilyGroupID, asOf)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) ListJobFamiliesByGroupIDsAsOf(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupIDs []uuid.UUID, asOf time.Time) ([]JobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobFamilyRow, error) {
		rows, err := s.repo.ListJobFamiliesByGroupIDsAsOf(txCtx, tenantID, jobFamilyGroupIDs, asOf)
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
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if code == "" || name == "" {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if effectiveDate.IsZero() {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	in.Code = code
	in.Name = name
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyRow, error) {
		row, err := s.repo.CreateJobFamily(txCtx, tenantID, in)
		if err != nil {
			return JobFamilyRow{}, mapPgError(err)
		}
		if _, err := s.repo.InsertJobFamilySlice(txCtx, tenantID, row.ID, name, in.IsActive, effectiveDate, endOfTime); err != nil {
			return JobFamilyRow{}, mapPgError(err)
		}
		return JobFamilyRow{
			ID:               row.ID,
			JobFamilyGroupID: row.JobFamilyGroupID,
			Code:             strings.TrimSpace(row.Code),
			Name:             name,
			IsActive:         in.IsActive,
		}, nil
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
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if effectiveDate.IsZero() {
		return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	mode := normalizeWriteMode(in.WriteMode)
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobFamilyRow, error) {
		current, err := s.repo.LockJobFamilySliceAt(txCtx, tenantID, id, effectiveDate)
		if err != nil {
			return JobFamilyRow{}, mapPgError(err)
		}
		newName := strings.TrimSpace(current.Name)
		if in.Name != nil {
			newName = strings.TrimSpace(*in.Name)
		}
		if newName == "" {
			return JobFamilyRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "name is required", nil)
		}
		newActive := current.IsActive
		if in.IsActive != nil {
			newActive = *in.IsActive
		}

		switch mode {
		case WriteModeUpdateFromDate:
			if effectiveDate.Equal(normalizeValidTimeDayUTC(current.EffectiveDate)) {
				return JobFamilyRow{}, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
			}
			if err := s.repo.TruncateJobFamilySlice(txCtx, tenantID, current.ID, prevValidDayUTC(effectiveDate)); err != nil {
				return JobFamilyRow{}, mapPgError(err)
			}
			if _, err := s.repo.InsertJobFamilySlice(txCtx, tenantID, id, newName, newActive, effectiveDate, normalizeValidTimeDayUTC(current.EndDate)); err != nil {
				return JobFamilyRow{}, mapPgError(err)
			}
		default:
			if err := s.repo.UpdateJobFamilySliceInPlace(txCtx, tenantID, current.ID, JobFamilySliceInPlacePatch{
				Name:     in.Name,
				IsActive: in.IsActive,
			}); err != nil {
				return JobFamilyRow{}, mapPgError(err)
			}
		}

		identityRow, err := s.repo.UpdateJobFamily(txCtx, tenantID, id, in)
		if err != nil {
			return JobFamilyRow{}, mapPgError(err)
		}
		return JobFamilyRow{
			ID:               identityRow.ID,
			JobFamilyGroupID: identityRow.JobFamilyGroupID,
			Code:             strings.TrimSpace(identityRow.Code),
			Name:             newName,
			IsActive:         newActive,
		}, nil
	})
}

func (s *OrgService) ListJobLevels(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobLevelRow, error) {
		rows, err := s.repo.ListJobLevels(txCtx, tenantID, asOf)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) CreateJobLevel(ctx context.Context, tenantID uuid.UUID, in JobLevelCreate) (JobLevelRow, error) {
	if tenantID == uuid.Nil {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if code == "" || name == "" {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if in.DisplayOrder < 0 {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "display_order must be >= 0", nil)
	}
	if effectiveDate.IsZero() {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	in.Code = code
	in.Name = name
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobLevelRow, error) {
		row, err := s.repo.CreateJobLevel(txCtx, tenantID, in)
		if err != nil {
			return JobLevelRow{}, mapPgError(err)
		}
		if _, err := s.repo.InsertJobLevelSlice(txCtx, tenantID, row.ID, name, in.DisplayOrder, in.IsActive, effectiveDate, endOfTime); err != nil {
			return JobLevelRow{}, mapPgError(err)
		}
		return JobLevelRow{
			ID:           row.ID,
			Code:         strings.TrimSpace(row.Code),
			Name:         name,
			DisplayOrder: in.DisplayOrder,
			IsActive:     in.IsActive,
		}, nil
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
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if effectiveDate.IsZero() {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	mode := normalizeWriteMode(in.WriteMode)
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if in.DisplayOrder != nil && *in.DisplayOrder < 0 {
		return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "display_order must be >= 0", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (JobLevelRow, error) {
		current, err := s.repo.LockJobLevelSliceAt(txCtx, tenantID, id, effectiveDate)
		if err != nil {
			return JobLevelRow{}, mapPgError(err)
		}

		newName := strings.TrimSpace(current.Name)
		if in.Name != nil {
			newName = strings.TrimSpace(*in.Name)
		}
		if newName == "" {
			return JobLevelRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "name is required", nil)
		}
		newOrder := current.DisplayOrder
		if in.DisplayOrder != nil {
			newOrder = *in.DisplayOrder
		}
		newActive := current.IsActive
		if in.IsActive != nil {
			newActive = *in.IsActive
		}

		switch mode {
		case WriteModeUpdateFromDate:
			if effectiveDate.Equal(normalizeValidTimeDayUTC(current.EffectiveDate)) {
				return JobLevelRow{}, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
			}
			if err := s.repo.TruncateJobLevelSlice(txCtx, tenantID, current.ID, prevValidDayUTC(effectiveDate)); err != nil {
				return JobLevelRow{}, mapPgError(err)
			}
			if _, err := s.repo.InsertJobLevelSlice(txCtx, tenantID, id, newName, newOrder, newActive, effectiveDate, normalizeValidTimeDayUTC(current.EndDate)); err != nil {
				return JobLevelRow{}, mapPgError(err)
			}
		default:
			if err := s.repo.UpdateJobLevelSliceInPlace(txCtx, tenantID, current.ID, JobLevelSliceInPlacePatch{
				Name:         in.Name,
				DisplayOrder: in.DisplayOrder,
				IsActive:     in.IsActive,
			}); err != nil {
				return JobLevelRow{}, mapPgError(err)
			}
		}

		identityRow, err := s.repo.UpdateJobLevel(txCtx, tenantID, id, in)
		if err != nil {
			return JobLevelRow{}, mapPgError(err)
		}
		return JobLevelRow{
			ID:           identityRow.ID,
			Code:         strings.TrimSpace(identityRow.Code),
			Name:         newName,
			DisplayOrder: newOrder,
			IsActive:     newActive,
		}, nil
	})
}

func (s *OrgService) ListJobProfiles(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileRow, error) {
		rows, err := s.repo.ListJobProfiles(txCtx, tenantID, asOf)
		return rows, mapPgError(err)
	})
}

func (s *OrgService) ListJobProfilesWithFamilies(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]JobProfileListItem, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileListItem, error) {
		profiles, err := s.repo.ListJobProfiles(txCtx, tenantID, asOf)
		if err != nil {
			return nil, mapPgError(err)
		}
		ids := make([]uuid.UUID, 0, len(profiles))
		for _, p := range profiles {
			ids = append(ids, p.ID)
		}
		familiesByProfileID, err := s.repo.ListJobProfileJobFamiliesByProfileIDsAsOf(txCtx, tenantID, ids, asOf)
		if err != nil {
			return nil, mapPgError(err)
		}

		out := make([]JobProfileListItem, 0, len(profiles))
		for _, p := range profiles {
			out = append(out, JobProfileListItem{
				JobProfileRow: p,
				JobFamilies:   familiesByProfileID[p.ID],
			})
		}
		return out, nil
	})
}

func (s *OrgService) CreateJobProfile(ctx context.Context, tenantID uuid.UUID, in JobProfileCreate) (JobProfileRow, error) {
	if tenantID == uuid.Nil {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if code == "" || name == "" {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/name are required", nil)
	}
	if effectiveDate.IsZero() {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
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
		refs := []byte("{}")
		sliceID, err := s.repo.InsertJobProfileSlice(txCtx, tenantID, row.ID, name, in.Description, in.IsActive, refs, effectiveDate, endOfTime)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		if err := s.repo.SetJobProfileSliceJobFamilies(txCtx, tenantID, sliceID, in.JobFamilies); err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		return JobProfileRow{
			ID:          row.ID,
			Code:        strings.TrimSpace(row.Code),
			Name:        name,
			Description: in.Description,
			IsActive:    in.IsActive,
		}, nil
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
	effectiveDate := normalizeValidTimeDayUTC(in.EffectiveDate)
	if effectiveDate.IsZero() {
		return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "effective_date is required", nil)
	}
	mode := normalizeWriteMode(in.WriteMode)
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
		current, err := s.repo.LockJobProfileSliceAt(txCtx, tenantID, id, effectiveDate)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		existingFamilies, err := s.repo.ListJobProfileSliceJobFamilies(txCtx, tenantID, current.ID)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		existingFamiliesSet := JobProfileJobFamiliesSet{Items: make([]JobProfileJobFamilySetItem, 0, len(existingFamilies))}
		for _, it := range existingFamilies {
			existingFamiliesSet.Items = append(existingFamiliesSet.Items, JobProfileJobFamilySetItem(it))
		}

		newName := strings.TrimSpace(current.Name)
		if in.Name != nil {
			newName = strings.TrimSpace(*in.Name)
		}
		if newName == "" {
			return JobProfileRow{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "name is required", nil)
		}
		newDesc := current.Description
		if in.Description != nil {
			newDesc = *in.Description
		}
		newActive := current.IsActive
		if in.IsActive != nil {
			newActive = *in.IsActive
		}
		familiesSet := existingFamiliesSet
		if in.JobFamilies != nil {
			familiesSet = *in.JobFamilies
		}
		if err := validateJobProfileJobFamiliesSet(familiesSet); err != nil {
			return JobProfileRow{}, err
		}

		refs := current.ExternalRefs
		if len(refs) == 0 {
			refs = []byte("{}")
		}

		switch mode {
		case WriteModeUpdateFromDate:
			if effectiveDate.Equal(normalizeValidTimeDayUTC(current.EffectiveDate)) {
				return JobProfileRow{}, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
			}
			if err := s.repo.TruncateJobProfileSlice(txCtx, tenantID, current.ID, prevValidDayUTC(effectiveDate)); err != nil {
				return JobProfileRow{}, mapPgError(err)
			}
			newSliceID, err := s.repo.InsertJobProfileSlice(txCtx, tenantID, id, newName, newDesc, newActive, refs, effectiveDate, normalizeValidTimeDayUTC(current.EndDate))
			if err != nil {
				return JobProfileRow{}, mapPgError(err)
			}
			if err := s.repo.SetJobProfileSliceJobFamilies(txCtx, tenantID, newSliceID, familiesSet); err != nil {
				return JobProfileRow{}, mapPgError(err)
			}
		default:
			if err := s.repo.UpdateJobProfileSliceInPlace(txCtx, tenantID, current.ID, JobProfileSliceInPlacePatch{
				Name:        in.Name,
				Description: in.Description,
				IsActive:    in.IsActive,
			}); err != nil {
				return JobProfileRow{}, mapPgError(err)
			}
			if in.JobFamilies != nil {
				if err := s.repo.SetJobProfileSliceJobFamilies(txCtx, tenantID, current.ID, *in.JobFamilies); err != nil {
					return JobProfileRow{}, mapPgError(err)
				}
			}
		}

		identityRow, err := s.repo.UpdateJobProfile(txCtx, tenantID, id, in)
		if err != nil {
			return JobProfileRow{}, mapPgError(err)
		}
		return JobProfileRow{
			ID:          identityRow.ID,
			Code:        strings.TrimSpace(identityRow.Code),
			Name:        newName,
			Description: newDesc,
			IsActive:    newActive,
		}, nil
	})
}

func (s *OrgService) ListJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, asOf time.Time) ([]JobProfileJobFamilyRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if jobProfileID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "job_profile_id is required", nil)
	}
	if asOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "as_of is required", nil)
	}
	asOf = normalizeValidTimeDayUTC(asOf)
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]JobProfileJobFamilyRow, error) {
		rows, err := s.repo.ListJobProfileJobFamilies(txCtx, tenantID, jobProfileID, asOf)
		return rows, mapPgError(err)
	})
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
