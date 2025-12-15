package outbox

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Relay struct {
	pool       *pgxpool.Pool
	table      pgx.Identifier
	dispatcher Dispatcher
	opts       RelayOptions

	lockKey int64

	m          *metrics
	tableLabel string
}

func NewRelay(pool *pgxpool.Pool, table pgx.Identifier, dispatcher Dispatcher, opts RelayOptions) (*Relay, error) {
	if pool == nil {
		return nil, invalidConfig("pool is required")
	}
	if len(table) == 0 {
		return nil, invalidConfig("table is required")
	}
	if dispatcher == nil {
		return nil, invalidConfig("dispatcher is required")
	}

	opts.setDefaults()

	r := &Relay{
		pool:       pool,
		table:      table,
		dispatcher: dispatcher,
		opts:       opts,
		m:          getMetrics(),
		tableLabel: TableLabel(table),
		lockKey:    advisoryLockKey("outbox:" + TableLabel(table)),
	}
	if r.opts.Logger == nil {
		r.opts.Logger = logrusNop()
	}
	return r, nil
}

func (r *Relay) Run(ctx context.Context) error {
	if ctx == nil {
		return invalidConfig("ctx is required")
	}

	if r.opts.SingleActive {
		return r.runSingleActive(ctx)
	}

	r.m.relayLeader.WithLabelValues(r.tableLabel).Set(1)
	return r.runLoop(ctx, nil)
}

func (r *Relay) runSingleActive(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			r.opts.Logger.WithError(err).Warn("outbox: failed to acquire connection for single-active relay")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.opts.PollInterval):
				continue
			}
		}

		leader, err := r.tryAcquireLeader(ctx, conn)
		if err != nil {
			conn.Release()
			r.opts.Logger.WithError(err).Warn("outbox: failed to attempt advisory lock")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.opts.PollInterval):
				continue
			}
		}

		if !leader {
			r.m.relayLeader.WithLabelValues(r.tableLabel).Set(0)
			conn.Release()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.opts.PollInterval):
				continue
			}
		}

		r.m.relayLeader.WithLabelValues(r.tableLabel).Set(1)
		r.opts.Logger.WithField("table", r.tableLabel).Info("outbox: relay became leader")

		err = r.runLoop(ctx, conn)
		_ = r.releaseLeader(context.Background(), conn)
		conn.Release()
		return err
	}
}

func (r *Relay) runLoop(ctx context.Context, conn *pgxpool.Conn) error {
	ticker := time.NewTicker(r.opts.PollInterval)
	defer ticker.Stop()

	nextDepthAt := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if time.Now().After(nextDepthAt) {
			if err := r.observeQueueDepth(ctx, conn); err != nil {
				r.opts.Logger.WithError(err).Debug("outbox: observe queue depth failed")
			}
			nextDepthAt = time.Now().Add(r.opts.ObserveQueueDepthEvery)
		}

		if err := r.processOnce(ctx, conn); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			r.opts.Logger.WithError(err).Warn("outbox: process tick failed")
		}
	}
}

type claimed struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Topic     string
	Payload   []byte
	EventID   uuid.UUID
	Sequence  int64
	Attempts  int
	ClaimedAt time.Time
}

func (r *Relay) processOnce(ctx context.Context, conn *pgxpool.Conn) error {
	now := time.Now()
	cutoff := now.Add(-r.opts.LockTTL)

	claimed, err := r.claim(ctx, conn, now, cutoff)
	if err != nil {
		return err
	}
	if len(claimed) == 0 {
		return nil
	}

	for _, c := range claimed {
		dispatchCtx := ctx
		var cancel func()
		if r.opts.DispatchTimeout > 0 {
			dispatchCtx, cancel = context.WithTimeout(ctx, r.opts.DispatchTimeout)
		}

		start := time.Now()
		err := r.dispatcher.Dispatch(dispatchCtx, DispatchedMessage{
			Meta: Meta{
				Table:    r.table,
				TenantID: c.TenantID,
				Topic:    c.Topic,
				EventID:  c.EventID,
				Sequence: c.Sequence,
				Attempts: c.Attempts,
			},
			Payload: c.Payload,
		})
		if cancel != nil {
			cancel()
		}

		latency := time.Since(start)
		if err == nil {
			r.recordDispatch(c.Topic, "success", latency)
			if ackErr := r.ack(ctx, conn, c.ID); ackErr != nil {
				r.opts.Logger.WithError(ackErr).WithFields(logFields(c, r.tableLabel)).Warn("outbox: ack failed")
			}
			continue
		}

		r.recordDispatch(c.Topic, "failure", latency)
		lastErr := truncateError(err, r.opts.LastErrorMaxLen)

		if c.Attempts >= r.opts.MaxAttempts {
			r.m.deadTotal.WithLabelValues(r.tableLabel, c.Topic).Inc()
			if deadErr := r.dead(ctx, conn, c.ID, lastErr); deadErr != nil {
				r.opts.Logger.WithError(deadErr).WithFields(logFields(c, r.tableLabel)).Warn("outbox: dead update failed")
			}
			continue
		}

		next := time.Now().Add(backoff(c.Attempts, r.opts.MaxBackoff) + jitter(r.opts.Rand, r.opts.JitterMax))
		if nackErr := r.nack(ctx, conn, c.ID, lastErr, next); nackErr != nil {
			r.opts.Logger.WithError(nackErr).WithFields(logFields(c, r.tableLabel)).Warn("outbox: nack failed")
		}
	}

	return nil
}

func (r *Relay) claim(ctx context.Context, conn *pgxpool.Conn, now, lockCutoff time.Time) ([]claimed, error) {
	exec := txExec{pool: r.pool, conn: conn}
	tx, err := exec.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.rollback(ctx)

	tableName := r.table.Sanitize()
	q := fmt.Sprintf(
		`SELECT id, tenant_id, topic, payload, event_id, sequence, attempts
		   FROM %s
		  WHERE published_at IS NULL
		    AND available_at <= $1
		    AND attempts < $2
		    AND (locked_at IS NULL OR locked_at < $3)
		  ORDER BY available_at, sequence
		  LIMIT $4
		  FOR UPDATE SKIP LOCKED`,
		tableName,
	)
	rows, err := tx.tx.Query(ctx, q, now, r.opts.MaxAttempts, lockCutoff, r.opts.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("outbox claim select: %w", err)
	}
	defer rows.Close()

	var items []claimed
	var ids []uuid.UUID
	for rows.Next() {
		var c claimed
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Topic, &c.Payload, &c.EventID, &c.Sequence, &c.Attempts); err != nil {
			return nil, fmt.Errorf("outbox claim scan: %w", err)
		}
		c.Attempts++
		c.ClaimedAt = now
		items = append(items, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox claim rows: %w", err)
	}
	if len(ids) == 0 {
		if err := tx.commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}

	update := fmt.Sprintf(`UPDATE %s SET locked_at = $1, attempts = attempts + 1 WHERE id = ANY($2)`, tableName)
	if _, err := tx.tx.Exec(ctx, update, now, pgtype.FlatArray[uuid.UUID](ids)); err != nil {
		return nil, fmt.Errorf("outbox claim update: %w", err)
	}

	if err := tx.commit(ctx); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Relay) ack(ctx context.Context, conn *pgxpool.Conn, id uuid.UUID) error {
	exec := txExec{pool: r.pool, conn: conn}
	tx, err := exec.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.rollback(ctx)

	tableName := r.table.Sanitize()
	q := fmt.Sprintf(
		`UPDATE %s
		    SET published_at = now(),
		        locked_at = NULL,
		        last_error = NULL
		  WHERE id = $1 AND published_at IS NULL`,
		tableName,
	)
	if _, err := tx.tx.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("outbox ack: %w", err)
	}
	return tx.commit(ctx)
}

func (r *Relay) nack(ctx context.Context, conn *pgxpool.Conn, id uuid.UUID, lastError string, nextAvailable time.Time) error {
	exec := txExec{pool: r.pool, conn: conn}
	tx, err := exec.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.rollback(ctx)

	tableName := r.table.Sanitize()
	q := fmt.Sprintf(
		`UPDATE %s
		    SET locked_at = NULL,
		        last_error = $2,
		        available_at = $3
		  WHERE id = $1 AND published_at IS NULL`,
		tableName,
	)
	if _, err := tx.tx.Exec(ctx, q, id, lastError, nextAvailable); err != nil {
		return fmt.Errorf("outbox nack: %w", err)
	}
	return tx.commit(ctx)
}

func (r *Relay) dead(ctx context.Context, conn *pgxpool.Conn, id uuid.UUID, lastError string) error {
	exec := txExec{pool: r.pool, conn: conn}
	tx, err := exec.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.rollback(ctx)

	tableName := r.table.Sanitize()
	q := fmt.Sprintf(
		`UPDATE %s
		    SET locked_at = NULL,
		        last_error = $2,
		        available_at = now()
		  WHERE id = $1 AND published_at IS NULL`,
		tableName,
	)
	if _, err := tx.tx.Exec(ctx, q, id, lastError); err != nil {
		return fmt.Errorf("outbox dead: %w", err)
	}
	return tx.commit(ctx)
}

func (r *Relay) observeQueueDepth(ctx context.Context, conn *pgxpool.Conn) error {
	exec := txExec{pool: r.pool, conn: conn}
	db := exec.queryer()

	tableName := r.table.Sanitize()
	pendingQ := fmt.Sprintf(`SELECT count(*) FROM %s WHERE published_at IS NULL`, tableName)
	lockedQ := fmt.Sprintf(`SELECT count(*) FROM %s WHERE published_at IS NULL AND locked_at IS NOT NULL`, tableName)

	var pending, locked int64
	if err := db.QueryRow(ctx, pendingQ).Scan(&pending); err != nil {
		return fmt.Errorf("outbox pending count: %w", err)
	}
	if err := db.QueryRow(ctx, lockedQ).Scan(&locked); err != nil {
		return fmt.Errorf("outbox locked count: %w", err)
	}

	r.m.pending.WithLabelValues(r.tableLabel).Set(float64(pending))
	r.m.locked.WithLabelValues(r.tableLabel).Set(float64(locked))
	return nil
}

func (r *Relay) recordDispatch(topic, result string, latency time.Duration) {
	r.m.dispatchTotal.WithLabelValues(r.tableLabel, topic, result).Inc()
	r.m.dispatchLatency.WithLabelValues(r.tableLabel, topic, result).Observe(latency.Seconds())
}

func (r *Relay) tryAcquireLeader(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1::bigint)`, r.lockKey).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *Relay) releaseLeader(ctx context.Context, conn *pgxpool.Conn) error {
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1::bigint)`, r.lockKey).Scan(&ok); err != nil {
		return err
	}
	return nil
}

func advisoryLockKey(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64())
}

type txExec struct {
	pool *pgxpool.Pool
	conn *pgxpool.Conn
}

func (e txExec) begin(ctx context.Context) (*txWrap, error) {
	if e.conn != nil {
		tx, err := e.conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, err
		}
		return &txWrap{tx: tx}, nil
	}
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	return &txWrap{tx: tx}, nil
}

func (e txExec) queryer() queryer {
	if e.conn != nil {
		return e.conn
	}
	return e.pool
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txWrap struct {
	tx pgx.Tx
}

func (t *txWrap) commit(ctx context.Context) error {
	if err := t.tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (t *txWrap) rollback(ctx context.Context) {
	_ = t.tx.Rollback(ctx)
}

func logFields(c claimed, table string) map[string]any {
	return map[string]any{
		"table":     table,
		"topic":     c.Topic,
		"event_id":  c.EventID.String(),
		"tenant_id": c.TenantID.String(),
		"sequence":  c.Sequence,
		"attempts":  c.Attempts,
	}
}
