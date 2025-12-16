package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

type SignatureVerifier interface {
	Verify(ctx context.Context, r *http.Request, body []byte) error
}

type ReplayProtector interface {
	Check(ctx context.Context, r *http.Request, body []byte) error
}

type Option func(*options)

type options struct {
	MaxBodyBytes int64
}

func WithMaxBodyBytes(n int64) Option {
	return func(o *options) {
		o.MaxBodyBytes = n
	}
}

func Bind(router *mux.Router, prefix string, verifier SignatureVerifier, protector ReplayProtector, opts ...Option) *mux.Router {
	if router == nil {
		return nil
	}
	if strings.TrimSpace(prefix) == "" {
		prefix = "/webhooks"
	}
	sub := router.PathPrefix(prefix).Subrouter()
	sub.Use(Middleware(verifier, protector, opts...))
	return sub
}

func Middleware(verifier SignatureVerifier, protector ReplayProtector, opts ...Option) mux.MiddlewareFunc {
	resolved := &options{
		MaxBodyBytes: 1024 * 1024, // 1MB default; override per provider if needed.
	}
	for _, opt := range opts {
		if opt != nil {
			opt(resolved)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if verifier == nil || protector == nil {
				writeJSONError(w, http.StatusInternalServerError, "WEBHOOK_MISCONFIGURED", "webhook middleware misconfigured", nil)
				return
			}

			body, err := readAndRestoreBody(r, resolved.MaxBodyBytes)
			if err != nil {
				code := "WEBHOOK_BAD_REQUEST"
				status := http.StatusBadRequest
				if errors.Is(err, errBodyTooLarge) {
					code = "WEBHOOK_PAYLOAD_TOO_LARGE"
					status = http.StatusRequestEntityTooLarge
				}
				writeJSONError(w, status, code, "invalid webhook payload", map[string]string{
					"error": err.Error(),
				})
				return
			}

			if err := verifier.Verify(r.Context(), r, body); err != nil {
				writeJSONError(w, http.StatusUnauthorized, "WEBHOOK_UNAUTHORIZED", "invalid webhook signature", map[string]string{
					"error": err.Error(),
				})
				return
			}

			if err := protector.Check(r.Context(), r, body); err != nil {
				if errors.Is(err, ErrReplayDetected) {
					writeJSONError(w, http.StatusConflict, "WEBHOOK_REPLAY", "webhook replay detected", map[string]string{
						"error": err.Error(),
					})
					return
				}
				writeJSONError(w, http.StatusBadRequest, "WEBHOOK_BAD_REQUEST", "invalid webhook payload", map[string]string{
					"error": err.Error(),
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

var errBodyTooLarge = errors.New("webhook payload too large")

var ErrReplayDetected = errors.New("webhook replay detected")

func readAndRestoreBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}

	limited := io.LimitReader(r.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errBodyTooLarge
	}

	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func writeJSONError(w http.ResponseWriter, status int, code, message string, meta map[string]string) {
	if w == nil {
		return
	}
	if strings.TrimSpace(code) == "" {
		code = "WEBHOOK_ERROR"
	}
	if strings.TrimSpace(message) == "" {
		message = "webhook error"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := webhookError{
		Code:    code,
		Message: message,
		Meta:    meta,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

type webhookError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta,omitempty"`
}
