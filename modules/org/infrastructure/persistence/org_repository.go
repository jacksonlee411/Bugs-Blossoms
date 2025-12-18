package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type OrgRepository struct{}

func NewOrgRepository() *OrgRepository {
	return &OrgRepository{}
}

func (r *OrgRepository) ListHierarchyAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]services.HierarchyNode, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT
	n.id,
	n.code,
	s.name,
	e.parent_node_id,
	e.depth,
	s.display_order,
	s.status
FROM org_nodes n
JOIN org_node_slices s
	ON s.tenant_id = n.tenant_id
	AND s.org_node_id = n.id
	AND s.effective_date <= $2
	AND s.end_date > $2
JOIN org_edges e
	ON e.tenant_id = n.tenant_id
	AND e.child_node_id = n.id
	AND e.hierarchy_type = $3
	AND e.effective_date <= $2
	AND e.end_date > $2
WHERE n.tenant_id = $1
ORDER BY e.depth ASC, s.display_order ASC, s.name ASC
`, pgUUID(tenantID), asOf, hierarchyType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.HierarchyNode, 0, 64)
	for rows.Next() {
		var node services.HierarchyNode
		var parent pgtype.UUID
		if err := rows.Scan(&node.ID, &node.Code, &node.Name, &parent, &node.Depth, &node.DisplayOrder, &node.Status); err != nil {
			return nil, err
		}
		if parent.Valid {
			pid := uuid.UUID(parent.Bytes)
			node.ParentID = &pid
		}
		out = append(out, node)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}
