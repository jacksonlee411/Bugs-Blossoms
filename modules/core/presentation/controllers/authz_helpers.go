package controllers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
)

func ensureAuthz(
	w http.ResponseWriter,
	r *http.Request,
	object,
	action string,
	legacyPerm *permission.Permission,
	opts ...authz.RequestOption,
) bool {
	capKey := authzutil.CapabilityKey(object, action)

	currentUser, err := composables.UseUser(r.Context())
	if err != nil || currentUser == nil {
		recordForbiddenCapability(authz.ViewStateFromContext(r.Context()), r, object, action, capKey)
		writeForbiddenResponse(w, r, object, action)
		return false
	}

	tenantID := tenantIDFromContext(r)
	ctxWithState, state := authzutil.EnsureViewState(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}

	svc := authz.Use()
	mode := svc.Mode()
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authz.DomainFromTenant(tenantID),
		object,
		authz.NormalizeAction(action),
		opts...,
	)

	allowed, authzErr := enforceRequest(r.Context(), svc, req, mode)
	if authzErr != nil {
		recordForbiddenCapability(state, r, object, action, capKey)
		writeForbiddenResponse(w, r, object, action)
		return false
	}

	if allowed {
		if state != nil {
			state.SetCapability(capKey, true)
		}
		return true
	}

	if mode == authz.ModeShadow && legacyPerm != nil && currentUser.Can(legacyPerm) {
		if state != nil {
			state.SetCapability(capKey, true)
		}
		return true
	}

	recordForbiddenCapability(state, r, object, action, capKey)
	writeForbiddenResponse(w, r, object, action)
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

func writeForbiddenResponse(w http.ResponseWriter, r *http.Request, object, action string) {
	msg := fmt.Sprintf("Forbidden: %s %s. 如需申请权限，请访问 /core/api/authz/requests。", object, action)
	if htmx.IsHxRequest(r) {
		w.Header().Set("HX-Retarget", "body")
		w.Header().Set("HX-Reswap", "innerHTML")
	}
	http.Error(w, msg, http.StatusForbidden)
}

func tenantIDFromContext(r *http.Request) uuid.UUID {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		return uuid.Nil
	}
	return tenantID
}

func authzDomainFromContext(r *http.Request) string {
	tenantID := tenantIDFromContext(r)
	return authz.DomainFromTenant(tenantID)
}

func authzSubjectForUser(tenantID uuid.UUID, u user.User) string {
	return authzutil.SubjectForUser(tenantID, u)
}

func parseDebugAttributes(values map[string][]string) authz.Attributes {
	attrs := authz.Attributes{}
	const prefix = "attr."
	for key, vals := range values {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		attrKey := strings.TrimSpace(strings.TrimPrefix(key, prefix))
		if attrKey == "" || len(vals) == 0 {
			continue
		}
		attrs[attrKey] = vals[len(vals)-1]
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func recordForbiddenCapability(state *authz.ViewState, r *http.Request, object, action, capKey string) {
	if state == nil {
		return
	}
	state.SetCapability(capKey, false)
	state.AddMissingPolicy(authz.MissingPolicy{
		Domain: authzDomainFromContext(r),
		Object: object,
		Action: authz.NormalizeAction(action),
	})
}
