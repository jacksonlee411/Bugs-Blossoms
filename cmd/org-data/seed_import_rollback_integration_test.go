package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrgData_SeedImport_ApplyAndRollbackByManifest(t *testing.T) {
	t.Setenv("ORG_DATA_QUALITY_ENABLED", "true") // ensure config singleton doesn't lock this as false.

	if !canDialPostgres(t) {
		if strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true") {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT)")
		}
		t.Skip("postgres is not reachable; skipping org-data seed import integration test")
	}

	dbName := t.Name()
	itf.CreateDB(dbName)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, itf.DbOpts(dbName))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	root := filepath.Clean(filepath.Join("..", ".."))
	migrations := []string{
		"00001_org_baseline.sql",
		"00002_org_migration_smoke.sql",
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
		"20251231120000_org_remove_job_family_allocation_percent.sql",
		"20260101020855_org_job_catalog_effective_dated_slices_phase_a.sql",
		"20260101020930_org_job_catalog_effective_dated_slices_gates_and_backfill.sql",
		"20260104100000_org_drop_job_profile_job_families_legacy.sql",
		"20260104120000_org_drop_job_catalog_identity_legacy_columns.sql",
	}
	for _, f := range migrations {
		sql := readGooseUpSQL(t, filepath.Join(root, "migrations", "org", f))
		_, err = pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO tenants (id) VALUES ($1)", tenantID)
	require.NoError(t, err)

	inputDir := t.TempDir()
	writeOrgDataNodesCSV(t, inputDir, []string{
		"code,name,effective_date,parent_code,display_order",
		"D0000,Company,2025-01-01,,0",
		"D0001,Dept A,2025-01-01,D0000,1",
	})

	nodes, err := parseNodesCSV(filepath.Join(inputDir, "nodes.csv"))
	require.NoError(t, err)

	runID := uuid.New()
	startedAt := time.Now().UTC()
	data, err := normalizeAndValidate(runID, tenantID, startedAt, nodes, nil, nil, false)
	require.NoError(t, err)

	manifest, err := applySeedImport(ctx, pool, data, importOptions{
		tenantID: tenantID,
		inputDir: inputDir,
		backend:  "db",
		mode:     "seed",
	})
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Inserted.OrgNodes)
	require.NotEmpty(t, manifest.Inserted.OrgNodeSlices)
	require.NotEmpty(t, manifest.Inserted.OrgEdges)

	require.Equal(t, int64(len(manifest.Inserted.OrgNodes)), countRows(t, pool, tenantID, "org_nodes"))
	require.Equal(t, int64(len(manifest.Inserted.OrgNodeSlices)), countRows(t, pool, tenantID, "org_node_slices"))
	require.Equal(t, int64(len(manifest.Inserted.OrgEdges)), countRows(t, pool, tenantID, "org_edges"))

	require.NoError(t, rollbackByManifest(ctx, pool, tenantID, manifest))

	require.Equal(t, int64(0), countRows(t, pool, tenantID, "org_assignments"))
	require.Equal(t, int64(0), countRows(t, pool, tenantID, "org_positions"))
	require.Equal(t, int64(0), countRows(t, pool, tenantID, "org_edges"))
	require.Equal(t, int64(0), countRows(t, pool, tenantID, "org_node_slices"))
	require.Equal(t, int64(0), countRows(t, pool, tenantID, "org_nodes"))
}

func writeOrgDataNodesCSV(tb testing.TB, dir string, lines []string) {
	tb.Helper()

	content := strings.Join(lines, "\n") + "\n"
	require.NoError(tb, os.WriteFile(filepath.Join(dir, "nodes.csv"), []byte(content), 0o644))
}

func countRows(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID, table string) int64 {
	tb.Helper()

	var n int64
	require.NoError(tb, pool.QueryRow(context.Background(), "SELECT count(*)::bigint FROM "+table+" WHERE tenant_id=$1", tenantID).Scan(&n))
	return n
}

func readGooseUpSQL(tb testing.TB, path string) string {
	tb.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(tb, err)
	s := string(raw)
	if idx := strings.Index(s, "-- +goose Down"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func canDialPostgres(tb testing.TB) bool {
	tb.Helper()

	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	if host == "" {
		host = "localhost"
	}
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	if port == "" {
		port = "5432"
	}
	addr := net.JoinHostPort(host, port)

	dialer := &net.Dialer{Timeout: 250 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
