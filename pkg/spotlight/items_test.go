package spotlight

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/go-i18n/v2/i18n"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"golang.org/x/text/language"
)

func TestQuickLinks_AuthorizeByCapability(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	u := user.New(
		"Link",
		"Tester",
		internet.MustParseEmail("link@test.com"),
		user.UILanguageEN,
		user.WithTenantID(tenantID),
		user.WithID(1),
	)

	state := authz.NewViewState(authz.SubjectForUserID(tenantID, "user-1"), authz.DomainFromTenant(tenantID))
	state.SetCapability("logging.logs.view", true)

	ctx := withLocalizer(t, context.Background())
	ctx = authz.WithViewState(ctx, state)
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, u)

	links := QuickLinks{}
	links.Add(NewQuickLink(nil, "NavigationLinks.Logs", "/logs").RequireAuthz("logging.logs", "view"))

	found := links.Find(ctx, "logs")
	require.Len(t, found, 1)
}

func TestQuickLinks_DeniedWhenCapabilityMissing(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	u := user.New(
		"NoAccess",
		"User",
		internet.MustParseEmail("deny@test.com"),
		user.UILanguageEN,
		user.WithTenantID(tenantID),
		user.WithID(2),
	)

	state := authz.NewViewState(authz.SubjectForUserID(tenantID, "user-2"), authz.DomainFromTenant(tenantID))
	state.SetCapability("logging.logs.view", false)

	ctx := withLocalizer(t, context.Background())
	ctx = authz.WithViewState(ctx, state)
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, u)

	links := QuickLinks{}
	links.Add(NewQuickLink(nil, "NavigationLinks.Logs", "/logs").RequireAuthz("logging.logs", "view"))

	found := links.Find(ctx, "logs")
	require.Empty(t, found)
}

func withLocalizer(t *testing.T, ctx context.Context) context.Context {
	bundle := i18n.NewBundle(language.English)
	err := bundle.AddMessages(language.English, &i18n.Message{
		ID:    "NavigationLinks.Logs",
		Other: "Logs",
	})
	require.NoError(t, err)
	localizer := i18n.NewLocalizer(bundle, "en")
	ctx = intl.WithLocale(ctx, language.English)
	ctx = intl.WithLocalizer(ctx, localizer)
	return ctx
}
