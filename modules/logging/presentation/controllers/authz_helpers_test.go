package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/iota-uz/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/constants"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/types"
)

func TestEnsureLoggingAuthz_AllowsWhenModeDisabled(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	tenantID := uuid.New()
	u := user.New(
		"Test",
		"User",
		internet.MustParseEmail("allow@example.com"),
		user.UILanguageEN,
		user.WithTenantID(tenantID),
		user.WithID(1),
	)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, u)
	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.True(t, allowed)
	require.Equal(t, 200, rr.Code)
}

func TestEnsureLoggingAuthz_ForbidsWithoutUser(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, uuid.New())
	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, 403, rr.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "forbidden", payload["error"])
	require.Equal(t, "logging.logs", payload["object"])
	require.Equal(t, "view", payload["action"])
	require.Equal(t, "/core/api/authz/requests", payload["request_url"])
	require.Contains(t, payload, "missing_policies")
	require.Contains(t, payload, "debug_url")
}

func TestEnsureLoggingAuthz_DeniedPopulatesViewState(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantlessUser := user.New(
		"Denied",
		"User",
		internet.MustParseEmail("denied@example.com"),
		user.UILanguageEN,
		user.WithID(12),
	)

	ctx := contextWithLogger(t)
	ctx = composables.WithUser(ctx, tenantlessUser)

	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "logging.logs", payload["object"])
	require.Equal(t, "view", payload["action"])
	require.Equal(t, loggingAuthzDomain, payload["domain"])
	require.NotEmpty(t, payload["subject"])

	missing, ok := payload["missing_policies"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, missing)
}

func TestEnsureLoggingAuthz_ActionLogEnabledWithNilDBDoesNotBlockResponse(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	cfgSingleton := configuration.Use()
	origActionLogEnabled := cfgSingleton.ActionLogEnabled
	cfgSingleton.ActionLogEnabled = true
	t.Cleanup(func() { cfgSingleton.ActionLogEnabled = origActionLogEnabled })

	tenantID := uuid.New()
	currentUser := user.New(
		"Denied",
		"User",
		internet.MustParseEmail("nologs@example.com"),
		user.UILanguageEN,
		user.WithID(33),
		user.WithTenantID(tenantID),
	)

	app := application.New(&application.ApplicationOptions{
		EventBus: eventbus.NewEventPublisher(nil),
		Bundle:   application.LoadBundle(),
	})
	app.RegisterServices(
		services.NewLogsService(&noopAuthLogRepo{}, &noopActionLogRepo{}),
	)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, currentUser)
	ctx = context.WithValue(ctx, constants.AppKey, app)

	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "logging.logs", payload["object"])
	require.Equal(t, "view", payload["action"])
}

type stubPageCtx struct {
	prefix string
	state  *authz.ViewState
}

func (p *stubPageCtx) T(key string, _ ...map[string]interface{}) string {
	if p.prefix != "" {
		return p.prefix + "." + key
	}
	return key
}

func (p *stubPageCtx) TSafe(key string, args ...map[string]interface{}) string {
	return p.T(key, args...)
}

func (p *stubPageCtx) Namespace(prefix string) types.PageContextProvider {
	next := *p
	if next.prefix != "" {
		next.prefix = next.prefix + "." + prefix
	} else {
		next.prefix = prefix
	}
	return &next
}

func (p *stubPageCtx) ToJSLocale() string { return "en-US" }

func (p *stubPageCtx) GetLocale() language.Tag { return language.English }

func (p *stubPageCtx) GetURL() *url.URL { return &url.URL{} }

func (p *stubPageCtx) GetLocalizer() *i18n.Localizer { return nil }

func (p *stubPageCtx) AuthzState() *authz.ViewState { return p.state }

func (p *stubPageCtx) SetAuthzState(state *authz.ViewState) { p.state = state }

func (p *stubPageCtx) CanAuthz(string, string) bool { return false }

func TestEnsureLoggingAuthz_ForbiddenHTMXRendersUnauthorized(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, uuid.New())
	ctx = composables.WithPageCtx(ctx, &stubPageCtx{})

	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)
	req.Header.Set("Hx-Request", "true")

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Equal(t, "body", rr.Header().Get("Hx-Retarget"))
	require.Equal(t, "innerHTML", rr.Header().Get("Hx-Reswap"))
	require.Contains(t, rr.Body.String(), "data-authz-container")
}

func TestEnsureLoggingAuthz_ForbiddenHTMLFallbackRendersUnauthorized(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, uuid.New())
	ctx = composables.WithPageCtx(ctx, &stubPageCtx{})

	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "data-authz-container")
	require.Contains(t, rr.Body.String(), "data-request-url=\"/core/api/authz/requests\"")
}

func TestEnsureLoggingAuthz_ForbiddenHTMLFallbackWithoutPageCtxReturnsPlainText(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/logs", nil).WithContext(ctx)

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "Forbidden:")
}

func setAuthzEnv(t *testing.T) {
	t.Helper()
	root := filepath.Clean("../../../../")
	t.Setenv("AUTHZ_MODEL_PATH", filepath.Join(root, "config/access/model.conf"))
	t.Setenv("AUTHZ_POLICY_PATH", filepath.Join(root, "config/access/policy.csv"))
}

func contextWithLogger(t *testing.T) context.Context {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	return context.WithValue(context.Background(), constants.LoggerKey, logrus.NewEntry(logger))
}

type noopAuthLogRepo struct{}

func (noopAuthLogRepo) List(ctx context.Context, params *authenticationlog.FindParams) ([]*authenticationlog.AuthenticationLog, error) {
	return nil, nil
}

func (noopAuthLogRepo) Count(ctx context.Context, params *authenticationlog.FindParams) (int64, error) {
	return 0, nil
}

func (noopAuthLogRepo) Create(ctx context.Context, log *authenticationlog.AuthenticationLog) error {
	return nil
}

type noopActionLogRepo struct{}

func (noopActionLogRepo) List(ctx context.Context, params *actionlog.FindParams) ([]*actionlog.ActionLog, error) {
	return nil, nil
}

func (noopActionLogRepo) Count(ctx context.Context, params *actionlog.FindParams) (int64, error) {
	return 0, nil
}

func (noopActionLogRepo) Create(ctx context.Context, log *actionlog.ActionLog) error {
	return nil
}
