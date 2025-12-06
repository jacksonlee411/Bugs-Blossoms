package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

// ActionLogMiddleware records successful requests into action_logs when enabled.
// It is kept lightweight: best-effort logging, per-request transaction, and no request blocking on failure.
func ActionLogMiddleware(app application.Application) mux.MiddlewareFunc {
	conf := configuration.Use()
	if !conf.ActionLogEnabled {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			tenantID, err := composables.UseTenantID(r.Context())
			if err != nil {
				return
			}
			currentUser, err := composables.UseUser(r.Context())
			if err != nil || currentUser == nil {
				return
			}

			userID := currentUser.ID()
			ua, _ := composables.UseUserAgent(r.Context())
			ip, _ := composables.UseIP(r.Context())

			logsService := app.Service(services.LogsService{}).(*services.LogsService)
			ctx := r.Context()

			tx, txErr := app.DB().Begin(ctx)
			if txErr != nil {
				composables.UseLogger(ctx).WithError(txErr).Warn("action-log: failed to begin transaction")
				return
			}
			committed := false
			defer func() {
				if !committed {
					_ = tx.Rollback(ctx)
				}
			}()

			ctx = composables.WithTx(ctx, tx)
			ctx = composables.WithTenantID(ctx, tenantID)

			entry := &actionlog.ActionLog{
				TenantID:  tenantID,
				UserID:    &userID,
				Method:    strings.ToUpper(r.Method),
				Path:      r.URL.Path,
				UserAgent: ua,
				IP:        ip,
				CreatedAt: time.Now(),
			}

			if err := logsService.CreateActionLog(ctx, entry); err != nil {
				composables.UseLogger(ctx).WithError(err).Warn("action-log: failed to persist request")
				return
			}
			if err := tx.Commit(ctx); err != nil {
				composables.UseLogger(ctx).WithError(err).Warn("action-log: failed to commit transaction")
				return
			}
			committed = true
		})
	}
}
