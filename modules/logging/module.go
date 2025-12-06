package logging

import (
	"embed"

	"github.com/iota-uz/iota-sdk/modules/logging/handlers"
	"github.com/iota-uz/iota-sdk/modules/logging/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/logging/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/spotlight"
)

//go:embed presentation/locales/*.json
var localeFiles embed.FS

//go:embed infrastructure/persistence/schema/logging-schema.sql
var migrationFiles embed.FS

func NewModule() application.Module {
	return &Module{}
}

type Module struct {
}

func (m *Module) Register(app application.Application) error {
	app.RegisterLocaleFiles(&localeFiles)
	app.Migrations().RegisterSchema(&migrationFiles)
	app.RegisterServices(
		services.NewLogsService(
			persistence.NewAuthenticationLogRepository(),
			persistence.NewActionLogRepository(),
		),
	)
	app.RegisterControllers(
		controllers.NewLogsController(app),
	)
	app.QuickLinks().Add(
		spotlight.NewQuickLink(LogsLink.Icon, LogsLink.Name, LogsLink.Href).
			RequireAuthz(LogsLink.AuthzObject, LogsLink.AuthzAction),
	)
	handlers.RegisterSessionEventHandlers(app)
	app.RegisterMiddleware(handlers.ActionLogMiddleware(app))
	return nil
}

func (m *Module) Name() string {
	return "logging"
}
