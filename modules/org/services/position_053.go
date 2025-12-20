package services

import (
	"time"

	"github.com/google/uuid"
)

type PositionInsert struct {
	PositionID    uuid.UUID
	OrgNodeID     uuid.UUID
	Code          string
	Title         *string
	LegacyStatus  string
	IsAutoCreated bool
	EffectiveDate time.Time
	EndDate       time.Time
}

type PositionSliceInsert struct {
	OrgNodeID           uuid.UUID
	Title               *string
	LifecycleStatus     string
	CapacityFTE         float64
	ReportsToPositionID *uuid.UUID
	EffectiveDate       time.Time
	EndDate             time.Time
}

type PositionSliceInPlacePatch struct {
	OrgNodeID           *uuid.UUID
	Title               *string
	LifecycleStatus     *string
	CapacityFTE         *float64
	ReportsToPositionID *uuid.UUID
}

type PositionListFilter struct {
	OrgNodeID       *uuid.UUID
	OrgNodeIDs      []uuid.UUID
	Q               *string
	LifecycleStatus *string
	StaffingState   *string
	IsAutoCreated   *bool
	Limit           int
	Offset          int
}

type PositionViewRow struct {
	PositionID      uuid.UUID `json:"position_id"`
	Code            string    `json:"code"`
	OrgNodeID       uuid.UUID `json:"org_node_id"`
	Title           *string   `json:"title,omitempty"`
	LifecycleStatus string    `json:"lifecycle_status"`
	IsAutoCreated   bool      `json:"is_auto_created"`
	CapacityFTE     float64   `json:"capacity_fte"`
	OccupiedFTE     float64   `json:"occupied_fte"`
	StaffingState   string    `json:"staffing_state"`
	EffectiveDate   time.Time `json:"effective_date"`
	EndDate         time.Time `json:"end_date"`
}

type PositionSliceRow struct {
	ID                  uuid.UUID
	PositionID          uuid.UUID
	OrgNodeID           uuid.UUID
	Title               *string
	LifecycleStatus     string
	CapacityFTE         float64
	ReportsToPositionID *uuid.UUID
	EffectiveDate       time.Time
	EndDate             time.Time
}
