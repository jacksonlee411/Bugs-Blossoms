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
	orgBaseline := readGooseUpSQL(t, filepath.Join(root, "migrations", "org", "00001_org_baseline.sql"))
	_, err = pool.Exec(ctx, orgBaseline)
	require.NoError(t, err)

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
