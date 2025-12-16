package services

import (
	"context"

	"github.com/iota-uz/iota-sdk/modules/hrm/domain/aggregates/employee"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
)

type EmployeeService struct {
	repo      employee.Repository
	publisher eventbus.EventBus
}

func NewEmployeeService(repo employee.Repository, publisher eventbus.EventBus) *EmployeeService {
	return &EmployeeService{
		repo:      repo,
		publisher: publisher,
	}
}

func (s *EmployeeService) Count(ctx context.Context) (int64, error) {
	return composables.InTenantTxResult(ctx, func(txCtx context.Context) (int64, error) {
		return s.repo.Count(txCtx)
	})
}

func (s *EmployeeService) GetAll(ctx context.Context) ([]employee.Employee, error) {
	return composables.InTenantTxResult(ctx, func(txCtx context.Context) ([]employee.Employee, error) {
		return s.repo.GetAll(txCtx)
	})
}

func (s *EmployeeService) GetByID(ctx context.Context, id uint) (employee.Employee, error) {
	return composables.InTenantTxResult(ctx, func(txCtx context.Context) (employee.Employee, error) {
		return s.repo.GetByID(txCtx, id)
	})
}

func (s *EmployeeService) GetPaginated(ctx context.Context, params *employee.FindParams) ([]employee.Employee, error) {
	return composables.InTenantTxResult(ctx, func(txCtx context.Context) ([]employee.Employee, error) {
		return s.repo.GetPaginated(txCtx, params)
	})
}

func (s *EmployeeService) Create(ctx context.Context, data *employee.CreateDTO) error {
	if err := authorizeHRM(ctx, EmployeesAuthzObject, "create"); err != nil {
		return err
	}
	return composables.InTenantTx(ctx, func(txCtx context.Context) error {
		entity, err := data.ToEntity()
		if err != nil {
			return err
		}
		createdEntity, err := s.repo.Create(txCtx, entity)
		if err != nil {
			return err
		}
		ev, err := employee.NewCreatedEvent(txCtx, *data, createdEntity)
		if err != nil {
			return err
		}
		s.publisher.Publish(ev)
		return nil
	})
}

func (s *EmployeeService) Update(ctx context.Context, id uint, data *employee.UpdateDTO) error {
	if err := authorizeHRM(ctx, EmployeesAuthzObject, "update"); err != nil {
		return err
	}
	return composables.InTenantTx(ctx, func(txCtx context.Context) error {
		entity, err := data.ToEntity(id)
		if err != nil {
			return err
		}
		if err := s.repo.Update(txCtx, entity); err != nil {
			return err
		}
		ev, err := employee.NewUpdatedEvent(txCtx, *data, entity)
		if err != nil {
			return err
		}
		s.publisher.Publish(ev)
		return nil
	})
}

func (s *EmployeeService) Delete(ctx context.Context, id uint) (employee.Employee, error) {
	if err := authorizeHRM(ctx, EmployeesAuthzObject, "delete"); err != nil {
		return nil, err
	}
	return composables.InTenantTxResult(ctx, func(txCtx context.Context) (employee.Employee, error) {
		if err := s.repo.Delete(txCtx, id); err != nil {
			return nil, err
		}
		entity, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return nil, err
		}
		ev, err := employee.NewDeletedEvent(txCtx, entity)
		if err != nil {
			return nil, err
		}
		s.publisher.Publish(ev)
		return entity, nil
	})
}
