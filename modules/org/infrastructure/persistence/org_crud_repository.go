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
		pgValidDate(slice.EffectiveDate),
		pgValidDate(slice.EndDate),
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
	`, pgUUID(tenantID), hierarchyType, pgNullableUUID(parentID), pgUUID(childID), pgValidDate(effectiveDate), pgValidDate(endDate)).Scan(&id); err != nil {
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
	WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date <= $4 AND end_date >= $4
)`, pgUUID(tenantID), hierarchyType, pgUUID(nodeID), pgValidDate(asOf)).Scan(&exists); err != nil {
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
	WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date <= $3 AND end_date >= $3
	ORDER BY effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(nodeID), pgValidDate(asOf))

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
WHERE tenant_id=$1 AND org_node_id=$2 AND effective_date <= $3 AND end_date >= $3
ORDER BY effective_date DESC
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), pgUUID(nodeID), pgValidDate(asOf))

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
	_, err = tx.Exec(ctx, `UPDATE org_node_slices SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), pgValidDate(endDate))
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
`, pgUUID(tenantID), pgUUID(nodeID), pgValidDate(after)).Scan(&next)
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
WHERE tenant_id=$1 AND hierarchy_type=$2 AND child_node_id=$3 AND effective_date <= $4 AND end_date >= $4
ORDER BY effective_date DESC
LIMIT 1
FOR UPDATE
`, pgUUID(tenantID), hierarchyType, pgUUID(childID), pgValidDate(asOf))

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
WHERE tenant_id=$1 AND hierarchy_type=$2 AND effective_date <= $3 AND end_date >= $3 AND path <@ $4::ltree
ORDER BY depth ASC
FOR UPDATE
`, pgUUID(tenantID), hierarchyType, pgValidDate(asOf), movedPath)
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

func (r *OrgRepository) CountDescendantEdgesNeedingPathRewriteFrom(ctx context.Context, tenantID uuid.UUID, hierarchyType string, fromDate time.Time, oldPrefix string) (int, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return 0, err
	}

	var count int
	if err := tx.QueryRow(ctx, `
SELECT count(*) AS affected_edges
FROM org_edges e
WHERE e.tenant_id=$1
  AND e.hierarchy_type=$2
  AND e.effective_date >= $3::date
  AND e.path <@ $4::ltree
  AND nlevel(e.path) > nlevel($4::ltree)
`, pgUUID(tenantID), hierarchyType, pgValidDate(fromDate), oldPrefix).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *OrgRepository) RewriteDescendantEdgesPathPrefixFrom(ctx context.Context, tenantID uuid.UUID, hierarchyType string, fromDate time.Time, oldPrefix string, newPrefix string) (int64, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return 0, err
	}

	ct, err := tx.Exec(ctx, `
UPDATE org_edges e
SET
  path  = ($5::ltree) || subpath(e.path, nlevel($4::ltree)),
  depth = nlevel(($5::ltree) || subpath(e.path, nlevel($4::ltree))) - 1
WHERE e.tenant_id=$1
  AND e.hierarchy_type=$2
  AND e.effective_date >= $3::date
  AND e.path <@ $4::ltree
  AND nlevel(e.path) > nlevel($4::ltree)
`, pgUUID(tenantID), hierarchyType, pgValidDate(fromDate), oldPrefix, newPrefix)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (r *OrgRepository) TruncateEdge(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_edges SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(edgeID), pgValidDate(endDate))
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
		FROM org_position_slices
		WHERE tenant_id=$1 AND position_id=$2 AND effective_date <= $3 AND end_date >= $3
	)
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) InsertAutoPosition(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, orgNodeID uuid.UUID, code string, jobProfileID uuid.UUID, effectiveDate time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
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
			`,
		pgUUID(tenantID),
		pgUUID(positionID),
		pgUUID(orgNodeID),
		code,
		pgValidDate(effectiveDate),
		pgValidDate(endDate),
	)
	if err != nil {
		return uuid.Nil, err
	}

	var sliceID uuid.UUID
	if err := tx.QueryRow(ctx, `
	WITH existing AS (
		SELECT id
		FROM org_position_slices
		WHERE tenant_id=$1 AND position_id=$2 AND effective_date=$4 AND end_date=$5
	),
	ins AS (
		INSERT INTO org_position_slices (
			tenant_id,
			position_id,
			org_node_id,
			lifecycle_status,
			capacity_fte,
			job_profile_id,
			effective_date,
			end_date
		)
		SELECT $1,$2,$3,'active',1.0,$6,$4,$5
		WHERE NOT EXISTS (SELECT 1 FROM existing)
		RETURNING id
	)
	SELECT id FROM ins
	UNION ALL
	SELECT id FROM existing
	LIMIT 1
	`,
		pgUUID(tenantID),
		pgUUID(positionID),
		pgUUID(orgNodeID),
		pgValidDate(effectiveDate),
		pgValidDate(endDate),
		pgUUID(jobProfileID),
	).Scan(&sliceID); err != nil {
		return uuid.Nil, err
	}
	return sliceID, nil
}

func (r *OrgRepository) GetPositionOrgNodeAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var orgNodeID uuid.UUID
	if err := tx.QueryRow(ctx, `
	SELECT org_node_id
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date <= $3 AND end_date >= $3
	ORDER BY effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf)).Scan(&orgNodeID); err != nil {
		return uuid.Nil, err
	}
	return orgNodeID, nil
}

func (r *OrgRepository) LockPositionSliceAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (services.PositionSliceRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.PositionSliceRow{}, err
	}

	row := tx.QueryRow(ctx, `
	SELECT
		id,
		position_id,
		org_node_id,
		title,
		lifecycle_status,
		position_type,
		employment_type,
		capacity_fte,
		reports_to_position_id,
		job_level_code,
		job_profile_id,
		cost_center_code,
		profile,
		effective_date,
		end_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date <= $3 AND end_date >= $3
	ORDER BY effective_date DESC
	LIMIT 1
	FOR UPDATE
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf))

	var out services.PositionSliceRow
	var title pgtype.Text
	var reportsTo pgtype.UUID
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var jobLevelCode pgtype.Text
	var costCenterCode pgtype.Text
	var profile []byte
	if err := row.Scan(
		&out.ID,
		&out.PositionID,
		&out.OrgNodeID,
		&title,
		&out.LifecycleStatus,
		&positionType,
		&employmentType,
		&out.CapacityFTE,
		&reportsTo,
		&jobLevelCode,
		&out.JobProfileID,
		&costCenterCode,
		&profile,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.PositionSliceRow{}, err
	}
	out.Title = nullableText(title)
	out.ReportsToPositionID = nullableUUID(reportsTo)
	out.PositionType = nullableText(positionType)
	out.EmploymentType = nullableText(employmentType)
	out.JobLevelCode = nullableText(jobLevelCode)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) SumAllocatedFTEAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (float64, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return 0, err
	}
	var sum float64
	if err := tx.QueryRow(ctx, `
	SELECT COALESCE(SUM(allocated_fte), 0)
	FROM org_assignments
	WHERE tenant_id=$1
	  AND position_id=$2
	  AND assignment_type='primary'
	  AND employment_status='active'
	  AND effective_date <= $3
	  AND end_date >= $3
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf)).Scan(&sum); err != nil {
		return 0, err
	}
	return sum, nil
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
		allocated_fte,
		employment_status,
		effective_date,
		end_date
FROM org_assignments
WHERE tenant_id=$1 AND id=$2 AND effective_date <= $3 AND end_date >= $3
FOR UPDATE
	`, pgUUID(tenantID), pgUUID(assignmentID), pgValidDate(asOf))
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
		&out.EmploymentStatus,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.AssignmentRow{}, err
	}
	return out, nil
}

func (r *OrgRepository) LockAssignmentForTimelineAt(ctx context.Context, tenantID uuid.UUID, subjectType string, subjectID uuid.UUID, assignmentType string, asOf time.Time) (services.AssignmentRow, error) {
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
		employment_status,
		effective_date,
		end_date
	FROM org_assignments
	WHERE tenant_id=$1
		AND subject_type=$2
		AND subject_id=$3
		AND assignment_type=$4
		AND effective_date <= $5
		AND end_date >= $5
	ORDER BY effective_date DESC
	LIMIT 1
	FOR UPDATE
	`, pgUUID(tenantID), subjectType, pgUUID(subjectID), assignmentType, pgValidDate(asOf))

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
		&out.EmploymentStatus,
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
	_, err = tx.Exec(ctx, `UPDATE org_assignments SET end_date=$3 WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(assignmentID), pgValidDate(endDate))
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
`, pgUUID(tenantID), pgUUID(assignmentID), pgValidDate(after)).Scan(&next)
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
			allocated_fte,
			employment_status,
			effective_date,
			end_date
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id
		`, pgUUID(tenantID), pgUUID(assignment.PositionID), assignment.SubjectType, pgUUID(assignment.SubjectID), assignment.Pernr, assignment.AssignmentType, assignment.IsPrimary, assignment.AllocatedFTE, assignment.EmploymentStatus, pgValidDate(assignment.EffectiveDate), pgValidDate(assignment.EndDate)).Scan(&id); err != nil {
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
				ps.org_node_id,
				a.assignment_type,
				a.is_primary,
				a.allocated_fte,
				a.employment_status,
				a.effective_date,
				a.end_date,
				p.code AS position_code,
				ps.title AS position_title,
				n.code AS org_node_code,
				ns.name AS org_node_name,
				jfg.code AS job_family_group_code,
				jfgs.name AS job_family_group_name,
				jf.code AS job_family_code,
				jfs.name AS job_family_name,
				jp.code AS job_profile_code,
				jps.name AS job_profile_name,
				ps.job_level_code AS job_level_code,
				jls.name AS job_level_name,
				(
					SELECT e.event_type
					FROM org_personnel_events e
					WHERE e.tenant_id=a.tenant_id
				AND e.person_uuid=a.subject_id
				AND e.event_type IN ('hire', 'transfer')
				AND e.effective_date = a.effective_date
			ORDER BY e.created_at DESC
			LIMIT 1
		) AS start_event_type,
		(
			SELECT e.event_type
			FROM org_personnel_events e
			WHERE e.tenant_id=a.tenant_id
				AND e.person_uuid=a.subject_id
				AND e.event_type = 'termination'
				AND e.effective_date = a.end_date
			ORDER BY e.created_at DESC
			LIMIT 1
		) AS end_event_type
	FROM org_assignments a
	JOIN org_positions p
		ON p.tenant_id=a.tenant_id
		AND p.id=a.position_id
	JOIN org_position_slices ps
		ON ps.tenant_id=a.tenant_id
		AND ps.position_id=a.position_id
		AND ps.effective_date <= a.effective_date
		AND ps.end_date >= a.effective_date
		JOIN org_nodes n
			ON n.tenant_id=a.tenant_id
			AND n.id=ps.org_node_id
		LEFT JOIN org_node_slices ns
			ON ns.tenant_id=a.tenant_id
			AND ns.org_node_id=ps.org_node_id
			AND ns.effective_date <= a.effective_date
			AND ns.end_date >= a.effective_date
			LEFT JOIN org_job_profiles jp
				ON jp.tenant_id=a.tenant_id
				AND jp.id=ps.job_profile_id
			LEFT JOIN org_job_profile_slices jps
				ON jps.tenant_id=a.tenant_id
				AND jps.job_profile_id=ps.job_profile_id
				AND jps.effective_date <= a.effective_date
				AND jps.end_date >= a.effective_date
			LEFT JOIN org_job_profile_slice_job_families jpsjf
				ON jpsjf.tenant_id=a.tenant_id
				AND jpsjf.job_profile_slice_id=jps.id
				AND jpsjf.is_primary=TRUE
			LEFT JOIN org_job_families jf
				ON jf.tenant_id=a.tenant_id
				AND jf.id=jpsjf.job_family_id
			LEFT JOIN org_job_family_slices jfs
				ON jfs.tenant_id=a.tenant_id
				AND jfs.job_family_id=jf.id
				AND jfs.effective_date <= a.effective_date
				AND jfs.end_date >= a.effective_date
			LEFT JOIN org_job_family_groups jfg
				ON jfg.tenant_id=a.tenant_id
				AND jfg.id=jf.job_family_group_id
			LEFT JOIN org_job_family_group_slices jfgs
				ON jfgs.tenant_id=a.tenant_id
				AND jfgs.job_family_group_id=jfg.id
				AND jfgs.effective_date <= a.effective_date
				AND jfgs.end_date >= a.effective_date
			LEFT JOIN org_job_levels jl
				ON jl.tenant_id=a.tenant_id
				AND jl.code=ps.job_level_code
			LEFT JOIN org_job_level_slices jls
				ON jls.tenant_id=a.tenant_id
				AND jls.job_level_id=jl.id
				AND jls.effective_date <= a.effective_date
				AND jls.end_date >= a.effective_date
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
		var positionCode string
		var positionTitle pgtype.Text
		var orgNodeCode string
		var orgNodeName pgtype.Text
		var jobFamilyGroupCode pgtype.Text
		var jobFamilyGroupName pgtype.Text
		var jobFamilyCode pgtype.Text
		var jobFamilyName pgtype.Text
		var jobProfileCode pgtype.Text
		var jobProfileName pgtype.Text
		var jobLevelCode pgtype.Text
		var jobLevelName pgtype.Text
		var startEventType pgtype.Text
		var endEventType pgtype.Text
		if err := rows.Scan(
			&v.ID,
			&v.PositionID,
			&v.OrgNodeID,
			&v.AssignmentType,
			&v.IsPrimary,
			&v.AllocatedFTE,
			&v.EmploymentStatus,
			&v.EffectiveDate,
			&v.EndDate,
			&positionCode,
			&positionTitle,
			&orgNodeCode,
			&orgNodeName,
			&jobFamilyGroupCode,
			&jobFamilyGroupName,
			&jobFamilyCode,
			&jobFamilyName,
			&jobProfileCode,
			&jobProfileName,
			&jobLevelCode,
			&jobLevelName,
			&startEventType,
			&endEventType,
		); err != nil {
			return nil, err
		}
		if positionCode != "" {
			c := positionCode
			v.PositionCode = &c
		}
		v.PositionTitle = nullableText(positionTitle)
		if orgNodeCode != "" {
			c := orgNodeCode
			v.OrgNodeCode = &c
		}
		v.OrgNodeName = nullableText(orgNodeName)
		v.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
		v.JobFamilyGroupName = nullableText(jobFamilyGroupName)
		v.JobFamilyCode = nullableText(jobFamilyCode)
		v.JobFamilyName = nullableText(jobFamilyName)
		v.JobProfileCode = nullableText(jobProfileCode)
		v.JobProfileName = nullableText(jobProfileName)
		v.JobLevelCode = nullableText(jobLevelCode)
		v.JobLevelName = nullableText(jobLevelName)
		v.StartEventType = nullableText(startEventType)
		v.EndEventType = nullableText(endEventType)
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
				s.org_node_id,
				a.assignment_type,
				a.is_primary,
				a.allocated_fte,
				a.employment_status,
				a.effective_date,
				a.end_date,
				p.code AS position_code,
				s.title AS position_title,
				n.code AS org_node_code,
				ns.name AS org_node_name,
				jfg.code AS job_family_group_code,
				jfgs.name AS job_family_group_name,
				jf.code AS job_family_code,
				jfs.name AS job_family_name,
				jp.code AS job_profile_code,
				jps.name AS job_profile_name,
				s.job_level_code AS job_level_code,
				jls.name AS job_level_name,
				(
					SELECT e.event_type
					FROM org_personnel_events e
					WHERE e.tenant_id=a.tenant_id
				AND e.person_uuid=a.subject_id
				AND e.event_type IN ('hire', 'transfer')
				AND e.effective_date = a.effective_date
			ORDER BY e.created_at DESC
			LIMIT 1
		) AS start_event_type,
		(
			SELECT e.event_type
			FROM org_personnel_events e
			WHERE e.tenant_id=a.tenant_id
				AND e.person_uuid=a.subject_id
				AND e.event_type = 'termination'
				AND e.effective_date = a.end_date
			ORDER BY e.created_at DESC
			LIMIT 1
			) AS end_event_type
		FROM org_assignments a
		JOIN org_positions p
			ON p.tenant_id=a.tenant_id
			AND p.id=a.position_id
		JOIN org_position_slices s
			ON s.tenant_id=a.tenant_id
			AND s.position_id=a.position_id
			AND s.effective_date <= $3
			AND s.end_date >= $3
		JOIN org_nodes n
			ON n.tenant_id=a.tenant_id
			AND n.id=s.org_node_id
		LEFT JOIN org_node_slices ns
			ON ns.tenant_id=a.tenant_id
			AND ns.org_node_id=s.org_node_id
			AND ns.effective_date <= $3
			AND ns.end_date >= $3
			LEFT JOIN org_job_profiles jp
				ON jp.tenant_id=a.tenant_id
				AND jp.id=s.job_profile_id
			LEFT JOIN org_job_profile_slices jps
				ON jps.tenant_id=a.tenant_id
				AND jps.job_profile_id=s.job_profile_id
				AND jps.effective_date <= $3
				AND jps.end_date >= $3
			LEFT JOIN org_job_profile_slice_job_families jpsjf
				ON jpsjf.tenant_id=a.tenant_id
				AND jpsjf.job_profile_slice_id=jps.id
				AND jpsjf.is_primary=TRUE
			LEFT JOIN org_job_families jf
				ON jf.tenant_id=a.tenant_id
				AND jf.id=jpsjf.job_family_id
			LEFT JOIN org_job_family_slices jfs
				ON jfs.tenant_id=a.tenant_id
				AND jfs.job_family_id=jf.id
				AND jfs.effective_date <= $3
				AND jfs.end_date >= $3
			LEFT JOIN org_job_family_groups jfg
				ON jfg.tenant_id=a.tenant_id
				AND jfg.id=jf.job_family_group_id
			LEFT JOIN org_job_family_group_slices jfgs
				ON jfgs.tenant_id=a.tenant_id
				AND jfgs.job_family_group_id=jfg.id
				AND jfgs.effective_date <= $3
				AND jfgs.end_date >= $3
			LEFT JOIN org_job_levels jl
				ON jl.tenant_id=a.tenant_id
				AND jl.code=s.job_level_code
			LEFT JOIN org_job_level_slices jls
				ON jls.tenant_id=a.tenant_id
				AND jls.job_level_id=jl.id
				AND jls.effective_date <= $3
				AND jls.end_date >= $3
			WHERE a.tenant_id=$1 AND a.subject_id=$2 AND a.effective_date <= $3 AND a.end_date >= $3
			ORDER BY a.effective_date DESC
			`, pgUUID(tenantID), pgUUID(subjectID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.AssignmentViewRow, 0, 16)
	for rows.Next() {
		var v services.AssignmentViewRow
		var positionCode string
		var positionTitle pgtype.Text
		var orgNodeCode string
		var orgNodeName pgtype.Text
		var jobFamilyGroupCode pgtype.Text
		var jobFamilyGroupName pgtype.Text
		var jobFamilyCode pgtype.Text
		var jobFamilyName pgtype.Text
		var jobProfileCode pgtype.Text
		var jobProfileName pgtype.Text
		var jobLevelCode pgtype.Text
		var jobLevelName pgtype.Text
		var startEventType pgtype.Text
		var endEventType pgtype.Text
		if err := rows.Scan(
			&v.ID,
			&v.PositionID,
			&v.OrgNodeID,
			&v.AssignmentType,
			&v.IsPrimary,
			&v.AllocatedFTE,
			&v.EmploymentStatus,
			&v.EffectiveDate,
			&v.EndDate,
			&positionCode,
			&positionTitle,
			&orgNodeCode,
			&orgNodeName,
			&jobFamilyGroupCode,
			&jobFamilyGroupName,
			&jobFamilyCode,
			&jobFamilyName,
			&jobProfileCode,
			&jobProfileName,
			&jobLevelCode,
			&jobLevelName,
			&startEventType,
			&endEventType,
		); err != nil {
			return nil, err
		}
		if positionCode != "" {
			c := positionCode
			v.PositionCode = &c
		}
		v.PositionTitle = nullableText(positionTitle)
		if orgNodeCode != "" {
			c := orgNodeCode
			v.OrgNodeCode = &c
		}
		v.OrgNodeName = nullableText(orgNodeName)
		v.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
		v.JobFamilyGroupName = nullableText(jobFamilyGroupName)
		v.JobFamilyCode = nullableText(jobFamilyCode)
		v.JobFamilyName = nullableText(jobFamilyName)
		v.JobProfileCode = nullableText(jobProfileCode)
		v.JobProfileName = nullableText(jobProfileName)
		v.JobLevelCode = nullableText(jobLevelCode)
		v.JobLevelName = nullableText(jobLevelName)
		v.StartEventType = nullableText(startEventType)
		v.EndEventType = nullableText(endEventType)
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
