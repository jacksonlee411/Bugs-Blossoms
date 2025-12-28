package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

func activeClosureBuildID(ctx context.Context, tx repo.Tx, tenantID uuid.UUID, hierarchyType string) (uuid.UUID, error) {
	var buildID uuid.UUID
	err := tx.QueryRow(ctx, `
SELECT build_id
FROM org_hierarchy_closure_builds
WHERE tenant_id=$1
  AND hierarchy_type=$2
  AND is_active=true
  AND status='ready'
ORDER BY built_at DESC
LIMIT 1
`, pgUUID(tenantID), hierarchyType).Scan(&buildID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("%w (closure): tenant_id=%s hierarchy_type=%s", services.ErrOrgDeepReadBuildNotReady, tenantID, hierarchyType)
		}
		return uuid.Nil, err
	}
	return buildID, nil
}

func activeSnapshotBuildID(ctx context.Context, tx repo.Tx, tenantID uuid.UUID, hierarchyType string, asOfDate string) (uuid.UUID, error) {
	var buildID uuid.UUID
	err := tx.QueryRow(ctx, `
SELECT build_id
FROM org_hierarchy_snapshot_builds
WHERE tenant_id=$1
  AND hierarchy_type=$2
  AND as_of_date=$3::date
  AND is_active=true
  AND status='ready'
ORDER BY built_at DESC
LIMIT 1
`, pgUUID(tenantID), hierarchyType, asOfDate).Scan(&buildID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("%w (snapshot): tenant_id=%s hierarchy_type=%s as_of_date=%s", services.ErrOrgDeepReadBuildNotReady, tenantID, hierarchyType, asOfDate)
		}
		return uuid.Nil, err
	}
	return buildID, nil
}

func (r *OrgRepository) ListAncestorsAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, descendantNodeID uuid.UUID, asOf time.Time, backend services.DeepReadBackend) ([]services.DeepReadRelation, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	switch backend {
	case services.DeepReadBackendEdges:
		rows, err := tx.Query(ctx, `
WITH target AS (
	SELECT path, depth
	FROM org_edges
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND child_node_id=$3
	  AND effective_date <= $4
	  AND end_date >= $4
	ORDER BY effective_date DESC
	LIMIT 1
)
SELECT e.child_node_id, (target.depth - e.depth) AS depth
FROM target
JOIN org_edges e
	ON e.tenant_id=$1
	AND e.hierarchy_type=$2
	AND e.effective_date <= $4
	AND e.end_date >= $4
	AND e.path @> target.path
ORDER BY (target.depth - e.depth) ASC, e.child_node_id ASC
`, pgUUID(tenantID), hierarchyType, pgUUID(descendantNodeID), pgValidDate(asOf))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 16)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	case services.DeepReadBackendClosure:
		buildID, err := activeClosureBuildID(ctx, tx, tenantID, hierarchyType)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `
	SELECT ancestor_node_id, depth
	FROM org_hierarchy_closure
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND build_id=$3
	  AND descendant_node_id=$4
	  AND effective_date <= $5
	  AND end_date >= $5
	ORDER BY depth ASC, ancestor_node_id ASC
	`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), pgUUID(descendantNodeID), pgValidDate(asOf))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 16)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	case services.DeepReadBackendSnapshot:
		asOfDate := asOf.UTC().Format("2006-01-02")
		buildID, err := activeSnapshotBuildID(ctx, tx, tenantID, hierarchyType, asOfDate)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `
SELECT ancestor_node_id, depth
FROM org_hierarchy_snapshots
WHERE tenant_id=$1
  AND hierarchy_type=$2
  AND as_of_date=$3::date
  AND build_id=$4
  AND descendant_node_id=$5
ORDER BY depth ASC, ancestor_node_id ASC
`, pgUUID(tenantID), hierarchyType, asOfDate, pgUUID(buildID), pgUUID(descendantNodeID))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 16)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported deep read backend: %s", backend)
	}
}

func (r *OrgRepository) ListDescendantsAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, ancestorNodeID uuid.UUID, asOf time.Time, backend services.DeepReadBackend) ([]services.DeepReadRelation, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	switch backend {
	case services.DeepReadBackendEdges:
		rows, err := tx.Query(ctx, `
WITH target AS (
	SELECT path, depth
	FROM org_edges
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND child_node_id=$3
	  AND effective_date <= $4
	  AND end_date >= $4
	ORDER BY effective_date DESC
	LIMIT 1
)
SELECT e.child_node_id, (e.depth - target.depth) AS depth
FROM target
JOIN org_edges e
	ON e.tenant_id=$1
	AND e.hierarchy_type=$2
	AND e.effective_date <= $4
	AND e.end_date >= $4
	AND e.path <@ target.path
ORDER BY (e.depth - target.depth) ASC, e.child_node_id ASC
`, pgUUID(tenantID), hierarchyType, pgUUID(ancestorNodeID), pgValidDate(asOf))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 32)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	case services.DeepReadBackendClosure:
		buildID, err := activeClosureBuildID(ctx, tx, tenantID, hierarchyType)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `
	SELECT descendant_node_id, depth
	FROM org_hierarchy_closure
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND build_id=$3
	  AND ancestor_node_id=$4
	  AND effective_date <= $5
	  AND end_date >= $5
	ORDER BY depth ASC, descendant_node_id ASC
	`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), pgUUID(ancestorNodeID), pgValidDate(asOf))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 32)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	case services.DeepReadBackendSnapshot:
		asOfDate := asOf.UTC().Format("2006-01-02")
		buildID, err := activeSnapshotBuildID(ctx, tx, tenantID, hierarchyType, asOfDate)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `
SELECT descendant_node_id, depth
FROM org_hierarchy_snapshots
WHERE tenant_id=$1
  AND hierarchy_type=$2
  AND as_of_date=$3::date
  AND build_id=$4
  AND ancestor_node_id=$5
ORDER BY depth ASC, descendant_node_id ASC
`, pgUUID(tenantID), hierarchyType, asOfDate, pgUUID(buildID), pgUUID(ancestorNodeID))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, 32)
		for rows.Next() {
			var rel services.DeepReadRelation
			if err := rows.Scan(&rel.NodeID, &rel.Depth); err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported deep read backend: %s", backend)
	}
}
