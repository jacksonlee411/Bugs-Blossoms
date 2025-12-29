package persistence

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ListNodeSlicesTimeline(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) ([]services.NodeSliceRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
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
	WHERE tenant_id=$1 AND org_node_id=$2
	ORDER BY effective_date DESC
	`, pgUUID(tenantID), pgUUID(nodeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.NodeSliceRow, 0, 16)
	for rows.Next() {
		var row services.NodeSliceRow
		var i18nRaw []byte
		var legal pgtype.UUID
		var location pgtype.UUID
		var parentHint pgtype.UUID
		var company pgtype.Text
		var manager pgtype.Int8
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&i18nRaw,
			&row.Status,
			&legal,
			&company,
			&location,
			&row.DisplayOrder,
			&parentHint,
			&manager,
			&row.EffectiveDate,
			&row.EndDate,
		); err != nil {
			return nil, err
		}

		if len(i18nRaw) > 0 {
			_ = json.Unmarshal(i18nRaw, &row.I18nNames)
		}

		row.LegalEntityID = nullableUUID(legal)
		row.LocationID = nullableUUID(location)
		row.ParentHint = nullableUUID(parentHint)
		row.CompanyCode = nullableText(company)
		row.ManagerUserID = nullableInt8(manager)

		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *OrgRepository) ListEdgesTimelineAsChild(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childNodeID uuid.UUID) ([]services.EdgeTimelineRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
	SELECT
		e.id,
		e.parent_node_id,
		e.child_node_id,
		e.effective_date,
		e.end_date,
		p.code,
		ps.name
	FROM org_edges e
	LEFT JOIN org_nodes p
		ON p.tenant_id = e.tenant_id
		AND p.id = e.parent_node_id
	LEFT JOIN LATERAL (
		SELECT s.name
		FROM org_node_slices s
		WHERE s.tenant_id = e.tenant_id
			AND s.org_node_id = e.parent_node_id
			AND s.effective_date <= e.effective_date
			AND s.end_date >= e.effective_date
		ORDER BY s.effective_date DESC
		LIMIT 1
	) ps ON true
	WHERE e.tenant_id=$1
		AND e.hierarchy_type=$2
		AND e.child_node_id=$3
	ORDER BY e.effective_date DESC
	`, pgUUID(tenantID), strings.TrimSpace(hierarchyType), pgUUID(childNodeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]services.EdgeTimelineRow, 0, 16)
	for rows.Next() {
		var row services.EdgeTimelineRow
		var parentID pgtype.UUID
		var parentCode pgtype.Text
		var parentName pgtype.Text
		if err := rows.Scan(
			&row.EdgeID,
			&parentID,
			&row.ChildNodeID,
			&row.EffectiveDate,
			&row.EndDate,
			&parentCode,
			&parentName,
		); err != nil {
			return nil, err
		}

		if parentID.Valid {
			pid := uuid.UUID(parentID.Bytes)
			row.ParentNodeID = &pid
		}

		if parentCode.Valid {
			v := strings.TrimSpace(parentCode.String)
			if v != "" {
				row.ParentCode = &v
			}
		}
		if parentName.Valid {
			v := strings.TrimSpace(parentName.String)
			if v != "" {
				row.ParentNameAtStart = &v
			}
		}

		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}
