package outbox

import (
	"fmt"

	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

var (
	ErrInvalidConfig = serrors.NewError("OUTBOX_INVALID_CONFIG", "invalid outbox configuration", "")
)

func invalidConfig(msg string, args ...any) error {
	return fmt.Errorf("%w: "+msg, append([]any{ErrInvalidConfig}, args...)...)
}
