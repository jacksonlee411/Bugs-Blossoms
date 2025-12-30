package mappers

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func PositionsToViewModels(rows []services.PositionViewRow) []viewmodels.OrgPositionRow {
	out := make([]viewmodels.OrgPositionRow, 0, len(rows))
	for _, r := range rows {
		title := ""
		if r.Title != nil {
			title = strings.TrimSpace(*r.Title)
		}
		positionType := ""
		if r.PositionType != nil {
			positionType = strings.TrimSpace(*r.PositionType)
		}
		employmentType := ""
		if r.EmploymentType != nil {
			employmentType = strings.TrimSpace(*r.EmploymentType)
		}
		jobFamilyGroupCode := ""
		if r.JobFamilyGroupCode != nil {
			jobFamilyGroupCode = strings.TrimSpace(*r.JobFamilyGroupCode)
		}
		jobFamilyCode := ""
		if r.JobFamilyCode != nil {
			jobFamilyCode = strings.TrimSpace(*r.JobFamilyCode)
		}
		jobLevelCode := ""
		if r.JobLevelCode != nil {
			jobLevelCode = strings.TrimSpace(*r.JobLevelCode)
		}
		costCenterCode := ""
		if r.CostCenterCode != nil {
			costCenterCode = strings.TrimSpace(*r.CostCenterCode)
		}
		out = append(out, viewmodels.OrgPositionRow{
			ID:                 r.PositionID,
			Code:               strings.TrimSpace(r.Code),
			Title:              title,
			OrgNodeID:          r.OrgNodeID,
			LifecycleStatus:    strings.TrimSpace(r.LifecycleStatus),
			IsAutoCreated:      r.IsAutoCreated,
			CapacityFTE:        r.CapacityFTE,
			OccupiedFTE:        r.OccupiedFTE,
			AvailableFTE:       r.CapacityFTE - r.OccupiedFTE,
			StaffingState:      strings.TrimSpace(r.StaffingState),
			PositionType:       positionType,
			EmploymentType:     employmentType,
			JobFamilyGroupCode: jobFamilyGroupCode,
			JobFamilyCode:      jobFamilyCode,
			JobLevelCode:       jobLevelCode,
			JobProfileID:       r.JobProfileID,
			CostCenterCode:     costCenterCode,
			EffectiveDate:      r.EffectiveDate,
			EndDate:            r.EndDate,
		})
	}
	return out
}

func PositionDetailsFrom(row services.PositionViewRow, reportsToID *uuid.UUID) *viewmodels.OrgPositionDetails {
	items := PositionsToViewModels([]services.PositionViewRow{row})
	if len(items) == 0 {
		return nil
	}
	return &viewmodels.OrgPositionDetails{
		Row:                 items[0],
		ReportsToPositionID: reportsToID,
		ReportsToLabel:      "",
		JobFamilyGroupLabel: "",
		JobFamilyLabel:      "",
		JobLevelLabel:       "",
	}
}

func PositionTimelineToViewModels(slices []services.PositionSliceRow) []viewmodels.OrgPositionTimelineItem {
	out := make([]viewmodels.OrgPositionTimelineItem, 0, len(slices))
	for _, s := range slices {
		title := ""
		if s.Title != nil {
			title = strings.TrimSpace(*s.Title)
		}
		out = append(out, viewmodels.OrgPositionTimelineItem{
			ID:                  s.ID,
			EffectiveDate:       s.EffectiveDate,
			EndDate:             s.EndDate,
			OrgNodeID:           s.OrgNodeID,
			Title:               title,
			LifecycleStatus:     strings.TrimSpace(s.LifecycleStatus),
			CapacityFTE:         s.CapacityFTE,
			ReportsToPositionID: s.ReportsToPositionID,
		})
	}
	return out
}

func FindPositionSliceAt(slices []services.PositionSliceRow, asOf time.Time) *services.PositionSliceRow {
	asOf = asOf.UTC()
	for _, s := range slices {
		if !s.EffectiveDate.After(asOf) && s.EndDate.After(asOf) {
			sliceAt := s
			return &sliceAt
		}
	}
	return nil
}
