package services_test

import (
	"context"
	"encoding/json"
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

func setupOrg059ADB(tb testing.TB, reasonCodeMode string) (context.Context, *pgxpool.Pool, uuid.UUID, uuid.UUID, time.Time, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 059A integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, uuid.Nil, time.Time{}, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	applyAllOrgMigrationsFor058(tb, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000059")
	ensureTenant(tb, ctx, pool, tenantID)
	seedPerson(tb, ctx, pool, tenantID, uuid.New(), "000123", "Test Person 000123")
	seedPerson(tb, ctx, pool, tenantID, uuid.New(), "000456", "Test Person 000456")
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)
	_, err = pool.Exec(ctx, `
UPDATE org_settings
SET
	position_catalog_validation_mode='disabled',
	position_restrictions_validation_mode='disabled',
	reason_code_mode=$2
WHERE tenant_id=$1
`, tenantID, reasonCodeMode)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(tb, ctx, pool, tenantID, 1, asOf, "deep", 59)
	nodes := buildPerfNodes(tb, tenantID, 1, "deep", 59)
	rootID := nodes[0].ID

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, rootID, asOf, svc
}

func TestOrg059AShadow_Position_MissingReasonCodeFillsLegacyAndMetaFlags(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg059ADB(t, "shadow")
	jobProfileID := seedOrg059AJobProfile(t, ctx, tenantID, svc)

	initiatorID := uuid.New()
	reqID := "req-059a-shadow-pos"
	_, err := svc.CreatePosition(ctx, tenantID, reqID, initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-059A-SHADOW",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		Profile:        json.RawMessage(`{}`),
		ReasonCode:     "",
	})
	require.NoError(t, err)

	var reasonCode, mode, missing, filled string
	err = pool.QueryRow(ctx, `
SELECT
	meta->>'reason_code',
	meta->>'reason_code_mode',
	meta->>'reason_code_original_missing',
	meta->>'reason_code_filled'
FROM org_audit_logs
WHERE tenant_id=$1 AND request_id=$2
`, tenantID, reqID).Scan(&reasonCode, &mode, &missing, &filled)
	require.NoError(t, err)
	require.Equal(t, "legacy", reasonCode)
	require.Equal(t, "shadow", mode)
	require.Equal(t, "true", missing)
	require.Equal(t, "true", filled)
}

func TestOrg059AEnforce_MissingReasonCodeBlocksPositionAndAssignment_NoAuditNoOutbox(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg059ADB(t, "enforce")
	jobProfileID := seedOrg059AJobProfile(t, ctx, tenantID, svc)

	initiatorID := uuid.New()

	reqMissingPos := "req-059a-enforce-pos-missing"
	_, err := svc.CreatePosition(ctx, tenantID, reqMissingPos, initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-059A-ENFORCE-1",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		Profile:        json.RawMessage(`{}`),
		ReasonCode:     "",
	})
	require.Error(t, err)
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 400, svcErr.Status)
	require.Equal(t, "ORG_INVALID_BODY", svcErr.Code)

	var auditCount int64
	err = pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM org_audit_logs WHERE tenant_id=$1 AND request_id=$2`, tenantID, reqMissingPos).Scan(&auditCount)
	require.NoError(t, err)
	require.Zero(t, auditCount)
	var outboxCount int64
	err = pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM org_outbox WHERE tenant_id=$1 AND payload->>'request_id'=$2`, tenantID, reqMissingPos).Scan(&outboxCount)
	require.NoError(t, err)
	require.Zero(t, outboxCount)

	pos, err := svc.CreatePosition(ctx, tenantID, "req-059a-enforce-pos-ok", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-059A-ENFORCE-OK",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		Profile:        json.RawMessage(`{}`),
		ReasonCode:     "create",
	})
	require.NoError(t, err)

	reqMissingAsg := "req-059a-enforce-asg-missing"
	_, err = svc.CreateAssignment(ctx, tenantID, reqMissingAsg, initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:         "000123",
		EffectiveDate: asOf,
		PositionID:    &pos.PositionID,
		AllocatedFTE:  1.0,
		ReasonCode:    "",
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 400, svcErr.Status)
	require.Equal(t, "ORG_INVALID_BODY", svcErr.Code)

	err = pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM org_audit_logs WHERE tenant_id=$1 AND request_id=$2`, tenantID, reqMissingAsg).Scan(&auditCount)
	require.NoError(t, err)
	require.Zero(t, auditCount)
	err = pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM org_outbox WHERE tenant_id=$1 AND payload->>'request_id'=$2`, tenantID, reqMissingAsg).Scan(&outboxCount)
	require.NoError(t, err)
	require.Zero(t, outboxCount)
}

func TestOrg059ADisabled_Assignment_MissingReasonCodeKeepsEmptyAndMetaFlags(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg059ADB(t, "disabled")
	jobProfileID := seedOrg059AJobProfile(t, ctx, tenantID, svc)

	initiatorID := uuid.New()

	posReqID := "req-059a-disabled-pos"
	pos, err := svc.CreatePosition(ctx, tenantID, posReqID, initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-059A-DISABLED",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		Profile:        json.RawMessage(`{}`),
		ReasonCode:     "",
	})
	require.NoError(t, err)

	var posReasonCode, posMode, posMissing, posFilled string
	err = pool.QueryRow(ctx, `
SELECT
	meta->>'reason_code',
	meta->>'reason_code_mode',
	meta->>'reason_code_original_missing',
	meta->>'reason_code_filled'
FROM org_audit_logs
	WHERE tenant_id=$1 AND request_id=$2
`, tenantID, posReqID).Scan(&posReasonCode, &posMode, &posMissing, &posFilled)
	require.NoError(t, err)
	require.Empty(t, posReasonCode)
	require.Equal(t, "disabled", posMode)
	require.Equal(t, "true", posMissing)
	require.Equal(t, "false", posFilled)

	asgReqID := "req-059a-disabled-asg"
	_, err = svc.CreateAssignment(ctx, tenantID, asgReqID, initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:         "000456",
		EffectiveDate: asOf,
		PositionID:    &pos.PositionID,
		AllocatedFTE:  1.0,
		ReasonCode:    "",
	})
	require.NoError(t, err)

	var asgReasonCode, asgMode, asgMissing, asgFilled string
	err = pool.QueryRow(ctx, `
SELECT
	meta->>'reason_code',
	meta->>'reason_code_mode',
	meta->>'reason_code_original_missing',
	meta->>'reason_code_filled'
FROM org_audit_logs
	WHERE tenant_id=$1 AND request_id=$2
`, tenantID, asgReqID).Scan(&asgReasonCode, &asgMode, &asgMissing, &asgFilled)
	require.NoError(t, err)
	require.Empty(t, asgReasonCode)
	require.Equal(t, "disabled", asgMode)
	require.Equal(t, "true", asgMissing)
	require.Equal(t, "false", asgFilled)
}

func TestOrg059A_MigrationsIncludeReasonCodeModeColumn(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 059A migration check")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)

	files := []string{
		"00001_org_baseline.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251221090000_org_reason_code_mode.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000059")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode) VALUES ($1,'disabled',0,'shadow')`, tenantID)
	require.NoError(t, err)
}

func seedOrg059AJobProfile(t *testing.T, ctx context.Context, tenantID uuid.UUID, svc *orgsvc.OrgService) uuid.UUID {
	t.Helper()

	group, err := svc.CreateJobFamilyGroup(ctx, tenantID, orgsvc.JobFamilyGroupCreate{
		Code:     "TST",
		Name:     "Test Group",
		IsActive: true,
	})
	require.NoError(t, err)

	family, err := svc.CreateJobFamily(ctx, tenantID, orgsvc.JobFamilyCreate{
		JobFamilyGroupID: group.ID,
		Code:             "TST-FAMILY",
		Name:             "Test Family",
		IsActive:         true,
	})
	require.NoError(t, err)

	profile, err := svc.CreateJobProfile(ctx, tenantID, orgsvc.JobProfileCreate{
		Code:     "TST-PROFILE",
		Name:     "Test Profile",
		IsActive: true,
		JobFamilies: orgsvc.JobProfileJobFamiliesSet{
			Items: []orgsvc.JobProfileJobFamilySetItem{
				{JobFamilyID: family.ID, IsPrimary: true},
			},
		},
	})
	require.NoError(t, err)
	return profile.ID
}
