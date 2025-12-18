package services_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/domain/changerequest"
	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func TestChangeRequestRepository_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true")

	pool := newOrgTestDB(t, ctx, isCI)
	t.Cleanup(pool.Close)

	tenantA := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tenantB := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantA.String()+":tenantB"))
	ensureTenantCR(t, ctx, pool, tenantA)
	ensureTenantCR(t, ctx, pool, tenantB)

	repo := persistence.NewChangeRequestRepository()

	requester := uuid.New()
	payload := json.RawMessage(`{"effective_date":"2025-03-01","commands":[{"type":"node.update","payload":{"id":"00000000-0000-0000-0000-000000000002","name":"X"}}]}`)

	crID := withTenantTx(t, ctx, pool, tenantA, func(txCtx context.Context) uuid.UUID {
		cr, err := repo.Upsert(txCtx, &changerequest.ChangeRequest{
			RequestID:            "req-1",
			RequesterID:          requester,
			Status:               changerequest.StatusDraft,
			PayloadSchemaVersion: 1,
			Payload:              payload,
			Notes:                nil,
		})
		require.NoError(t, err)
		return cr.ID
	})

	withTenantTx(t, ctx, pool, tenantB, func(txCtx context.Context) uuid.UUID {
		_, err := repo.GetByID(txCtx, crID)
		require.Error(t, err)
		require.ErrorIs(t, err, pgx.ErrNoRows)
		return uuid.Nil
	})
}

func withTenantTx[T any](tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(context.Context) T) T {
	tb.Helper()
	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTx(ctx, tx)
	txCtx = composables.WithTenantID(txCtx, tenantID)

	out := fn(txCtx)
	require.NoError(tb, tx.Commit(ctx))
	return out
}

func ensureTenantCR(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) {
	tb.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO tenants (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, tenantID)
	require.NoError(tb, err)
}

func newOrgTestDB(tb testing.TB, ctx context.Context, isCI bool) *pgxpool.Pool {
	tb.Helper()

	conf := configuration.Use()
	host := strings.TrimSpace(conf.Database.Host)
	if host == "" {
		host = "localhost"
	}
	port := strings.TrimSpace(conf.Database.Port)
	if port == "" {
		port = "5432"
	}
	user := strings.TrimSpace(conf.Database.User)
	if user == "" {
		user = "postgres"
	}
	password := conf.Database.Password

	// Try to connect to postgres database to create a fresh DB for this test.
	adminDSN := "postgres://" + user + ":" + password + "@" + host + ":" + port + "/postgres?sslmode=disable"
	adminConn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		if isCI {
			require.NoError(tb, err)
		}
		tb.Skip("postgres is not reachable; skipping integration test")
	}
	tb.Cleanup(func() { _ = adminConn.Close(ctx) })

	dbName := "itf_" + strings.ToLower(strings.ReplaceAll(tb.Name(), "/", "_"))
	dbName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, dbName)

	_, _ = adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	_, err = adminConn.Exec(ctx, "CREATE DATABASE "+dbName)
	if err != nil {
		if isCI {
			require.NoError(tb, err)
		}
		tb.Skip("failed to create test database; skipping integration test")
	}

	pool, err := pgxpool.New(ctx, "postgres://"+user+":"+password+"@"+host+":"+port+"/"+dbName+"?sslmode=disable")
	require.NoError(tb, err)

	applyGooseUpSQL(tb, ctx, pool, filepath.Join("..", "..", "..", "migrations", "org", "00001_org_baseline.sql"))
	applyGooseUpSQL(tb, ctx, pool, filepath.Join("..", "..", "..", "migrations", "org", "20251218005114_org_placeholders_and_event_contracts.sql"))

	// Keep DB around until pool closed.
	tb.Cleanup(func() {
		pool.Close()
		_, _ = adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	})

	return pool
}

func applyGooseUpSQL(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, relPath string) {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Clean(relPath))
	require.NoError(tb, err)
	sql := extractGooseUp(string(raw))
	require.NotEmpty(tb, strings.TrimSpace(sql))
	_, err = pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol)
	require.NoError(tb, err)
}

func extractGooseUp(raw string) string {
	const up = "-- +goose Up"
	const down = "-- +goose Down"
	start := strings.Index(raw, up)
	if start < 0 {
		return raw
	}
	raw = raw[start+len(up):]
	if end := strings.Index(raw, down); end >= 0 {
		raw = raw[:end]
	}
	return raw
}
