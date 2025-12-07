package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	"github.com/iota-uz/iota-sdk/pkg/htmx"
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
		stageStore: usePolicyStageStore(),
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
	pageEntries, total := paginatePolicies(entries, params)
	pageData := make([]dtos.PolicyEntryResponse, 0, len(pageEntries))
	for _, entry := range pageEntries {
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
		Total: total,
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
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	draft, err := svc.Get(r.Context(), tenantID, id)
	if err != nil {
		c.respondServiceError(w, r, err)
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
	payload, err := decodePolicyDraftRequest(r)
	if err != nil {
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", err.Error())
		return
	}
	tenantID := tenantIDFromContext(r)
	requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	logger.WithFields(logrus.Fields{
		"diff_len": len(payload.Diff),
		"subject":  payload.Subject,
		"domain":   payload.Domain,
		"object":   payload.Object,
		"action":   payload.Action,
	}).Debug("authz api: create draft payload")
	diffStr := strings.TrimSpace(string(payload.Diff))
	if diffStr == "" || strings.EqualFold(diffStr, "null") {
		payload, err = c.buildDraftFromStage(r.Context(), tenantID, currentUser, payload)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_EMPTY", err.Error())
			return
		}
	}
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
		logger.WithError(err).Error("authz api: create draft failed")
		c.respondServiceError(w, r, err)
		return
	}
	if htmx.IsHxRequest(r) {
		htmx.SetTrigger(w, "policies:staged", `{"total":0}`)
		htmx.TriggerToast(w, htmx.ToastVariantSuccess, "提交成功", "已提交策略草稿")
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
		c.writeHTMXError(w, r, http.StatusUnauthorized, "AUTHZ_NO_USER", "user not found in context")
		return
	}
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_NO_TENANT", "tenant not found in context")
		return
	}
	key := policyStageKey(currentUser.ID(), tenantID)

	switch r.Method {
	case http.MethodPost:
		payload, err := decodeStagePolicyRequest(r)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", err.Error())
			return
		}
		entries, err := c.stageStore.Add(key, payload)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
			return
		}
		htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(entries)))
		writeJSON(w, http.StatusCreated, dtos.StagePolicyResponse{
			Data:  entries,
			Total: len(entries),
		})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", "id is required")
			return
		}
		entries, err := c.stageStore.Delete(key, id)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
			return
		}
		htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(entries)))
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
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	draft, err := svc.Cancel(r.Context(), tenantID, id)
	if err != nil {
		c.respondServiceError(w, r, err)
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
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	locker := r.URL.Query().Get("locker")
	tenantID := tenantIDFromContext(r)
	draft, err := svc.TriggerBot(r.Context(), tenantID, id, locker)
	if err != nil {
		c.respondServiceError(w, r, err)
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
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	draft, err := svc.Revert(r.Context(), tenantID, id, requesterID)
	if err != nil {
		c.respondServiceError(w, r, err)
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
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_ID", "invalid request id")
		return
	}
	tenantID := tenantIDFromContext(r)
	approverID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	draft, err := reviewFunc(r.Context(), tenantID, id, approverID)
	if err != nil {
		c.respondServiceError(w, r, err)
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
		c.writeHTMXError(w, r, http.StatusForbidden, "AUTHZ_FORBIDDEN", "permission denied")
		return false
	}
	return true
}

func (c *AuthzAPIController) writeHTMXError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
) {
	if htmx.IsHxRequest(r) {
		htmx.TriggerToast(w, htmx.ToastVariantError, "操作失败", message)
	}
	writeJSONError(w, status, code, message)
}

func (c *AuthzAPIController) respondServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrPolicyDraftNotFound):
		c.writeHTMXError(w, r, http.StatusNotFound, "AUTHZ_NOT_FOUND", "request not found")
	case errors.Is(err, services.ErrInvalidDiff),
		errors.Is(err, services.ErrRevisionMismatch):
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_REQUEST", err.Error())
	case errors.Is(err, services.ErrInvalidStatusTransition):
		c.writeHTMXError(w, r, http.StatusConflict, "AUTHZ_INVALID_STATE", err.Error())
	case errors.Is(err, services.ErrMissingSnapshot):
		c.writeHTMXError(w, r, http.StatusConflict, "AUTHZ_NO_SNAPSHOT", err.Error())
	case errors.Is(err, services.ErrTenantMismatch):
		c.writeHTMXError(w, r, http.StatusForbidden, "AUTHZ_FORBIDDEN", "tenant mismatch")
	default:
		c.writeHTMXError(w, r, http.StatusInternalServerError, "AUTHZ_ERROR", "internal error")
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

func requestIDFromHeader(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(raw))
}

func decodeStagePolicyRequest(r *http.Request) (dtos.StagePolicyRequest, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload dtos.StagePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return dtos.StagePolicyRequest{}, err
		}
		return payload, nil
	}
	if err := r.ParseForm(); err != nil {
		return dtos.StagePolicyRequest{}, err
	}
	return dtos.StagePolicyRequest{
		Type:    r.FormValue("type"),
		Subject: r.FormValue("subject"),
		Domain:  r.FormValue("domain"),
		Object:  r.FormValue("object"),
		Action:  r.FormValue("action"),
		Effect:  r.FormValue("effect"),
	}, nil
}

func decodePolicyDraftRequest(r *http.Request) (dtos.PolicyDraftRequest, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload dtos.PolicyDraftRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return dtos.PolicyDraftRequest{}, err
		}
		return payload, nil
	}
	if err := r.ParseForm(); err != nil {
		return dtos.PolicyDraftRequest{}, err
	}
	return dtos.PolicyDraftRequest{
		Object:       r.FormValue("object"),
		Action:       r.FormValue("action"),
		Reason:       r.FormValue("reason"),
		BaseRevision: r.FormValue("base_revision"),
		Domain:       r.FormValue("domain"),
		Subject:      r.FormValue("subject"),
	}, nil
}

func (c *AuthzAPIController) buildDraftFromStage(
	ctx context.Context,
	tenantID uuid.UUID,
	currentUser user.User,
	payload dtos.PolicyDraftRequest,
) (dtos.PolicyDraftRequest, error) {
	key := policyStageKey(currentUser.ID(), tenantID)
	subject := strings.TrimSpace(payload.Subject)
	domain := strings.TrimSpace(payload.Domain)
	entries := c.stageStore.List(key, subject, domain)
	if len(entries) == 0 {
		return payload, errors.New("暂无可提交的暂存规则")
	}
	if subject == "" {
		subject = entries[0].Subject
	}
	if domain == "" {
		domain = entries[0].Domain
	}
	filtered := make([]dtos.StagedPolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Subject == subject && entry.Domain == domain {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return payload, errors.New("暂存规则与当前筛选不匹配")
	}
	first := filtered[0]
	if strings.TrimSpace(first.Object) == "" || strings.TrimSpace(first.Action) == "" {
		return payload, errors.New("暂存规则缺少对象或动作")
	}
	logrus.WithFields(logrus.Fields{
		"stage_count": len(filtered),
		"subject":     first.Subject,
		"domain":      first.Domain,
		"object":      first.Object,
		"action":      first.Action,
	}).Debug("authz stage draft build")
	if logger := composables.UseLogger(ctx); logger != nil {
		logger.WithFields(logrus.Fields{
			"subject": first.Subject,
			"domain":  first.Domain,
			"object":  first.Object,
			"action":  first.Action,
		}).Info("authz stage draft build")
	}
	patches := make([]map[string]any, 0, len(filtered))
	for _, entry := range filtered {
		op := "add"
		if entry.StageKind == "remove" {
			op = "remove"
		}
		patches = append(patches, map[string]any{
			"op":   op,
			"path": "/p",
			"value": []string{
				entry.Subject,
				entry.Object,
				entry.Action,
				entry.Domain,
				entry.Effect,
			},
		})
	}
	diffBytes, err := json.Marshal(patches)
	if err != nil {
		return payload, err
	}
	payload.Subject = subject
	payload.Domain = domain
	if strings.TrimSpace(payload.Object) == "" {
		payload.Object = first.Object
	}
	if strings.TrimSpace(payload.Action) == "" {
		payload.Action = first.Action
	}
	payload.Diff = diffBytes
	c.stageStore.Clear(key, subject, domain)
	return payload, nil
}
