package viewmodels

import (
	"time"

	"github.com/google/uuid"
)

type OrgNodeDetails struct {
	ID            uuid.UUID
	Code          string
	Name          string
	Status        string
	DisplayOrder  int
	ParentHint    *uuid.UUID
	LegalEntityID *uuid.UUID
	CompanyCode   *string
	LocationID    *uuid.UUID
	ManagerUserID *int64
	EffectiveDate time.Time
	EndDate       time.Time
	I18nNamesJSON string
	IsRoot        bool
}
