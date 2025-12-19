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

func setupOrg032DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, time.Time, []perfNode, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 032 integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, time.Time{}, nil, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	schemaSQL := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "00001_org_baseline.sql")))
	_, err := pool.Exec(ctx, schemaSQL)
	require.NoError(tb, err)

	orgSettingsSQL := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "20251218130000_org_settings_and_audit.sql")))
	_, err = pool.Exec(ctx, orgSettingsSQL)
	require.NoError(tb, err)

	m032 := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "20251219195000_org_security_group_mappings_and_links.sql")))
	_, err = pool.Exec(ctx, m032)
	require.NoError(tb, err)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(tb, ctx, pool, tenantID)
	_, err = pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)

	asOfDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(tb, tenantID, 4, "deep", 42)
	seedOrgTreeFromNodes(tb, ctx, pool, tenantID, nodes, asOfDate)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, asOfDate, nodes, svc
}

func TestOrg032PermissionPreview_InheritanceAndLinks(t *testing.T) {
	ctx, _, tenantID, asOfDate, nodes, svc := setupOrg032DB(t)

	rootID := nodes[0].ID
	childID := nodes[1].ID
	leafID := nodes[len(nodes)-1].ID
	initiatorID := uuid.New()

	_, err := svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-1", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        rootID,
		SecurityGroupKey: "wd:ROOT",
		AppliesToSubtree: true,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-2", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        childID,
		SecurityGroupKey: "wd:CHILD",
		AppliesToSubtree: false,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-3", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        rootID,
		SecurityGroupKey: "wd:HRBP",
		AppliesToSubtree: true,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-4", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        leafID,
		SecurityGroupKey: "wd:HRBP",
		AppliesToSubtree: false,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateOrgLink(ctx, tenantID, "req-032-5", initiatorID, orgsvc.CreateOrgLinkInput{
		OrgNodeID:     rootID,
		ObjectType:    "cost_center",
		ObjectKey:     "CC-ROOT",
		LinkType:      "uses",
		Metadata:      map[string]any{},
		EffectiveDate: asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateOrgLink(ctx, tenantID, "req-032-6", initiatorID, orgsvc.CreateOrgLinkInput{
		OrgNodeID:     leafID,
		ObjectType:    "cost_center",
		ObjectKey:     "CC-LEAF-1",
		LinkType:      "uses",
		Metadata:      map[string]any{"k": "v"},
		EffectiveDate: asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateOrgLink(ctx, tenantID, "req-032-7", initiatorID, orgsvc.CreateOrgLinkInput{
		OrgNodeID:     leafID,
		ObjectType:    "cost_center",
		ObjectKey:     "CC-LEAF-2",
		LinkType:      "uses",
		Metadata:      map[string]any{},
		EffectiveDate: asOfDate,
	})
	require.NoError(t, err)

	res, err := svc.PermissionPreview(ctx, tenantID, orgsvc.PermissionPreviewInput{
		OrgNodeID:             leafID,
		EffectiveDate:         asOfDate,
		IncludeSecurityGroups: true,
		IncludeLinks:          true,
		LimitLinks:            1,
	})
	require.NoError(t, err)

	byKey := map[string]orgsvc.PermissionPreviewSecurityGroup{}
	for _, g := range res.SecurityGroups {
		byKey[g.SecurityGroupKey] = g
	}

	require.Contains(t, byKey, "wd:ROOT")
	require.Equal(t, rootID, byKey["wd:ROOT"].SourceOrgNodeID)
	require.Equal(t, 3, byKey["wd:ROOT"].SourceDepth)

	require.NotContains(t, byKey, "wd:CHILD")

	require.Contains(t, byKey, "wd:HRBP")
	require.Equal(t, leafID, byKey["wd:HRBP"].SourceOrgNodeID)
	require.Equal(t, 0, byKey["wd:HRBP"].SourceDepth)
	require.False(t, byKey["wd:HRBP"].AppliesToSubtree)

	require.Contains(t, res.Warnings, "links_truncated")
	require.Len(t, res.Links, 1)
	require.Equal(t, "cost_center", res.Links[0].ObjectType)
	require.Contains(t, []string{"CC-LEAF-1", "CC-LEAF-2"}, res.Links[0].ObjectKey)
	require.Equal(t, leafID, res.Links[0].Source.OrgNodeID)
}

func TestOrg032SecurityGroupMappings_NoOverlap(t *testing.T) {
	ctx, _, tenantID, asOfDate, nodes, svc := setupOrg032DB(t)

	rootID := nodes[0].ID
	initiatorID := uuid.New()

	_, err := svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-ol-1", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        rootID,
		SecurityGroupKey: "wd:OVERLAP",
		AppliesToSubtree: true,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)

	_, err = svc.CreateSecurityGroupMapping(ctx, tenantID, "req-032-ol-2", initiatorID, orgsvc.CreateSecurityGroupMappingInput{
		OrgNodeID:        rootID,
		SecurityGroupKey: "wd:OVERLAP",
		AppliesToSubtree: true,
		EffectiveDate:    asOfDate.AddDate(0, 1, 0),
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 409, svcErr.Status)
	require.Equal(t, "ORG_OVERLAP", svcErr.Code)
}
