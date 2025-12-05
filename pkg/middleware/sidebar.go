package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/components/sidebar"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/constants"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	pkgsidebar "github.com/iota-uz/iota-sdk/pkg/sidebar"
	"github.com/iota-uz/iota-sdk/pkg/types"
)

func filterItems(ctx context.Context, items []types.NavigationItem, user user.User, tenantID uuid.UUID, state *authz.ViewState) []types.NavigationItem {
	filteredItems := make([]types.NavigationItem, 0, len(items))
	for _, item := range items {
		if hasNavigationAccess(ctx, item, user, tenantID, state) {
			filteredItems = append(filteredItems, types.NavigationItem{
				Name:        item.Name,
				Href:        item.Href,
				Children:    filterItems(ctx, item.Children, user, tenantID, state),
				Icon:        item.Icon,
				Permissions: item.Permissions,
				AuthzObject: item.AuthzObject,
				AuthzAction: item.AuthzAction,
			})
		}
	}
	return filteredItems
}

func getEnabledNavItems(items []types.NavigationItem) []types.NavigationItem {
	var out []types.NavigationItem
	for _, item := range items {
		if len(item.Children) > 0 {
			children := getEnabledNavItems(item.Children)
			childrenLen := len(children)
			if childrenLen == 0 {
				continue
			}
			if childrenLen == 1 {
				out = append(out, children[0])
			} else {
				item.Children = children
				out = append(out, item)
			}
		} else {
			out = append(out, item)
		}
	}

	return out
}

func hasNavigationAccess(ctx context.Context, item types.NavigationItem, user user.User, tenantID uuid.UUID, state *authz.ViewState) bool {
	if user == nil {
		return false
	}
	if allowed, decided := navCapability(ctx, item, user, tenantID, state); decided {
		return allowed
	}
	return item.HasPermission(user)
}

func navCapability(ctx context.Context, item types.NavigationItem, user user.User, tenantID uuid.UUID, state *authz.ViewState) (bool, bool) {
	if strings.TrimSpace(item.AuthzObject) == "" {
		return false, false
	}
	action := item.AuthzAction
	if action == "" {
		action = "list"
	}
	allowed, decided, err := authzutil.CheckCapability(ctx, state, tenantID, user, item.AuthzObject, action)
	if err != nil {
		return false, false
	}
	return allowed, decided
}

func NavItems() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				app, err := application.UseApp(r.Context())
				if err != nil {
					panic(err.Error())
				}
				localizer, ok := intl.UseLocalizer(r.Context())
				if !ok {
					panic("localizer not found in context")
				}
				u, err := composables.UseUser(r.Context())
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}

				tenantID, err := composables.UseTenantID(r.Context())
				if err != nil {
					tenantID = uuid.Nil
				}

				var state *authz.ViewState
				ctxWithState := r.Context()
				if u != nil {
					ctxWithState, state = authzutil.EnsureViewState(ctxWithState, tenantID, u)
					r = r.WithContext(ctxWithState)
				} else {
					state = authz.ViewStateFromContext(ctxWithState)
				}

				filtered := filterItems(r.Context(), app.NavItems(localizer), u, tenantID, state)
				enabledNavItems := getEnabledNavItems(filtered)

				// Build sidebar props with configurable tab groups
				tabGroups := pkgsidebar.BuildTabGroups(enabledNavItems, localizer)

				sidebarProps := sidebar.Props{
					Header:       layouts.DefaultSidebarHeader(),
					TabGroups:    tabGroups,
					Footer:       layouts.DefaultSidebarFooter(),
					InitialState: sidebar.SidebarAuto, // Default: respect localStorage
				}

				ctx := context.WithValue(r.Context(), constants.AllNavItemsKey, filtered)
				ctx = context.WithValue(ctx, constants.NavItemsKey, enabledNavItems)
				ctx = context.WithValue(ctx, constants.SidebarPropsKey, sidebarProps)
				next.ServeHTTP(w, r.WithContext(ctx))
			},
		)
	}
}
