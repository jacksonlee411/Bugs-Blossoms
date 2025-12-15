package outbox

import (
	"context"
	"fmt"

	"github.com/iota-uz/iota-sdk/pkg/repo"
	"github.com/jackc/pgx/v5"
)

type Publisher interface {
	Enqueue(ctx context.Context, tx repo.Tx, table pgx.Identifier, msg Message) (sequence int64, err error)
}

type publisher struct {
	m *metrics
}

func NewPublisher() Publisher {
	return &publisher{m: getMetrics()}
}

func (p *publisher) Enqueue(ctx context.Context, tx repo.Tx, table pgx.Identifier, msg Message) (int64, error) {
	if msg.TenantID == uuidZero() {
		return 0, fmt.Errorf("%w: tenant_id is required", ErrInvalidConfig)
	}
	if msg.EventID == uuidZero() {
		return 0, fmt.Errorf("%w: event_id is required", ErrInvalidConfig)
	}
	if msg.Topic == "" {
		return 0, fmt.Errorf("%w: topic is required", ErrInvalidConfig)
	}
	if len(table) == 0 {
		return 0, fmt.Errorf("%w: table is required", ErrInvalidConfig)
	}

	tableName := table.Sanitize()
	q := fmt.Sprintf(
		`INSERT INTO %s (tenant_id, topic, payload, event_id, available_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (event_id) DO UPDATE SET event_id = EXCLUDED.event_id
		 RETURNING sequence`,
		tableName,
	)

	var sequence int64
	if err := tx.QueryRow(ctx, q, msg.TenantID, msg.Topic, msg.Payload, msg.EventID).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("outbox enqueue: %w", err)
	}

	p.m.enqueueTotal.WithLabelValues(TableLabel(table), msg.Topic).Inc()

	return sequence, nil
}
