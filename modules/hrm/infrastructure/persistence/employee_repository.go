package persistence

import (
	"context"
	"errors"
	"fmt"

	gerrors "github.com/go-faster/errors"
	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/hrm/domain/aggregates/employee"
	employeesqlc "github.com/iota-uz/iota-sdk/modules/hrm/infrastructure/sqlc/employee"
	"github.com/iota-uz/iota-sdk/pkg/composables"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrEmployeeNotFound = gerrors.New("employee not found")
)

type GormEmployeeRepository struct{}

func NewEmployeeRepository() employee.Repository {
	return &GormEmployeeRepository{}
}

func (g *GormEmployeeRepository) GetPaginated(ctx context.Context, params *employee.FindParams) ([]employee.Employee, error) {
	if params == nil {
		params = &employee.FindParams{}
	}

	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := queries.ListEmployeesPaginated(ctx, employeesqlc.ListEmployeesPaginatedParams{
		TenantID:  pgTenantID,
		RowOffset: int32(offset),
		RowLimit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}

	return toDomainEmployeesFromPaginated(rows)
}

func (g *GormEmployeeRepository) Count(ctx context.Context) (int64, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return 0, err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return 0, err
	}

	return queries.CountEmployees(ctx, pgTenantID)
}

func (g *GormEmployeeRepository) GetAll(ctx context.Context) ([]employee.Employee, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListEmployeesByTenant(ctx, pgTenantID)
	if err != nil {
		return nil, err
	}

	return toDomainEmployeesFromTenantList(rows)
}

func (g *GormEmployeeRepository) GetByID(ctx context.Context, id uint) (employee.Employee, error) {
	tenantUUID, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return nil, err
	}

	row, err := queries.GetEmployeeByID(ctx, employeesqlc.GetEmployeeByIDParams{
		ID:       int32(id),
		TenantID: pgTenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmployeeNotFound
		}
		return nil, err
	}

	if !row.TenantID.Valid || row.TenantID.Bytes != tenantUUID {
		return nil, ErrEmployeeNotFound
	}

	return toDomainEmployeeFromGetRow(row)
}

func (g *GormEmployeeRepository) Create(ctx context.Context, data employee.Employee) (employee.Employee, error) {
	tenantUUID, _, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return nil, err
	}

	employeeParams, metaParams, err := buildEmployeeInsertParams(data, tenantUUID)
	if err != nil {
		return nil, err
	}

	newID, err := queries.CreateEmployee(ctx, employeeParams)
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to create employee")
	}

	metaParams.EmployeeID = newID
	if err := queries.CreateEmployeeMeta(ctx, metaParams); err != nil {
		return nil, gerrors.Wrap(err, "failed to create employee meta")
	}

	return g.GetByID(ctx, uint(newID))
}

func (g *GormEmployeeRepository) Update(ctx context.Context, data employee.Employee) error {
	tenantUUID, _, err := tenantIDs(ctx)
	if err != nil {
		return err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return err
	}

	employeeParams, metaParams, err := buildEmployeeUpdateParams(data, tenantUUID)
	if err != nil {
		return err
	}

	employeeParams.ID = int32(data.ID())
	if err := queries.UpdateEmployee(ctx, employeeParams); err != nil {
		return err
	}

	metaParams.EmployeeID = int32(data.ID())
	return queries.UpdateEmployeeMeta(ctx, metaParams)
}

func (g *GormEmployeeRepository) Delete(ctx context.Context, id uint) error {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return err
	}

	queries, err := g.employeeQueries(ctx)
	if err != nil {
		return err
	}

	if err := queries.DeleteEmployeeMeta(ctx, int32(id)); err != nil {
		return err
	}

	return queries.DeleteEmployee(ctx, employeesqlc.DeleteEmployeeParams{
		ID:       int32(id),
		TenantID: pgTenantID,
	})
}

func (g *GormEmployeeRepository) employeeQueries(ctx context.Context) (*employeesqlc.Queries, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	return employeesqlc.New(tx), nil
}

func tenantIDs(ctx context.Context) (uuid.UUID, pgtype.UUID, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return uuid.Nil, pgtype.UUID{}, fmt.Errorf("failed to get tenant from context: %w", err)
	}
	return tenantID, pgUUIDFromUUID(tenantID), nil
}
