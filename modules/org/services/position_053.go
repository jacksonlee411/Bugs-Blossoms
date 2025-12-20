package services

import (
	"encoding/json"
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
	PositionType        *string
	EmploymentType      *string
	CapacityFTE         float64
	ReportsToPositionID *uuid.UUID
	JobFamilyGroupCode  *string
	JobFamilyCode       *string
	JobRoleCode         *string
	JobLevelCode        *string
	JobProfileID        *uuid.UUID
	CostCenterCode      *string
	Profile             json.RawMessage
	EffectiveDate       time.Time
	EndDate             time.Time
}

type PositionSliceInPlacePatch struct {
	OrgNodeID           *uuid.UUID
	Title               *string
	LifecycleStatus     *string
	PositionType        *string
	EmploymentType      *string
	CapacityFTE         *float64
	ReportsToPositionID *uuid.UUID
	JobFamilyGroupCode  *string
	JobFamilyCode       *string
	JobRoleCode         *string
	JobLevelCode        *string
	JobProfileID        *uuid.UUID
	CostCenterCode      *string
	Profile             *json.RawMessage
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
	PositionID          uuid.UUID       `json:"position_id"`
	Code                string          `json:"code"`
	OrgNodeID           uuid.UUID       `json:"org_node_id"`
	Title               *string         `json:"title,omitempty"`
	LifecycleStatus     string          `json:"lifecycle_status"`
	IsAutoCreated       bool            `json:"is_auto_created"`
	CapacityFTE         float64         `json:"capacity_fte"`
	OccupiedFTE         float64         `json:"occupied_fte"`
	StaffingState       string          `json:"staffing_state"`
	PositionType        *string         `json:"position_type,omitempty"`
	EmploymentType      *string         `json:"employment_type,omitempty"`
	ReportsToPositionID *uuid.UUID      `json:"reports_to_position_id,omitempty"`
	JobFamilyGroupCode  *string         `json:"job_family_group_code,omitempty"`
	JobFamilyCode       *string         `json:"job_family_code,omitempty"`
	JobRoleCode         *string         `json:"job_role_code,omitempty"`
	JobLevelCode        *string         `json:"job_level_code,omitempty"`
	JobProfileID        *uuid.UUID      `json:"job_profile_id,omitempty"`
	CostCenterCode      *string         `json:"cost_center_code,omitempty"`
	Profile             json.RawMessage `json:"profile,omitempty"`
	EffectiveDate       time.Time       `json:"effective_date"`
	EndDate             time.Time       `json:"end_date"`
}

type PositionSliceRow struct {
	ID                  uuid.UUID
	PositionID          uuid.UUID
	OrgNodeID           uuid.UUID
	Title               *string
	LifecycleStatus     string
	PositionType        *string
	EmploymentType      *string
	CapacityFTE         float64
	ReportsToPositionID *uuid.UUID
	JobFamilyGroupCode  *string
	JobFamilyCode       *string
	JobRoleCode         *string
	JobLevelCode        *string
	JobProfileID        *uuid.UUID
	CostCenterCode      *string
	Profile             json.RawMessage
	EffectiveDate       time.Time
	EndDate             time.Time
}
