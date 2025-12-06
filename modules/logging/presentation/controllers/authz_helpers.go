package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	corecomponents "github.com/iota-uz/iota-sdk/modules/core/presentation/templates/components"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
)

const logsAuthzObject = "logging.logs"

func ensureLoggingAuthz(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	opts ...authz.RequestOption,
) bool {
	capKey := authzutil.CapabilityKey(logsAuthzObject, action)

	currentUser, err := composables.UseUser(r.Context())
	if err != nil || currentUser == nil {
		recordForbiddenCapability(authz.ViewStateFromContext(r.Context()), r, logsAuthzObject, action, capKey)
		writeForbiddenResponse(w, r, logsAuthzObject, action)
		return false
	}

	tenantID := tenantIDFromContext(r)
	ctxWithState, state := authzutil.EnsureViewState(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}

	svc := authz.Use()
	mode := svc.Mode()
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authz.DomainFromTenant(tenantID),
		logsAuthzObject,
		authz.NormalizeAction(action),
		opts...,
	)

	allowed, authzErr := enforceRequest(r.Context(), svc, req, mode)
	if authzErr != nil {
		recordForbiddenCapability(state, r, logsAuthzObject, action, capKey)
		logUnauthorizedAccess(r, req, mode, authzErr)
		writeForbiddenResponse(w, r, logsAuthzObject, action)
		return false
	}

	if allowed {
		if state != nil {
			state.SetCapability(capKey, true)
		}
		return true
	}

	recordForbiddenCapability(state, r, logsAuthzObject, action, capKey)
	logUnauthorizedAccess(r, req, mode, nil)
	writeForbiddenResponse(w, r, logsAuthzObject, action)
	return false
}

func enforceRequest(ctx context.Context, svc *authz.Service, req authz.Request, mode authz.Mode) (bool, error) {
	if svc == nil {
		return true, nil
	}
	if err := svc.Authorize(ctx, req); err != nil {
		return false, err
	}

	switch mode {
	case authz.ModeDisabled, authz.ModeEnforce:
		return true, nil
	case authz.ModeShadow:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	default:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	}
}

func writeForbiddenResponse(w http.ResponseWriter, r *http.Request, object, action string) {
	msg := fmt.Sprintf("Forbidden: %s %s. 如需申请权限，请访问 /core/api/authz/requests。", object, action)

	state := authz.ViewStateFromContext(r.Context())
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") {
		payload := map[string]interface{}{
			"message":         msg,
			"object":          object,
			"action":          action,
			"missingPolicies": nil,
		}
		if state != nil {
			payload["missingPolicies"] = state.MissingPolicies
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			composables.UseLogger(r.Context()).WithError(err).Warn("failed to encode forbidden response")
		}
		return
	}

	if htmx.IsHxRequest(r) {
		w.Header().Set("Hx-Retarget", "body")
		w.Header().Set("Hx-Reswap", "innerHTML")
	}
	if _, ok := composables.TryUsePageCtx(r.Context()); ok {
		props := &corecomponents.UnauthorizedProps{
			Object:    object,
			Action:    action,
			Operation: fmt.Sprintf("%s %s", object, action),
			State:     state,
			Request:   "/core/api/authz/requests",
		}
		w.WriteHeader(http.StatusForbidden)
		templ.Handler(corecomponents.Unauthorized(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	http.Error(w, msg, http.StatusForbidden)
}

func tenantIDFromContext(r *http.Request) uuid.UUID {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		return uuid.Nil
	}
	return tenantID
}

func authzDomainFromContext(r *http.Request) string {
	tenantID := tenantIDFromContext(r)
	return authz.DomainFromTenant(tenantID)
}

func recordForbiddenCapability(state *authz.ViewState, r *http.Request, object, action, capKey string) {
	if state == nil {
		return
	}
	state.SetCapability(capKey, false)
	state.AddMissingPolicy(authz.MissingPolicy{
		Domain: authzDomainFromContext(r),
		Object: object,
		Action: authz.NormalizeAction(action),
	})
}

func logUnauthorizedAccess(r *http.Request, req authz.Request, mode authz.Mode, err error) {
	logger := composables.UseLogger(r.Context())
	ip, _ := composables.UseIP(r.Context())
	ua, _ := composables.UseUserAgent(r.Context())

	fields := logrus.Fields{
		"authz.subject": req.Subject,
		"authz.domain":  req.Domain,
		"authz.object":  req.Object,
		"authz.action":  req.Action,
		"authz.mode":    mode,
		"ip":            ip,
		"user_agent":    ua,
		"request_id":    r.Header.Get("X-Request-ID"),
	}

	entry := logger.WithFields(fields)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Warn("logging.authz.forbidden")

	if !configuration.Use().ActionLogEnabled {
		return
	}

	app, appErr := application.UseApp(r.Context())
	if appErr != nil {
		return
	}
	currentUser, userErr := composables.UseUser(r.Context())
	if userErr != nil || currentUser == nil {
		return
	}
	tenantID := tenantIDFromContext(r)
	if tenantID == uuid.Nil {
		return
	}

	tx, txErr := app.DB().Begin(r.Context())
	if txErr != nil {
		logger.WithError(txErr).Warn("action-log: failed to begin transaction")
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(r.Context())
		}
	}()

	ctx := composables.WithTx(r.Context(), tx)
	ctx = composables.WithTenantID(ctx, tenantID)

	userID := currentUser.ID()
	entryPayload := &actionlog.ActionLog{
		TenantID:  tenantID,
		UserID:    &userID,
		Method:    strings.ToUpper(r.Method),
		Path:      r.URL.Path,
		UserAgent: ua,
		IP:        ip,
		CreatedAt: time.Now(),
	}

	logsService := app.Service(services.LogsService{}).(*services.LogsService)
	if err := logsService.CreateActionLog(ctx, entryPayload); err != nil {
		logger.WithError(err).Warn("action-log: failed to persist forbidden request")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		logger.WithError(err).Warn("action-log: failed to commit transaction")
		return
	}
	committed = true
}
