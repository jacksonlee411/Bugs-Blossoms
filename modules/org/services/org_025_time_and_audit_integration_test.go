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
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func setupOrg025DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, time.Time, []perfNode, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 025 integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, time.Time{}, nil, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	root := filepath.Clean(filepath.Join("..", "..", ".."))
	migrations := []string{
		"00001_org_baseline.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251218150000_org_outbox.sql",
		"20251220200000_org_job_catalog_profiles_and_validation_modes.sql",
		"20251221090000_org_reason_code_mode.sql",
	}
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Join(root, "migrations", "org", f))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(tb, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(tb, tenantID, 3, "balanced", 42)
	seedOrgTreeFromNodes(tb, ctx, pool, tenantID, nodes, asOf)

	// Prepare a boundary for ShiftBoundaryNode on the second child (node B).
	nodeB := nodes[2]
	boundary := asOf.AddDate(0, 1, 0)
	_, err = pool.Exec(ctx, `
UPDATE org_node_slices
SET end_date=$1
WHERE tenant_id=$2 AND org_node_id=$3 AND effective_date=$4
`, boundary, tenantID, nodeB.ID, asOf)
	require.NoError(tb, err)
	_, err = pool.Exec(ctx, `
INSERT INTO org_node_slices (tenant_id, org_node_id, name, display_order, parent_hint, effective_date, end_date)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`, tenantID, nodeB.ID, nodeB.Code, nodeB.DisplayOrder, nodeB.ParentID, boundary, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(tb, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, asOf, nodes, svc
}

func TestOrg025_NodeCorrectRescindShiftBoundaryCorrectMove_WriteAuditLogs(t *testing.T) {
	ctx, pool, tenantID, asOf, nodes, svc := setupOrg025DB(t)

	rootID := nodes[0].ID
	nodeA := nodes[1].ID
	nodeB := nodes[2].ID
	initiatorID := uuid.New()

	// Correct node A in-place.
	{
		reqID := "req-025-correct"
		newName := "Corrected A"
		_, err := svc.CorrectNode(ctx, tenantID, reqID, initiatorID, orgsvc.CorrectNodeInput{
			NodeID: nodeA,
			AsOf:   asOf,
			Name:   &newName,
		})
		require.NoError(t, err)

		require.Equal(t, int64(1), countAuditOps(t, pool, tenantID, reqID, "Correct"))
	}

	// Correct-move node A under node B at slice start.
	{
		reqID := "req-025-correct-move"
		_, err := svc.CorrectMoveNode(ctx, tenantID, reqID, initiatorID, orgsvc.CorrectMoveNodeInput{
			NodeID:        nodeA,
			EffectiveDate: asOf,
			NewParentID:   nodeB,
		})
		require.NoError(t, err)

		var parentID uuid.UUID
		err = pool.QueryRow(context.Background(), `
SELECT parent_node_id
FROM org_edges
WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND child_node_id=$2
  AND effective_date <= $3 AND end_date > $3
`, tenantID, nodeA, asOf).Scan(&parentID)
		require.NoError(t, err)
		require.Equal(t, nodeB, parentID)

		require.Equal(t, int64(1), countAuditOps(t, pool, tenantID, reqID, "CorrectMove"))
	}

	// Rescind node A at a later date (should not delete history).
	{
		reqID := "req-025-rescind"
		effective := asOf.AddDate(0, 0, 1)
		_, err := svc.RescindNode(ctx, tenantID, reqID, initiatorID, orgsvc.RescindNodeInput{
			NodeID:        nodeA,
			EffectiveDate: effective,
			Reason:        "test",
		})
		require.NoError(t, err)

		var status string
		err = pool.QueryRow(context.Background(), `
SELECT status
FROM org_node_slices
WHERE tenant_id=$1 AND org_node_id=$2
  AND effective_date <= $3 AND end_date > $3
ORDER BY effective_date DESC
LIMIT 1
`, tenantID, nodeA, effective).Scan(&status)
		require.NoError(t, err)
		require.Equal(t, "rescinded", status)

		require.Equal(t, int64(1), countAuditOps(t, pool, tenantID, reqID, "Rescind"))
	}

	// ShiftBoundary for node B (two audit records expected).
	{
		reqID := "req-025-shift-boundary"
		targetStart := asOf.AddDate(0, 1, 0)
		newStart := targetStart.AddDate(0, 0, 5)
		_, err := svc.ShiftBoundaryNode(ctx, tenantID, reqID, initiatorID, orgsvc.ShiftBoundaryNodeInput{
			NodeID:              nodeB,
			TargetEffectiveDate: targetStart,
			NewEffectiveDate:    newStart,
		})
		require.NoError(t, err)

		require.Equal(t, int64(2), countAuditOps(t, pool, tenantID, reqID, "ShiftBoundary"))

		var count int
		err = pool.QueryRow(context.Background(), `
SELECT count(*)
FROM org_node_slices
WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date=$3
`, tenantID, nodeB, newStart).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	}

	// Sanity: root remains root.
	{
		var isRoot bool
		err := pool.QueryRow(context.Background(), `SELECT is_root FROM org_nodes WHERE tenant_id=$1 AND id=$2`, tenantID, rootID).Scan(&isRoot)
		require.NoError(t, err)
		require.True(t, isRoot)
	}
}

func countAuditOps(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID, requestID string, operation string) int64 {
	tb.Helper()

	var n int64
	err := pool.QueryRow(context.Background(), `
SELECT count(*)
FROM org_audit_logs
WHERE tenant_id=$1
  AND request_id=$2
  AND meta->>'operation'=$3
`, tenantID, requestID, operation).Scan(&n)
	require.NoError(tb, err)
	return n
}
