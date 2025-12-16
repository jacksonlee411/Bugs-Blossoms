package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func RequireTenantFromHost(app application.Application) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := normalizeHost(r.Host)
			if host == "" {
				http.NotFound(w, r)
				return
			}

			tenantService := app.Service(services.TenantService{}).(*services.TenantService)
			t, err := tenantService.GetByDomain(r.Context(), host)
			if err != nil {
				logger := composables.UseLogger(r.Context())
				logger.WithField("host", host).WithField("path", r.URL.Path).WithError(err).Warn("tenant not found for host")
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(composables.WithTenantID(r.Context(), t.ID())))
		})
	}
}

func normalizeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ToLower(raw)
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return strings.ToLower(strings.TrimSpace(h))
	}
	return raw
}
