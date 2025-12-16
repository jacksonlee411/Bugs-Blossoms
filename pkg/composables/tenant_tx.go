package composables

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/pkg/constants"
)

func InTenantTx(ctx context.Context, fn func(context.Context) error) error {
	if existing, ok := ctx.Value(constants.TxKey).(pgx.Tx); ok && existing != nil {
		if err := ApplyTenantRLS(ctx, existing); err != nil {
			return err
		}
		return fn(ctx)
	}

	pool, err := UsePool(ctx)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	txCtx := WithTx(ctx, tx)
	if err := ApplyTenantRLS(txCtx, tx); err != nil {
		if rErr := tx.Rollback(ctx); rErr != nil {
			return errors.Join(err, rErr)
		}
		return err
	}

	if err := fn(txCtx); err != nil {
		if rErr := tx.Rollback(ctx); rErr != nil {
			return errors.Join(err, rErr)
		}
		return err
	}
	return tx.Commit(ctx)
}

func InTenantTxResult[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var out T
	err := InTenantTx(ctx, func(txCtx context.Context) error {
		var innerErr error
		out, innerErr = fn(txCtx)
		return innerErr
	})
	return out, err
}
