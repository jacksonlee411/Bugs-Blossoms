package mappers

import (
	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func HierarchyToTree(nodes []services.HierarchyNode, selectedNodeID *uuid.UUID) *viewmodels.OrgTree {
	out := make([]viewmodels.OrgTreeNode, 0, len(nodes))
	for _, n := range nodes {
		selected := false
		if selectedNodeID != nil && *selectedNodeID == n.ID {
			selected = true
		}
		out = append(out, viewmodels.OrgTreeNode{
			ID:           n.ID,
			Code:         n.Code,
			Name:         n.Name,
			Depth:        n.Depth,
			DisplayOrder: n.DisplayOrder,
			Status:       n.Status,
			Selected:     selected,
		})
	}
	return &viewmodels.OrgTree{Nodes: out}
}
