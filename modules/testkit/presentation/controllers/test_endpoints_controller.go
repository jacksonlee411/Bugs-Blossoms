package controllers

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/modules/testkit/domain/schemas"
	"github.com/iota-uz/iota-sdk/modules/testkit/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type TestEndpointsController struct {
	app         application.Application
	testService *services.TestDataService
}

func NewTestEndpointsController(app application.Application) application.Controller {
	return &TestEndpointsController{
		app:         app,
		testService: services.NewTestDataService(app),
	}
}

func (c *TestEndpointsController) Key() string {
	return "/__test__"
}

func (c *TestEndpointsController) Register(r *mux.Router) {
	// Additional safety check in middleware
	r.Use(c.testEndpointsMiddleware)

	// Reset endpoint - truncates all data
	r.HandleFunc("/__test__/reset", c.handleReset).Methods(http.MethodPost)

	// Populate endpoint - accepts JSON data specification
	r.HandleFunc("/__test__/populate", c.handlePopulate).Methods(http.MethodPost)

	// Seed endpoint - applies preset scenarios
	r.HandleFunc("/__test__/seed", c.handleSeed).Methods(http.MethodPost)
	r.HandleFunc("/__test__/seed", c.handleListSeedScenarios).Methods(http.MethodGet)

	// Utility endpoint - returns a deterministic error response for UI/e2e testing
	r.HandleFunc("/__test__/http_error", c.handleHTTPError).Methods(http.MethodGet)

	// Health check for test endpoints
	r.HandleFunc("/__test__/health", c.handleHealth).Methods(http.MethodGet)
}

func (c *TestEndpointsController) testEndpointsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conf := configuration.Use()
		logger := composables.UseLogger(r.Context())

		if !conf.EnableTestEndpoints {
			logger.Warn("Test endpoints accessed but not enabled")
			http.Error(w, "Test endpoints not enabled", http.StatusNotFound)
			return
		}

		logger.Debug("Test endpoint accessed: " + r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (c *TestEndpointsController) handleReset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := composables.UseLogger(ctx)

	type resetRequest struct {
		ReseedMinimal bool `json:"reseedMinimal,omitempty"`
	}

	var req resetRequest
	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
		}
	}

	logger.Warn("Resetting test database")

	if err := c.testService.ResetDatabase(ctx, req.ReseedMinimal); err != nil {
		logger.WithError(err).Error("Failed to reset database")
		http.Error(w, "Failed to reset database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":       true,
		"message":       "Database reset successfully",
		"reseedMinimal": req.ReseedMinimal,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

func (c *TestEndpointsController) handlePopulate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := composables.UseLogger(ctx)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	req, err := schemas.ParsePopulateRequest(body)
	if err != nil {
		logger.WithError(err).Error("Failed to parse populate request")
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	logger.WithField("version", req.Version).Info("Populating test data")

	result, err := c.testService.PopulateData(ctx, req)
	if err != nil {
		logger.WithError(err).Error("Failed to populate data")
		response := schemas.PopulateResponse{
			Success: false,
			Error:   err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
			logger.WithError(encodeErr).Error("Failed to encode error response")
		}
		return
	}

	response := schemas.PopulateResponse{
		Success: true,
		Message: "Data populated successfully",
		Data:    result,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

func (c *TestEndpointsController) handleSeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := composables.UseLogger(ctx)

	type seedRequest struct {
		Scenario string `json:"scenario"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req seedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Scenario == "" {
		req.Scenario = "minimal"
	}

	logger.WithField("scenario", req.Scenario).Info("Seeding test data")

	if err := c.testService.SeedScenario(ctx, req.Scenario); err != nil {
		logger.WithError(err).Error("Failed to seed scenario")
		http.Error(w, "Failed to seed scenario: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":  true,
		"message":  "Scenario seeded successfully",
		"scenario": req.Scenario,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

func (c *TestEndpointsController) handleHTTPError(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	logger := composables.UseLogger(r.Context())

	status := http.StatusInternalServerError
	if rawStatus := strings.TrimSpace(query.Get("status")); rawStatus != "" {
		if parsed, err := strconv.Atoi(rawStatus); err == nil {
			status = parsed
		}
	}
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}

	format := strings.ToLower(strings.TrimSpace(query.Get("format")))
	if format == "" {
		format = "json"
	}

	code := strings.TrimSpace(query.Get("code"))
	if code == "" {
		code = "TEST_HTTP_ERROR"
	}

	message := strings.TrimSpace(query.Get("message"))
	if message == "" {
		message = "Test endpoint error"
	}

	switch format {
	case "text":
		http.Error(w, message, status)
		return
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<div data-testid="test-http-error">` + html.EscapeString(message) + `</div>`))
		return
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"code":    code,
			"message": message,
		}); err != nil {
			logger.WithError(err).Error("Failed to encode http_error response")
		}
		return
	}
}

func (c *TestEndpointsController) handleListSeedScenarios(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := composables.UseLogger(ctx)
	scenarios := c.testService.GetAvailableScenarios()

	response := map[string]interface{}{
		"success":   true,
		"scenarios": scenarios,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

func (c *TestEndpointsController) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := composables.UseLogger(ctx)
	conf := configuration.Use()

	response := map[string]interface{}{
		"success": true,
		"message": "Test endpoints are healthy",
		"config": map[string]interface{}{
			"enableTestEndpoints": conf.EnableTestEndpoints,
			"environment":         conf.GoAppEnvironment,
			"database": map[string]interface{}{
				"host": conf.Database.Host,
				"port": conf.Database.Port,
				"name": conf.Database.Name,
				"user": conf.Database.User,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}
