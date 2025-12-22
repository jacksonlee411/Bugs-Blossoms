package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	goi18n "github.com/iota-uz/go-i18n/v2/i18n"

	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	"github.com/iota-uz/iota-sdk/modules/person/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/person/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type PersonAPIController struct {
	app      application.Application
	persons  *services.PersonService
	basePath string
}

func NewPersonAPIController(app application.Application) application.Controller {
	return &PersonAPIController{
		app:      app,
		persons:  app.Service(services.PersonService{}).(*services.PersonService),
		basePath: "/person/api",
	}
}

func (c *PersonAPIController) Key() string {
	return c.basePath
}

func (c *PersonAPIController) Register(r *mux.Router) {
	commonMiddleware := []mux.MiddlewareFunc{
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideLocalizer(c.app),
	}

	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(commonMiddleware...)
	router.HandleFunc("/persons:options", c.Options).Methods(http.MethodGet)

	writeRouter := r.PathPrefix(c.basePath).Subrouter()
	writeRouter.Use(commonMiddleware...)
	writeRouter.Use(middleware.WithTransaction())
	writeRouter.HandleFunc("/persons", c.Create).Methods(http.MethodPost)
}

func (c *PersonAPIController) Options(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "list") {
		return
	}

	limit := 20
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items, _, err := c.persons.GetPaginated(r.Context(), &person.FindParams{Q: q, Limit: limit, Offset: 0})
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "PERSON_INTERNAL", "internal error")
		return
	}

	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		vm := mappers.PersonToListItem(p)
		out = append(out, map[string]any{
			"person_uuid":  vm.PersonUUID,
			"pernr":        vm.Pernr,
			"display_name": vm.DisplayName,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": out,
	})
}

func (c *PersonAPIController) Create(w http.ResponseWriter, r *http.Request) {
	if !ensurePersonAuthz(w, r, personPersonsAuthzObject, "create") {
		return
	}

	var dto person.CreateDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "PERSON_INVALID_JSON", "invalid json")
		return
	}

	if errs, ok := dto.Ok(r.Context()); !ok {
		message := "validation failed"
		if v := strings.TrimSpace(errs["Pernr"]); v != "" {
			message = v
		} else if v := strings.TrimSpace(errs["DisplayName"]); v != "" {
			message = v
		}
		writeAPIError(w, r, http.StatusUnprocessableEntity, "PERSON_VALIDATION_FAILED", message)
		return
	}

	created, err := c.persons.Create(r.Context(), &dto)
	if err != nil {
		if errors.Is(err, person.ErrPernrTaken) {
			message := "pernr already exists"
			if l, ok := intl.UseLocalizer(r.Context()); ok {
				localized := l.MustLocalize(&goi18n.LocalizeConfig{MessageID: "Person.Errors.PernrTaken"})
				if strings.TrimSpace(localized) != "" {
					message = localized
				}
			}
			writeAPIError(w, r, http.StatusConflict, "PERSON_PERNR_CONFLICT", message)
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "PERSON_INTERNAL", "internal error")
		return
	}

	vm := mappers.PersonToListItem(created)
	writeJSON(w, http.StatusCreated, map[string]any{
		"person_uuid":  vm.PersonUUID,
		"pernr":        vm.Pernr,
		"display_name": vm.DisplayName,
		"status":       vm.Status,
		"created_at":   created.CreatedAt(),
		"updated_at":   created.UpdatedAt(),
	})
}
