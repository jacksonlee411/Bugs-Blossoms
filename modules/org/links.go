package org

import (
	icons "github.com/iota-uz/icons/phosphor"

	"github.com/iota-uz/iota-sdk/pkg/types"
)

var OrgLink = types.NavigationItem{
	Name: "NavigationLinks.Org",
	Icon: icons.TreeStructure(icons.Props{Size: "20"}),
	Href: "#",
	Children: []types.NavigationItem{
		{
			Name:        "NavigationLinks.OrgStructure",
			Icon:        nil,
			Href:        "/org/nodes",
			Children:    nil,
			AuthzObject: "org.hierarchies",
			AuthzAction: "read",
		},
		{
			Name:        "NavigationLinks.OrgPositions",
			Icon:        nil,
			Href:        "/org/positions",
			Children:    nil,
			AuthzObject: "org.positions",
			AuthzAction: "read",
		},
		{
			Name:        "NavigationLinks.JobCatalog",
			Icon:        nil,
			Href:        "/org/job-catalog",
			Children:    nil,
			AuthzObject: "org.job_catalog",
			AuthzAction: "read",
		},
	},
}

var NavItems = []types.NavigationItem{
	OrgLink,
}
