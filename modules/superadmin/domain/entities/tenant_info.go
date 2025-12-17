package entities

import (
	"time"

	"github.com/google/uuid"
)

// TenantInfo represents a tenant with additional metadata for superadmin view
type TenantInfo struct {
	ID                   uuid.UUID
	Name                 string
	Email                *string
	Phone                *string
	Domain               string // display domain (prefer primary domain)
	IsActive             bool
	IdentityMode         string
	AllowSSO             bool
	SSOConnectionsTotal  int
	SSOConnectionsActive int
	UserCount            int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
