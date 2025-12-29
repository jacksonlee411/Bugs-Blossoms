package viewmodels

import (
	"time"

	"github.com/google/uuid"
)

type OrgNodeSliceRecord struct {
	SliceID       uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
	Name          string
	Status        string
	ActiveAtAsOf  bool
}

type OrgEdgeSliceRecord struct {
	EdgeID            uuid.UUID
	ParentNodeID      *uuid.UUID
	ParentNameAtStart *string
	ParentCode        *string
	EffectiveDate     time.Time
	EndDate           time.Time
	ActiveAtAsOf      bool
	IsEarliest        bool
}
