package services_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
	"github.com/iota-uz/iota-sdk/pkg/orglabels"
)

func TestOrg069B_DeleteEdgeSliceAndStitch_RewritesFutureDescendantPaths(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 069B integration test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-00000000069b")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	rootID := uuid.New()
	xID := uuid.New()
	yID := uuid.New()
	zID := uuid.New()

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := []perfNode{
		{ID: rootID, Code: "ROOT", ParentID: nil, DisplayOrder: 0},
		{ID: xID, Code: "X", ParentID: &rootID, DisplayOrder: 0},
		{ID: yID, Code: "Y", ParentID: &xID, DisplayOrder: 0},
		{ID: zID, Code: "Z", ParentID: &yID, DisplayOrder: 0},
	}
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOf)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	initiatorID := uuid.New()
	e1 := asOf.AddDate(0, 0, 10)

	_, err = svc.MoveNode(reqCtx, tenantID, "req-069b-move-y", initiatorID, orgsvc.MoveNodeInput{
		NodeID:        yID,
		NewParentID:   rootID,
		EffectiveDate: e1,
	})
	require.NoError(t, err)

	before, err := orglabels.ResolveOrgNodeLongNamesAsOf(reqCtx, tenantID, e1, []uuid.UUID{zID})
	require.NoError(t, err)
	require.Equal(t, "ROOT / Y / Z", before[zID])

	_, err = svc.DeleteEdgeSliceAndStitch(reqCtx, tenantID, "req-069b-delete-y", initiatorID, orgsvc.DeleteEdgeSliceAndStitchInput{
		HierarchyType:       "OrgUnit",
		ChildNodeID:         yID,
		TargetEffectiveDate: e1,
	})
	require.NoError(t, err)

	after, err := orglabels.ResolveOrgNodeLongNamesAsOf(reqCtx, tenantID, e1, []uuid.UUID{zID})
	require.NoError(t, err)
	require.Equal(t, "ROOT / X / Y / Z", after[zID])

	var inconsistent int
	err = pool.QueryRow(ctx, `
SELECT count(*) AS inconsistent_edges
FROM org_edges c
JOIN org_edges p
  ON p.tenant_id=c.tenant_id
 AND p.hierarchy_type=c.hierarchy_type
 AND p.child_node_id=c.parent_node_id
 AND p.effective_date <= c.effective_date
 AND p.end_date >= c.effective_date
WHERE c.tenant_id=$1
  AND c.hierarchy_type=$2
  AND c.parent_node_id IS NOT NULL
  AND NOT (p.path @> c.path)
`, tenantID, "OrgUnit").Scan(&inconsistent)
	require.NoError(t, err)
	require.Equal(t, 0, inconsistent)
}

func TestOrg069B_DeleteEdgeSliceAndStitch_RejectsDeletingFirstSlice(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 069B integration test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)
	applyAllOrgMigrationsFor058(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-00000000069b")
	ensureTenant(t, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days, reason_code_mode)
VALUES ($1,'disabled',0,'disabled')
ON CONFLICT (tenant_id) DO UPDATE
SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days, reason_code_mode=excluded.reason_code_mode
`, tenantID)
	require.NoError(t, err)

	rootID := uuid.New()
	childID := uuid.New()

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := []perfNode{
		{ID: rootID, Code: "ROOT", ParentID: nil, DisplayOrder: 0},
		{ID: childID, Code: "CHILD", ParentID: &rootID, DisplayOrder: 0},
	}
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOf)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	initiatorID := uuid.New()
	_, err = svc.DeleteEdgeSliceAndStitch(reqCtx, tenantID, "req-069b-delete-first", initiatorID, orgsvc.DeleteEdgeSliceAndStitchInput{
		HierarchyType:       "OrgUnit",
		ChildNodeID:         childID,
		TargetEffectiveDate: asOf,
	})
	require.Error(t, err)
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_CANNOT_DELETE_FIRST_EDGE_SLICE", svcErr.Code)
}
