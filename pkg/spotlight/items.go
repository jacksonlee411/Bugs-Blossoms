package spotlight

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	spotlightui "github.com/iota-uz/iota-sdk/components/spotlight"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

// Item represents a renderable spotlight entry.
type Item interface {
	templ.Component
}

// NewItem creates a simple Item with a static label and link.
func NewItem(icon templ.Component, label, link string) Item {
	return &item{label: label, icon: icon, link: link}
}

type item struct {
	label string
	icon  templ.Component
	link  string
}

func (i *item) Render(ctx context.Context, w io.Writer) error {
	return spotlightui.LinkItem(i.label, i.link, i.icon).Render(ctx, w)
}

func NewQuickLink(icon templ.Component, trKey, link string) *QuickLink {
	return &QuickLink{trKey: trKey, icon: icon, link: link}
}

type QuickLink struct {
	trKey       string
	icon        templ.Component
	link        string
	permissions []*permission.Permission
	authzObject string
	authzAction string
}

func (i *QuickLink) Render(ctx context.Context, w io.Writer) error {
	label := intl.MustT(ctx, i.trKey)
	return spotlightui.LinkItem(label, i.link, i.icon).Render(ctx, w)
}

// RequirePermissions configures legacy permissions used as a fallback when authz metadata is absent.
func (i *QuickLink) RequirePermissions(perms ...*permission.Permission) *QuickLink {
	i.permissions = append(i.permissions, perms...)
	return i
}

// RequireAuthz sets the authz object/action that governs the quick link visibility.
func (i *QuickLink) RequireAuthz(object, action string) *QuickLink {
	i.authzObject = object
	i.authzAction = action
	return i
}

type QuickLinks struct {
	items []*QuickLink
}

func (ql *QuickLinks) Find(ctx context.Context, q string) []Item {
	links := ql.authorizedLinks(ctx)
	if len(links) == 0 {
		return nil
	}
	words := make([]string, len(links))
	for i, it := range links {
		words[i] = intl.MustT(ctx, it.trKey)
	}
	ranks := fuzzy.RankFindNormalizedFold(q, words)
	sort.Sort(ranks)

	result := make([]Item, 0, len(ranks))
	for _, rank := range ranks {
		result = append(result, links[rank.OriginalIndex])
	}
	return result
}

func (ql *QuickLinks) Add(links ...*QuickLink) {
	ql.items = append(ql.items, links...)
}

func (ql *QuickLinks) authorizedLinks(ctx context.Context) []*QuickLink {
	u, err := composables.UseUser(ctx)
	if err != nil || u == nil {
		return nil
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		tenantID = uuid.Nil
	}

	_, state := authzutil.EnsureViewState(ctx, tenantID, u)
	if state == nil {
		state = authz.ViewStateFromContext(ctx)
	}

	filtered := make([]*QuickLink, 0, len(ql.items))
	for _, link := range ql.items {
		if link.allowed(ctx, u, tenantID, state) {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func (i *QuickLink) allowed(ctx context.Context, u user.User, tenantID uuid.UUID, state *authz.ViewState) bool {
	if allowed, decided := i.evaluateCapability(ctx, u, tenantID, state); decided {
		return allowed
	}
	if len(i.permissions) == 0 {
		return true
	}
	if u == nil {
		return false
	}
	for _, perm := range i.permissions {
		if !u.Can(perm) {
			return false
		}
	}
	return true
}

func (i *QuickLink) evaluateCapability(ctx context.Context, u user.User, tenantID uuid.UUID, state *authz.ViewState) (bool, bool) {
	if strings.TrimSpace(i.authzObject) == "" {
		return false, false
	}
	action := i.authzAction
	if action == "" {
		action = "list"
	}
	allowed, decided, err := authzutil.CheckCapability(ctx, state, tenantID, u, i.authzObject, action)
	if err != nil {
		return false, false
	}
	return allowed, decided
}
