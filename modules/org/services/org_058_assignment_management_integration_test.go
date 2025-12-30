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

func applyAllOrgMigrationsFor058(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) {
	tb.Helper()

	applyAllPersonMigrations(tb, ctx, pool)

	files := []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251221090000_org_reason_code_mode.sql",
		"20251218150000_org_outbox.sql",
		"20251219090000_org_hierarchy_closure_and_snapshots.sql",
		"20251219195000_org_security_group_mappings_and_links.sql",
		"20251219220000_org_reporting_nodes_and_view.sql",
		"20251220160000_org_position_slices_and_fte.sql",
		"20251220200000_org_job_catalog_profiles_and_validation_modes.sql",
		"20251222120000_org_personnel_events.sql",
		"20251227090000_org_valid_time_day_granularity.sql",
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}
}

func seedOrgPosition(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID, orgNodeID, positionID uuid.UUID, code string, capacityFTE float64, effectiveDate, endDate time.Time) {
	tb.Helper()

	jobProfileID, jobFamilyID := ensureTestJobProfileWith100PercentFamily(tb, ctx, pool, tenantID)
	sliceID := uuid.New()

	_, err := pool.Exec(ctx, `
		INSERT INTO org_positions (tenant_id, id, org_node_id, code, status, is_auto_created, effective_date, end_date)
		VALUES (
			$1,$2,$3,$4,'active',false,
		($5 AT TIME ZONE 'UTC')::date,
		($6 AT TIME ZONE 'UTC')::date
	)
	`, tenantID, positionID, orgNodeID, code, effectiveDate, endDate)
	require.NoError(tb, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO org_position_slices (tenant_id, id, position_id, org_node_id, lifecycle_status, capacity_fte, job_profile_id, effective_date, end_date)
		VALUES (
			$1,$2,$3,$4,'active',$5::numeric(9,2),$6,
			($7 AT TIME ZONE 'UTC')::date,
			($8 AT TIME ZONE 'UTC')::date
		)
		`, tenantID, sliceID, positionID, orgNodeID, capacityFTE, jobProfileID, effectiveDate, endDate)
	require.NoError(tb, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, allocation_percent, is_primary)
		VALUES ($1,$2,$3,100,true)
		ON CONFLICT (tenant_id, position_slice_id, job_family_id) DO UPDATE
		SET allocation_percent=excluded.allocation_percent, is_primary=excluded.is_primary
	`, tenantID, sliceID, jobFamilyID)
	require.NoError(tb, err)
}

func ensureTestJobProfileWith100PercentFamily(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) (uuid.UUID, uuid.UUID) {
	tb.Helper()

	var groupID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO org_job_family_groups (tenant_id, code, name, is_active)
		VALUES ($1,'SEED-GROUP','Seed Group',true)
		ON CONFLICT (tenant_id, code) DO UPDATE
		SET name=excluded.name, is_active=excluded.is_active
		RETURNING id
	`, tenantID).Scan(&groupID)
	require.NoError(tb, err)

	var familyID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO org_job_families (tenant_id, job_family_group_id, code, name, is_active)
		VALUES ($1,$2,'SEED-FAMILY','Seed Family',true)
		ON CONFLICT (tenant_id, job_family_group_id, code) DO UPDATE
		SET name=excluded.name, is_active=excluded.is_active
		RETURNING id
	`, tenantID, groupID).Scan(&familyID)
	require.NoError(tb, err)

	var profileID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO org_job_profiles (tenant_id, code, name, description, is_active)
		VALUES ($1,'SEED-PROFILE','Seed Profile',NULL,true)
		ON CONFLICT (tenant_id, code) DO UPDATE
		SET name=excluded.name, description=excluded.description, is_active=excluded.is_active
		RETURNING id
	`, tenantID).Scan(&profileID)
	require.NoError(tb, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO org_job_profile_job_families (tenant_id, job_profile_id, job_family_id, allocation_percent, is_primary)
		VALUES ($1,$2,$3,100,true)
		ON CONFLICT (tenant_id, job_profile_id, job_family_id) DO UPDATE
		SET allocation_percent=excluded.allocation_percent, is_primary=excluded.is_primary
	`, tenantID, profileID, familyID)
	require.NoError(tb, err)

	return profileID, familyID
}

func TestOrg058_ExtendedAssignmentTypes_NonPrimaryDoesNotAffectCapacity(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 058 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)
	seedPerson(t, ctx, pool, tenantID, uuid.New(), "000123", "Test Person 000123")
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
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

	positionID := uuid.New()
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, positionID, "POS-1", 1.0, asOf, endDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	primary, err := svc.CreateAssignment(reqCtx, tenantID, "req-primary", uuid.New(), orgsvc.CreateAssignmentInput{
		Pernr:          "000123",
		EffectiveDate:  asOf,
		AssignmentType: "primary",
		AllocatedFTE:   1.0,
		PositionID:     &positionID,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, primary.AssignmentID)

	matrix, err := svc.CreateAssignment(reqCtx, tenantID, "req-matrix", uuid.New(), orgsvc.CreateAssignmentInput{
		Pernr:          "000123",
		EffectiveDate:  asOf,
		AssignmentType: "matrix",
		AllocatedFTE:   1.0,
		PositionID:     &positionID,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, matrix.AssignmentID)
	require.NotEqual(t, primary.AssignmentID, matrix.AssignmentID)

	var isPrimary bool
	err = pool.QueryRow(ctx, `SELECT is_primary FROM org_assignments WHERE tenant_id=$1 AND id=$2`, tenantID, matrix.AssignmentID).Scan(&isPrimary)
	require.NoError(t, err)
	require.False(t, isPrimary)

	var occupied float64
	err = pool.QueryRow(ctx, `
SELECT COALESCE(SUM(allocated_fte),0)::float8
FROM org_assignments
WHERE tenant_id=$1 AND position_id=$2 AND assignment_type='primary' AND effective_date <= $3 AND end_date > $3
`, tenantID, positionID, asOf).Scan(&occupied)
	require.NoError(t, err)
	require.InDelta(t, 1.0, occupied, 0.0001)
}

func TestOrg058_AssignmentUpdate_TransferAndMultiSegments(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 058 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)
	seedPerson(t, ctx, pool, tenantID, uuid.New(), "000123", "Test Person 000123")
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(t, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	orgNodeID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES ($1,$2,'OrgUnit','ROOT',true)
`, tenantID, orgNodeID)
	require.NoError(t, err)

	posA := uuid.New()
	posB := uuid.New()
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, posA, "POS-A", 1.0, asOf, endDate)
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, posB, "POS-B", 1.0, asOf, endDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	created, err := svc.CreateAssignment(reqCtx, tenantID, "req-create", uuid.New(), orgsvc.CreateAssignmentInput{
		Pernr:         "000123",
		EffectiveDate: asOf,
		PositionID:    &posA,
		AllocatedFTE:  1.0,
	})
	require.NoError(t, err)

	updated1, err := svc.UpdateAssignment(reqCtx, tenantID, "req-update-1", uuid.New(), orgsvc.UpdateAssignmentInput{
		AssignmentID:  created.AssignmentID,
		EffectiveDate: d2,
		ReasonCode:    "transfer",
		PositionID:    &posB,
		AllocatedFTE:  ptrFloat64(1.0),
	})
	require.NoError(t, err)

	updated2, err := svc.UpdateAssignment(reqCtx, tenantID, "req-update-2", uuid.New(), orgsvc.UpdateAssignmentInput{
		AssignmentID:  updated1.AssignmentID,
		EffectiveDate: d3,
		ReasonCode:    "transfer",
		PositionID:    &posA,
		AllocatedFTE:  ptrFloat64(1.0),
	})
	require.NoError(t, err)
	require.NotEqual(t, created.AssignmentID, updated1.AssignmentID)
	require.NotEqual(t, updated1.AssignmentID, updated2.AssignmentID)

	_, timeline, _, err := svc.GetAssignments(reqCtx, tenantID, "person:000123", nil)
	require.NoError(t, err)
	require.Len(t, timeline, 3)

	require.Equal(t, posA, timeline[0].PositionID)
	require.True(t, timeline[0].EffectiveDate.UTC().Equal(asOf))
	require.True(t, timeline[0].EndDate.UTC().Equal(d2.AddDate(0, 0, -1)))

	require.Equal(t, posB, timeline[1].PositionID)
	require.True(t, timeline[1].EffectiveDate.UTC().Equal(d2))
	require.True(t, timeline[1].EndDate.UTC().Equal(d3.AddDate(0, 0, -1)))

	require.Equal(t, posA, timeline[2].PositionID)
	require.True(t, timeline[2].EffectiveDate.UTC().Equal(d3))
	require.True(t, timeline[2].EndDate.UTC().Equal(endDate))

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM org_outbox WHERE tenant_id=$1`, tenantID).Scan(&count)
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, 3)
}

func TestOrg058_AssignmentCorrectAndRescind_WritesAuditAndOutbox(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 058 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)
	seedPerson(t, ctx, pool, tenantID, uuid.New(), "000123", "Test Person 000123")
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(t, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	orgNodeID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES ($1,$2,'OrgUnit','ROOT',true)
`, tenantID, orgNodeID)
	require.NoError(t, err)

	posA := uuid.New()
	posB := uuid.New()
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, posA, "POS-A", 1.0, asOf, endDate)
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, posB, "POS-B", 1.0, asOf, endDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	created, err := svc.CreateAssignment(reqCtx, tenantID, "req-create", uuid.New(), orgsvc.CreateAssignmentInput{
		Pernr:         "000123",
		EffectiveDate: asOf,
		ReasonCode:    "create",
		PositionID:    &posA,
		AllocatedFTE:  1.0,
	})
	require.NoError(t, err)

	note := "fix position"
	_, err = svc.CorrectAssignment(reqCtx, tenantID, "req-correct", uuid.New(), orgsvc.CorrectAssignmentInput{
		AssignmentID: created.AssignmentID,
		ReasonCode:   "correct",
		ReasonNote:   &note,
		PositionID:   &posB,
	})
	require.NoError(t, err)

	var correctedPos uuid.UUID
	err = pool.QueryRow(ctx, `SELECT position_id FROM org_assignments WHERE tenant_id=$1 AND id=$2`, tenantID, created.AssignmentID).Scan(&correctedPos)
	require.NoError(t, err)
	require.Equal(t, posB, correctedPos)

	_, err = svc.RescindAssignment(reqCtx, tenantID, "req-rescind", uuid.New(), orgsvc.RescindAssignmentInput{
		AssignmentID:  created.AssignmentID,
		EffectiveDate: d2,
		ReasonCode:    "rescind",
		ReasonNote:    &note,
	})
	require.NoError(t, err)

	var newEnd time.Time
	err = pool.QueryRow(ctx, `SELECT end_date FROM org_assignments WHERE tenant_id=$1 AND id=$2`, tenantID, created.AssignmentID).Scan(&newEnd)
	require.NoError(t, err)
	require.True(t, newEnd.UTC().Equal(d2.AddDate(0, 0, -1)))

	var createReason string
	err = pool.QueryRow(ctx, `SELECT meta->>'reason_code' FROM org_audit_logs WHERE tenant_id=$1 AND request_id=$2`, tenantID, "req-create").Scan(&createReason)
	require.NoError(t, err)
	require.Equal(t, "create", createReason)

	var correctReason string
	var correctNote string
	err = pool.QueryRow(ctx, `SELECT meta->>'reason_code', meta->>'reason_note' FROM org_audit_logs WHERE tenant_id=$1 AND request_id=$2`, tenantID, "req-correct").Scan(&correctReason, &correctNote)
	require.NoError(t, err)
	require.Equal(t, "correct", correctReason)
	require.Equal(t, note, correctNote)

	var rescindReason string
	var rescindNote string
	err = pool.QueryRow(ctx, `SELECT meta->>'reason_code', meta->>'reason_note' FROM org_audit_logs WHERE tenant_id=$1 AND request_id=$2`, tenantID, "req-rescind").Scan(&rescindReason, &rescindNote)
	require.NoError(t, err)
	require.Equal(t, "rescind", rescindReason)
	require.Equal(t, note, rescindNote)

	var outboxCorrected int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM org_outbox WHERE tenant_id=$1 AND payload->>'change_type'='assignment.corrected'`, tenantID).Scan(&outboxCorrected)
	require.NoError(t, err)
	require.GreaterOrEqual(t, outboxCorrected, 1)

	var outboxRescinded int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM org_outbox WHERE tenant_id=$1 AND payload->>'change_type'='assignment.rescinded'`, tenantID).Scan(&outboxRescinded)
	require.NoError(t, err)
	require.GreaterOrEqual(t, outboxRescinded, 1)
}

func ptrFloat64(v float64) *float64 { return &v }
