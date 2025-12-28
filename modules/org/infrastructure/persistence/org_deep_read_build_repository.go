package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

const orgDeepReadMaxDepth = 2048

func deepReadLockKey(parts ...string) string {
	return fmt.Sprintf("org:deep-read:%s", joinNonEmpty(parts...))
}

func joinNonEmpty(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" {
			out = p
			continue
		}
		out += ":" + p
	}
	return out
}

func withDeepReadLock(ctx context.Context, key string, fn func(context.Context) error) error {
	return composables.InTx(ctx, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, "SELECT pg_advisory_xact_lock(hashtext($1))", key)
		if err != nil {
			return err
		}
		return fn(txCtx)
	})
}

func (r *OrgRepository) BuildDeepReadSnapshot(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOfDate time.Time, apply bool, sourceRequestID string) (services.DeepReadBuildResult, error) {
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return services.DeepReadBuildResult{}, err
	}

	asOfDate = time.Date(asOfDate.UTC().Year(), asOfDate.UTC().Month(), asOfDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	asOfDateStr := asOfDate.Format("2006-01-02")
	lockKey := deepReadLockKey("snapshot", tenantID.String(), hierarchyType, asOfDateStr)

	ctx = composables.WithPool(ctx, pool)
	ctx = composables.WithTenantID(ctx, tenantID)

	var out services.DeepReadBuildResult
	out.TenantID = tenantID
	out.HierarchyType = hierarchyType
	out.Backend = services.DeepReadBackendSnapshot
	out.AsOfDate = asOfDateStr
	out.DryRun = !apply
	out.SourceRequestID = sourceRequestID

	if !apply {
		err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, err := composables.UseTx(txCtx)
			if err != nil {
				return err
			}

			return tx.QueryRow(txCtx, `
	WITH RECURSIVE edges_asof AS (
		SELECT parent_node_id, child_node_id
		FROM org_edges
		WHERE tenant_id=$1
		  AND hierarchy_type=$2
		  AND effective_date <= $3
		  AND end_date >= $3
	),
closure AS (
	SELECT e.child_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 0 AS depth
	FROM edges_asof e
	UNION ALL
	SELECT e.parent_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 1 AS depth
	FROM edges_asof e
	WHERE e.parent_node_id IS NOT NULL
	UNION ALL
	SELECT c.ancestor_node_id, e.child_node_id, c.depth + 1
	FROM closure c
	JOIN edges_asof e ON e.parent_node_id = c.descendant_node_id
	WHERE c.depth < $4
),
dedup AS (
	SELECT ancestor_node_id, descendant_node_id, MIN(depth) AS depth
	FROM closure
	GROUP BY ancestor_node_id, descendant_node_id
)
SELECT COUNT(*)::bigint, COALESCE(MAX(depth), 0)::int
FROM dedup
	`, pgUUID(tenantID), hierarchyType, pgValidDate(asOfDate), orgDeepReadMaxDepth).Scan(&out.RowCount, &out.MaxDepth)
		})
		return out, err
	}

	buildID := uuid.New()
	out.BuildID = buildID

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
INSERT INTO org_hierarchy_snapshot_builds (tenant_id, hierarchy_type, as_of_date, build_id, status, is_active, source_request_id)
VALUES ($1,$2,$3::date,$4,'building',false,$5)
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID), nullableString(sourceRequestID))
		return err
	}); err != nil {
		return out, err
	}

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}

		tag, err := tx.Exec(txCtx, `
INSERT INTO org_hierarchy_snapshots (
	tenant_id,
	hierarchy_type,
	as_of_date,
	build_id,
	ancestor_node_id,
	descendant_node_id,
	depth
)
	WITH RECURSIVE edges_asof AS (
		SELECT parent_node_id, child_node_id
		FROM org_edges
		WHERE tenant_id=$1
		  AND hierarchy_type=$2
		  AND effective_date <= $3
		  AND end_date >= $3
	),
closure AS (
	SELECT e.child_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 0 AS depth
	FROM edges_asof e
	UNION ALL
	SELECT e.parent_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 1 AS depth
	FROM edges_asof e
	WHERE e.parent_node_id IS NOT NULL
	UNION ALL
	SELECT c.ancestor_node_id, e.child_node_id, c.depth + 1
	FROM closure c
	JOIN edges_asof e ON e.parent_node_id = c.descendant_node_id
	WHERE c.depth < $4
),
dedup AS (
	SELECT ancestor_node_id, descendant_node_id, MIN(depth) AS depth
	FROM closure
	GROUP BY ancestor_node_id, descendant_node_id
)
SELECT
	$1,
	$2,
	$5::date,
	$6,
	ancestor_node_id,
	descendant_node_id,
	depth
FROM dedup
`, pgUUID(tenantID), hierarchyType, pgValidDate(asOfDate), orgDeepReadMaxDepth, asOfDateStr, pgUUID(buildID))
		if err != nil {
			return err
		}
		out.RowCount = tag.RowsAffected()

		return tx.QueryRow(txCtx, `
SELECT COALESCE(MAX(depth), 0)::int
FROM org_hierarchy_snapshots
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID)).Scan(&out.MaxDepth)
	}); err != nil {
		_ = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, err := composables.UseTx(txCtx)
			if err != nil {
				return err
			}
			_, err = tx.Exec(txCtx, `
UPDATE org_hierarchy_snapshot_builds
SET status='failed', notes=$5
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID), err.Error())
			return err
		})
		return out, err
	}

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
UPDATE org_hierarchy_snapshot_builds
SET is_active=false
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND is_active=true
`, pgUUID(tenantID), hierarchyType, asOfDateStr)
		if err != nil {
			return err
		}
		ct, err := tx.Exec(txCtx, `
UPDATE org_hierarchy_snapshot_builds
SET status='ready', is_active=true
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID))
		if err != nil {
			return err
		}
		if ct.RowsAffected() != 1 {
			return fmt.Errorf("snapshot build activate failed: rows_affected=%d", ct.RowsAffected())
		}
		out.Activated = true
		return nil
	}); err != nil {
		_ = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, txErr := composables.UseTx(txCtx)
			if txErr != nil {
				return txErr
			}
			_, txErr = tx.Exec(txCtx, `
UPDATE org_hierarchy_snapshot_builds
SET status='failed', notes=$5, is_active=false
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID), err.Error())
			return txErr
		})
		return out, err
	}

	return out, nil
}

func (r *OrgRepository) BuildDeepReadClosure(ctx context.Context, tenantID uuid.UUID, hierarchyType string, apply bool, sourceRequestID string) (services.DeepReadBuildResult, error) {
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return services.DeepReadBuildResult{}, err
	}
	lockKey := deepReadLockKey("closure", tenantID.String(), hierarchyType)

	ctx = composables.WithPool(ctx, pool)
	ctx = composables.WithTenantID(ctx, tenantID)

	var out services.DeepReadBuildResult
	out.TenantID = tenantID
	out.HierarchyType = hierarchyType
	out.Backend = services.DeepReadBackendClosure
	out.DryRun = !apply
	out.SourceRequestID = sourceRequestID

	if !apply {
		err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, err := composables.UseTx(txCtx)
			if err != nil {
				return err
			}

			return tx.QueryRow(txCtx, `
WITH RECURSIVE edges AS (
	SELECT parent_node_id, child_node_id, effective_date, end_date
	FROM org_edges
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
),
closure AS (
	SELECT e.child_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 0 AS depth, e.effective_date, e.end_date
	FROM edges e
	UNION ALL
	SELECT e.parent_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 1 AS depth, e.effective_date, e.end_date
	FROM edges e
	WHERE e.parent_node_id IS NOT NULL
	UNION ALL
	SELECT
		c.ancestor_node_id,
		e.child_node_id AS descendant_node_id,
		c.depth + 1 AS depth,
		GREATEST(c.effective_date, e.effective_date) AS effective_date,
		LEAST(c.end_date, e.end_date) AS end_date
	FROM closure c
	JOIN edges e ON e.parent_node_id = c.descendant_node_id
	WHERE c.depth < $3
	  AND GREATEST(c.effective_date, e.effective_date) < LEAST(c.end_date, e.end_date)
),
dedup AS (
	SELECT DISTINCT ancestor_node_id, descendant_node_id, depth, effective_date, end_date
	FROM closure
)
SELECT COUNT(*)::bigint, COALESCE(MAX(depth), 0)::int
FROM dedup
`, pgUUID(tenantID), hierarchyType, orgDeepReadMaxDepth).Scan(&out.RowCount, &out.MaxDepth)
		})
		return out, err
	}

	buildID := uuid.New()
	out.BuildID = buildID

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
INSERT INTO org_hierarchy_closure_builds (tenant_id, hierarchy_type, build_id, status, is_active, source_request_id)
VALUES ($1,$2,$3,'building',false,$4)
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), nullableString(sourceRequestID))
		return err
	}); err != nil {
		return out, err
	}

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}

		tag, err := tx.Exec(txCtx, `
	INSERT INTO org_hierarchy_closure (
		tenant_id,
		hierarchy_type,
		build_id,
		ancestor_node_id,
		descendant_node_id,
		depth,
		effective_date,
		end_date
	)
	WITH RECURSIVE edges AS (
		SELECT parent_node_id, child_node_id, effective_date, end_date
		FROM org_edges
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
),
closure AS (
	SELECT e.child_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 0 AS depth, e.effective_date, e.end_date
	FROM edges e
	UNION ALL
	SELECT e.parent_node_id AS ancestor_node_id, e.child_node_id AS descendant_node_id, 1 AS depth, e.effective_date, e.end_date
	FROM edges e
	WHERE e.parent_node_id IS NOT NULL
	UNION ALL
	SELECT
		c.ancestor_node_id,
		e.child_node_id AS descendant_node_id,
		c.depth + 1 AS depth,
		GREATEST(c.effective_date, e.effective_date) AS effective_date,
		LEAST(c.end_date, e.end_date) AS end_date
	FROM closure c
	JOIN edges e ON e.parent_node_id = c.descendant_node_id
	WHERE c.depth < $3
	  AND GREATEST(c.effective_date, e.effective_date) <= LEAST(c.end_date, e.end_date)
),
dedup AS (
	SELECT DISTINCT ancestor_node_id, descendant_node_id, depth, effective_date, end_date
	FROM closure
)
	SELECT
		$1,
		$2,
		$4,
		ancestor_node_id,
		descendant_node_id,
		depth,
		effective_date,
		end_date
	FROM dedup
	`, pgUUID(tenantID), hierarchyType, orgDeepReadMaxDepth, pgUUID(buildID))
		if err != nil {
			return err
		}
		out.RowCount = tag.RowsAffected()

		return tx.QueryRow(txCtx, `
SELECT COALESCE(MAX(depth), 0)::int
FROM org_hierarchy_closure
WHERE tenant_id=$1 AND hierarchy_type=$2 AND build_id=$3
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID)).Scan(&out.MaxDepth)
	}); err != nil {
		_ = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, txErr := composables.UseTx(txCtx)
			if txErr != nil {
				return txErr
			}
			_, txErr = tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET status='failed', notes=$4
WHERE tenant_id=$1 AND hierarchy_type=$2 AND build_id=$3
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), err.Error())
			return txErr
		})
		return out, err
	}

	if err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET is_active=false
WHERE tenant_id=$1 AND hierarchy_type=$2 AND is_active=true
`, pgUUID(tenantID), hierarchyType)
		if err != nil {
			return err
		}
		ct, err := tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET status='ready', is_active=true
WHERE tenant_id=$1 AND hierarchy_type=$2 AND build_id=$3
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID))
		if err != nil {
			return err
		}
		if ct.RowsAffected() != 1 {
			return fmt.Errorf("closure build activate failed: rows_affected=%d", ct.RowsAffected())
		}
		out.Activated = true
		return nil
	}); err != nil {
		_ = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, txErr := composables.UseTx(txCtx)
			if txErr != nil {
				return txErr
			}
			_, txErr = tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET status='failed', notes=$4, is_active=false
WHERE tenant_id=$1 AND hierarchy_type=$2 AND build_id=$3
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), err.Error())
			return txErr
		})
		return out, err
	}

	return out, nil
}

func (r *OrgRepository) ActivateDeepReadClosureBuild(ctx context.Context, tenantID uuid.UUID, hierarchyType string, buildID uuid.UUID) (uuid.UUID, error) {
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	lockKey := deepReadLockKey("closure", tenantID.String(), hierarchyType)

	ctx = composables.WithPool(ctx, pool)
	ctx = composables.WithTenantID(ctx, tenantID)

	var previous uuid.UUID
	err = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}
		_ = tx.QueryRow(txCtx, `
SELECT build_id
FROM org_hierarchy_closure_builds
WHERE tenant_id=$1 AND hierarchy_type=$2 AND is_active=true
ORDER BY built_at DESC
LIMIT 1
`, pgUUID(tenantID), hierarchyType).Scan(&previous)

		_, err = tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET is_active=false
WHERE tenant_id=$1 AND hierarchy_type=$2 AND is_active=true
`, pgUUID(tenantID), hierarchyType)
		if err != nil {
			return err
		}

		ct, err := tx.Exec(txCtx, `
UPDATE org_hierarchy_closure_builds
SET is_active=true
WHERE tenant_id=$1 AND hierarchy_type=$2 AND build_id=$3 AND status='ready'
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID))
		if err != nil {
			return err
		}
		if ct.RowsAffected() != 1 {
			return fmt.Errorf("closure activate failed: build not found/ready (tenant_id=%s hierarchy_type=%s build_id=%s)", tenantID, hierarchyType, buildID)
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return previous, nil
}

func (r *OrgRepository) PruneDeepReadClosureBuilds(ctx context.Context, tenantID uuid.UUID, hierarchyType string, keep int) (services.DeepReadPruneResult, error) {
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return services.DeepReadPruneResult{}, err
	}
	lockKey := deepReadLockKey("closure", tenantID.String(), hierarchyType)

	ctx = composables.WithPool(ctx, pool)
	ctx = composables.WithTenantID(ctx, tenantID)

	if keep <= 0 {
		keep = 1
	}

	var out services.DeepReadPruneResult
	out.TenantID = tenantID
	out.HierarchyType = hierarchyType
	out.Backend = services.DeepReadBackendClosure

	err = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}

		tag, err := tx.Exec(txCtx, `
WITH keep_builds AS (
	SELECT build_id
	FROM org_hierarchy_closure_builds
	WHERE tenant_id=$1 AND hierarchy_type=$2
	ORDER BY is_active DESC, built_at DESC
	LIMIT $3
),
to_delete AS (
	SELECT build_id
	FROM org_hierarchy_closure_builds
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND build_id NOT IN (SELECT build_id FROM keep_builds)
)
DELETE FROM org_hierarchy_closure_builds b
USING to_delete d
WHERE b.tenant_id=$1
  AND b.hierarchy_type=$2
  AND b.build_id=d.build_id
`, pgUUID(tenantID), hierarchyType, keep)
		if err != nil {
			return err
		}
		out.DeletedBuilds = int(tag.RowsAffected())
		return nil
	})
	return out, err
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
