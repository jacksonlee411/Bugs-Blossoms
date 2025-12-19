package persistence

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListSecurityGroupMappings(ctx context.Context, tenantID uuid.UUID, filter services.SecurityGroupMappingListFilter) ([]services.SecurityGroupMappingRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
SELECT
  id,
  org_node_id,
  security_group_key,
  applies_to_subtree,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_security_group_mappings
WHERE tenant_id = $1
`
	args := []any{pgUUID(tenantID)}
	i := 2

	if filter.OrgNodeID != nil && *filter.OrgNodeID != uuid.Nil {
		q += "\n  AND org_node_id = $" + itoa(i)
		args = append(args, pgUUID(*filter.OrgNodeID))
		i++
	}
	if filter.SecurityGroupKey != nil && *filter.SecurityGroupKey != "" {
		q += "\n  AND security_group_key = $" + itoa(i)
		args = append(args, *filter.SecurityGroupKey)
		i++
	}
	if filter.AsOf != nil && !filter.AsOf.IsZero() {
		q += "\n  AND effective_date <= $" + itoa(i) + "\n  AND end_date > $" + itoa(i)
		args = append(args, filter.AsOf.UTC())
		i++
	}
	if filter.CursorAt != nil && filter.CursorID != nil && !filter.CursorAt.IsZero() && *filter.CursorID != uuid.Nil {
		q += "\n  AND (effective_date, id) < ($" + itoa(i) + ", $" + itoa(i+1) + ")"
		args = append(args, filter.CursorAt.UTC(), pgUUID(*filter.CursorID))
		i += 2
	}

	q += "\nORDER BY effective_date DESC, id DESC\nLIMIT $" + itoa(i)
	args = append(args, filter.Limit+1)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SecurityGroupMappingRow, 0, minInt(filter.Limit+1, 64))
	for rows.Next() {
		var row services.SecurityGroupMappingRow
		if err := rows.Scan(
			&row.ID,
			&row.OrgNodeID,
			&row.SecurityGroupKey,
			&row.AppliesToSubtree,
			&row.EffectiveDate,
			&row.EndDate,
			&row.CreatedAt,
			&row.UpdatedAt,
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

func (r *OrgRepository) InsertSecurityGroupMapping(ctx context.Context, tenantID uuid.UUID, in services.SecurityGroupMappingInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO org_security_group_mappings (
  tenant_id,
  org_node_id,
  security_group_key,
  applies_to_subtree,
  effective_date,
  end_date
)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id
`, pgUUID(tenantID), pgUUID(in.OrgNodeID), in.SecurityGroupKey, in.AppliesToSubtree, in.EffectiveDate, in.EndDate).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) LockSecurityGroupMappingByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (services.SecurityGroupMappingRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.SecurityGroupMappingRow{}, err
	}

	var row services.SecurityGroupMappingRow
	err = tx.QueryRow(ctx, `
SELECT
  id,
  org_node_id,
  security_group_key,
  applies_to_subtree,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_security_group_mappings
WHERE tenant_id=$1 AND id=$2
FOR UPDATE
`, pgUUID(tenantID), pgUUID(id)).Scan(
		&row.ID,
		&row.OrgNodeID,
		&row.SecurityGroupKey,
		&row.AppliesToSubtree,
		&row.EffectiveDate,
		&row.EndDate,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return services.SecurityGroupMappingRow{}, err
	}
	return row, nil
}

func (r *OrgRepository) UpdateSecurityGroupMappingEndDate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
UPDATE org_security_group_mappings
SET end_date=$3, updated_at=now()
WHERE tenant_id=$1 AND id=$2
`, pgUUID(tenantID), pgUUID(id), endDate)
	return err
}

func (r *OrgRepository) ListSecurityGroupMappingsForNodesAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeIDs []uuid.UUID, asOf time.Time) ([]services.SecurityGroupMappingRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if len(orgNodeIDs) == 0 {
		return []services.SecurityGroupMappingRow{}, nil
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	rows, err := tx.Query(ctx, `
SELECT
  id,
  org_node_id,
  security_group_key,
  applies_to_subtree,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_security_group_mappings
WHERE tenant_id=$1
  AND org_node_id = ANY($2::uuid[])
  AND effective_date <= $3
  AND end_date > $3
ORDER BY effective_date DESC, id DESC
`, pgUUID(tenantID), orgNodeIDs, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.SecurityGroupMappingRow, 0, 32)
	for rows.Next() {
		var row services.SecurityGroupMappingRow
		if err := rows.Scan(
			&row.ID,
			&row.OrgNodeID,
			&row.SecurityGroupKey,
			&row.AppliesToSubtree,
			&row.EffectiveDate,
			&row.EndDate,
			&row.CreatedAt,
			&row.UpdatedAt,
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

func (r *OrgRepository) ListOrgLinks(ctx context.Context, tenantID uuid.UUID, filter services.OrgLinkListFilter) ([]services.OrgLinkRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	q := `
SELECT
  id,
  org_node_id,
  object_type,
  object_key,
  link_type,
  metadata,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_links
WHERE tenant_id = $1
`
	args := []any{pgUUID(tenantID)}
	i := 2

	if filter.OrgNodeID != nil && *filter.OrgNodeID != uuid.Nil {
		q += "\n  AND org_node_id = $" + itoa(i)
		args = append(args, pgUUID(*filter.OrgNodeID))
		i++
	}
	if filter.ObjectType != nil && *filter.ObjectType != "" {
		q += "\n  AND object_type = $" + itoa(i)
		args = append(args, *filter.ObjectType)
		i++
	}
	if filter.ObjectKey != nil && *filter.ObjectKey != "" {
		q += "\n  AND object_key = $" + itoa(i)
		args = append(args, *filter.ObjectKey)
		i++
	}
	if filter.AsOf != nil && !filter.AsOf.IsZero() {
		q += "\n  AND effective_date <= $" + itoa(i) + "\n  AND end_date > $" + itoa(i)
		args = append(args, filter.AsOf.UTC())
		i++
	}
	if filter.CursorAt != nil && filter.CursorID != nil && !filter.CursorAt.IsZero() && *filter.CursorID != uuid.Nil {
		q += "\n  AND (effective_date, id) < ($" + itoa(i) + ", $" + itoa(i+1) + ")"
		args = append(args, filter.CursorAt.UTC(), pgUUID(*filter.CursorID))
		i += 2
	}

	q += "\nORDER BY effective_date DESC, id DESC\nLIMIT $" + itoa(i)
	args = append(args, filter.Limit+1)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.OrgLinkRow, 0, minInt(filter.Limit+1, 64))
	for rows.Next() {
		var row services.OrgLinkRow
		var meta json.RawMessage
		if err := rows.Scan(
			&row.ID,
			&row.OrgNodeID,
			&row.ObjectType,
			&row.ObjectKey,
			&row.LinkType,
			&meta,
			&row.EffectiveDate,
			&row.EndDate,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.Metadata = meta
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) InsertOrgLink(ctx context.Context, tenantID uuid.UUID, in services.OrgLinkInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO org_links (
  tenant_id,
  org_node_id,
  object_type,
  object_key,
  link_type,
  metadata,
  effective_date,
  end_date
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id
`, pgUUID(tenantID), pgUUID(in.OrgNodeID), in.ObjectType, in.ObjectKey, in.LinkType, in.Metadata, in.EffectiveDate, in.EndDate).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *OrgRepository) LockOrgLinkByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (services.OrgLinkRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.OrgLinkRow{}, err
	}

	var row services.OrgLinkRow
	var meta json.RawMessage
	err = tx.QueryRow(ctx, `
SELECT
  id,
  org_node_id,
  object_type,
  object_key,
  link_type,
  metadata,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_links
WHERE tenant_id=$1 AND id=$2
FOR UPDATE
`, pgUUID(tenantID), pgUUID(id)).Scan(
		&row.ID,
		&row.OrgNodeID,
		&row.ObjectType,
		&row.ObjectKey,
		&row.LinkType,
		&meta,
		&row.EffectiveDate,
		&row.EndDate,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return services.OrgLinkRow{}, err
	}
	row.Metadata = meta
	return row, nil
}

func (r *OrgRepository) UpdateOrgLinkEndDate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, endDate time.Time) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
UPDATE org_links
SET end_date=$3, updated_at=now()
WHERE tenant_id=$1 AND id=$2
`, pgUUID(tenantID), pgUUID(id), endDate)
	return err
}

func (r *OrgRepository) ListOrgLinksForNodeAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeID uuid.UUID, asOf time.Time, limit int) ([]services.OrgLinkRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 1
	}

	rows, err := tx.Query(ctx, `
SELECT
  id,
  org_node_id,
  object_type,
  object_key,
  link_type,
  metadata,
  effective_date,
  end_date,
  created_at,
  updated_at
FROM org_links
WHERE tenant_id=$1
  AND org_node_id=$2
  AND effective_date <= $3
  AND end_date > $3
ORDER BY object_type ASC, object_key ASC, link_type ASC, id ASC
LIMIT $4
`, pgUUID(tenantID), pgUUID(orgNodeID), asOf.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.OrgLinkRow, 0, minInt(limit, 64))
	for rows.Next() {
		var row services.OrgLinkRow
		var meta json.RawMessage
		if err := rows.Scan(
			&row.ID,
			&row.OrgNodeID,
			&row.ObjectType,
			&row.ObjectKey,
			&row.LinkType,
			&meta,
			&row.EffectiveDate,
			&row.EndDate,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.Metadata = meta
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
