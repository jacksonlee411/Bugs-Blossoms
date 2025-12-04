package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePolicyEntry_GroupingPolicy(t *testing.T) {
	entry, err := parsePolicyEntry("g, tenant:foo:user:123, role:core.superadmin, foo-tenant")
	require.NoError(t, err)
	require.Equal(t, "g", entry.Type)
	require.Equal(t, "tenant:foo:user:123", entry.Subject)
	require.Equal(t, "role:core.superadmin", entry.Object)
	require.Equal(t, "foo-tenant", entry.Domain)
	require.Equal(t, "*", entry.Action)
}

func TestParsePolicyEntry_PolicyDefaults(t *testing.T) {
	entry, err := parsePolicyEntry("p, role:core.superadmin, *, *, *")
	require.NoError(t, err)
	require.Equal(t, "p", entry.Type)
	require.Equal(t, "role:core.superadmin", entry.Subject)
	require.Equal(t, "*", entry.Domain)
	require.Equal(t, "*", entry.Object)
	require.Equal(t, "*", entry.Action)
	require.Equal(t, "allow", entry.Effect)
}
