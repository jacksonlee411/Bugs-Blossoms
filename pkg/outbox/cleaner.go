package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Cleaner struct {
	pool       *pgxpool.Pool
	table      pgx.Identifier
	opts       CleanerOptions
	tableLabel string
}

func NewCleaner(pool *pgxpool.Pool, table pgx.Identifier, opts CleanerOptions) (*Cleaner, error) {
	if pool == nil {
		return nil, invalidConfig("pool is required")
	}
	if len(table) == 0 {
		return nil, invalidConfig("table is required")
	}
	opts.setDefaults()
	if opts.Logger == nil {
		opts.Logger = logrusNop()
	}
	if opts.DeadRetention > 0 && opts.DeadAttemptsThreshold <= 0 {
		return nil, invalidConfig("dead retention requires DeadAttemptsThreshold > 0")
	}
	return &Cleaner{
		pool:       pool,
		table:      table,
		opts:       opts,
		tableLabel: TableLabel(table),
	}, nil
}

func (c *Cleaner) Run(ctx context.Context) error {
	if ctx == nil {
		return invalidConfig("ctx is required")
	}
	if !c.opts.Enabled {
		return nil
	}

	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if err := c.cleanOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			c.opts.Logger.WithError(err).WithField("table", c.tableLabel).Warn("outbox: cleaner tick failed")
		}
	}
}

func (c *Cleaner) cleanOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-c.opts.Retention)

	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tableName := c.table.Sanitize()

	q := fmt.Sprintf(`DELETE FROM %s WHERE published_at IS NOT NULL AND published_at < $1`, tableName)
	if _, err := tx.Exec(ctx, q, cutoff); err != nil {
		return fmt.Errorf("outbox cleaner delete published: %w", err)
	}

	if c.opts.DeadRetention > 0 {
		deadCutoff := time.Now().Add(-c.opts.DeadRetention)
		deadQ := fmt.Sprintf(
			`DELETE FROM %s
			  WHERE published_at IS NULL
			    AND attempts >= $1
			    AND created_at < $2`,
			tableName,
		)
		if _, err := tx.Exec(ctx, deadQ, c.opts.DeadAttemptsThreshold, deadCutoff); err != nil {
			return fmt.Errorf("outbox cleaner delete dead: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}
