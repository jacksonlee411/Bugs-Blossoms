package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) HasChildrenAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM org_edges
	WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND parent_node_id=$2 AND effective_date <= $3 AND end_date > $3
)
`, pgUUID(tenantID), pgUUID(nodeID), asOf.UTC()).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) HasPositionsAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM org_positions
	WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date <= $3 AND end_date > $3
)
`, pgUUID(tenantID), pgUUID(nodeID), asOf.UTC()).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) LockNodeSliceStartingAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, effectiveDate time.Time) (services.NodeSliceRow, error) {
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
WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date=$3
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), pgUUID(nodeID), effectiveDate.UTC())

	return scanNodeSliceRow(row)
}

func (r *OrgRepository) LockNodeSliceEndingAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, endDate time.Time) (services.NodeSliceRow, error) {
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
WHERE tenant_id=$1 AND org_node_id=$2 AND end_date=$3
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), pgUUID(nodeID), endDate.UTC())

	return scanNodeSliceRow(row)
}

type pgRow interface {
	Scan(dest ...any) error
}

func scanNodeSliceRow(row pgRow) (services.NodeSliceRow, error) {
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

func (r *OrgRepository) UpdateNodeSliceInPlace(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, patch services.NodeSliceInPlacePatch) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	var i18nJSON string
	hasI18n := patch.I18nNames != nil
	if hasI18n {
		b, err := json.Marshal(patch.I18nNames)
		if err != nil {
			return err
		}
		i18nJSON = string(b)
	}

	setLegal := patch.LegalEntityID != nil
	setCompany := patch.CompanyCode != nil
	setLocation := patch.LocationID != nil
	setParentHint := patch.ParentHint != nil
	setManager := patch.ManagerUserID != nil

	_, err = tx.Exec(ctx, `
UPDATE org_node_slices
SET
	name = COALESCE($3, name),
	i18n_names = CASE WHEN $4 THEN $5::jsonb ELSE i18n_names END,
	status = COALESCE($6, status),
	display_order = COALESCE($7, display_order),
	legal_entity_id = CASE WHEN $8 THEN $9 ELSE legal_entity_id END,
	company_code = CASE WHEN $10 THEN $11 ELSE company_code END,
	location_id = CASE WHEN $12 THEN $13 ELSE location_id END,
	parent_hint = CASE WHEN $14 THEN $15 ELSE parent_hint END,
	manager_user_id = CASE WHEN $16 THEN $17 ELSE manager_user_id END,
	updated_at = now()
WHERE tenant_id=$1 AND id=$2
`, pgUUID(tenantID),
		pgUUID(sliceID),
		patch.Name,
		hasI18n,
		i18nJSON,
		patch.Status,
		patch.DisplayOrder,
		setLegal,
		pgNullableUUID(derefUUIDPtr(patch.LegalEntityID)),
		setCompany,
		pgNullableText(derefStringPtr(patch.CompanyCode)),
		setLocation,
		pgNullableUUID(derefUUIDPtr(patch.LocationID)),
		setParentHint,
		pgNullableUUID(derefUUIDPtr(patch.ParentHint)),
		setManager,
		pgNullableInt8(derefInt64Ptr(patch.ManagerUserID)),
	)
	return err
}

func (r *OrgRepository) UpdateNodeSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, effectiveDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_node_slices SET effective_date=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), effectiveDate.UTC())
	return err
}

func (r *OrgRepository) UpdateNodeSliceEndDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_node_slices SET end_date=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), endDate.UTC())
	return err
}

func (r *OrgRepository) DeleteNodeSlicesFrom(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, from time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_node_slices WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date >= $3`, pgUUID(tenantID), pgUUID(nodeID), from.UTC())
	return err
}

func (r *OrgRepository) LockEdgeStartingAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, effectiveDate time.Time) (services.EdgeRow, error) {
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
WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date=$4
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), hierarchyType, pgUUID(childID), effectiveDate.UTC())

	var out services.EdgeRow
	var parent pgtype.UUID
	if err := row.Scan(&out.ID, &parent, &out.ChildNodeID, &out.Path, &out.Depth, &out.EffectiveDate, &out.EndDate); err != nil {
		return services.EdgeRow{}, err
	}
	out.ParentNodeID = nullableUUID(parent)
	return out, nil
}

func (r *OrgRepository) DeleteEdgeByID(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_edges WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(edgeID))
	return err
}

func (r *OrgRepository) DeleteEdgesFrom(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, from time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_edges WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date >= $4`, pgUUID(tenantID), hierarchyType, pgUUID(childID), from.UTC())
	return err
}

func (r *OrgRepository) LockAssignmentByID(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID) (services.AssignmentRow, error) {
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
		allocated_fte,
		effective_date,
		end_date
	FROM org_assignments
	WHERE tenant_id=$1 AND id=$2
	FOR UPDATE
	`, pgUUID(tenantID), pgUUID(assignmentID))
	var out services.AssignmentRow
	if err := row.Scan(
		&out.ID,
		&out.PositionID,
		&out.SubjectType,
		&out.SubjectID,
		&out.Pernr,
		&out.AssignmentType,
		&out.IsPrimary,
		&out.AllocatedFTE,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.AssignmentRow{}, err
	}
	return out, nil
}

func (r *OrgRepository) UpdateAssignmentInPlace(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, patch services.AssignmentInPlacePatch) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	setPosition := patch.PositionID != nil
	setPernr := patch.Pernr != nil
	setSubject := patch.SubjectID != nil
	_, err = tx.Exec(ctx, `
UPDATE org_assignments
SET
	position_id = CASE WHEN $3 THEN $4 ELSE position_id END,
	pernr = CASE WHEN $5 THEN $6 ELSE pernr END,
	subject_id = CASE WHEN $7 THEN $8 ELSE subject_id END,
	updated_at = now()
WHERE tenant_id=$1 AND id=$2
`, pgUUID(tenantID),
		pgUUID(assignmentID),
		setPosition,
		pgNullableUUID(patch.PositionID),
		setPernr,
		derefString(patch.Pernr),
		setSubject,
		pgNullableUUID(patch.SubjectID),
	)
	return err
}

func (r *OrgRepository) UpdateAssignmentEndDate(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_assignments SET end_date=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(assignmentID), endDate.UTC())
	return err
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefUUIDPtr(v **uuid.UUID) *uuid.UUID {
	if v == nil {
		return nil
	}
	return *v
}

func derefStringPtr(v **string) *string {
	if v == nil {
		return nil
	}
	return *v
}

func derefInt64Ptr(v **int64) *int64 {
	if v == nil {
		return nil
	}
	return *v
}
