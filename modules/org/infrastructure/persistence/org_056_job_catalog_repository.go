package persistence

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (r *OrgRepository) ListJobRoles(ctx context.Context, tenantID uuid.UUID, jobFamilyID uuid.UUID) ([]services.JobRoleRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, job_family_id, code, name, is_active
	FROM org_job_roles
	WHERE tenant_id=$1 AND job_family_id=$2
	ORDER BY code ASC, id ASC
	`, pgUUID(tenantID), pgUUID(jobFamilyID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobRoleRow, 0, 64)
	for rows.Next() {
		var row services.JobRoleRow
		if err := rows.Scan(&row.ID, &row.JobFamilyID, &row.Code, &row.Name, &row.IsActive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OrgRepository) CreateJobRole(ctx context.Context, tenantID uuid.UUID, in services.JobRoleCreate) (services.JobRoleRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobRoleRow{}, err
	}
	var row services.JobRoleRow
	err = tx.QueryRow(ctx, `
	INSERT INTO org_job_roles (tenant_id, job_family_id, code, name, is_active)
	VALUES ($1,$2,$3,$4,$5)
	RETURNING id, job_family_id, code, name, is_active
	`,
		pgUUID(tenantID),
		pgUUID(in.JobFamilyID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		in.IsActive,
	).Scan(&row.ID, &row.JobFamilyID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) UpdateJobRole(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in services.JobRoleUpdate) (services.JobRoleRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobRoleRow{}, err
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

	var row services.JobRoleRow
	err = tx.QueryRow(ctx, `
	UPDATE org_job_roles
	SET
		name = CASE WHEN $3 THEN $4 ELSE name END,
		is_active = CASE WHEN $5 THEN $6 ELSE is_active END,
		updated_at = now()
	WHERE tenant_id=$1 AND id=$2
	RETURNING id, job_family_id, code, name, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setActive,
		active,
	).Scan(&row.ID, &row.JobFamilyID, &row.Code, &row.Name, &row.IsActive)
	return row, err
}

func (r *OrgRepository) ListJobLevels(ctx context.Context, tenantID uuid.UUID, jobRoleID uuid.UUID) ([]services.JobLevelRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT id, job_role_id, code, name, display_order, is_active
	FROM org_job_levels
	WHERE tenant_id=$1 AND job_role_id=$2
	ORDER BY display_order ASC, code ASC, id ASC
	`, pgUUID(tenantID), pgUUID(jobRoleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobLevelRow, 0, 64)
	for rows.Next() {
		var row services.JobLevelRow
		if err := rows.Scan(&row.ID, &row.JobRoleID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive); err != nil {
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
	INSERT INTO org_job_levels (tenant_id, job_role_id, code, name, display_order, is_active)
	VALUES ($1,$2,$3,$4,$5,$6)
	RETURNING id, job_role_id, code, name, display_order, is_active
	`,
		pgUUID(tenantID),
		pgUUID(in.JobRoleID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		in.DisplayOrder,
		in.IsActive,
	).Scan(&row.ID, &row.JobRoleID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive)
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
	RETURNING id, job_role_id, code, name, display_order, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setOrder,
		order,
		setActive,
		active,
	).Scan(&row.ID, &row.JobRoleID, &row.Code, &row.Name, &row.DisplayOrder, &row.IsActive)
	return row, err
}

func (r *OrgRepository) ListJobProfiles(ctx context.Context, tenantID uuid.UUID, jobRoleID *uuid.UUID) ([]services.JobProfileRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	var jobRole pgtype.UUID
	if jobRoleID != nil {
		jobRole = pgUUID(*jobRoleID)
	}
	rows, err := tx.Query(ctx, `
	SELECT id, code, name, description, job_role_id, is_active
	FROM org_job_profiles
	WHERE tenant_id=$1 AND ($2::uuid IS NULL OR job_role_id=$2)
	ORDER BY code ASC, id ASC
	`, pgUUID(tenantID), jobRole)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]services.JobProfileRow, 0, 64)
	for rows.Next() {
		var row services.JobProfileRow
		var desc pgtype.Text
		if err := rows.Scan(&row.ID, &row.Code, &row.Name, &desc, &row.JobRoleID, &row.IsActive); err != nil {
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
	INSERT INTO org_job_profiles (tenant_id, code, name, description, job_role_id, is_active)
	VALUES ($1,$2,$3,$4,$5,$6)
	RETURNING id, code, name, description, job_role_id, is_active
	`,
		pgUUID(tenantID),
		strings.TrimSpace(in.Code),
		strings.TrimSpace(in.Name),
		desc,
		pgUUID(in.JobRoleID),
		in.IsActive,
	).Scan(&row.ID, &row.Code, &row.Name, &desc, &row.JobRoleID, &row.IsActive)
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
	RETURNING id, code, name, description, job_role_id, is_active
	`,
		pgUUID(tenantID),
		pgUUID(id),
		setName,
		name,
		setDesc,
		desc,
		setActive,
		active,
	).Scan(&row.ID, &row.Code, &row.Name, &outDesc, &row.JobRoleID, &row.IsActive)
	row.Description = nullableText(outDesc)
	return row, err
}

func (r *OrgRepository) SetJobProfileAllowedLevels(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, in services.JobProfileAllowedLevelsSet) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
	DELETE FROM org_job_profile_allowed_job_levels
	WHERE tenant_id=$1 AND job_profile_id=$2
	`, pgUUID(tenantID), pgUUID(jobProfileID))
	if err != nil {
		return err
	}
	if len(in.JobLevelIDs) == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
	INSERT INTO org_job_profile_allowed_job_levels (tenant_id, job_profile_id, job_level_id)
	SELECT $1, $2, UNNEST($3::uuid[])
	`, pgUUID(tenantID), pgUUID(jobProfileID), in.JobLevelIDs)
	return err
}

func (r *OrgRepository) ListJobProfileAllowedLevels(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) ([]uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
	SELECT job_level_id
	FROM org_job_profile_allowed_job_levels
	WHERE tenant_id=$1 AND job_profile_id=$2
	ORDER BY job_level_id ASC
	`, pgUUID(tenantID), pgUUID(jobProfileID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0, 16)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *OrgRepository) JobLevelExistsUnderRole(ctx context.Context, tenantID uuid.UUID, jobRoleID uuid.UUID, jobLevelID uuid.UUID) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
	SELECT EXISTS(
		SELECT 1
		FROM org_job_levels
		WHERE tenant_id=$1 AND job_role_id=$2 AND id=$3
	)
	`, pgUUID(tenantID), pgUUID(jobRoleID), pgUUID(jobLevelID)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *OrgRepository) GetJobProfileRef(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) (services.JobProfileRef, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobProfileRef{}, err
	}
	var ref services.JobProfileRef
	if err := tx.QueryRow(ctx, `
	SELECT id, job_role_id, is_active
	FROM org_job_profiles
	WHERE tenant_id=$1 AND id=$2
	`, pgUUID(tenantID), pgUUID(jobProfileID)).Scan(&ref.ID, &ref.JobRoleID, &ref.IsActive); err != nil {
		return services.JobProfileRef{}, err
	}
	return ref, nil
}

func (r *OrgRepository) JobProfileAllowsLevel(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, jobLevelID uuid.UUID) (bool, bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, false, err
	}
	var hasAllowlist bool
	var inAllowlist bool
	if err := tx.QueryRow(ctx, `
	SELECT
		EXISTS(
			SELECT 1
			FROM org_job_profile_allowed_job_levels
			WHERE tenant_id=$1 AND job_profile_id=$2
		) AS has_allowlist,
		EXISTS(
			SELECT 1
			FROM org_job_profile_allowed_job_levels
			WHERE tenant_id=$1 AND job_profile_id=$2 AND job_level_id=$3
		) AS in_allowlist
	`, pgUUID(tenantID), pgUUID(jobProfileID), pgUUID(jobLevelID)).Scan(&hasAllowlist, &inAllowlist); err != nil {
		return false, false, err
	}
	if !hasAllowlist {
		return true, true, nil
	}
	return inAllowlist, false, nil
}

func (r *OrgRepository) ResolveJobCatalogPathByCodes(ctx context.Context, tenantID uuid.UUID, codes services.JobCatalogCodes) (services.JobCatalogResolvedPath, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.JobCatalogResolvedPath{}, err
	}

	groupCode := strings.TrimSpace(codes.JobFamilyGroupCode)
	familyCode := strings.TrimSpace(codes.JobFamilyCode)
	roleCode := strings.TrimSpace(codes.JobRoleCode)
	levelCode := strings.TrimSpace(codes.JobLevelCode)
	if groupCode == "" || familyCode == "" || roleCode == "" || levelCode == "" {
		return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
	}

	var groupID uuid.UUID
	var groupActive bool
	if err := tx.QueryRow(ctx, `
	SELECT id, is_active
	FROM org_job_family_groups
	WHERE tenant_id=$1 AND code=$2
	`, pgUUID(tenantID), groupCode).Scan(&groupID, &groupActive); err != nil {
		if err == pgx.ErrNoRows {
			return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
		}
		return services.JobCatalogResolvedPath{}, err
	}
	if !groupActive {
		return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
	}

	var familyID uuid.UUID
	var familyActive bool
	if err := tx.QueryRow(ctx, `
	SELECT id, is_active
	FROM org_job_families
	WHERE tenant_id=$1 AND job_family_group_id=$2 AND code=$3
	`, pgUUID(tenantID), pgUUID(groupID), familyCode).Scan(&familyID, &familyActive); err != nil {
		if err == pgx.ErrNoRows {
			var existsAny bool
			_ = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM org_job_families WHERE tenant_id=$1 AND code=$2 AND is_active
				)
				`, pgUUID(tenantID), familyCode).Scan(&existsAny)
			if existsAny {
				return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInvalidHierarchy
			}
			return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
		}
		return services.JobCatalogResolvedPath{}, err
	}
	if !familyActive {
		return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
	}

	var roleID uuid.UUID
	var roleActive bool
	if err := tx.QueryRow(ctx, `
	SELECT id, is_active
	FROM org_job_roles
	WHERE tenant_id=$1 AND job_family_id=$2 AND code=$3
	`, pgUUID(tenantID), pgUUID(familyID), roleCode).Scan(&roleID, &roleActive); err != nil {
		if err == pgx.ErrNoRows {
			var existsAny bool
			_ = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM org_job_roles WHERE tenant_id=$1 AND code=$2 AND is_active
				)
				`, pgUUID(tenantID), roleCode).Scan(&existsAny)
			if existsAny {
				return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInvalidHierarchy
			}
			return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
		}
		return services.JobCatalogResolvedPath{}, err
	}
	if !roleActive {
		return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
	}

	var levelID uuid.UUID
	var levelActive bool
	if err := tx.QueryRow(ctx, `
	SELECT id, is_active
	FROM org_job_levels
	WHERE tenant_id=$1 AND job_role_id=$2 AND code=$3
	`, pgUUID(tenantID), pgUUID(roleID), levelCode).Scan(&levelID, &levelActive); err != nil {
		if err == pgx.ErrNoRows {
			var existsAny bool
			_ = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM org_job_levels WHERE tenant_id=$1 AND code=$2 AND is_active
				)
				`, pgUUID(tenantID), levelCode).Scan(&existsAny)
			if existsAny {
				return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInvalidHierarchy
			}
			return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
		}
		return services.JobCatalogResolvedPath{}, err
	}
	if !levelActive {
		return services.JobCatalogResolvedPath{}, services.ErrJobCatalogInactiveOrMissing
	}

	return services.JobCatalogResolvedPath{
		JobFamilyGroupID: groupID,
		JobFamilyID:      familyID,
		JobRoleID:        roleID,
		JobLevelID:       levelID,
	}, nil
}
