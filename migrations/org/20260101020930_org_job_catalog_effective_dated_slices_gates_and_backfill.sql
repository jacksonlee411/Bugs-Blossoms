-- +goose Up
-- DEV-PLAN-075 (Phase A): add DB gates (gap-free + profile-slice primary validation) and backfill baseline slices.

-- org_job_profile_slice_job_families: enforce "exactly one primary" per slice (deferrable; commit-time).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_slice_job_families_validate () RETURNS TRIGGER AS $$
DECLARE
	t_id uuid;
	s_id uuid;
	primary_count int;
	parent_exists boolean;
BEGIN
	t_id := COALESCE(NEW.tenant_id, OLD.tenant_id);
	s_id := COALESCE(NEW.job_profile_slice_id, OLD.job_profile_slice_id);

	SELECT EXISTS (
		SELECT 1
		FROM org_job_profile_slices s
		WHERE s.tenant_id = t_id
		  AND s.id = s_id
	) INTO parent_exists;
	IF NOT parent_exists THEN
		RETURN NULL;
	END IF;

	SELECT COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)
	INTO primary_count
	FROM org_job_profile_slice_job_families
	WHERE tenant_id = t_id
	  AND job_profile_slice_id = s_id;

	IF primary_count <> 1 THEN
		RAISE EXCEPTION USING
			ERRCODE = '23000',
			CONSTRAINT = 'org_job_profile_slice_job_families_invalid_body',
			MESSAGE = format('job profile slice job families must have exactly one primary (tenant_id=%s job_profile_slice_id=%s count=%s)', t_id, s_id, primary_count);
	END IF;

	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_profile_slice_job_families_validate_trigger ON org_job_profile_slice_job_families;
CREATE CONSTRAINT TRIGGER org_job_profile_slice_job_families_validate_trigger
	AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_slice_job_families DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE FUNCTION org_job_profile_slice_job_families_validate ();

-- DEV-PLAN-075: commit-time gap-free gate (DEFERRABLE CONSTRAINT TRIGGER).

-- org_job_family_group_slices: key (tenant_id, job_family_group_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_family_group_slices_gap_free_assert(p_tenant_id uuid, p_job_family_group_id uuid) RETURNS void AS $$
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
		FROM org_job_family_group_slices
		WHERE tenant_id = p_tenant_id AND job_family_group_id = p_job_family_group_id
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
			CONSTRAINT = 'org_job_family_group_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s job_family_group_id=%s)', p_tenant_id, p_job_family_group_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_family_group_slices_gap_free_trigger() RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_job_family_group_slices_gap_free_assert(OLD.tenant_id, OLD.job_family_group_id);
		RETURN NULL;
	END IF;
	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.job_family_group_id) IS DISTINCT FROM (NEW.tenant_id, NEW.job_family_group_id) THEN
		PERFORM org_job_family_group_slices_gap_free_assert(OLD.tenant_id, OLD.job_family_group_id);
	END IF;
	PERFORM org_job_family_group_slices_gap_free_assert(NEW.tenant_id, NEW.job_family_group_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_family_group_slices_gap_free ON org_job_family_group_slices;
CREATE CONSTRAINT TRIGGER org_job_family_group_slices_gap_free
	AFTER INSERT OR UPDATE OR DELETE ON org_job_family_group_slices DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE FUNCTION org_job_family_group_slices_gap_free_trigger();

-- org_job_family_slices: key (tenant_id, job_family_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_family_slices_gap_free_assert(p_tenant_id uuid, p_job_family_id uuid) RETURNS void AS $$
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
		FROM org_job_family_slices
		WHERE tenant_id = p_tenant_id AND job_family_id = p_job_family_id
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
			CONSTRAINT = 'org_job_family_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s job_family_id=%s)', p_tenant_id, p_job_family_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_family_slices_gap_free_trigger() RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_job_family_slices_gap_free_assert(OLD.tenant_id, OLD.job_family_id);
		RETURN NULL;
	END IF;
	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.job_family_id) IS DISTINCT FROM (NEW.tenant_id, NEW.job_family_id) THEN
		PERFORM org_job_family_slices_gap_free_assert(OLD.tenant_id, OLD.job_family_id);
	END IF;
	PERFORM org_job_family_slices_gap_free_assert(NEW.tenant_id, NEW.job_family_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_family_slices_gap_free ON org_job_family_slices;
CREATE CONSTRAINT TRIGGER org_job_family_slices_gap_free
	AFTER INSERT OR UPDATE OR DELETE ON org_job_family_slices DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE FUNCTION org_job_family_slices_gap_free_trigger();

-- org_job_level_slices: key (tenant_id, job_level_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_level_slices_gap_free_assert(p_tenant_id uuid, p_job_level_id uuid) RETURNS void AS $$
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
		FROM org_job_level_slices
		WHERE tenant_id = p_tenant_id AND job_level_id = p_job_level_id
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
			CONSTRAINT = 'org_job_level_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s job_level_id=%s)', p_tenant_id, p_job_level_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_level_slices_gap_free_trigger() RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_job_level_slices_gap_free_assert(OLD.tenant_id, OLD.job_level_id);
		RETURN NULL;
	END IF;
	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.job_level_id) IS DISTINCT FROM (NEW.tenant_id, NEW.job_level_id) THEN
		PERFORM org_job_level_slices_gap_free_assert(OLD.tenant_id, OLD.job_level_id);
	END IF;
	PERFORM org_job_level_slices_gap_free_assert(NEW.tenant_id, NEW.job_level_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_level_slices_gap_free ON org_job_level_slices;
CREATE CONSTRAINT TRIGGER org_job_level_slices_gap_free
	AFTER INSERT OR UPDATE OR DELETE ON org_job_level_slices DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE FUNCTION org_job_level_slices_gap_free_trigger();

-- org_job_profile_slices: key (tenant_id, job_profile_id)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_slices_gap_free_assert(p_tenant_id uuid, p_job_profile_id uuid) RETURNS void AS $$
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
		FROM org_job_profile_slices
		WHERE tenant_id = p_tenant_id AND job_profile_id = p_job_profile_id
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
			CONSTRAINT = 'org_job_profile_slices_gap_free',
			MESSAGE = format('time slices must be gap-free (tenant_id=%s job_profile_id=%s)', p_tenant_id, p_job_profile_id);
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION org_job_profile_slices_gap_free_trigger() RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM org_job_profile_slices_gap_free_assert(OLD.tenant_id, OLD.job_profile_id);
		RETURN NULL;
	END IF;
	IF TG_OP = 'UPDATE' AND (OLD.tenant_id, OLD.job_profile_id) IS DISTINCT FROM (NEW.tenant_id, NEW.job_profile_id) THEN
		PERFORM org_job_profile_slices_gap_free_assert(OLD.tenant_id, OLD.job_profile_id);
	END IF;
	PERFORM org_job_profile_slices_gap_free_assert(NEW.tenant_id, NEW.job_profile_id);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS org_job_profile_slices_gap_free ON org_job_profile_slices;
CREATE CONSTRAINT TRIGGER org_job_profile_slices_gap_free
	AFTER INSERT OR UPDATE OR DELETE ON org_job_profile_slices DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE FUNCTION org_job_profile_slices_gap_free_trigger();

-- Baseline backfill: create one initial slice per existing identity row.
INSERT INTO org_job_family_group_slices (tenant_id, job_family_group_id, name, is_active, effective_date, end_date)
SELECT g.tenant_id, g.id, g.name, g.is_active, DATE '1900-01-01', DATE '9999-12-31'
FROM org_job_family_groups g
WHERE NOT EXISTS (
	SELECT 1
	FROM org_job_family_group_slices s
	WHERE s.tenant_id = g.tenant_id
	  AND s.job_family_group_id = g.id
);

INSERT INTO org_job_family_slices (tenant_id, job_family_id, name, is_active, effective_date, end_date)
SELECT f.tenant_id, f.id, f.name, f.is_active, DATE '1900-01-01', DATE '9999-12-31'
FROM org_job_families f
WHERE NOT EXISTS (
	SELECT 1
	FROM org_job_family_slices s
	WHERE s.tenant_id = f.tenant_id
	  AND s.job_family_id = f.id
);

INSERT INTO org_job_level_slices (tenant_id, job_level_id, name, display_order, is_active, effective_date, end_date)
SELECT l.tenant_id, l.id, l.name, l.display_order, l.is_active, DATE '1900-01-01', DATE '9999-12-31'
FROM org_job_levels l
WHERE NOT EXISTS (
	SELECT 1
	FROM org_job_level_slices s
	WHERE s.tenant_id = l.tenant_id
	  AND s.job_level_id = l.id
);

INSERT INTO org_job_profile_slices (tenant_id, job_profile_id, name, description, is_active, external_refs, effective_date, end_date)
SELECT p.tenant_id, p.id, p.name, p.description, p.is_active, p.external_refs, DATE '1900-01-01', DATE '9999-12-31'
FROM org_job_profiles p
WHERE NOT EXISTS (
	SELECT 1
	FROM org_job_profile_slices s
	WHERE s.tenant_id = p.tenant_id
	  AND s.job_profile_id = p.id
);

INSERT INTO org_job_profile_slice_job_families (tenant_id, job_profile_slice_id, job_family_id, is_primary)
SELECT jf.tenant_id, s.id, jf.job_family_id, jf.is_primary
FROM org_job_profile_job_families jf
JOIN org_job_profile_slices s
  ON s.tenant_id = jf.tenant_id
 AND s.job_profile_id = jf.job_profile_id
 AND s.effective_date = DATE '1900-01-01'
WHERE NOT EXISTS (
	SELECT 1
	FROM org_job_profile_slice_job_families x
	WHERE x.tenant_id = jf.tenant_id
	  AND x.job_profile_slice_id = s.id
	  AND x.job_family_id = jf.job_family_id
);

-- +goose Down
-- Best-effort cleanup. Note: schema rollback is handled by the previous migration dropping tables.

DROP TRIGGER IF EXISTS org_job_profile_slices_gap_free ON org_job_profile_slices;
DROP FUNCTION IF EXISTS org_job_profile_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_job_profile_slices_gap_free_assert(uuid, uuid);

DROP TRIGGER IF EXISTS org_job_level_slices_gap_free ON org_job_level_slices;
DROP FUNCTION IF EXISTS org_job_level_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_job_level_slices_gap_free_assert(uuid, uuid);

DROP TRIGGER IF EXISTS org_job_family_slices_gap_free ON org_job_family_slices;
DROP FUNCTION IF EXISTS org_job_family_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_job_family_slices_gap_free_assert(uuid, uuid);

DROP TRIGGER IF EXISTS org_job_family_group_slices_gap_free ON org_job_family_group_slices;
DROP FUNCTION IF EXISTS org_job_family_group_slices_gap_free_trigger();
DROP FUNCTION IF EXISTS org_job_family_group_slices_gap_free_assert(uuid, uuid);

DROP TRIGGER IF EXISTS org_job_profile_slice_job_families_validate_trigger ON org_job_profile_slice_job_families;
DROP FUNCTION IF EXISTS org_job_profile_slice_job_families_validate();

