package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
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
	app        application.Application
	basePath   string
	stageStore *policyStageStore
}

// NewAuthzAPIController wires the controller into the router.
func NewAuthzAPIController(app application.Application) application.Controller {
	return &AuthzAPIController{
		app:        app,
		basePath:   "/core/api/authz",
		stageStore: newPolicyStageStore(),
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
	router.HandleFunc("/policies/stage", di.H(c.stagePolicy)).Methods(http.MethodPost, http.MethodDelete)
	router.Handle(
		"/debug",
		middleware.RateLimit(middleware.RateLimitConfig{
			RequestsPerPeriod: 20,
			Period:            time.Minute,
			KeyFunc:           middleware.EndpointKeyFunc("core.api.authz.debug"),
		})(di.H(c.debugRequest)),
	).Methods(http.MethodGet)
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
	params, err := parsePolicyListParams(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", err.Error())
		return
	}
	entries, err := svc.Policies(r.Context())
	if err != nil {
		logger.WithError(err).Error("authz api: list policies failed")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_POLICIES_ERROR", "failed to read policies")
		return
	}
	filtered := filterPolicies(entries, params)
	sorted := sortPolicies(filtered, params.SortField, params.SortAsc)
	start := params.Offset()
	end := start + params.Limit
	if start > len(sorted) {
		start = len(sorted)
	}
	if end > len(sorted) {
		end = len(sorted)
	}
	pageData := make([]dtos.PolicyEntryResponse, 0, end-start)
	for _, entry := range sorted[start:end] {
		pageData = append(pageData, dtos.PolicyEntryResponse{
			Type:    entry.Type,
			Subject: entry.Subject,
			Domain:  entry.Domain,
			Object:  entry.Object,
			Action:  entry.Action,
			Effect:  entry.Effect,
		})
	}
	writeJSON(w, http.StatusOK, dtos.PolicyListResponse{
		Data:  pageData,
		Total: len(sorted),
		Page:  params.Page,
		Limit: params.Limit,
	})
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
	requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)
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

func (c *AuthzAPIController) stagePolicy(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
) {
	if !c.ensurePermission(w, r, permissions.AuthzRequestsWrite) {
		return
	}
	currentUser, err := composables.UseUser(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "AUTHZ_NO_USER", "user not found in context")
		return
	}
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_NO_TENANT", "tenant not found in context")
		return
	}
	key := policyStageKey(currentUser.ID(), tenantID)

	switch r.Method {
	case http.MethodPost:
		var payload dtos.StagePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_BODY", "unable to parse request body")
			return
		}
		entries, err := c.stageStore.Add(key, payload)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, dtos.StagePolicyResponse{
			Data:  entries,
			Total: len(entries),
		})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", "id is required")
			return
		}
		entries, err := c.stageStore.Delete(key, id)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, dtos.StagePolicyResponse{
			Data:  entries,
			Total: len(entries),
		})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "AUTHZ_METHOD_NOT_ALLOWED", "method not allowed")
	}
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
	requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)
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

	attrs := parseDebugAttributes(query)
	opts := []authz.RequestOption{}
	if len(attrs) > 0 {
		opts = append(opts, authz.WithAttributes(attrs))
	}
	req := authz.NewRequest(subject, domain, object, action, opts...)
	svc := authz.Use()
	result, err := svc.Inspect(r.Context(), req)
	if err != nil {
		logger.WithError(err).Error("authz api: debug check failed")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_DEBUG_ERROR", "failed to evaluate request")
		return
	}
	tenantID := tenantIDFromContext(r)
	logFields := logrus.Fields{
		"request_id": requestIDFromHeader(r),
		"subject":    result.OriginalRequest.Subject,
		"domain":     result.OriginalRequest.Domain,
		"object":     result.OriginalRequest.Object,
		"action":     result.OriginalRequest.Action,
		"tenant_id":  tenantID.String(),
		"allowed":    result.Allowed,
		"latency_ms": result.Latency.Microseconds() / 1_000,
	}
	if len(result.Trace) > 0 {
		logFields["matched_policy"] = result.Trace
	}
	if len(result.OriginalRequest.Attributes) > 0 {
		logFields["attributes"] = result.OriginalRequest.Attributes
	}
	logger.WithFields(logFields).Info("authz debug evaluated request")

	writeJSON(w, http.StatusOK, dtos.DebugResponse{
		Allowed:       result.Allowed,
		Mode:          string(result.Mode),
		LatencyMillis: result.Latency.Milliseconds(),
		Request: dtos.DebugRequestDTO{
			Subject: result.OriginalRequest.Subject,
			Domain:  result.OriginalRequest.Domain,
			Object:  result.OriginalRequest.Object,
			Action:  result.OriginalRequest.Action,
		},
		Attributes: result.OriginalRequest.Attributes,
		Trace: dtos.DebugTraceDTO{
			MatchedPolicy: result.Trace,
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
	approverID := authzutil.NormalizedUserUUID(tenantID, currentUser)
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

type policyListParams struct {
	Subject   string
	Domain    string
	Type      string
	Search    string
	Page      int
	Limit     int
	SortField string
	SortAsc   bool
}

func (p policyListParams) Offset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit
}

func parsePolicyListParams(values url.Values) (policyListParams, error) {
	params := policyListParams{
		Page:      1,
		Limit:     50,
		SortField: "object",
		SortAsc:   true,
	}
	if subject := strings.TrimSpace(values.Get("subject")); subject != "" {
		params.Subject = subject
	}
	if domain := strings.TrimSpace(values.Get("domain")); domain != "" {
		params.Domain = domain
	}
	if typ := strings.TrimSpace(values.Get("type")); typ != "" {
		params.Type = typ
	}
	if search := strings.TrimSpace(values.Get("q")); search != "" {
		params.Search = search
	}
	if page := values.Get("page"); page != "" {
		val, err := strconv.Atoi(page)
		if err != nil || val < 1 {
			return params, errors.New("page must be a positive integer")
		}
		params.Page = val
	}
	if limit := values.Get("limit"); limit != "" {
		val, err := strconv.Atoi(limit)
		if err != nil || val < 1 {
			return params, errors.New("limit must be a positive integer")
		}
		if val > 500 {
			val = 500
		}
		params.Limit = val
	}
	if sort := values.Get("sort"); sort != "" {
		parts := strings.Split(sort, ":")
		field := strings.TrimSpace(parts[0])
		if field != "" {
			params.SortField = field
		}
		if len(parts) > 1 && strings.EqualFold(parts[1], "desc") {
			params.SortAsc = false
		}
	}
	return params, nil
}

func filterPolicies(entries []services.PolicyEntry, params policyListParams) []services.PolicyEntry {
	results := make([]services.PolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if params.Subject != "" && entry.Subject != params.Subject {
			continue
		}
		if params.Domain != "" && entry.Domain != params.Domain {
			continue
		}
		if params.Type != "" && entry.Type != params.Type {
			continue
		}
		if params.Search != "" {
			search := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(entry.Object), search) &&
				!strings.Contains(strings.ToLower(entry.Action), search) {
				continue
			}
		}
		results = append(results, entry)
	}
	return results
}

func sortPolicies(entries []services.PolicyEntry, field string, asc bool) []services.PolicyEntry {
	less := func(i, j int) bool { return entries[i].Object < entries[j].Object }
	switch field {
	case "subject":
		less = func(i, j int) bool { return entries[i].Subject < entries[j].Subject }
	case "domain":
		less = func(i, j int) bool { return entries[i].Domain < entries[j].Domain }
	case "type":
		less = func(i, j int) bool { return entries[i].Type < entries[j].Type }
	case "action":
		less = func(i, j int) bool { return entries[i].Action < entries[j].Action }
	}
	sorted := make([]services.PolicyEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		if asc {
			return less(i, j)
		}
		return less(j, i)
	})
	return sorted
}

func requestIDFromHeader(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(raw))
}

type policyStageStore struct {
	mu   sync.Mutex
	data map[string][]dtos.StagedPolicyEntry
}

func newPolicyStageStore() *policyStageStore {
	return &policyStageStore{
		data: make(map[string][]dtos.StagedPolicyEntry),
	}
}

func policyStageKey(userID uint, tenantID uuid.UUID) string {
	return fmt.Sprintf("%d:%s", userID, tenantID.String())
}

func (s *policyStageStore) Add(key string, payload dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	if strings.TrimSpace(payload.Type) == "" {
		return nil, errors.New("type is required")
	}
	if strings.TrimSpace(payload.Object) == "" {
		return nil, errors.New("object is required")
	}
	if strings.TrimSpace(payload.Action) == "" {
		return nil, errors.New("action is required")
	}
	if strings.TrimSpace(payload.Effect) == "" {
		return nil, errors.New("effect is required")
	}
	if strings.TrimSpace(payload.Domain) == "" {
		return nil, errors.New("domain is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.data[key]
	if len(entries) >= 50 {
		return nil, errors.New("stage limit reached (50)")
	}
	id := uuid.New().String()
	entry := dtos.StagedPolicyEntry{
		ID: id,
		PolicyEntryResponse: dtos.PolicyEntryResponse{
			Type:    payload.Type,
			Subject: payload.Subject,
			Domain:  payload.Domain,
			Object:  payload.Object,
			Action:  authz.NormalizeAction(payload.Action),
			Effect:  payload.Effect,
		},
	}
	entries = append(entries, entry)
	s.data[key] = entries
	return entries, nil
}

func (s *policyStageStore) Delete(key string, id string) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.data[key]
	if !ok {
		return []dtos.StagedPolicyEntry{}, nil
	}
	next := make([]dtos.StagedPolicyEntry, 0, len(entries))
	found := false
	for _, entry := range entries {
		if entry.ID == id {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return nil, errors.New("stage entry not found")
	}
	s.data[key] = next
	return next, nil
}
