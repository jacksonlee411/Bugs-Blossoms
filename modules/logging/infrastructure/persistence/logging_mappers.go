package persistence

import (
	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/infrastructure/persistence/models"
)

func toDBAuthenticationLog(log *authenticationlog.AuthenticationLog) *models.AuthenticationLog {
	return &models.AuthenticationLog{
		ID:        log.ID,
		TenantID:  log.TenantID.String(),
		UserID:    log.UserID,
		IP:        log.IP,
		UserAgent: log.UserAgent,
		CreatedAt: log.CreatedAt,
	}
}

func toDomainAuthenticationLog(dbLog *models.AuthenticationLog) *authenticationlog.AuthenticationLog {
	tenantID, err := uuid.Parse(dbLog.TenantID)
	if err != nil {
		tenantID = uuid.Nil
	}

	return &authenticationlog.AuthenticationLog{
		ID:        dbLog.ID,
		TenantID:  tenantID,
		UserID:    dbLog.UserID,
		IP:        dbLog.IP,
		UserAgent: dbLog.UserAgent,
		CreatedAt: dbLog.CreatedAt,
	}
}

func toDBActionLog(log *actionlog.ActionLog) *models.ActionLog {
	return &models.ActionLog{
		ID:        log.ID,
		TenantID:  log.TenantID.String(),
		UserID:    log.UserID,
		Method:    log.Method,
		Path:      log.Path,
		Before:    log.Before,
		After:     log.After,
		UserAgent: log.UserAgent,
		IP:        log.IP,
		CreatedAt: log.CreatedAt,
	}
}

func toDomainActionLog(dbLog *models.ActionLog) *actionlog.ActionLog {
	tenantID, err := uuid.Parse(dbLog.TenantID)
	if err != nil {
		tenantID = uuid.Nil
	}

	return &actionlog.ActionLog{
		ID:        dbLog.ID,
		TenantID:  tenantID,
		UserID:    dbLog.UserID,
		Method:    dbLog.Method,
		Path:      dbLog.Path,
		Before:    dbLog.Before,
		After:     dbLog.After,
		UserAgent: dbLog.UserAgent,
		IP:        dbLog.IP,
		CreatedAt: dbLog.CreatedAt,
	}
}
