package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	coredtos "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(err)
	}
}

func ensureRequestID(w http.ResponseWriter, r *http.Request) string {
	if r == nil {
		return ""
	}
	header := strings.TrimSpace(configuration.Use().RequestIDHeader)
	if header == "" {
		header = "X-Request-ID"
	}

	requestID := strings.TrimSpace(r.Header.Get(header))
	if requestID == "" {
		requestID = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	}
	if requestID == "" {
		requestID = uuid.NewString()
		w.Header().Set(header, requestID)
	}
	return requestID
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	meta := map[string]string{
		"request_id": ensureRequestID(w, r),
	}
	writeJSON(w, status, coredtos.APIError{
		Code:    code,
		Message: message,
		Meta:    meta,
	})
}
