package mappers

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func HierarchyToTree(nodes []services.HierarchyNode, selectedNodeID *uuid.UUID) *viewmodels.OrgTree {
	byID := make(map[uuid.UUID]services.HierarchyNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	childrenByParent := make(map[uuid.UUID][]services.HierarchyNode, len(nodes))
	for _, n := range nodes {
		parentID := uuid.Nil
		if n.ParentID != nil {
			parentID = *n.ParentID
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], n)
	}

	for parentID := range childrenByParent {
		siblings := childrenByParent[parentID]
		sort.SliceStable(siblings, func(i, j int) bool {
			if siblings[i].DisplayOrder != siblings[j].DisplayOrder {
				return siblings[i].DisplayOrder < siblings[j].DisplayOrder
			}
			ni := strings.TrimSpace(siblings[i].Name)
			nj := strings.TrimSpace(siblings[j].Name)
			if ni != nj {
				return ni < nj
			}
			ci := strings.TrimSpace(siblings[i].Code)
			cj := strings.TrimSpace(siblings[j].Code)
			if ci != cj {
				return ci < cj
			}
			return siblings[i].ID.String() < siblings[j].ID.String()
		})
		childrenByParent[parentID] = siblings
	}

	isRoot := func(n services.HierarchyNode) bool {
		if n.ParentID == nil || *n.ParentID == uuid.Nil {
			return true
		}
		_, ok := byID[*n.ParentID]
		return !ok
	}

	roots := make([]services.HierarchyNode, 0, 8)
	for _, n := range nodes {
		if isRoot(n) {
			roots = append(roots, n)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].DisplayOrder != roots[j].DisplayOrder {
			return roots[i].DisplayOrder < roots[j].DisplayOrder
		}
		ni := strings.TrimSpace(roots[i].Name)
		nj := strings.TrimSpace(roots[j].Name)
		if ni != nj {
			return ni < nj
		}
		ci := strings.TrimSpace(roots[i].Code)
		cj := strings.TrimSpace(roots[j].Code)
		if ci != cj {
			return ci < cj
		}
		return roots[i].ID.String() < roots[j].ID.String()
	})

	out := make([]viewmodels.OrgTreeNode, 0, len(nodes))
	visited := make(map[uuid.UUID]struct{}, len(nodes))
	var walk func(n services.HierarchyNode)
	walk = func(n services.HierarchyNode) {
		if _, ok := visited[n.ID]; ok {
			return
		}
		visited[n.ID] = struct{}{}

		selected := selectedNodeID != nil && *selectedNodeID == n.ID
		out = append(out, viewmodels.OrgTreeNode{
			ID:           n.ID,
			Code:         n.Code,
			Name:         n.Name,
			Depth:        n.Depth,
			DisplayOrder: n.DisplayOrder,
			Status:       n.Status,
			Selected:     selected,
		})

		for _, child := range childrenByParent[n.ID] {
			walk(child)
		}
	}

	for _, r := range roots {
		walk(r)
	}

	if len(visited) != len(byID) {
		remaining := make([]services.HierarchyNode, 0, len(byID)-len(visited))
		for _, n := range nodes {
			if _, ok := visited[n.ID]; ok {
				continue
			}
			remaining = append(remaining, n)
		}
		sort.SliceStable(remaining, func(i, j int) bool {
			if remaining[i].DisplayOrder != remaining[j].DisplayOrder {
				return remaining[i].DisplayOrder < remaining[j].DisplayOrder
			}
			ni := strings.TrimSpace(remaining[i].Name)
			nj := strings.TrimSpace(remaining[j].Name)
			if ni != nj {
				return ni < nj
			}
			ci := strings.TrimSpace(remaining[i].Code)
			cj := strings.TrimSpace(remaining[j].Code)
			if ci != cj {
				return ci < cj
			}
			return remaining[i].ID.String() < remaining[j].ID.String()
		})
		for _, n := range remaining {
			walk(n)
		}
	}

	return &viewmodels.OrgTree{Nodes: out}
}
