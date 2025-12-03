package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	permissionEntity "github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/pkg/application"
	authz "github.com/iota-uz/iota-sdk/pkg/authz"
	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/di"
	"github.com/iota-uz/iota-sdk/pkg/middleware"

	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/services"
)

// AuthzAPIController exposes REST APIs for policy drafts and policy listings.
type AuthzAPIController struct {
	app      application.Application
	basePath string
}

// NewAuthzAPIController wires the controller into the router.
func NewAuthzAPIController(app application.Application) application.Controller {
	return &AuthzAPIController{
		app:      app,
		basePath: "/core/api/authz",
	}
}

// Key implements application.Controller.
func (c *AuthzAPIController) Key() string {
	return c.basePath
}

// Register registers routes.
func (c *AuthzAPIController) Register(r *mux.Router) {
	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(
		middleware.Authorize(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideLocalizer(c.app),
	)

	router.HandleFunc("/policies", di.H(c.listPolicies)).Methods(http.MethodGet)
	router.HandleFunc("/requests", di.H(c.listRequests)).Methods(http.MethodGet)
	router.HandleFunc("/requests", di.H(c.createRequest)).Methods(http.MethodPost)
	router.HandleFunc("/requests/{id}", di.H(c.getRequest)).Methods(http.MethodGet)
	router.HandleFunc("/requests/{id}/approve", di.H(c.approveRequest)).Methods(http.MethodPost)
	router.HandleFunc("/requests/{id}/reject", di.H(c.rejectRequest)).Methods(http.MethodPost)
	router.HandleFunc("/requests/{id}/cancel", di.H(c.cancelRequest)).Methods(http.MethodPost)
	router.HandleFunc("/requests/{id}/trigger-bot", di.H(c.triggerBot)).Methods(http.MethodPost)
	router.HandleFunc("/requests/{id}/revert", di.H(c.revertRequest)).Methods(http.MethodPost)
	router.HandleFunc("/debug", di.H(c.debugRequest)).Methods(http.MethodGet)
}

func (c *AuthzAPIController) listPolicies(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
) {
	if !c.ensurePermission(w, r, permissions.AuthzDebug) {
		return
	}
	entries, err := svc.Policies(r.Context())
	if err != nil {
		logger.WithError(err).Error("authz api: list policies failed")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_POLICIES_ERROR", "failed to read policies")
		return
	}
	data := make([]dtos.PolicyEntryResponse, 0, len(entries))
	for _, entry := range entries {
		data = append(data, dtos.PolicyEntryResponse{
			Type:    entry.Type,
			Subject: entry.Subject,
			Domain:  entry.Domain,
			Object:  entry.Object,
			Action:  entry.Action,
			Effect:  entry.Effect,
		})
	}
	writeJSON(w, http.StatusOK, data)
}

func (c *AuthzAPIController) listRequests(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsRead) {
		return
	}
	params, err := parseListParams(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", err.Error())
		return
	}
	tenantID := tenantIDFromContext(r)
	drafts, total, err := svc.List(r.Context(), tenantID, params)
	if err != nil {
		logger.WithError(err).Error("authz api: list requests failed")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_LIST_ERROR", "failed to list requests")
		return
	}
	resp := dtos.PolicyDraftListResponse{
		Data:  make([]dtos.PolicyDraftResponse, 0, len(drafts)),
		Total: total,
	}
	for _, draft := range drafts {
		resp.Data = append(resp.Data, dtos.NewPolicyDraftResponse(draft))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *AuthzAPIController) getRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsRead) {
		return
	}
	id, err := parseUUID(mux.Vars(r)["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	draft, err := svc.Get(r.Context(), tenantID, id)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) createRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
	currentUser user.User,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsWrite) {
		return
	}
	var payload dtos.PolicyDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_BODY", "unable to parse request body")
		return
	}
	tenantID := tenantIDFromContext(r)
	requesterID := normalizedUserUUID(tenantID, currentUser)
	params := services.CreatePolicyDraftParams{
		RequesterID:  requesterID,
		Object:       payload.Object,
		Action:       payload.Action,
		Reason:       payload.Reason,
		Diff:         payload.Diff,
		BaseRevision: payload.BaseRevision,
		Domain:       payload.Domain,
	}
	draft, err := svc.Create(r.Context(), tenantID, params)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) approveRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
	currentUser user.User,
) {
	c.reviewRequest(w, r, logger, svc, currentUser, svc.Approve)
}

func (c *AuthzAPIController) rejectRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
	currentUser user.User,
) {
	c.reviewRequest(w, r, logger, svc, currentUser, svc.Reject)
}

func (c *AuthzAPIController) cancelRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsWrite) {
		return
	}
	id, err := parseUUID(mux.Vars(r)["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	draft, err := svc.Cancel(r.Context(), tenantID, id)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) triggerBot(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsReview) {
		return
	}
	id, err := parseUUID(mux.Vars(r)["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	locker := r.URL.Query().Get("locker")
	tenantID := tenantIDFromContext(r)
	draft, err := svc.TriggerBot(r.Context(), tenantID, id, locker)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) revertRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
	currentUser user.User,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsReview) {
		return
	}
	id, err := parseUUID(mux.Vars(r)["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	requesterID := normalizedUserUUID(tenantID, currentUser)
	draft, err := svc.Revert(r.Context(), tenantID, id, requesterID)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) debugRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
) {
	if !c.ensurePermission(w, r, permissions.AuthzDebug) {
		return
	}
	query := r.URL.Query()
	subject := query.Get("subject")
	if subject == "" {
		tenantID := tenantIDFromContext(r)
		user, err := composables.UseUser(r.Context())
		if err == nil && user != nil {
			subject = authzSubjectForUser(tenantID, user)
		}
	}
	domain := query.Get("domain")
	if domain == "" {
		domain = authzDomainFromContext(r)
	}

	object := query.Get("object")
	action := query.Get("action")
	if object == "" || action == "" {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_DEBUG_INVALID", "object and action are required")
		return
	}

	req := authz.NewRequest(subject, domain, object, action)
	svc := authz.Use()
	allowed, err := svc.Check(r.Context(), req)
	if err != nil {
		logger.WithError(err).Error("authz api: debug check failed")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_DEBUG_ERROR", "failed to evaluate request")
		return
	}
	writeJSON(w, http.StatusOK, dtos.DebugResponse{
		Allowed: allowed,
		Mode:    string(svc.Mode()),
		Request: dtos.DebugRequestDTO{
			Subject: subject,
			Domain:  domain,
			Object:  object,
			Action:  action,
		},
	})
}

func (c *AuthzAPIController) reviewRequest(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.PolicyDraftService,
	currentUser user.User,
	reviewFunc func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (services.PolicyDraft, error),
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsReview) {
		return
	}
	id, err := parseUUID(mux.Vars(r)["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	approverID := normalizedUserUUID(tenantID, currentUser)
	draft, err := reviewFunc(r.Context(), tenantID, id, approverID)
	if err != nil {
		c.respondServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dtos.NewPolicyDraftResponse(draft))
}

func (c *AuthzAPIController) ensurePermission(
	w http.ResponseWriter,
	r *http.Request,
	perm *permissionEntity.Permission,
) bool {
	if err := composables.CanUser(r.Context(), perm); err != nil {
		writeJSONError(w, http.StatusForbidden, "AUTHZ_FORBIDDEN", "permission denied")
		return false
	}
	return true
}

func (c *AuthzAPIController) respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrPolicyDraftNotFound):
		writeJSONError(w, http.StatusNotFound, "AUTHZ_NOT_FOUND", "request not found")
	case errors.Is(err, services.ErrInvalidDiff),
		errors.Is(err, services.ErrRevisionMismatch):
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_REQUEST", err.Error())
	case errors.Is(err, services.ErrInvalidStatusTransition):
		writeJSONError(w, http.StatusConflict, "AUTHZ_INVALID_STATE", err.Error())
	case errors.Is(err, services.ErrMissingSnapshot):
		writeJSONError(w, http.StatusConflict, "AUTHZ_NO_SNAPSHOT", err.Error())
	case errors.Is(err, services.ErrTenantMismatch):
		writeJSONError(w, http.StatusForbidden, "AUTHZ_FORBIDDEN", "tenant mismatch")
	default:
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_ERROR", "internal error")
	}
}

func parseListParams(r *http.Request) (services.ListPolicyDraftsParams, error) {
	query := r.URL.Query()
	params := services.ListPolicyDraftsParams{}

	if limit := query.Get("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			return params, errors.New("limit must be numeric")
		}
		params.Limit = value
	}
	if offset := query.Get("offset"); offset != "" {
		value, err := strconv.Atoi(offset)
		if err != nil {
			return params, errors.New("offset must be numeric")
		}
		params.Offset = value
	}
	if sort := strings.ToLower(query.Get("sort")); sort == "asc" {
		params.SortAsc = true
	}
	for _, status := range query["status"] {
		params.Statuses = append(params.Statuses, authzPersistence.PolicyChangeStatus(status))
	}
	return params, nil
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(raw))
}

func tenantIDFromContext(r *http.Request) uuid.UUID {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		return uuid.Nil
	}
	return tenantID
}

func authzDomainFromContext(r *http.Request) string {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		return "global"
	}
	return authz.DomainFromTenant(tenantID)
}

func authzSubjectForUser(tenantID uuid.UUID, u user.User) string {
	return authz.SubjectForUser(tenantID, normalizedUserUUID(tenantID, u))
}

var userNamespace = uuid.MustParse("7f1d14be-672e-49c7-91ad-e50eb1d35815")

func normalizedUserUUID(tenantID uuid.UUID, u user.User) uuid.UUID {
	payload := fmt.Sprintf("%s:%d", tenantID.String(), u.ID())
	return uuid.NewSHA1(userNamespace, []byte(payload))
}
