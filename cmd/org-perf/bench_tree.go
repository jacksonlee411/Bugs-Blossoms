package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type perfReport struct {
	Scenario   string  `json:"scenario"`
	Scale      string  `json:"scale"`
	Profile    string  `json:"profile"`
	Backend    string  `json:"backend"`
	P50Ms      float64 `json:"p50_ms"`
	P95Ms      float64 `json:"p95_ms"`
	P99Ms      float64 `json:"p99_ms"`
	Count      int     `json:"count"`
	StartedAt  string  `json:"started_at"`
	FinishedAt string  `json:"finished_at"`
	GitRev     string  `json:"git_rev"`
	DBVersion  string  `json:"db_version"`
}

func newBenchTreeCmd() *cobra.Command {
	var (
		tenantIDStr   string
		scale         string
		seed          int64
		profile       string
		backend       string
		effectiveDate string
		iterations    int
		warmup        int
		concurrency   int
		baseURL       string
		authToken     string
		outputPath    string
	)

	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Benchmark Org tree read",
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID, err := uuid.Parse(strings.TrimSpace(tenantIDStr))
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			_, err = parseScale(scale)
			if err != nil {
				return err
			}
			asOf, err := parseEffectiveDate(effectiveDate)
			if err != nil {
				return fmt.Errorf("invalid --effective-date: %w", err)
			}
			if asOf.IsZero() {
				asOf = time.Now().UTC()
			}

			backend = strings.ToLower(strings.TrimSpace(backend))
			if backend == "" {
				backend = "db"
			}
			switch backend {
			case "db", "api":
			default:
				return fmt.Errorf("unsupported --backend %q (expected db|api)", backend)
			}

			if iterations <= 0 {
				return errors.New("iterations must be positive")
			}
			if warmup < 0 {
				return errors.New("warmup must be non-negative")
			}
			if concurrency <= 0 {
				return errors.New("concurrency must be positive")
			}
			if concurrency != 1 {
				return errors.New("only --concurrency=1 is supported in M1")
			}

			if err := ensureDir(outputPath); err != nil {
				return err
			}

			startedAt := time.Now().UTC()
			var (
				samples   []float64
				dbVersion string
			)
			switch backend {
			case "db":
				pool, err := openBenchPool(context.Background())
				if err != nil {
					return err
				}
				defer pool.Close()

				if err := ensureOrgTablesExist(context.Background(), pool); err != nil {
					return err
				}
				if err := runBenchWarmup(context.Background(), pool, tenantID, asOf, warmup); err != nil {
					return err
				}
				samples, err = runBenchMeasure(context.Background(), pool, tenantID, asOf, iterations)
				if err != nil {
					return err
				}
				dbVersion, _ = detectDBVersion(context.Background(), pool)
			case "api":
				token := strings.TrimSpace(authToken)
				if token == "" {
					token = strings.TrimSpace(os.Getenv("AUTH_TOKEN"))
				}
				if token == "" {
					return errors.New("api backend requires --auth-token or AUTH_TOKEN")
				}
				if err := runBenchWarmupAPI(context.Background(), baseURL, token, asOf, warmup); err != nil {
					return err
				}
				samples, err = runBenchMeasureAPI(context.Background(), baseURL, token, asOf, iterations)
				if err != nil {
					return err
				}
			}
			p50, p95, p99 := percentiles(samples)
			finishedAt := time.Now().UTC()

			report := perfReport{
				Scenario:   "org_tree",
				Scale:      strings.TrimSpace(scale),
				Profile:    strings.ToLower(strings.TrimSpace(profile)),
				Backend:    backend,
				P50Ms:      p50,
				P95Ms:      p95,
				P99Ms:      p99,
				Count:      len(samples),
				StartedAt:  startedAt.Format(time.RFC3339Nano),
				FinishedAt: finishedAt.Format(time.RFC3339Nano),
				GitRev:     detectGitRevision(),
				DBVersion:  dbVersion,
			}

			f, err := os.Create(outputPath)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenantIDStr, "tenant", "00000000-0000-0000-0000-000000000001", "tenant uuid")
	cmd.Flags().StringVar(&scale, "scale", "1k", "dataset scale metadata (e.g. 1k)")
	cmd.Flags().Int64Var(&seed, "seed", 42, "dataset seed metadata")
	cmd.Flags().StringVar(&profile, "profile", "balanced", "dataset profile metadata")
	cmd.Flags().StringVar(&backend, "backend", "db", "backend (db|api)")
	cmd.Flags().StringVar(&effectiveDate, "effective-date", "", "as-of effective date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().IntVar(&iterations, "iterations", 200, "iterations to measure")
	cmd.Flags().IntVar(&warmup, "warmup", 50, "warmup iterations (not measured)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "concurrency level (M1 only supports 1)")
	cmd.Flags().StringVar(&baseURL, "base-url", "http://localhost:3200", "base URL for api backend")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "auth token for api backend (or set AUTH_TOKEN)")
	cmd.Flags().StringVar(&outputPath, "output", "./tmp/org-perf/report.json", "output report path")

	return cmd
}

func openBenchPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg := configuration.Use()
	dsn := strings.TrimSpace(cfg.Database.Opts)
	if dsn == "" {
		return nil, errors.New("missing database dsn")
	}
	return pgxpool.New(ctx, dsn)
}

func detectDBVersion(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var version string
	if err := pool.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

func runBenchWarmup(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, asOf time.Time, warmup int) error {
	for i := 0; i < warmup; i++ {
		if _, err := queryHierarchy(ctx, pool, tenantID, asOf); err != nil {
			return err
		}
	}
	return nil
}

func runBenchMeasure(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, asOf time.Time, iterations int) ([]float64, error) {
	out := make([]float64, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, err := queryHierarchy(ctx, pool, tenantID, asOf)
		dur := time.Since(start)
		if err != nil {
			return nil, err
		}
		out = append(out, float64(dur.Microseconds())/1000.0)
	}
	return out, nil
}

func queryHierarchy(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, asOf time.Time) (int, error) {
	rows, err := pool.Query(ctx, `
	SELECT
		n.id,
		n.code,
		s.name,
		e.parent_node_id,
		e.depth,
		s.display_order,
		s.status
	FROM org_nodes n
	JOIN org_node_slices s
		ON s.tenant_id = n.tenant_id
		AND s.org_node_id = n.id
		AND s.effective_date <= $2
		AND s.end_date > $2
	JOIN org_edges e
		ON e.tenant_id = n.tenant_id
		AND e.child_node_id = n.id
		AND e.hierarchy_type = $3
		AND e.effective_date <= $2
		AND e.end_date > $2
	WHERE n.tenant_id = $1
	ORDER BY e.depth ASC, s.display_order ASC, s.name ASC
	`, tenantID, asOf, "OrgUnit")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var (
			id           any
			code         string
			name         string
			parent       any
			depth        int
			displayOrder int
			status       string
		)
		if err := rows.Scan(&id, &code, &name, &parent, &depth, &displayOrder, &status); err != nil {
			return 0, err
		}
		n++
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}
	return n, nil
}

func buildHierarchiesURL(base string, asOf time.Time) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/org/api/hierarchies"
	q := u.Query()
	q.Set("type", "OrgUnit")
	q.Set("effective_date", asOf.UTC().Format(time.RFC3339))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func runBenchWarmupAPI(ctx context.Context, baseURL, authToken string, asOf time.Time, warmup int) error {
	_, err := runBenchMeasureAPI(ctx, baseURL, authToken, asOf, warmup)
	return err
}

func runBenchMeasureAPI(ctx context.Context, baseURL, authToken string, asOf time.Time, iterations int) ([]float64, error) {
	endpoint, err := buildHierarchiesURL(baseURL, asOf)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	out := make([]float64, 0, iterations)
	for i := 0; i < iterations; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", authToken)

		start := time.Now()
		res, err := client.Do(req)
		dur := time.Since(start)
		if err != nil {
			return nil, err
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("api bench failed: status=%d url=%s", res.StatusCode, endpoint)
		}
		out = append(out, float64(dur.Microseconds())/1000.0)
	}
	return out, nil
}
