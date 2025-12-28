package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) GetActiveSnapshotBuildID(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	asOfDate := asOf.UTC().Format("2006-01-02")
	return activeSnapshotBuildID(ctx, tx, tenantID, hierarchyType, asOfDate)
}

func (r *OrgRepository) GetStaffingSummaryReport(
	ctx context.Context,
	tenantID uuid.UUID,
	asOf time.Time,
	orgNodeIDs []uuid.UUID,
	lifecycleStatuses []string,
	includeSystem bool,
	groupBy services.StaffingGroupBy,
) (services.StaffingSummaryDBResult, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.StaffingSummaryDBResult{}, err
	}
	if len(orgNodeIDs) == 0 {
		return services.StaffingSummaryDBResult{}, nil
	}

	const baseCTE = `
WITH pos AS (
	SELECT
		p.id AS position_id,
		COALESCE(NULLIF(btrim(s.position_type), ''), 'unknown') AS position_type,
		s.capacity_fte::float8 AS capacity_fte,
		COALESCE(SUM(a.allocated_fte), 0)::float8 AS occupied_fte
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
		AND s.org_node_id = ANY($3::uuid[])
		AND s.lifecycle_status = ANY($4::text[])
		AND ($5 OR p.is_auto_created = false)
	GROUP BY p.id, position_type, s.capacity_fte
)
`

	row := tx.QueryRow(ctx, baseCTE+`
SELECT
	COUNT(*)::int AS positions_total,
	COALESCE(SUM(capacity_fte), 0)::float8 AS capacity_fte,
	COALESCE(SUM(occupied_fte), 0)::float8 AS occupied_fte,
	COALESCE(SUM(GREATEST(capacity_fte - occupied_fte, 0)), 0)::float8 AS available_fte
FROM pos
`, pgUUID(tenantID), pgValidDate(asOf), orgNodeIDs, lifecycleStatuses, includeSystem)

	var out services.StaffingSummaryDBResult
	if err := row.Scan(
		&out.Totals.PositionsTotal,
		&out.Totals.CapacityFTE,
		&out.Totals.OccupiedFTE,
		&out.Totals.AvailableFTE,
	); err != nil {
		return services.StaffingSummaryDBResult{}, err
	}

	if groupBy != services.StaffingGroupByPositionType {
		return out, nil
	}

	rows, err := tx.Query(ctx, baseCTE+`
SELECT
	position_type AS key,
	COUNT(*)::int AS positions_total,
	COALESCE(SUM(capacity_fte), 0)::float8 AS capacity_fte,
	COALESCE(SUM(occupied_fte), 0)::float8 AS occupied_fte,
	COALESCE(SUM(GREATEST(capacity_fte - occupied_fte, 0)), 0)::float8 AS available_fte
FROM pos
GROUP BY position_type
ORDER BY position_type ASC
`, pgUUID(tenantID), pgValidDate(asOf), orgNodeIDs, lifecycleStatuses, includeSystem)
	if err != nil {
		return services.StaffingSummaryDBResult{}, err
	}
	defer rows.Close()

	out.Breakdown = make([]services.StaffingAggregateRow, 0, minInt(len(orgNodeIDs), 64))
	for rows.Next() {
		var row services.StaffingAggregateRow
		if err := rows.Scan(
			&row.Key,
			&row.PositionsTotal,
			&row.CapacityFTE,
			&row.OccupiedFTE,
			&row.AvailableFTE,
		); err != nil {
			return services.StaffingSummaryDBResult{}, err
		}
		out.Breakdown = append(out.Breakdown, row)
	}
	if rows.Err() != nil {
		return services.StaffingSummaryDBResult{}, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListStaffingVacanciesReport(
	ctx context.Context,
	tenantID uuid.UUID,
	asOf time.Time,
	orgNodeIDs []uuid.UUID,
	lifecycleStatuses []string,
	includeSystem bool,
	limit int,
	cursor *uuid.UUID,
) (services.StaffingVacanciesDBResult, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.StaffingVacanciesDBResult{}, err
	}
	if len(orgNodeIDs) == 0 || limit <= 0 {
		return services.StaffingVacanciesDBResult{}, nil
	}

	limitPlusOne := limit + 1

	rows, err := tx.Query(ctx, `
WITH vacancies AS (
	SELECT
		p.id AS position_id,
		p.code AS position_code,
		s.org_node_id,
		COALESCE(NULLIF(btrim(s.position_type), ''), 'unknown') AS position_type,
		s.capacity_fte::float8 AS capacity_fte,
		COALESCE(SUM(a.allocated_fte), 0)::float8 AS occupied_fte
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
		AND s.org_node_id = ANY($3::uuid[])
		AND s.lifecycle_status = ANY($4::text[])
		AND ($5 OR p.is_auto_created = false)
		AND ($6::uuid IS NULL OR p.id > $6::uuid)
	GROUP BY p.id, p.code, s.org_node_id, position_type, s.capacity_fte
	HAVING COALESCE(SUM(a.allocated_fte), 0) = 0
	ORDER BY p.id ASC
	LIMIT $7
)
	SELECT
		v.position_id,
		v.position_code,
		v.org_node_id,
		v.position_type,
		v.capacity_fte,
		v.occupied_fte,
		COALESCE(prev.last_end + 1, inc.inception) AS vacancy_since,
		GREATEST(
			0,
			($2::date - COALESCE(prev.last_end + 1, inc.inception))
		)::int AS vacancy_age_days
	FROM vacancies v
LEFT JOIN LATERAL (
	SELECT MAX(end_date) AS last_end
	FROM org_assignments a2
	WHERE a2.tenant_id = $1
		AND a2.position_id = v.position_id
		AND a2.assignment_type = 'primary'
		AND a2.end_date <= $2
) prev ON true
LEFT JOIN LATERAL (
	SELECT MIN(effective_date) AS inception
	FROM org_position_slices s2
	WHERE s2.tenant_id = $1
		AND s2.position_id = v.position_id
) inc ON true
ORDER BY v.position_id ASC
`, pgUUID(tenantID), pgValidDate(asOf), orgNodeIDs, lifecycleStatuses, includeSystem, pgNullableUUID(cursor), limitPlusOne)
	if err != nil {
		return services.StaffingVacanciesDBResult{}, err
	}
	defer rows.Close()

	out := make([]services.StaffingVacancyRow, 0, minInt(limitPlusOne, 64))
	for rows.Next() {
		var row services.StaffingVacancyRow
		if err := rows.Scan(
			&row.PositionID,
			&row.PositionCode,
			&row.OrgNodeID,
			&row.PositionType,
			&row.CapacityFTE,
			&row.OccupiedFTE,
			&row.VacancySince,
			&row.VacancyAgeDays,
		); err != nil {
			return services.StaffingVacanciesDBResult{}, err
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return services.StaffingVacanciesDBResult{}, rows.Err()
	}

	var nextCursorID *uuid.UUID
	if len(out) > limit {
		id := out[limit-1].PositionID
		nextCursorID = &id
		out = out[:limit]
	}

	return services.StaffingVacanciesDBResult{
		Items:      out,
		NextCursor: nextCursorID,
	}, nil
}

func (r *OrgRepository) GetStaffingTimeToFillReport(
	ctx context.Context,
	tenantID uuid.UUID,
	from time.Time,
	to time.Time,
	orgNodeIDs []uuid.UUID,
	lifecycleStatuses []string,
	includeSystem bool,
	groupBy services.StaffingGroupBy,
) (services.StaffingTimeToFillDBResult, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.StaffingTimeToFillDBResult{}, err
	}
	if len(orgNodeIDs) == 0 {
		return services.StaffingTimeToFillDBResult{}, nil
	}

	baseCTE := `
WITH inception AS (
	SELECT tenant_id, position_id, MIN(effective_date) AS inception_date
	FROM org_position_slices
	WHERE tenant_id = $1
	GROUP BY tenant_id, position_id
),
	events AS (
			SELECT
				COALESCE(NULLIF(btrim(s.position_type), ''), 'unknown') AS position_type,
				GREATEST(
					0,
					(a.effective_date - COALESCE(
						(SELECT (MAX(end_date) + 1)
							FROM org_assignments prev
							WHERE prev.tenant_id = a.tenant_id
								AND prev.position_id = a.position_id
								AND prev.assignment_type = 'primary'
								AND prev.end_date <= a.effective_date),
						inc.inception_date
					))
				)::int AS ttf_days
		FROM org_assignments a
	JOIN org_positions p
		ON p.tenant_id = a.tenant_id
		AND p.id = a.position_id
		JOIN org_position_slices s
			ON s.tenant_id = p.tenant_id
			AND s.position_id = p.id
			AND s.effective_date <= a.effective_date
			AND s.end_date >= a.effective_date
	JOIN inception inc
		ON inc.tenant_id = p.tenant_id
		AND inc.position_id = p.id
	WHERE a.tenant_id = $1
		AND a.assignment_type = 'primary'
		AND a.effective_date >= $2
		AND a.effective_date < $3
		AND s.capacity_fte = 1.0
		AND s.org_node_id = ANY($4::uuid[])
		AND s.lifecycle_status = ANY($5::text[])
		AND ($6 OR p.is_auto_created = false)
		AND NOT EXISTS (
			SELECT 1
			FROM org_assignments prev2
			WHERE prev2.tenant_id = a.tenant_id
				AND prev2.position_id = a.position_id
					AND prev2.assignment_type = 'primary'
					AND prev2.effective_date < a.effective_date
					AND prev2.end_date >= a.effective_date
			)
	)
	`

	var q string
	switch groupBy {
	case services.StaffingGroupByPositionType:
		q = baseCTE + `
SELECT
	'' AS key,
	COUNT(*)::int AS filled_count,
	COALESCE(AVG(ttf_days), 0)::float8 AS avg_days,
	COALESCE(percentile_disc(0.5) WITHIN GROUP (ORDER BY ttf_days), 0)::int AS p50_days,
	COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY ttf_days), 0)::int AS p95_days
FROM events
UNION ALL
SELECT
	position_type AS key,
	COUNT(*)::int AS filled_count,
	COALESCE(AVG(ttf_days), 0)::float8 AS avg_days,
	NULL::int AS p50_days,
	NULL::int AS p95_days
FROM events
GROUP BY position_type
ORDER BY key ASC
`
	case services.StaffingGroupByNone, services.StaffingGroupByJobLevel:
		q = baseCTE + `
SELECT
	'' AS key,
	COUNT(*)::int AS filled_count,
	COALESCE(AVG(ttf_days), 0)::float8 AS avg_days,
	COALESCE(percentile_disc(0.5) WITHIN GROUP (ORDER BY ttf_days), 0)::int AS p50_days,
	COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY ttf_days), 0)::int AS p95_days
FROM events
`
	}

	rows, err := tx.Query(ctx, q, pgUUID(tenantID), pgValidDate(from), pgValidDate(to), orgNodeIDs, lifecycleStatuses, includeSystem)
	if err != nil {
		return services.StaffingTimeToFillDBResult{}, err
	}
	defer rows.Close()

	var out services.StaffingTimeToFillDBResult
	for rows.Next() {
		var key string
		var p50 *int
		var p95 *int
		var filled int
		var avg float64
		if err := rows.Scan(&key, &filled, &avg, &p50, &p95); err != nil {
			return services.StaffingTimeToFillDBResult{}, err
		}
		if key == "" {
			out.Summary = services.StaffingTimeToFillSummary{
				FilledCount: filled,
				AvgDays:     avg,
				P50Days:     0,
				P95Days:     0,
			}
			if p50 != nil {
				out.Summary.P50Days = *p50
			}
			if p95 != nil {
				out.Summary.P95Days = *p95
			}
			continue
		}
		out.Breakdown = append(out.Breakdown, services.StaffingTimeToFillBreakdownRow{
			Key:         key,
			FilledCount: filled,
			AvgDays:     avg,
		})
	}
	if rows.Err() != nil {
		return services.StaffingTimeToFillDBResult{}, rows.Err()
	}
	return out, nil
}
