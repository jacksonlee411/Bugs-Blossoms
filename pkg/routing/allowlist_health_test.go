package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowlist_LoadsAndHasCriticalRules(t *testing.T) {
	serverRules, err := LoadAllowlist("", "server")
	require.NoError(t, err)

	superadminRules, err := LoadAllowlist("", "superadmin")
	require.NoError(t, err)
	require.NotEmpty(t, superadminRules)

	requireAllowlistRule(t, serverRules, "/api/v1", RouteClassPublicAPI)
	requireAllowlistRule(t, serverRules, "/health", RouteClassOps)
	requireAllowlistRule(t, serverRules, "/debug/prometheus", RouteClassOps)
	requireAllowlistRule(t, serverRules, "/_dev", RouteClassDevOnly)
	requireAllowlistRule(t, serverRules, "/playground", RouteClassDevOnly)
	requireAllowlistRule(t, serverRules, "/__test__", RouteClassTest)
}

func requireAllowlistRule(t *testing.T, rules []AllowlistRule, prefix string, class RouteClass) {
	t.Helper()

	for _, rule := range rules {
		if rule.Prefix == prefix && rule.Class == class {
			return
		}
	}
	t.Fatalf("allowlist missing rule: %q -> %q", prefix, class)
}
