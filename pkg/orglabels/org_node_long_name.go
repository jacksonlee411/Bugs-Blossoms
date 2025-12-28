package orglabels

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type OrgNodeLongNameQuery struct {
	OrgNodeID uuid.UUID
	AsOfDay   time.Time
}

type OrgNodeLongNameKey struct {
	OrgNodeID uuid.UUID
	AsOfDate  string
}

func ResolveOrgNodeLongNamesAsOf(
	ctx context.Context,
	tenantID uuid.UUID,
	asOfDay time.Time,
	orgNodeIDs []uuid.UUID,
) (map[uuid.UUID]string, error) {
	asOfDay = normalizeValidDayUTC(asOfDay)
	if tenantID == uuid.Nil || asOfDay.IsZero() || len(orgNodeIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	queries := make([]OrgNodeLongNameQuery, 0, len(orgNodeIDs))
	for _, id := range orgNodeIDs {
		if id == uuid.Nil {
			continue
		}
		queries = append(queries, OrgNodeLongNameQuery{OrgNodeID: id, AsOfDay: asOfDay})
	}
	if len(queries) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	byKey, err := ResolveOrgNodeLongNames(ctx, tenantID, queries)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]string, len(orgNodeIDs))
	asOfDate := asOfDay.Format(time.DateOnly)
	for _, id := range orgNodeIDs {
		if id == uuid.Nil {
			continue
		}
		out[id] = byKey[OrgNodeLongNameKey{OrgNodeID: id, AsOfDate: asOfDate}]
	}
	return out, nil
}

func ResolveOrgNodeLongNames(
	ctx context.Context,
	tenantID uuid.UUID,
	queries []OrgNodeLongNameQuery,
) (map[OrgNodeLongNameKey]string, error) {
	if tenantID == uuid.Nil || len(queries) == 0 {
		return map[OrgNodeLongNameKey]string{}, nil
	}

	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}

	type pair struct {
		id    uuid.UUID
		asOf  time.Time
		asStr string
	}

	seen := make(map[OrgNodeLongNameKey]struct{}, len(queries))
	pairs := make([]pair, 0, len(queries))
	for _, q := range queries {
		if q.OrgNodeID == uuid.Nil {
			continue
		}
		asOf := normalizeValidDayUTC(q.AsOfDay)
		if asOf.IsZero() {
			continue
		}
		asStr := asOf.Format(time.DateOnly)
		k := OrgNodeLongNameKey{OrgNodeID: q.OrgNodeID, AsOfDate: asStr}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		pairs = append(pairs, pair{id: q.OrgNodeID, asOf: asOf, asStr: asStr})
	}
	if len(pairs) == 0 {
		return map[OrgNodeLongNameKey]string{}, nil
	}

	nodeIDs := make([]uuid.UUID, 0, len(pairs))
	asOfDates := make([]string, 0, len(pairs))
	for _, p := range pairs {
		nodeIDs = append(nodeIDs, p.id)
		asOfDates = append(asOfDates, p.asStr)
	}

	rows, err := tx.Query(ctx, mixedAsOfQuery, pgUUID(tenantID), pgUUIDArray(nodeIDs), pgTextArray(asOfDates))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[OrgNodeLongNameKey]string, len(pairs))
	for rows.Next() {
		var nodeID uuid.UUID
		var asOfDate string
		var longName string
		if err := rows.Scan(&nodeID, &asOfDate, &longName); err != nil {
			return nil, err
		}
		asOfDate = strings.TrimSpace(asOfDate)
		out[OrgNodeLongNameKey{OrgNodeID: nodeID, AsOfDate: asOfDate}] = strings.TrimSpace(longName)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	for _, p := range pairs {
		k := OrgNodeLongNameKey{OrgNodeID: p.id, AsOfDate: p.asStr}
		if _, ok := out[k]; !ok {
			out[k] = ""
		}
	}
	return out, nil
}

const mixedAsOfQuery = `
WITH input AS (
  SELECT *
  FROM unnest($2::uuid[], $3::text[]) AS q(org_node_id, as_of_date)
),
target AS (
  SELECT
    i.org_node_id,
    i.as_of_date::date AS as_of_day,
    e.path
  FROM input i
  JOIN org_edges e
    ON e.tenant_id=$1
   AND e.hierarchy_type='OrgUnit'
   AND e.child_node_id=i.org_node_id
   AND e.effective_date <= i.as_of_date::date
   AND e.end_date >= i.as_of_date::date
),
path_parts AS (
  SELECT
    t.org_node_id,
    t.as_of_day,
    p.ord,
    p.key_text::uuid AS ancestor_id
  FROM target t
  CROSS JOIN LATERAL unnest(string_to_array(t.path::text, '.')) WITH ORDINALITY AS p(key_text, ord)
),
parts AS (
  SELECT
    a.org_node_id,
    a.as_of_day,
    a.ord,
    COALESCE(NULLIF(BTRIM(ns.name),''), NULLIF(BTRIM(n.code),''), n.id::text) AS part
  FROM path_parts a
  JOIN org_nodes n
    ON n.tenant_id=$1 AND n.id=a.ancestor_id
  LEFT JOIN org_node_slices ns
    ON ns.tenant_id=$1 AND ns.org_node_id=a.ancestor_id
   AND ns.effective_date <= a.as_of_day AND ns.end_date >= a.as_of_day
)
SELECT
  org_node_id,
  as_of_day::text AS as_of_date,
  string_agg(part, ' / ' ORDER BY ord ASC) AS long_name
FROM parts
GROUP BY org_node_id, as_of_day
`

func normalizeValidDayUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgUUIDArray(ids []uuid.UUID) pgtype.FlatArray[uuid.UUID] {
	return pgtype.FlatArray[uuid.UUID](ids)
}

func pgTextArray(v []string) pgtype.FlatArray[string] {
	return pgtype.FlatArray[string](v)
}
