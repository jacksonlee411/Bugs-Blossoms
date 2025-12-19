package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListAttributeInheritanceRulesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]services.AttributeInheritanceRule, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
	SELECT
		attribute_name,
		can_override,
		inheritance_break_node_type
	FROM org_attribute_inheritance_rules
	WHERE tenant_id=$1
		AND hierarchy_type=$2
		AND effective_date <= $3
		AND end_date > $3
	ORDER BY attribute_name ASC
	`, pgUUID(tenantID), hierarchyType, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.AttributeInheritanceRule, 0, 8)
	for rows.Next() {
		var row services.AttributeInheritanceRule
		var breakType pgtype.Text
		if err := rows.Scan(&row.AttributeName, &row.CanOverride, &breakType); err != nil {
			return nil, err
		}
		row.InheritanceBreakNodeType = nullableText(breakType)
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListNodeAttributesAsOf(ctx context.Context, tenantID uuid.UUID, nodeIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID]services.OrgNodeAttributes, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]services.OrgNodeAttributes, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
	SELECT
		org_node_id,
		legal_entity_id,
		company_code,
		location_id,
		manager_user_id
	FROM org_node_slices
	WHERE tenant_id=$1
		AND org_node_id = ANY($2)
		AND effective_date <= $3
		AND end_date > $3
	`, pgUUID(tenantID), pgtype.FlatArray[uuid.UUID](nodeIDs), asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeID   uuid.UUID
			legal    pgtype.UUID
			company  pgtype.Text
			location pgtype.UUID
			manager  pgtype.Int8
		)
		if err := rows.Scan(&nodeID, &legal, &company, &location, &manager); err != nil {
			return nil, err
		}
		out[nodeID] = services.OrgNodeAttributes{
			LegalEntityID: nullableUUID(legal),
			CompanyCode:   nullableText(company),
			LocationID:    nullableUUID(location),
			ManagerUserID: nullableInt8(manager),
		}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]services.OrgRole, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
	SELECT id, code, name, description, is_system
	FROM org_roles
	WHERE tenant_id=$1
	ORDER BY name ASC
	`, pgUUID(tenantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.OrgRole, 0, 16)
	for rows.Next() {
		var r services.OrgRole
		var desc pgtype.Text
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &desc, &r.IsSystem); err != nil {
			return nil, err
		}
		r.Description = nullableText(desc)
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListRoleAssignmentsAsOf(
	ctx context.Context,
	tenantID uuid.UUID,
	hierarchyType string,
	orgNodeID uuid.UUID,
	asOf time.Time,
	includeInherited bool,
	backend services.DeepReadBackend,
	roleCode *string,
	subjectType *string,
	subjectID *uuid.UUID,
) ([]services.RoleAssignmentRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	type query struct {
		sql  string
		args []any
	}

	var q query
	if includeInherited {
		switch backend {
		case services.DeepReadBackendEdges:
			var path string
			err := tx.QueryRow(ctx, `
		SELECT path::text
		FROM org_edges
		WHERE tenant_id=$1
			AND hierarchy_type=$2
			AND child_node_id=$3
			AND effective_date <= $4
			AND end_date > $4
		ORDER BY effective_date DESC
		LIMIT 1
		`, pgUUID(tenantID), hierarchyType, pgUUID(orgNodeID), asOf).Scan(&path)
			if err != nil {
				return nil, err
			}

			q.sql = `
		SELECT
			a.id,
			a.role_id,
			r.code,
			a.subject_type,
			a.subject_id,
			a.org_node_id,
			a.effective_date,
			a.end_date
		FROM org_role_assignments a
		JOIN org_roles r
			ON r.tenant_id=a.tenant_id
			AND r.id=a.role_id
		JOIN org_edges e
			ON e.tenant_id=a.tenant_id
			AND e.hierarchy_type=$2
			AND e.child_node_id=a.org_node_id
			AND e.effective_date <= $4
			AND e.end_date > $4
		WHERE a.tenant_id=$1
			AND a.effective_date <= $4
			AND a.end_date > $4
			AND e.path @> $3::ltree
		`
			q.args = []any{pgUUID(tenantID), hierarchyType, path, asOf}
		case services.DeepReadBackendClosure:
			buildID, err := activeClosureBuildID(ctx, tx, tenantID, hierarchyType)
			if err != nil {
				return nil, err
			}

			q.sql = `
		SELECT
			a.id,
			a.role_id,
			r.code,
			a.subject_type,
			a.subject_id,
			a.org_node_id,
			a.effective_date,
			a.end_date
		FROM org_role_assignments a
		JOIN org_roles r
			ON r.tenant_id=a.tenant_id
			AND r.id=a.role_id
			JOIN org_hierarchy_closure c
				ON c.tenant_id=a.tenant_id
				AND c.hierarchy_type=$2
				AND c.build_id=$3
				AND c.ancestor_node_id=a.org_node_id
				AND c.descendant_node_id=$4
				AND tstzrange(c.effective_date, c.end_date, '[)') @> $5::timestamptz
			WHERE a.tenant_id=$1
				AND a.effective_date <= $5
				AND a.end_date > $5
			`
			q.args = []any{pgUUID(tenantID), hierarchyType, pgUUID(buildID), pgUUID(orgNodeID), asOf}
		case services.DeepReadBackendSnapshot:
			asOfDate := asOf.UTC().Format("2006-01-02")
			buildID, err := activeSnapshotBuildID(ctx, tx, tenantID, hierarchyType, asOfDate)
			if err != nil {
				return nil, err
			}

			q.sql = `
		SELECT
			a.id,
			a.role_id,
			r.code,
			a.subject_type,
			a.subject_id,
			a.org_node_id,
			a.effective_date,
			a.end_date
		FROM org_role_assignments a
		JOIN org_roles r
			ON r.tenant_id=a.tenant_id
			AND r.id=a.role_id
		JOIN org_hierarchy_snapshots s
			ON s.tenant_id=a.tenant_id
			AND s.hierarchy_type=$2
			AND s.as_of_date=$3::date
			AND s.build_id=$4
			AND s.ancestor_node_id=a.org_node_id
			AND s.descendant_node_id=$5
		WHERE a.tenant_id=$1
			AND a.effective_date <= $6
			AND a.end_date > $6
		`
			q.args = []any{pgUUID(tenantID), hierarchyType, asOfDate, pgUUID(buildID), pgUUID(orgNodeID), asOf}
		default:
			return nil, fmt.Errorf("unsupported deep read backend: %s", backend)
		}
	} else {
		q.sql = `
		SELECT
			a.id,
			a.role_id,
			r.code,
			a.subject_type,
			a.subject_id,
			a.org_node_id,
			a.effective_date,
			a.end_date
		FROM org_role_assignments a
		JOIN org_roles r
			ON r.tenant_id=a.tenant_id
			AND r.id=a.role_id
		WHERE a.tenant_id=$1
			AND a.org_node_id=$2
			AND a.effective_date <= $3
			AND a.end_date > $3
		`
		q.args = []any{pgUUID(tenantID), pgUUID(orgNodeID), asOf}
	}

	if roleCode != nil && strings.TrimSpace(*roleCode) != "" {
		q.sql += fmt.Sprintf(" AND r.code = $%d", len(q.args)+1)
		q.args = append(q.args, strings.TrimSpace(*roleCode))
	}
	if subjectType != nil && strings.TrimSpace(*subjectType) != "" && subjectID != nil && *subjectID != uuid.Nil {
		q.sql += fmt.Sprintf(" AND a.subject_type = $%d AND a.subject_id = $%d", len(q.args)+1, len(q.args)+2)
		q.args = append(q.args, strings.TrimSpace(*subjectType), pgUUID(*subjectID))
	}

	q.sql += " ORDER BY r.code ASC, a.subject_type ASC, a.subject_id ASC, a.org_node_id ASC, a.effective_date DESC"

	rows, err := tx.Query(ctx, q.sql, q.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.RoleAssignmentRow, 0, 16)
	for rows.Next() {
		var row services.RoleAssignmentRow
		if err := rows.Scan(
			&row.AssignmentID,
			&row.RoleID,
			&row.RoleCode,
			&row.SubjectType,
			&row.SubjectID,
			&row.SourceOrgNodeID,
			&row.EffectiveDate,
			&row.EndDate,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) GetEdgeAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, asOf time.Time) (services.EdgeRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.EdgeRow{}, err
	}

	row := tx.QueryRow(ctx, `
	SELECT
		id,
		parent_node_id,
		child_node_id,
		path::text,
		depth,
		effective_date,
		end_date
	FROM org_edges
	WHERE tenant_id=$1
		AND hierarchy_type=$2
		AND child_node_id=$3
		AND effective_date <= $4
		AND end_date > $4
	ORDER BY effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), hierarchyType, pgUUID(childID), asOf)

	var out services.EdgeRow
	var parent pgtype.UUID
	if err := row.Scan(&out.ID, &parent, &out.ChildNodeID, &out.Path, &out.Depth, &out.EffectiveDate, &out.EndDate); err != nil {
		return services.EdgeRow{}, err
	}
	out.ParentNodeID = nullableUUID(parent)
	return out, nil
}
