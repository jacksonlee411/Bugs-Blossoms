package mappers

import (
	"fmt"
	"time"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/presentation/viewmodels"
)

func AuthenticationLogToViewModel(log *authenticationlog.AuthenticationLog) *viewmodels.AuthenticationLog {
	if log == nil {
		return nil
	}

	return &viewmodels.AuthenticationLog{
		ID:        log.ID,
		UserID:    log.UserID,
		IP:        log.IP,
		UserAgent: log.UserAgent,
		CreatedAt: log.CreatedAt.Format(time.DateTime),
	}
}

func ActionLogToViewModel(log *actionlog.ActionLog) *viewmodels.ActionLog {
	if log == nil {
		return nil
	}

	userID := ""
	if log.UserID != nil {
		userID = fmt.Sprintf("%d", *log.UserID)
	}

	return &viewmodels.ActionLog{
		ID:        log.ID,
		UserID:    userID,
		Method:    log.Method,
		Path:      log.Path,
		IP:        log.IP,
		UserAgent: log.UserAgent,
		CreatedAt: log.CreatedAt.Format(time.DateTime),
	}
}
