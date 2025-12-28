package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	coredtos "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type orgTestNode struct {
	ID           uuid.UUID
	Code         string
	ParentID     *uuid.UUID
	DisplayOrder int
}

func TestOrgAPIController_GetSnapshot_PaginatesWithCursor(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
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
	})

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := seedOrgTestTree(t, pool, tenantID, asOf, 6)

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: orgsvc.NewOrgService(persistence.NewOrgRepository()),
	}

	seen := map[string]struct{}{}
	cursor := (*string)(nil)
	for {
		path := "/org/api/snapshot?effective_date=" + url.QueryEscape(asOf.Format("2006-01-02")) +
			"&include=" + url.QueryEscape("nodes,edges") +
			"&limit=2"
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		req := newOrgAPIRequest(t, http.MethodGet, path, tenantID, u)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-snapshot")

		rr := httptest.NewRecorder()
		c.GetSnapshot(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", rr.Code, strings.TrimSpace(rr.Body.String()))
		}

		var res struct {
			TenantID      uuid.UUID             `json:"tenant_id"`
			EffectiveDate string                `json:"effective_date"`
			GeneratedAt   string                `json:"generated_at"`
			Includes      []string              `json:"includes"`
			Limit         int                   `json:"limit"`
			Items         []orgsvc.SnapshotItem `json:"items"`
			NextCursor    *string               `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		require.Equal(t, tenantID, res.TenantID)
		require.Equal(t, asOf.Format("2006-01-02"), res.EffectiveDate)
		require.Equal(t, "nodes", res.Includes[0])
		require.Equal(t, "edges", res.Includes[1])
		require.Equal(t, 2, res.Limit)
		require.NotEmpty(t, res.Items)

		for _, item := range res.Items {
			key := item.EntityType + ":" + item.EntityID.String()
			if _, ok := seen[key]; ok {
				t.Fatalf("duplicate snapshot item %s", key)
			}
			seen[key] = struct{}{}
		}

		if res.NextCursor == nil {
			break
		}
		cursor = res.NextCursor
	}

	// Snapshot default ordering: nodes first, then edges.
	require.Len(t, seen, len(nodes)*2)
}

func TestOrgAPIController_Batch_DryRunHasNoSideEffects(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
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
	})
	ensureOrgSettings(t, pool, tenantID)

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: orgsvc.NewOrgService(persistence.NewOrgRepository()),
	}

	body := mustJSON(t, map[string]any{
		"dry_run":        true,
		"effective_date": "2025-01-01",
		"commands": []any{
			map[string]any{
				"type":    "node.create",
				"payload": map[string]any{"code": "ROOT", "name": "Company"},
			},
		},
	})

	req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/batch", tenantID, u, body)
	req = req.WithContext(composables.WithPool(req.Context(), pool))
	req.Header.Set("X-Request-ID", "req-org-batch-dry-run")

	rr := httptest.NewRecorder()
	c.Batch(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		DryRun         bool `json:"dry_run"`
		EventsEnqueued int  `json:"events_enqueued"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.DryRun)
	require.Equal(t, 0, resp.EventsEnqueued)

	ctx := context.Background()
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_nodes", tenantID))
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_outbox", tenantID))
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_audit_logs", tenantID))
}

func TestOrgAPIController_Batch_InvalidCommandReturnsCommandIndexMeta(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
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
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: orgsvc.NewOrgService(persistence.NewOrgRepository()),
	}

	body := mustJSON(t, map[string]any{
		"dry_run":        true,
		"effective_date": "2025-01-01",
		"commands": []any{
			map[string]any{
				"type": "node.create",
			},
		},
	})

	req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/batch", tenantID, u, body)
	req = req.WithContext(composables.WithPool(req.Context(), pool))
	req.Header.Set("X-Request-ID", "req-org-batch-invalid")

	rr := httptest.NewRecorder()
	c.Batch(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	var apiErr coredtos.APIError
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &apiErr))
	require.Equal(t, "ORG_BATCH_INVALID_COMMAND", apiErr.Code)
	require.Equal(t, "payload is required", apiErr.Message)
	require.Equal(t, "req-org-batch-invalid", apiErr.Meta["request_id"])
	require.Equal(t, "0", apiErr.Meta["command_index"])
	require.Equal(t, "node.create", apiErr.Meta["command_type"])
}

func setupOrgTestDB(tb testing.TB, migrations []string) (*pgxpool.Pool, uuid.UUID) {
	tb.Helper()

	isCI := strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org controller integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, uuid.Nil
	}

	pool := newPool(tb, dbName)
	tb.Cleanup(pool.Close)

	root := filepath.Clean("../../../../")
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Join(root, "migrations", "org", f))
		_, err := pool.Exec(context.Background(), sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(context.Background(), "INSERT INTO tenants (id) VALUES ($1) ON CONFLICT (id) DO NOTHING", tenantID)
	require.NoError(tb, err)

	return pool, tenantID
}

func ensureOrgSettings(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID) {
	tb.Helper()

	_, err := pool.Exec(context.Background(), `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)
}

func seedOrgTestTree(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID, asOf time.Time, count int) []orgTestNode {
	tb.Helper()

	require.GreaterOrEqual(tb, count, 2, "count must be >= 2")

	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte("org-snapshot:"+tenantID.String()))
	nodes := make([]orgTestNode, 0, count)
	rootID := uuid.NewSHA1(namespace, []byte("node:0"))
	nodes = append(nodes, orgTestNode{ID: rootID, Code: "D0000", ParentID: nil, DisplayOrder: 0})
	for i := 1; i < count; i++ {
		p := rootID
		id := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
		nodes = append(nodes, orgTestNode{
			ID:           id,
			Code:         fmt.Sprintf("D%04d", i),
			ParentID:     &p,
			DisplayOrder: i - 1,
		})
	}

	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for _, n := range nodes {
		_, err := pool.Exec(ctx, "INSERT INTO org_nodes (tenant_id, id, type, code, is_root) VALUES ($1,$2,$3,$4,$5)",
			tenantID, n.ID, "OrgUnit", n.Code, n.ParentID == nil,
		)
		require.NoError(tb, err)
	}
	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_node_slices
					(tenant_id, org_node_id, name, display_order, parent_hint, effective_date, end_date)
					VALUES (
						$1,$2,$3,$4,$5,
						($6 AT TIME ZONE 'UTC')::date,
						($7 AT TIME ZONE 'UTC')::date
					)`,
			tenantID, n.ID, n.Code, n.DisplayOrder, n.ParentID, asOf, endDate,
		)
		require.NoError(tb, err)
	}
	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_edges
					(tenant_id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
					VALUES (
						$1,$2,$3,$4,
						($5 AT TIME ZONE 'UTC')::date,
						($6 AT TIME ZONE 'UTC')::date
					)`,
			tenantID, "OrgUnit", n.ParentID, n.ID, asOf, endDate,
		)
		require.NoError(tb, err)
	}
	return nodes
}

func newOrgAPIRequestWithBody(t *testing.T, method, path string, tenantID uuid.UUID, u coreuser.User, body []byte) *http.Request {
	t.Helper()

	req := newOrgAPIRequest(t, method, path, tenantID, u)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newTestOrgUser(tenantID uuid.UUID) coreuser.User {
	return coreuser.New(
		"Test",
		"User",
		internet.MustParseEmail("test@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(1),
		coreuser.WithTenantID(tenantID),
	)
}

func mustJSON(tb testing.TB, v any) []byte {
	tb.Helper()
	b, err := json.Marshal(v)
	require.NoError(tb, err)
	return b
}

func countRows(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, table string, tenantID uuid.UUID) int64 {
	tb.Helper()

	var n int64
	err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", tenantID).Scan(&n)
	require.NoError(tb, err)
	return n
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
					tb.Skipf("postgres is not reachable; skipping integration test: %v", err)
				}
			}
			panic(r)
		}
	}()

	createDB(tb, name)
	return true
}

func newPool(tb testing.TB, dbName string) *pgxpool.Pool {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbOpts(dbName))
	require.NoError(tb, err)
	config.MaxConns = 4
	config.MinConns = 1
	config.MaxConnLifetime = 5 * time.Minute
	config.MaxConnIdleTime = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(tb, err)
	return pool
}

func dbOpts(name string) string {
	dbName := sanitizeDBName(name)
	c := configuration.Use()
	return fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s password=%s sslmode=disable",
		c.Database.Host, c.Database.Port, c.Database.User, dbName, c.Database.Password,
	)
}

func sanitizeDBName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "t_" + hex.EncodeToString(sum[:8])
}

func createDB(tb testing.TB, name string) {
	tb.Helper()

	c := configuration.Use()
	adminConnStr := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=postgres password=%s sslmode=disable",
		c.Database.Host, c.Database.Port, c.Database.User, c.Database.Password,
	)

	dbName := sanitizeDBName(name)

	var lastErr error
	const maxAttempts = 60
	for attempt := 0; attempt < maxAttempts; attempt++ {
		db, err := sql.Open("postgres", adminConnStr)
		if err != nil {
			lastErr = err
			continue
		}
		func() {
			defer func() {
				_ = db.Close()
			}()

			pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := db.PingContext(pingCtx); err != nil {
				lastErr = err
				return
			}

			dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err = db.ExecContext(dropCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName))
			if err != nil {
				_, err = db.ExecContext(dropCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			}
			if err != nil {
				lastErr = err
				return
			}

			createCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err = db.ExecContext(createCtx, fmt.Sprintf("CREATE DATABASE %s", dbName))
			if err != nil {
				lastErr = err
				return
			}

			lastErr = nil
		}()

		if lastErr == nil {
			return
		}
		if !isTransientPostgresError(lastErr) {
			panic(lastErr)
		}

		delay := time.Duration(attempt+1) * 100 * time.Millisecond
		if delay > time.Second {
			delay = time.Second
		}
		time.Sleep(delay)
	}
	panic(lastErr)
}

func isTransientPostgresError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "pg_database_datname_index"),
		strings.Contains(msg, "already exists"):
		return true
	case strings.Contains(msg, "the database system is starting up"),
		strings.Contains(msg, "the database system is not yet accepting connections"),
		strings.Contains(msg, "connect: connection refused"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "connection reset by peer"),
		strings.Contains(msg, "EOF"):
		return true
	default:
		return false
	}
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
