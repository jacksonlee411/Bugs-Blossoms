-- +goose Up
-- DEV-PLAN-066: Commit-time gap-free gate (DEFERRABLE CONSTRAINT TRIGGER).

-- org_node_slices: key (tenant_id, org_node_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_node_slices_gap_free_assert(p_tenant_id uuid, p_org_node_id uuid) RETURNS void AS $$
DECLARE
	has_gap boolean;
	row_count bigint;
	last_end date;
BEGIN
	WITH ordered AS (
		SELECT
			effective_date,
			end_date,
			lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date
		FROM org_node_slices
		WHERE tenant_id = p_tenant_id AND org_node_id = p_org_node_id
		ORDER BY effective_date
	)
	SELECT
		EXISTS (
			SELECT 1
			FROM ordered
			WHERE prev_end_date IS NOT NULL AND (prev_end_date + 1) <> effective_date
		),
		(SELECT COUNT(*) FROM ordered),
		(SELECT end_date FROM ordered ORDER BY effective_date DESC LIMIT 1)
	INTO has_gap, row_count, last_end;

	IF row_count > 0 AND (has_gap OR last_end <> DATE '9999-12-31') THEN
		RAISE EXCEPTION USING
			ERRCODE = '23000',
			CONSTRAINT = 'org_node_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s org_node_id=%s)', p_tenant_id, p_org_node_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_node_slices_gap_free_trigger() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_node_slices_gap_free_assert(OLD.tenant_id, OLD.org_node_id);
		RETURN NULL;
	END IF;

	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.org_node_id) IS DISTINCT FROM (NEW.tenant_id, NEW.org_node_id) THEN
		PERFORM org_node_slices_gap_free_assert(OLD.tenant_id, OLD.org_node_id);
	END IF;
	PERFORM org_node_slices_gap_free_assert(NEW.tenant_id, NEW.org_node_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_node_slices_gap_free ON org_node_slices;
CREATE CONSTRAINT TRIGGER org_node_slices_gap_free
AFTER INSERT OR UPDATE OR DELETE ON org_node_slices
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_node_slices_gap_free_trigger();

-- org_position_slices: key (tenant_id, position_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_position_slices_gap_free_assert(p_tenant_id uuid, p_position_id uuid) RETURNS void AS $$
DECLARE
	has_gap boolean;
	row_count bigint;
	last_end date;
BEGIN
	WITH ordered AS (
		SELECT
			effective_date,
			end_date,
			lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date
		FROM org_position_slices
		WHERE tenant_id = p_tenant_id AND position_id = p_position_id
		ORDER BY effective_date
	)
	SELECT
		EXISTS (
			SELECT 1
			FROM ordered
			WHERE prev_end_date IS NOT NULL AND (prev_end_date + 1) <> effective_date
		),
		(SELECT COUNT(*) FROM ordered),
		(SELECT end_date FROM ordered ORDER BY effective_date DESC LIMIT 1)
	INTO has_gap, row_count, last_end;

	IF row_count > 0 AND (has_gap OR last_end <> DATE '9999-12-31') THEN
		RAISE EXCEPTION USING
			ERRCODE = '23000',
			CONSTRAINT = 'org_position_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s position_id=%s)', p_tenant_id, p_position_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_position_slices_gap_free_trigger() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_position_slices_gap_free_assert(OLD.tenant_id, OLD.position_id);
		RETURN NULL;
	END IF;

	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.position_id) IS DISTINCT FROM (NEW.tenant_id, NEW.position_id) THEN
		PERFORM org_position_slices_gap_free_assert(OLD.tenant_id, OLD.position_id);
	END IF;
	PERFORM org_position_slices_gap_free_assert(NEW.tenant_id, NEW.position_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_position_slices_gap_free ON org_position_slices;
CREATE CONSTRAINT TRIGGER org_position_slices_gap_free
AFTER INSERT OR UPDATE OR DELETE ON org_position_slices
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_position_slices_gap_free_trigger();

-- org_edges: key (tenant_id, hierarchy_type, child_node_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_edges_gap_free_assert(p_tenant_id uuid, p_hierarchy_type text, p_child_node_id uuid) RETURNS void AS $$
DECLARE
	has_gap boolean;
	row_count bigint;
	last_end date;
BEGIN
	WITH ordered AS (
		SELECT
			effective_date,
			end_date,
			lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date
		FROM org_edges
		WHERE tenant_id = p_tenant_id AND hierarchy_type = p_hierarchy_type AND child_node_id = p_child_node_id
		ORDER BY effective_date
	)
	SELECT
		EXISTS (
			SELECT 1
			FROM ordered
			WHERE prev_end_date IS NOT NULL AND (prev_end_date + 1) <> effective_date
		),
		(SELECT COUNT(*) FROM ordered),
		(SELECT end_date FROM ordered ORDER BY effective_date DESC LIMIT 1)
	INTO has_gap, row_count, last_end;

	IF row_count > 0 AND (has_gap OR last_end <> DATE '9999-12-31') THEN
		RAISE EXCEPTION USING
			ERRCODE = '23000',
			CONSTRAINT = 'org_edges_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s hierarchy_type=%s child_node_id=%s)', p_tenant_id, p_hierarchy_type, p_child_node_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_edges_gap_free_trigger() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_edges_gap_free_assert(OLD.tenant_id, OLD.hierarchy_type, OLD.child_node_id);
		RETURN NULL;
	END IF;

	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.hierarchy_type, OLD.child_node_id) IS DISTINCT FROM (NEW.tenant_id, NEW.hierarchy_type, NEW.child_node_id) THEN
		PERFORM org_edges_gap_free_assert(OLD.tenant_id, OLD.hierarchy_type, OLD.child_node_id);
	END IF;
	PERFORM org_edges_gap_free_assert(NEW.tenant_id, NEW.hierarchy_type, NEW.child_node_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_edges_gap_free ON org_edges;
CREATE CONSTRAINT TRIGGER org_edges_gap_free
AFTER INSERT OR UPDATE OR DELETE ON org_edges
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_edges_gap_free_trigger();

-- org_assignments: key (tenant_id, subject_type, subject_id, assignment_type) but only assignment_type='primary'
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_assignments_gap_free_assert(p_tenant_id uuid, p_subject_type text, p_subject_id uuid, p_assignment_type text) RETURNS void AS $$
DECLARE
	has_gap boolean;
	row_count bigint;
	last_end date;
BEGIN
	IF p_assignment_type IS DISTINCT FROM 'primary' THEN
		RETURN;
	END IF;

	WITH ordered AS (
		SELECT
			effective_date,
			end_date,
			lag(end_date) OVER (ORDER BY effective_date) AS prev_end_date
		FROM org_assignments
		WHERE tenant_id = p_tenant_id
			AND subject_type = p_subject_type
			AND subject_id = p_subject_id
			AND assignment_type = p_assignment_type
		ORDER BY effective_date
	)
	SELECT
		EXISTS (
			SELECT 1
			FROM ordered
			WHERE prev_end_date IS NOT NULL AND (prev_end_date + 1) <> effective_date
		),
		(SELECT COUNT(*) FROM ordered),
		(SELECT end_date FROM ordered ORDER BY effective_date DESC LIMIT 1)
	INTO has_gap, row_count, last_end;

	IF row_count > 0 AND (has_gap OR last_end <> DATE '9999-12-31') THEN
		RAISE EXCEPTION USING
			ERRCODE = '23000',
			CONSTRAINT = 'org_assignments_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s subject_type=%s subject_id=%s assignment_type=%s)', p_tenant_id, p_subject_type, p_subject_id, p_assignment_type);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_assignments_gap_free_trigger() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_assignments_gap_free_assert(OLD.tenant_id, OLD.subject_type, OLD.subject_id, OLD.assignment_type);
		RETURN NULL;
	END IF;

	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.subject_type, OLD.subject_id, OLD.assignment_type) IS DISTINCT FROM (NEW.tenant_id, NEW.subject_type, NEW.subject_id, NEW.assignment_type) THEN
		PERFORM org_assignments_gap_free_assert(OLD.tenant_id, OLD.subject_type, OLD.subject_id, OLD.assignment_type);
	END IF;
	PERFORM org_assignments_gap_free_assert(NEW.tenant_id, NEW.subject_type, NEW.subject_id, NEW.assignment_type);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_assignments_gap_free ON org_assignments;
CREATE CONSTRAINT TRIGGER org_assignments_gap_free
AFTER INSERT OR UPDATE OR DELETE ON org_assignments
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION org_assignments_gap_free_trigger();

-- +goose Down
DROP TRIGGER IF EXISTS org_assignments_gap_free ON org_assignments;
DROP FUNCTION IF EXISTS org_assignments_gap_free_trigger();
DROP FUNCTION IF EXISTS org_assignments_gap_free_assert(uuid, text, uuid, text);

DROP TRIGGER IF EXISTS org_edges_gap_free ON org_edges;
DROP FUNCTION IF EXISTS org_edges_gap_free_trigger();
DROP FUNCTION IF EXISTS org_edges_gap_free_assert(uuid, text, uuid);

DROP TRIGGER IF EXISTS org_position_slices_gap_free ON org_position_slices;
DROP FUNCTION IF EXISTS org_position_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_position_slices_gap_free_assert(uuid, uuid);

DROP TRIGGER IF EXISTS org_node_slices_gap_free ON org_node_slices;
DROP FUNCTION IF EXISTS org_node_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_node_slices_gap_free_assert(uuid, uuid);
