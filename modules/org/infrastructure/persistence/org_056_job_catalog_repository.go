package persistence

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListJobFamilyGroups(ctx context.Context, tenantID uuid.UUID) ([]services.JobFamilyGroupRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, code, name, is_active
	FROM org_job_family_groups
	WHERE tenant_id=$1
	ORDER BY code ASC, id ASC
	`, pgUUID(tenantID))
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

func (r *OrgRepository) ListJobFamilies(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupID uuid.UUID) ([]services.JobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, job_family_group_id, code, name, is_active
	FROM org_job_families
	WHERE tenant_id=$1 AND job_family_group_id=$2
	ORDER BY code ASC, id ASC
	`, pgUUID(tenantID), pgUUID(jobFamilyGroupID))
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

func (r *OrgRepository) ListJobLevels(ctx context.Context, tenantID uuid.UUID) ([]services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, code, name, display_order, is_active
	FROM org_job_levels
	WHERE tenant_id=$1
	ORDER BY display_order ASC, code ASC, id ASC
	`, pgUUID(tenantID))
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

func (r *OrgRepository) ListJobProfiles(ctx context.Context, tenantID uuid.UUID) ([]services.JobProfileRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, code, name, description, is_active
	FROM org_job_profiles
	WHERE tenant_id=$1
	ORDER BY code ASC, id ASC
	`, pgUUID(tenantID))
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

func (r *OrgRepository) GetJobProfileRef(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) (services.JobProfileRef, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobProfileRef{}, err
	}
	var ref services.JobProfileRef
	if err := tx.QueryRow(ctx, `
	SELECT id, is_active
	FROM org_job_profiles
	WHERE tenant_id=$1 AND id=$2
	`, pgUUID(tenantID), pgUUID(jobProfileID)).Scan(&ref.ID, &ref.IsActive); err != nil {
		return services.JobProfileRef{}, err
	}
	return ref, nil
}

func (r *OrgRepository) GetJobLevelByCode(ctx context.Context, tenantID uuid.UUID, code string) (services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobLevelRow{}, err
	}
	var row services.JobLevelRow
	if err := tx.QueryRow(ctx, `
	SELECT id, code, name, display_order, is_active
	FROM org_job_levels
	WHERE tenant_id=$1 AND code=$2
	LIMIT 1
	`, pgUUID(tenantID), strings.TrimSpace(code)).Scan(&row.ID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive); err != nil {
		return services.JobLevelRow{}, err
	}
	return row, nil
}

func (r *OrgRepository) ListJobProfileJobFamilies(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) ([]services.JobProfileJobFamilyRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT
			jf.id,
			jf.code,
			jf.name,
			jpf.is_primary,
			jfg.id,
			jfg.code,
			jfg.name
	FROM org_job_profile_job_families jpf
	JOIN org_job_families jf
		ON jf.tenant_id = jpf.tenant_id
		AND jf.id = jpf.job_family_id
	JOIN org_job_family_groups jfg
		ON jfg.tenant_id = jf.tenant_id
		AND jfg.id = jf.job_family_group_id
	WHERE jpf.tenant_id=$1 AND jpf.job_profile_id=$2
	ORDER BY jpf.is_primary DESC, jf.code ASC, jf.id ASC
	`, pgUUID(tenantID), pgUUID(jobProfileID))
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
