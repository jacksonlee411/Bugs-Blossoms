package viewmodels

import (
	"time"

	"github.com/google/uuid"
)

type OrgPositionRow struct {
	ID              uuid.UUID
	Code            string
	Title           string
	OrgNodeID       uuid.UUID
	LifecycleStatus string
	IsAutoCreated   bool
	CapacityFTE     float64
	OccupiedFTE     float64
	AvailableFTE    float64
	StaffingState   string
	EffectiveDate   time.Time
	EndDate         time.Time
}

type OrgPositionDetails struct {
	Row                 OrgPositionRow
	ReportsToPositionID *uuid.UUID
}

type OrgPositionTimelineItem struct {
	ID                  uuid.UUID
	EffectiveDate       time.Time
	EndDate             time.Time
	OrgNodeID           uuid.UUID
	Title               string
	LifecycleStatus     string
	CapacityFTE         float64
	ReportsToPositionID *uuid.UUID
}
