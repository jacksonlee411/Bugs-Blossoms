package services_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrgTreeQueryBudget(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true")

	profile := strings.ToLower(strings.TrimSpace(getenvDefault("PROFILE", "balanced")))
	seed := parseInt64Default(t, "SEED", 42)
	scale := parseScaleDefault(t, "SCALE", 1000)
	if scale < 10 {
		scale = 10
	}
	asOf := parseEffectiveDateDefault(t, "EFFECTIVE_DATE", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	largeTenantID := parseUUIDDefault(t, "TENANT_ID", uuid.MustParse("00000000-0000-0000-0000-000000000001"))
	smallTenantID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(largeTenantID.String()+":small"))

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping query budget test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	tracer := &queryCountTracer{}
	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), tracer)
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

	ensureTenant(t, ctx, pool, largeTenantID)
	ensureTenant(t, ctx, pool, smallTenantID)

	seedOrgTree(t, ctx, pool, smallTenantID, 10, asOf, profile, seed)
	seedOrgTree(t, ctx, pool, largeTenantID, scale, asOf, profile, seed)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)

	queriesSmall := measureHierarchyQueries(t, ctx, pool, tracer, svc, smallTenantID, asOf)
	queriesLarge := measureHierarchyQueries(t, ctx, pool, tracer, svc, largeTenantID, asOf)

	const expected = 1
	require.Equal(t, expected, queriesSmall, "unexpected query count for small dataset")
	require.Equal(t, expected, queriesLarge, "unexpected query count for large dataset")
}

func canDialPostgres(tb testing.TB) bool {
	tb.Helper()

	cfg := configuration.Use()
	host := strings.TrimSpace(cfg.Database.Host)
	if host == "" {
		host = "localhost"
	}
	port := strings.TrimSpace(cfg.Database.Port)
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

func safeCreateDB(tb testing.TB, name string) bool {
	tb.Helper()

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				msg := err.Error()
				if strings.Contains(msg, "connect: connection refused") || strings.Contains(msg, "i/o timeout") {
					if strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true") {
						panic(r)
					}
					tb.Skipf("postgres is not reachable; skipping query budget test: %v", err)
				}
			}
			panic(r)
		}
	}()

	itf.CreateDB(name)
	return true
}

type queryCountTracer struct {
	mu sync.Mutex
	n  int
}

func (t *queryCountTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	t.n++
	t.mu.Unlock()
	return ctx
}

func (t *queryCountTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {}

func (t *queryCountTracer) Reset() {
	t.mu.Lock()
	t.n = 0
	t.mu.Unlock()
}

func (t *queryCountTracer) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.n
}

func newPoolWithQueryTracer(tb testing.TB, dbOpts string, tracer pgx.QueryTracer) *pgxpool.Pool {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbOpts)
	require.NoError(tb, err)
	config.ConnConfig.Tracer = tracer
	config.MaxConns = 4
	config.MinConns = 1
	config.MaxConnLifetime = 5 * time.Minute
	config.MaxConnIdleTime = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(tb, err)
	return pool
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

func ensureTenant(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) {
	tb.Helper()

	_, err := pool.Exec(ctx, "INSERT INTO tenants (id) VALUES ($1) ON CONFLICT (id) DO NOTHING", tenantID)
	require.NoError(tb, err)
}

type perfNode struct {
	ID           uuid.UUID
	Code         string
	ParentID     *uuid.UUID
	DisplayOrder int
}

func seedOrgTree(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, count int, asOf time.Time, profile string, seed int64) {
	tb.Helper()

	nodes := buildPerfNodes(tb, tenantID, count, profile, seed)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	for _, n := range nodes {
		_, err := pool.Exec(ctx, "INSERT INTO org_nodes (tenant_id, id, type, code, is_root) VALUES ($1,$2,$3,$4,$5)",
			tenantID, n.ID, "OrgUnit", n.Code, n.ParentID == nil,
		)
		require.NoError(tb, err)
	}

	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_node_slices
					(tenant_id, org_node_id, name, display_order, parent_hint, effective_date, end_date)
					VALUES ($1,$2,$3,$4,$5,$6::date,$7::date)`,
			tenantID, n.ID, n.Code, n.DisplayOrder, n.ParentID, asOf, endDate,
		)
		require.NoError(tb, err)
	}

	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_edges
					(tenant_id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
					VALUES ($1,$2,$3,$4,$5::date,$6::date)`,
			tenantID, "OrgUnit", n.ParentID, n.ID, asOf, endDate,
		)
		require.NoError(tb, err)
	}
}

func buildPerfNodes(tb testing.TB, tenantID uuid.UUID, count int, profile string, seed int64) []perfNode {
	tb.Helper()

	require.Positive(tb, count, "count must be positive")

	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:org-query-budget:%d", tenantID, seed)))
	rootID := uuid.NewSHA1(namespace, []byte("node:0"))
	nodes := []perfNode{
		{ID: rootID, Code: "D0000", ParentID: nil, DisplayOrder: 0},
	}
	if count == 1 {
		return nodes
	}

	switch profile {
	case "balanced", "":
		maxChildren := 4
		queue := []uuid.UUID{rootID}
		children := map[uuid.UUID]int{}
		siblingOrder := map[uuid.UUID]int{}
		for i := 1; i < count; i++ {
			parent := queue[0]
			parentID := parent
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			order := siblingOrder[parent]
			siblingOrder[parent] = order + 1
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				ParentID:     &parentID,
				DisplayOrder: order,
			})
			queue = append(queue, nodeID)
			children[parent]++
			if children[parent] >= maxChildren {
				queue = queue[1:]
			}
		}
		return nodes
	case "wide":
		parent := rootID
		for i := 1; i < count; i++ {
			parentID := parent
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				ParentID:     &parentID,
				DisplayOrder: i - 1,
			})
		}
		return nodes
	case "deep":
		parent := rootID
		for i := 1; i < count; i++ {
			parentID := parent
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				ParentID:     &parentID,
				DisplayOrder: 0,
			})
			parent = nodeID
		}
		return nodes
	default:
		tb.Fatalf("unknown profile: %s", profile)
		return nil
	}
}

func measureHierarchyQueries(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tracer *queryCountTracer, svc *orgsvc.OrgService, tenantID uuid.UUID, asOf time.Time) int {
	tb.Helper()

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	nodes, _, err := svc.GetHierarchyAsOf(reqCtx, tenantID, "OrgUnit", asOf)
	require.NoError(tb, err)
	require.NotEmpty(tb, nodes)

	return tracer.Count()
}

func getenvDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func parseUUIDDefault(tb testing.TB, key string, def uuid.UUID) uuid.UUID {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	id, err := uuid.Parse(raw)
	require.NoError(tb, err, "invalid %s", key)
	return id
}

func parseInt64Default(tb testing.TB, key string, def int64) int64 {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(tb, err, "invalid %s", key)
	return n
}

func parseScaleDefault(tb testing.TB, key string, def int) int {
	tb.Helper()

	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return def
	}
	if raw == "1k" || raw == "1000" {
		return 1000
	}
	n, err := strconv.Atoi(raw)
	require.NoError(tb, err, "invalid %s", key)
	require.Positive(tb, n, "invalid %s", key)
	return n
}

func parseEffectiveDateDefault(tb testing.TB, key string, def time.Time) time.Time {
	tb.Helper()

	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC()
	}
	tm, err := time.Parse(time.RFC3339, raw)
	require.NoError(tb, err, "invalid %s", key)
	return tm.UTC()
}
