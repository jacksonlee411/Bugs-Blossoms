package authorization

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/pkg/authz"
)

func TestNormalizePoliciesFiltersAndFallsBack(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	domain := authz.DomainFromTenant(tenantID)
	state := authz.NewViewState("tenant:demo:user:1", domain)
	state.AddMissingPolicy(authz.MissingPolicy{Domain: domain, Object: "core.users", Action: "list"})
	state.AddMissingPolicy(authz.MissingPolicy{Domain: domain, Object: "logging.logs", Action: "view"})

	filtered := normalizePolicies(state, "core.users", "list")
	require.Len(t, filtered, 1)
	require.Equal(t, "core.users", filtered[0].Object)
	require.Equal(t, "list", filtered[0].Action)

	fallback := normalizePolicies(state, "unknown", "delete")
	require.Len(t, fallback, len(state.MissingPolicies))
}

func TestSuggestedDiffBuildsJSON(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	domain := authz.DomainFromTenant(tenantID)
	state := authz.NewViewState("tenant:demo:user:42", domain)
	policies := []authz.MissingPolicy{{
		Domain: domain,
		Object: "logging.logs",
		Action: "view",
	}}

	diff := suggestedDiff(state, policies)
	require.NotEqual(t, "[]", diff)

	var suggestions []authz.PolicySuggestion
	require.NoError(t, json.Unmarshal([]byte(diff), &suggestions))
	require.Len(t, suggestions, 1)
	require.Equal(t, state.Subject, suggestions[0].Subject)
	require.Equal(t, domain, suggestions[0].Domain)
	require.Equal(t, "logging.logs", suggestions[0].Object)
	require.Equal(t, "view", suggestions[0].Action)
	require.Equal(t, "allow", suggestions[0].Effect)
	require.Equal(t, "[]", suggestedDiff(nil, policies))
}

func TestResolveHelpers(t *testing.T) {
	setAuthzPolicyPath(t)

	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	domain := authz.DomainFromTenant(tenantID)
	state := authz.NewViewState("tenant:demo:user:99", domain)

	require.Equal(t, "provided-subject", resolveSubject(state, " provided-subject "))
	require.Equal(t, state.Subject, resolveSubject(state, ""))

	require.Equal(t, domain, resolveDomain(state, " "))
	require.Equal(t, "custom-domain", resolveDomain(state, " custom-domain "))

	require.Equal(t, "given-revision", resolveBaseRevision(context.Background(), " given-revision "))
	require.NotEmpty(t, resolveBaseRevision(context.Background(), ""))
}

func setAuthzPolicyPath(t *testing.T) {
	t.Helper()
	policyPath := filepath.Clean(filepath.Join("..", "..", "config", "access", "policy.csv"))
	t.Setenv("AUTHZ_POLICY_PATH", policyPath)
}
