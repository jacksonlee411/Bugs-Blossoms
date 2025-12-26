package authzutil

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/iota-uz/iota-sdk/pkg/authz"
)

// ForbiddenPayload represents the unified forbidden response contract.
type ForbiddenPayload struct {
	Error           string                `json:"error"`
	Message         string                `json:"message"`
	Object          string                `json:"object"`
	Action          string                `json:"action"`
	Subject         string                `json:"subject"`
	Domain          string                `json:"domain"`
	MissingPolicies []authz.MissingPolicy `json:"missing_policies"`
	DebugURL        string                `json:"debug_url"`
	BaseRevision    string                `json:"base_revision,omitempty"`
	RequestID       string                `json:"request_id,omitempty"`
}

// BuildForbiddenPayload constructs a forbidden payload using view state and request context.
func BuildForbiddenPayload(r *http.Request, state *authz.ViewState, object, action string) ForbiddenPayload {
	normalizedAction := authz.NormalizeAction(action)
	domain := DomainFromContext(r.Context())
	subject := ""
	var missingPolicies []authz.MissingPolicy
	baseRevision := BaseRevision(r.Context())
	requestID := RequestIDFromRequest(r)

	if state != nil {
		subject = state.Subject
		if state.Tenant != "" {
			domain = state.Tenant
		}
		missingPolicies = append(missingPolicies, state.MissingPolicies...)
	}

	if len(missingPolicies) == 0 && object != "" && normalizedAction != "" {
		missingPolicies = []authz.MissingPolicy{{
			Domain: domain,
			Object: object,
			Action: normalizedAction,
		}}
	}

	debugURL := "/core/api/authz/debug"
	if subject != "" && object != "" && normalizedAction != "" {
		q := url.Values{}
		q.Set("subject", subject)
		q.Set("object", object)
		q.Set("action", normalizedAction)
		if domain != "" {
			q.Set("domain", domain)
		}
		debugURL = fmt.Sprintf("/core/api/authz/debug?%s", q.Encode())
	}

	return ForbiddenPayload{
		Error:           "forbidden",
		Message:         fmt.Sprintf("Forbidden: %s %s.", object, normalizedAction),
		Object:          object,
		Action:          normalizedAction,
		Subject:         subject,
		Domain:          domain,
		MissingPolicies: missingPolicies,
		DebugURL:        debugURL,
		BaseRevision:    baseRevision,
		RequestID:       requestID,
	}
}
