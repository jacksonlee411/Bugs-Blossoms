package entities

import (
	"time"

	"github.com/google/uuid"
)

type TenantAuthSettings struct {
	TenantID      uuid.UUID
	IdentityMode  string
	AllowPassword bool
	AllowGoogle   bool
	AllowSSO      bool
	UpdatedAt     time.Time
}
