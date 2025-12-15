package outbox

import "context"

type Dispatcher interface {
	Dispatch(ctx context.Context, msg DispatchedMessage) error
}
