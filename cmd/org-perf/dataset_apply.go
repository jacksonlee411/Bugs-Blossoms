package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type perfDataset struct {
	TenantID  uuid.UUID
	AsOf      time.Time
	Profile   string
	Scale     int
	Seed      int64
	Nodes     []perfNode
	Positions []perfPosition
	Assigns   []perfAssignment
}

type perfNode struct {
	ID           uuid.UUID
	Code         string
	Name         string
	ParentID     *uuid.UUID
	DisplayOrder int
}

type perfPosition struct {
	ID        uuid.UUID
	OrgNodeID uuid.UUID
	Code      string
}

type perfAssignment struct {
	PositionID     uuid.UUID
	SubjectID      uuid.UUID
	Pernr          string
	AssignmentType string
	IsPrimary      bool
}

func newDatasetApplyCmd() *cobra.Command {
	var (
		tenantIDStr string
		scale       string
		seed        int64
		profile     string
		backend     string
		apply       bool
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Generate and (optionally) import a deterministic dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID, err := uuid.Parse(strings.TrimSpace(tenantIDStr))
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			count, err := parseScale(scale)
			if err != nil {
				return err
			}
			profile = strings.ToLower(strings.TrimSpace(profile))
			if profile == "" {
				profile = "balanced"
			}

			backend = strings.ToLower(strings.TrimSpace(backend))
			if backend == "" {
				backend = "db"
			}
			if backend != "db" {
				return fmt.Errorf("unsupported --backend %q (only db is implemented)", backend)
			}

			asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			ds, err := buildDataset(tenantID, asOf, profile, count, seed)
			if err != nil {
				return err
			}

			summary := map[string]any{
				"tenant_id":      ds.TenantID.String(),
				"effective_date": ds.AsOf.Format(time.RFC3339),
				"profile":        ds.Profile,
				"scale":          ds.Scale,
				"seed":           ds.Seed,
				"nodes":          len(ds.Nodes),
				"positions":      len(ds.Positions),
				"assignments":    len(ds.Assigns),
				"apply":          apply,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(summary); err != nil {
				return err
			}

			if !apply {
				return nil
			}

			pool, err := openPool(context.Background())
			if err != nil {
				return err
			}
			defer pool.Close()

			if err := ensureOrgTablesExist(context.Background(), pool); err != nil {
				return err
			}
			if err := ensurePersonTablesExist(context.Background(), pool); err != nil {
				return err
			}
			if err := ensureTenantExists(context.Background(), pool, tenantID); err != nil {
				return err
			}
			if err := ensureOrgTenantEmpty(context.Background(), pool, tenantID); err != nil {
				return err
			}

			return importDataset(context.Background(), pool, ds)
		},
	}

	cmd.Flags().StringVar(&tenantIDStr, "tenant", "00000000-0000-0000-0000-000000000001", "tenant uuid")
	cmd.Flags().StringVar(&scale, "scale", "1k", "dataset scale (e.g. 1k)")
	cmd.Flags().Int64Var(&seed, "seed", 42, "random seed")
	cmd.Flags().StringVar(&profile, "profile", "balanced", "dataset profile (balanced|wide|deep)")
	cmd.Flags().StringVar(&backend, "backend", "db", "backend (db|api)")
	cmd.Flags().BoolVar(&apply, "apply", false, "apply to database (default dry-run)")

	return cmd
}

func openPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg := configuration.Use()
	dsn := strings.TrimSpace(cfg.Database.Opts)
	if dsn == "" {
		return nil, errors.New("missing database dsn")
	}
	return pgxpool.New(ctx, dsn)
}

func ensureOrgTablesExist(ctx context.Context, pool *pgxpool.Pool) error {
	var ok bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.org_nodes') IS NOT NULL").Scan(&ok); err != nil {
		return err
	}
	if ok {
		return nil
	}
	return errors.New("org schema not found; run `make org migrate up` first")
}

func ensurePersonTablesExist(ctx context.Context, pool *pgxpool.Pool) error {
	var ok bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.persons') IS NOT NULL").Scan(&ok); err != nil {
		return err
	}
	if ok {
		return nil
	}
	return errors.New("person schema not found; run `PERSON_MIGRATIONS=1 make db migrate up` first")
}

func ensureTenantExists(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) error {
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tenants WHERE id=$1)", tenantID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	var hasName bool
	if err := pool.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='tenants' AND column_name='name'
		)`,
	).Scan(&hasName); err != nil {
		return err
	}

	if hasName {
		name := "Org Perf Tenant " + tenantID.String()[:8]
		domain := tenantID.String()[:8] + ".org-perf.local"
		_, err := pool.Exec(
			ctx,
			`INSERT INTO tenants (id, name, domain, is_active, created_at, updated_at)
			 VALUES ($1,$2,$3,TRUE,NOW(),NOW())
			 ON CONFLICT (id) DO NOTHING`,
			tenantID,
			name,
			domain,
		)
		return err
	}

	_, err := pool.Exec(ctx, "INSERT INTO tenants (id) VALUES ($1) ON CONFLICT (id) DO NOTHING", tenantID)
	return err
}

func ensureOrgTenantEmpty(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) error {
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM org_nodes WHERE tenant_id=$1 LIMIT 1)", tenantID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("org_nodes already exists for tenant %s; use a dedicated empty tenant", tenantID)
	}
	return nil
}

func buildDataset(tenantID uuid.UUID, asOf time.Time, profile string, count int, seed int64) (*perfDataset, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("tenant_id is required")
	}
	if count <= 0 {
		return nil, errors.New("scale must be positive")
	}

	nodes, err := buildNodes(tenantID, profile, count, seed)
	if err != nil {
		return nil, err
	}

	positions := make([]perfPosition, 0, len(nodes))
	for _, node := range nodes {
		posID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:pos:%s", tenantID, node.ID)))
		positions = append(positions, perfPosition{
			ID:        posID,
			OrgNodeID: node.ID,
			Code:      fmt.Sprintf("POS-%s", node.Code),
		})
	}

	assignCount := len(nodes)
	if assignCount > 100 {
		assignCount = 100
	}
	assignments := make([]perfAssignment, 0, assignCount)
	for i := 0; i < assignCount; i++ {
		pernr := fmt.Sprintf("%06d", i+1)
		personUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:person_uuid:%s", tenantID, pernr)))
		assignments = append(assignments, perfAssignment{
			PositionID:     positions[i].ID,
			SubjectID:      personUUID,
			Pernr:          pernr,
			AssignmentType: "primary",
			IsPrimary:      true,
		})
	}

	return &perfDataset{
		TenantID:  tenantID,
		AsOf:      asOf,
		Profile:   profile,
		Scale:     count,
		Seed:      seed,
		Nodes:     nodes,
		Positions: positions,
		Assigns:   assignments,
	}, nil
}

func buildNodes(tenantID uuid.UUID, profile string, count int, seed int64) ([]perfNode, error) {
	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:org-perf:%d", tenantID, seed)))

	nodes := make([]perfNode, 0, count)
	rootID := uuid.NewSHA1(namespace, []byte("node:0"))
	nodes = append(nodes, perfNode{
		ID:           rootID,
		Code:         "D0000",
		Name:         "D0000",
		ParentID:     nil,
		DisplayOrder: 0,
	})
	if count == 1 {
		return nodes, nil
	}

	switch profile {
	case "balanced":
		maxChildren := 4
		queue := []uuid.UUID{rootID}
		children := map[uuid.UUID]int{}
		siblingOrder := map[uuid.UUID]int{}
		for i := 1; i < count; i++ {
			parent := queue[0]
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			order := siblingOrder[parent]
			siblingOrder[parent] = order + 1
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				Name:         fmt.Sprintf("D%04d", i),
				ParentID:     &parent,
				DisplayOrder: order,
			})
			queue = append(queue, nodeID)
			children[parent]++
			if children[parent] >= maxChildren {
				queue = queue[1:]
			}
		}
		return nodes, nil
	case "wide":
		k := 40
		if k > count-1 {
			k = count - 1
		}
		siblingOrder := 0
		level1 := make([]uuid.UUID, 0, k)
		for i := 1; i <= k; i++ {
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			parent := rootID
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				Name:         fmt.Sprintf("D%04d", i),
				ParentID:     &parent,
				DisplayOrder: siblingOrder,
			})
			siblingOrder++
			level1 = append(level1, nodeID)
		}
		remaining := count - 1 - k
		if remaining <= 0 {
			return nodes, nil
		}
		j := k + 1
		for remaining > 0 {
			for _, parent := range level1 {
				if remaining <= 0 {
					break
				}
				nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", j)))
				nodes = append(nodes, perfNode{
					ID:           nodeID,
					Code:         fmt.Sprintf("D%04d", j),
					Name:         fmt.Sprintf("D%04d", j),
					ParentID:     &parent,
					DisplayOrder: 0,
				})
				j++
				remaining--
			}
		}
		return nodes, nil
	case "deep":
		parent := rootID
		for i := 1; i < count; i++ {
			nodeID := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("node:%d", i)))
			nodes = append(nodes, perfNode{
				ID:           nodeID,
				Code:         fmt.Sprintf("D%04d", i),
				Name:         fmt.Sprintf("D%04d", i),
				ParentID:     &parent,
				DisplayOrder: 0,
			})
			parent = nodeID
		}
		return nodes, nil
	default:
		return nil, fmt.Errorf("unknown profile: %s", profile)
	}
}

func importDataset(ctx context.Context, pool *pgxpool.Pool, ds *perfDataset) error {
	if ds == nil {
		return errors.New("dataset is nil")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "org_nodes"},
		[]string{"tenant_id", "id", "type", "code", "is_root"},
		pgx.CopyFromSlice(len(ds.Nodes), func(i int) ([]any, error) {
			n := ds.Nodes[i]
			return []any{ds.TenantID, n.ID, "OrgUnit", n.Code, n.ParentID == nil}, nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "org_node_slices"},
		[]string{"tenant_id", "org_node_id", "name", "i18n_names", "status", "display_order", "parent_hint", "effective_date", "end_date"},
		pgx.CopyFromSlice(len(ds.Nodes), func(i int) ([]any, error) {
			n := ds.Nodes[i]
			return []any{
				ds.TenantID,
				n.ID,
				n.Name,
				[]byte(`{}`),
				"active",
				n.DisplayOrder,
				n.ParentID,
				ds.AsOf,
				endDate,
			}, nil
		}),
	)
	if err != nil {
		return err
	}

	edges := make([]perfNode, 0, len(ds.Nodes))
	edges = append(edges, ds.Nodes...)
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "org_edges"},
		[]string{"tenant_id", "hierarchy_type", "parent_node_id", "child_node_id", "effective_date", "end_date"},
		pgx.CopyFromSlice(len(edges), func(i int) ([]any, error) {
			n := edges[i]
			return []any{ds.TenantID, "OrgUnit", n.ParentID, n.ID, ds.AsOf, endDate}, nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "org_positions"},
		[]string{"tenant_id", "id", "org_node_id", "code", "status", "is_auto_created", "effective_date", "end_date"},
		pgx.CopyFromSlice(len(ds.Positions), func(i int) ([]any, error) {
			p := ds.Positions[i]
			return []any{ds.TenantID, p.ID, p.OrgNodeID, p.Code, "active", false, ds.AsOf, endDate}, nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "persons"},
		[]string{"tenant_id", "person_uuid", "pernr", "display_name", "status"},
		pgx.CopyFromSlice(len(ds.Assigns), func(i int) ([]any, error) {
			a := ds.Assigns[i]
			return []any{ds.TenantID, a.SubjectID, a.Pernr, "Perf " + a.Pernr, "active"}, nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "org_assignments"},
		[]string{"tenant_id", "position_id", "subject_type", "subject_id", "pernr", "assignment_type", "is_primary", "effective_date", "end_date"},
		pgx.CopyFromSlice(len(ds.Assigns), func(i int) ([]any, error) {
			a := ds.Assigns[i]
			return []any{ds.TenantID, a.PositionID, "person", a.SubjectID, a.Pernr, a.AssignmentType, a.IsPrimary, ds.AsOf, endDate}, nil
		}),
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
