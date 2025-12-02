package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/scripts/authz/internal/common"
	"github.com/iota-uz/iota-sdk/scripts/authz/internal/legacy"
)

type fixtureCase struct {
	Subject string `yaml:"subject"`
	Domain  string `yaml:"domain"`
	Object  string `yaml:"object"`
	Action  string `yaml:"action"`
	Legacy  bool   `yaml:"legacy"`
	Note    string `yaml:"note,omitempty"`
}

type mismatch struct {
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Legacy  bool   `json:"legacy"`
	Casbin  bool   `json:"casbin"`
	Reason  string `json:"reason"`
}

func main() {
	var (
		dsn           = flag.String("dsn", "", "PostgreSQL DSN for sampling (optional when using fixtures)")
		sampleRatio   = flag.Float64("sample", 0.2, "Sample ratio per tenant (0-1)")
		fixturesPath  = flag.String("fixtures", "", "Optional YAML fixture file for deterministic parity checks")
		emitMetrics   = flag.Bool("emit-metrics", false, "Print parity metrics as JSON")
		maxPerTenant  = flag.Int("max-per-tenant", 500, "Maximum users sampled per tenant")
		minPerTenant  = flag.Int("min-per-tenant", 50, "Minimum users sampled per tenant when population allows")
		connectionLim = flag.Int("pool-size", 4, "Maximum DB connections when sampling from database")
	)
	flag.Parse()

	if fixtures := strings.TrimSpace(*fixturesPath); fixtures != "" {
		if err := runFixtureParity(fixtures); err != nil {
			fmt.Fprintf(os.Stderr, "fixture parity failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "fixture parity passed")
		return
	}

	cfg := configuration.Use()
	if *dsn == "" {
		*dsn = cfg.Database.Opts
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	pool.Config().MaxConns = int32(*connectionLim)

	snapshot, err := legacy.LoadSnapshot(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load snapshot: %v\n", err)
		os.Exit(1)
	}

	randomizer := rand.New(rand.NewSource(time.Now().UnixNano()))
	mismatches, total := compareSnapshot(ctx, snapshot, *sampleRatio, *minPerTenant, *maxPerTenant, randomizer)
	if *emitMetrics {
		payload, err := json.Marshal(map[string]any{
			"total_checked": total,
			"mismatches":    len(mismatches),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal metrics: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s\n", payload)
	}
	if len(mismatches) > 0 {
		for _, diff := range mismatches {
			fmt.Fprintf(os.Stderr, "mismatch: %+v\n", diff)
		}
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "parity ok: checked %d combinations\n", total)
}

func runFixtureParity(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fixtures []fixtureCase
	if err := yaml.Unmarshal(data, &fixtures); err != nil {
		return fmt.Errorf("parse fixtures: %w", err)
	}

	svc := authz.Use()
	for _, fx := range fixtures {
		req := authz.NewRequest(fx.Subject, fx.Domain, fx.Object, fx.Action)
		allowed, err := svc.Check(context.Background(), req)
		if err != nil {
			return err
		}
		if allowed != fx.Legacy {
			return fmt.Errorf("fixture mismatch [%s]: legacy=%t casbin=%t", fx.Note, fx.Legacy, allowed)
		}
	}
	return nil
}

func compareSnapshot(ctx context.Context, snapshot *legacy.Snapshot, ratio float64, minSample, maxSample int, randomizer *rand.Rand) ([]mismatch, int) {
	svc := authz.Use()
	usersByTenant := map[string][]legacy.User{}
	for _, user := range snapshot.Users {
		key := user.TenantID.String()
		usersByTenant[key] = append(usersByTenant[key], user)
	}

	var (
		result []mismatch
		total  int
	)

	for _, users := range usersByTenant {
		if len(users) == 0 {
			continue
		}
		sampled := sampleUsers(users, ratio, minSample, maxSample, randomizer)
		for _, user := range sampled {
			legacyMap := buildLegacyPermissionSet(user, snapshot)
			for permID, perm := range snapshot.Permissions {
				legacyAllowed := legacyMap[permID]
				object := authz.ObjectName(common.ModuleForPermission(perm.Name, perm.Resource), perm.Resource)
				action := authz.NormalizeAction(perm.Action)
				subject := authz.SubjectForUserID(user.TenantID, strconv.FormatInt(user.ID, 10))
				domain := authz.DomainFromTenant(user.TenantID)
				req := authz.NewRequest(subject, domain, object, action)
				authorized, err := svc.Check(ctx, req)
				if err != nil {
					result = append(result, mismatch{
						Subject: subject,
						Domain:  domain,
						Object:  object,
						Action:  action,
						Legacy:  legacyAllowed,
						Casbin:  authorized,
						Reason:  err.Error(),
					})
					continue
				}
				if authorized != legacyAllowed {
					result = append(result, mismatch{
						Subject: subject,
						Domain:  domain,
						Object:  object,
						Action:  action,
						Legacy:  legacyAllowed,
						Casbin:  authorized,
						Reason:  "decision mismatch",
					})
				}
				total++
			}
		}
	}
	return result, total
}

func sampleUsers(users []legacy.User, ratio float64, minSample, maxSample int, randomizer *rand.Rand) []legacy.User {
	shuffled := append([]legacy.User(nil), users...)
	randomizer.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	count := int(math.Ceil(float64(len(users)) * ratio))
	if count > len(users) {
		count = len(users)
	}
	if count < minSample && len(users) >= minSample {
		count = minSample
	}
	if count > maxSample {
		count = maxSample
	}
	if count == 0 && len(users) > 0 {
		count = 1
	}
	return shuffled[:count]
}

func buildLegacyPermissionSet(user legacy.User, snapshot *legacy.Snapshot) map[uuid.UUID]bool {
	set := map[uuid.UUID]bool{}
	for _, permID := range snapshot.UserPermissions[user.ID] {
		set[permID] = true
	}
	for _, roleID := range snapshot.UserRoles[user.ID] {
		for _, permID := range snapshot.RolePermissions[roleID] {
			set[permID] = true
		}
	}
	return set
}
