package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) GetTenantRootNodeID(ctx context.Context, tenantID uuid.UUID) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT id
FROM org_nodes
WHERE tenant_id=$1 AND is_root=true
LIMIT 1
`, pgUUID(tenantID)).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) ListOrgNodesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, nodeIDs []uuid.UUID, asOf time.Time) ([]services.OrgNodeAsOfRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodeIDs) == 0 {
		return []services.OrgNodeAsOfRow{}, nil
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	rows, err := tx.Query(ctx, `
SELECT
	n.id,
	n.code,
	s.name,
	s.status,
	e.parent_node_id
FROM org_nodes n
JOIN org_node_slices s
	ON s.tenant_id = n.tenant_id
	AND s.org_node_id = n.id
	AND s.effective_date <= $3
	AND s.end_date > $3
LEFT JOIN org_edges e
	ON e.tenant_id = n.tenant_id
	AND e.child_node_id = n.id
	AND e.hierarchy_type = $4
	AND e.effective_date <= $3
	AND e.end_date > $3
WHERE n.tenant_id = $1
  AND n.id = ANY($2::uuid[])
ORDER BY n.id ASC
`, pgUUID(tenantID), nodeIDs, asOf, hierarchyType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.OrgNodeAsOfRow, 0, len(nodeIDs))
	for rows.Next() {
		var row services.OrgNodeAsOfRow
		var parent pgtype.UUID
		if err := rows.Scan(&row.ID, &row.Code, &row.Name, &row.Status, &parent); err != nil {
			return nil, err
		}
		if parent.Valid {
			p := uuid.UUID(parent.Bytes)
			row.ParentID = &p
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListDescendantsForExportAsOf(
	ctx context.Context,
	tenantID uuid.UUID,
	hierarchyType string,
	rootNodeID uuid.UUID,
	asOf time.Time,
	backend services.DeepReadBackend,
	afterNodeID *uuid.UUID,
	maxDepth *int,
	limit int,
) ([]services.DeepReadRelation, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 1
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
	  AND end_date > $4
	ORDER BY effective_date DESC
	LIMIT 1
)
SELECT e.child_node_id, (e.depth - target.depth) AS depth
FROM target
JOIN org_edges e
	ON e.tenant_id=$1
	AND e.hierarchy_type=$2
	AND e.effective_date <= $4
	AND e.end_date > $4
	AND e.path <@ target.path
WHERE ($5::uuid IS NULL OR e.child_node_id > $5::uuid)
  AND ($6::int IS NULL OR (e.depth - target.depth) <= $6::int)
ORDER BY e.child_node_id ASC
LIMIT $7
`, pgUUID(tenantID), hierarchyType, pgUUID(rootNodeID), asOf, pgNullableUUID(afterNodeID), maxDepth, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, minInt(limit, 64))
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
  AND tstzrange(effective_date, end_date, '[)') @> $5::timestamptz
  AND ($6::uuid IS NULL OR descendant_node_id > $6::uuid)
  AND ($7::int IS NULL OR depth <= $7::int)
ORDER BY descendant_node_id ASC
LIMIT $8
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), pgUUID(rootNodeID), asOf, pgNullableUUID(afterNodeID), maxDepth, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, minInt(limit, 64))
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
  AND ($6::uuid IS NULL OR descendant_node_id > $6::uuid)
  AND ($7::int IS NULL OR depth <= $7::int)
ORDER BY descendant_node_id ASC
LIMIT $8
`, pgUUID(tenantID), hierarchyType, asOfDate, pgUUID(buildID), pgUUID(rootNodeID), pgNullableUUID(afterNodeID), maxDepth, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]services.DeepReadRelation, 0, minInt(limit, 64))
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

func (r *OrgRepository) ResolveSecurityGroupKeysForNodesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, nodeIDs []uuid.UUID, asOf time.Time, backend services.DeepReadBackend) (map[uuid.UUID][]string, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodeIDs) == 0 {
		return map[uuid.UUID][]string{}, nil
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	var rows pgx.Rows
	switch backend {
	case services.DeepReadBackendEdges:
		rows, err = tx.Query(ctx, `
WITH targets AS (
	SELECT DISTINCT ON (child_node_id) child_node_id, path, depth
	FROM org_edges
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND child_node_id = ANY($3::uuid[])
	  AND effective_date <= $4
	  AND end_date > $4
	ORDER BY child_node_id, effective_date DESC
),
rels AS (
	SELECT
		t.child_node_id AS descendant_node_id,
		e.child_node_id AS ancestor_node_id,
		(t.depth - e.depth) AS depth
	FROM targets t
	JOIN org_edges e
		ON e.tenant_id=$1
		AND e.hierarchy_type=$2
		AND e.effective_date <= $4
		AND e.end_date > $4
		AND e.path @> t.path
),
best AS (
	SELECT DISTINCT ON (r.descendant_node_id, m.security_group_key)
		r.descendant_node_id AS org_node_id,
		m.security_group_key,
		r.depth AS source_depth,
		m.org_node_id AS source_org_node_id
	FROM rels r
	JOIN org_security_group_mappings m
		ON m.tenant_id = $1
		AND m.org_node_id = r.ancestor_node_id
	WHERE m.effective_date <= $4
		AND m.end_date > $4
		AND (m.applies_to_subtree OR m.org_node_id = r.descendant_node_id)
	ORDER BY r.descendant_node_id, m.security_group_key, r.depth ASC, m.org_node_id ASC
),
best_dedup AS (
	SELECT org_node_id, security_group_key, MIN(source_depth) AS min_depth
	FROM best
	GROUP BY org_node_id, security_group_key
)
SELECT org_node_id, array_agg(security_group_key ORDER BY min_depth ASC, security_group_key ASC) AS keys
FROM best_dedup
GROUP BY org_node_id
`, pgUUID(tenantID), hierarchyType, nodeIDs, asOf)
		if err != nil {
			return nil, err
		}
	case services.DeepReadBackendClosure:
		buildID, err := activeClosureBuildID(ctx, tx, tenantID, hierarchyType)
		if err != nil {
			return nil, err
		}
		rows, err = tx.Query(ctx, `
WITH rels AS (
	SELECT descendant_node_id, ancestor_node_id, depth
	FROM org_hierarchy_closure
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND build_id=$3
	  AND descendant_node_id = ANY($4::uuid[])
	  AND tstzrange(effective_date, end_date, '[)') @> $5::timestamptz
),
best AS (
	SELECT DISTINCT ON (r.descendant_node_id, m.security_group_key)
		r.descendant_node_id AS org_node_id,
		m.security_group_key,
		r.depth AS source_depth,
		m.org_node_id AS source_org_node_id
	FROM rels r
	JOIN org_security_group_mappings m
		ON m.tenant_id = $1
		AND m.org_node_id = r.ancestor_node_id
	WHERE m.effective_date <= $5
		AND m.end_date > $5
		AND (m.applies_to_subtree OR m.org_node_id = r.descendant_node_id)
	ORDER BY r.descendant_node_id, m.security_group_key, r.depth ASC, m.org_node_id ASC
),
best_dedup AS (
	SELECT org_node_id, security_group_key, MIN(source_depth) AS min_depth
	FROM best
	GROUP BY org_node_id, security_group_key
)
SELECT org_node_id, array_agg(security_group_key ORDER BY min_depth ASC, security_group_key ASC) AS keys
FROM best_dedup
GROUP BY org_node_id
`, pgUUID(tenantID), hierarchyType, pgUUID(buildID), nodeIDs, asOf)
		if err != nil {
			return nil, err
		}
	case services.DeepReadBackendSnapshot:
		asOfDate := asOf.UTC().Format("2006-01-02")
		buildID, err := activeSnapshotBuildID(ctx, tx, tenantID, hierarchyType, asOfDate)
		if err != nil {
			return nil, err
		}
		rows, err = tx.Query(ctx, `
WITH rels AS (
	SELECT descendant_node_id, ancestor_node_id, depth
	FROM org_hierarchy_snapshots
	WHERE tenant_id=$1
	  AND hierarchy_type=$2
	  AND as_of_date=$3::date
	  AND build_id=$4
	  AND descendant_node_id = ANY($5::uuid[])
),
best AS (
	SELECT DISTINCT ON (r.descendant_node_id, m.security_group_key)
		r.descendant_node_id AS org_node_id,
		m.security_group_key,
		r.depth AS source_depth,
		m.org_node_id AS source_org_node_id
	FROM rels r
	JOIN org_security_group_mappings m
		ON m.tenant_id = $1
		AND m.org_node_id = r.ancestor_node_id
	WHERE m.effective_date <= $6
		AND m.end_date > $6
		AND (m.applies_to_subtree OR m.org_node_id = r.descendant_node_id)
	ORDER BY r.descendant_node_id, m.security_group_key, r.depth ASC, m.org_node_id ASC
),
best_dedup AS (
	SELECT org_node_id, security_group_key, MIN(source_depth) AS min_depth
	FROM best
	GROUP BY org_node_id, security_group_key
)
SELECT org_node_id, array_agg(security_group_key ORDER BY min_depth ASC, security_group_key ASC) AS keys
FROM best_dedup
GROUP BY org_node_id
`, pgUUID(tenantID), hierarchyType, asOfDate, pgUUID(buildID), nodeIDs, asOf)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported deep read backend: %s", backend)
	}
	defer rows.Close()

	out := map[uuid.UUID][]string{}
	for rows.Next() {
		var nodeID uuid.UUID
		var keys []string
		if err := rows.Scan(&nodeID, &keys); err != nil {
			return nil, err
		}
		out[nodeID] = keys
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListOrgLinkSummariesForNodesAsOf(ctx context.Context, tenantID uuid.UUID, nodeIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID][]services.OrgLinkSummary, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodeIDs) == 0 {
		return map[uuid.UUID][]services.OrgLinkSummary{}, nil
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	rows, err := tx.Query(ctx, `
SELECT
	org_node_id,
	object_type,
	object_key,
	link_type
FROM org_links
WHERE tenant_id=$1
  AND org_node_id = ANY($2::uuid[])
  AND effective_date <= $3
  AND end_date > $3
ORDER BY org_node_id ASC, object_type ASC, object_key ASC, link_type ASC
`, pgUUID(tenantID), nodeIDs, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[uuid.UUID][]services.OrgLinkSummary{}
	for rows.Next() {
		var nodeID uuid.UUID
		var item services.OrgLinkSummary
		if err := rows.Scan(&nodeID, &item.ObjectType, &item.ObjectKey, &item.LinkType); err != nil {
			return nil, err
		}
		out[nodeID] = append(out[nodeID], item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}
