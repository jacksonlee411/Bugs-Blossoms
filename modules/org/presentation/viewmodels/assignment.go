package viewmodels

import (
	"time"

	"github.com/google/uuid"
)

type OrgAssignmentRow struct {
	ID            uuid.UUID
	PositionID    uuid.UUID
	OrgNodeID     uuid.UUID
	Pernr         string
	PositionCode  string
	EffectiveDate time.Time
	EndDate       time.Time
}

type OrgAssignmentsTimeline struct {
	Subject string
	Pernr   string
	Rows    []OrgAssignmentRow
}
