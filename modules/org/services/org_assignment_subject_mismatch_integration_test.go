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
)

func TestOrgAssignment_SubjectMismatch_ReturnsServiceError(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org subject mismatch test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)

	applyAllPersonMigrations(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000061")
	ensureTenant(t, ctx, pool, tenantID)

	pernr := "000123"
	personUUID := uuid.New()
	seedPerson(t, ctx, pool, tenantID, personUUID, pernr, "Test Person 000123")

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	wrongSubjectID := uuid.New()
	_, err := svc.CreateAssignment(reqCtx, tenantID, "req-061-mismatch", uuid.New(), orgsvc.CreateAssignmentInput{
		Pernr:         pernr,
		EffectiveDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		SubjectID:     &wrongSubjectID,
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_SUBJECT_MISMATCH", svcErr.Code)
}
