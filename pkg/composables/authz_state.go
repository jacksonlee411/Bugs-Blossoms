package composables

import (
	"context"

	"github.com/iota-uz/iota-sdk/pkg/authz"
)

// UseAuthzViewState returns the authz.ViewState stored in the context, when available.
func UseAuthzViewState(ctx context.Context) *authz.ViewState {
	return authz.ViewStateFromContext(ctx)
}

// WithAuthzViewState attaches an authz.ViewState to the context.
func WithAuthzViewState(ctx context.Context, state *authz.ViewState) context.Context {
	return authz.WithViewState(ctx, state)
}
