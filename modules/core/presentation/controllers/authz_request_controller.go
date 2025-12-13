package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/authz"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

// AuthzRequestController handles the display of policy change requests.
// It implements the P0 requirement for a minimal read-only detail page.
type AuthzRequestController struct {
	app      application.Application
	basePath string
}

func NewAuthzRequestController(app application.Application) application.Controller {
	return &AuthzRequestController{
		app:      app,
		basePath: "/core/authz",
	}
}

func (c *AuthzRequestController) Key() string {
	return c.basePath
}

func (c *AuthzRequestController) Register(r *mux.Router) {
	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	)
	router.HandleFunc("/requests/{id}", c.Get).Methods(http.MethodGet)
}

func (c *AuthzRequestController) Get(w http.ResponseWriter, r *http.Request) {
	if err := composables.CanUser(r.Context(), permissions.AuthzRequestsRead); err != nil {
		writeForbiddenResponse(w, r, "core.authz", "requests")
		return
	}

	rawID := mux.Vars(r)["id"]
	id, err := uuid.Parse(strings.TrimSpace(rawID))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	svc := c.app.Service(services.PolicyDraftService{}).(*services.PolicyDraftService)
	tenantID := tenantIDFromContext(r)
	draft, err := svc.Get(r.Context(), tenantID, id)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPolicyDraftNotFound):
			http.NotFound(w, r)
		case errors.Is(err, services.ErrTenantMismatch):
			writeForbiddenResponse(w, r, "core.authz", "requests")
		default:
			composables.UseLogger(r.Context()).WithError(err).Error("authz ui: failed to load request")
			http.Error(w, "failed to load request", http.StatusInternalServerError)
		}
		return
	}

	viewModel := buildAuthzRequestDetail(draft)

	templ.Handler(authz.RequestDetail(viewModel), templ.WithStreaming()).ServeHTTP(w, r)
}

func buildAuthzRequestDetail(draft services.PolicyDraft) *viewmodels.AuthzRequestDetail {
	var pretty bytes.Buffer
	diffJSON := ""
	if len(draft.Diff) > 0 {
		if err := json.Indent(&pretty, draft.Diff, "", "  "); err == nil {
			diffJSON = pretty.String()
		} else {
			diffJSON = string(draft.Diff)
		}
	}

	diffItems := make([]viewmodels.PolicyDiffItem, 0)
	if len(draft.Diff) > 0 {
		var rawOps []map[string]any
		if err := json.Unmarshal(draft.Diff, &rawOps); err == nil {
			for _, op := range rawOps {
				opName, _ := op["op"].(string)
				path, _ := op["path"].(string)
				var text string
				if val, ok := op["value"]; ok {
					switch v := val.(type) {
					case string:
						text = v
					default:
						if b, err := json.Marshal(v); err == nil {
							text = string(b)
						}
					}
				}
				diffItems = append(diffItems, viewmodels.PolicyDiffItem{
					Op:   opName,
					Path: path,
					Text: text,
				})
			}
		}
	}

	var reviewed string
	if draft.ReviewedAt != nil {
		reviewed = draft.ReviewedAt.Format(time.RFC3339)
	}

	status := string(draft.Status)

	var botLog string
	if draft.ErrorLog != nil {
		botLog = strings.TrimSpace(*draft.ErrorLog)
	}

	var prLink string
	if draft.PRLink != nil {
		prLink = strings.TrimSpace(*draft.PRLink)
	}

	return &viewmodels.AuthzRequestDetail{
		ID:          draft.ID.String(),
		Status:      status,
		StatusClass: statusBadgeClass(status),
		Requester:   draft.Subject,
		CreatedAt:   draft.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   draft.UpdatedAt.Format(time.RFC3339),
		ReviewedAt:  reviewed,
		Object:      draft.Object,
		Action:      draft.Action,
		Domain:      draft.Domain,
		Reason:      draft.Reason,
		DiffJSON:    diffJSON,
		Diff:        diffItems,
		BotLog:      botLog,
		PRLink:      prLink,
	}
}

func statusBadgeClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending_review":
		return "bg-yellow-100 text-yellow-800 border border-yellow-200"
	case "approved", "merged":
		return "bg-green-100 text-green-800 border border-green-200"
	case "failed", "rejected", "canceled":
		return "bg-red-100 text-red-800 border border-red-200"
	default:
		return "bg-gray-100 text-gray-800 border border-gray-200"
	}
}
