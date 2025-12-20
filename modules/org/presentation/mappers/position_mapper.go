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
		out = append(out, viewmodels.OrgPositionRow{
			ID:              r.PositionID,
			Code:            strings.TrimSpace(r.Code),
			Title:           title,
			OrgNodeID:       r.OrgNodeID,
			LifecycleStatus: strings.TrimSpace(r.LifecycleStatus),
			IsAutoCreated:   r.IsAutoCreated,
			CapacityFTE:     r.CapacityFTE,
			OccupiedFTE:     r.OccupiedFTE,
			AvailableFTE:    r.CapacityFTE - r.OccupiedFTE,
			StaffingState:   strings.TrimSpace(r.StaffingState),
			EffectiveDate:   r.EffectiveDate,
			EndDate:         r.EndDate,
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
