package middleware

import (
	"net/http"

	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"github.com/iota-uz/iota-sdk/pkg/types"

	"github.com/gorilla/mux"
	"github.com/iota-uz/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func WithPageContext() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				localizer, found := intl.UseLocalizer(ctx)
				if !found || localizer == nil {
					app, err := application.UseApp(ctx)
					if err != nil {
						panic(intl.ErrNoLocalizer)
					}
					locale, ok := intl.UseLocale(ctx)
					if !ok {
						locale = language.English
						ctx = intl.WithLocale(ctx, locale)
					}
					localizer = i18n.NewLocalizer(app.Bundle(), locale.String())
					ctx = intl.WithLocalizer(ctx, localizer)
					r = r.WithContext(ctx)
				}

				locale, ok := intl.UseLocale(r.Context())
				if !ok {
					locale = language.English
					r = r.WithContext(intl.WithLocale(r.Context(), locale))
				}
				//nolint:staticcheck // SA1019: This is the legitimate factory for creating PageContext instances
				pageCtx := &types.PageContext{
					URL:       r.URL,
					Localizer: localizer,
					Locale:    locale,
				}
				pageCtx.SetAuthzState(composables.UseAuthzViewState(r.Context()))
				next.ServeHTTP(w, r.WithContext(composables.WithPageCtx(r.Context(), pageCtx)))
			},
		)
	}
}
