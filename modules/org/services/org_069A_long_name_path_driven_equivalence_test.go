package services_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
	"github.com/iota-uz/iota-sdk/pkg/orglabels"
)

func TestOrg069ALongNamePathDrivenMatchesLegacyMixedQuery(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping integration test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool, err := pgxpool.New(ctx, itf.DbOpts(dbName))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	files := []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251218150000_org_outbox.sql",
		"20251219090000_org_hierarchy_closure_and_snapshots.sql",
		"20251219195000_org_security_group_mappings_and_links.sql",
		"20251219220000_org_reporting_nodes_and_view.sql",
		"20251220160000_org_position_slices_and_fte.sql",
		"20251220200000_org_job_catalog_profiles_and_validation_modes.sql",
		"20251221090000_org_reason_code_mode.sql",
		"20251222120000_org_personnel_events.sql",
		"20251227090000_org_valid_time_day_granularity.sql",
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(t, ctx, pool, tenantID, 250, asOf, "balanced", 69)

	nodeIDs := querySomeOrgNodeIDs(t, ctx, pool, tenantID, 100)
	require.Greater(t, len(nodeIDs), 10)

	queries := make([]orglabels.OrgNodeLongNameQuery, 0, len(nodeIDs))
	for i, id := range nodeIDs {
		day := asOf
		if i%2 == 1 {
			day = asOf.AddDate(0, 0, 1)
		}
		queries = append(queries, orglabels.OrgNodeLongNameQuery{OrgNodeID: id, AsOfDay: day})
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	got, err := orglabels.ResolveOrgNodeLongNames(reqCtx, tenantID, queries)
	require.NoError(t, err)
	require.NotEmpty(t, got)

	want, err := legacyOrgNodeLongNamesMixed(reqCtx, tx, tenantID, queries)
	require.NoError(t, err)

	for _, q := range queries {
		k := orglabels.OrgNodeLongNameKey{OrgNodeID: q.OrgNodeID, AsOfDate: normalizeAsOfDate(q.AsOfDay)}
		require.Equal(t, strings.TrimSpace(want[k]), strings.TrimSpace(got[k]), "mismatch for %s", k)
	}
}

func legacyOrgNodeLongNamesMixed(
	ctx context.Context,
	tx pgx.Tx,
	tenantID uuid.UUID,
	queries []orglabels.OrgNodeLongNameQuery,
) (map[orglabels.OrgNodeLongNameKey]string, error) {
	if tenantID == uuid.Nil || len(queries) == 0 {
		return map[orglabels.OrgNodeLongNameKey]string{}, nil
	}

	nodeIDs := make([]uuid.UUID, 0, len(queries))
	asOfDates := make([]string, 0, len(queries))
	keys := make([]orglabels.OrgNodeLongNameKey, 0, len(queries))

	for _, q := range queries {
		if q.OrgNodeID == uuid.Nil {
			continue
		}
		asStr := normalizeAsOfDate(q.AsOfDay)
		nodeIDs = append(nodeIDs, q.OrgNodeID)
		asOfDates = append(asOfDates, asStr)
		keys = append(keys, orglabels.OrgNodeLongNameKey{OrgNodeID: q.OrgNodeID, AsOfDate: asStr})
	}
	if len(nodeIDs) == 0 {
		return map[orglabels.OrgNodeLongNameKey]string{}, nil
	}

	rows, err := tx.Query(ctx, legacyMixedAsOfQuery, tenantID, pgtype.FlatArray[uuid.UUID](nodeIDs), pgtype.FlatArray[string](asOfDates))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[orglabels.OrgNodeLongNameKey]string, len(nodeIDs))
	for rows.Next() {
		var nodeID uuid.UUID
		var asOfDate string
		var longName string
		if err := rows.Scan(&nodeID, &asOfDate, &longName); err != nil {
			return nil, err
		}
		out[orglabels.OrgNodeLongNameKey{OrgNodeID: nodeID, AsOfDate: strings.TrimSpace(asOfDate)}] = strings.TrimSpace(longName)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	for _, k := range keys {
		if _, ok := out[k]; !ok {
			out[k] = ""
		}
	}
	return out, nil
}

func normalizeAsOfDate(t time.Time) string {
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

const legacyMixedAsOfQuery = `
WITH input AS (
  SELECT *
  FROM unnest($2::uuid[], $3::text[]) AS q(org_node_id, as_of_date)
),
target AS (
  SELECT
    i.org_node_id,
    i.as_of_date::date AS as_of_day,
    e.path,
    e.depth AS target_depth
  FROM input i
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_date::date
   AND e.end_date >= i.as_of_date::date
),
ancestors AS (
  SELECT
    t.org_node_id,
    t.as_of_day,
    e.child_node_id AS ancestor_id,
    (t.target_depth - e.depth) AS rel_depth
  FROM target t
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.effective_date <= t.as_of_day
   AND e.end_date >= t.as_of_day
   AND e.path @> t.path
),
parts AS (
  SELECT
    a.org_node_id,
    a.as_of_day,
    a.rel_depth,
    COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM ancestors a
  JOIN org_nodes n
    ON n.tenant_id=$1 AND n.id=a.ancestor_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id=$1 AND ns.org_node_id=a.ancestor_id
   AND ns.effective_date <= a.as_of_day AND ns.end_date >= a.as_of_day
)
SELECT
  org_node_id,
  as_of_day::text AS as_of_date,
  string_agg(part, ' / ' ORDER BY rel_depth DESC) AS long_name
FROM parts
GROUP BY org_node_id, as_of_day
`
