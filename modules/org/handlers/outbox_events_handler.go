package handlers

import (
	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/outbox"
)

type OutboxEventsHandler struct {
	org *services.OrgService
}

func RegisterOutboxEventHandlers(app application.Application) {
	handler := &OutboxEventsHandler{
		org: app.Service(services.OrgService{}).(*services.OrgService),
	}
	app.EventPublisher().Subscribe(handler.onOrgEventV1)
}

func (h *OutboxEventsHandler) onOrgEventV1(meta *outbox.Meta, ev *events.OrgEventV1) error {
	if h == nil || h.org == nil || meta == nil || ev == nil {
		return nil
	}
	h.org.InvalidateTenantCacheWithReason(meta.TenantID, "outbox_event")
	return nil
}
