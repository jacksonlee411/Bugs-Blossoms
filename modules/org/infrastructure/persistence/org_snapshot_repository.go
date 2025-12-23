package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListSnapshotNodes(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]services.SnapshotItem, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
SELECT
  n.id,
  jsonb_build_object(
    'org_node_id', n.id,
    'is_root', n.is_root,
    'code', n.code,
    'name', s.name,
    'status', s.status,
    'parent_node_id', e.parent_node_id,
    'effective_date', s.effective_date,
    'end_date', s.end_date
  ) AS new_values
FROM org_nodes n
JOIN org_node_slices s
  ON s.tenant_id = n.tenant_id
  AND s.org_node_id = n.id
  AND s.effective_date <= $2
  AND s.end_date > $2
LEFT JOIN org_edges e
  ON e.tenant_id = n.tenant_id
  AND e.child_node_id = n.id
  AND e.hierarchy_type = 'OrgUnit'
  AND e.effective_date <= $2
  AND e.end_date > $2
WHERE n.tenant_id = $1
  AND ($3::uuid IS NULL OR n.id > $3)
`
	args := []any{pgUUID(tenantID), asOf, pgNullableUUID(afterID), limit}
	q += " ORDER BY n.id ASC LIMIT $4"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SnapshotItem, 0, minInt(limit, 64))
	for rows.Next() {
		var id uuid.UUID
		var raw json.RawMessage
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out = append(out, services.SnapshotItem{
			EntityType: "org_node",
			EntityID:   id,
			NewValues:  raw,
		})
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListSnapshotEdges(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]services.SnapshotItem, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
SELECT
  e.id,
  jsonb_build_object(
    'edge_id', e.id,
    'hierarchy_type', e.hierarchy_type,
    'parent_node_id', e.parent_node_id,
    'child_node_id', e.child_node_id,
    'effective_date', e.effective_date,
    'end_date', e.end_date
  ) AS new_values
FROM org_edges e
WHERE e.tenant_id = $1
  AND e.hierarchy_type = 'OrgUnit'
  AND e.effective_date <= $2
  AND e.end_date > $2
  AND ($3::uuid IS NULL OR e.id > $3)
`
	args := []any{pgUUID(tenantID), asOf, pgNullableUUID(afterID), limit}
	q += " ORDER BY e.id ASC LIMIT $4"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SnapshotItem, 0, minInt(limit, 64))
	for rows.Next() {
		var id uuid.UUID
		var raw json.RawMessage
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out = append(out, services.SnapshotItem{
			EntityType: "org_edge",
			EntityID:   id,
			NewValues:  raw,
		})
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListSnapshotPositions(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]services.SnapshotItem, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
	SELECT
	  p.id,
	  jsonb_build_object(
	    'position_id', p.id,
	    'org_node_id', s.org_node_id,
	    'code', p.code,
	    'lifecycle_status', s.lifecycle_status,
	    'is_auto_created', p.is_auto_created,
	    'capacity_fte', s.capacity_fte,
	    'effective_date', s.effective_date,
	    'end_date', s.end_date
	  ) AS new_values
	FROM org_positions p
	JOIN org_position_slices s
	  ON s.tenant_id = p.tenant_id
	  AND s.position_id = p.id
	  AND s.effective_date <= $2
	  AND s.end_date > $2
	WHERE p.tenant_id = $1
	  AND ($3::uuid IS NULL OR p.id > $3)
	`
	args := []any{pgUUID(tenantID), asOf, pgNullableUUID(afterID), limit}
	q += " ORDER BY p.id ASC LIMIT $4"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SnapshotItem, 0, minInt(limit, 64))
	for rows.Next() {
		var id uuid.UUID
		var raw json.RawMessage
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out = append(out, services.SnapshotItem{
			EntityType: "org_position",
			EntityID:   id,
			NewValues:  raw,
		})
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListSnapshotAssignments(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]services.SnapshotItem, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
	SELECT
	  a.id,
	  jsonb_build_object(
	    'org_assignment_id', a.id,
	    'position_id', a.position_id,
	    'subject_type', a.subject_type,
	    'subject_id', a.subject_id,
	    'pernr', a.pernr,
	    'assignment_type', a.assignment_type,
	    'is_primary', a.is_primary,
	    'allocated_fte', a.allocated_fte,
	    'effective_date', a.effective_date,
	    'end_date', a.end_date
	  ) AS new_values
	FROM org_assignments a
	WHERE a.tenant_id = $1
  AND a.effective_date <= $2
  AND a.end_date > $2
  AND ($3::uuid IS NULL OR a.id > $3)
`
	args := []any{pgUUID(tenantID), asOf, pgNullableUUID(afterID), limit}
	q += " ORDER BY a.id ASC LIMIT $4"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SnapshotItem, 0, minInt(limit, 64))
	for rows.Next() {
		var id uuid.UUID
		var raw json.RawMessage
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out = append(out, services.SnapshotItem{
			EntityType: "org_assignment",
			EntityID:   id,
			NewValues:  raw,
		})
	}
	return out, rows.Err()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
