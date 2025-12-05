package authzutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

var userNamespace = uuid.MustParse("7f1d14be-672e-49c7-91ad-e50eb1d35815")

// NormalizedUserUUID deterministically maps an internal numeric ID into a UUID.
func NormalizedUserUUID(tenantID uuid.UUID, u user.User) uuid.UUID {
	payload := fmt.Sprintf("%s:%d", tenantID.String(), u.ID())
	return uuid.NewSHA1(userNamespace, []byte(payload))
}

// SubjectForUser returns the canonical subject identifier for Casbin checks.
func SubjectForUser(tenantID uuid.UUID, u user.User) string {
	return authz.SubjectForUser(tenantID, NormalizedUserUUID(tenantID, u))
}

// NewViewState constructs a ViewState from tenant + user pair.
func NewViewState(tenantID uuid.UUID, u user.User) *authz.ViewState {
	return authz.NewViewState(SubjectForUser(tenantID, u), authz.DomainFromTenant(tenantID))
}

// CapabilityKey normalizes object/action into a canonical capability identifier.
func CapabilityKey(object, action string) string {
	return fmt.Sprintf("%s.%s", strings.ToLower(object), authz.NormalizeAction(action))
}

// EnsureViewState guarantees that a view state exists in the provided context.
func EnsureViewState(ctx context.Context, tenantID uuid.UUID, u user.User) (context.Context, *authz.ViewState) {
	if u == nil {
		state := authz.ViewStateFromContext(ctx)
		syncPageContext(ctx, state)
		return ctx, state
	}
	if state := authz.ViewStateFromContext(ctx); state != nil {
		syncPageContext(ctx, state)
		return ctx, state
	}
	viewState := NewViewState(tenantID, u)
	ctxWithState := authz.WithViewState(ctx, viewState)
	syncPageContext(ctxWithState, viewState)
	return ctxWithState, viewState
}

// CheckCapability evaluates the authz capability for the supplied object/action.
// It returns (allowed, decided, error) where decided indicates whether authz rules were applied.
func CheckCapability(
	ctx context.Context,
	state *authz.ViewState,
	tenantID uuid.UUID,
	u user.User,
	object,
	action string,
) (bool, bool, error) {
	if u == nil || strings.TrimSpace(object) == "" {
		return false, false, nil
	}
	action = authz.NormalizeAction(action)
	capKey := CapabilityKey(object, action)
	if allowed, ok := state.CapabilityValue(capKey); ok {
		return allowed, true, nil
	}

	req := authz.NewRequest(
		SubjectForUser(tenantID, u),
		authz.DomainFromTenant(tenantID),
		object,
		action,
	)
	allowed, err := authz.Use().Check(ctx, req)
	if err != nil {
		return false, true, err
	}
	if state != nil {
		state.SetCapability(capKey, allowed)
		if !allowed {
			state.AddMissingPolicy(authz.MissingPolicy{
				Domain: authz.DomainFromTenant(tenantID),
				Object: object,
				Action: action,
			})
		}
	}
	return allowed, true, nil
}

func syncPageContext(ctx context.Context, state *authz.ViewState) {
	if state == nil {
		return
	}
	pageCtx, ok := composables.TryUsePageCtx(ctx)
	if !ok {
		return
	}
	pageCtx.SetAuthzState(state)
}
