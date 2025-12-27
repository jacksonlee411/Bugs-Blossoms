package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

const logsAuthzObject = "logging.logs"

func ensureLoggingAuthz(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	opts ...authz.RequestOption,
) bool {
	capKey := authzutil.CapabilityKey(logsAuthzObject, action)
	tenantID := tenantIDFromContext(r)
	currentUser, err := composables.UseUser(r.Context())
	ctxWithState, state := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}
	authzDomain := authz.DomainFromTenant(tenantID)
	subject := resolveLoggingSubject(tenantID, currentUser)
	req := authz.NewRequest(
		subject,
		authzDomain,
		logsAuthzObject,
		authz.NormalizeAction(action),
		opts...,
	)

	if err != nil || currentUser == nil {
		recordForbiddenCapability(state, r, logsAuthzObject, action, capKey)
		logUnauthorizedAccess(r, req, authz.Use().Mode(), err)
		layouts.WriteAuthzForbiddenResponse(w, r, logsAuthzObject, action)
		return false
	}

	svc := authz.Use()
	mode := svc.Mode()

	allowed, authzErr := enforceRequest(r.Context(), svc, req, mode)
	if authzErr != nil {
		recordForbiddenCapability(state, r, logsAuthzObject, action, capKey)
		logUnauthorizedAccess(r, req, mode, authzErr)
		layouts.WriteAuthzForbiddenResponse(w, r, logsAuthzObject, action)
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
	layouts.WriteAuthzForbiddenResponse(w, r, logsAuthzObject, action)
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

func tenantIDFromContext(r *http.Request) uuid.UUID {
	return authzutil.TenantIDFromContext(r.Context())
}

func authzDomainFromContext(r *http.Request) string {
	return authzutil.DomainFromContext(r.Context())
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
	tenantID := tenantIDFromContext(r)
	traceID := ""
	if spanCtx := trace.SpanFromContext(r.Context()).SpanContext(); spanCtx.HasTraceID() {
		traceID = spanCtx.TraceID().String()
	}

	fields := logrus.Fields{
		"authz.subject": req.Subject,
		"authz.domain":  req.Domain,
		"authz.object":  req.Object,
		"authz.action":  req.Action,
		"authz.mode":    mode,
		"ip":            ip,
		"user_agent":    ua,
		"request_id":    r.Header.Get("X-Request-ID"),
		"tenant_id":     tenantID,
		"http.method":   r.Method,
		"http.path":     r.URL.Path,
	}
	if traceID != "" {
		fields["trace_id"] = traceID
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
	if app.DB() == nil {
		logger.Warn("action-log: database pool not available, skip audit persistence")
		return
	}
	currentUser, userErr := composables.UseUser(r.Context())
	if userErr != nil || currentUser == nil {
		return
	}
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

func resolveLoggingSubject(tenantID uuid.UUID, currentUser user.User) string {
	if currentUser == nil {
		return authz.SubjectForUserID(tenantID, "anonymous")
	}
	return authzutil.SubjectForUser(tenantID, currentUser)
}
