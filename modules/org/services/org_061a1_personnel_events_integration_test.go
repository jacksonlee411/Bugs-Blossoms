package services_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func applyAllOrgMigrationsFor061A1(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) {
	tb.Helper()

	applyAllOrgMigrationsFor058(tb, ctx, pool)

	sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "20251222120000_org_personnel_events.sql")))
	_, err := pool.Exec(ctx, sql)
	require.NoError(tb, err)
}

func TestOrg061A1_TerminationPersonnelEvent_EndsAllAssignmentsAndWritesEvent(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 061A1 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor061A1(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	personUUID := uuid.New()
	seedPerson(t, ctx, pool, tenantID, personUUID, "000123", "Test Person 000123")
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	cfg := configuration.Use()
	prev := cfg.EnableOrgExtendedAssignmentTypes
	cfg.EnableOrgExtendedAssignmentTypes = true
	t.Cleanup(func() { cfg.EnableOrgExtendedAssignmentTypes = prev })

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	orgNodeID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES ($1,$2,'OrgUnit','ROOT',true)
`, tenantID, orgNodeID)
	require.NoError(t, err)

	position1 := uuid.New()
	position2 := uuid.New()
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, position1, "POS-1", 1.0, asOf, endDate)
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, position2, "POS-2", 1.0, asOf, endDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)
	initiatorID := uuid.New()

	_, err = svc.CreateAssignment(reqCtx, tenantID, "req-primary", initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:          "000123",
		EffectiveDate:  asOf,
		AssignmentType: "primary",
		AllocatedFTE:   1.0,
		PositionID:     &position1,
	})
	require.NoError(t, err)
	_, err = svc.CreateAssignment(reqCtx, tenantID, "req-matrix", initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:          "000123",
		EffectiveDate:  asOf,
		AssignmentType: "matrix",
		AllocatedFTE:   1.0,
		PositionID:     &position2,
	})
	require.NoError(t, err)

	terminationDate := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	res, err := svc.TerminationPersonnelEvent(reqCtx, tenantID, "req-termination", initiatorID, orgsvc.TerminationPersonnelEventInput{
		Pernr:         "000123",
		EffectiveDate: terminationDate,
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "termination", res.Event.EventType)
	require.Equal(t, "legacy", res.Event.ReasonCode)

	var endedCount int
	err = pool.QueryRow(ctx, `
SELECT count(*)
FROM org_assignments
WHERE tenant_id = $1 AND subject_id = $2 AND end_date = $3
`, tenantID, personUUID, terminationDate).Scan(&endedCount)
	require.NoError(t, err)
	require.Equal(t, 2, endedCount)

	var terminatedIDsCount int
	err = pool.QueryRow(ctx, `
SELECT jsonb_array_length(payload->'terminated_assignment_ids')
FROM org_personnel_events
WHERE tenant_id = $1 AND request_id = $2
`, tenantID, "req-termination").Scan(&terminatedIDsCount)
	require.NoError(t, err)
	require.Equal(t, 2, terminatedIDsCount)
}
