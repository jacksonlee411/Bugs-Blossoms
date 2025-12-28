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

	initiatorID := uuid.New()
	res, err := svc.CreatePosition(ctx, tenantID, "req-056-shadow", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-SHADOW",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "FIN",
		JobFamilyCode:      "FIN-ACC",
		JobRoleCode:        "FIN-MGR",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)

	var code string
	err = pool.QueryRow(ctx, `
	SELECT meta->>'position_catalog_validation_error_code'
	FROM org_audit_logs
	WHERE tenant_id=$1 AND request_id=$2 AND entity_id=$3
	`, tenantID, "req-056-shadow", res.PositionID).Scan(&code)
	require.NoError(t, err)
	require.Equal(t, "ORG_JOB_CATALOG_INACTIVE_OR_MISSING", code)
}

func TestOrg056CatalogValidation_EnforceBlocksInvalidCodes(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_catalog_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	initiatorID := uuid.New()
	_, err = svc.CreatePosition(ctx, tenantID, "req-056-enforce", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-ENFORCE",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "FIN",
		JobFamilyCode:      "FIN-ACC",
		JobRoleCode:        "FIN-MGR",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_JOB_CATALOG_INACTIVE_OR_MISSING", svcErr.Code)
}

func TestOrg056JobProfileConflict_EnforceBlocks(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_catalog_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{Code: "FIN", Name: "Finance", IsActive: true})
	require.NoError(t, err)
	fam1, err := svc.CreateJobFamily(ctx, tenantID, orgsvc.JobFamilyCreate{JobFamilyGroupID: group.ID, Code: "FIN-ACC", Name: "Accounting", IsActive: true})
	require.NoError(t, err)
	role1, err := svc.CreateJobRole(ctx, tenantID, orgsvc.JobRoleCreate{JobFamilyID: fam1.ID, Code: "FIN-MGR", Name: "Manager", IsActive: true})
	require.NoError(t, err)
	level1, err := svc.CreateJobLevel(ctx, tenantID, orgsvc.JobLevelCreate{JobRoleID: role1.ID, Code: "L1", Name: "L1", DisplayOrder: 0, IsActive: true})
	require.NoError(t, err)
	profile1, err := svc.CreateJobProfile(ctx, tenantID, orgsvc.JobProfileCreate{Code: "FIN-MGR-P1", Name: "P1", JobRoleID: role1.ID, IsActive: true})
	require.NoError(t, err)
	require.NoError(t, svc.SetJobProfileAllowedLevels(ctx, tenantID, profile1.ID, orgsvc.JobProfileAllowedLevelsSet{JobLevelIDs: []uuid.UUID{level1.ID}}))

	fam2, err := svc.CreateJobFamily(ctx, tenantID, orgsvc.JobFamilyCreate{JobFamilyGroupID: group.ID, Code: "FIN-OPS", Name: "Ops", IsActive: true})
	require.NoError(t, err)
	role2, err := svc.CreateJobRole(ctx, tenantID, orgsvc.JobRoleCreate{JobFamilyID: fam2.ID, Code: "OPS-ROLE", Name: "OpsRole", IsActive: true})
	require.NoError(t, err)
	_, err = svc.CreateJobLevel(ctx, tenantID, orgsvc.JobLevelCreate{JobRoleID: role2.ID, Code: "L1", Name: "L1", DisplayOrder: 0, IsActive: true})
	require.NoError(t, err)
	profile2, err := svc.CreateJobProfile(ctx, tenantID, orgsvc.JobProfileCreate{Code: "OPS-P2", Name: "P2", JobRoleID: role2.ID, IsActive: true})
	require.NoError(t, err)

	initiatorID := uuid.New()
	_, err = svc.CreatePosition(ctx, tenantID, "req-056-profile-conflict", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-PROFILE-CONFLICT",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "FIN",
		JobFamilyCode:      "FIN-ACC",
		JobRoleCode:        "FIN-MGR",
		JobLevelCode:       "L1",
		JobProfileID:       &profile2.ID,
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_JOB_PROFILE_CONFLICT", svcErr.Code)
}

func TestOrg056SetPositionRestrictions_RejectsMismatch(t *testing.T) {
	ctx, _, tenantID, rootNodeID, asOf, svc := setupOrg056DB(t)

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{Code: "FIN", Name: "Finance", IsActive: true})
	require.NoError(t, err)
	fam1, err := svc.CreateJobFamily(ctx, tenantID, orgsvc.JobFamilyCreate{JobFamilyGroupID: group.ID, Code: "FIN-ACC", Name: "Accounting", IsActive: true})
	require.NoError(t, err)
	role1, err := svc.CreateJobRole(ctx, tenantID, orgsvc.JobRoleCreate{JobFamilyID: fam1.ID, Code: "FIN-MGR", Name: "Manager", IsActive: true})
	require.NoError(t, err)
	_, err = svc.CreateJobLevel(ctx, tenantID, orgsvc.JobLevelCreate{JobRoleID: role1.ID, Code: "L1", Name: "L1", DisplayOrder: 0, IsActive: true})
	require.NoError(t, err)
	profile1, err := svc.CreateJobProfile(ctx, tenantID, orgsvc.JobProfileCreate{Code: "FIN-MGR-P1", Name: "P1", JobRoleID: role1.ID, IsActive: true})
	require.NoError(t, err)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-056-pos", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-R",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "FIN",
		JobFamilyCode:      "FIN-ACC",
		JobRoleCode:        "FIN-MGR",
		JobLevelCode:       "L1",
		JobProfileID:       &profile1.ID,
		CapacityFTE:        1.0,
		ReasonCode:         "create",
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

	_, err := pool.Exec(ctx, `UPDATE org_settings SET position_restrictions_validation_mode='enforce' WHERE tenant_id=$1`, tenantID)
	require.NoError(t, err)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-056-pos2", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-A",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "FIN",
		JobFamilyCode:      "FIN-ACC",
		JobRoleCode:        "FIN-MGR",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
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
