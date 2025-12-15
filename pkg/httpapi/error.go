package httpapi

import (
	"encoding/json"
	"net/http"
)

// ErrorEnvelope standardizes JSON error responses for API namespaces.
type ErrorEnvelope struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Meta    map[string]string `json:"meta,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	if w == nil {
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, code, message string, meta map[string]string) error {
	return WriteJSON(w, status, &ErrorEnvelope{
		Code:    code,
		Message: message,
		Meta:    meta,
	})
}
