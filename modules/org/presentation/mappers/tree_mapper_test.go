package mappers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func TestHierarchyToTree_PreOrderByParentID(t *testing.T) {
	rootID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	aID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	a1ID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	bID := uuid.MustParse("00000000-0000-0000-0000-000000000004")

	nodes := []services.HierarchyNode{
		{ID: bID, Name: "B", DisplayOrder: 2, ParentID: &rootID, Depth: 1},
		{ID: a1ID, Name: "A1", DisplayOrder: 0, ParentID: &aID, Depth: 2},
		{ID: rootID, Name: "ROOT", DisplayOrder: 0, ParentID: nil, Depth: 0},
		{ID: aID, Name: "A", DisplayOrder: 1, ParentID: &rootID, Depth: 1},
	}

	tree := HierarchyToTree(nodes, nil)
	require.NotNil(t, tree)
	require.Len(t, tree.Nodes, 4)

	got := []uuid.UUID{tree.Nodes[0].ID, tree.Nodes[1].ID, tree.Nodes[2].ID, tree.Nodes[3].ID}
	want := []uuid.UUID{rootID, aID, a1ID, bID}
	require.Equal(t, want, got)

	// Depth is preserved from the source list; ordering is the responsibility of HierarchyToTree.
	require.Equal(t, 0, tree.Nodes[0].Depth)
	require.Equal(t, 1, tree.Nodes[1].Depth)
	require.Equal(t, 2, tree.Nodes[2].Depth)
	require.Equal(t, 1, tree.Nodes[3].Depth)
}

func TestHierarchyToTree_TreatsMissingParentAsRoot(t *testing.T) {
	missingParentID := uuid.MustParse("00000000-0000-0000-0000-000000000099")
	orphanID := uuid.MustParse("00000000-0000-0000-0000-000000000100")

	nodes := []services.HierarchyNode{
		{ID: orphanID, Name: "ORPHAN", DisplayOrder: 0, ParentID: &missingParentID, Depth: 1},
	}

	tree := HierarchyToTree(nodes, nil)
	require.NotNil(t, tree)
	require.Len(t, tree.Nodes, 1)
	require.Equal(t, orphanID, tree.Nodes[0].ID)
}
