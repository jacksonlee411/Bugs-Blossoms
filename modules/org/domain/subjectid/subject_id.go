package subjectid

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	subjectIDNamespaceV1 = uuid.MustParse("ce7c5394-3959-40ff-9d92-a1c2684d94cc")
)

// NormalizedSubjectID returns the deterministic subject_id for org_assignments.
// SSOT: docs/dev-plans/026-org-api-authz-and-events.md#L340
func NormalizedSubjectID(tenantID uuid.UUID, subjectType, pernr string) (uuid.UUID, error) {
	if tenantID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("tenant_id is required")
	}
	subjectType = strings.TrimSpace(subjectType)
	if subjectType == "" {
		return uuid.Nil, fmt.Errorf("subject_type is required")
	}
	if subjectType != "person" {
		return uuid.Nil, fmt.Errorf("unsupported subject_type: %s", subjectType)
	}

	pernr = strings.TrimSpace(pernr)
	if pernr == "" {
		return uuid.Nil, fmt.Errorf("pernr is required")
	}

	payload := fmt.Sprintf("%s:%s:%s", tenantID, subjectType, pernr)
	return uuid.NewSHA1(subjectIDNamespaceV1, []byte(payload)), nil
}
