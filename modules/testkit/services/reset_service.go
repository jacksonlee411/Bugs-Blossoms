package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type ResetService struct {
	app application.Application
}

func NewResetService(app application.Application) *ResetService {
	return &ResetService{
		app: app,
	}
}

const testkitResetLockID int64 = 991337531

func (s *ResetService) TruncateAllTables(ctx context.Context) error {
	logger := composables.UseLogger(ctx)
	db := s.app.DB()

	// Ensure only one reset runs at a time to avoid clobbering in parallel E2E workers
	if _, err := db.Exec(ctx, "SELECT pg_advisory_lock($1)", testkitResetLockID); err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}
	defer func() {
		if _, err := db.Exec(ctx, "SELECT pg_advisory_unlock($1)", testkitResetLockID); err != nil {
			logger.WithError(err).Error("failed to release advisory lock")
		}
	}()

	// Get all table names except migration-related tables
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		AND table_name NOT LIKE '%migration%'
		AND table_name NOT LIKE 'schema_%'
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating table rows: %w", err)
	}

	if len(tables) == 0 {
		logger.Info("No tables found to truncate")
		return nil
	}

	// Begin transaction for atomic truncation
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Disable foreign key checks temporarily
	if _, err := tx.Exec(ctx, "SET session_replication_role = replica;"); err != nil {
		return fmt.Errorf("failed to disable foreign key checks: %w", err)
	}

	// Prepare single TRUNCATE statement to avoid sequential locking deadlocks
	quoted := make([]string, len(tables))
	for i, table := range tables {
		quoted[i] = fmt.Sprintf(`"%s"`, table)
	}
	truncateQuery := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE;", strings.Join(quoted, ", "))
	if _, err := tx.Exec(ctx, truncateQuery); err != nil {
		logger.WithError(err).Error("Failed to truncate tables")
		return fmt.Errorf("failed to truncate tables: %w", err)
	}
	logger.WithField("tables", strings.Join(tables, ", ")).Debug("Truncated tables")

	// Re-enable foreign key checks
	if _, err := tx.Exec(ctx, "SET session_replication_role = DEFAULT;"); err != nil {
		return fmt.Errorf("failed to re-enable foreign key checks: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit truncate transaction: %w", err)
	}

	logger.WithField("tableCount", len(tables)).Info("Successfully truncated all tables")

	return nil
}

func (s *ResetService) CleanUploads(ctx context.Context) error {
	logger := composables.UseLogger(ctx)
	// TODO: Implement file cleanup for test uploads
	// This should clean up files in the uploads directory that were created during tests
	logger.Debug("Upload cleanup not yet implemented")
	return nil
}
