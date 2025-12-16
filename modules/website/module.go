package website

import (
	"embed"

	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	corePersistence "github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/website/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/website/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/website/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/redis/go-redis/v9"
)

//go:embed presentation/locales/*.json
var LocaleFiles embed.FS

//go:embed infrastructure/persistence/schema/website-schema.sql
var MigrationFiles embed.FS

func NewModule() application.Module {
	return &Module{}
}

type Module struct {
}

func (m *Module) Register(app application.Application) error {
	conf := configuration.Use()
	userRepo := corePersistence.NewUserRepository(
		corePersistence.NewUploadRepository(),
	)
	threadRepo := persistence.NewThreadRepository(redis.NewClient(&redis.Options{Addr: conf.RedisURL}))
	aiconfigRepo := persistence.NewAIChatConfigRepository()
	app.RegisterServices(
		services.NewAIChatConfigService(aiconfigRepo),
		services.NewWebsiteChatService(
			services.WebsiteChatServiceConfig{
				AIConfigRepo: aiconfigRepo,
				UserRepo:     userRepo,
				ThreadRepo:   threadRepo,
				AIUserEmail:  internet.MustParseEmail("ai@llm.com"),
			},
		),
	)
	app.RegisterControllers(
		controllers.NewAIChatController(controllers.AIChatControllerConfig{
			BasePath: "/website/ai-chat",
			App:      app,
		}),
		controllers.NewAIChatAPIController(controllers.AIChatAPIControllerConfig{
			BasePath:   "/api/v1/website/ai-chat",
			AliasPaths: []string{"/api/website/ai-chat"},
			App:        app,
		}),
	)
	app.RegisterLocaleFiles(&LocaleFiles)
	app.Migrations().RegisterSchema(&MigrationFiles)
	return nil
}

func (m *Module) Name() string {
	return "website"
}
