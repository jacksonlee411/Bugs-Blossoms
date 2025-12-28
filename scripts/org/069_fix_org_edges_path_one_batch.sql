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

DO $$
DECLARE
  v_tenant_id uuid := (:'tenant_id')::uuid;
  v_hierarchy_type text := (:'hierarchy_type')::text;
  v_child_edge_id uuid := (:'child_edge_id')::uuid;

  v_child_node_id uuid;
  v_effective_date date;
  v_old_prefix ltree;

  v_parent_node_id uuid;
  v_parent_path ltree;

  v_child_key ltree;
  v_new_prefix ltree;

  v_rows bigint;
BEGIN
  SELECT c.child_node_id, c.parent_node_id, c.effective_date, c.path
  INTO v_child_node_id, v_parent_node_id, v_effective_date, v_old_prefix
  FROM org_edges c
  WHERE c.tenant_id=v_tenant_id AND c.hierarchy_type=v_hierarchy_type AND c.id=v_child_edge_id
  LIMIT 1
  FOR UPDATE;

  IF v_child_node_id IS NULL THEN
    RAISE EXCEPTION 'child edge not found (tenant_id=%, hierarchy_type=%, child_edge_id=%)', v_tenant_id, v_hierarchy_type, v_child_edge_id;
  END IF;

  IF v_parent_node_id IS NULL THEN
    RAISE EXCEPTION 'child edge has NULL parent_node_id (tenant_id=%, hierarchy_type=%, child_edge_id=%)', v_tenant_id, v_hierarchy_type, v_child_edge_id;
  END IF;

  SELECT p.path
  INTO v_parent_path
  FROM org_edges p
  WHERE p.tenant_id=v_tenant_id
    AND p.hierarchy_type=v_hierarchy_type
    AND p.child_node_id=v_parent_node_id
    AND p.effective_date <= v_effective_date
    AND p.end_date >= v_effective_date
  ORDER BY p.effective_date DESC
  LIMIT 1
  FOR UPDATE;

  IF v_parent_path IS NULL THEN
    RAISE EXCEPTION 'parent edge not found at effective_date (tenant_id=%, hierarchy_type=%, parent_node_id=%, effective_date=%)', v_tenant_id, v_hierarchy_type, v_parent_node_id, v_effective_date;
  END IF;

  v_child_key := replace(lower(v_child_node_id::text), '-', '')::ltree;
  v_new_prefix := v_parent_path || v_child_key;

  UPDATE org_edges
  SET
    path=v_new_prefix,
    depth=nlevel(v_new_prefix) - 1
  WHERE tenant_id=v_tenant_id AND hierarchy_type=v_hierarchy_type AND id=v_child_edge_id;

  UPDATE org_edges e
  SET
    path  = (v_new_prefix) || subpath(e.path, nlevel(v_old_prefix)),
    depth = nlevel((v_new_prefix) || subpath(e.path, nlevel(v_old_prefix))) - 1
  WHERE e.tenant_id=v_tenant_id
    AND e.hierarchy_type=v_hierarchy_type
    AND e.effective_date >= v_effective_date
    AND e.path <@ v_old_prefix
    AND nlevel(e.path) > nlevel(v_old_prefix);
  GET DIAGNOSTICS v_rows = ROW_COUNT;

  RAISE NOTICE 'fixed child_edge_id=%, effective_date=%, rewritten_descendant_edges=%', v_child_edge_id, v_effective_date, v_rows;
END $$;

