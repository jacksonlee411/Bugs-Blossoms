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
	"github.com/iota-uz/iota-sdk/modules/hrm/domain/aggregates/employee"
	hrmservices "github.com/iota-uz/iota-sdk/modules/hrm/services"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/aichatconfig"
	websiteseed "github.com/iota-uz/iota-sdk/modules/website/seed"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/commands/common"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	"github.com/iota-uz/iota-sdk/pkg/repo"
	"github.com/iota-uz/iota-sdk/pkg/shared"
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
	noHRMPermissions := filterOutHRMPermissions(allPermissions)
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
		createLimitedUserSeedFunc(noHRMPermissions),
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

	if err := seedEmployees(ctxWithTenant, app); err != nil {
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

func filterOutHRMPermissions(perms []*permission.Permission) []*permission.Permission {
	filtered := make([]*permission.Permission, 0, len(perms))
	for _, p := range perms {
		if !strings.HasPrefix(p.Name, "hrm.") {
			filtered = append(filtered, p)
		}
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
					Filter: repo.Eq("NoHRM"),
				},
			},
		})
		if err != nil {
			return err
		}

		limitedRole := role.New(
			"NoHRM",
			role.WithDescription("User without HRM permissions"),
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
		existing, err := userRepo.GetByEmail(ctx, "nohrm@example.com")
		if err != nil && !errors.Is(err, persistence.ErrUserNotFound) {
			return err
		}
		if existing != nil {
			return nil
		}

		limitedUser, err := user.New(
			"NoHRM",
			"User",
			internet.MustParseEmail("nohrm@example.com"),
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

func seedEmployees(ctx context.Context, app application.Application) error {
	service := app.Service(hrmservices.EmployeeService{}).(*hrmservices.EmployeeService)
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

	employees := []employee.CreateDTO{
		{
			FirstName:         "Ava",
			LastName:          "Reed",
			MiddleName:        "Marie",
			Email:             "ava.reed@example.com",
			Phone:             "+14155550001",
			Salary:            5200.00,
			PrimaryLanguage:   "en",
			SecondaryLanguage: "ru",
			BirthDate:         toDateOnly(1990, time.March, 5),
			HireDate:          toDateOnly(2023, time.January, 10),
			Notes:             "HR operations lead for onboarding workflows.",
		},
		{
			FirstName:         "Bruno",
			LastName:          "Silva",
			MiddleName:        "Henrique",
			Email:             "bruno.silva@example.com",
			Phone:             "+14155550002",
			Salary:            6100.00,
			PrimaryLanguage:   "pt",
			SecondaryLanguage: "en",
			BirthDate:         toDateOnly(1988, time.June, 17),
			HireDate:          toDateOnly(2022, time.September, 2),
			Notes:             "Payroll specialist focusing on LATAM compliance.",
		},
		{
			FirstName:         "Chloe",
			LastName:          "Tanaka",
			MiddleName:        "Aiko",
			Email:             "chloe.tanaka@example.com",
			Phone:             "+14155550003",
			Salary:            5700.00,
			PrimaryLanguage:   "ja",
			SecondaryLanguage: "en",
			BirthDate:         toDateOnly(1993, time.November, 28),
			HireDate:          toDateOnly(2023, time.April, 14),
			Notes:             "People partner coordinating APAC reviews.",
		},
	}

	for _, emp := range employees {
		dto := emp
		if err := service.Create(ctx, &dto); err != nil {
			return fmt.Errorf("failed to create employee %s: %w", dto.Email, err)
		}
	}

	return nil
}

func toDateOnly(year int, month time.Month, day int) shared.DateOnly {
	return shared.DateOnly(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}
