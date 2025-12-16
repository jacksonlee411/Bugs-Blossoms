package middleware

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
)

// WithTransaction is deprecated and will be removed in the future.
func WithTransaction() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pool, err := composables.UsePool(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			tx, err := pool.Begin(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer func() {
				if err := tx.Rollback(r.Context()); err != nil {
					if errors.Is(err, pgx.ErrTxClosed) {
						return
					}
					logger := composables.UseLogger(r.Context())
					logger.WithError(err).Error("failed to rollback transaction")
				}
			}()
			ctxWithTx := composables.WithTx(r.Context(), tx)
			if err := composables.ApplyTenantRLS(ctxWithTx, tx); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			r = r.WithContext(ctxWithTx)
			next.ServeHTTP(w, r)
			if err := tx.Commit(r.Context()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	}
}
