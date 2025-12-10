package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, message string, meta ...map[string]string) {
	payload := &dtos.APIError{
		Code:    code,
		Message: message,
	}
	if len(meta) > 0 && meta[0] != nil {
		payload.Meta = meta[0]
	}
	writeJSON(w, status, payload)
}
