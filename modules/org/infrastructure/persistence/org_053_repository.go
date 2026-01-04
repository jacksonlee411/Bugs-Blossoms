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
			end_date
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id
		`,
		pgUUID(tenantID),
		pgUUID(in.PositionID),
		pgUUID(in.OrgNodeID),
		strings.TrimSpace(in.Code),
		pgNullableText(in.Title),
		strings.TrimSpace(in.LegacyStatus),
		in.IsAutoCreated,
		pgValidDate(in.EffectiveDate),
		pgValidDate(in.EndDate),
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
			job_level_code,
			job_profile_id,
				cost_center_code,
				profile,
				effective_date,
				end_date
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15)
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
		pgNullableText(in.JobLevelCode),
		pgUUID(in.JobProfileID),
		pgNullableText(in.CostCenterCode),
		profile,
		pgValidDate(in.EffectiveDate),
		pgValidDate(in.EndDate),
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) CopyJobProfileJobFamiliesToPositionSlice(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, positionSliceID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
	INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, is_primary)
	SELECT $1, $3, sf.job_family_id, sf.is_primary
	FROM org_position_slices ps
	JOIN org_job_profile_slices jps
		ON jps.tenant_id=ps.tenant_id
		AND jps.job_profile_id=ps.job_profile_id
		AND jps.effective_date <= ps.effective_date
		AND jps.end_date >= ps.effective_date
	JOIN org_job_profile_slice_job_families sf
		ON sf.tenant_id=jps.tenant_id
		AND sf.job_profile_slice_id=jps.id
	WHERE ps.tenant_id=$1
		AND ps.id=$3
		AND ps.job_profile_id=$2
	ON CONFLICT (tenant_id, position_slice_id, job_family_id) DO NOTHING
	`, pgUUID(tenantID), pgUUID(jobProfileID), pgUUID(positionSliceID))
	return err
}

func (r *OrgRepository) CopyPositionSliceJobFamilies(ctx context.Context, tenantID uuid.UUID, fromSliceID uuid.UUID, toSliceID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
	INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, is_primary)
	SELECT tenant_id, $3, job_family_id, is_primary
	FROM org_position_slice_job_families
	WHERE tenant_id=$1 AND position_slice_id=$2
	ON CONFLICT (tenant_id, position_slice_id, job_family_id) DO NOTHING
	`, pgUUID(tenantID), pgUUID(fromSliceID), pgUUID(toSliceID))
	return err
}

func (r *OrgRepository) ResetPositionSliceJobFamiliesFromProfile(ctx context.Context, tenantID uuid.UUID, positionSliceID uuid.UUID, jobProfileID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
	DELETE FROM org_position_slice_job_families
	WHERE tenant_id=$1 AND position_slice_id=$2
	`, pgUUID(tenantID), pgUUID(positionSliceID))
	if err != nil {
		return err
	}
	return r.CopyJobProfileJobFamiliesToPositionSlice(ctx, tenantID, jobProfileID, positionSliceID)
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
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(effectiveDate))

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
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(endDate))

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

func (r *OrgRepository) TruncatePositionSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	return r.UpdatePositionSliceEndDate(ctx, tenantID, sliceID, endDate)
}

func (r *OrgRepository) UpdatePositionSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, effectiveDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_position_slices SET effective_date=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), pgValidDate(effectiveDate))
	return err
}

func (r *OrgRepository) UpdatePositionSliceEndDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE org_position_slices SET end_date=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID), pgValidDate(endDate))
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
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(after)).Scan(&next)
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
		var jobLevelCode pgtype.Text
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
			&jobLevelCode,
			&row.JobProfileID,
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
		row.JobLevelCode = nullableText(jobLevelCode)
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
		AND s.end_date >= $2
	LEFT JOIN org_assignments a
		ON a.tenant_id = p.tenant_id
		AND a.position_id = p.id
		AND a.assignment_type = 'primary'
		AND a.effective_date <= $2
		AND a.end_date >= $2
	WHERE p.tenant_id = $1
	`
	args := []any{pgUUID(tenantID), pgValidDate(asOf)}
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
			jc.job_family_group_code,
			jc.job_family_code,
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
			AND s.end_date >= $3
		LEFT JOIN LATERAL (
			SELECT
				jfg.code AS job_family_group_code,
				jf.code AS job_family_code
			FROM org_position_slice_job_families psjf
			JOIN org_job_families jf
				ON jf.tenant_id = psjf.tenant_id
				AND jf.id = psjf.job_family_id
			JOIN org_job_family_groups jfg
				ON jfg.tenant_id = jf.tenant_id
				AND jfg.id = jf.job_family_group_id
			WHERE psjf.tenant_id = s.tenant_id
			  AND psjf.position_slice_id = s.id
			  AND psjf.is_primary = TRUE
			LIMIT 1
		) jc ON TRUE
		LEFT JOIN org_assignments a
			ON a.tenant_id = p.tenant_id
			AND a.position_id = p.id
			AND a.assignment_type = 'primary'
		AND a.effective_date <= $3
		AND a.end_date >= $3
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
			jc.job_family_group_code,
			jc.job_family_code,
			s.job_level_code,
			s.job_profile_id,
			s.cost_center_code,
			s.profile,
			s.effective_date,
			s.end_date
		`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf))

	var out services.PositionViewRow
	var title pgtype.Text
	var positionType pgtype.Text
	var employmentType pgtype.Text
	var reportsTo pgtype.UUID
	var jobFamilyGroupCode pgtype.Text
	var jobFamilyCode pgtype.Text
	var jobLevelCode pgtype.Text
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
		&jobLevelCode,
		&out.JobProfileID,
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
	out.JobLevelCode = nullableText(jobLevelCode)
	out.CostCenterCode = nullableText(costCenterCode)
	out.Profile = profile
	return out, nil
}

func (r *OrgRepository) DeletePositionSlicesFrom(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, from time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_position_slices WHERE tenant_id=$1 AND position_id=$2 AND effective_date >= $3`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(from))
	return err
}

func (r *OrgRepository) DeletePositionSliceByID(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM org_position_slices WHERE tenant_id=$1 AND id=$2`, pgUUID(tenantID), pgUUID(sliceID))
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
			job_level_code = CASE WHEN $17 THEN $18 ELSE job_level_code END,
			job_profile_id = CASE WHEN $19 THEN $20 ELSE job_profile_id END,
			cost_center_code = CASE WHEN $21 THEN $22 ELSE cost_center_code END,
			profile = CASE WHEN $23 THEN $24::jsonb ELSE profile END,
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
		  AND end_date >= $3
	)
	`, pgUUID(tenantID), pgUUID(positionID), pgValidDate(asOf)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
