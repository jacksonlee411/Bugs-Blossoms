package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) BuildOrgReportingNodes(
	ctx context.Context,
	tenantID uuid.UUID,
	hierarchyType string,
	asOfDate time.Time,
	includeSecurityGroups bool,
	includeLinks bool,
	apply bool,
	sourceRequestID string,
) (services.OrgReportingBuildResult, error) {
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return services.OrgReportingBuildResult{}, err
	}

	asOfDate = time.Date(asOfDate.UTC().Year(), asOfDate.UTC().Month(), asOfDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	asOfDateStr := asOfDate.Format("2006-01-02")
	asOfTS := asOfDate
	lockKey := deepReadLockKey("reporting", tenantID.String(), hierarchyType, asOfDateStr)

	ctx = composables.WithPool(ctx, pool)
	ctx = composables.WithTenantID(ctx, tenantID)

	var out services.OrgReportingBuildResult
	out.TenantID = tenantID
	out.HierarchyType = hierarchyType
	out.AsOfDate = asOfDateStr
	out.DryRun = !apply
	out.IncludedSecurityGroups = includeSecurityGroups
	out.IncludedLinks = includeLinks

	if !apply {
		err := withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
			tx, err := composables.UseTx(txCtx)
			if err != nil {
				return err
			}

			buildID, err := activeSnapshotBuildID(txCtx, tx, tenantID, hierarchyType, asOfDateStr)
			if err != nil {
				return err
			}
			out.SnapshotBuildID = buildID

			return tx.QueryRow(txCtx, `
SELECT COUNT(DISTINCT descendant_node_id)::bigint
FROM org_hierarchy_snapshots
WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID)).Scan(&out.RowCount)
		})
		return out, err
	}

	err = withDeepReadLock(ctx, lockKey, func(txCtx context.Context) error {
		tx, err := composables.UseTx(txCtx)
		if err != nil {
			return err
		}

		buildID, err := activeSnapshotBuildID(txCtx, tx, tenantID, hierarchyType, asOfDateStr)
		if err != nil {
			return err
		}
		out.SnapshotBuildID = buildID

		if _, err := tx.Exec(txCtx, `
	DELETE FROM org_reporting_nodes
	WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3::date AND build_id=$4
	`, pgUUID(tenantID), hierarchyType, asOfDateStr, pgUUID(buildID)); err != nil {
			return err
		}

		ctes := []string{`
	nodes_asof AS (
		SELECT
			n.id AS org_node_id,
			n.code,
			s.name,
		s.status,
		jsonb_strip_nulls(jsonb_build_object(
			'legal_entity_id', s.legal_entity_id,
			'company_code', s.company_code,
			'location_id', s.location_id,
			'manager_user_id', s.manager_user_id
		)) AS attributes
		FROM org_nodes n
		JOIN org_node_slices s
			ON s.tenant_id = n.tenant_id
			AND s.org_node_id = n.id
			AND s.effective_date <= $3
			AND s.end_date > $3
		WHERE n.tenant_id = $1
	)`, `
	paths AS (
		SELECT
			s.descendant_node_id AS org_node_id,
			array_agg(s.ancestor_node_id ORDER BY s.depth DESC) AS path_node_ids,
			array_agg(an.code ORDER BY s.depth DESC) AS path_codes,
			array_agg(ans.name ORDER BY s.depth DESC) AS path_names
		FROM org_hierarchy_snapshots s
		JOIN org_nodes an
			ON an.tenant_id = s.tenant_id
			AND an.id = s.ancestor_node_id
		JOIN org_node_slices ans
			ON ans.tenant_id = an.tenant_id
			AND ans.org_node_id = an.id
			AND ans.effective_date <= $3
			AND ans.end_date > $3
		WHERE s.tenant_id = $1
			AND s.hierarchy_type = $2
			AND s.as_of_date = $4::date
			AND s.build_id = $5
		GROUP BY s.descendant_node_id
	)`}

		joins := []string{
			"JOIN paths p ON p.org_node_id = n.org_node_id",
		}

		sgSelect := `'{}'::text[]`
		if includeSecurityGroups {
			ctes = append(ctes, `
	sg_best AS (
		SELECT DISTINCT ON (snap.descendant_node_id, m.security_group_key)
			snap.descendant_node_id AS org_node_id,
			m.security_group_key,
		snap.depth AS source_depth,
		m.org_node_id AS source_org_node_id
	FROM org_hierarchy_snapshots snap
	JOIN org_security_group_mappings m
		ON m.tenant_id = snap.tenant_id
		AND m.org_node_id = snap.ancestor_node_id
		WHERE snap.tenant_id = $1
			AND snap.hierarchy_type = $2
			AND snap.as_of_date = $4::date
			AND snap.build_id = $5
			AND m.effective_date <= $3
			AND m.end_date > $3
			AND (m.applies_to_subtree OR m.org_node_id = snap.descendant_node_id)
		ORDER BY snap.descendant_node_id, m.security_group_key, snap.depth ASC, m.org_node_id ASC
	)`, `
	sg AS (
		SELECT
			org_node_id,
			array_agg(security_group_key ORDER BY min_source_depth ASC, security_group_key ASC) AS security_group_keys
		FROM (
		SELECT org_node_id, security_group_key, MIN(source_depth) AS min_source_depth
			FROM sg_best
			GROUP BY org_node_id, security_group_key
		) t
		GROUP BY org_node_id
	)`)
			joins = append(joins, "LEFT JOIN sg ON sg.org_node_id = n.org_node_id")
			sgSelect = `COALESCE(sg.security_group_keys, '{}'::text[])`
		}

		linksSelect := `'[]'::jsonb`
		if includeLinks {
			ctes = append(ctes, `
	l AS (
		SELECT
			org_node_id,
			jsonb_agg(jsonb_build_object(
				'object_type', object_type,
			'object_key', object_key,
			'link_type', link_type
		) ORDER BY object_type ASC, object_key ASC, link_type ASC) AS links
	FROM org_links
		WHERE tenant_id = $1
			AND effective_date <= $3
			AND end_date > $3
		GROUP BY org_node_id
	)`)
			joins = append(joins, "LEFT JOIN l ON l.org_node_id = n.org_node_id")
			linksSelect = `COALESCE(l.links, '[]'::jsonb)`
		}

		query := "WITH " + strings.Join(ctes, ",\n") + `
	INSERT INTO org_reporting_nodes (
		tenant_id,
		hierarchy_type,
	as_of_date,
	build_id,
	org_node_id,
	code,
	name,
	status,
	parent_node_id,
	depth,
	path_node_ids,
	path_codes,
	path_names,
	attributes,
	security_group_keys,
	links
)
SELECT
	$1,
	$2,
	$4::date,
	$5,
	n.org_node_id,
	n.code,
	n.name,
	n.status,
	CASE
		WHEN array_length(p.path_node_ids, 1) <= 1 THEN NULL
		ELSE p.path_node_ids[array_length(p.path_node_ids, 1) - 1]
	END AS parent_node_id,
	GREATEST(array_length(p.path_node_ids, 1) - 1, 0) AS depth,
	p.path_node_ids,
		p.path_codes,
		p.path_names,
		n.attributes,
		` + sgSelect + ` AS security_group_keys,
		` + linksSelect + ` AS links
	FROM nodes_asof n
	` + strings.Join(joins, "\n") + `
	`

		tag, err := tx.Exec(txCtx, query, pgUUID(tenantID), hierarchyType, asOfTS, asOfDateStr, pgUUID(buildID))
		if err != nil {
			return err
		}

		out.RowCount = tag.RowsAffected()
		return nil
	})

	return out, err
}
