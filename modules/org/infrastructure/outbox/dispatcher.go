package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/outbox"
)

type Dispatcher struct {
	bus eventbus.EventBusWithError
}

func NewDispatcher(bus eventbus.EventBusWithError) *Dispatcher {
	return &Dispatcher{bus: bus}
}

func (d *Dispatcher) Dispatch(ctx context.Context, msg outbox.DispatchedMessage) error {
	_ = ctx
	if d == nil || d.bus == nil {
		return fmt.Errorf("org outbox dispatcher: bus is nil")
	}

	switch msg.Meta.Topic {
	case events.TopicOrgChangedV1, events.TopicOrgAssignmentChangedV1:
	default:
		return fmt.Errorf("org outbox dispatcher: unsupported topic %q", msg.Meta.Topic)
	}

	var ev events.OrgEventV1
	if err := json.Unmarshal(msg.Payload, &ev); err != nil {
		return fmt.Errorf("org outbox dispatcher: decode payload: %w", err)
	}

	return d.bus.PublishE(&msg.Meta, &ev)
}
