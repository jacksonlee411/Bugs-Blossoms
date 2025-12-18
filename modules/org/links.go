package org

import (
	icons "github.com/iota-uz/icons/phosphor"

	"github.com/iota-uz/iota-sdk/pkg/types"
)

var OrgLink = types.NavigationItem{
	Name:        "NavigationLinks.Org",
	Icon:        icons.TreeStructure(icons.Props{Size: "20"}),
	Href:        "/org/nodes",
	Children:    nil,
	AuthzObject: "org.hierarchies",
	AuthzAction: "read",
}

var NavItems = []types.NavigationItem{
	OrgLink,
}
