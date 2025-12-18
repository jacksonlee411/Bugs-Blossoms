package org

import (
	"embed"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/org/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
)

//go:embed presentation/locales/*.json
var localeFiles embed.FS

//go:embed infrastructure/persistence/schema/org-schema.sql
var migrationFiles embed.FS

func NewModule() application.Module {
	return &Module{}
}

type Module struct{}

func (m *Module) Register(app application.Application) error {
	app.RegisterLocaleFiles(&localeFiles)
	app.Migrations().RegisterSchema(&migrationFiles)

	app.RegisterServices(
		services.NewChangeRequestService(persistence.NewChangeRequestRepository()),
		services.NewOrgService(
			persistence.NewOrgRepository(),
		),
	)

	app.RegisterControllers(
		controllers.NewOrgAPIController(app),
		controllers.NewOrgUIController(app),
	)

	return nil
}

func (m *Module) Name() string {
	return "org"
}
