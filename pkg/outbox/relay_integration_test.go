//go:build integration

package outbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type stubDispatcher struct {
	failTopic string
	calls     []DispatchedMessage
}

func (d *stubDispatcher) Dispatch(ctx context.Context, msg DispatchedMessage) error {
	_ = ctx
	d.calls = append(d.calls, msg)
	if msg.Meta.Topic == d.failTopic {
		return errors.New("poison")
	}
	return nil
}

func TestRelay_Integration_NoHeadOfLineBlocking_AndDead(t *testing.T) {
	dsn := os.Getenv("OUTBOX_TEST_DSN")
	if dsn == "" {
		t.Skip("OUTBOX_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	tableName := "outbox_it_" + uuid.NewString()[:8]
	table, err := ParseIdentifier("public." + tableName)
	if err != nil {
		t.Fatalf("parse table: %v", err)
	}

	_, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto;`)
	if err != nil {
		t.Fatalf("create extension: %v", err)
	}

	createSQL := fmt.Sprintf(`
CREATE TABLE %s (
  id           UUID        NOT NULL DEFAULT gen_random_uuid(),
  tenant_id    UUID        NOT NULL,
  topic        TEXT        NOT NULL,
  payload      JSONB       NOT NULL,
  event_id     UUID        NOT NULL,
  sequence     BIGSERIAL   NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NULL,
  attempts     INT         NOT NULL DEFAULT 0,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at    TIMESTAMPTZ NULL,
  last_error   TEXT        NULL,
  CONSTRAINT %s_pkey PRIMARY KEY (id),
  CONSTRAINT %s_event_id_key UNIQUE (event_id),
  CONSTRAINT %s_attempts_nonnegative CHECK (attempts >= 0)
);
`, table.Sanitize(), tableName, tableName, tableName)
	if _, err := pool.Exec(ctx, createSQL); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", table.Sanitize()))
	})

	p := NewPublisher()

	tenantID := uuid.New()
	failTopic := "test.fail.v1"
	okTopic := "test.ok.v1"

	eventFail := uuid.New()
	eventOK := uuid.New()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := p.Enqueue(ctx, tx, table, Message{TenantID: tenantID, Topic: failTopic, EventID: eventFail, Payload: []byte(`{"x":1}`)}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("enqueue fail: %v", err)
	}
	if _, err := p.Enqueue(ctx, tx, table, Message{TenantID: tenantID, Topic: okTopic, EventID: eventOK, Payload: []byte(`{"y":2}`)}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("enqueue ok: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	t.Run("enqueue is idempotent by event_id", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		evt := uuid.New()
		seq1, err := p.Enqueue(ctx, tx, table, Message{TenantID: tenantID, Topic: okTopic, EventID: evt, Payload: []byte(`{"z":3}`)})
		if err != nil {
			t.Fatalf("enqueue 1: %v", err)
		}
		seq2, err := p.Enqueue(ctx, tx, table, Message{TenantID: tenantID, Topic: okTopic, EventID: evt, Payload: []byte(`{"z":3}`)})
		if err != nil {
			t.Fatalf("enqueue 2: %v", err)
		}
		if seq1 != seq2 {
			t.Fatalf("expected same sequence, got %d and %d", seq1, seq2)
		}
	})

	dispatcher := &stubDispatcher{failTopic: failTopic}
	relay, err := NewRelay(pool, table, dispatcher, RelayOptions{
		PollInterval:           10 * time.Millisecond,
		BatchSize:              10,
		LockTTL:                1 * time.Second,
		MaxAttempts:            1, // fail message enters dead on first attempt
		SingleActive:           false,
		LastErrorMaxLen:        1024,
		ObserveQueueDepthEvery: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	if err := relay.processOnce(ctx, nil); err != nil {
		t.Fatalf("processOnce: %v", err)
	}

	if len(dispatcher.calls) != 2 {
		t.Fatalf("expected 2 dispatch calls, got %d", len(dispatcher.calls))
	}

	var publishedOK bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT published_at IS NOT NULL FROM %s WHERE event_id=$1`, table.Sanitize()), eventOK).Scan(&publishedOK); err != nil {
		t.Fatalf("query ok: %v", err)
	}
	if !publishedOK {
		t.Fatalf("expected ok message to be published")
	}

	var attempts int
	var availableAt time.Time
	var lastErr *string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT attempts, available_at, last_error FROM %s WHERE event_id=$1`, table.Sanitize()), eventFail).Scan(&attempts, &availableAt, &lastErr); err != nil {
		t.Fatalf("query fail: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", attempts)
	}
	if lastErr == nil || *lastErr == "" {
		t.Fatalf("expected last_error to be set")
	}
}
