package services

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func mapPgErrorToServiceError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return newServiceError(http.StatusNotFound, "ORG_NOT_FOUND", "not found", err)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.Code {
	case "23505": // unique_violation
		recordWriteConflict("unique")
		switch pgErr.ConstraintName {
		case "org_nodes_tenant_id_code_key":
			return newServiceError(http.StatusConflict, "ORG_CODE_CONFLICT", "code already exists", err)
		case "org_positions_tenant_id_code_key":
			return newServiceError(http.StatusConflict, "ORG_POSITION_CODE_CONFLICT", "position code already exists", err)
		case "org_nodes_tenant_root_unique":
			return newServiceError(http.StatusConflict, "ORG_OVERLAP", "root already exists", err)
		default:
			return newServiceError(http.StatusConflict, "ORG_OVERLAP", "unique constraint violated", err)
		}
	case "23P01": // exclusion_violation
		recordWriteConflict("overlap")
		if strings.Contains(pgErr.ConstraintName, "org_assignments_primary_unique_in_time") {
			return newServiceError(http.StatusConflict, "ORG_PRIMARY_CONFLICT", "primary assignment conflict", err)
		}
		return newServiceError(http.StatusConflict, "ORG_OVERLAP", "time window overlap", err)
	case "23503": // foreign_key_violation
		recordWriteConflict("foreign_key")
		return newServiceError(http.StatusUnprocessableEntity, "ORG_PARENT_NOT_FOUND", "foreign key violation", err)
	case "23000": // integrity_constraint_violation (e.g. cycle detected trigger)
		recordWriteConflict("overlap")
		return newServiceError(http.StatusConflict, "ORG_OVERLAP", "integrity constraint violated", err)
	default:
		return newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", fmt.Sprintf("database error (%s)", pgErr.Code), err)
	}
}
