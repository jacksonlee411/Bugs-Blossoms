package controllers

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

const personAuthzDomain = "person"

func ensurePersonAuthz(
	w http.ResponseWriter,
	r *http.Request,
	object,
	action string,
	opts ...authz.RequestOption,
) bool {
	capKey := authzutil.CapabilityKey(object, action)

	tenantID := authzutil.TenantIDFromContext(r.Context())
	currentUser, err := composables.UseUser(r.Context())
	ctxWithState, state := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}
	if state != nil {
		state.Tenant = personAuthzDomain
	}

	if err != nil || currentUser == nil {
		recordForbiddenCapability(state, r, object, action, capKey)
		layouts.WriteAuthzForbiddenResponse(w, r, object, action)
		return false
	}

	svc := authz.Use()
	mode := svc.Mode()
	authzDomain := authz.DomainFromTenant(tenantID)
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authzDomain,
		object,
		authz.NormalizeAction(action),
		opts...,
	)

	allowed, authzErr := enforceRequest(r.Context(), svc, req, mode)
	if authzErr != nil {
		recordForbiddenCapability(state, r, object, action, capKey)
		layouts.WriteAuthzForbiddenResponse(w, r, object, action)
		return false
	}

	if allowed {
		if state != nil {
			state.SetCapability(capKey, true)
		}
		return true
	}

	recordForbiddenCapability(state, r, object, action, capKey)
	layouts.WriteAuthzForbiddenResponse(w, r, object, action)
	return false
}

func enforceRequest(ctx context.Context, svc *authz.Service, req authz.Request, mode authz.Mode) (bool, error) {
	if svc == nil {
		return true, nil
	}
	if err := svc.Authorize(ctx, req); err != nil {
		return false, err
	}

	switch mode {
	case authz.ModeDisabled, authz.ModeEnforce:
		return true, nil
	case authz.ModeShadow:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	default:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	}
}

func recordForbiddenCapability(state *authz.ViewState, r *http.Request, object, action, capKey string) {
	if state == nil {
		return
	}
	state.SetCapability(capKey, false)
	state.AddMissingPolicy(authz.MissingPolicy{
		Domain: authzutil.DomainFromContext(r.Context()),
		Object: object,
		Action: authz.NormalizeAction(action),
	})
}

func ensurePageCapabilities(r *http.Request, object string, actions ...string) {
	if len(actions) == 0 || strings.TrimSpace(object) == "" {
		return
	}

	state := authz.ViewStateFromContext(r.Context())
	if state == nil {
		return
	}

	currentUser, err := composables.UseUser(r.Context())
	if err != nil || currentUser == nil {
		return
	}

	tenantID := authzutil.TenantIDFromContext(r.Context())
	logger := composables.UseLogger(r.Context())

	for _, action := range actions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		if _, _, err := checkPersonCapability(r.Context(), state, tenantID, currentUser, object, action); err != nil {
			logger.WithError(err).WithField("capability", action).Warn("failed to evaluate capability")
		}
	}
}

func checkPersonCapability(
	ctx context.Context,
	state *authz.ViewState,
	tenantID uuid.UUID,
	u coreuser.User,
	object,
	action string,
) (bool, bool, error) {
	if u == nil || strings.TrimSpace(object) == "" {
		return false, false, nil
	}
	action = authz.NormalizeAction(action)
	capKey := authzutil.CapabilityKey(object, action)
	if allowed, ok := state.CapabilityValue(capKey); ok {
		return allowed, true, nil
	}

	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, u),
		authz.DomainFromTenant(tenantID),
		object,
		action,
	)
	allowed, err := authz.Use().Check(ctx, req)
	if err != nil {
		return false, true, err
	}
	state.SetCapability(capKey, allowed)
	if !allowed {
		state.AddMissingPolicy(authz.MissingPolicy{
			Domain: personAuthzDomain,
			Object: object,
			Action: action,
		})
	}
	return allowed, true, nil
}
