package controllers

import (
	"net/http"

	"github.com/iota-uz/iota-sdk/pkg/httpapi"
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	_ = httpapi.WriteJSON(w, status, payload)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string, meta ...map[string]string) {
	var resolvedMeta map[string]string
	if len(meta) > 0 {
		resolvedMeta = meta[0]
	}
	_ = httpapi.WriteError(w, status, code, message, resolvedMeta)
}
