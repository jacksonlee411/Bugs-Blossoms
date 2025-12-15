package eventbus

import (
	"context"

	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/outbox"
)

type Dispatcher struct {
	bus eventbus.EventBusWithError
}

func New(bus eventbus.EventBusWithError) *Dispatcher {
	return &Dispatcher{
		bus: bus,
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, msg outbox.DispatchedMessage) error {
	_ = ctx
	// PublishE will surface panic/handler error and allow relay retries.
	// Subscriber signature options (examples):
	// - func(meta *outbox.Meta, topic string, payload json.RawMessage) error
	// - func(meta *outbox.Meta, topic string, payload json.RawMessage)
	return d.bus.PublishE(&msg.Meta, msg.Meta.Topic, msg.Payload)
}
