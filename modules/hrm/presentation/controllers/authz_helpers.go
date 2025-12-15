package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	authzcomponents "github.com/iota-uz/iota-sdk/components/authorization"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	corepermissions "github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/modules/hrm/permissions"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/routing"
)

const hrmAuthzDomain = "hrm"

func ensureHRMAuthz(
	w http.ResponseWriter,
	r *http.Request,
	object,
	action string,
	legacyPerm *permission.Permission,
	opts ...authz.RequestOption,
) bool {
	capKey := authzutil.CapabilityKey(object, action)

	tenantID := tenantIDFromContext(r)
	currentUser, err := composables.UseUser(r.Context())
	ctxWithState, state := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}
	if state != nil {
		state.Tenant = hrmAuthzDomain
	}

	if err != nil || currentUser == nil {
		recordForbiddenCapability(state, r, object, action, capKey)
		writeForbiddenResponse(w, r, object, action)
		return false
	}

	svc := authz.Use()
	mode := svc.Mode()
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		hrmAuthzDomain,
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
	state := authz.ViewStateFromContext(r.Context())
	payload := authzutil.BuildForbiddenPayload(r, state, object, action)
	if routing.IsJSONOnlyNamespacePath(r.URL.Path) ||
		strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			composables.UseLogger(r.Context()).WithError(err).Warn("failed to encode forbidden response")
		}
		return
	}

	if pageCtx, ok := composables.TryUsePageCtx(r.Context()); ok {
		if state == nil {
			state = pageCtx.AuthzState()
		}
		canDebug := composables.CanUser(r.Context(), corepermissions.AuthzDebug) == nil
		props := &authzcomponents.UnauthorizedProps{
			Object:        payload.Object,
			Action:        payload.Action,
			Operation:     fmt.Sprintf("%s %s", payload.Object, payload.Action),
			State:         state,
			RequestURL:    payload.RequestURL,
			Subject:       payload.Subject,
			Domain:        payload.Domain,
			DebugURL:      payload.DebugURL,
			BaseRevision:  payload.BaseRevision,
			RequestID:     payload.RequestID,
			ShowInspector: canDebug,
			CanDebug:      canDebug,
		}
		w.WriteHeader(http.StatusForbidden)
		if htmx.IsHxRequest(r) {
			w.Header().Set("Hx-Retarget", "body")
			w.Header().Set("Hx-Reswap", "innerHTML")
			templ.Handler(authzcomponents.Unauthorized(props), templ.WithStreaming()).ServeHTTP(w, r)
			return
		}
		templ.Handler(unauthorizedPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	http.Error(w, payload.Message, http.StatusForbidden)
}

func unauthorizedPage(props *authzcomponents.UnauthorizedProps) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageCtx := composables.UsePageCtx(ctx)
		title := strings.TrimSpace(pageCtx.T("Authz.Unauthorized.Title"))
		if title == "" {
			title = "Unauthorized"
		}

		content := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			if _, err := io.WriteString(w, `<main class="mx-auto w-full max-w-5xl p-6">`); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `<h1 class="sr-only">`+templ.EscapeString(title)+`</h1>`); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `<div class="flex justify-center">`); err != nil {
				return err
			}
			if err := authzcomponents.Unauthorized(props).Render(ctx, w); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `</div></main>`); err != nil {
				return err
			}
			return nil
		})

		currentUser, err := composables.UseUser(ctx)
		if err == nil && currentUser != nil {
			if _, headErr := layouts.UseHead(ctx); headErr == nil {
				if _, sidebarErr := layouts.UseSidebarProps(ctx); sidebarErr == nil {
					layout := layouts.Authenticated(layouts.AuthenticatedProps{
						BaseProps: layouts.BaseProps{
							Title: title,
						},
					})
					return layout.Render(templ.WithChildren(ctx, content), w)
				}
			}
		}

		if _, headErr := layouts.UseHead(ctx); headErr == nil {
			base := layouts.Base(&layouts.BaseProps{
				Title:        title,
				WebsocketURL: "/ws",
			})
			return base.Render(templ.WithChildren(ctx, content), w)
		}

		return content.Render(ctx, w)
	})
}

func tenantIDFromContext(r *http.Request) uuid.UUID {
	return authzutil.TenantIDFromContext(r.Context())
}

func authzDomainFromContext(r *http.Request) string {
	return authzutil.DomainFromContext(r.Context())
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

	tenantID := tenantIDFromContext(r)
	logger := composables.UseLogger(r.Context())

	for _, action := range actions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		if _, _, err := authzutil.CheckCapability(r.Context(), state, tenantID, currentUser, object, action); err != nil {
			logger.WithError(err).WithField("capability", action).Warn("failed to evaluate capability")
		}
	}
}

func legacyEmployeePermission(action string) *permission.Permission {
	switch action {
	case "list", "view":
		return permissions.EmployeeRead
	case "create":
		return permissions.EmployeeCreate
	case "update":
		return permissions.EmployeeUpdate
	case "delete":
		return permissions.EmployeeDelete
	default:
		return nil
	}
}
