package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/iota-uz/go-i18n/v2/i18n"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/types"
)

func TestEnsureAuthz_ForbiddenJSONContract(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureAuthz(rr, req, authz.ObjectName("core", "users"), "list", nil)

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "core.users", payload.Object)
	require.Equal(t, "list", payload.Action)
	require.Equal(t, "/core/api/authz/requests", payload.RequestURL)
	require.NotEmpty(t, payload.DebugURL)
	require.Equal(t, "global", payload.Domain)
	require.NotEmpty(t, payload.Subject)
	require.NotEmpty(t, payload.MissingPolicies)

	state := authz.ViewStateFromContext(req.Context())
	require.NotNil(t, state)
	require.Equal(t, payload.Subject, state.Subject)
	require.Equal(t, payload.Domain, state.Tenant)
}

func TestEnsureAuthz_ForbiddenHTMXContract(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Hx-Request", "true")

	rr := httptest.NewRecorder()
	allowed := ensureAuthz(rr, req, authz.ObjectName("core", "users"), "list", nil)

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Equal(t, "body", rr.Header().Get("Hx-Retarget"))
	require.Equal(t, "innerHTML", rr.Header().Get("Hx-Reswap"))
	require.NotEmpty(t, rr.Body.String())
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

func TestEnsureAuthz_ForbiddenHTMLFallbackRendersUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req = req.WithContext(composables.WithPageCtx(req.Context(), &stubPageCtx{}))

	rr := httptest.NewRecorder()
	allowed := ensureAuthz(rr, req, authz.ObjectName("core", "users"), "list", nil)

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "Authz.Unauthorized.Title")
}
