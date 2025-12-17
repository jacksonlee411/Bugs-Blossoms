package entities

import (
	"time"

	"github.com/google/uuid"
)

type TenantSSOConnection struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	ConnectionID        string
	DisplayName         string
	Protocol            string
	Enabled             bool
	JacksonBaseURL      string
	KratosProviderID    string
	SAMLMetadataURL     *string
	SAMLMetadataXML     *string
	OIDCIssuer          *string
	OIDCClientID        *string
	OIDCClientSecretRef *string
	LastTestStatus      *string
	LastTestError       *string
	LastTestAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
