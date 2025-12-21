package services

import (
	"time"

	"github.com/google/uuid"
)

type StaffingScope string

const (
	StaffingScopeSelf    StaffingScope = "self"
	StaffingScopeSubtree StaffingScope = "subtree"
)

type StaffingGroupBy string

const (
	StaffingGroupByNone         StaffingGroupBy = "none"
	StaffingGroupByJobLevel     StaffingGroupBy = "job_level"
	StaffingGroupByPositionType StaffingGroupBy = "position_type"
)

type StaffingReportSource struct {
	DeepReadBackend DeepReadBackend
	SnapshotBuildID *uuid.UUID
}

type StaffingTotals struct {
	PositionsTotal int
	CapacityFTE    float64
	OccupiedFTE    float64
	AvailableFTE   float64
	FillRate       float64
}

type StaffingBreakdownRow struct {
	Key            string
	PositionsTotal int
	CapacityFTE    float64
	OccupiedFTE    float64
	AvailableFTE   float64
	FillRate       float64
}

type StaffingAggregateRow struct {
	Key            string
	PositionsTotal int
	CapacityFTE    float64
	OccupiedFTE    float64
	AvailableFTE   float64
}

type StaffingSummaryDBResult struct {
	Totals    StaffingAggregateRow
	Breakdown []StaffingAggregateRow
}

type StaffingSummaryResult struct {
	TenantID      uuid.UUID
	OrgNodeID     uuid.UUID
	EffectiveDate time.Time
	Scope         StaffingScope
	Totals        StaffingTotals
	Breakdown     []StaffingBreakdownRow
	Source        StaffingReportSource
}

type StaffingVacancyRow struct {
	PositionID     uuid.UUID
	PositionCode   string
	OrgNodeID      uuid.UUID
	CapacityFTE    float64
	OccupiedFTE    float64
	VacancySince   time.Time
	VacancyAgeDays int
	JobLevelID     *uuid.UUID
	PositionType   string
}

type StaffingVacanciesDBResult struct {
	Items      []StaffingVacancyRow
	NextCursor *uuid.UUID
}

type StaffingVacanciesResult struct {
	TenantID      uuid.UUID
	OrgNodeID     uuid.UUID
	EffectiveDate time.Time
	Scope         StaffingScope
	Items         []StaffingVacancyRow
	NextCursor    *uuid.UUID
	Source        StaffingReportSource
}

type StaffingTimeToFillSummary struct {
	FilledCount int
	AvgDays     float64
	P50Days     int
	P95Days     int
}

type StaffingTimeToFillBreakdownRow struct {
	Key         string
	FilledCount int
	AvgDays     float64
}

type StaffingTimeToFillDBResult struct {
	Summary   StaffingTimeToFillSummary
	Breakdown []StaffingTimeToFillBreakdownRow
}

type StaffingTimeToFillResult struct {
	TenantID  uuid.UUID
	OrgNodeID uuid.UUID
	From      time.Time
	To        time.Time
	Scope     StaffingScope
	Summary   StaffingTimeToFillSummary
	Breakdown []StaffingTimeToFillBreakdownRow
	Source    StaffingReportSource
}
