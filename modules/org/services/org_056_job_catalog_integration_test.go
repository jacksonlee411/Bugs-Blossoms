package services_test

import (
	"context"
	"fmt"
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
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func setupOrg056DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, uuid.UUID, time.Time, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 056 integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, uuid.Nil, time.Time{}, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	applyAllPersonMigrations(tb, ctx, pool)

	migrations := []string{
		"00001_org_baseline.sql",
		"00002_org_migration_smoke.sql",
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
		"20251231120000_org_remove_job_family_allocation_percent.sql",
		"20260101020855_org_job_catalog_effective_dated_slices_phase_a.sql",
		"20260101020930_org_job_catalog_effective_dated_slices_gates_and_backfill.sql",
		"20260104100000_org_drop_job_profile_job_families_legacy.sql",
		"20260104120000_org_drop_job_catalog_identity_legacy_columns.sql",
	}
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000056")
	ensureTenant(tb, ctx, pool, tenantID)
	seedPerson(tb, ctx, pool, tenantID, uuid.New(), "00000001", "Test Person 00000001")
	_, err := pool.Exec(ctx, `
	INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
	VALUES ($1,'disabled',0)
	ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
	`, tenantID)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(tb, ctx, pool, tenantID, 2, asOf, "deep", 56)
	nodes := buildPerfNodes(tb, tenantID, 2, "deep", 56)
	rootID := nodes[0].ID

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, rootID, asOf, svc
}

func TestOrg056CatalogValidation_ShadowWritesButAudits(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	jobProfileID := seedOrg056JobProfile(t, ctx, tenantID, svc, asOf, true)

	initiatorID := uuid.New()
	res, err := svc.CreatePosition(ctx, tenantID, "req-056-shadow", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-SHADOW",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		ReasonCode:     "create",
	})
	require.NoError(t, err)

	var code string
	err = pool.QueryRow(ctx, `
	SELECT meta->>'position_catalog_validation_error_code'
		FROM org_audit_logs
		WHERE tenant_id=$1 AND request_id=$2 AND entity_id=$3
		`, tenantID, "req-056-shadow", res.PositionID).Scan(&code)
	require.NoError(t, err)
	require.Equal(t, "ORG_JOB_LEVEL_NOT_FOUND", code)
}

func TestOrg056CatalogValidation_EnforceBlocksInvalidCodes(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	jobProfileID := seedOrg056JobProfile(t, ctx, tenantID, svc, asOf, true)

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_catalog_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	initiatorID := uuid.New()
	_, err = svc.CreatePosition(ctx, tenantID, "req-056-enforce", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-ENFORCE",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		ReasonCode:     "create",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_JOB_LEVEL_NOT_FOUND", svcErr.Code)
}

func TestOrg056JobProfileInactive_EnforceBlocks(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_catalog_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	jobProfileID := seedOrg056JobProfile(t, ctx, tenantID, svc, asOf, false)

	initiatorID := uuid.New()
	_, err = svc.CreatePosition(ctx, tenantID, "req-056-profile-inactive", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-PROFILE-INACTIVE",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		CapacityFTE:    1.0,
		ReasonCode:     "create",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_JOB_PROFILE_INACTIVE", svcErr.Code)
}

func TestOrg056SetPositionRestrictions_RejectsMismatch(t *testing.T) {
	ctx, _, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	jobProfileID := seedOrg056JobProfile(t, ctx, tenantID, svc, asOf, true)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-056-pos", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-R",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		CapacityFTE:    1.0,
		ReasonCode:     "create",
	})
	require.NoError(t, err)

	other := uuid.New()
	restrictions := []byte(fmt.Sprintf(`{"allowed_job_profile_ids":["%s"]}`, other.String()))
	_, err = svc.SetPositionRestrictions(ctx, tenantID, "req-056-restrictions", initiatorID, orgsvc.SetPositionRestrictionsInput{
		PositionID:           pos.PositionID,
		EffectiveDate:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		PositionRestrictions: restrictions,
		ReasonCode:           "restrictions_update",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_POSITION_RESTRICTIONS_PROFILE_MISMATCH", svcErr.Code)
}

func TestOrg056AssignmentRestrictions_EnforceBlocksCorruptRestrictions(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	jobProfileID := seedOrg056JobProfile(t, ctx, tenantID, svc, asOf, true)

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_restrictions_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-056-pos2", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-A",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		CapacityFTE:    1.0,
		ReasonCode:     "create",
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
	UPDATE org_position_slices
	SET profile='{"position_restrictions":{"allowed_job_profile_ids":["00000000-0000-0000-0000-00000000abcd"]}}'::jsonb
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date <= $3 AND end_date > $3
	`, tenantID, pos.PositionID, asOf)
	require.NoError(t, err)

	_, err = svc.CreateAssignment(ctx, tenantID, "req-056-assign", initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:         "00000001",
		EffectiveDate: asOf,
		PositionID:    &pos.PositionID,
		AllocatedFTE:  1.0,
		ReasonCode:    "assign",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_POSITION_RESTRICTIONS_PROFILE_MISMATCH", svcErr.Code)
}

func TestOrg056JobCatalog_FamilyGroupUpdateFromDate_CreatesSlices(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	_ = pool
	_ = rootNodeID

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{
		Code:          "TST",
		Name:          "Base",
		IsActive:      true,
		EffectiveDate: asOf,
	})
	require.NoError(t, err)

	d2 := time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC)
	name0 := "U0"
	active0 := true
	_, err = svc.UpdateJobFamilyGroup(ctx, tenantID, group.ID, orgsvc.JobFamilyGroupUpdate{
		Name:          &name0,
		IsActive:      &active0,
		EffectiveDate: d2,
		WriteMode:     orgsvc.WriteModeUpdateFromDate,
	})
	require.NoError(t, err)

	d3 := time.Date(2025, 12, 3, 0, 0, 0, 0, time.UTC)
	name1 := "U1"
	_, err = svc.UpdateJobFamilyGroup(ctx, tenantID, group.ID, orgsvc.JobFamilyGroupUpdate{
		Name:          &name1,
		IsActive:      &active0,
		EffectiveDate: d3,
		WriteMode:     orgsvc.WriteModeUpdateFromDate,
	})
	require.NoError(t, err)

	assertNameAt := func(t *testing.T, d time.Time, want string) {
		t.Helper()
		rows, err := svc.ListJobFamilyGroups(ctx, tenantID, d)
		require.NoError(t, err)
		for _, r := range rows {
			if r.ID == group.ID {
				require.Equal(t, want, r.Name)
				return
			}
		}
		t.Fatalf("job family group not found: %s", group.ID)
	}

	assertNameAt(t, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), "Base")
	assertNameAt(t, d2, "U0")
	assertNameAt(t, d3, "U1")
}

func TestOrg056JobCatalog_FamilyGroupUpdate_RequiresWriteMode(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)
	_ = pool
	_ = rootNodeID

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{
		Code:          "TST",
		Name:          "Base",
		IsActive:      true,
		EffectiveDate: asOf,
	})
	require.NoError(t, err)

	d2 := time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC)
	name0 := "U0"
	active0 := true
	_, err = svc.UpdateJobFamilyGroup(ctx, tenantID, group.ID, orgsvc.JobFamilyGroupUpdate{
		Name:          &name0,
		IsActive:      &active0,
		EffectiveDate: d2,
		WriteMode:     "",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 400, svcErr.Status)
	require.Equal(t, "ORG_INVALID_BODY", svcErr.Code)
}

func seedOrg056JobProfile(t *testing.T, ctx context.Context, tenantID uuid.UUID, svc *orgsvc.OrgService, asOf time.Time, isActive bool) uuid.UUID {
	t.Helper()

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{Code: "FIN", Name: "Finance", IsActive: true, EffectiveDate: asOf})
	require.NoError(t, err)
	family, err := svc.CreateJobFamily(ctx, tenantID, orgsvc.JobFamilyCreate{JobFamilyGroupID: group.ID, Code: "FIN-ACC", Name: "Accounting", IsActive: true, EffectiveDate: asOf})
	require.NoError(t, err)
	profile, err := svc.CreateJobProfile(ctx, tenantID, orgsvc.JobProfileCreate{
		Code:     "FIN-P1",
		Name:     "Finance Profile 1",
		IsActive: isActive,
		JobFamilies: orgsvc.JobProfileJobFamiliesSet{
			Items: []orgsvc.JobProfileJobFamilySetItem{
				{JobFamilyID: family.ID, IsPrimary: true},
			},
		},
		EffectiveDate: asOf,
	})
	require.NoError(t, err)
	return profile.ID
}
