package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/role"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	coreSession "github.com/iota-uz/iota-sdk/modules/core/domain/entities/session"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	coreseed "github.com/iota-uz/iota-sdk/modules/core/seed"
	coreservices "github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	personservices "github.com/iota-uz/iota-sdk/modules/person/services"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/aichatconfig"
	websiteseed "github.com/iota-uz/iota-sdk/modules/website/seed"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/commands/common"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

// Seed populates the e2e database with test data
func Seed() error {
	// Set environment variable for e2e database
	_ = os.Setenv("DB_NAME", E2E_DB_NAME)

	conf := configuration.Use()
	ctx := context.Background()

	pool, err := GetE2EPool()
	if err != nil {
		return fmt.Errorf("failed to connect to e2e database: %w", err)
	}
	defer pool.Close()

	app, err := common.NewApplication(pool, modules.BuiltInModules...)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	app.RegisterNavItems(modules.NavLinks...)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	seeder := application.NewSeeder()

	// Create test user
	usr, err := user.New(
		"Test",
		"User",
		internet.MustParseEmail("test@gmail.com"),
		user.UILanguageEN,
	).SetPassword("TestPass123!")
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	// Add default tenant to context
	defaultTenant := &composables.Tenant{
		ID:     uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:   "Default",
		Domain: "default.localhost",
	}

	allPermissions := defaults.AllPermissions()
	noPersonPermissions := filterOutPersonPermissions(allPermissions)
	seeder.Register(
		coreseed.CreateDefaultTenant,
		coreseed.CreateCurrencies,
		func(ctx context.Context, app application.Application) error {
			return coreseed.CreatePermissions(ctx, app, allPermissions)
		},
		coreseed.UserSeedFunc(usr, allPermissions),
		coreseed.UserSeedFunc(user.New(
			"AI",
			"User",
			internet.MustParseEmail("ai@llm.com"),
			user.UILanguageEN,
			user.WithTenantID(defaultTenant.ID),
		), allPermissions),
		createLimitedUserSeedFunc(noPersonPermissions),
		websiteseed.AIChatConfigSeedFunc(aichatconfig.MustNew(
			"gemma-12b-it",
			aichatconfig.AIModelTypeOpenAI,
			"https://llm2.eai.uz/v1",
			aichatconfig.WithTenantID(defaultTenant.ID),
		)),
	)

	ctxWithTenant := composables.WithTenantID(
		composables.WithTx(ctx, tx),
		defaultTenant.ID,
	)

	if err := seeder.Seed(ctxWithTenant, app); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("rollback failed: %w (original error: %w)", rollbackErr, err)
		}
		return err
	}

	if err := seedPersons(ctxWithTenant, app); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("rollback failed: %w (original error: %w)", rollbackErr, err)
		}
		return err
	}

	if err := tx.Commit(ctxWithTenant); err != nil {
		return err
	}

	conf.Logger().Info("Seeded e2e database with test data")
	return nil
}

func filterOutPersonPermissions(perms []*permission.Permission) []*permission.Permission {
	filtered := make([]*permission.Permission, 0, len(perms))
	skipPrefixes := []string{
		"person.",
		"logging.",
		"logs.",
		"log.",
		"action_logs.",
		"authentication_logs.",
		"audit_logs.",
		"audit.",
		"action-log.",
		"logging-",
		"log-",
		"audit-",
		"action-log-",
		"audit_log.",
		"audit_log-",
		"auditlog.",
		"auditlog-",
		"actionlogs.",
		"actionlogs-",
		"authenticationlogs.",
		"authenticationlogs-",
	}
	for _, p := range perms {
		shouldSkip := false
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(p.Name, prefix) {
				shouldSkip = true
				break
			}
		}
		if shouldSkip {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

func createLimitedUserSeedFunc(perms []*permission.Permission) application.SeedFunc {
	return func(ctx context.Context, app application.Application) error {
		tenantID, err := composables.UseTenantID(ctx)
		if err != nil {
			return err
		}

		roleRepo := persistence.NewRoleRepository()
		roles, err := roleRepo.GetPaginated(ctx, &role.FindParams{
			Filters: []role.Filter{
				{
					Column: role.NameField,
					Filter: repo.Eq("NoPerson"),
				},
			},
		})
		if err != nil {
			return err
		}

		limitedRole := role.New("NoPerson",
			role.WithDescription("User without Person permissions"),
			role.WithPermissions(perms),
			role.WithType(role.TypeSystem),
			role.WithTenantID(tenantID),
		)
		if len(roles) == 0 {
			if limitedRole, err = roleRepo.Create(ctx, limitedRole); err != nil {
				return err
			}
		} else {
			limitedRole = roles[0]
		}

		uploadRepo := persistence.NewUploadRepository()
		userRepo := persistence.NewUserRepository(uploadRepo)
		existing, err := userRepo.GetByEmail(ctx, "noperson@example.com")
		if err != nil && !errors.Is(err, persistence.ErrUserNotFound) {
			return err
		}
		if existing != nil {
			return nil
		}

		limitedUser, err := user.New(
			"NoPerson",
			"User",
			internet.MustParseEmail("noperson@example.com"),
			user.UILanguageEN,
			user.WithTenantID(tenantID),
		).SetPassword("TestPass123!")
		if err != nil {
			return err
		}

		_, err = userRepo.Create(ctx, limitedUser.AddRole(limitedRole))
		return err
	}
}

func seedPersons(ctx context.Context, app application.Application) error {
	service := app.Service(personservices.PersonService{}).(*personservices.PersonService)
	userService := app.Service(coreservices.UserService{}).(*coreservices.UserService)

	adminUser, err := userService.GetByEmail(ctx, "test@gmail.com")
	if err != nil {
		return fmt.Errorf("failed to load default user: %w", err)
	}

	ctx = composables.WithUser(ctx, adminUser)
	ctx = composables.WithSession(ctx, &coreSession.Session{
		Token:     "seed-session",
		UserID:    adminUser.ID(),
		TenantID:  adminUser.TenantID(),
		IP:        "127.0.0.1",
		UserAgent: "seed",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	})

	persons := []person.CreateDTO{
		{Pernr: "0001", DisplayName: "Ava Reed"},
		{Pernr: "0002", DisplayName: "Bruno Silva"},
		{Pernr: "0003", DisplayName: "Chloe Tanaka"},
	}

	for _, p := range persons {
		dto := p
		if _, err := service.Create(ctx, &dto); err != nil {
			if errors.Is(err, person.ErrPernrTaken) {
				continue
			}
			return fmt.Errorf("failed to create person pernr=%s: %w", dto.Pernr, err)
		}
	}

	return nil
}
