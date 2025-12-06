package services

import (
	"context"
	"errors"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
)

type LogsService struct {
	authRepo   authenticationlog.Repository
	actionRepo actionlog.Repository
}

func NewLogsService(
	authRepo authenticationlog.Repository,
	actionRepo actionlog.Repository,
) *LogsService {
	return &LogsService{
		authRepo:   authRepo,
		actionRepo: actionRepo,
	}
}

func (s *LogsService) ListAuthenticationLogs(
	ctx context.Context,
	params *authenticationlog.FindParams,
) ([]*authenticationlog.AuthenticationLog, int64, error) {
	if err := authorizeLogging(ctx, "view"); err != nil {
		return nil, 0, err
	}
	if params == nil {
		params = &authenticationlog.FindParams{}
	}

	logs, err := s.authRepo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.authRepo.Count(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

func (s *LogsService) ListActionLogs(
	ctx context.Context,
	params *actionlog.FindParams,
) ([]*actionlog.ActionLog, int64, error) {
	if err := authorizeLogging(ctx, "view"); err != nil {
		return nil, 0, err
	}
	if params == nil {
		params = &actionlog.FindParams{}
	}

	logs, err := s.actionRepo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.actionRepo.Count(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	return logs, count, nil
}

func (s *LogsService) CreateAuthenticationLog(ctx context.Context, log *authenticationlog.AuthenticationLog) error {
	if log == nil {
		return errors.New("authentication log payload is required")
	}
	return s.authRepo.Create(ctx, log)
}

func (s *LogsService) CreateActionLog(ctx context.Context, log *actionlog.ActionLog) error {
	if log == nil {
		return errors.New("action log payload is required")
	}
	return s.actionRepo.Create(ctx, log)
}
