package services_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrg066_DeleteNodeSliceAndStitch_DeletesMiddleSliceAndKeepsGapFree(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 066 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000066")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	nodeID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES ($1,$2,'OrgUnit','ROOT',true)
`, tenantID, nodeID)
	require.NoError(t, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2025, 1, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
INSERT INTO org_node_slices (tenant_id, org_node_id, name, status, effective_date, end_date)
VALUES
  ($1,$2,'A','active',$3::date,$4::date),
  ($1,$2,'B','active',$5::date,$6::date),
  ($1,$2,'C','active',$7::date,$8::date)
`, tenantID, nodeID,
		asOf, time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		mid, time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC), end,
	)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)
	initiatorID := uuid.New()

	_, err = svc.DeleteNodeSliceAndStitch(reqCtx, tenantID, "req-node-delete", initiatorID, orgsvc.DeleteNodeSliceAndStitchInput{
		NodeID:              nodeID,
		TargetEffectiveDate: mid,
		ReasonCode:          "legacy",
	})
	require.NoError(t, err)

	var gapCount int
	err = pool.QueryRow(ctx, `
WITH ordered AS (
  SELECT effective_date, end_date, lag(end_date) OVER (ORDER BY effective_date) AS prev_end
  FROM org_node_slices
  WHERE tenant_id=$1 AND org_node_id=$2
)
SELECT COUNT(*)::int
FROM ordered
WHERE prev_end IS NOT NULL AND prev_end + 1 <> effective_date
`, tenantID, nodeID).Scan(&gapCount)
	require.NoError(t, err)
	require.Equal(t, 0, gapCount)

	// DB gate: bypassing service (delete only) should fail at commit.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `DELETE FROM org_node_slices WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date=$3`, tenantID, nodeID, time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	err = tx.Commit(ctx)
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23000", pgErr.Code)
	require.Equal(t, "org_node_slices_gap_free", pgErr.ConstraintName)
}

func TestOrg066_DeleteAssignmentAndStitch_DeletesMiddleSliceAndKeepsGapFree(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 066 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000066")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	personUUID := uuid.New()
	seedPerson(t, ctx, pool, tenantID, personUUID, "000066", "Test Person 000066")

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	orgNodeID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES ($1,$2,'OrgUnit','ROOT',true)
`, tenantID, orgNodeID)
	require.NoError(t, err)

	positionID := uuid.New()
	seedOrgPosition(t, ctx, pool, tenantID, orgNodeID, positionID, "POS-066", 1.0, asOf, endDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)
	initiatorID := uuid.New()

	created, err := svc.CreateAssignment(reqCtx, tenantID, "req-asg-create", initiatorID, orgsvc.CreateAssignmentInput{
		Pernr:          "000066",
		EffectiveDate:  asOf,
		AssignmentType: "primary",
		AllocatedFTE:   1.0,
		PositionID:     &positionID,
	})
	require.NoError(t, err)

	update1Date := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	updated1, err := svc.UpdateAssignment(reqCtx, tenantID, "req-asg-upd1", initiatorID, orgsvc.UpdateAssignmentInput{
		AssignmentID:  created.AssignmentID,
		EffectiveDate: update1Date,
		ReasonCode:    "legacy",
		PositionID:    &positionID,
	})
	require.NoError(t, err)

	update2Date := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	updated2, err := svc.UpdateAssignment(reqCtx, tenantID, "req-asg-upd2", initiatorID, orgsvc.UpdateAssignmentInput{
		AssignmentID:  updated1.AssignmentID,
		EffectiveDate: update2Date,
		ReasonCode:    "legacy",
		PositionID:    &positionID,
	})
	require.NoError(t, err)
	require.NotEqual(t, created.AssignmentID, updated1.AssignmentID)
	require.NotEqual(t, updated1.AssignmentID, updated2.AssignmentID)

	_, err = svc.DeleteAssignmentAndStitch(reqCtx, tenantID, "req-asg-delete", initiatorID, orgsvc.DeleteAssignmentAndStitchInput{
		AssignmentID: updated1.AssignmentID,
		ReasonCode:   "legacy",
	})
	require.NoError(t, err)

	var gapCount int
	err = pool.QueryRow(ctx, `
WITH ordered AS (
  SELECT effective_date, end_date, lag(end_date) OVER (ORDER BY effective_date) AS prev_end
  FROM org_assignments
  WHERE tenant_id=$1 AND subject_type='person' AND subject_id=$2 AND assignment_type='primary'
  ORDER BY effective_date
)
SELECT COUNT(*)::int
FROM ordered
WHERE prev_end IS NOT NULL AND prev_end + 1 <> effective_date
`, tenantID, personUUID).Scan(&gapCount)
	require.NoError(t, err)
	require.Equal(t, 0, gapCount)

	var lastEnd time.Time
	err = pool.QueryRow(ctx, `
SELECT end_date
FROM org_assignments
WHERE tenant_id=$1 AND subject_id=$2 AND assignment_type='primary'
ORDER BY effective_date DESC
LIMIT 1
`, tenantID, personUUID).Scan(&lastEnd)
	require.NoError(t, err)
	require.Equal(t, endDate, lastEnd)
}

func TestOrg066_DeletePositionSliceAndStitch_DeletesMiddleSliceAndKeepsGapFree(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 066 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000066")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	jobProfileID, _ := ensureTestJobProfileWith100PercentFamily(t, ctx, pool, tenantID)

	orgNodeID := uuid.New()
	_, err = pool.Exec(ctx, `
	INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
	VALUES ($1,$2,'OrgUnit','ROOT',true)
	`, tenantID, orgNodeID)
	require.NoError(t, err)

	positionID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_positions (tenant_id, id, org_node_id, code, status, is_auto_created, effective_date, end_date)
VALUES ($1,$2,$3,'POS-066','active',false,$4::date,$5::date)
`, tenantID, positionID, orgNodeID, asOf, end)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
	INSERT INTO org_position_slices (tenant_id, position_id, org_node_id, lifecycle_status, capacity_fte, job_profile_id, effective_date, end_date)
	VALUES
	  ($1,$2,$3,'active',1.0,$4,$5::date,$6::date),
	  ($1,$2,$3,'active',1.0,$4,$7::date,$8::date),
	  ($1,$2,$3,'active',1.0,$4,$9::date,$10::date)
	`, tenantID, positionID, orgNodeID, jobProfileID,
		asOf, time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 11, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC), end,
	)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)
	initiatorID := uuid.New()

	_, err = svc.DeletePositionSliceAndStitch(reqCtx, tenantID, "req-pos-delete", initiatorID, orgsvc.DeletePositionSliceAndStitchInput{
		PositionID:          positionID,
		TargetEffectiveDate: time.Date(2025, 1, 11, 0, 0, 0, 0, time.UTC),
		ReasonCode:          "legacy",
	})
	require.NoError(t, err)

	var gapCount int
	err = pool.QueryRow(ctx, `
WITH ordered AS (
  SELECT effective_date, end_date, lag(end_date) OVER (ORDER BY effective_date) AS prev_end
  FROM org_position_slices
  WHERE tenant_id=$1 AND position_id=$2
)
SELECT COUNT(*)::int
FROM ordered
WHERE prev_end IS NOT NULL AND prev_end + 1 <> effective_date
`, tenantID, positionID).Scan(&gapCount)
	require.NoError(t, err)
	require.Equal(t, 0, gapCount)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `DELETE FROM org_position_slices WHERE tenant_id=$1 AND position_id=$2 AND effective_date=$3`, tenantID, positionID, time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	err = tx.Commit(ctx)
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23000", pgErr.Code)
	require.Equal(t, "org_position_slices_gap_free", pgErr.ConstraintName)
}

func TestOrg066_DeleteEdgeSliceAndStitch_DeletesMiddleSliceAndKeepsGapFree(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 066 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000066")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	parentID := uuid.New()
	childID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root)
VALUES
  ($1,$2,'OrgUnit','PARENT',true),
  ($1,$3,'OrgUnit','CHILD',false)
`, tenantID, parentID, childID)
	require.NoError(t, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
INSERT INTO org_edges (tenant_id, hierarchy_type, parent_node_id, child_node_id, path, depth, effective_date, end_date)
VALUES ($1,'OrgUnit',NULL,$2,'x'::ltree,0,$3::date,$4::date)
`, tenantID, parentID, asOf, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO org_edges (tenant_id, hierarchy_type, parent_node_id, child_node_id, path, depth, effective_date, end_date)
VALUES
  ($1,'OrgUnit',$2,$3,'x'::ltree,0,$4::date,$5::date),
  ($1,'OrgUnit',$2,$3,'x'::ltree,0,$6::date,$7::date),
  ($1,'OrgUnit',$2,$3,'x'::ltree,0,$8::date,$9::date)
`, tenantID, parentID, childID,
		asOf, time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 11, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)
	initiatorID := uuid.New()

	_, err = svc.DeleteEdgeSliceAndStitch(reqCtx, tenantID, "req-edge-delete", initiatorID, orgsvc.DeleteEdgeSliceAndStitchInput{
		HierarchyType:       "OrgUnit",
		ChildNodeID:         childID,
		TargetEffectiveDate: time.Date(2025, 1, 11, 0, 0, 0, 0, time.UTC),
		ReasonCode:          "legacy",
	})
	require.NoError(t, err)

	var gapCount int
	err = pool.QueryRow(ctx, `
WITH ordered AS (
  SELECT effective_date, end_date, lag(end_date) OVER (ORDER BY effective_date) AS prev_end
  FROM org_edges
  WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND child_node_id=$2
)
SELECT COUNT(*)::int
FROM ordered
WHERE prev_end IS NOT NULL AND prev_end + 1 <> effective_date
`, tenantID, childID).Scan(&gapCount)
	require.NoError(t, err)
	require.Equal(t, 0, gapCount)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `DELETE FROM org_edges WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND child_node_id=$2 AND effective_date=$3`, tenantID, childID, time.Date(2025, 1, 21, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	err = tx.Commit(ctx)
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23000", pgErr.Code)
	require.Equal(t, "org_edges_gap_free", pgErr.ConstraintName)
}
