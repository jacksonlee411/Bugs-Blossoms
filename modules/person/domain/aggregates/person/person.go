package person

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

type Person struct {
	tenantID    uuid.UUID
	personUUID  uuid.UUID
	pernr       string
	displayName string
	status      Status
	createdAt   time.Time
	updatedAt   time.Time
}

func New(tenantID uuid.UUID, pernr string, displayName string) Person {
	return Person{
		tenantID:    tenantID,
		pernr:       normalizePernr(pernr),
		displayName: strings.TrimSpace(displayName),
		status:      StatusActive,
	}
}

func Hydrate(
	tenantID uuid.UUID,
	personUUID uuid.UUID,
	pernr string,
	displayName string,
	status Status,
	createdAt time.Time,
	updatedAt time.Time,
) Person {
	return Person{
		tenantID:    tenantID,
		personUUID:  personUUID,
		pernr:       normalizePernr(pernr),
		displayName: strings.TrimSpace(displayName),
		status:      status,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}
}

func (p Person) TenantID() uuid.UUID   { return p.tenantID }
func (p Person) PersonUUID() uuid.UUID { return p.personUUID }
func (p Person) Pernr() string         { return p.pernr }
func (p Person) DisplayName() string   { return p.displayName }
func (p Person) Status() Status        { return p.status }
func (p Person) CreatedAt() time.Time  { return p.createdAt }
func (p Person) UpdatedAt() time.Time  { return p.updatedAt }
func (p Person) IsZero() bool          { return p.personUUID == uuid.Nil && p.pernr == "" }
func normalizePernr(v string) string   { return strings.TrimSpace(v) }
