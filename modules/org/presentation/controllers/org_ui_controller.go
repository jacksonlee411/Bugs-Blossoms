package controllers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type OrgUIController struct {
	app      application.Application
	basePath string
}

func NewOrgUIController(app application.Application) application.Controller {
	return &OrgUIController{
		app:      app,
		basePath: "/org",
	}
}

func (c *OrgUIController) Key() string {
	return c.basePath
}

func (c *OrgUIController) Register(r *mux.Router) {
	common := []mux.MiddlewareFunc{
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	}

	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(common...)
	router.HandleFunc("", c.Index).Methods(http.MethodGet)
}

func (c *OrgUIController) Index(w http.ResponseWriter, r *http.Request) {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil || !services.OrgRolloutEnabledForTenant(tenantID) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<html><body><h1>Org</h1><ul><li><a href="/org/api/hierarchies?type=OrgUnit">GET /org/api/hierarchies</a></li></ul></body></html>`))
}
