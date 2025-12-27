package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	corepermissions "github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/logging/presentation/templates/pages/logs"
	"github.com/iota-uz/iota-sdk/modules/logging/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/mapping"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type LogsController struct {
	app         application.Application
	logsService *services.LogsService
	basePath    string
}

func NewLogsController(app application.Application) application.Controller {
	return &LogsController{
		app:         app,
		logsService: app.Service(services.LogsService{}).(*services.LogsService),
		basePath:    "/logs",
	}
}

func (c *LogsController) Key() string {
	return c.basePath
}

func (c *LogsController) Register(r *mux.Router) {
	commonMiddleware := []mux.MiddlewareFunc{
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	}

	getRouter := r.PathPrefix(c.basePath).Subrouter()
	getRouter.Use(commonMiddleware...)
	getRouter.HandleFunc("", c.List).Methods(http.MethodGet)
}

func (c *LogsController) List(w http.ResponseWriter, r *http.Request) {
	tenantID := authzutil.TenantIDFromContext(r.Context())
	currentUser, userErr := composables.UseUser(r.Context())
	ctxWithState, state := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}

	pagination := composables.UsePaginated(r)
	tab := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tab")))
	if tab != "action" {
		tab = "authentication"
	}

	authParams, authFilters := buildAuthenticationFilters(r, pagination.Limit, pagination.Offset)
	actionParams, actionFilters := buildActionFilters(r, pagination.Limit, pagination.Offset)

	var authLogs []*authenticationlog.AuthenticationLog
	var authTotal int64
	var actionLogs []*actionlog.ActionLog
	var actionTotal int64

	canView := false
	if userErr == nil && currentUser != nil {
		allowed, _, err := authzutil.CheckCapability(r.Context(), state, tenantID, currentUser, logsAuthzObject, "view")
		if err == nil {
			canView = allowed
		}
	} else if state != nil {
		recordForbiddenCapability(state, r, logsAuthzObject, "view", authzutil.CapabilityKey(logsAuthzObject, "view"))
	}

	if !canView {
		accept := strings.ToLower(r.Header.Get("Accept"))
		if strings.Contains(accept, "application/json") || htmx.IsHxRequest(r) {
			layouts.WriteAuthzForbiddenResponse(w, r, logsAuthzObject, "view")
			return
		}
	}

	var err error
	if canView {
		switch tab {
		case "authentication":
			authLogs, authTotal, err = c.logsService.ListAuthenticationLogs(r.Context(), authParams)
		case "action":
			actionLogs, actionTotal, err = c.logsService.ListActionLogs(r.Context(), actionParams)
		default:
			err = errors.New("unsupported tab")
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	canDebug := composables.CanUser(r.Context(), corepermissions.AuthzDebug) == nil
	canRequest := composables.CanUser(r.Context(), corepermissions.AuthzRequestsWrite) == nil
	props := &viewmodels.LogsPageProps{
		BasePath:  c.basePath,
		ActiveTab: tab,
		Authentication: viewmodels.AuthenticationSection{
			Logs:    mapping.MapViewModels(authLogs, mappers.AuthenticationLogToViewModel),
			Total:   authTotal,
			Filters: authFilters,
			Page:    pagination.Page,
			PerPage: pagination.Limit,
		},
		Action: viewmodels.ActionSection{
			Logs:    mapping.MapViewModels(actionLogs, mappers.ActionLogToViewModel),
			Total:   actionTotal,
			Filters: actionFilters,
			Page:    pagination.Page,
			PerPage: pagination.Limit,
		},
		CanView:    canView,
		CanRequest: canRequest,
		CanDebug:   canDebug,
	}

	if htmx.IsHxRequest(r) {
		templ.Handler(logs.TabContent(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	templ.Handler(logs.Index(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func buildAuthenticationFilters(
	r *http.Request,
	limit int,
	offset int,
) (*authenticationlog.FindParams, viewmodels.AuthenticationFilters) {
	q := r.URL.Query()
	filters := viewmodels.AuthenticationFilters{
		UserID:    strings.TrimSpace(q.Get("user_id")),
		IP:        strings.TrimSpace(q.Get("ip")),
		UserAgent: strings.TrimSpace(q.Get("user_agent")),
		From:      strings.TrimSpace(q.Get("from")),
		To:        strings.TrimSpace(q.Get("to")),
	}

	params := &authenticationlog.FindParams{
		IP:        filters.IP,
		UserAgent: filters.UserAgent,
		Limit:     limit,
		Offset:    offset,
	}

	if filters.UserID != "" {
		if parsed, err := strconv.ParseUint(filters.UserID, 10, 64); err == nil {
			userID := uint(parsed)
			params.UserID = &userID
		}
	}
	if filters.From != "" {
		if parsed, err := time.Parse(time.DateOnly, filters.From); err == nil {
			params.From = &parsed
		}
	}
	if filters.To != "" {
		if parsed, err := time.Parse(time.DateOnly, filters.To); err == nil {
			params.To = &parsed
		}
	}
	return params, filters
}

func buildActionFilters(
	r *http.Request,
	limit int,
	offset int,
) (*actionlog.FindParams, viewmodels.ActionFilters) {
	q := r.URL.Query()
	filters := viewmodels.ActionFilters{
		UserID:    strings.TrimSpace(q.Get("user_id")),
		Method:    strings.TrimSpace(q.Get("method")),
		Path:      strings.TrimSpace(q.Get("path")),
		IP:        strings.TrimSpace(q.Get("ip")),
		UserAgent: strings.TrimSpace(q.Get("user_agent")),
		From:      strings.TrimSpace(q.Get("from")),
		To:        strings.TrimSpace(q.Get("to")),
	}

	params := &actionlog.FindParams{
		Method:    filters.Method,
		Path:      filters.Path,
		IP:        filters.IP,
		UserAgent: filters.UserAgent,
		Limit:     limit,
		Offset:    offset,
	}

	if filters.UserID != "" {
		if parsed, err := strconv.ParseUint(filters.UserID, 10, 64); err == nil {
			userID := uint(parsed)
			params.UserID = &userID
		}
	}
	if filters.From != "" {
		if parsed, err := time.Parse(time.DateOnly, filters.From); err == nil {
			params.From = &parsed
		}
	}
	if filters.To != "" {
		if parsed, err := time.Parse(time.DateOnly, filters.To); err == nil {
			params.To = &parsed
		}
	}
	return params, filters
}
