package viewmodels

import "github.com/google/uuid"

type OrgTreeNode struct {
	ID           uuid.UUID
	Code         string
	Name         string
	Depth        int
	DisplayOrder int
	Status       string
	Selected     bool
}

type OrgTree struct {
	Nodes []OrgTreeNode
}
