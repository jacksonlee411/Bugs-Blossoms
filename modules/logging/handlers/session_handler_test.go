package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/session"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
)

type stubLogsService struct {
	created []*authenticationlog.AuthenticationLog
}

func (s *stubLogsService) CreateAuthenticationLog(ctx context.Context, log *authenticationlog.AuthenticationLog) error {
	s.created = append(s.created, log)
	return nil
}

func TestSessionEventsHandler_PublishesToLoggingRepo(t *testing.T) {
	app := application.New(&application.ApplicationOptions{
		EventBus: eventbus.NewEventPublisher(nil),
	})

	stubSvc := &stubLogsService{}
	handler := NewSessionEventsHandler(app, stubSvc)
	app.EventPublisher().Subscribe(handler.onSessionCreated)

	tenantID := uuid.New()
	event := session.CreatedEvent{
		Result: session.Session{
			UserID:    99,
			IP:        "10.0.0.1",
			UserAgent: "agent",
			CreatedAt: time.Now(),
			TenantID:  tenantID,
		},
	}

	app.EventPublisher().Publish(event)

	require.Len(t, stubSvc.created, 1)
	created := stubSvc.created[0]
	require.Equal(t, tenantID, created.TenantID)
	require.Equal(t, uint(99), created.UserID)
	require.Equal(t, "10.0.0.1", created.IP)
	require.Equal(t, "agent", created.UserAgent)
}
