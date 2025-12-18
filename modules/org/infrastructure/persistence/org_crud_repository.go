package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) HasRoot(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_nodes WHERE tenant_id=$1 AND is_root=true)`, pgUUID(tenantID)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) InsertNode(ctx context.Context, tenantID uuid.UUID, nodeType, code string, isRoot bool) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO org_nodes (tenant_id, type, code, is_root)
VALUES ($1, $2, $3, $4)
RETURNING id
`, pgUUID(tenantID), nodeType, code, isRoot).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) InsertNodeSlice(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, slice services.NodeSliceInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	i18nNames := "{}"
	if slice.I18nNames != nil {
		b, err := json.Marshal(slice.I18nNames)
		if err != nil {
			return uuid.Nil, err
		}
		i18nNames = string(b)
	}

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO org_node_slices (
	tenant_id,
	org_node_id,
	name,
	i18n_names,
	status,
	legal_entity_id,
	company_code,
	location_id,
	display_order,
	parent_hint,
	manager_user_id,
	effective_date,
	end_date
)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id
	`,
		pgUUID(tenantID),
		pgUUID(nodeID),
		slice.Name,
		i18nNames,
		slice.Status,
		pgNullableUUID(slice.LegalEntityID),
		pgNullableText(slice.CompanyCode),
		pgNullableUUID(slice.LocationID),
		slice.DisplayOrder,
		pgNullableUUID(slice.ParentHint),
		pgNullableInt8(slice.ManagerUserID),
		slice.EffectiveDate,
		slice.EndDate,
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) InsertEdge(ctx context.Context, tenantID uuid.UUID, hierarchyType string, parentID *uuid.UUID, childID uuid.UUID, effectiveDate, endDate time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO org_edges (tenant_id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id
`, pgUUID(tenantID), hierarchyType, pgNullableUUID(parentID), pgUUID(childID), effectiveDate, endDate).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) NodeExistsAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, hierarchyType string, asOf time.Time) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM org_edges
	WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date <= $4 AND end_date > $4
)`, pgUUID(tenantID), hierarchyType, pgUUID(nodeID), asOf).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) GetNode(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) (services.NodeRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.NodeRow{}, err
	}

	var out services.NodeRow
	if err := tx.QueryRow(ctx, `
	SELECT id, code, is_root, type
	FROM org_nodes
	WHERE tenant_id=$1 AND id=$2
	`, pgUUID(tenantID), pgUUID(nodeID)).Scan(&out.ID, &out.Code, &out.IsRoot, &out.Type); err != nil {
		return services.NodeRow{}, err
	}
	return out, nil
}

func (r *OrgRepository) GetNodeIsRoot(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var isRoot bool
	if err := tx.QueryRow(ctx, `SELECT is_root FROM org_nodes WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(nodeID)).Scan(&isRoot); err != nil {
		return false, err
	}
	return isRoot, nil
}

func (r *OrgRepository) GetNodeSliceAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (services.NodeSliceRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.NodeSliceRow{}, err
	}

	row := tx.QueryRow(ctx, `
	SELECT
		id,
		name,
		i18n_names,
		status,
		legal_entity_id,
		company_code,
		location_id,
		display_order,
		parent_hint,
		manager_user_id,
		effective_date,
		end_date
	FROM org_node_slices
	WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date <= $3 AND end_date > $3
	ORDER BY effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(nodeID), asOf)

	var out services.NodeSliceRow
	var i18nRaw []byte
	var legal pgtype.UUID
	var location pgtype.UUID
	var parentHint pgtype.UUID
	var company pgtype.Text
	var manager pgtype.Int8
	if err := row.Scan(
		&out.ID,
		&out.Name,
		&i18nRaw,
		&out.Status,
		&legal,
		&company,
		&location,
		&out.DisplayOrder,
		&parentHint,
		&manager,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.NodeSliceRow{}, err
	}

	if len(i18nRaw) > 0 {
		_ = json.Unmarshal(i18nRaw, &out.I18nNames)
	}

	out.LegalEntityID = nullableUUID(legal)
	out.LocationID = nullableUUID(location)
	out.ParentHint = nullableUUID(parentHint)
	out.CompanyCode = nullableText(company)
	out.ManagerUserID = nullableInt8(manager)

	return out, nil
}

func (r *OrgRepository) LockNodeSliceAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (services.NodeSliceRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.NodeSliceRow{}, err
	}

	row := tx.QueryRow(ctx, `
SELECT
	id,
	name,
	i18n_names,
	status,
	legal_entity_id,
	company_code,
	location_id,
	display_order,
	parent_hint,
	manager_user_id,
	effective_date,
	end_date
FROM org_node_slices
WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date <= $3 AND end_date > $3
ORDER BY effective_date DESC
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), pgUUID(nodeID), asOf)

	var out services.NodeSliceRow
	var i18nRaw []byte
	var legal pgtype.UUID
	var location pgtype.UUID
	var parentHint pgtype.UUID
	var company pgtype.Text
	var manager pgtype.Int8
	if err := row.Scan(
		&out.ID,
		&out.Name,
		&i18nRaw,
		&out.Status,
		&legal,
		&company,
		&location,
		&out.DisplayOrder,
		&parentHint,
		&manager,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.NodeSliceRow{}, err
	}

	if len(i18nRaw) > 0 {
		_ = json.Unmarshal(i18nRaw, &out.I18nNames)
	}

	out.LegalEntityID = nullableUUID(legal)
	out.LocationID = nullableUUID(location)
	out.ParentHint = nullableUUID(parentHint)
	out.CompanyCode = nullableText(company)
	out.ManagerUserID = nullableInt8(manager)

	return out, nil
}

func (r *OrgRepository) TruncateNodeSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_node_slices SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), endDate)
	return err
}

func (r *OrgRepository) NextNodeSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, after time.Time) (time.Time, bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return time.Time{}, false, err
	}

	var next time.Time
	err = tx.QueryRow(ctx, `
SELECT effective_date
FROM org_node_slices
WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date > $3
ORDER BY effective_date ASC
LIMIT 1
`, pgUUID(tenantID), pgUUID(nodeID), after).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return next, true, nil
}

func (r *OrgRepository) LockEdgeAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, asOf time.Time) (services.EdgeRow, error) {
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
WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date <= $4 AND end_date > $4
ORDER BY effective_date DESC
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), hierarchyType, pgUUID(childID), asOf)

	var out services.EdgeRow
	var parent pgtype.UUID
	if err := row.Scan(&out.ID, &parent, &out.ChildNodeID, &out.Path, &out.Depth, &out.EffectiveDate, &out.EndDate); err != nil {
		return services.EdgeRow{}, err
	}
	out.ParentNodeID = nullableUUID(parent)
	return out, nil
}

func (r *OrgRepository) LockEdgesInSubtree(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time, movedPath string) ([]services.EdgeRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
SELECT
	id,
	parent_node_id,
	child_node_id,
	path::text,
	depth,
	effective_date,
	end_date
FROM org_edges
WHERE tenant_id=$1 AND hierarchy_type=$2 AND effective_date <= $3 AND end_date > $3 AND path <@ $4::ltree
ORDER BY depth ASC
FOR UPDATE
`, pgUUID(tenantID), hierarchyType, asOf, movedPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.EdgeRow, 0, 64)
	for rows.Next() {
		var e services.EdgeRow
		var parent pgtype.UUID
		if err := rows.Scan(&e.ID, &parent, &e.ChildNodeID, &e.Path, &e.Depth, &e.EffectiveDate, &e.EndDate); err != nil {
			return nil, err
		}
		e.ParentNodeID = nullableUUID(parent)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *OrgRepository) TruncateEdge(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_edges SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(edgeID), endDate)
	return err
}

func (r *OrgRepository) PositionExistsAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM org_positions
	WHERE tenant_id=$1 AND id=$2 AND effective_date <= $3 AND end_date > $3
)
`, pgUUID(tenantID), pgUUID(positionID), asOf).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) InsertAutoPosition(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, orgNodeID uuid.UUID, code string, effectiveDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO org_positions (
	tenant_id,
	id,
	org_node_id,
	code,
	status,
	is_auto_created,
	effective_date,
	end_date
)
VALUES ($1,$2,$3,$4,'active',true,$5,$6)
ON CONFLICT (id) DO NOTHING
`, pgUUID(tenantID), pgUUID(positionID), pgUUID(orgNodeID), code, effectiveDate, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	return err
}

func (r *OrgRepository) GetPositionOrgNodeAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var orgNodeID uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT org_node_id
FROM org_positions
WHERE tenant_id=$1 AND id=$2 AND effective_date <= $3 AND end_date > $3
ORDER BY effective_date DESC
LIMIT 1
`, pgUUID(tenantID), pgUUID(positionID), asOf).Scan(&orgNodeID); err != nil {
		return uuid.Nil, err
	}
	return orgNodeID, nil
}

func (r *OrgRepository) LockAssignmentAt(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, asOf time.Time) (services.AssignmentRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.AssignmentRow{}, err
	}
	row := tx.QueryRow(ctx, `
SELECT
	id,
	position_id,
	subject_type,
	subject_id,
	pernr,
	assignment_type,
	is_primary,
	effective_date,
	end_date
FROM org_assignments
WHERE tenant_id=$1 AND id=$2 AND effective_date <= $3 AND end_date > $3
FOR UPDATE
`, pgUUID(tenantID), pgUUID(assignmentID), asOf)
	var out services.AssignmentRow
	if err := row.Scan(
		&out.ID,
		&out.PositionID,
		&out.SubjectType,
		&out.SubjectID,
		&out.Pernr,
		&out.AssignmentType,
		&out.IsPrimary,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.AssignmentRow{}, err
	}
	return out, nil
}

func (r *OrgRepository) TruncateAssignment(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_assignments SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(assignmentID), endDate)
	return err
}

func (r *OrgRepository) NextAssignmentEffectiveDate(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, after time.Time) (time.Time, bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	var next time.Time
	err = tx.QueryRow(ctx, `
SELECT effective_date
FROM org_assignments
WHERE tenant_id=$1 AND id=$2 AND effective_date > $3
ORDER BY effective_date ASC
LIMIT 1
`, pgUUID(tenantID), pgUUID(assignmentID), after).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return next, true, nil
}

func (r *OrgRepository) InsertAssignment(ctx context.Context, tenantID uuid.UUID, assignment services.AssignmentInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO org_assignments (
	tenant_id,
	position_id,
	subject_type,
	subject_id,
	pernr,
	assignment_type,
	is_primary,
	effective_date,
	end_date
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id
`, pgUUID(tenantID), pgUUID(assignment.PositionID), assignment.SubjectType, pgUUID(assignment.SubjectID), assignment.Pernr, assignment.AssignmentType, assignment.IsPrimary, assignment.EffectiveDate, assignment.EndDate).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) ListAssignmentsTimeline(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID) ([]services.AssignmentViewRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT
	a.id,
	a.position_id,
	p.org_node_id,
	a.assignment_type,
	a.is_primary,
	a.effective_date,
	a.end_date
FROM org_assignments a
JOIN org_positions p
	ON p.tenant_id=a.tenant_id
	AND p.id=a.position_id
	AND p.effective_date <= a.effective_date
	AND p.end_date > a.effective_date
WHERE a.tenant_id=$1 AND a.subject_id=$2
ORDER BY a.effective_date ASC
`, pgUUID(tenantID), pgUUID(subjectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.AssignmentViewRow, 0, 16)
	for rows.Next() {
		var v services.AssignmentViewRow
		if err := rows.Scan(&v.ID, &v.PositionID, &v.OrgNodeID, &v.AssignmentType, &v.IsPrimary, &v.EffectiveDate, &v.EndDate); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListAssignmentsAsOf(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID, asOf time.Time) ([]services.AssignmentViewRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT
	a.id,
	a.position_id,
	p.org_node_id,
	a.assignment_type,
	a.is_primary,
	a.effective_date,
	a.end_date
FROM org_assignments a
JOIN org_positions p
	ON p.tenant_id=a.tenant_id
	AND p.id=a.position_id
	AND p.effective_date <= $3
	AND p.end_date > $3
WHERE a.tenant_id=$1 AND a.subject_id=$2 AND a.effective_date <= $3 AND a.end_date > $3
ORDER BY a.effective_date DESC
`, pgUUID(tenantID), pgUUID(subjectID), asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.AssignmentViewRow, 0, 16)
	for rows.Next() {
		var v services.AssignmentViewRow
		if err := rows.Scan(&v.ID, &v.PositionID, &v.OrgNodeID, &v.AssignmentType, &v.IsPrimary, &v.EffectiveDate, &v.EndDate); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func pgNullableUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil || *id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func pgNullableText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func pgNullableInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func nullableUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func nullableText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func nullableInt8(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	i := v.Int64
	return &i
}
