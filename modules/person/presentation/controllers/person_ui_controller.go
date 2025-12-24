package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	corepermissions "github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	"github.com/iota-uz/iota-sdk/modules/person/presentation/mappers"
	persontemplates "github.com/iota-uz/iota-sdk/modules/person/presentation/templates/pages/persons"
	"github.com/iota-uz/iota-sdk/modules/person/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/person/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

var personPersonsAuthzObject = "person.persons"

type PersonUIController struct {
	app      application.Application
	persons  *services.PersonService
	basePath string
}

func NewPersonUIController(app application.Application) application.Controller {
	return &PersonUIController{
		app:      app,
		persons:  app.Service(services.PersonService{}).(*services.PersonService),
		basePath: "/person",
	}
}

func (c *PersonUIController) Key() string {
	return c.basePath
}

func (c *PersonUIController) Register(r *mux.Router) {
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

	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(commonMiddleware...)
	router.HandleFunc("", c.RedirectRoot).Methods(http.MethodGet)
	router.HandleFunc("/persons", c.List).Methods(http.MethodGet)
	router.HandleFunc("/persons/new", c.NewForm).Methods(http.MethodGet)
	router.HandleFunc("/persons/{person_uuid}", c.Detail).Methods(http.MethodGet)
	router.HandleFunc("/persons:by-pernr", c.RedirectByPernr).Methods(http.MethodGet)

	writeRouter := r.PathPrefix(c.basePath).Subrouter()
	writeRouter.Use(commonMiddleware...)
	writeRouter.Use(middleware.WithTransaction())
	writeRouter.HandleFunc("/persons", c.Create).Methods(http.MethodPost)
}

func (c *PersonUIController) RedirectRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/person/persons", http.StatusFound)
}

func (c *PersonUIController) List(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "list") {
		return
	}
	ensurePageCapabilities(r, personPersonsAuthzObject, "create")

	params := composables.UsePaginated(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	entities, _, err := c.persons.GetPaginated(r.Context(), &person.FindParams{
		Q:      q,
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]*viewmodels.PersonListItem, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mappers.PersonToListItem(entity))
	}

	pageCtx, _ := composables.TryUsePageCtx(r.Context())
	canCreate := pageCtx != nil && pageCtx.CanAuthz(personPersonsAuthzObject, "create")
	props := &viewmodels.PersonsListPageProps{
		Items:      items,
		Q:          q,
		NewURL:     "/person/persons/new",
		CanCreate:  canCreate,
		CanRequest: composables.CanUser(r.Context(), corepermissions.AuthzRequestsWrite) == nil,
		CanDebug:   composables.CanUser(r.Context(), corepermissions.AuthzDebug) == nil,
	}

	templ.Handler(persontemplates.Index(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *PersonUIController) NewForm(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "create") {
		return
	}

	props := &viewmodels.PersonCreatePageProps{
		Errors: map[string]string{},
		Form:   &viewmodels.PersonCreateFormVM{},
		PostTo: "/person/persons",
	}
	templ.Handler(persontemplates.New(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *PersonUIController) Create(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "create") {
		return
	}

	dto, err := composables.UseForm(&person.CreateDTO{}, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	errorsMap, ok := dto.Ok(r.Context())
	if !ok {
		props := &viewmodels.PersonCreatePageProps{
			Errors: errorsMap,
			Form: &viewmodels.PersonCreateFormVM{
				Pernr:       dto.Pernr,
				DisplayName: dto.DisplayName,
			},
			PostTo: "/person/persons",
		}
		templ.Handler(persontemplates.CreateForm(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	created, err := c.persons.Create(r.Context(), dto)
	if err != nil {
		if errors.Is(err, person.ErrPernrTaken) {
			pageCtx, _ := composables.TryUsePageCtx(r.Context())
			msg := "Pernr already exists"
			if pageCtx != nil {
				if localized := strings.TrimSpace(pageCtx.T("Person.Errors.PernrTaken")); localized != "" {
					msg = localized
				}
			}
			props := &viewmodels.PersonCreatePageProps{
				Errors: map[string]string{"Pernr": msg},
				Form: &viewmodels.PersonCreateFormVM{
					Pernr:       dto.Pernr,
					DisplayName: dto.DisplayName,
				},
				PostTo: "/person/persons",
			}
			templ.Handler(persontemplates.CreateForm(props), templ.WithStreaming()).ServeHTTP(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/person/persons/%s?step=assignment", created.PersonUUID().String()), http.StatusFound)
}

func (c *PersonUIController) Detail(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "view") {
		return
	}

	raw := mux.Vars(r)["person_uuid"]
	personUUID, err := uuid.Parse(raw)
	if err != nil {
		http.Error(w, "invalid person_uuid", http.StatusBadRequest)
		return
	}

	entity, err := c.persons.GetByUUID(r.Context(), personUUID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	props := &viewmodels.PersonDetailPageProps{
		Person:        mappers.PersonToListItem(entity),
		BackURL:       "/person/persons",
		CanRequest:    composables.CanUser(r.Context(), corepermissions.AuthzRequestsWrite) == nil,
		CanDebug:      composables.CanUser(r.Context(), corepermissions.AuthzDebug) == nil,
		EffectiveDate: time.Now().UTC().Format("2006-01-02"),
		Step:          strings.TrimSpace(r.URL.Query().Get("step")),
	}

	templ.Handler(persontemplates.Detail(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *PersonUIController) RedirectByPernr(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "view") {
		return
	}

	pernr := strings.TrimSpace(r.URL.Query().Get("pernr"))
	if pernr == "" {
		http.Error(w, "pernr is required", http.StatusBadRequest)
		return
	}

	entity, err := c.persons.GetByPernr(r.Context(), pernr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/person/persons/%s", entity.PersonUUID().String()), http.StatusFound)
}
