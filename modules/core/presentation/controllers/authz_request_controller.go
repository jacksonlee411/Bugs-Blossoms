package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/authz"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	authzpkg "github.com/iota-uz/iota-sdk/pkg/authz"
	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
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
	router.HandleFunc("/requests", c.List).Methods(http.MethodGet)
	router.HandleFunc("/requests/{id}", c.Get).Methods(http.MethodGet)
}

func (c *AuthzRequestController) List(w http.ResponseWriter, r *http.Request) {
	if err := composables.CanUser(r.Context(), permissions.AuthzRequestsRead); err != nil {
		layouts.WriteAuthzForbiddenResponse(w, r, "core.authz", "read")
		return
	}

	params := composables.UsePaginated(r)
	query := r.URL.Query()

	subject := strings.TrimSpace(query.Get("subject"))
	domain := strings.TrimSpace(query.Get("domain"))
	statuses := query["status"]

	mine := query.Get("mine") == "1" || strings.EqualFold(query.Get("mine"), "true")
	sortAsc := strings.EqualFold(strings.TrimSpace(query.Get("sort")), "asc")
	canReview := composables.CanUser(r.Context(), permissions.AuthzRequestsReview) == nil

	listParams := services.ListPolicyDraftsParams{
		Subject:  subject,
		Domain:   domain,
		Limit:    params.Limit,
		Offset:   params.Offset,
		SortAsc:  sortAsc,
		Statuses: make([]authzPersistence.PolicyChangeStatus, 0, len(statuses)),
	}
	for _, status := range statuses {
		if val := strings.TrimSpace(status); val != "" {
			listParams.Statuses = append(listParams.Statuses, authzPersistence.PolicyChangeStatus(val))
		}
	}

	tenantID := tenantIDFromContext(r)
	if mine {
		currentUser, err := composables.UseUser(r.Context())
		if err == nil && currentUser != nil {
			requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)
			listParams.RequesterID = &requesterID
		}
	}

	svc := c.app.Service(services.PolicyDraftService{}).(*services.PolicyDraftService)
	drafts, total, err := svc.List(r.Context(), tenantID, listParams)
	if err != nil {
		composables.UseLogger(r.Context()).WithError(err).Error("authz ui: failed to list requests")
		http.Error(w, "failed to list requests", http.StatusInternalServerError)
		return
	}

	items := make([]viewmodels.AuthzRequestListItem, 0, len(drafts))
	for _, draft := range drafts {
		items = append(items, buildAuthzRequestListItem(draft))
	}

	viewModel := &viewmodels.AuthzRequestList{
		Items:     items,
		Total:     total,
		Page:      params.Page,
		Limit:     params.Limit,
		Mine:      mine,
		Statuses:  statuses,
		Subject:   subject,
		Domain:    domain,
		CanReview: canReview,
	}

	templ.Handler(authz.Requests(viewModel), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *AuthzRequestController) Get(w http.ResponseWriter, r *http.Request) {
	if err := composables.CanUser(r.Context(), permissions.AuthzRequestsRead); err != nil {
		layouts.WriteAuthzForbiddenResponse(w, r, "core.authz", "read")
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
			layouts.WriteAuthzForbiddenResponse(w, r, "core.authz", "read")
		default:
			composables.UseLogger(r.Context()).WithError(err).Error("authz ui: failed to load request")
			http.Error(w, "failed to load request", http.StatusInternalServerError)
		}
		return
	}

	viewModel := buildAuthzRequestDetail(r.Context(), draft)

	templ.Handler(authz.RequestDetail(viewModel), templ.WithStreaming()).ServeHTTP(w, r)
}

func buildAuthzRequestListItem(draft services.PolicyDraft) viewmodels.AuthzRequestListItem {
	status := string(draft.Status)

	return viewmodels.AuthzRequestListItem{
		ID:          draft.ID.String(),
		Status:      status,
		StatusClass: statusBadgeClass(status),
		Object:      strings.TrimSpace(draft.Object),
		Action:      strings.TrimSpace(draft.Action),
		Domain:      strings.TrimSpace(draft.Domain),
		Reason:      strings.TrimSpace(draft.Reason),
		CreatedAt:   draft.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   draft.UpdatedAt.Format(time.RFC3339),
		ViewURL:     fmt.Sprintf("/core/authz/requests/%s", draft.ID),
	}
}

func buildAuthzRequestDetail(ctx context.Context, draft services.PolicyDraft) *viewmodels.AuthzRequestDetail {
	var pretty bytes.Buffer
	diffJSON := ""
	if len(draft.Diff) > 0 {
		if err := json.Indent(&pretty, draft.Diff, "", "  "); err == nil {
			diffJSON = pretty.String()
		} else {
			diffJSON = string(draft.Diff)
		}
	}

	diffKind := ""
	diffItems := make([]viewmodels.PolicyDiffItem, 0)
	suggestions := make([]viewmodels.AuthzPolicySuggestionItem, 0)
	if len(draft.Diff) > 0 {
		var rawOps []map[string]any
		if err := json.Unmarshal(draft.Diff, &rawOps); err == nil && len(rawOps) > 0 {
			if _, ok := rawOps[0]["op"]; ok {
				diffKind = "patch"
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
			} else if _, ok := rawOps[0]["subject"]; ok {
				diffKind = "suggestions"
				var parsed []authzpkg.PolicySuggestion
				if err := json.Unmarshal(draft.Diff, &parsed); err == nil {
					for _, item := range parsed {
						suggestions = append(suggestions, viewmodels.AuthzPolicySuggestionItem{
							Subject: item.Subject,
							Domain:  item.Domain,
							Object:  item.Object,
							Action:  item.Action,
							Effect:  item.Effect,
						})
					}
				}
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

	canReview := composables.CanUser(ctx, permissions.AuthzRequestsReview) == nil
	canCancel := composables.CanUser(ctx, permissions.AuthzRequestsWrite) == nil
	canRevert := canReview && len(draft.AppliedPolicySnapshot) > 0
	canRetryBot := canReview || canCancel
	retryToken := ""
	if canRetryBot && draft.Status == authzPersistence.PolicyChangeStatusFailed {
		if token, err := authzutil.GenerateRetryToken(draft.ID, time.Minute); err == nil {
			retryToken = token
		}
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
		DiffKind:    diffKind,
		Diff:        diffItems,
		Suggestions: suggestions,
		BotLog:      botLog,
		PRLink:      prLink,
		CanReview:   canReview,
		CanCancel:   canCancel,
		CanRevert:   canRevert,
		CanRetryBot: canRetryBot,
		RetryToken:  retryToken,
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
