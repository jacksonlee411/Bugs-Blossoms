package person

import (
	"embed"

	"github.com/iota-uz/iota-sdk/modules/person/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/person/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/person/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/spotlight"
)

//go:embed presentation/locales/*.json
var localeFiles embed.FS

//go:embed infrastructure/persistence/schema/person-schema.sql
var migrationFiles embed.FS

func NewModule() application.Module {
	return &Module{}
}

type Module struct{}

func (m *Module) Register(app application.Application) error {
	app.RegisterLocaleFiles(&localeFiles)
	app.Migrations().RegisterSchema(&migrationFiles)

	app.RegisterServices(
		services.NewPersonService(persistence.NewPersonRepository()),
	)

	app.RegisterControllers(
		controllers.NewPersonAPIController(app),
		controllers.NewPersonUIController(app),
	)

	app.QuickLinks().Add(
		spotlight.NewQuickLink(PersonsLink.Icon, PersonsLink.Name, PersonsLink.Href).
			RequireAuthz(PersonsLink.AuthzObject, PersonsLink.AuthzAction),
	)

	return nil
}

func (m *Module) Name() string {
	return "person"
}
