package entities

import (
	"time"

	"github.com/google/uuid"
)

type TenantDomain struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	Hostname                  string
	IsPrimary                 bool
	VerificationToken         string
	LastVerificationAttemptAt *time.Time
	LastVerificationError     *string
	VerifiedAt                *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}
