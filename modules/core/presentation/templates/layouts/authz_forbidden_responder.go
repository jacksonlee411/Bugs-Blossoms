package layouts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	authzcomponents "github.com/iota-uz/iota-sdk/components/authorization"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	corepermissions "github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	htmxheaders "github.com/iota-uz/iota-sdk/pkg/htmx"
)

func WriteAuthzForbiddenResponse(w http.ResponseWriter, r *http.Request, object, action string) {
	state := authz.ViewStateFromContext(r.Context())
	payload := authzutil.BuildForbiddenPayload(r, state, object, action)

	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			composables.UseLogger(r.Context()).WithError(err).Warn("failed to encode forbidden response")
		}
		return
	}

	pageCtx, ok := composables.TryUsePageCtx(r.Context())
	if ok {
		if state == nil {
			state = pageCtx.AuthzState()
		}
		canDebug := composables.CanUser(r.Context(), corepermissions.AuthzDebug) == nil
		props := &authzcomponents.UnauthorizedProps{
			Object:        payload.Object,
			Action:        payload.Action,
			Operation:     fmt.Sprintf("%s %s", payload.Object, payload.Action),
			State:         state,
			Subject:       payload.Subject,
			Domain:        payload.Domain,
			DebugURL:      payload.DebugURL,
			BaseRevision:  payload.BaseRevision,
			RequestID:     payload.RequestID,
			ShowInspector: canDebug,
			CanDebug:      canDebug,
		}

		isHTMX := htmxheaders.IsHxRequest(r)
		if isHTMX {
			htmxheaders.Retarget(w, "body")
			htmxheaders.Reswap(w, "innerHTML")
		}

		w.WriteHeader(http.StatusForbidden)
		if isHTMX {
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
			if _, headErr := UseHead(ctx); headErr == nil {
				if _, sidebarErr := UseSidebarProps(ctx); sidebarErr == nil {
					layout := Authenticated(AuthenticatedProps{
						BaseProps: BaseProps{
							Title: title,
						},
					})
					return layout.Render(templ.WithChildren(ctx, content), w)
				}
			}
		}

		if _, headErr := UseHead(ctx); headErr == nil {
			base := Base(&BaseProps{
				Title:        title,
				WebsocketURL: "/ws",
			})
			return base.Render(templ.WithChildren(ctx, content), w)
		}

		return content.Render(ctx, w)
	})
}
