package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListJobFamilyGroups(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]services.JobFamilyGroupRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT g.id, g.code, s.name, s.is_active
	FROM org_job_family_groups g
	JOIN org_job_family_group_slices s
		ON s.tenant_id=g.tenant_id
		AND s.job_family_group_id=g.id
		AND s.effective_date <= $2
		AND s.end_date >= $2
	WHERE g.tenant_id=$1
	ORDER BY g.code ASC, g.id ASC
	`, pgUUID(tenantID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobFamilyGroupRow, 0, 64)
	for rows.Next() {
		var row services.JobFamilyGroupRow
		if err := rows.Scan(&row.ID, &row.Code, &row.Name, &row.IsActive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) CreateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, in services.JobFamilyGroupCreate) (services.JobFamilyGroupRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobFamilyGroupRow{}, err
	}
	var row services.JobFamilyGroupRow
	err = tx.QueryRow(ctx, `
	INSERT INTO org_job_family_groups (tenant_id, code, name, is_active)
	VALUES ($1,$2,$3,$4)
	RETURNING id, code, name, is_active
	`,
		pgUUID(tenantID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		in.IsActive,
	).Scan(&row.ID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) UpdateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in services.JobFamilyGroupUpdate) (services.JobFamilyGroupRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobFamilyGroupRow{}, err
	}
	setName := in.Name != nil
	setActive := in.IsActive != nil
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	active := false
	if in.IsActive != nil {
		active = *in.IsActive
	}

	var row services.JobFamilyGroupRow
	err = tx.QueryRow(ctx, `
	UPDATE org_job_family_groups
	SET
		name = CASE WHEN $3 THEN $4 ELSE name END,
		is_active = CASE WHEN $5 THEN $6 ELSE is_active END,
		updated_at = now()
	WHERE tenant_id=$1 AND id=$2
	RETURNING id, code, name, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setActive,
		active,
	).Scan(&row.ID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) ListJobFamilies(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupID uuid.UUID, asOf time.Time) ([]services.JobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT f.id, f.job_family_group_id, f.code, s.name, s.is_active
	FROM org_job_families f
	JOIN org_job_family_slices s
		ON s.tenant_id=f.tenant_id
		AND s.job_family_id=f.id
		AND s.effective_date <= $3
		AND s.end_date >= $3
	WHERE f.tenant_id=$1 AND f.job_family_group_id=$2
	ORDER BY f.code ASC, f.id ASC
	`, pgUUID(tenantID), pgUUID(jobFamilyGroupID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobFamilyRow, 0, 64)
	for rows.Next() {
		var row services.JobFamilyRow
		if err := rows.Scan(&row.ID, &row.JobFamilyGroupID, &row.Code, &row.Name, &row.IsActive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListJobFamiliesByGroupIDsAsOf(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupIDs []uuid.UUID, asOf time.Time) ([]services.JobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if len(jobFamilyGroupIDs) == 0 {
		return []services.JobFamilyRow{}, nil
	}
	rows, err := tx.Query(ctx, `
	SELECT f.id, f.job_family_group_id, f.code, s.name, s.is_active
	FROM org_job_families f
	JOIN org_job_family_slices s
		ON s.tenant_id=f.tenant_id
		AND s.job_family_id=f.id
		AND s.effective_date <= $3
		AND s.end_date >= $3
	WHERE f.tenant_id=$1 AND f.job_family_group_id = ANY($2::uuid[])
	ORDER BY f.job_family_group_id ASC, f.code ASC, f.id ASC
	`, pgUUID(tenantID), jobFamilyGroupIDs, pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobFamilyRow, 0, 64)
	for rows.Next() {
		var row services.JobFamilyRow
		if err := rows.Scan(&row.ID, &row.JobFamilyGroupID, &row.Code, &row.Name, &row.IsActive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) CreateJobFamily(ctx context.Context, tenantID uuid.UUID, in services.JobFamilyCreate) (services.JobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobFamilyRow{}, err
	}
	var row services.JobFamilyRow
	err = tx.QueryRow(ctx, `
	INSERT INTO org_job_families (tenant_id, job_family_group_id, code, name, is_active)
	VALUES ($1,$2,$3,$4,$5)
	RETURNING id, job_family_group_id, code, name, is_active
	`,
		pgUUID(tenantID),
		pgUUID(in.JobFamilyGroupID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		in.IsActive,
	).Scan(&row.ID, &row.JobFamilyGroupID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) UpdateJobFamily(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in services.JobFamilyUpdate) (services.JobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobFamilyRow{}, err
	}
	setName := in.Name != nil
	setActive := in.IsActive != nil
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	active := false
	if in.IsActive != nil {
		active = *in.IsActive
	}

	var row services.JobFamilyRow
	err = tx.QueryRow(ctx, `
	UPDATE org_job_families
	SET
		name = CASE WHEN $3 THEN $4 ELSE name END,
		is_active = CASE WHEN $5 THEN $6 ELSE is_active END,
		updated_at = now()
	WHERE tenant_id=$1 AND id=$2
	RETURNING id, job_family_group_id, code, name, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setActive,
		active,
	).Scan(&row.ID, &row.JobFamilyGroupID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) ListJobLevels(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT l.id, l.code, s.name, s.display_order, s.is_active
	FROM org_job_levels l
	JOIN org_job_level_slices s
		ON s.tenant_id=l.tenant_id
		AND s.job_level_id=l.id
		AND s.effective_date <= $2
		AND s.end_date >= $2
	WHERE l.tenant_id=$1
	ORDER BY s.display_order ASC, l.code ASC, l.id ASC
	`, pgUUID(tenantID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobLevelRow, 0, 64)
	for rows.Next() {
		var row services.JobLevelRow
		if err := rows.Scan(&row.ID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) CreateJobLevel(ctx context.Context, tenantID uuid.UUID, in services.JobLevelCreate) (services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobLevelRow{}, err
	}
	var row services.JobLevelRow
	err = tx.QueryRow(ctx, `
	INSERT INTO org_job_levels (tenant_id, code, name, display_order, is_active)
	VALUES ($1,$2,$3,$4,$5)
	RETURNING id, code, name, display_order, is_active
	`,
		pgUUID(tenantID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		in.DisplayOrder,
		in.IsActive,
	).Scan(&row.ID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive)
	return row, err
}

func (r *OrgRepository) UpdateJobLevel(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in services.JobLevelUpdate) (services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobLevelRow{}, err
	}
	setName := in.Name != nil
	setOrder := in.DisplayOrder != nil
	setActive := in.IsActive != nil
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	order := 0
	if in.DisplayOrder != nil {
		order = *in.DisplayOrder
	}
	active := false
	if in.IsActive != nil {
		active = *in.IsActive
	}

	var row services.JobLevelRow
	err = tx.QueryRow(ctx, `
	UPDATE org_job_levels
	SET
		name = CASE WHEN $3 THEN $4 ELSE name END,
		display_order = CASE WHEN $5 THEN $6 ELSE display_order END,
		is_active = CASE WHEN $7 THEN $8 ELSE is_active END,
		updated_at = now()
	WHERE tenant_id=$1 AND id=$2
	RETURNING id, code, name, display_order, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setOrder,
		order,
		setActive,
		active,
	).Scan(&row.ID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive)
	return row, err
}

func (r *OrgRepository) ListJobProfiles(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]services.JobProfileRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT p.id, p.code, s.name, s.description, s.is_active
	FROM org_job_profiles p
	JOIN org_job_profile_slices s
		ON s.tenant_id=p.tenant_id
		AND s.job_profile_id=p.id
		AND s.effective_date <= $2
		AND s.end_date >= $2
	WHERE p.tenant_id=$1
	ORDER BY p.code ASC, p.id ASC
	`, pgUUID(tenantID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobProfileRow, 0, 64)
	for rows.Next() {
		var row services.JobProfileRow
		var desc pgtype.Text
		if err := rows.Scan(&row.ID, &row.Code, &row.Name, &desc, &row.IsActive); err != nil {
			return nil, err
		}
		row.Description = nullableText(desc)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) CreateJobProfile(ctx context.Context, tenantID uuid.UUID, in services.JobProfileCreate) (services.JobProfileRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobProfileRow{}, err
	}
	var row services.JobProfileRow
	var desc pgtype.Text
	if in.Description != nil {
		desc = pgtype.Text{String: *in.Description, Valid: true}
	}
	err = tx.QueryRow(ctx, `
	INSERT INTO org_job_profiles (tenant_id, code, name, description, is_active)
	VALUES ($1,$2,$3,$4,$5)
	RETURNING id, code, name, description, is_active
	`,
		pgUUID(tenantID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		desc,
		in.IsActive,
	).Scan(&row.ID, &row.Code, &row.Name, &desc, &row.IsActive)
	row.Description = nullableText(desc)
	return row, err
}

func (r *OrgRepository) UpdateJobProfile(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in services.JobProfileUpdate) (services.JobProfileRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobProfileRow{}, err
	}
	setName := in.Name != nil
	setDesc := in.Description != nil
	setActive := in.IsActive != nil
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	desc := pgtype.Text{}
	if in.Description != nil && *in.Description != nil {
		desc = pgtype.Text{String: strings.TrimSpace(**in.Description), Valid: true}
	}
	active := false
	if in.IsActive != nil {
		active = *in.IsActive
	}

	var row services.JobProfileRow
	var outDesc pgtype.Text
	err = tx.QueryRow(ctx, `
	UPDATE org_job_profiles
	SET
		name = CASE WHEN $3 THEN $4 ELSE name END,
		description = CASE WHEN $5 THEN $6 ELSE description END,
		is_active = CASE WHEN $7 THEN $8 ELSE is_active END,
		updated_at = now()
	WHERE tenant_id=$1 AND id=$2
	RETURNING id, code, name, description, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setDesc,
		desc,
		setActive,
		active,
	).Scan(&row.ID, &row.Code, &row.Name, &outDesc, &row.IsActive)
	row.Description = nullableText(outDesc)
	return row, err
}

func (r *OrgRepository) GetJobProfileRef(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, asOf time.Time) (services.JobProfileRef, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobProfileRef{}, err
	}
	var ref services.JobProfileRef
	if err := tx.QueryRow(ctx, `
	SELECT p.id, s.is_active
	FROM org_job_profiles p
	JOIN org_job_profile_slices s
		ON s.tenant_id=p.tenant_id
		AND s.job_profile_id=p.id
		AND s.effective_date <= $3
		AND s.end_date >= $3
	WHERE p.tenant_id=$1 AND p.id=$2
	ORDER BY s.effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), pgUUID(jobProfileID), pgValidDate(asOf)).Scan(&ref.ID, &ref.IsActive); err != nil {
		return services.JobProfileRef{}, err
	}
	return ref, nil
}

func (r *OrgRepository) GetJobLevelByCode(ctx context.Context, tenantID uuid.UUID, code string, asOf time.Time) (services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobLevelRow{}, err
	}
	var row services.JobLevelRow
	if err := tx.QueryRow(ctx, `
	SELECT l.id, l.code, s.name, s.display_order, s.is_active
	FROM org_job_levels l
	JOIN org_job_level_slices s
		ON s.tenant_id=l.tenant_id
		AND s.job_level_id=l.id
		AND s.effective_date <= $3
		AND s.end_date >= $3
	WHERE l.tenant_id=$1 AND l.code=$2
	ORDER BY s.effective_date DESC
	LIMIT 1
	`, pgUUID(tenantID), strings.TrimSpace(code), pgValidDate(asOf)).Scan(&row.ID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive); err != nil {
		return services.JobLevelRow{}, err
	}
	return row, nil
}

func (r *OrgRepository) ListJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, asOf time.Time) ([]services.JobProfileJobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		WITH profile_slice AS (
			SELECT s.id
			FROM org_job_profile_slices s
			WHERE s.tenant_id=$1
				AND s.job_profile_id=$2
				AND s.effective_date <= $3
				AND s.end_date >= $3
			ORDER BY s.effective_date DESC
			LIMIT 1
		)
		SELECT
			jf.id,
			jf.code,
			jfs.name,
			pjf.is_primary,
			jfg.id,
			jfg.code,
			jfgs.name
		FROM profile_slice ps
		JOIN org_job_profile_slice_job_families pjf
			ON pjf.tenant_id=$1
			AND pjf.job_profile_slice_id=ps.id
		JOIN org_job_families jf
			ON jf.tenant_id=pjf.tenant_id
			AND jf.id=pjf.job_family_id
		JOIN org_job_family_slices jfs
			ON jfs.tenant_id=jf.tenant_id
			AND jfs.job_family_id=jf.id
			AND jfs.effective_date <= $3
			AND jfs.end_date >= $3
		JOIN org_job_family_groups jfg
			ON jfg.tenant_id=jf.tenant_id
			AND jfg.id=jf.job_family_group_id
		JOIN org_job_family_group_slices jfgs
			ON jfgs.tenant_id=jfg.tenant_id
			AND jfgs.job_family_group_id=jfg.id
			AND jfgs.effective_date <= $3
			AND jfgs.end_date >= $3
		ORDER BY pjf.is_primary DESC, jf.code ASC, jf.id ASC
		`, pgUUID(tenantID), pgUUID(jobProfileID), pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.JobProfileJobFamilyRow, 0, 16)
	for rows.Next() {
		var row services.JobProfileJobFamilyRow
		if err := rows.Scan(
			&row.JobFamilyID,
			&row.JobFamilyCode,
			&row.JobFamilyName,
			&row.IsPrimary,
			&row.JobFamilyGroupID,
			&row.JobFamilyGroupCode,
			&row.JobFamilyGroupName,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) ListJobProfileJobFamiliesByProfileIDsAsOf(ctx context.Context, tenantID uuid.UUID, jobProfileIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID][]services.JobProfileJobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]services.JobProfileJobFamilyRow, len(jobProfileIDs))
	if len(jobProfileIDs) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		WITH prof_ids AS (
			SELECT unnest($2::uuid[]) AS job_profile_id
		),
		profile_slices AS (
			SELECT i.job_profile_id, s.id AS slice_id
			FROM prof_ids i
			JOIN org_job_profile_slices s
				ON s.tenant_id=$1
				AND s.job_profile_id=i.job_profile_id
				AND s.effective_date <= $3
				AND s.end_date >= $3
		)
		SELECT
			ps.job_profile_id,
			jf.id,
			jf.code,
			jfs.name,
			pjf.is_primary,
			jfg.id,
			jfg.code,
			jfgs.name
		FROM profile_slices ps
		JOIN org_job_profile_slice_job_families pjf
			ON pjf.tenant_id=$1
			AND pjf.job_profile_slice_id=ps.slice_id
		JOIN org_job_families jf
			ON jf.tenant_id=pjf.tenant_id
			AND jf.id=pjf.job_family_id
		JOIN org_job_family_slices jfs
			ON jfs.tenant_id=jf.tenant_id
			AND jfs.job_family_id=jf.id
			AND jfs.effective_date <= $3
			AND jfs.end_date >= $3
		JOIN org_job_family_groups jfg
			ON jfg.tenant_id=jf.tenant_id
			AND jfg.id=jf.job_family_group_id
		JOIN org_job_family_group_slices jfgs
			ON jfgs.tenant_id=jfg.tenant_id
			AND jfgs.job_family_group_id=jfg.id
			AND jfgs.effective_date <= $3
			AND jfgs.end_date >= $3
		ORDER BY ps.job_profile_id ASC, pjf.is_primary DESC, jf.code ASC, jf.id ASC
		`, pgUUID(tenantID), jobProfileIDs, pgValidDate(asOf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var profileID uuid.UUID
		var row services.JobProfileJobFamilyRow
		if err := rows.Scan(
			&profileID,
			&row.JobFamilyID,
			&row.JobFamilyCode,
			&row.JobFamilyName,
			&row.IsPrimary,
			&row.JobFamilyGroupID,
			&row.JobFamilyGroupCode,
			&row.JobFamilyGroupName,
		); err != nil {
			return nil, err
		}
		out[profileID] = append(out[profileID], row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *OrgRepository) SetJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, in services.JobProfileJobFamiliesSet) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
	DELETE FROM org_job_profile_job_families
	WHERE tenant_id=$1 AND job_profile_id=$2
	`, pgUUID(tenantID), pgUUID(jobProfileID))
	if err != nil {
		return err
	}
	if len(in.Items) == 0 {
		return nil
	}

	familyIDs := make([]uuid.UUID, 0, len(in.Items))
	primaries := make([]bool, 0, len(in.Items))
	for _, it := range in.Items {
		familyIDs = append(familyIDs, it.JobFamilyID)
		primaries = append(primaries, it.IsPrimary)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO org_job_profile_job_families (
			tenant_id,
			job_profile_id,
			job_family_id,
			is_primary
		)
		SELECT $1, $2, x.job_family_id, x.is_primary
		FROM UNNEST($3::uuid[], $4::bool[]) AS x(job_family_id, is_primary)
		`, pgUUID(tenantID), pgUUID(jobProfileID), familyIDs, primaries)
	return err
}
