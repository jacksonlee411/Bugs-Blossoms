package main

import (
	"context"
	"fmt"
	"time"

	"github.com/iota-uz/iota-sdk/pkg/commands/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := common.GetDatabasePool(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("db connect failed: %w", err)
	}
	return pool, nil
}
