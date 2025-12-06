package logging

import (
	icons "github.com/iota-uz/icons/phosphor"

	"github.com/iota-uz/iota-sdk/pkg/types"
)

var LogsLink = types.NavigationItem{
	Name:        "NavigationLinks.Logs",
	Icon:        icons.List(icons.Props{Size: "20"}),
	Href:        "/logs",
	AuthzObject: "logging.logs",
	AuthzAction: "view",
	Children:    nil,
}

var NavItems = []types.NavigationItem{
	LogsLink,
}
