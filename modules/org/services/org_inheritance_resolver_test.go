package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestResolveOrgAttributes_CanOverrideTrue(t *testing.T) {
	rootID := uuid.New()
	childID := uuid.New()

	nodes := []HierarchyNode{
		{ID: rootID, ParentID: nil},
		{ID: childID, ParentID: &rootID},
	}

	explicit := map[uuid.UUID]OrgNodeAttributes{
		rootID:  {CompanyCode: strPtr("ACME")},
		childID: {CompanyCode: strPtr("FOO")},
	}
	rules := map[string]AttributeInheritanceRule{
		"company_code": {AttributeName: "company_code", CanOverride: true},
	}

	resolved, sources := resolveOrgAttributes(nodes, explicit, rules)

	require.Equal(t, "FOO", requirePtr(t, resolved[childID].CompanyCode))
	require.Equal(t, childID, requireUUIDPtr(t, sources[childID].CompanyCode))

	explicit[childID] = OrgNodeAttributes{CompanyCode: nil}
	resolved, sources = resolveOrgAttributes(nodes, explicit, rules)

	require.Equal(t, "ACME", requirePtr(t, resolved[childID].CompanyCode))
	require.Equal(t, rootID, requireUUIDPtr(t, sources[childID].CompanyCode))
}

func TestResolveOrgAttributes_CanOverrideFalse(t *testing.T) {
	rootID := uuid.New()
	childID := uuid.New()

	nodes := []HierarchyNode{
		{ID: rootID, ParentID: nil},
		{ID: childID, ParentID: &rootID},
	}

	explicit := map[uuid.UUID]OrgNodeAttributes{
		rootID:  {CompanyCode: strPtr("ACME")},
		childID: {CompanyCode: strPtr("FOO")},
	}
	rules := map[string]AttributeInheritanceRule{
		"company_code": {AttributeName: "company_code", CanOverride: false},
	}

	resolved, sources := resolveOrgAttributes(nodes, explicit, rules)

	require.Equal(t, "ACME", requirePtr(t, resolved[childID].CompanyCode))
	require.Equal(t, rootID, requireUUIDPtr(t, sources[childID].CompanyCode))
}

func TestResolveOrgAttributes_MultiLevelSourcePropagation(t *testing.T) {
	rootID := uuid.New()
	midID := uuid.New()
	leafID := uuid.New()

	nodes := []HierarchyNode{
		{ID: rootID, ParentID: nil},
		{ID: midID, ParentID: &rootID},
		{ID: leafID, ParentID: &midID},
	}

	explicit := map[uuid.UUID]OrgNodeAttributes{
		rootID: {CompanyCode: strPtr("ACME")},
		midID:  {},
		leafID: {},
	}
	rules := map[string]AttributeInheritanceRule{
		"company_code": {AttributeName: "company_code", CanOverride: true},
	}

	resolved, sources := resolveOrgAttributes(nodes, explicit, rules)

	require.Equal(t, "ACME", requirePtr(t, resolved[midID].CompanyCode))
	require.Equal(t, rootID, requireUUIDPtr(t, sources[midID].CompanyCode))

	require.Equal(t, "ACME", requirePtr(t, resolved[leafID].CompanyCode))
	require.Equal(t, rootID, requireUUIDPtr(t, sources[leafID].CompanyCode))
}

func TestResolveOrgAttributes_NoRule_NoInheritance(t *testing.T) {
	rootID := uuid.New()
	childID := uuid.New()

	nodes := []HierarchyNode{
		{ID: rootID, ParentID: nil},
		{ID: childID, ParentID: &rootID},
	}

	explicit := map[uuid.UUID]OrgNodeAttributes{
		rootID: {CompanyCode: strPtr("ACME")},
	}
	rules := map[string]AttributeInheritanceRule{}

	resolved, sources := resolveOrgAttributes(nodes, explicit, rules)

	require.Equal(t, "ACME", requirePtr(t, resolved[rootID].CompanyCode))
	require.Equal(t, rootID, requireUUIDPtr(t, sources[rootID].CompanyCode))

	require.Nil(t, resolved[childID].CompanyCode)
	require.Nil(t, sources[childID].CompanyCode)
}

func strPtr(v string) *string { return &v }

func requirePtr(tb testing.TB, v *string) string {
	tb.Helper()
	require.NotNil(tb, v)
	return *v
}

func requireUUIDPtr(tb testing.TB, v *uuid.UUID) uuid.UUID {
	tb.Helper()
	require.NotNil(tb, v)
	return *v
}
