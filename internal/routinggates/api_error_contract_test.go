package routinggates

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	corecontrollers "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

func TestAPIErrorContracts_JSONOnly_For404And405(t *testing.T) {
	app := application.New(&application.ApplicationOptions{
		Bundle: application.LoadBundle(),
	})

	opts := corecontrollers.ErrorHandlersOptions{Entrypoint: "server"}
	notFound := corecontrollers.NotFound(app, opts)
	methodNotAllowed := corecontrollers.MethodNotAllowed(opts)

	t.Run("404_public_api_is_json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/__nonexistent__", nil)
		req.Header.Set("X-Request-ID", "req-404-public")
		notFound(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var payload apiError
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "NOT_FOUND", payload.Code)
		require.Equal(t, "not found", payload.Message)
		require.Equal(t, "/api/v1/__nonexistent__", payload.Meta["path"])
	})

	t.Run("404_internal_api_is_json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/core/api/__nonexistent__", nil)
		notFound(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var payload apiError
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "NOT_FOUND", payload.Code)
		require.Equal(t, "not found", payload.Message)
		require.Equal(t, "/core/api/__nonexistent__", payload.Meta["path"])
	})

	t.Run("405_public_api_is_json", func(t *testing.T) {
		r := mux.NewRouter()
		r.MethodNotAllowedHandler = methodNotAllowed
		r.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/ping", nil)
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var payload apiError
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "METHOD_NOT_ALLOWED", payload.Code)
		require.Equal(t, "method not allowed", payload.Message)
		require.Equal(t, http.MethodPost, payload.Meta["method"])
		require.Equal(t, "/api/v1/ping", payload.Meta["path"])
	})

	t.Run("405_internal_api_is_json", func(t *testing.T) {
		r := mux.NewRouter()
		r.MethodNotAllowedHandler = methodNotAllowed
		r.HandleFunc("/core/api/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://example.com/core/api/ping", nil)
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var payload apiError
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "METHOD_NOT_ALLOWED", payload.Code)
		require.Equal(t, "method not allowed", payload.Message)
		require.Equal(t, http.MethodPost, payload.Meta["method"])
		require.Equal(t, "/core/api/ping", payload.Meta["path"])
	})
}

func TestAPIErrorContracts_PanicRecovery_IsJSON(t *testing.T) {
	logger := logrus.New()
	opts := middleware.DefaultLoggerOptions()
	opts.Entrypoint = "server"

	h := middleware.WithLogger(logger, opts)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/panic", nil)
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var payload panicError
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, "INTERNAL_SERVER_ERROR", payload.Code)
	require.Equal(t, "internal server error", payload.Message)
	require.Equal(t, "/api/v1/panic", payload.Meta["path"])
	require.NotEmpty(t, payload.Meta["request_id"])
}

type apiError struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Meta    map[string]string `json:"meta"`
}

type panicError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta"`
}
