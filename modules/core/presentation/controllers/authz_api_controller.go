package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	permissionEntity "github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	authz "github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/di"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

// AuthzAPIController exposes REST APIs for policy listings and direct apply.
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
	router.HandleFunc("/policies/stage", di.H(c.stagePolicy)).Methods(http.MethodPost, http.MethodDelete)
	router.Handle(
		"/policies/apply",
		middleware.RateLimit(middleware.RateLimitConfig{
			RequestsPerPeriod: 20,
			Period:            time.Minute,
			KeyFunc:           middleware.EndpointKeyFunc("core.api.authz.policies.apply"),
		})(di.H(c.applyPolicies)),
	).Methods(http.MethodPost)
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
	svc *services.AuthzPolicyService,
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

func (c *AuthzAPIController) stagePolicy(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
) {
	_ = logger
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
		payloads, err := decodeStagePolicyRequests(r)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", err.Error())
			return
		}
		entries, createdIDs, err := c.stageStore.AddManyWithIDs(key, payloads)
		if err != nil {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
			return
		}
		htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(entries)))
		writeJSON(w, http.StatusCreated, dtos.StagePolicyResponse{
			Data:       entries,
			Total:      len(entries),
			CreatedIDs: createdIDs,
		})
	case http.MethodDelete:
		contentType := strings.ToLower(r.Header.Get("Content-Type"))
		if strings.Contains(contentType, "application/json") {
			var payload struct {
				IDs []string `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", "invalid delete payload")
				return
			}
			entries, err := c.stageStore.DeleteMany(key, payload.IDs)
			if err != nil {
				c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_ERROR", err.Error())
				return
			}
			htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(entries)))
			writeJSON(w, http.StatusOK, dtos.StagePolicyResponse{
				Data:  entries,
				Total: len(entries),
			})
			return
		}

		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id != "" {
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
			return
		}

		subject := strings.TrimSpace(r.URL.Query().Get("subject"))
		domain := strings.TrimSpace(r.URL.Query().Get("domain"))
		if subject == "" && domain == "" {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", "id or subject/domain is required")
			return
		}
		c.stageStore.Clear(key, subject, domain)
		entries := c.stageStore.List(key, "", "")
		htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(entries)))
		writeJSON(w, http.StatusOK, dtos.StagePolicyResponse{
			Data:  entries,
			Total: len(entries),
		})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "AUTHZ_METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (c *AuthzAPIController) applyPolicies(
	w http.ResponseWriter,
	r *http.Request,
	logger *logrus.Entry,
	svc *services.AuthzPolicyService,
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

	payload, err := decodeApplyPolicyRequest(r)
	if err != nil {
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", err.Error())
		return
	}
	payload.BaseRevision = strings.TrimSpace(payload.BaseRevision)
	payload.Subject = strings.TrimSpace(payload.Subject)
	payload.Domain = strings.TrimSpace(payload.Domain)

	if payload.BaseRevision == "" || payload.Subject == "" || payload.Domain == "" {
		c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_INVALID_BODY", "base_revision, subject and domain are required")
		return
	}

	key := policyStageKey(currentUser.ID(), tenantID)
	changes := make([]services.PolicyChange, 0, len(payload.Changes))
	if len(payload.Changes) > 0 {
		changes = append(changes, payload.Changes...)
	} else {
		entries := c.stageStore.List(key, payload.Subject, payload.Domain)
		if len(entries) == 0 {
			c.writeHTMXError(w, r, http.StatusBadRequest, "AUTHZ_STAGE_EMPTY", "no staged policies found")
			return
		}
		for _, entry := range entries {
			changes = append(changes, services.PolicyChange{
				StageKind: entry.StageKind,
				PolicyEntry: services.PolicyEntry{
					Type:    entry.Type,
					Subject: entry.Subject,
					Domain:  entry.Domain,
					Object:  entry.Object,
					Action:  entry.Action,
					Effect:  entry.Effect,
				},
			})
		}
	}

	result, err := svc.ApplyAndReload(r.Context(), payload.BaseRevision, changes, authz.Use().ReloadPolicy)
	if err != nil {
		logger.WithError(err).Warn("authz api: apply failed")
		c.respondApplyError(w, r, err)
		return
	}

	c.stageStore.Clear(key, payload.Subject, payload.Domain)
	remaining := c.stageStore.List(key, "", "")

	if htmx.IsHxRequest(r) {
		htmx.SetTrigger(w, "policies:staged", fmt.Sprintf(`{"total":%d}`, len(remaining)))
		if detail, marshalErr := json.Marshal(map[string]any{
			"base_revision": result.BaseRevision,
			"revision":      result.Revision,
			"added":         result.Added,
			"removed":       result.Removed,
		}); marshalErr == nil {
			htmx.SetTrigger(w, "authz:policies-applied", string(detail))
		}
	}

	writeJSON(w, http.StatusOK, result)
}

type applyPolicyPayload struct {
	BaseRevision string                  `json:"base_revision"`
	Subject      string                  `json:"subject"`
	Domain       string                  `json:"domain"`
	Reason       string                  `json:"reason"`
	Changes      []services.PolicyChange `json:"changes"`
}

func decodeApplyPolicyRequest(r *http.Request) (applyPolicyPayload, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload applyPolicyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return applyPolicyPayload{}, err
		}
		return payload, nil
	}
	if err := r.ParseForm(); err != nil {
		return applyPolicyPayload{}, err
	}
	return applyPolicyPayload{
		BaseRevision: r.FormValue("base_revision"),
		Subject:      r.FormValue("subject"),
		Domain:       r.FormValue("domain"),
		Reason:       r.FormValue("reason"),
	}, nil
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
		"request_id": authzutil.RequestIDFromRequest(r),
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

func (c *AuthzAPIController) respondApplyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrRevisionMismatch):
		meta := map[string]string{}
		if rev := authzutil.BaseRevision(r.Context()); rev != "" {
			meta["base_revision"] = rev
		}
		c.writeHTMXErrorWithMeta(w, r, http.StatusConflict, "AUTHZ_BASE_REVISION_MISMATCH", "Policy base revision is stale, please refresh", meta)
	case errors.Is(err, services.ErrPolicyApply):
		c.writeHTMXError(w, r, http.StatusUnprocessableEntity, "AUTHZ_POLICY_APPLY_FAILED", err.Error())
	default:
		c.writeHTMXError(w, r, http.StatusInternalServerError, "AUTHZ_POLICY_WRITE_FAILED", "failed to apply policies")
	}
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

type errorTriggerPayload struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Meta    map[string]string `json:"meta,omitempty"`
}

func (c *AuthzAPIController) writeHTMXError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
) {
	c.writeHTMXErrorWithMeta(w, r, status, code, message, nil)
}

func (c *AuthzAPIController) writeHTMXErrorWithMeta(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
	meta map[string]string,
) {
	msgKey := errorMessageKey(code, message)
	if htmx.IsHxRequest(r) {
		triggerErrorToast(w, msgKey, code, meta)
	}
	writeJSONError(w, status, code, msgKey, meta)
}

func triggerErrorToast(w http.ResponseWriter, message string, code string, meta map[string]string) {
	payload := errorTriggerPayload{
		Message: message,
		Code:    code,
	}
	if meta != nil {
		payload.Meta = meta
	}
	if detail, err := json.Marshal(payload); err == nil {
		notify := fmt.Sprintf(`{"variant":"error","title":"%s","message":"%s"}`, code, message)
		w.Header().Set("Hx-Trigger", fmt.Sprintf(`{"showErrorToast": %s, "notify": %s}`, string(detail), notify))
	}
}

func errorMessageKey(code string, fallback string) string {
	switch code {
	case "AUTHZ_INVALID_BODY", "AUTHZ_INVALID_QUERY":
		return "Request validation failed"
	case "AUTHZ_STAGE_EMPTY":
		return "No staged policies to apply"
	case "AUTHZ_BASE_REVISION_MISMATCH":
		return "Policy base revision is stale, please refresh"
	case "AUTHZ_FORBIDDEN":
		return "Permission denied"
	default:
		if strings.TrimSpace(fallback) != "" {
			return fallback
		}
		return "Request failed, please try again"
	}
}

func decodeStagePolicyRequests(r *http.Request) ([]dtos.StagePolicyRequest, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			return nil, err
		}
		trimmed := bytesTrimSpace(raw)
		if len(trimmed) == 0 {
			return nil, errors.New("empty payload")
		}
		if trimmed[0] == '[' {
			var payload []dtos.StagePolicyRequest
			if err := json.Unmarshal(trimmed, &payload); err != nil {
				return nil, err
			}
			return payload, nil
		}
		var payload dtos.StagePolicyRequest
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return nil, err
		}
		return []dtos.StagePolicyRequest{payload}, nil
	}

	payload, err := decodeStagePolicyRequest(r)
	if err != nil {
		return nil, err
	}
	return []dtos.StagePolicyRequest{payload}, nil
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
		Type:      r.FormValue("type"),
		Subject:   r.FormValue("subject"),
		Domain:    r.FormValue("domain"),
		Object:    r.FormValue("object"),
		Action:    r.FormValue("action"),
		Effect:    r.FormValue("effect"),
		StageKind: r.FormValue("stage_kind"),
	}, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
