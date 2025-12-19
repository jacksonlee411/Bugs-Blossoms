package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type runOptions struct {
	BaseURL       string
	TenantID      string
	SID           string
	Profile       string
	OutPath       string
	EffectiveDate string
	ParentNodeID  string
	P99LimitMS    int
}

func newRunCmd() *cobra.Command {
	var opts runOptions

	cmd := &cobra.Command{
		Use:   "run --profile <name> --base-url <url> --tenant <uuid> --sid <cookie> --out <path>",
		Short: "Run a load test profile and write org_load_report.v1 JSON",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(opts.BaseURL) == "" {
				return errors.New("--base-url is required")
			}
			if strings.TrimSpace(opts.TenantID) == "" {
				return errors.New("--tenant is required")
			}
			if strings.TrimSpace(opts.SID) == "" {
				return errors.New("--sid is required")
			}
			if strings.TrimSpace(opts.Profile) == "" {
				return errors.New("--profile is required")
			}
			if strings.TrimSpace(opts.OutPath) == "" {
				return errors.New("--out is required")
			}
			if strings.TrimSpace(opts.EffectiveDate) == "" {
				opts.EffectiveDate = time.Now().UTC().Format("2006-01-02")
			}

			p, err := builtinProfile(opts.Profile, opts.EffectiveDate)
			if err != nil {
				return err
			}
			if p.RequiresWrites && strings.TrimSpace(opts.ParentNodeID) == "" {
				return errors.New("--parent-node-id is required for org_mix_read_write")
			}

			client := newHTTPClient()
			if err := smokeCheck(cmd.Context(), client, opts.BaseURL); err != nil {
				return err
			}

			startedAt := time.Now().UTC()
			runID := uuid.NewString()
			stats := newStats()

			ctx, cancel := context.WithTimeout(cmd.Context(), p.Duration)
			defer cancel()

			wg := sync.WaitGroup{}
			wg.Add(p.VUs)
			for i := 0; i < p.VUs; i++ {
				go func(workerID int) {
					defer wg.Done()
					r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
					for {
						select {
						case <-ctx.Done():
							return
						default:
						}
						t := pickTarget(r, p.Targets)
						stats.record(doRequest(ctx, client, opts, t))
					}
				}(i)
			}
			wg.Wait()

			finishedAt := time.Now().UTC()

			backend, cacheEnabled := discoverBackend(cmd.Context(), client, opts.BaseURL, opts.SID)

			report := loadReportV1{
				SchemaVersion: 1,
				RunID:         runID,
				StartedAt:     startedAt.Format(time.RFC3339),
				FinishedAt:    finishedAt.Format(time.RFC3339),
				Results:       stats.results(),
				Notes:         "",
			}
			report.Target.BaseURL = opts.BaseURL
			report.Target.TenantID = opts.TenantID
			report.Profile.Name = p.Name
			report.Profile.VUs = p.VUs
			report.Profile.DurationSeconds = int(p.Duration.Seconds())
			report.Backend.DeepReadBackend = backend
			report.Backend.CacheEnabled = cacheEnabled

			p99Limit := opts.P99LimitMS
			if p99Limit <= 0 {
				p99Limit = p.DefaultP99MS
			}
			report.Thresholds = []loadReportThreshold{
				{
					Name:  "p99_ms",
					Limit: p99Limit,
					OK:    stats.p99All() <= p99Limit,
				},
			}

			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(opts.OutPath, data, 0o644); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Profile, "profile", "", "profile name (org_read_1k|org_read_10k|org_mix_read_write)")
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "http://localhost:3200", "server base URL")
	cmd.Flags().StringVar(&opts.TenantID, "tenant", "", "tenant UUID (for report metadata)")
	cmd.Flags().StringVar(&opts.SID, "sid", "", "session cookie value (sid)")
	cmd.Flags().StringVar(&opts.OutPath, "out", "", "output report path")
	cmd.Flags().StringVar(&opts.EffectiveDate, "effective-date", "", "effective date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.ParentNodeID, "parent-node-id", "", "parent node UUID (required for org_mix_read_write)")
	cmd.Flags().IntVar(&opts.P99LimitMS, "p99-limit-ms", 0, "p99 latency threshold in milliseconds (default per profile)")

	return cmd
}

func smokeCheck(ctx context.Context, client *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("health check failed: status=%d", resp.StatusCode)
	}
	return nil
}

func discoverBackend(ctx context.Context, client *http.Client, baseURL, sid string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/org/api/ops/health", nil)
	if err != nil {
		return "", false
	}
	req.AddCookie(&http.Cookie{Name: "sid", Value: sid})
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return "", false
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false
	}
	checks, _ := payload["checks"].(map[string]any)
	if checks == nil {
		return "", false
	}
	deepRead, _ := checks["deep_read"].(map[string]any)
	cache, _ := checks["cache"].(map[string]any)
	backend := ""
	cacheEnabled := false
	if deepRead != nil {
		if details, _ := deepRead["details"].(map[string]any); details != nil {
			if v, ok := details["backend"].(string); ok {
				backend = v
			}
		}
	}
	if cache != nil {
		if details, _ := cache["details"].(map[string]any); details != nil {
			if v, ok := details["enabled"].(bool); ok {
				cacheEnabled = v
			}
		}
	}
	return backend, cacheEnabled
}

type requestResult struct {
	Endpoint   string
	DurationMS int
	StatusCode int
	Err        error
}

func doRequest(ctx context.Context, client *http.Client, opts runOptions, t target) requestResult {
	url := strings.TrimRight(opts.BaseURL, "/") + t.Path

	var bodyBytes []byte
	if t.Body != nil {
		var err error
		bodyBytes, err = t.Body(&opts)
		if err != nil {
			return requestResult{Endpoint: t.Endpoint, Err: err}
		}
	}

	var bodyReader *bytes.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, t.Method, url, bodyReader)
	if err != nil {
		return requestResult{Endpoint: t.Endpoint, Err: err}
	}
	req.AddCookie(&http.Cookie{Name: "sid", Value: opts.SID})
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-Id", uuid.NewString())
	if t.Method == http.MethodPost || t.Method == http.MethodPatch || t.Method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return requestResult{Endpoint: t.Endpoint, DurationMS: int(time.Since(start).Milliseconds()), Err: err}
	}
	_ = resp.Body.Close()
	return requestResult{Endpoint: t.Endpoint, DurationMS: int(time.Since(start).Milliseconds()), StatusCode: resp.StatusCode}
}

func pickTarget(r *rand.Rand, targets []target) target {
	total := 0
	for _, t := range targets {
		total += t.Weight
	}
	x := r.Intn(total)
	for _, t := range targets {
		x -= t.Weight
		if x < 0 {
			return t
		}
	}
	return targets[len(targets)-1]
}

type endpointStats struct {
	count     int
	errors    int
	latencies []int
}

type stats struct {
	mu        sync.Mutex
	endpoints map[string]*endpointStats
}

func newStats() *stats {
	return &stats{
		endpoints: map[string]*endpointStats{},
	}
}

func (s *stats) record(res requestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es := s.endpoints[res.Endpoint]
	if es == nil {
		es = &endpointStats{latencies: make([]int, 0, 1024)}
		s.endpoints[res.Endpoint] = es
	}
	es.count++
	if res.Err != nil || res.StatusCode >= 400 {
		es.errors++
	}
	if res.DurationMS > 0 {
		es.latencies = append(es.latencies, res.DurationMS)
	}
}

func (s *stats) results() []loadReportResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]loadReportResult, 0, len(s.endpoints))
	for endpoint, es := range s.endpoints {
		p50, p95, p99 := percentiles(es.latencies)
		out = append(out, loadReportResult{
			Endpoint: endpoint,
			Count:    es.count,
			Errors:   es.errors,
			P50MS:    p50,
			P95MS:    p95,
			P99MS:    p99,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Endpoint < out[j].Endpoint })
	return out
}

func (s *stats) p99All() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := make([]int, 0, 4096)
	for _, es := range s.endpoints {
		all = append(all, es.latencies...)
	}
	_, _, p99 := percentiles(all)
	return p99
}

func percentiles(ms []int) (int, int, int) {
	if len(ms) == 0 {
		return 0, 0, 0
	}
	cp := append([]int(nil), ms...)
	sort.Ints(cp)
	p50 := cp[int(float64(len(cp)-1)*0.50)]
	p95 := cp[int(float64(len(cp)-1)*0.95)]
	p99 := cp[int(float64(len(cp)-1)*0.99)]
	return p50, p95, p99
}

func buildBatchDryRunCreateNode(opts *runOptions) ([]byte, error) {
	parentID := strings.TrimSpace(opts.ParentNodeID)
	if parentID == "" {
		return nil, errors.New("parent-node-id is required")
	}
	code := "load_" + uuid.NewString()[:8]
	payload := map[string]any{
		"dry_run":        true,
		"effective_date": opts.EffectiveDate,
		"commands": []map[string]any{
			{
				"type": "node.create",
				"payload": map[string]any{
					"code":           code,
					"name":           "load",
					"parent_id":      parentID,
					"effective_date": opts.EffectiveDate,
				},
			},
		},
	}
	return json.Marshal(payload)
}
