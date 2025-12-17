package webhooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type headerVerifier struct{}

func (headerVerifier) Verify(_ context.Context, r *http.Request, _ []byte) error {
	if r.Header.Get("X-Signature") != "ok" {
		return errors.New("bad signature")
	}
	return nil
}

type idReplayProtector struct {
	seen map[string]struct{}
}

func (p *idReplayProtector) Check(_ context.Context, r *http.Request, _ []byte) error {
	id := r.Header.Get("X-Webhook-Id")
	if id == "" {
		return errors.New("missing webhook id")
	}
	if _, ok := p.seen[id]; ok {
		return ErrReplayDetected
	}
	p.seen[id] = struct{}{}
	return nil
}

func TestMiddleware_AllowsAndRestoresBody(t *testing.T) {
	router := mux.NewRouter()
	sub := Bind(router, "/webhooks/test", headerVerifier{}, &idReplayProtector{seen: map[string]struct{}{}})
	require.NotNil(t, sub)
	sub.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", bytes.NewBufferString("hello"))
	req.Header.Set("X-Signature", "ok")
	req.Header.Set("X-Webhook-Id", "evt-1")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "hello", rr.Body.String())
}

func TestMiddleware_DeniesInvalidSignature(t *testing.T) {
	router := mux.NewRouter()
	sub := Bind(router, "/webhooks/test", headerVerifier{}, &idReplayProtector{seen: map[string]struct{}{}})
	require.NotNil(t, sub)
	sub.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", nil)
	req.Header.Set("X-Webhook-Id", "evt-1")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestMiddleware_DeniesReplay(t *testing.T) {
	router := mux.NewRouter()
	sub := Bind(router, "/webhooks/test", headerVerifier{}, &idReplayProtector{seen: map[string]struct{}{}})
	require.NotNil(t, sub)
	sub.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	first := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", nil)
	first.Header.Set("X-Signature", "ok")
	first.Header.Set("X-Webhook-Id", "evt-1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, first)
	require.Equal(t, http.StatusOK, rr.Code)

	second := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", nil)
	second.Header.Set("X-Signature", "ok")
	second.Header.Set("X-Webhook-Id", "evt-1")
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, second)

	require.Equal(t, http.StatusConflict, rr2.Code)
	require.Equal(t, "application/json", rr2.Header().Get("Content-Type"))
}

func TestMiddleware_BadPayloadIfReplayProtectorCannotCheck(t *testing.T) {
	router := mux.NewRouter()
	sub := Bind(router, "/webhooks/test", headerVerifier{}, &idReplayProtector{seen: map[string]struct{}{}})
	require.NotNil(t, sub)
	sub.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", nil)
	req.Header.Set("X-Signature", "ok")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestMiddleware_DeniesWhenMisconfigured(t *testing.T) {
	router := mux.NewRouter()
	router.Use(Middleware(nil, nil))
	router.HandleFunc("/webhooks/test/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/webhooks/test/ping", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}
