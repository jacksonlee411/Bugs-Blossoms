package controllers

import (
	"net/http"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/error_pages"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
	"github.com/iota-uz/iota-sdk/pkg/routing"
)

// RenderForbidden is a helper function that can be used directly in controllers
// to render the 403 forbidden page when permission checks fail
func RenderForbidden(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusForbidden)
	if err := error_pages.ForbiddenContent().Render(r.Context(), w); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handler404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	if err := error_pages.NotFoundContent().Render(r.Context(), w); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

type ErrorHandlersOptions struct {
	Entrypoint    string
	AllowlistPath string
}

func NotFound(app application.Application, opts ...ErrorHandlersOptions) http.HandlerFunc {
	var resolvedOpts ErrorHandlersOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}

	rules, err := routing.LoadAllowlist(resolvedOpts.AllowlistPath, resolvedOpts.Entrypoint)
	if err != nil {
		rules = nil
	}
	classifier := routing.NewClassifier(rules)

	return func(w http.ResponseWriter, r *http.Request) {
		class := classifier.ClassifyPath(r.URL.Path)
		if class == routing.RouteClassInternalAPI || class == routing.RouteClassPublicAPI || class == routing.RouteClassWebhook {
			meta := map[string]string{
				"path": r.URL.Path,
			}
			if requestID := requestIDFromResponse(w, r); requestID != "" {
				meta["request_id"] = requestID
			}
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "not found", meta)
			return
		}

		handler := middleware.WithPageContext()(http.HandlerFunc(handler404))
		handler = middleware.ProvideLocalizer(app)(handler)
		handler.ServeHTTP(w, r)
	}
}

func MethodNotAllowed(opts ...ErrorHandlersOptions) http.HandlerFunc {
	var resolvedOpts ErrorHandlersOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}

	rules, err := routing.LoadAllowlist(resolvedOpts.AllowlistPath, resolvedOpts.Entrypoint)
	if err != nil {
		rules = nil
	}
	classifier := routing.NewClassifier(rules)

	return func(w http.ResponseWriter, r *http.Request) {
		class := classifier.ClassifyPath(r.URL.Path)
		if class == routing.RouteClassInternalAPI || class == routing.RouteClassPublicAPI || class == routing.RouteClassWebhook {
			meta := map[string]string{
				"method": r.Method,
				"path":   r.URL.Path,
			}
			if requestID := requestIDFromResponse(w, r); requestID != "" {
				meta["request_id"] = requestID
			}
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", meta)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func requestIDFromResponse(w http.ResponseWriter, r *http.Request) string {
	if w != nil {
		if requestID := strings.TrimSpace(w.Header().Get("X-Request-Id")); requestID != "" {
			return requestID
		}
		if requestID := strings.TrimSpace(w.Header().Get("X-Request-ID")); requestID != "" {
			return requestID
		}
	}
	if r != nil {
		if requestID := strings.TrimSpace(r.Header.Get("X-Request-Id")); requestID != "" {
			return requestID
		}
		return strings.TrimSpace(r.Header.Get("X-Request-ID"))
	}
	return ""
}
