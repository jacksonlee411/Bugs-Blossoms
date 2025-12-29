-- DEV-PLAN-069 / C.2
-- Fix 1 inconsistent org_edge slice (child_edge_id) and rewrite descendant paths from its old prefix.
--
-- This is meant to be executed OFFLINE and iterated until the inconsistency count reaches 0.
--
-- Usage:
--   PGPASSWORD=... psql "$DSN" -v ON_ERROR_STOP=1 \
--     -v tenant_id='00000000-0000-0000-0000-000000000001' \
--     -v hierarchy_type='OrgUnit' \
--     -v child_edge_id='32207122-ea2e-4614-ab24-4936e5491ae6' \
--     -f scripts/org/069_fix_org_edges_path_one_batch.sql
--
\set tenant_id :tenant_id
\set hierarchy_type :hierarchy_type
\set child_edge_id :child_edge_id

-- NOTE:
-- This script uses plain SQL (no DO $$) because psql variable substitution does not expand inside dollar-quoted
-- strings, and a DO block would see literals like :'tenant_id' at the server side.

BEGIN;

WITH child AS (
  SELECT
    c.id,
    c.child_node_id,
    c.parent_node_id,
    c.effective_date::date AS effective_date,
    c.path AS old_prefix
  FROM org_edges c
  WHERE c.tenant_id=(:'tenant_id')::uuid
    AND c.hierarchy_type=(:'hierarchy_type')::text
    AND c.id=(:'child_edge_id')::uuid
  FOR UPDATE
),
parent AS (
  SELECT p.path AS parent_path
  FROM org_edges p
  JOIN child c
    ON p.tenant_id=(:'tenant_id')::uuid
   AND p.hierarchy_type=(:'hierarchy_type')::text
   AND p.child_node_id=c.parent_node_id
   AND p.effective_date <= c.effective_date
   AND p.end_date >= c.effective_date
  ORDER BY p.effective_date DESC
  LIMIT 1
  FOR UPDATE
),
calc AS (
  SELECT
    c.id AS child_edge_id,
    c.effective_date,
    c.old_prefix,
    (p.parent_path || replace(lower(c.child_node_id::text), '-', '')::ltree) AS new_prefix
  FROM child c
  JOIN parent p ON true
),
fix_child AS (
  UPDATE org_edges e
  SET
    path=calc.new_prefix,
    depth=nlevel(calc.new_prefix) - 1
  FROM calc
  WHERE e.tenant_id=(:'tenant_id')::uuid
    AND e.hierarchy_type=(:'hierarchy_type')::text
    AND e.id=calc.child_edge_id
  RETURNING 1
),
rewrite_desc AS (
  UPDATE org_edges e
  SET
    path  = (calc.new_prefix) || subpath(e.path, nlevel(calc.old_prefix)),
    depth = nlevel((calc.new_prefix) || subpath(e.path, nlevel(calc.old_prefix))) - 1
  FROM calc
  WHERE e.tenant_id=(:'tenant_id')::uuid
    AND e.hierarchy_type=(:'hierarchy_type')::text
    AND e.effective_date >= calc.effective_date
    AND e.path <@ calc.old_prefix
    AND nlevel(e.path) > nlevel(calc.old_prefix)
  RETURNING 1
)
SELECT
  calc.child_edge_id,
  calc.effective_date,
  (SELECT count(*) FROM rewrite_desc) AS rewritten_descendant_edges
FROM calc;

COMMIT;
