package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) InsertPosition(ctx context.Context, tenantID uuid.UUID, in services.PositionInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO org_positions (
			tenant_id,
			id,
		org_node_id,
		code,
			title,
			status,
			is_auto_created,
			effective_date,
			end_date,
			effective_on,
			end_on
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id
		`,
		pgUUID(tenantID),
		pgUUID(in.PositionID),
		pgUUID(in.OrgNodeID),
		strings.TrimSpace(in.Code),
		pgNullableText(in.Title),
		strings.TrimSpace(in.LegacyStatus),
		in.IsAutoCreated,
		in.EffectiveDate.UTC(),
		in.EndDate.UTC(),
		pgEffectiveOnFromEffectiveDate(in.EffectiveDate),
		pgEndOnFromEndDate(in.EndDate),
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) GetPositionIsAutoCreated(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var isAutoCreated bool
	if err := tx.QueryRow(ctx, `
	SELECT is_auto_created
	FROM org_positions
	WHERE tenant_id=$1 AND id=$2
	`, pgUUID(tenantID), pgUUID(positionID)).Scan(&isAutoCreated); err != nil {
		return false, err
	}
	return isAutoCreated, nil
}

func (r *OrgRepository) InsertPositionSlice(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, in services.PositionSliceInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	profile := "{}"
	if len(in.Profile) != 0 {
		profile = string(in.Profile)
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO org_position_slices (
			tenant_id,
			position_id,
		org_node_id,
		title,
		lifecycle_status,
		position_type,
		employment_type,
		capacity_fte,
		reports_to_position_id,
		job_family_group_code,
		job_family_code,
		job_role_code,
		job_level_code,
		job_profile_id,
			cost_center_code,
			profile,
			effective_date,
			end_date,
			effective_on,
			end_on
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,$17,$18,$19,$20)
		RETURNING id
		`,
		pgUUID(tenantID),
		pgUUID(positionID),
		pgUUID(in.OrgNodeID),
		pgNullableText(in.Title),
		strings.TrimSpace(in.LifecycleStatus),
		pgNullableText(in.PositionType),
		pgNullableText(in.EmploymentType),
		in.CapacityFTE,
		pgNullableUUID(in.ReportsToPositionID),
		pgNullableText(in.JobFamilyGroupCode),
		pgNullableText(in.JobFamilyCode),
		pgNullableText(in.JobRoleCode),
		pgNullableText(in.JobLevelCode),
		pgNullableUUID(in.JobProfileID),
		pgNullableText(in.CostCenterCode),
		profile,
		in.EffectiveDate.UTC(),
		in.EndDate.UTC(),
		pgEffectiveOnFromEffectiveDate(in.EffectiveDate),
		pgEndOnFromEndDate(in.EndDate),
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) GetPositionSliceAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (services.PositionSliceRow, error) {
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
		job_family_group_code,
		job_family_code,
		job_role_code,
		job_level_code,
		job_profile_id,
		cost_center_code,
		profile,
		effective_date,
		end_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date <= $3 AND end_date > $3
	ORDER BY effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(positionID), asOf.UTC())

	var out services.PositionSliceRow
	var title pgtype.Text
	var reportsTo pgtype.UUID
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var jobFamilyGroupCode pgtype.Text
	var jobFamilyCode pgtype.Text
	var jobRoleCode pgtype.Text
	var jobLevelCode pgtype.Text
	var jobProfileID pgtype.UUID
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
		&jobFamilyGroupCode,
		&jobFamilyCode,
		&jobRoleCode,
		&jobLevelCode,
		&jobProfileID,
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
	out.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
	out.JobFamilyCode = nullableText(jobFamilyCode)
	out.JobRoleCode = nullableText(jobRoleCode)
	out.JobLevelCode = nullableText(jobLevelCode)
	out.JobProfileID = nullableUUID(jobProfileID)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) LockPositionSliceStartingAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, effectiveDate time.Time) (services.PositionSliceRow, error) {
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
		job_family_group_code,
		job_family_code,
		job_role_code,
		job_level_code,
		job_profile_id,
		cost_center_code,
		profile,
		effective_date,
		end_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date=$3
	LIMIT 1
	FOR UPDATE
	`, pgUUID(tenantID), pgUUID(positionID), effectiveDate.UTC())

	var out services.PositionSliceRow
	var title pgtype.Text
	var reportsTo pgtype.UUID
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var jobFamilyGroupCode pgtype.Text
	var jobFamilyCode pgtype.Text
	var jobRoleCode pgtype.Text
	var jobLevelCode pgtype.Text
	var jobProfileID pgtype.UUID
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
		&jobFamilyGroupCode,
		&jobFamilyCode,
		&jobRoleCode,
		&jobLevelCode,
		&jobProfileID,
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
	out.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
	out.JobFamilyCode = nullableText(jobFamilyCode)
	out.JobRoleCode = nullableText(jobRoleCode)
	out.JobLevelCode = nullableText(jobLevelCode)
	out.JobProfileID = nullableUUID(jobProfileID)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) LockPositionSliceEndingAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, endDate time.Time) (services.PositionSliceRow, error) {
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
		job_family_group_code,
		job_family_code,
		job_role_code,
		job_level_code,
		job_profile_id,
		cost_center_code,
		profile,
		effective_date,
		end_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND end_date=$3
	LIMIT 1
	FOR UPDATE
	`, pgUUID(tenantID), pgUUID(positionID), endDate.UTC())

	var out services.PositionSliceRow
	var title pgtype.Text
	var reportsTo pgtype.UUID
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var jobFamilyGroupCode pgtype.Text
	var jobFamilyCode pgtype.Text
	var jobRoleCode pgtype.Text
	var jobLevelCode pgtype.Text
	var jobProfileID pgtype.UUID
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
		&jobFamilyGroupCode,
		&jobFamilyCode,
		&jobRoleCode,
		&jobLevelCode,
		&jobProfileID,
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
	out.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
	out.JobFamilyCode = nullableText(jobFamilyCode)
	out.JobRoleCode = nullableText(jobRoleCode)
	out.JobLevelCode = nullableText(jobLevelCode)
	out.JobProfileID = nullableUUID(jobProfileID)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) TruncatePositionSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	return r.UpdatePositionSliceEndDate(ctx, tenantID, sliceID, endDate)
}

func (r *OrgRepository) UpdatePositionSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, effectiveDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_position_slices SET effective_date=$3, effective_on=$4, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), effectiveDate.UTC(), pgEffectiveOnFromEffectiveDate(effectiveDate))
	return err
}

func (r *OrgRepository) UpdatePositionSliceEndDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_position_slices SET end_date=$3, end_on=$4, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), endDate.UTC(), pgEndOnFromEndDate(endDate))
	return err
}

func (r *OrgRepository) NextPositionSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, after time.Time) (time.Time, bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	var next time.Time
	err = tx.QueryRow(ctx, `
	SELECT effective_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2 AND effective_date > $3
	ORDER BY effective_date ASC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(positionID), after.UTC()).Scan(&next)
	if err != nil {
		if err == pgx.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return next, true, nil
}

func (r *OrgRepository) ListPositionSlicesTimeline(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID) ([]services.PositionSliceRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
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
		job_family_group_code,
		job_family_code,
		job_role_code,
		job_level_code,
		job_profile_id,
		cost_center_code,
		profile,
		effective_date,
		end_date
	FROM org_position_slices
	WHERE tenant_id=$1 AND position_id=$2
	ORDER BY effective_date ASC
	`, pgUUID(tenantID), pgUUID(positionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.PositionSliceRow, 0, 16)
	for rows.Next() {
		var row services.PositionSliceRow
		var title pgtype.Text
		var reportsTo pgtype.UUID
		var positionType pgtype.Text
		var employmentType pgtype.Text
		var jobFamilyGroupCode pgtype.Text
		var jobFamilyCode pgtype.Text
		var jobRoleCode pgtype.Text
		var jobLevelCode pgtype.Text
		var jobProfileID pgtype.UUID
		var costCenterCode pgtype.Text
		var profile []byte
		if err := rows.Scan(
			&row.ID,
			&row.PositionID,
			&row.OrgNodeID,
			&title,
			&row.LifecycleStatus,
			&positionType,
			&employmentType,
			&row.CapacityFTE,
			&reportsTo,
			&jobFamilyGroupCode,
			&jobFamilyCode,
			&jobRoleCode,
			&jobLevelCode,
			&jobProfileID,
			&costCenterCode,
			&profile,
			&row.EffectiveDate,
			&row.EndDate,
		); err != nil {
			return nil, err
		}
		row.Title = nullableText(title)
		row.ReportsToPositionID = nullableUUID(reportsTo)
		row.PositionType = nullableText(positionType)
		row.EmploymentType = nullableText(employmentType)
		row.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
		row.JobFamilyCode = nullableText(jobFamilyCode)
		row.JobRoleCode = nullableText(jobRoleCode)
		row.JobLevelCode = nullableText(jobLevelCode)
		row.JobProfileID = nullableUUID(jobProfileID)
		row.CostCenterCode = nullableText(costCenterCode)
		row.Profile = profile
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListPositionsAsOf(ctx context.Context, tenantID uuid.UUID, asOf time.Time, filter services.PositionListFilter) ([]services.PositionViewRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	asOf = asOf.UTC()

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	q := `
	SELECT
		p.id,
		p.code,
		s.org_node_id,
		s.title,
		s.lifecycle_status,
		p.is_auto_created,
		s.capacity_fte::float8 AS capacity_fte,
		COALESCE(SUM(a.allocated_fte), 0)::float8 AS occupied_fte,
		CASE
			WHEN COALESCE(SUM(a.allocated_fte), 0) = 0 THEN 'empty'
			WHEN COALESCE(SUM(a.allocated_fte), 0) < s.capacity_fte THEN 'partially_filled'
			ELSE 'filled'
		END AS staffing_state,
		s.effective_date,
		s.end_date
	FROM org_positions p
	JOIN org_position_slices s
		ON s.tenant_id = p.tenant_id
		AND s.position_id = p.id
		AND s.effective_date <= $2
		AND s.end_date > $2
	LEFT JOIN org_assignments a
		ON a.tenant_id = p.tenant_id
		AND a.position_id = p.id
		AND a.assignment_type = 'primary'
		AND a.effective_date <= $2
		AND a.end_date > $2
	WHERE p.tenant_id = $1
	`
	args := []any{pgUUID(tenantID), asOf}
	argPos := 3

	if len(filter.OrgNodeIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(filter.OrgNodeIDs))
		for _, id := range filter.OrgNodeIDs {
			if id == uuid.Nil {
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			q += fmt.Sprintf(" AND s.org_node_id = ANY($%d)", argPos)
			args = append(args, pgUUIDArray(ids))
			argPos++
		}
	} else if filter.OrgNodeID != nil && *filter.OrgNodeID != uuid.Nil {
		q += fmt.Sprintf(" AND s.org_node_id = $%d", argPos)
		args = append(args, pgUUID(*filter.OrgNodeID))
		argPos++
	}
	if filter.IsAutoCreated != nil {
		q += fmt.Sprintf(" AND p.is_auto_created = $%d", argPos)
		args = append(args, *filter.IsAutoCreated)
		argPos++
	}
	if filter.LifecycleStatus != nil {
		v := strings.TrimSpace(*filter.LifecycleStatus)
		if v != "" {
			q += fmt.Sprintf(" AND s.lifecycle_status = $%d", argPos)
			args = append(args, v)
			argPos++
		}
	}
	if filter.Q != nil {
		v := strings.TrimSpace(*filter.Q)
		if v != "" {
			q += fmt.Sprintf(" AND (p.code ILIKE $%d OR COALESCE(s.title,'') ILIKE $%d)", argPos, argPos)
			args = append(args, "%"+v+"%")
			argPos++
		}
	}

	q += `
	GROUP BY
		p.id,
		p.code,
		s.org_node_id,
		s.title,
		s.lifecycle_status,
		p.is_auto_created,
		s.capacity_fte,
		s.effective_date,
		s.end_date
	`
	if filter.StaffingState != nil {
		v := strings.TrimSpace(*filter.StaffingState)
		switch v {
		case "":
		case "empty":
			q += " HAVING COALESCE(SUM(a.allocated_fte), 0) = 0"
		case "partially_filled":
			q += " HAVING COALESCE(SUM(a.allocated_fte), 0) > 0 AND COALESCE(SUM(a.allocated_fte), 0) < s.capacity_fte"
		case "filled":
			q += " HAVING COALESCE(SUM(a.allocated_fte), 0) >= s.capacity_fte"
		default:
			return nil, fmt.Errorf("invalid staffing_state")
		}
	}

	q += ` ORDER BY p.code ASC, p.id ASC `
	q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.PositionViewRow, 0, minInt(limit, 64))
	for rows.Next() {
		var row services.PositionViewRow
		var title pgtype.Text
		if err := rows.Scan(
			&row.PositionID,
			&row.Code,
			&row.OrgNodeID,
			&title,
			&row.LifecycleStatus,
			&row.IsAutoCreated,
			&row.CapacityFTE,
			&row.OccupiedFTE,
			&row.StaffingState,
			&row.EffectiveDate,
			&row.EndDate,
		); err != nil {
			return nil, err
		}
		row.Title = nullableText(title)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) GetPositionAsOf(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (services.PositionViewRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.PositionViewRow{}, err
	}
	asOf = asOf.UTC()

	row := tx.QueryRow(ctx, `
	SELECT
		p.id,
		p.code,
		s.org_node_id,
		s.title,
		s.lifecycle_status,
		p.is_auto_created,
		s.capacity_fte::float8 AS capacity_fte,
		COALESCE(SUM(a.allocated_fte), 0)::float8 AS occupied_fte,
		CASE
			WHEN COALESCE(SUM(a.allocated_fte), 0) = 0 THEN 'empty'
			WHEN COALESCE(SUM(a.allocated_fte), 0) < s.capacity_fte THEN 'partially_filled'
			ELSE 'filled'
		END AS staffing_state,
		s.position_type,
		s.employment_type,
		s.reports_to_position_id,
		s.job_family_group_code,
		s.job_family_code,
		s.job_role_code,
		s.job_level_code,
		s.job_profile_id,
		s.cost_center_code,
		s.profile,
		s.effective_date,
		s.end_date
	FROM org_positions p
	JOIN org_position_slices s
		ON s.tenant_id = p.tenant_id
		AND s.position_id = p.id
		AND s.effective_date <= $3
		AND s.end_date > $3
	LEFT JOIN org_assignments a
		ON a.tenant_id = p.tenant_id
		AND a.position_id = p.id
		AND a.assignment_type = 'primary'
		AND a.effective_date <= $3
		AND a.end_date > $3
	WHERE p.tenant_id=$1 AND p.id=$2
	GROUP BY
		p.id,
		p.code,
		s.org_node_id,
		s.title,
		s.lifecycle_status,
		p.is_auto_created,
		s.capacity_fte,
		s.position_type,
		s.employment_type,
		s.reports_to_position_id,
		s.job_family_group_code,
		s.job_family_code,
		s.job_role_code,
		s.job_level_code,
		s.job_profile_id,
		s.cost_center_code,
		s.profile,
		s.effective_date,
		s.end_date
	`, pgUUID(tenantID), pgUUID(positionID), asOf)

	var out services.PositionViewRow
	var title pgtype.Text
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var reportsTo pgtype.UUID
	var jobFamilyGroupCode pgtype.Text
	var jobFamilyCode pgtype.Text
	var jobRoleCode pgtype.Text
	var jobLevelCode pgtype.Text
	var jobProfileID pgtype.UUID
	var costCenterCode pgtype.Text
	var profile []byte
	if err := row.Scan(
		&out.PositionID,
		&out.Code,
		&out.OrgNodeID,
		&title,
		&out.LifecycleStatus,
		&out.IsAutoCreated,
		&out.CapacityFTE,
		&out.OccupiedFTE,
		&out.StaffingState,
		&positionType,
		&employmentType,
		&reportsTo,
		&jobFamilyGroupCode,
		&jobFamilyCode,
		&jobRoleCode,
		&jobLevelCode,
		&jobProfileID,
		&costCenterCode,
		&profile,
		&out.EffectiveDate,
		&out.EndDate,
	); err != nil {
		return services.PositionViewRow{}, err
	}
	out.Title = nullableText(title)
	out.PositionType = nullableText(positionType)
	out.EmploymentType = nullableText(employmentType)
	out.ReportsToPositionID = nullableUUID(reportsTo)
	out.JobFamilyGroupCode = nullableText(jobFamilyGroupCode)
	out.JobFamilyCode = nullableText(jobFamilyCode)
	out.JobRoleCode = nullableText(jobRoleCode)
	out.JobLevelCode = nullableText(jobLevelCode)
	out.JobProfileID = nullableUUID(jobProfileID)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) DeletePositionSlicesFrom(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, from time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_position_slices WHERE tenant_id=$1 AND position_id=$2 AND effective_date >= $3`, pgUUID(tenantID), pgUUID(positionID), from.UTC())
	return err
}

func (r *OrgRepository) UpdatePositionSliceInPlace(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, patch services.PositionSliceInPlacePatch) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	setOrgNode := patch.OrgNodeID != nil
	setTitle := patch.Title != nil
	setLifecycle := patch.LifecycleStatus != nil
	setPositionType := patch.PositionType != nil
	setEmploymentType := patch.EmploymentType != nil
	setCapacity := patch.CapacityFTE != nil
	setReportsTo := patch.ReportsToPositionID != nil
	setJobFamilyGroupCode := patch.JobFamilyGroupCode != nil
	setJobFamilyCode := patch.JobFamilyCode != nil
	setJobRoleCode := patch.JobRoleCode != nil
	setJobLevelCode := patch.JobLevelCode != nil
	setJobProfileID := patch.JobProfileID != nil
	setCostCenterCode := patch.CostCenterCode != nil
	setProfile := patch.Profile != nil

	profile := "{}"
	if setProfile {
		raw := *patch.Profile
		if len(raw) != 0 {
			profile = string(raw)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE org_position_slices
		SET
			org_node_id = CASE WHEN $3 THEN $4 ELSE org_node_id END,
			title = CASE WHEN $5 THEN $6 ELSE title END,
			lifecycle_status = CASE WHEN $7 THEN $8 ELSE lifecycle_status END,
			position_type = CASE WHEN $9 THEN $10 ELSE position_type END,
			employment_type = CASE WHEN $11 THEN $12 ELSE employment_type END,
			capacity_fte = CASE WHEN $13 THEN $14 ELSE capacity_fte END,
			reports_to_position_id = CASE WHEN $15 THEN $16 ELSE reports_to_position_id END,
			job_family_group_code = CASE WHEN $17 THEN $18 ELSE job_family_group_code END,
			job_family_code = CASE WHEN $19 THEN $20 ELSE job_family_code END,
			job_role_code = CASE WHEN $21 THEN $22 ELSE job_role_code END,
			job_level_code = CASE WHEN $23 THEN $24 ELSE job_level_code END,
			job_profile_id = CASE WHEN $25 THEN $26 ELSE job_profile_id END,
			cost_center_code = CASE WHEN $27 THEN $28 ELSE cost_center_code END,
			profile = CASE WHEN $29 THEN $30::jsonb ELSE profile END,
			updated_at = now()
		WHERE tenant_id=$1 AND id=$2
		`,
		pgUUID(tenantID),
		pgUUID(sliceID),
		setOrgNode,
		pgNullableUUID(patch.OrgNodeID),
		setTitle,
		pgNullableText(patch.Title),
		setLifecycle,
		derefString(patch.LifecycleStatus),
		setPositionType,
		pgNullableText(patch.PositionType),
		setEmploymentType,
		pgNullableText(patch.EmploymentType),
		setCapacity,
		derefFloat64(patch.CapacityFTE),
		setReportsTo,
		pgNullableUUID(patch.ReportsToPositionID),
		setJobFamilyGroupCode,
		pgNullableText(patch.JobFamilyGroupCode),
		setJobFamilyCode,
		pgNullableText(patch.JobFamilyCode),
		setJobRoleCode,
		pgNullableText(patch.JobRoleCode),
		setJobLevelCode,
		pgNullableText(patch.JobLevelCode),
		setJobProfileID,
		pgNullableUUID(patch.JobProfileID),
		setCostCenterCode,
		pgNullableText(patch.CostCenterCode),
		setProfile,
		profile,
	)
	return err
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func (r *OrgRepository) HasPositionSubordinatesAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
	SELECT EXISTS(
		SELECT 1
		FROM org_position_slices
		WHERE tenant_id=$1
		  AND reports_to_position_id=$2
		  AND lifecycle_status <> 'rescinded'
		  AND effective_date <= $3
		  AND end_date > $3
	)
	`, pgUUID(tenantID), pgUUID(positionID), asOf.UTC()).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
