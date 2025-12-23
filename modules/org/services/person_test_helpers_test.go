package services_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func applyAllPersonMigrations(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) {
	tb.Helper()

	files := []string{
		"00001_person_baseline.sql",
		"00002_person_migration_smoke.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "person", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}
}

func seedPerson(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID, personUUID uuid.UUID, pernr, displayName string) {
	tb.Helper()

	_, err := pool.Exec(ctx, `
INSERT INTO persons (tenant_id, person_uuid, pernr, display_name, status)
VALUES ($1,$2,$3,$4,'active')
`, tenantID, personUUID, pernr, displayName)
	require.NoError(tb, err)
}
