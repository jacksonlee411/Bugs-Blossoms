package handlers

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/session"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type SessionEventsHandler struct {
	app     application.Application
	service *services.LogsService
	logger  *logrus.Logger
}

func RegisterSessionEventHandlers(app application.Application) {
	handler := &SessionEventsHandler{
		app:     app,
		service: app.Service(services.LogsService{}).(*services.LogsService),
		logger:  configuration.Use().Logger(),
	}
	app.EventPublisher().Subscribe(handler.onSessionCreated)
}

func (h *SessionEventsHandler) onSessionCreated(event session.CreatedEvent) {
	if h.service == nil || h.app == nil {
		return
	}

	ctx := composables.WithPool(context.Background(), h.app.DB())
	ctx = composables.WithTenantID(ctx, event.Result.TenantID)

	logEntry := &authenticationlog.AuthenticationLog{
		UserID:    event.Result.UserID,
		IP:        event.Result.IP,
		UserAgent: event.Result.UserAgent,
		CreatedAt: event.Result.CreatedAt,
		TenantID:  event.Result.TenantID,
	}

	if err := h.service.CreateAuthenticationLog(ctx, logEntry); err != nil {
		h.logger.WithError(err).
			WithField("user_id", event.Result.UserID).
			Warn("failed to persist authentication log")
	}
}
