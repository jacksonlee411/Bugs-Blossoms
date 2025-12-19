package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type apiError struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Meta    map[string]string `json:"meta,omitempty"`
}

type snapshotItem struct {
	EntityType string          `json:"entity_type"`
	EntityID   uuid.UUID       `json:"entity_id"`
	NewValues  json.RawMessage `json:"new_values"`
}

type snapshotResult struct {
	TenantID      uuid.UUID      `json:"tenant_id"`
	EffectiveDate time.Time      `json:"effective_date"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Includes      []string       `json:"includes"`
	Limit         int            `json:"limit"`
	Items         []snapshotItem `json:"items"`
	NextCursor    *string        `json:"next_cursor"`
}

type orgAPIClient struct {
	baseURL         *url.URL
	authorization   string
	httpClient      *http.Client
	requestIDHeader string
}

func newOrgAPIClient(baseURL, authorization string) (*orgAPIClient, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = configuration.Use().Origin
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, withCode(exitUsage, fmt.Errorf("invalid --base-url: %q", baseURL))
	}
	return &orgAPIClient{
		baseURL:         u,
		authorization:   strings.TrimSpace(authorization),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		requestIDHeader: configuration.Use().RequestIDHeader,
	}, nil
}

func (c *orgAPIClient) doJSON(ctx context.Context, method, path string, query url.Values, reqBody any, out any) (int, *apiError, error) {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return 0, nil, withCode(exitValidation, fmt.Errorf("json marshal request: %w", err))
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return 0, nil, withCode(exitDB, fmt.Errorf("http request: %w", err))
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.requestIDHeader != "" {
		req.Header.Set(c.requestIDHeader, uuid.NewString())
	}
	if c.authorization != "" {
		req.Header.Set("Authorization", c.authorization)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, withCode(exitDB, fmt.Errorf("http do: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, withCode(exitDB, fmt.Errorf("http read: %w", err))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && strings.TrimSpace(apiErr.Code) != "" {
			return resp.StatusCode, &apiErr, nil
		}
		return resp.StatusCode, nil, withCode(exitDB, fmt.Errorf("http status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody))))
	}

	if out == nil {
		return resp.StatusCode, nil, nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return resp.StatusCode, nil, withCode(exitDB, fmt.Errorf("json unmarshal response: %w", err))
	}
	return resp.StatusCode, nil, nil
}

func (c *orgAPIClient) getSnapshotAll(ctx context.Context, effectiveDate string, include []string) (*snapshotResult, error) {
	all := &snapshotResult{
		Items:      []snapshotItem{},
		NextCursor: nil,
	}

	cursor := ""
	for {
		q := url.Values{}
		if strings.TrimSpace(effectiveDate) != "" {
			q.Set("effective_date", strings.TrimSpace(effectiveDate))
		}
		if len(include) > 0 {
			q.Set("include", strings.Join(include, ","))
		}
		q.Set("limit", "10000")
		if strings.TrimSpace(cursor) != "" {
			q.Set("cursor", cursor)
		}

		var page snapshotResult
		_, apiErr, err := c.doJSON(ctx, http.MethodGet, "/org/api/snapshot", q, nil, &page)
		if err != nil {
			return nil, err
		}
		if apiErr != nil {
			return nil, withCode(exitDB, fmt.Errorf("snapshot failed: %s (%s)", apiErr.Message, apiErr.Code))
		}

		if all.TenantID == uuid.Nil {
			all.TenantID = page.TenantID
			all.EffectiveDate = page.EffectiveDate
			all.GeneratedAt = page.GeneratedAt
			all.Includes = page.Includes
			all.Limit = page.Limit
		}
		all.Items = append(all.Items, page.Items...)

		if page.NextCursor == nil || strings.TrimSpace(*page.NextCursor) == "" {
			all.NextCursor = nil
			break
		}
		cursor = *page.NextCursor
	}
	return all, nil
}

func (c *orgAPIClient) postBatch(ctx context.Context, req qualityBatchRequest) (qualityFixResults, error) {
	var out struct {
		DryRun         bool                        `json:"dry_run"`
		Results        []qualityBatchCommandResult `json:"results"`
		EventsEnqueued int                         `json:"events_enqueued"`
	}

	_, apiErr, err := c.doJSON(ctx, http.MethodPost, "/org/api/batch", nil, req, &out)
	if err != nil {
		return qualityFixResults{Ok: false, Error: nil}, err
	}
	if apiErr != nil {
		return qualityFixResults{
			Ok:             false,
			EventsEnqueued: 0,
			BatchResults:   nil,
			Error:          apiErr,
		}, nil
	}
	return qualityFixResults{
		Ok:             true,
		EventsEnqueued: out.EventsEnqueued,
		BatchResults:   out.Results,
		Error:          nil,
	}, nil
}

func (c *orgAPIClient) getChangeRequest(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
	if id == uuid.Nil {
		return nil, withCode(exitUsage, fmt.Errorf("--change-request-id is invalid"))
	}
	var out struct {
		Payload json.RawMessage `json:"payload"`
	}
	_, apiErr, err := c.doJSON(ctx, http.MethodGet, "/org/api/change-requests/"+id.String(), nil, nil, &out)
	if err != nil {
		return nil, err
	}
	if apiErr != nil {
		return nil, withCode(exitDB, fmt.Errorf("change-request get failed: %s (%s)", apiErr.Message, apiErr.Code))
	}
	return out.Payload, nil
}

func (c *orgAPIClient) postPreflight(ctx context.Context, effectiveDate string, commands []qualityBatchCommand) (json.RawMessage, error) {
	req := struct {
		EffectiveDate string                `json:"effective_date"`
		Commands      []qualityBatchCommand `json:"commands"`
	}{
		EffectiveDate: strings.TrimSpace(effectiveDate),
		Commands:      commands,
	}

	var raw json.RawMessage
	_, apiErr, err := c.doJSON(ctx, http.MethodPost, "/org/api/preflight", nil, req, &raw)
	if err != nil {
		return nil, err
	}
	if apiErr != nil {
		return nil, withCode(exitDB, fmt.Errorf("preflight failed: %s (%s)", apiErr.Message, apiErr.Code))
	}
	return raw, nil
}
