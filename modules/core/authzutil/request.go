package authzutil

import (
	"net/http"
	"strings"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

// RequestIDFromRequest extracts the request id from common headers.
func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := configuration.Use().RequestIDHeader
	if strings.TrimSpace(header) == "" {
		header = "X-Request-ID"
	}
	if id := r.Header.Get(header); id != "" {
		return id
	}
	return r.Header.Get("X-Request-Id")
}
