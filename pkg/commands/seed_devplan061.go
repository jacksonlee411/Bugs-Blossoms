package commands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/modules"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	coreseed "github.com/iota-uz/iota-sdk/modules/core/seed"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	personservices "github.com/iota-uz/iota-sdk/modules/person/services"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/aichatconfig"
	websiteseed "github.com/iota-uz/iota-sdk/modules/website/seed"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/commands/common"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
)

func SeedDevPlan061(mods ...application.Module) error {
	conf := configuration.Use()
	ctx := context.Background()

	app, pool, err := common.NewApplicationWithDefaults(mods...)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}
	defer pool.Close()

	app.RegisterNavItems(modules.NavLinks...)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seeder := application.NewSeeder()

	defaultTenant := &composables.Tenant{
		ID:     uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:   "Default",
		Domain: "default.localhost",
	}

	usr, err := user.New(
		"Test",
		"User",
		internet.MustParseEmail("test@gmail.com"),
		user.UILanguageEN,
	).SetPassword("TestPass123!")
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	allPermissions := defaults.AllPermissions()
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
		return fmt.Errorf("failed to seed base dataset: %w", err)
	}

	if err := ensureOrg061Tables(ctxWithTenant); err != nil {
		return err
	}

	if err := seedOrgPositionPersonEvents061(ctxWithTenant, app, defaultTenant.ID); err != nil {
		return fmt.Errorf("failed to seed DEV-PLAN-061 dataset: %w", err)
	}

	if err := tx.Commit(ctxWithTenant); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	conf.Logger().Info("DEV-PLAN-061 dataset seeded successfully!")
	return nil
}

func ensureOrg061Tables(ctx context.Context) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	required := []string{
		"public.org_nodes",
		"public.org_assignments",
		"public.org_personnel_events",
	}

	for _, table := range required {
		var name *string
		if err := tx.QueryRow(ctx, `SELECT to_regclass($1)`, table).Scan(&name); err != nil {
			return err
		}
		if name == nil || *name == "" {
			return fmt.Errorf("missing table %s (请先运行 `make org migrate up`)", table)
		}
	}

	return nil
}

func seedOrgPositionPersonEvents061(ctx context.Context, app application.Application, tenantID uuid.UUID) error {
	org := app.Service(services.OrgService{}).(*services.OrgService)
	personSvc := app.Service(personservices.PersonService{}).(*personservices.PersonService)

	initiatorID := uuid.NewSHA1(uuid.MustParse("11a13257-665c-4d78-b7fa-4a27be33f9af"), []byte("seed_061"))

	now := time.Now().UTC()
	baseEffective := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	rootID, err := ensureOrgRootNode061(ctx, org, tenantID, initiatorID, baseEffective)
	if err != nil {
		return err
	}

	deptCodes := []struct {
		Code string
		Name string
	}{
		{Code: "D061-ENG", Name: "Engineering"},
		{Code: "D061-HR", Name: "Human Resources"},
		{Code: "D061-SALES", Name: "Sales"},
		{Code: "D061-OPS", Name: "Operations"},
	}

	deptIDs := make([]uuid.UUID, 0, len(deptCodes))
	for _, d := range deptCodes {
		id, err := ensureOrgNodeByCode(ctx, org, tenantID, initiatorID, d.Code, d.Name, &rootID, baseEffective)
		if err != nil {
			return err
		}
		deptIDs = append(deptIDs, id)
	}

	people := []struct {
		Pernr       string
		DisplayName string
	}{
		{Pernr: "061001", DisplayName: "Ava Reed"},
		{Pernr: "061002", DisplayName: "Bruno Silva"},
		{Pernr: "061003", DisplayName: "Chloe Tanaka"},
		{Pernr: "061004", DisplayName: "Diego Alvarez"},
		{Pernr: "061005", DisplayName: "Elena Petrova"},
		{Pernr: "061006", DisplayName: "Farah Khan"},
		{Pernr: "061007", DisplayName: "Gabriel Martin"},
		{Pernr: "061008", DisplayName: "Hana Suzuki"},
		{Pernr: "061009", DisplayName: "Ibrahim Noor"},
		{Pernr: "061010", DisplayName: "Jia Wei"},
		{Pernr: "061011", DisplayName: "Kira Novak"},
		{Pernr: "061012", DisplayName: "Liam O'Connor"},
		{Pernr: "061013", DisplayName: "Mina Park"},
		{Pernr: "061014", DisplayName: "Noah Johnson"},
		{Pernr: "061015", DisplayName: "Olivia Rossi"},
		{Pernr: "061016", DisplayName: "Pavel Smirnov"},
		{Pernr: "061017", DisplayName: "Quinn Murphy"},
		{Pernr: "061018", DisplayName: "Rina Sato"},
		{Pernr: "061019", DisplayName: "Santiago Perez"},
		{Pernr: "061020", DisplayName: "Tara Williams"},
	}

	for i, p := range people {
		dto := person.CreateDTO{
			Pernr:       p.Pernr,
			DisplayName: p.DisplayName,
		}
		if _, err := personSvc.Create(ctx, &dto); err != nil {
			if errors.Is(err, person.ErrPernrTaken) {
				// continue to events so the seed is re-runnable
			} else {
				return fmt.Errorf("create person pernr=%s: %w", p.Pernr, err)
			}
		}

		hireDate := baseEffective.AddDate(0, 0, i)
		orgNodeID := deptIDs[i%len(deptIDs)]

		if _, err := org.HirePersonnelEvent(ctx, tenantID, seedRequestID("hire", p.Pernr), initiatorID, services.HirePersonnelEventInput{
			Pernr:         p.Pernr,
			OrgNodeID:     orgNodeID,
			PositionID:    nil,
			EffectiveDate: hireDate,
			AllocatedFTE:  1.0,
			ReasonCode:    "seed_hire",
		}); err != nil {
			return fmt.Errorf("hire personnel event pernr=%s: %w", p.Pernr, err)
		}
	}

	for _, pernr := range []string{"061017", "061018", "061019"} {
		transferDate := baseEffective.AddDate(0, 0, 45)
		if _, err := org.TransferPersonnelEvent(ctx, tenantID, seedRequestID("transfer", pernr), initiatorID, services.TransferPersonnelEventInput{
			Pernr:         pernr,
			OrgNodeID:     deptIDs[(len(deptIDs)-1)%len(deptIDs)],
			PositionID:    nil,
			EffectiveDate: transferDate,
			AllocatedFTE:  1.0,
			ReasonCode:    "seed_transfer",
		}); err != nil {
			return fmt.Errorf("transfer personnel event pernr=%s: %w", pernr, err)
		}
	}

	terminationDate := baseEffective.AddDate(0, 0, 75)
	if _, err := org.TerminationPersonnelEvent(ctx, tenantID, seedRequestID("termination", "061020"), initiatorID, services.TerminationPersonnelEventInput{
		Pernr:         "061020",
		EffectiveDate: terminationDate,
		ReasonCode:    "seed_termination",
	}); err != nil {
		return fmt.Errorf("termination personnel event pernr=%s: %w", "061020", err)
	}

	return nil
}

func seedRequestID(kind, pernr string) string {
	return fmt.Sprintf("seed_061:%s:%s", kind, pernr)
}

func ensureOrgRootNode061(ctx context.Context, org *services.OrgService, tenantID uuid.UUID, initiatorID uuid.UUID, effectiveDate time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	const code = "D061-ROOT"

	var existing uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT id
FROM org_nodes
WHERE tenant_id=$1 AND code=$2
LIMIT 1
`, tenantID, code).Scan(&existing); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	var parentID *uuid.UUID
	var currentRoot uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT id
FROM org_nodes
WHERE tenant_id=$1 AND is_root=true
ORDER BY id
LIMIT 1
`, tenantID).Scan(&currentRoot); err == nil {
		parentID = &currentRoot
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	res, err := org.CreateNode(ctx, tenantID, seedRequestID("node", code), initiatorID, services.CreateNodeInput{
		Code:          code,
		Name:          "DEV-PLAN-061 Root",
		ParentID:      parentID,
		EffectiveDate: effectiveDate,
		Status:        "active",
		DisplayOrder:  0,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return res.NodeID, nil
}

func ensureOrgNodeByCode(ctx context.Context, org *services.OrgService, tenantID uuid.UUID, initiatorID uuid.UUID, code, name string, parentID *uuid.UUID, effectiveDate time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var existing uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT id
FROM org_nodes
WHERE tenant_id=$1 AND code=$2
LIMIT 1
`, tenantID, code).Scan(&existing); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	res, err := org.CreateNode(ctx, tenantID, seedRequestID("node", code), initiatorID, services.CreateNodeInput{
		Code:          code,
		Name:          name,
		ParentID:      parentID,
		EffectiveDate: effectiveDate,
		Status:        "active",
		DisplayOrder:  0,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return res.NodeID, nil
}
