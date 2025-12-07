package authzutil

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/iota-uz/iota-sdk/pkg/authz"
)

// ForbiddenPayload represents the unified forbidden response contract.
type ForbiddenPayload struct {
	Error           string                   `json:"error"`
	Message         string                   `json:"message"`
	Object          string                   `json:"object"`
	Action          string                   `json:"action"`
	Subject         string                   `json:"subject"`
	Domain          string                   `json:"domain"`
	MissingPolicies []authz.MissingPolicy    `json:"missing_policies"`
	SuggestDiff     []authz.PolicySuggestion `json:"suggest_diff"`
	RequestURL      string                   `json:"request_url"`
	DebugURL        string                   `json:"debug_url"`
}

// BuildForbiddenPayload constructs a forbidden payload using view state and request context.
func BuildForbiddenPayload(r *http.Request, state *authz.ViewState, object, action string) ForbiddenPayload {
	normalizedAction := authz.NormalizeAction(action)
	domain := DomainFromContext(r.Context())
	subject := ""
	var missingPolicies []authz.MissingPolicy
	var suggestDiff []authz.PolicySuggestion

	if state != nil {
		subject = state.Subject
		if state.Tenant != "" {
			domain = state.Tenant
		}
		missingPolicies = append(missingPolicies, state.MissingPolicies...)
		for _, policy := range state.MissingPolicies {
			suggestDiff = append(suggestDiff, state.SuggestDiff(policy)...)
		}
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
		Message:         fmt.Sprintf("Forbidden: %s %s. 如需申请权限，请访问 /core/api/authz/requests。", object, normalizedAction),
		Object:          object,
		Action:          normalizedAction,
		Subject:         subject,
		Domain:          domain,
		MissingPolicies: missingPolicies,
		SuggestDiff:     suggestDiff,
		RequestURL:      "/core/api/authz/requests",
		DebugURL:        debugURL,
	}
}
