package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/scripts/authz/internal/common"
	"github.com/iota-uz/iota-sdk/scripts/authz/internal/legacy"
)

type policyEntry struct {
	raw     string
	sortKey string
	kind    string
}

func main() {
	var (
		dsn             = flag.String("dsn", "", "PostgreSQL DSN to export from (defaults to configuration database options)")
		output          = flag.String("out", "", "Output .gz file path (defaults to tmp/policy_export_<ts>.csv.gz)")
		hashSubjects    = flag.Bool("hash-subjects", true, "Hash user identifiers in exported policy rows")
		requiredEnv     = flag.String("allowed-env", "production_export", "Value required in ALLOWED_ENV to run this command")
		dryRun          = flag.Bool("dry-run", false, "Print summary without writing a file")
		connectionLimit = flag.Int("pool-size", 4, "Maximum connections for the export session")
	)
	flag.Parse()

	if env := os.Getenv("ALLOWED_ENV"); env != *requiredEnv {
		fmt.Fprintf(os.Stderr, "export blocked: ALLOWED_ENV=%s (expected %s)\n", env, *requiredEnv)
		os.Exit(1)
	}

	cfg := configuration.Use()
	if *dsn == "" {
		*dsn = cfg.Database.Opts
	}
	if *output == "" {
		if err := os.MkdirAll("tmp", 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create tmp directory: %v\n", err)
			os.Exit(1)
		}
		*output = filepath.Join("tmp", fmt.Sprintf("policy_export_%d.csv.gz", time.Now().Unix()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	pool.Config().MaxConns = int32(*connectionLimit)

	snapshot, err := legacy.LoadSnapshot(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load snapshot: %v\n", err)
		os.Exit(1)
	}

	entries := buildPolicyEntries(snapshot, *hashSubjects)
	if *dryRun {
		reportSummary(entries)
		return
	}
	if err := writeCompressed(*output, entries); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write export: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "exported %d policy lines to %s\n", len(entries), *output)
}

func buildPolicyEntries(snapshot *legacy.Snapshot, hashSubjects bool) []policyEntry {
	dedup := map[string]struct{}{}
	var entries []policyEntry

	appendEntry := func(kind, raw, key string) {
		if _, exists := dedup[key]; exists {
			return
		}
		dedup[key] = struct{}{}
		entries = append(entries, policyEntry{
			raw:     raw,
			sortKey: key,
			kind:    kind,
		})
	}

	for roleID, permIDs := range snapshot.RolePermissions {
		role, ok := snapshot.Roles[roleID]
		if !ok {
			continue
		}
		domain := authz.DomainFromTenant(role.TenantID)
		roleSubject := common.RoleSubject(role.TenantID, role.Name)
		for _, permID := range permIDs {
			perm, ok := snapshot.Permissions[permID]
			if !ok {
				continue
			}
			module := common.ModuleForPermission(perm.Name, perm.Resource)
			object := authz.ObjectName(module, perm.Resource)
			action := authz.NormalizeAction(perm.Action)
			raw := fmt.Sprintf("p, %s, %s, %s, %s, allow", roleSubject, domain, object, action)
			key := strings.Join([]string{"p", roleSubject, domain, object, action}, "|")
			appendEntry("p", raw, key)
		}
	}

	for userID, roleIDs := range snapshot.UserRoles {
		user, ok := snapshot.Users[userID]
		if !ok {
			continue
		}
		domain := authz.DomainFromTenant(user.TenantID)
		userIdentifier := strconv.FormatInt(userID, 10)
		if hashSubjects {
			userIdentifier = common.HashID(userIdentifier, true)
		}
		subject := authz.SubjectForUserID(user.TenantID, userIdentifier)
		for _, roleID := range roleIDs {
			role, ok := snapshot.Roles[roleID]
			if !ok {
				continue
			}
			roleSubject := common.RoleSubject(role.TenantID, role.Name)
			raw := fmt.Sprintf("g, %s, %s, %s", subject, roleSubject, domain)
			key := strings.Join([]string{"g", subject, roleSubject, domain}, "|")
			appendEntry("g", raw, key)
		}
	}

	for userID, permIDs := range snapshot.UserPermissions {
		user, ok := snapshot.Users[userID]
		if !ok {
			continue
		}
		domain := authz.DomainFromTenant(user.TenantID)
		userIdentifier := strconv.FormatInt(userID, 10)
		if hashSubjects {
			userIdentifier = common.HashID(userIdentifier, true)
		}
		subject := authz.SubjectForUserID(user.TenantID, userIdentifier)

		for _, permID := range permIDs {
			perm, ok := snapshot.Permissions[permID]
			if !ok {
				continue
			}
			module := common.ModuleForPermission(perm.Name, perm.Resource)
			object := authz.ObjectName(module, perm.Resource)
			action := authz.NormalizeAction(perm.Action)
			raw := fmt.Sprintf("p, %s, %s, %s, %s, allow", subject, domain, object, action)
			key := strings.Join([]string{"p", subject, domain, object, action}, "|")
			appendEntry("p", raw, key)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sortKey < entries[j].sortKey
	})
	return entries
}

func writeCompressed(path string, entries []policyEntry) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gzw := gzip.NewWriter(f)
	defer func() {
		if cerr := gzw.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	writer := bufio.NewWriter(gzw)
	defer func() {
		if cerr := writer.Flush(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	if _, err = writer.WriteString("# Legacy export generated at "); err != nil {
		return err
	}
	if _, err = writer.WriteString(time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err = writer.WriteString("\n"); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err = writer.WriteString(entry.raw); err != nil {
			return err
		}
		if err = writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	return nil
}

func reportSummary(entries []policyEntry) {
	var policies, relations int
	for _, entry := range entries {
		switch entry.kind {
		case "p":
			policies++
		case "g":
			relations++
		}
	}
	fmt.Fprintf(os.Stdout, "dry-run summary: %d policy rows (p) and %d role bindings (g)\n", policies, relations)
}
