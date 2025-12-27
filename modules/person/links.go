package person

import (
	icons "github.com/iota-uz/icons/phosphor"

	"github.com/iota-uz/iota-sdk/pkg/types"
)

var PersonsLink = types.NavigationItem{
	Name:        "NavigationLinks.Persons",
	Icon:        nil,
	Href:        "/person/persons",
	AuthzObject: "person.persons",
	AuthzAction: "list",
	Children:    nil,
}

var JobDataLink = types.NavigationItem{
	Name:        "NavigationLinks.JobData",
	Icon:        nil,
	Href:        "/org/assignments",
	AuthzObject: "org.assignments",
	AuthzAction: "read",
	Children:    nil,
}

var PersonLink = types.NavigationItem{
	Name:        "NavigationLinks.Person",
	Icon:        icons.Users(icons.Props{Size: "20"}),
	Href:        "/person",
	AuthzObject: "person.persons",
	AuthzAction: "list",
	Children: []types.NavigationItem{
		PersonsLink,
		JobDataLink,
	},
}

var NavItems = []types.NavigationItem{
	PersonLink,
}
