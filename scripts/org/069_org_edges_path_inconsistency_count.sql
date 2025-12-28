-- DEV-PLAN-069 / C.1
-- Count org_edges slices whose parent edge path is not a prefix of the child edge path.
--
-- Usage:
--   PGPASSWORD=... psql "$DSN" -v ON_ERROR_STOP=1 \
--     -v tenant_id='00000000-0000-0000-0000-000000000001' \
--     -v hierarchy_type='OrgUnit' \
--     -f scripts/org/069_org_edges_path_inconsistency_count.sql
--
\set tenant_id :tenant_id
\set hierarchy_type :hierarchy_type

SELECT count(*) AS inconsistent_edges
FROM org_edges c
JOIN org_edges p
  ON p.tenant_id=c.tenant_id
 AND p.hierarchy_type=c.hierarchy_type
 AND p.child_node_id=c.parent_node_id
 AND p.effective_date <= c.effective_date
 AND p.end_date >= c.effective_date
WHERE c.tenant_id=(:'tenant_id')::uuid
  AND c.hierarchy_type=(:'hierarchy_type')::text
  AND c.parent_node_id IS NOT NULL
  AND NOT (p.path @> c.path);

