package authorization

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
)

func normalizeAction(action string) string {
	return authz.NormalizeAction(action)
}

func normalizePolicies(state *authz.ViewState, object, action string) []authz.MissingPolicy {
	if state == nil {
		return nil
	}
	if strings.TrimSpace(object) == "" && strings.TrimSpace(action) == "" {
		return state.MissingPolicies
	}
	normalizedObject := strings.ToLower(strings.TrimSpace(object))
	normalizedAction := normalizeAction(action)

	matched := make([]authz.MissingPolicy, 0, len(state.MissingPolicies))
	for _, policy := range state.MissingPolicies {
		if normalizedObject != "" && !strings.EqualFold(policy.Object, normalizedObject) {
			continue
		}
		if normalizedAction != "*" && policy.Action != normalizedAction {
			continue
		}
		matched = append(matched, policy)
	}
	if len(matched) == 0 {
		return state.MissingPolicies
	}
	return matched
}

func suggestedDiff(state *authz.ViewState, policies []authz.MissingPolicy) string {
	if state == nil || len(policies) == 0 {
		return "[]"
	}
	suggestions := make([]authz.PolicySuggestion, 0, len(policies))
	for _, policy := range policies {
		suggestions = append(suggestions, state.SuggestDiff(policy)...)
	}
	if len(suggestions) == 0 {
		return "[]"
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func parseSuggestedDiff(diff string) []authz.PolicySuggestion {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return nil
	}
	var suggestions []authz.PolicySuggestion
	if err := json.Unmarshal([]byte(diff), &suggestions); err != nil {
		return nil
	}
	return suggestions
}

func resolveSubject(state *authz.ViewState, provided string) string {
	subject := strings.TrimSpace(provided)
	if subject == "" && state != nil {
		subject = state.Subject
	}
	return subject
}

func resolveDomain(state *authz.ViewState, provided string) string {
	domain := strings.TrimSpace(provided)
	if domain == "" && state != nil {
		domain = state.Tenant
	}
	return domain
}

func resolveBaseRevision(ctx context.Context, provided string) string {
	baseRevision := strings.TrimSpace(provided)
	if baseRevision == "" {
		baseRevision = authzutil.BaseRevision(ctx)
	}
	return baseRevision
}
